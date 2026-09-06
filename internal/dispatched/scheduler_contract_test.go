package dispatched_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andrewcostello/forecast/internal/dispatched"
	"github.com/andrewcostello/forecast/internal/dispatched/schedulea"
	"github.com/andrewcostello/forecast/internal/dispatched/scheduleb"
)

type scheduleArm func(dispatched.Graph, int) (dispatched.Schedule, error)

type publicScheduleCall struct {
	name string
	call scheduleArm
}

const (
	largeCapChildEnv        = "FC_SCHEDULE_LARGE_CAP_CHILD"
	largeCapChildTimeout    = 20 * time.Second
	largeCapParentTimeout   = 30 * time.Second
	largeCapMemoryLimit     = 256 << 20
	largeCapAllocationLimit = 8 << 20
)

var largeCapChildArm = flag.String("fc-schedule-large-cap-child", "", "internal marker for the isolated scheduler large-cap probe")

// TestFCScheduleAContract is the plan-reserved FC-2A contract group.
func TestFCScheduleAContract(t *testing.T) {
	runScheduleContract(t, "A", schedulea.Schedule, (schedulea.Scheduler{}).Schedule)
}

// TestFCScheduleBContract is the plan-reserved FC-2B contract group.
func TestFCScheduleBContract(t *testing.T) {
	runScheduleContract(t, "B", scheduleb.Schedule, (scheduleb.Scheduler{}).Schedule)
}

func runScheduleContract(t *testing.T, armName string, arm, schedulerMethod scheduleArm) {
	t.Helper()
	publicCalls := []publicScheduleCall{
		{name: "package-function", call: arm},
		{name: "scheduler-method", call: schedulerMethod},
	}

	t.Run("fixture-corpus-integrity", testFixtureCorpusIntegrity)
	t.Run("fixture-loader", testScheduleFixtureLoader)
	t.Run("sentinel-registry", testSchedulerSentinelRegistry)
	t.Run("oracle-zero-drain", testTickOracleZeroDrain)
	t.Run("generated-corpus-shape", testGeneratedCorpusShape)
	t.Run("generated-oracle-domain", testGeneratedOracleDomain)
	t.Run("comparison-mutations", testScheduleComparisonMutations)
	t.Run("fixture-expectation-mutations", testFixtureExpectationMutations)
	t.Run("panel-discriminator-mutations", testPanelDiscriminatorMutations)

	t.Run("hand-fixtures", func(t *testing.T) {
		fixtures := loadHandFixtures(t)
		for _, fixture := range fixtures {
			fixture := fixture
			t.Run(fixture.Name, func(t *testing.T) {
				for _, publicCall := range publicCalls {
					publicCall := publicCall
					t.Run(publicCall.name, func(t *testing.T) {
						graph := fixtureGraph(t, fixture)
						before := cloneGraph(graph)
						got, err := publicCall.call(graph, fixture.Concurrency)
						if failure := fixtureResultFailure(t, fixture, got, err); failure != "" {
							t.Error(failure)
						}
						if !reflect.DeepEqual(graph, before) {
							t.Errorf("input graph mutated\n got: %#v\nwant: %#v", graph, before)
						}
						if fixture.Expect.Error == "" {
							assertPureSchedule(t, publicCall.call, before, fixture.Concurrency, fixtureExpectedSchedule(t, fixture))
						}
					})
				}
			})
		}
	})

	t.Run("generated-corpus", func(t *testing.T) {
		for _, publicCall := range publicCalls {
			publicCall := publicCall
			t.Run(publicCall.name, func(t *testing.T) {
				compared := 0
				err := walkGeneratedCorpus(func(id string, graph dispatched.Graph, maxParallel int) error {
					want := tickOracle(t, graph, maxParallel)
					got, scheduleErr := publicCall.call(cloneGraph(graph), maxParallel)
					if scheduleErr != nil {
						return fmt.Errorf("%s: unexpected error: %w", id, scheduleErr)
					}
					if !schedulesEqual(got, want) {
						return fmt.Errorf("%s: schedule mismatch\n got: %#v\nwant: %#v", id, got, want)
					}
					compared++
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				if compared != generatedCorpusTotal {
					t.Fatalf("compared generated cases = %d, want %d", compared, generatedCorpusTotal)
				}
			})
		}
	})

	t.Run("first-offending-node", func(t *testing.T) {
		testFirstOffendingNodeNames(t, publicCalls)
	})

	t.Run("large-cap-child-only", func(t *testing.T) {
		if !isLargeCapChildProcess(armName) {
			return
		}
		runLargeCapChild(t, arm)
		fmt.Fprintln(os.Stdout, largeCapSuccessToken(armName))
	})

	t.Run("large-cap-storage", func(t *testing.T) {
		runParent, failure := largeCapStorageDecision(armName, os.Getenv(largeCapChildEnv), *largeCapChildArm, os.Args[1:])
		if failure != "" {
			t.Fatal(failure)
		}
		if !runParent {
			return
		}
		runLargeCapProbe(t, armName)
	})
}

func TestFCScheduleLargeCapChildIsolation(t *testing.T) {
	wantFilter := "^TestFCScheduleAContract$/^large-cap-child-only$"
	if got := largeCapChildRunFilter("A"); got != wantFilter {
		t.Fatalf("arm A child filter = %q, want %q", got, wantFilter)
	}
	if got, want := largeCapChildRunFilter("B"), "^TestFCScheduleBContract$/^large-cap-child-only$"; got != want {
		t.Fatalf("arm B child filter = %q, want %q", got, want)
	}
	exactRun := []string{"-test.run=" + wantFilter}
	if isLargeCapChildInvocation("A", "A", "", exactRun) {
		t.Fatal("ambient child environment alone armed the large-cap probe")
	}
	if isLargeCapChildInvocation("A", "A", "A", []string{"-test.run=^TestFCScheduleAContract$"}) {
		t.Fatal("broad contract selector armed the large-cap probe")
	}
	if !isLargeCapMarkedInvocation("A", "A", "A") {
		t.Fatal("explicitly marked child was not recognized for recursion guard")
	}
	if !isLargeCapChildInvocation("A", "A", "A", exactRun) {
		t.Fatal("dedicated child protocol did not arm the large-cap probe")
	}
	if runParent, failure := largeCapStorageDecision("A", "A", "A", exactRun); runParent || failure != "" {
		t.Fatalf("exact child storage decision = (run=%t, failure=%q), want a clean child-only return", runParent, failure)
	}
	broadRun := []string{"-test.run=^TestFCScheduleAContract$"}
	if runParent, failure := largeCapStorageDecision("A", "A", "A", broadRun); runParent || failure == "" {
		t.Fatalf("marked but unarmed storage decision = (run=%t, failure=%q), want a loud failure", runParent, failure)
	}
	if runParent, failure := largeCapStorageDecision("A", "A", "", broadRun); !runParent || failure != "" {
		t.Fatalf("ambient environment storage decision = (run=%t, failure=%q), want parent probe", runParent, failure)
	}
	if isLargeCapChildInvocation("B", "A", "A", exactRun) {
		t.Fatal("arm A child protocol armed arm B")
	}
	if largeCapHandshakePresent([]byte("PASS\n"), "A") {
		t.Fatal("ordinary passing output satisfied the large-cap handshake")
	}
	if largeCapHandshakePresent([]byte(largeCapSuccessToken("B")+"\nPASS\n"), "A") {
		t.Fatal("arm B handshake satisfied arm A")
	}
	if !largeCapHandshakePresent([]byte(largeCapSuccessToken("A")+"\nPASS\n"), "A") {
		t.Fatal("exact arm A handshake was not recognized")
	}
}

func testFirstOffendingNodeNames(t *testing.T, publicCalls []publicScheduleCall) {
	t.Helper()
	const (
		firstKey = "node-7QX4P9"
		laterKey = "node-2VKM8R"
	)
	if strings.Contains(firstKey, laterKey) || strings.Contains(laterKey, firstKey) {
		t.Fatal("first-offender test keys are not collision-safe")
	}

	cases := []struct {
		name         string
		graph        dispatched.Graph
		wantErr      error
		wantFirstKey string
	}{
		{
			name: "duplicate-key",
			graph: dispatched.Graph{Nodes: []dispatched.Node{
				{Key: firstKey},
				{Key: firstKey},
				{Key: laterKey},
				{Key: laterKey},
			}},
			wantErr:      dispatched.ErrDuplicateKey,
			wantFirstKey: firstKey,
		},
		{
			name: "unknown-dependency",
			graph: dispatched.Graph{Nodes: []dispatched.Node{
				{Key: firstKey, BlockedBy: []string{"missing-6NJ3TW"}},
				{Key: laterKey, BlockedBy: []string{"missing-4CYP5H"}},
			}},
			wantErr:      dispatched.ErrUnknownDependency,
			wantFirstKey: firstKey,
		},
		{
			name: "negative-duration",
			graph: dispatched.Graph{Nodes: []dispatched.Node{
				{Key: firstKey, Duration: -time.Nanosecond},
				{Key: laterKey, Duration: -2 * time.Nanosecond},
			}},
			wantErr:      dispatched.ErrNegativeValue,
			wantFirstKey: firstKey,
		},
		{
			name: "cycle",
			graph: dispatched.Graph{Nodes: []dispatched.Node{
				{Key: firstKey, BlockedBy: []string{firstKey}},
				{Key: laterKey, BlockedBy: []string{laterKey}},
			}},
			wantErr:      dispatched.ErrCycle,
			wantFirstKey: firstKey,
		},
		{
			name: "overflow",
			graph: dispatched.Graph{Nodes: []dispatched.Node{
				{Key: "node-9PT6FS", Duration: time.Duration(math.MaxInt64)},
				{Key: firstKey, Duration: time.Nanosecond},
				{Key: laterKey, Duration: time.Nanosecond},
			}},
			wantErr:      dispatched.ErrScheduleOverflow,
			wantFirstKey: firstKey,
		},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			for _, publicCall := range publicCalls {
				publicCall := publicCall
				t.Run(publicCall.name, func(t *testing.T) {
					got, err := publicCall.call(cloneGraph(testCase.graph), 1)
					if err == nil {
						t.Fatalf("error = nil, want %v naming first offending node %q", testCase.wantErr, testCase.wantFirstKey)
					}
					if !errors.Is(err, testCase.wantErr) {
						t.Errorf("error = %v, want errors.Is(%v)", err, testCase.wantErr)
					}
					if !reflect.DeepEqual(got, dispatched.Schedule{}) {
						t.Errorf("error schedule = %#v, want exact zero Schedule", got)
					}
					if !strings.Contains(err.Error(), testCase.wantFirstKey) {
						t.Errorf("error %q does not name first offending node %q", err, testCase.wantFirstKey)
					}
					if strings.Contains(err.Error(), laterKey) {
						t.Errorf("error %q names later offending node %q", err, laterKey)
					}
				})
			}
		})
	}
}

var handFixtureNames = []string{
	"F5-BLANK-DEPENDENCY",
	"F5-BLANK-KEY",
	"F5-BLOCKED-BY-KEY-ORDER",
	"F5-CAP-BINDS",
	"F5-CAP-FREE",
	"F5-CAP-OVER-N",
	"F5-CYCLE",
	"F5-CYCLE-BEFORE-OVERFLOW",
	"F5-DEP-SET-ORDER",
	"F5-DUP-DEPENDENCY",
	"F5-DUPLICATE-KEY",
	"F5-EMPTY",
	"F5-EMPTY-BAD-CAP",
	"F5-EMPTY-KEY",
	"F5-FLOAT-SECONDS-PRECISION",
	"F5-FORK-JOIN-CAP1",
	"F5-FORK-JOIN-FREE",
	"F5-LAST-NODE-KEY-ORDER",
	"F5-MIXED-UNITS",
	"F5-NANOSECOND-GRAIN",
	"F5-NEGATIVE",
	"F5-NEGATIVE-BEFORE-OVERFLOW",
	"F5-NEGATIVE-CAP",
	"F5-OVERFLOW",
	"F5-OVERFLOW-BOUNDARY",
	"F5-OVERFLOW-WRAP",
	"F5-PADDED-KEYS-DISTINCT",
	"F5-PADDED-LOOKUP-EXACT",
	"F5-PRECEDENCE-BLANK",
	"F5-PRECEDENCE-CAP",
	"F5-PRECEDENCE-CYCLE",
	"F5-PRECEDENCE-DUPLICATE",
	"F5-PRECEDENCE-NEGATIVE",
	"F5-PRECEDENCE-UNKNOWN",
	"F5-READY-DECLARATION-ORDER",
	"F5-RESOURCE-SKIPS-ZERO",
	"F5-RESOURCE-SKIPS-ZERO-DECLARED-FIRST",
	"F5-RESOURCE-TIE",
	"F5-RESOURCE-TIE-KEY-ORDER",
	"F5-REVERSE-DECLARATION",
	"F5-SELF-DEPENDENCY",
	"F5-SIMULTANEOUS-COMPLETIONS",
	"F5-UNICODE-BLANK",
	"F5-UNKNOWN-DEPENDENCY",
	"F5-WHITESPACE-DEPENDENCY",
	"F5-ZERO-DECLARATION-ORDER",
	"F5-ZERO-DRAIN",
	"F5-ZERO-RETIRE-BEFORE-READY",
	"F5-ZERO-WAITS-FOR-SLOT",
	"F5-ZERO-WIDTH-NONBLANK",
}

func loadHandFixtures(t *testing.T) []dispatched.ScheduleFixture {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "scheduler", "cases", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	wantPaths := make([]string, len(handFixtureNames))
	for i, name := range handFixtureNames {
		wantPaths[i] = filepath.Join("testdata", "scheduler", "cases", name+".json")
	}
	sort.Strings(wantPaths)
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("fixture corpus names changed\n got: %q\nwant: %q", paths, wantPaths)
	}
	fixtures := make([]dispatched.ScheduleFixture, 0, len(paths))
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		fixture, decodeErr := decodeScheduleFixture(data, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		if decodeErr != nil {
			t.Fatalf("load %s: %v", path, decodeErr)
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

func testFixtureCorpusIntegrity(t *testing.T) {
	const (
		wantFixtureCount      = 50
		wantSuccessCount      = 27
		wantRefusalCount      = 23
		wantCorpusFingerprint = "ed141c28c09d0f76dfc170d4e588259cf33e9c107197b665fd4966a99a7fb4f6"
	)

	fixtures := loadHandFixtures(t)
	if len(fixtures) != wantFixtureCount {
		t.Fatalf("fixture count = %d, want %d", len(fixtures), wantFixtureCount)
	}

	paths, err := filepath.Glob(filepath.Join("testdata", "scheduler", "cases", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s for corpus fingerprint: %v", path, readErr)
		}
		io.WriteString(hash, filepath.Base(path))
		hash.Write([]byte{0})
		hash.Write(data)
		hash.Write([]byte{0})
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != wantCorpusFingerprint {
		t.Fatalf("fixture corpus fingerprint = %s, want %s", got, wantCorpusFingerprint)
	}

	byName := make(map[string]dispatched.ScheduleFixture, len(fixtures))
	successCount, refusalCount := 0, 0
	for _, fixture := range fixtures {
		byName[fixture.Name] = fixture
		if fixture.Expect.Error != "" {
			refusalCount++
			continue
		}
		successCount++
		if failure := fixtureExpectationIntegrityFailure(t, fixture); failure != "" {
			t.Errorf("%s: %s", fixture.Name, failure)
		}
	}
	if successCount != wantSuccessCount || refusalCount != wantRefusalCount {
		t.Fatalf("fixture outcomes = %d success/%d refusal, want %d/%d", successCount, refusalCount, wantSuccessCount, wantRefusalCount)
	}

	capBinds := byName["F5-CAP-BINDS"]
	if capBinds.Concurrency != 1 || len(capBinds.Nodes) != 2 ||
		capBinds.Nodes[0].Key != "A" || capBinds.Nodes[0].Duration != "2s" ||
		capBinds.Nodes[1].Key != "B" || capBinds.Nodes[1].Duration != "3s" {
		t.Fatalf("F5-CAP-BINDS input changed: %#v", capBinds)
	}
	wantCapBinds := dispatched.Schedule{
		Makespan: 5 * time.Second,
		Trace: []dispatched.NodeTrace{
			{Key: "A", Start: 0, Finish: 2 * time.Second},
			{Key: "B", Start: 2 * time.Second, Finish: 5 * time.Second},
		},
		DependencyPath: []string{"B"},
		ExecutionChain: []dispatched.ChainStep{
			{Key: "A", Edge: dispatched.EdgeStart},
			{Key: "B", Edge: dispatched.EdgeResource},
		},
	}
	if got := fixtureExpectedSchedule(t, capBinds); !schedulesEqual(got, wantCapBinds) {
		t.Fatalf("F5-CAP-BINDS expected schedule changed\n got: %#v\nwant: %#v", got, wantCapBinds)
	}

	nanosecondGrain := byName["F5-NANOSECOND-GRAIN"]
	wantNanosecondNodes := []dispatched.FixtureNode{
		{Key: "A", Duration: "1m30s"},
		{Key: "B", Duration: "250ns"},
		{Key: "C", Duration: "1.5s", BlockedBy: []string{"B"}},
	}
	if nanosecondGrain.Concurrency != 1 || !fixtureNodesEqual(nanosecondGrain.Nodes, wantNanosecondNodes) {
		t.Fatalf("F5-NANOSECOND-GRAIN input changed: %#v", nanosecondGrain)
	}
	wantNanosecondSchedule := dispatched.Schedule{
		Makespan: 91500000250 * time.Nanosecond,
		Trace: []dispatched.NodeTrace{
			{Key: "A", Start: 0, Finish: 90 * time.Second},
			{Key: "B", Start: 90 * time.Second, Finish: 90000000250 * time.Nanosecond},
			{Key: "C", Start: 90000000250 * time.Nanosecond, Finish: 91500000250 * time.Nanosecond, BlockedBy: []string{"B"}},
		},
		DependencyPath: []string{"B", "C"},
		ExecutionChain: []dispatched.ChainStep{
			{Key: "A", Edge: dispatched.EdgeStart},
			{Key: "B", Edge: dispatched.EdgeResource},
			{Key: "C", Edge: dispatched.EdgeDependency},
		},
	}
	if got := fixtureExpectedSchedule(t, nanosecondGrain); !schedulesEqual(got, wantNanosecondSchedule) {
		t.Fatalf("F5-NANOSECOND-GRAIN expected schedule changed\n got: %#v\nwant: %#v", got, wantNanosecondSchedule)
	}

	for _, anchor := range []struct {
		name        string
		concurrency int
		nodes       []dispatched.FixtureNode
	}{
		{
			name:        "F5-LAST-NODE-KEY-ORDER",
			concurrency: 2,
			nodes: []dispatched.FixtureNode{
				{Key: "B", Duration: "2s"},
				{Key: "A", Duration: "2s"},
			},
		},
		{
			name:        "F5-RESOURCE-TIE-KEY-ORDER",
			concurrency: 2,
			nodes: []dispatched.FixtureNode{
				{Key: "B", Duration: "3s"},
				{Key: "A", Duration: "3s"},
				{Key: "C", Duration: "1s"},
			},
		},
		{
			name:        "F5-BLOCKED-BY-KEY-ORDER",
			concurrency: 2,
			nodes: []dispatched.FixtureNode{
				{Key: "B", Duration: "1s"},
				{Key: "A", Duration: "1s"},
				{Key: "C", Duration: "1s", BlockedBy: []string{"A", "B"}},
			},
		},
	} {
		fixture, ok := byName[anchor.name]
		if !ok {
			t.Errorf("required key-order fixture %s is absent", anchor.name)
			continue
		}
		if fixture.Concurrency != anchor.concurrency || !fixtureNodesEqual(fixture.Nodes, anchor.nodes) {
			t.Errorf("%s input changed: %#v", anchor.name, fixture)
		}
	}

	for _, expectation := range []struct {
		name, sentinel string
	}{
		{name: "F5-OVERFLOW", sentinel: "ErrScheduleOverflow"},
		{name: "F5-OVERFLOW-WRAP", sentinel: "ErrScheduleOverflow"},
		{name: "F5-NEGATIVE-BEFORE-OVERFLOW", sentinel: "ErrNegativeValue"},
		{name: "F5-NEGATIVE-CAP", sentinel: "ErrInvalidConcurrency"},
		{name: "F5-CYCLE-BEFORE-OVERFLOW", sentinel: "ErrCycle"},
	} {
		if got := byName[expectation.name].Expect.Error; got != expectation.sentinel {
			t.Errorf("%s error = %q, want %q", expectation.name, got, expectation.sentinel)
		}
	}
	assertFixtureDurationLiterals(t, byName, "F5-OVERFLOW",
		[]string{"4611686018427387904ns", "4611686018427387904ns"},
		[]time.Duration{1 << 62, 1 << 62})
	assertFixtureDurationLiterals(t, byName, "F5-OVERFLOW-WRAP",
		[]string{"9223372036854775807ns", "9223372036854775807ns", "5ns"},
		[]time.Duration{time.Duration(math.MaxInt64), time.Duration(math.MaxInt64), 5})
	assertFixtureDurationLiterals(t, byName, "F5-NEGATIVE-BEFORE-OVERFLOW",
		[]string{"9223372036854775807ns", "9223372036854775807ns", "-1ns"},
		[]time.Duration{time.Duration(math.MaxInt64), time.Duration(math.MaxInt64), -1})
	assertFixtureDurationLiterals(t, byName, "F5-CYCLE-BEFORE-OVERFLOW",
		[]string{"9223372036854775807ns", "9223372036854775807ns"},
		[]time.Duration{time.Duration(math.MaxInt64), time.Duration(math.MaxInt64)})
	assertFixtureDurationLiterals(t, byName, "F5-OVERFLOW-BOUNDARY",
		[]string{"9223372036854775807ns"},
		[]time.Duration{time.Duration(math.MaxInt64)})
	assertFixtureDurationLiterals(t, byName, "F5-NANOSECOND-GRAIN",
		[]string{"1m30s", "250ns", "1.5s"},
		[]time.Duration{90 * time.Second, 250, 1500000000})
	assertFixtureDurationLiterals(t, byName, "F5-FLOAT-SECONDS-PRECISION",
		[]string{"130841899126ns", "250ns", "1.5s"},
		[]time.Duration{130841899126, 250, 1500000000})
	negativeCap := byName["F5-NEGATIVE-CAP"]
	if negativeCap.Concurrency != -1 || len(negativeCap.Nodes) != 0 {
		t.Fatalf("F5-NEGATIVE-CAP input changed: %#v", negativeCap)
	}
}

func fixtureExpectationIntegrityFailure(t *testing.T, fixture dispatched.ScheduleFixture) string {
	t.Helper()
	graph := fixtureGraph(t, fixture)
	want := fixtureExpectedSchedule(t, fixture)
	if len(want.Trace) != len(graph.Nodes) {
		return fmt.Sprintf("trace length = %d, want %d", len(want.Trace), len(graph.Nodes))
	}
	if fixture.Concurrency < 1 {
		return fmt.Sprintf("successful fixture has invalid concurrency %d", fixture.Concurrency)
	}
	if len(graph.Nodes) == 0 {
		if want.Makespan != 0 || len(want.DependencyPath) != 0 || len(want.ExecutionChain) != 0 {
			return fmt.Sprintf("empty graph has nonzero/nonempty expected output: %#v", want)
		}
		return ""
	}
	if len(want.DependencyPath) == 0 || len(want.ExecutionChain) == 0 {
		return "nonempty success must record both dependency path and execution chain"
	}

	index := make(map[string]int, len(graph.Nodes))
	for i, node := range graph.Nodes {
		index[node.Key] = i
	}
	dependencies := make([][]int, len(graph.Nodes))
	maxFinish := time.Duration(0)
	lastNode := 0
	for i, trace := range want.Trace {
		node := graph.Nodes[i]
		if trace.Key != node.Key {
			return fmt.Sprintf("trace[%d] key = %q, want declaration key %q", i, trace.Key, node.Key)
		}
		if trace.Start < 0 || trace.Finish < trace.Start || trace.Finish-trace.Start != node.Duration {
			return fmt.Sprintf("trace[%d] interval %s..%s does not encode duration %s", i, trace.Start, trace.Finish, node.Duration)
		}
		seenDependencies := make([]bool, len(graph.Nodes))
		for _, dependency := range node.BlockedBy {
			dependencyIndex, ok := index[dependency]
			if !ok {
				return fmt.Sprintf("trace[%d] source node contains unknown dependency %q", i, dependency)
			}
			seenDependencies[dependencyIndex] = true
		}
		var wantBlockedBy []string
		for dependencyIndex, seen := range seenDependencies {
			if seen {
				dependencies[i] = append(dependencies[i], dependencyIndex)
				wantBlockedBy = append(wantBlockedBy, graph.Nodes[dependencyIndex].Key)
			}
		}
		if !stringSequencesEqual(trace.BlockedBy, wantBlockedBy) {
			return fmt.Sprintf("trace[%d] BlockedBy = %q, want distinct dependencies in declaration order %q", i, trace.BlockedBy, wantBlockedBy)
		}
		if trace.Finish > maxFinish {
			maxFinish = trace.Finish
			lastNode = i
		}
	}
	if want.Makespan != maxFinish {
		return fmt.Sprintf("makespan = %s, want maximum trace finish %s", want.Makespan, maxFinish)
	}
	if failure := fixtureScheduleProcessFailure(graph, fixture.Concurrency, want.Trace, dependencies); failure != "" {
		return failure
	}
	lastKey := graph.Nodes[lastNode].Key

	dependencyDuration := time.Duration(0)
	for i, key := range want.DependencyPath {
		nodeIndex, ok := index[key]
		if !ok {
			return fmt.Sprintf("dependency path contains unknown key %q", key)
		}
		dependencyDuration += graph.Nodes[nodeIndex].Duration
		if i > 0 && !containsString(graph.Nodes[nodeIndex].BlockedBy, want.DependencyPath[i-1]) {
			return fmt.Sprintf("dependency path edge %q -> %q is not in the graph", want.DependencyPath[i-1], key)
		}
	}
	lastDependencyKey := want.DependencyPath[len(want.DependencyPath)-1]
	if lastDependencyKey != lastKey {
		return fmt.Sprintf("dependency path terminates at %q, want last node %q", lastDependencyKey, lastKey)
	}
	lastDependency := index[lastDependencyKey]
	if want.Trace[lastDependency].Finish != want.Makespan || dependencyDuration > want.Makespan {
		return fmt.Sprintf("dependency path does not terminate at makespan: path duration %s, finish %s, makespan %s", dependencyDuration, want.Trace[lastDependency].Finish, want.Makespan)
	}
	derivedDependencyPath, failure := fixtureDependencyPath(want.Trace, dependencies, lastNode)
	if failure != "" {
		return failure
	}
	if !stringSequencesEqual(want.DependencyPath, derivedDependencyPath) {
		return fmt.Sprintf("dependency path = %q, want frozen predecessor path %q", want.DependencyPath, derivedDependencyPath)
	}

	executionDuration := time.Duration(0)
	for i, step := range want.ExecutionChain {
		nodeIndex, ok := index[step.Key]
		if !ok {
			return fmt.Sprintf("execution chain contains unknown key %q", step.Key)
		}
		executionDuration += graph.Nodes[nodeIndex].Duration
		if i == 0 {
			if step.Edge != dispatched.EdgeStart {
				return fmt.Sprintf("execution chain head edge = %q, want start", step.Edge)
			}
			continue
		}
		previous := want.ExecutionChain[i-1]
		previousIndex := index[previous.Key]
		if want.Trace[previousIndex].Finish != want.Trace[nodeIndex].Start {
			return fmt.Sprintf("execution edge %q -> %q does not join at one timestamp", previous.Key, step.Key)
		}
		switch step.Edge {
		case dispatched.EdgeDependency:
			if !containsString(graph.Nodes[nodeIndex].BlockedBy, previous.Key) {
				return fmt.Sprintf("execution dependency edge %q -> %q is not in the graph", previous.Key, step.Key)
			}
		case dispatched.EdgeResource:
			if graph.Nodes[previousIndex].Duration <= 0 {
				return fmt.Sprintf("resource predecessor %q has nonpositive duration", previous.Key)
			}
		default:
			return fmt.Sprintf("execution edge[%d] = %q", i, step.Edge)
		}
	}
	lastExecutionKey := want.ExecutionChain[len(want.ExecutionChain)-1].Key
	if lastExecutionKey != lastKey {
		return fmt.Sprintf("execution chain terminates at %q, want last node %q", lastExecutionKey, lastKey)
	}
	lastExecution := index[lastExecutionKey]
	if want.Trace[lastExecution].Finish != want.Makespan || executionDuration != want.Makespan {
		return fmt.Sprintf("execution chain does not explain makespan: duration %s, finish %s, makespan %s", executionDuration, want.Trace[lastExecution].Finish, want.Makespan)
	}
	derivedExecutionChain, failure := fixtureExecutionChain(graph, want.Trace, dependencies, lastNode)
	if failure != "" {
		return failure
	}
	if !reflect.DeepEqual(want.ExecutionChain, derivedExecutionChain) {
		return fmt.Sprintf("execution chain = %#v, want frozen predecessor chain %#v", want.ExecutionChain, derivedExecutionChain)
	}
	if anchor, ok := keyOrderScheduleAnchor(fixture.Name); ok && !schedulesEqual(want, anchor) {
		return fmt.Sprintf("key-order expectation differs from frozen anchor\n got: %#v\nwant: %#v", want, anchor)
	}
	return ""
}

func fixtureScheduleProcessFailure(graph dispatched.Graph, maxParallel int, trace []dispatched.NodeTrace, dependencies [][]int) string {
	if maxParallel < 1 {
		return fmt.Sprintf("successful fixture has invalid concurrency %d", maxParallel)
	}
	pending := make([]bool, len(graph.Nodes))
	running := make([]bool, len(graph.Nodes))
	completed := make([]bool, len(graph.Nodes))
	for i := range pending {
		pending[i] = true
	}
	completedCount, runningCount := 0, 0
	now := time.Duration(0)
	for completedCount < len(graph.Nodes) {
		for i := range running {
			if running[i] && trace[i].Finish < now {
				return fmt.Sprintf("trace[%d] remained running past finish %s at %s", i, trace[i].Finish, now)
			}
			if running[i] && trace[i].Finish == now {
				running[i] = false
				completed[i] = true
				runningCount--
				completedCount++
			}
		}

		for i := range pending {
			if !pending[i] || runningCount >= maxParallel || !fixtureDependenciesComplete(dependencies[i], completed) {
				continue
			}
			if trace[i].Start != now {
				return fmt.Sprintf("trace[%d] starts at %s, but COMPLETE/FILL starts it at %s", i, trace[i].Start, now)
			}
			pending[i] = false
			running[i] = true
			runningCount++
		}

		zeroRunning := false
		for i := range running {
			zeroRunning = zeroRunning || running[i] && trace[i].Finish == now
		}
		if zeroRunning {
			continue
		}
		for i := range pending {
			if pending[i] && trace[i].Start <= now {
				return fmt.Sprintf("trace[%d] starts at %s without readiness and slot availability in the FILL phase", i, trace[i].Start)
			}
		}
		if completedCount == len(graph.Nodes) {
			break
		}
		if runningCount == 0 {
			return "expected schedule makes no progress with pending nodes"
		}
		next := time.Duration(math.MaxInt64)
		for i := range running {
			if running[i] && trace[i].Finish < next {
				next = trace[i].Finish
			}
		}
		if next <= now {
			return fmt.Sprintf("expected schedule cannot advance from %s to %s", now, next)
		}
		now = next
	}
	return ""
}

func fixtureDependenciesComplete(dependencies []int, completed []bool) bool {
	for _, dependency := range dependencies {
		if !completed[dependency] {
			return false
		}
	}
	return true
}

func fixtureDependencyPath(trace []dispatched.NodeTrace, dependencies [][]int, last int) ([]string, string) {
	reverse := make([]string, 0, len(trace))
	seen := make([]bool, len(trace))
	current := last
	for {
		if seen[current] {
			return nil, "dependency predecessor reconstruction encountered a cycle"
		}
		seen[current] = true
		reverse = append(reverse, trace[current].Key)
		if len(dependencies[current]) == 0 {
			break
		}
		predecessor := dependencies[current][0]
		for _, dependency := range dependencies[current][1:] {
			if trace[dependency].Finish > trace[predecessor].Finish {
				predecessor = dependency
			}
		}
		current = predecessor
	}
	reverseStrings(reverse)
	return reverse, ""
}

func fixtureExecutionChain(graph dispatched.Graph, trace []dispatched.NodeTrace, dependencies [][]int, last int) ([]dispatched.ChainStep, string) {
	reverse := make([]dispatched.ChainStep, 0, len(trace))
	seen := make([]bool, len(trace))
	current := last
	for {
		if seen[current] {
			return nil, "execution predecessor reconstruction encountered a cycle"
		}
		seen[current] = true
		deps := dependencies[current]
		depReady := time.Duration(0)
		for _, dependency := range deps {
			if trace[dependency].Finish > depReady {
				depReady = trace[dependency].Finish
			}
		}
		switch {
		case len(deps) == 0 && trace[current].Start == 0:
			reverse = append(reverse, dispatched.ChainStep{Key: trace[current].Key, Edge: dispatched.EdgeStart})
			reverseChainSteps(reverse)
			return reverse, ""
		case len(deps) > 0 && trace[current].Start == depReady:
			predecessor := -1
			for _, dependency := range deps {
				if trace[dependency].Finish == depReady {
					predecessor = dependency
					break
				}
			}
			if predecessor < 0 {
				return nil, fmt.Sprintf("no dependency predecessor for %q", trace[current].Key)
			}
			reverse = append(reverse, dispatched.ChainStep{Key: trace[current].Key, Edge: dispatched.EdgeDependency})
			current = predecessor
		case trace[current].Start > depReady:
			predecessor := -1
			for i, candidate := range trace {
				if candidate.Finish == trace[current].Start && graph.Nodes[i].Duration > 0 {
					predecessor = i
					break
				}
			}
			if predecessor < 0 {
				return nil, fmt.Sprintf("no resource predecessor for %q", trace[current].Key)
			}
			reverse = append(reverse, dispatched.ChainStep{Key: trace[current].Key, Edge: dispatched.EdgeResource})
			current = predecessor
		default:
			return nil, fmt.Sprintf("execution predecessor cases are not exhaustive for %q", trace[current].Key)
		}
	}
}

func reverseChainSteps(values []dispatched.ChainStep) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func keyOrderScheduleAnchor(name string) (dispatched.Schedule, bool) {
	switch name {
	case "F5-LAST-NODE-KEY-ORDER":
		return dispatched.Schedule{
			Makespan:       2 * time.Second,
			Trace:          []dispatched.NodeTrace{{Key: "B", Finish: 2 * time.Second}, {Key: "A", Finish: 2 * time.Second}},
			DependencyPath: []string{"B"},
			ExecutionChain: []dispatched.ChainStep{{Key: "B", Edge: dispatched.EdgeStart}},
		}, true
	case "F5-RESOURCE-TIE-KEY-ORDER":
		return dispatched.Schedule{
			Makespan: 4 * time.Second,
			Trace: []dispatched.NodeTrace{
				{Key: "B", Finish: 3 * time.Second},
				{Key: "A", Finish: 3 * time.Second},
				{Key: "C", Start: 3 * time.Second, Finish: 4 * time.Second},
			},
			DependencyPath: []string{"C"},
			ExecutionChain: []dispatched.ChainStep{
				{Key: "B", Edge: dispatched.EdgeStart},
				{Key: "C", Edge: dispatched.EdgeResource},
			},
		}, true
	case "F5-BLOCKED-BY-KEY-ORDER":
		return dispatched.Schedule{
			Makespan: 2 * time.Second,
			Trace: []dispatched.NodeTrace{
				{Key: "B", Finish: time.Second},
				{Key: "A", Finish: time.Second},
				{Key: "C", Start: time.Second, Finish: 2 * time.Second, BlockedBy: []string{"B", "A"}},
			},
			DependencyPath: []string{"B", "C"},
			ExecutionChain: []dispatched.ChainStep{
				{Key: "B", Edge: dispatched.EdgeStart},
				{Key: "C", Edge: dispatched.EdgeDependency},
			},
		}, true
	default:
		return dispatched.Schedule{}, false
	}
}

func assertFixtureDurationLiterals(t *testing.T, fixtures map[string]dispatched.ScheduleFixture, name string, wantLiterals []string, wantValues []time.Duration) {
	t.Helper()
	fixture, ok := fixtures[name]
	if !ok {
		t.Fatalf("required fixture %s is absent", name)
	}
	if len(fixture.Nodes) != len(wantLiterals) || len(wantLiterals) != len(wantValues) {
		t.Fatalf("%s node count = %d, want %d", name, len(fixture.Nodes), len(wantLiterals))
	}
	for i, node := range fixture.Nodes {
		if node.Duration != wantLiterals[i] {
			t.Errorf("%s node[%d] duration literal = %q, want %q", name, i, node.Duration, wantLiterals[i])
			continue
		}
		parsed, err := time.ParseDuration(node.Duration)
		if err != nil || parsed != wantValues[i] {
			t.Errorf("%s node[%d] parsed duration = (%s, %v), want %dns", name, i, parsed, err, wantValues[i])
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringSequencesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func fixtureNodesEqual(left, right []dispatched.FixtureNode) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].Key != right[i].Key || left[i].Duration != right[i].Duration || !stringSequencesEqual(left[i].BlockedBy, right[i].BlockedBy) {
			return false
		}
	}
	return true
}

func testScheduleFixtureLoader(t *testing.T) {
	validEmpty := []byte(`{"name":"valid-empty","provenance":"synthetic","concurrency":1,"nodes":null,"expect":{"error":"","makespan":"0s","trace":null,"dependency_path":null,"execution_chain":null}}`)
	validSentinel := []byte(`{"name":"valid-sentinel","provenance":"synthetic","concurrency":0,"expect":{"error":"ErrInvalidConcurrency","makespan":"0s"}}`)
	for name, data := range map[string][]byte{"valid-empty": validEmpty, "valid-sentinel": validSentinel} {
		if _, err := decodeScheduleFixture(data, name); err != nil {
			t.Errorf("valid control %s: %v", name, err)
		}
	}

	invalid := map[string]string{
		"missing-name":             `{"provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s"}}`,
		"null-name":                `{"name":null,"provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s"}}`,
		"null-provenance":          `{"name":"bad","provenance":null,"concurrency":1,"expect":{"error":"","makespan":"0s"}}`,
		"null-concurrency":         `{"name":"bad","provenance":"synthetic","concurrency":null,"expect":{"error":"","makespan":"0s"}}`,
		"wrong-concurrency-type":   `{"name":"bad","provenance":"synthetic","concurrency":"1","expect":{"error":"","makespan":"0s"}}`,
		"wrong-note-type":          `{"name":"bad","provenance":"synthetic","note":3,"concurrency":1,"expect":{"error":"","makespan":"0s"}}`,
		"missing-expect":           `{"name":"bad","provenance":"synthetic","concurrency":1}`,
		"null-expect":              `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":null}`,
		"missing-node-key":         `{"name":"bad","provenance":"synthetic","concurrency":1,"nodes":[{"duration":"1s"}],"expect":{"error":"","makespan":"1s"}}`,
		"missing-node-duration":    `{"name":"bad","provenance":"synthetic","concurrency":1,"nodes":[{"key":"A"}],"expect":{"error":"","makespan":"1s"}}`,
		"null-node-key":            `{"name":"bad","provenance":"synthetic","concurrency":1,"nodes":[{"key":null,"duration":"1s"}],"expect":{"error":"","makespan":"1s"}}`,
		"null-node-duration":       `{"name":"bad","provenance":"synthetic","concurrency":1,"nodes":[{"key":"A","duration":null}],"expect":{"error":"","makespan":"1s"}}`,
		"wrong-node-key-type":      `{"name":"bad","provenance":"synthetic","concurrency":1,"nodes":[{"key":1,"duration":"1s"}],"expect":{"error":"","makespan":"1s"}}`,
		"wrong-duration-type":      `{"name":"bad","provenance":"synthetic","concurrency":1,"nodes":[{"key":"A","duration":1}],"expect":{"error":"","makespan":"1s"}}`,
		"null-node":                `{"name":"bad","provenance":"synthetic","concurrency":1,"nodes":[null],"expect":{"error":"","makespan":"0s"}}`,
		"wrong-nodes-type":         `{"name":"bad","provenance":"synthetic","concurrency":1,"nodes":{},"expect":{"error":"","makespan":"0s"}}`,
		"null-blocked-by-entry":    `{"name":"bad","provenance":"synthetic","concurrency":1,"nodes":[{"key":"A","duration":"1s","blocked_by":[null]}],"expect":{"error":"","makespan":"1s"}}`,
		"missing-error":            `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"makespan":"0s"}}`,
		"null-error":               `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":null,"makespan":"0s"}}`,
		"wrong-error-type":         `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":false,"makespan":"0s"}}`,
		"unknown-error":            `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"ErrScheduleOverflowed"}}`,
		"missing-success-makespan": `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":""}}`,
		"null-success-makespan":    `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":null}}`,
		"wrong-makespan-type":      `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":0}}`,
		"bad-duration":             `{"name":"bad","provenance":"synthetic","concurrency":1,"nodes":[{"key":"A","duration":"soon"}],"expect":{"error":"","makespan":"0s"}}`,
		"missing-trace-key":        `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","trace":[{"start":"0s","finish":"0s"}]}}`,
		"missing-trace-start":      `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","trace":[{"key":"A","finish":"0s"}]}}`,
		"missing-trace-finish":     `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","trace":[{"key":"A","start":"0s"}]}}`,
		"null-trace-start":         `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","trace":[{"key":"A","start":null,"finish":"0s"}]}}`,
		"null-trace-entry":         `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","trace":[null]}}`,
		"wrong-trace-type":         `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","trace":{}}}`,
		"null-path-entry":          `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","dependency_path":[null]}}`,
		"missing-chain-key":        `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","execution_chain":[{"edge":"start"}]}}`,
		"missing-chain-edge":       `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","execution_chain":[{"key":"A"}]}}`,
		"null-chain-edge":          `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","execution_chain":[{"key":"A","edge":null}]}}`,
		"null-chain-entry":         `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","execution_chain":[null]}}`,
		"invalid-edge":             `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","execution_chain":[{"key":"A","edge":"wait"}]}}`,
		"invalid-provenance":       `{"name":"bad","provenance":"recorded","concurrency":1,"expect":{"error":"","makespan":"0s"}}`,
		"filename-disagreement":    `{"name":"different","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s"}}`,
		"duplicate-key":            `{"name":"bad","name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s"}}`,
		"duplicate-nested-key":     `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","error":"","makespan":"0s"}}`,
		"unknown-top-key":          `{"name":"bad","provenance":"synthetic","concurrency":1,"surprise":true,"expect":{"error":"","makespan":"0s"}}`,
		"unknown-node-key":         `{"name":"bad","provenance":"synthetic","concurrency":1,"nodes":[{"key":"A","duration":"1s","surprise":true}],"expect":{"error":"","makespan":"1s"}}`,
		"unknown-expect-key":       `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","surprise":true}}`,
		"unknown-trace-key":        `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","trace":[{"key":"A","start":"0s","finish":"0s","surprise":true}]}}`,
		"unknown-chain-key":        `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s","execution_chain":[{"key":"A","edge":"start","surprise":true}]}}`,
		"error-nonzero-makespan":   `{"name":"bad","provenance":"synthetic","concurrency":0,"expect":{"error":"ErrInvalidConcurrency","makespan":"1ns"}}`,
		"error-nonempty-output":    `{"name":"bad","provenance":"synthetic","concurrency":0,"expect":{"error":"ErrInvalidConcurrency","dependency_path":["A"]}}`,
		"trailing-document":        `{"name":"bad","provenance":"synthetic","concurrency":1,"expect":{"error":"","makespan":"0s"}} {}`,
	}
	for name, raw := range invalid {
		t.Run(name, func(t *testing.T) {
			called := false
			err := decodeThen(raw, "bad", func(dispatched.ScheduleFixture) { called = true })
			if err == nil {
				t.Fatal("loader accepted invalid fixture")
			}
			if called {
				t.Fatal("scheduler callback reached after loader failure")
			}
		})
	}
}

func decodeThen(raw, stem string, call func(dispatched.ScheduleFixture)) error {
	fixture, err := decodeScheduleFixture([]byte(raw), stem)
	if err != nil {
		return err
	}
	call(fixture)
	return nil
}

func testSchedulerSentinelRegistry(t *testing.T) {
	wantNames := []string{"ErrInvalidConcurrency", "ErrBlankKey", "ErrDuplicateKey", "ErrUnknownDependency", "ErrNegativeValue", "ErrCycle", "ErrScheduleOverflow"}
	first := dispatched.SchedulerSentinels()
	if len(first) != len(wantNames) {
		t.Fatalf("sentinel count = %d, want %d", len(first), len(wantNames))
	}
	for i, want := range wantNames {
		if first[i].Name != want || first[i].Err == nil {
			t.Fatalf("sentinel[%d] = %#v, want %s with non-nil error", i, first[i], want)
		}
	}
	first[0] = dispatched.SchedulerSentinel{Name: "mutated", Err: errors.New("mutated")}
	second := dispatched.SchedulerSentinels()
	if second[0].Name != wantNames[0] || !errors.Is(second[0].Err, dispatched.ErrInvalidConcurrency) {
		t.Fatalf("registry accessor leaked caller mutation: %#v", second[0])
	}
	for _, entry := range second {
		got, ok := dispatched.LookupSchedulerSentinel(entry.Name)
		if !ok || !errors.Is(got, entry.Err) {
			t.Errorf("lookup %s = (%v, %v), want registry sentinel", entry.Name, got, ok)
		}
	}
	for _, unknown := range []string{"", "ErrScheduleOverflowed", "ErrNotImplemented"} {
		if got, ok := dispatched.LookupSchedulerSentinel(unknown); ok || got != nil {
			t.Errorf("lookup %q = (%v, %v), want (nil, false)", unknown, got, ok)
		}
	}
}

type rawObject map[string]json.RawMessage

func decodeScheduleFixture(data []byte, stem string) (dispatched.ScheduleFixture, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return dispatched.ScheduleFixture{}, err
	}
	root, err := decodeObject(data, "fixture", []string{"name", "provenance", "note", "concurrency", "nodes", "expect"}, []string{"name", "provenance", "concurrency", "expect"})
	if err != nil {
		return dispatched.ScheduleFixture{}, err
	}
	var fixture dispatched.ScheduleFixture
	if fixture.Name, err = requiredString(root, "name", "fixture"); err != nil {
		return fixture, err
	}
	if fixture.Name != stem {
		return fixture, fmt.Errorf("fixture name %q does not match filename stem %q", fixture.Name, stem)
	}
	if fixture.Provenance, err = requiredString(root, "provenance", "fixture"); err != nil {
		return fixture, err
	}
	if fixture.Provenance != "synthetic" {
		return fixture, fmt.Errorf("fixture provenance = %q, want synthetic", fixture.Provenance)
	}
	if raw, ok := root["note"]; ok {
		if fixture.Note, err = decodeString(raw, "fixture.note"); err != nil {
			return fixture, err
		}
	}
	if fixture.Concurrency, err = requiredInt(root, "concurrency", "fixture"); err != nil {
		return fixture, err
	}
	if fixture.Nodes, err = decodeFixtureNodes(root["nodes"]); err != nil {
		return fixture, err
	}
	if fixture.Expect, err = decodeFixtureExpectation(root["expect"]); err != nil {
		return fixture, err
	}
	return fixture, nil
}

func decodeFixtureNodes(raw json.RawMessage) ([]dispatched.FixtureNode, error) {
	items, err := optionalArray(raw, "fixture.nodes")
	if err != nil {
		return nil, err
	}
	nodes := make([]dispatched.FixtureNode, 0, len(items))
	for i, item := range items {
		where := fmt.Sprintf("fixture.nodes[%d]", i)
		object, objectErr := decodeObject(item, where, []string{"key", "duration", "blocked_by"}, []string{"key", "duration"})
		if objectErr != nil {
			return nil, objectErr
		}
		key, keyErr := requiredString(object, "key", where)
		if keyErr != nil {
			return nil, keyErr
		}
		duration, durationErr := requiredDurationString(object, "duration", where)
		if durationErr != nil {
			return nil, durationErr
		}
		blockedBy, blockedErr := optionalStringArray(object["blocked_by"], where+".blocked_by")
		if blockedErr != nil {
			return nil, blockedErr
		}
		nodes = append(nodes, dispatched.FixtureNode{Key: key, Duration: duration, BlockedBy: blockedBy})
	}
	return nodes, nil
}

func decodeFixtureExpectation(raw json.RawMessage) (dispatched.FixtureExpectation, error) {
	object, err := decodeObject(raw, "fixture.expect", []string{"error", "makespan", "trace", "dependency_path", "execution_chain"}, []string{"error"})
	if err != nil {
		return dispatched.FixtureExpectation{}, err
	}
	var expect dispatched.FixtureExpectation
	if expect.Error, err = requiredString(object, "error", "fixture.expect"); err != nil {
		return expect, err
	}
	if expect.Error != "" {
		if _, ok := dispatched.LookupSchedulerSentinel(expect.Error); !ok {
			return expect, fmt.Errorf("fixture.expect.error %q is unknown", expect.Error)
		}
	}
	if rawMakespan, ok := object["makespan"]; ok {
		if expect.Makespan, err = decodeDurationString(rawMakespan, "fixture.expect.makespan"); err != nil {
			return expect, err
		}
	} else if expect.Error == "" {
		return expect, errors.New("fixture.expect.makespan is required for success")
	}
	if expect.Trace, err = decodeFixtureTrace(object["trace"]); err != nil {
		return expect, err
	}
	if expect.DependencyPath, err = optionalStringArray(object["dependency_path"], "fixture.expect.dependency_path"); err != nil {
		return expect, err
	}
	if expect.ExecutionChain, err = decodeFixtureChain(object["execution_chain"]); err != nil {
		return expect, err
	}
	if expect.Error != "" {
		if expect.Makespan != "" {
			makespan, parseErr := time.ParseDuration(expect.Makespan)
			if parseErr != nil || makespan != 0 {
				return expect, errors.New("error fixture makespan must be absent or zero")
			}
		}
		if len(expect.Trace) != 0 || len(expect.DependencyPath) != 0 || len(expect.ExecutionChain) != 0 {
			return expect, errors.New("error fixture outputs must be empty")
		}
	}
	return expect, nil
}

func decodeFixtureTrace(raw json.RawMessage) ([]dispatched.FixtureTrace, error) {
	items, err := optionalArray(raw, "fixture.expect.trace")
	if err != nil {
		return nil, err
	}
	trace := make([]dispatched.FixtureTrace, 0, len(items))
	for i, item := range items {
		where := fmt.Sprintf("fixture.expect.trace[%d]", i)
		object, objectErr := decodeObject(item, where, []string{"key", "start", "finish", "blocked_by"}, []string{"key", "start", "finish"})
		if objectErr != nil {
			return nil, objectErr
		}
		key, keyErr := requiredString(object, "key", where)
		if keyErr != nil {
			return nil, keyErr
		}
		start, startErr := requiredDurationString(object, "start", where)
		if startErr != nil {
			return nil, startErr
		}
		finish, finishErr := requiredDurationString(object, "finish", where)
		if finishErr != nil {
			return nil, finishErr
		}
		blockedBy, blockedErr := optionalStringArray(object["blocked_by"], where+".blocked_by")
		if blockedErr != nil {
			return nil, blockedErr
		}
		trace = append(trace, dispatched.FixtureTrace{Key: key, Start: start, Finish: finish, BlockedBy: blockedBy})
	}
	return trace, nil
}

func decodeFixtureChain(raw json.RawMessage) ([]dispatched.FixtureChainStep, error) {
	items, err := optionalArray(raw, "fixture.expect.execution_chain")
	if err != nil {
		return nil, err
	}
	chain := make([]dispatched.FixtureChainStep, 0, len(items))
	for i, item := range items {
		where := fmt.Sprintf("fixture.expect.execution_chain[%d]", i)
		object, objectErr := decodeObject(item, where, []string{"key", "edge"}, []string{"key", "edge"})
		if objectErr != nil {
			return nil, objectErr
		}
		key, keyErr := requiredString(object, "key", where)
		if keyErr != nil {
			return nil, keyErr
		}
		edge, edgeErr := requiredString(object, "edge", where)
		if edgeErr != nil {
			return nil, edgeErr
		}
		kind := dispatched.EdgeKind(edge)
		if kind != dispatched.EdgeStart && kind != dispatched.EdgeDependency && kind != dispatched.EdgeResource {
			return nil, fmt.Errorf("%s.edge = %q, want start, dependency, or resource", where, edge)
		}
		chain = append(chain, dispatched.FixtureChainStep{Key: key, Edge: kind})
	}
	return chain, nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, keyErr := decoder.Token()
				if keyErr != nil {
					return keyErr
				}
				key, keyOK := keyToken.(string)
				if !keyOK {
					return errors.New("JSON object key is not a string")
				}
				if _, duplicate := seen[key]; duplicate {
					return fmt.Errorf("duplicate JSON object key %q", key)
				}
				seen[key] = struct{}{}
				if valueErr := walk(); valueErr != nil {
					return valueErr
				}
			}
			end, endErr := decoder.Token()
			if endErr != nil || end != json.Delim('}') {
				return fmt.Errorf("malformed JSON object: %v", endErr)
			}
		case '[':
			for decoder.More() {
				if valueErr := walk(); valueErr != nil {
					return valueErr
				}
			}
			end, endErr := decoder.Token()
			if endErr != nil || end != json.Delim(']') {
				return fmt.Errorf("malformed JSON array: %v", endErr)
			}
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delim)
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("trailing JSON value %v", token)
	}
	return nil
}

func decodeObject(raw []byte, where string, allowed, required []string) (rawObject, error) {
	if isJSONNull(raw) {
		return nil, fmt.Errorf("%s must be a non-null object", where)
	}
	var object rawObject
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be an object: %v", where, err)
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	for key := range object {
		if _, ok := allowedSet[key]; !ok {
			return nil, fmt.Errorf("%s has unknown key %q", where, key)
		}
	}
	for _, key := range required {
		if _, ok := object[key]; !ok {
			return nil, fmt.Errorf("%s.%s is required", where, key)
		}
	}
	return object, nil
}

func requiredString(object rawObject, key, where string) (string, error) {
	raw, ok := object[key]
	if !ok {
		return "", fmt.Errorf("%s.%s is required", where, key)
	}
	return decodeString(raw, where+"."+key)
}

func decodeString(raw json.RawMessage, where string) (string, error) {
	if isJSONNull(raw) {
		return "", fmt.Errorf("%s must be a non-null string", where)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s must be a string: %v", where, err)
	}
	return value, nil
}

func requiredInt(object rawObject, key, where string) (int, error) {
	raw, ok := object[key]
	if !ok || isJSONNull(raw) {
		return 0, fmt.Errorf("%s.%s must be a non-null integer", where, key)
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("%s.%s must be an integer: %v", where, key, err)
	}
	return value, nil
}

func requiredDurationString(object rawObject, key, where string) (string, error) {
	raw, ok := object[key]
	if !ok {
		return "", fmt.Errorf("%s.%s is required", where, key)
	}
	return decodeDurationString(raw, where+"."+key)
}

func decodeDurationString(raw json.RawMessage, where string) (string, error) {
	value, err := decodeString(raw, where)
	if err != nil {
		return "", err
	}
	if _, err := time.ParseDuration(value); err != nil {
		return "", fmt.Errorf("%s is not a duration: %v", where, err)
	}
	return value, nil
}

func optionalArray(raw json.RawMessage, where string) ([]json.RawMessage, error) {
	if len(raw) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("%s must be an array or null: %v", where, err)
	}
	for i, item := range items {
		if isJSONNull(item) {
			return nil, fmt.Errorf("%s[%d] must not be null", where, i)
		}
	}
	return items, nil
}

func optionalStringArray(raw json.RawMessage, where string) ([]string, error) {
	items, err := optionalArray(raw, where)
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(items))
	for i, item := range items {
		value, valueErr := decodeString(item, fmt.Sprintf("%s[%d]", where, i))
		if valueErr != nil {
			return nil, valueErr
		}
		values = append(values, value)
	}
	return values, nil
}

func isJSONNull(raw []byte) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func fixtureGraph(t *testing.T, fixture dispatched.ScheduleFixture) dispatched.Graph {
	t.Helper()
	graph := dispatched.Graph{Nodes: make([]dispatched.Node, len(fixture.Nodes))}
	for i, node := range fixture.Nodes {
		duration, err := time.ParseDuration(node.Duration)
		if err != nil {
			t.Fatalf("%s node %q duration: %v", fixture.Name, node.Key, err)
		}
		graph.Nodes[i] = dispatched.Node{Key: node.Key, Duration: duration, BlockedBy: cloneStrings(node.BlockedBy)}
	}
	return graph
}

func fixtureExpectedSchedule(t *testing.T, fixture dispatched.ScheduleFixture) dispatched.Schedule {
	t.Helper()
	makespan, err := time.ParseDuration(fixture.Expect.Makespan)
	if err != nil {
		t.Fatalf("%s expected makespan: %v", fixture.Name, err)
	}
	want := dispatched.Schedule{
		Makespan:       makespan,
		Trace:          make([]dispatched.NodeTrace, len(fixture.Expect.Trace)),
		DependencyPath: cloneStrings(fixture.Expect.DependencyPath),
		ExecutionChain: make([]dispatched.ChainStep, len(fixture.Expect.ExecutionChain)),
	}
	for i, trace := range fixture.Expect.Trace {
		start, startErr := time.ParseDuration(trace.Start)
		finish, finishErr := time.ParseDuration(trace.Finish)
		if startErr != nil || finishErr != nil {
			t.Fatalf("%s trace %q duration: start=%v finish=%v", fixture.Name, trace.Key, startErr, finishErr)
		}
		want.Trace[i] = dispatched.NodeTrace{Key: trace.Key, Start: start, Finish: finish, BlockedBy: cloneStrings(trace.BlockedBy)}
	}
	for i, step := range fixture.Expect.ExecutionChain {
		want.ExecutionChain[i] = dispatched.ChainStep{Key: step.Key, Edge: step.Edge}
	}
	return want
}

func fixtureResultFailure(t *testing.T, fixture dispatched.ScheduleFixture, got dispatched.Schedule, err error) string {
	t.Helper()
	if fixture.Expect.Error == "" {
		if err != nil {
			return fmt.Sprintf("error = %v, want success", err)
		}
		want := fixtureExpectedSchedule(t, fixture)
		if !schedulesEqual(got, want) {
			return fmt.Sprintf("complete schedule mismatch\n got: %#v\nwant: %#v", got, want)
		}
		return ""
	}
	wantErr, ok := dispatched.LookupSchedulerSentinel(fixture.Expect.Error)
	if !ok {
		t.Fatalf("fixture loader admitted unknown sentinel %q", fixture.Expect.Error)
	}
	if err == nil || !errors.Is(err, wantErr) {
		return fmt.Sprintf("error = %v, want errors.Is(%s)", err, fixture.Expect.Error)
	}
	matched := 0
	for _, sentinel := range dispatched.SchedulerSentinels() {
		if errors.Is(err, sentinel.Err) {
			matched++
			if sentinel.Name != fixture.Expect.Error {
				return fmt.Sprintf("error %v also matches unexpected %s", err, sentinel.Name)
			}
		}
	}
	if matched != 1 {
		return fmt.Sprintf("error %v matches %d scheduler sentinels, want exactly one", err, matched)
	}
	if errors.Is(err, dispatched.ErrNotImplemented) {
		return fmt.Sprintf("error %v also matches ErrNotImplemented", err)
	}
	if !reflect.DeepEqual(got, dispatched.Schedule{}) {
		return fmt.Sprintf("error schedule = %#v, want exact zero Schedule", got)
	}
	return ""
}

func assertPureSchedule(t *testing.T, arm scheduleArm, graph dispatched.Graph, maxParallel int, want dispatched.Schedule) {
	t.Helper()
	before := cloneGraph(graph)
	type result struct {
		schedule dispatched.Schedule
		err      error
	}
	results := make([]result, 2)
	var wait sync.WaitGroup
	for i := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index].schedule, results[index].err = arm(graph, maxParallel)
		}(i)
	}
	wait.Wait()
	if !reflect.DeepEqual(graph, before) {
		t.Errorf("parallel calls mutated graph\n got: %#v\nwant: %#v", graph, before)
	}
	for i, result := range results {
		if result.err != nil {
			t.Errorf("pure call %d error = %v, want success", i, result.err)
			return
		}
		if !schedulesEqual(result.schedule, want) {
			t.Errorf("pure call %d differs from fixture\n got: %#v\nwant: %#v", i, result.schedule, want)
		}
	}
	if !schedulesEqual(results[0].schedule, results[1].schedule) {
		t.Errorf("parallel calls differ\nfirst: %#v\nsecond: %#v", results[0].schedule, results[1].schedule)
	}
}

func schedulesEqual(left, right dispatched.Schedule) bool {
	return reflect.DeepEqual(canonicalSchedule(left), canonicalSchedule(right))
}

func canonicalSchedule(schedule dispatched.Schedule) dispatched.Schedule {
	canonical := dispatched.Schedule{
		Makespan:       schedule.Makespan,
		Trace:          make([]dispatched.NodeTrace, len(schedule.Trace)),
		DependencyPath: append([]string{}, schedule.DependencyPath...),
		ExecutionChain: append([]dispatched.ChainStep{}, schedule.ExecutionChain...),
	}
	for i, trace := range schedule.Trace {
		canonical.Trace[i] = trace
		canonical.Trace[i].BlockedBy = append([]string{}, trace.BlockedBy...)
	}
	return canonical
}

func cloneGraph(graph dispatched.Graph) dispatched.Graph {
	clone := dispatched.Graph{Nodes: make([]dispatched.Node, len(graph.Nodes))}
	for i, node := range graph.Nodes {
		clone.Nodes[i] = dispatched.Node{Key: node.Key, Duration: node.Duration, BlockedBy: cloneStrings(node.BlockedBy)}
	}
	return clone
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func testScheduleComparisonMutations(t *testing.T) {
	fixtures := loadHandFixtures(t)
	byName := make(map[string]dispatched.ScheduleFixture, len(fixtures))
	for _, fixture := range fixtures {
		byName[fixture.Name] = fixture
	}
	base := fixtureExpectedSchedule(t, byName["F5-CAP-BINDS"])
	mutations := map[string]func(dispatched.Schedule) dispatched.Schedule{
		"makespan": func(schedule dispatched.Schedule) dispatched.Schedule { schedule.Makespan++; return schedule },
		"trace-position": func(schedule dispatched.Schedule) dispatched.Schedule {
			schedule.Trace = append([]dispatched.NodeTrace(nil), schedule.Trace...)
			schedule.Trace[0], schedule.Trace[1] = schedule.Trace[1], schedule.Trace[0]
			return schedule
		},
		"trace-time": func(schedule dispatched.Schedule) dispatched.Schedule {
			schedule.Trace = append([]dispatched.NodeTrace(nil), schedule.Trace...)
			schedule.Trace[0].Finish++
			return schedule
		},
		"dependency-path": func(schedule dispatched.Schedule) dispatched.Schedule {
			schedule.DependencyPath = []string{"A"}
			return schedule
		},
		"execution-edge": func(schedule dispatched.Schedule) dispatched.Schedule {
			schedule.ExecutionChain = append([]dispatched.ChainStep(nil), schedule.ExecutionChain...)
			schedule.ExecutionChain[1].Edge = dispatched.EdgeDependency
			return schedule
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			if schedulesEqual(base, mutate(base)) {
				t.Fatal("complete-schedule comparison admitted mutation")
			}
		})
	}
	blocked := fixtureExpectedSchedule(t, byName["F5-BLOCKED-BY-KEY-ORDER"])
	blockedMutation := canonicalSchedule(blocked)
	blockedMutation.Trace = append([]dispatched.NodeTrace(nil), blockedMutation.Trace...)
	blockedMutation.Trace[2].BlockedBy = []string{"A", "B"}
	if schedulesEqual(blocked, blockedMutation) {
		t.Fatal("comparison admitted key-sorted BlockedBy mutation")
	}
	errorFixture := byName["F5-CYCLE"]
	if failure := fixtureResultFailure(t, errorFixture, dispatched.Schedule{}, dispatched.ErrUnknownDependency); failure == "" {
		t.Fatal("error comparison admitted wrong sentinel mutation")
	}
}

func testFixtureExpectationMutations(t *testing.T) {
	fixtures := loadHandFixtures(t)
	byName := make(map[string]dispatched.ScheduleFixture, len(fixtures))
	for _, fixture := range fixtures {
		byName[fixture.Name] = fixture
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			if fixture.Expect.Error == "" {
				want := fixtureExpectedSchedule(t, fixture)
				mutant := want
				mutant.Makespan++
				if schedulesEqual(want, mutant) {
					t.Fatal("comparison admitted a one-nanosecond expected-makespan mutation")
				}
				return
			}
			var wrong error
			for _, sentinel := range dispatched.SchedulerSentinels() {
				if sentinel.Name != fixture.Expect.Error {
					wrong = sentinel.Err
					break
				}
			}
			if failure := fixtureResultFailure(t, fixture, dispatched.Schedule{}, wrong); failure == "" {
				t.Fatal("comparison admitted a wrong-sentinel mutation")
			}
		})
	}

	mutations := []struct {
		name    string
		fixture string
		mutate  func(*dispatched.ScheduleFixture)
	}{
		{
			name:    "last-node-dependency-key-order",
			fixture: "F5-LAST-NODE-KEY-ORDER",
			mutate: func(fixture *dispatched.ScheduleFixture) {
				fixture.Expect.DependencyPath = []string{"A"}
			},
		},
		{
			name:    "last-node-execution-key-order",
			fixture: "F5-LAST-NODE-KEY-ORDER",
			mutate: func(fixture *dispatched.ScheduleFixture) {
				fixture.Expect.ExecutionChain = []dispatched.FixtureChainStep{{Key: "A", Edge: dispatched.EdgeStart}}
			},
		},
		{
			name:    "resource-predecessor-key-order",
			fixture: "F5-RESOURCE-TIE-KEY-ORDER",
			mutate: func(fixture *dispatched.ScheduleFixture) {
				fixture.Expect.ExecutionChain = append([]dispatched.FixtureChainStep(nil), fixture.Expect.ExecutionChain...)
				fixture.Expect.ExecutionChain[0].Key = "A"
			},
		},
		{
			name:    "blocked-by-key-order",
			fixture: "F5-BLOCKED-BY-KEY-ORDER",
			mutate: func(fixture *dispatched.ScheduleFixture) {
				fixture.Expect.Trace = append([]dispatched.FixtureTrace(nil), fixture.Expect.Trace...)
				fixture.Expect.Trace[2].BlockedBy = []string{"A", "B"}
			},
		},
		{
			name:    "dependency-predecessor-key-order",
			fixture: "F5-BLOCKED-BY-KEY-ORDER",
			mutate: func(fixture *dispatched.ScheduleFixture) {
				fixture.Expect.DependencyPath = []string{"A", "C"}
			},
		},
		{
			name:    "cap-overlap",
			fixture: "F5-CAP-FREE",
			mutate: func(fixture *dispatched.ScheduleFixture) {
				fixture.Concurrency = 1
			},
		},
		{
			name:    "empty-success-bad-cap",
			fixture: "F5-EMPTY",
			mutate: func(fixture *dispatched.ScheduleFixture) {
				fixture.Concurrency = 0
			},
		},
		{
			name:    "zero-bypasses-occupied-slot",
			fixture: "F5-ZERO-WAITS-FOR-SLOT",
			mutate: func(fixture *dispatched.ScheduleFixture) {
				fixture.Expect.Trace = append([]dispatched.FixtureTrace(nil), fixture.Expect.Trace...)
				fixture.Expect.Trace[1].Start = "0s"
				fixture.Expect.Trace[1].Finish = "0s"
			},
		},
		{
			name:    "shorter-dependency-predecessor",
			fixture: "F5-FORK-JOIN-FREE",
			mutate: func(fixture *dispatched.ScheduleFixture) {
				fixture.Expect.DependencyPath = []string{"A", "B", "D"}
			},
		},
		{
			name:    "execution-predecessor-tie",
			fixture: "F5-RESOURCE-TIE",
			mutate: func(fixture *dispatched.ScheduleFixture) {
				fixture.Expect.ExecutionChain = []dispatched.FixtureChainStep{
					{Key: "B", Edge: dispatched.EdgeStart},
					{Key: "C", Edge: dispatched.EdgeResource},
				}
			},
		},
	}
	for _, mutation := range mutations {
		mutation := mutation
		t.Run(mutation.name, func(t *testing.T) {
			fixture := byName[mutation.fixture]
			mutation.mutate(&fixture)
			if failure := fixtureExpectationIntegrityFailure(t, fixture); failure == "" {
				t.Fatal("fixture integrity admitted a semantic expectation mutation")
			}
		})
	}
}

func testPanelDiscriminatorMutations(t *testing.T) {
	fixtures := loadHandFixtures(t)
	byName := make(map[string]dispatched.ScheduleFixture, len(fixtures))
	for _, fixture := range fixtures {
		byName[fixture.Name] = fixture
	}

	t.Run("float-seconds-finish", func(t *testing.T) {
		fixture := byName["F5-FLOAT-SECONDS-PRECISION"]
		graph := fixtureGraph(t, fixture)
		mutant := canonicalSchedule(fixtureExpectedSchedule(t, fixture))
		mutant.Trace[0].Finish = floatSecondsFinish(0, graph.Nodes[0].Duration)
		mutant.Trace[1].Start = mutant.Trace[0].Finish
		mutant.Trace[1].Finish = floatSecondsFinish(mutant.Trace[1].Start, graph.Nodes[1].Duration)
		mutant.Trace[2].Start = mutant.Trace[1].Finish
		mutant.Trace[2].Finish = floatSecondsFinish(mutant.Trace[2].Start, graph.Nodes[2].Duration)
		mutant.Makespan = mutant.Trace[2].Finish
		if schedulesEqual(mutant, fixtureExpectedSchedule(t, fixture)) {
			t.Fatal("precision fixture does not distinguish the float-seconds finish mutant")
		}
		if failure := fixtureResultFailure(t, fixture, mutant, nil); failure == "" {
			t.Fatal("float-seconds finish mutant passed the precision fixture")
		}
	})

	t.Run("negative-cap-equality-check", func(t *testing.T) {
		fixture := byName["F5-NEGATIVE-CAP"]
		if failure := fixtureResultFailure(t, fixture, dispatched.Schedule{}, nil); failure == "" {
			t.Fatal("maxParallel == 0 mutant passed the negative-cap fixture")
		}
	})

	t.Run("overflow-strict-boundary", func(t *testing.T) {
		fixture := byName["F5-OVERFLOW-BOUNDARY"]
		if failure := fixtureResultFailure(t, fixture, dispatched.Schedule{}, dispatched.ErrScheduleOverflow); failure == "" {
			t.Fatal("duration >= MaxInt64-sum mutant passed the exact-boundary fixture")
		}
	})
}

func floatSecondsFinish(now, duration time.Duration) time.Duration {
	return time.Duration((now.Seconds() + duration.Seconds()) * float64(time.Second))
}

const generatedCorpusTotal = 84073

var generatedCounts = map[int]int{0: 1, 1: 8, 2: 96, 3: 2048, 4: 81920}

func testGeneratedCorpusShape(t *testing.T) {
	counts := map[int]int{}
	hash := sha256.New()
	err := walkGeneratedCorpus(func(id string, graph dispatched.Graph, maxParallel int) error {
		counts[len(graph.Nodes)]++
		fmt.Fprintf(hash, "%s|cap=%d", id, maxParallel)
		for _, node := range graph.Nodes {
			fmt.Fprintf(hash, "|%q:%d:%q", node.Key, node.Duration/time.Second, node.BlockedBy)
		}
		io.WriteString(hash, "\n")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(counts, generatedCounts) {
		t.Fatalf("generated counts = %#v, want %#v", counts, generatedCounts)
	}
	const wantFingerprint = "9f0df57ac996a7326a3dffb7a58dd63394eeddb2e32517c106294e637b5125bf"
	if got := hex.EncodeToString(hash.Sum(nil)); got != wantFingerprint {
		t.Fatalf("generated corpus fingerprint = %s, want %s", got, wantFingerprint)
	}
}

func testGeneratedOracleDomain(t *testing.T) {
	checked := 0
	err := walkGeneratedCorpus(func(id string, graph dispatched.Graph, maxParallel int) error {
		schedule := tickOracle(t, graph, maxParallel)
		if failure := oracleInvariantFailure(graph, maxParallel, schedule); failure != "" {
			return fmt.Errorf("%s: %s", id, failure)
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked != generatedCorpusTotal {
		t.Fatalf("oracle-checked cases = %d, want %d", checked, generatedCorpusTotal)
	}
}

func oracleInvariantFailure(graph dispatched.Graph, maxParallel int, schedule dispatched.Schedule) string {
	if len(schedule.Trace) != len(graph.Nodes) {
		return fmt.Sprintf("trace length = %d, want %d", len(schedule.Trace), len(graph.Nodes))
	}
	index := make(map[string]int, len(graph.Nodes))
	for i, node := range graph.Nodes {
		index[node.Key] = i
		trace := schedule.Trace[i]
		if trace.Key != node.Key {
			return fmt.Sprintf("trace[%d] key = %q, want %q", i, trace.Key, node.Key)
		}
		if trace.Finish-trace.Start != node.Duration {
			return fmt.Sprintf("trace[%d] duration = %s, want %s", i, trace.Finish-trace.Start, node.Duration)
		}
		for _, dependency := range trace.BlockedBy {
			dependencyIndex, ok := index[dependency]
			if !ok || schedule.Trace[dependencyIndex].Finish > trace.Start {
				return fmt.Sprintf("trace[%d] starts before dependency %q finishes", i, dependency)
			}
		}
	}
	for tick := time.Duration(0); tick < schedule.Makespan; tick += time.Second {
		active := 0
		for _, trace := range schedule.Trace {
			if trace.Start <= tick && tick < trace.Finish {
				active++
			}
		}
		if active > maxParallel {
			return fmt.Sprintf("active nodes at %s = %d, cap %d", tick, active, maxParallel)
		}
	}
	dependencyTotal := time.Duration(0)
	for _, key := range schedule.DependencyPath {
		nodeIndex, ok := index[key]
		if !ok {
			return fmt.Sprintf("dependency path contains unknown key %q", key)
		}
		dependencyTotal += graph.Nodes[nodeIndex].Duration
	}
	if dependencyTotal > schedule.Makespan {
		return fmt.Sprintf("dependency path duration %s exceeds makespan %s", dependencyTotal, schedule.Makespan)
	}
	executionTotal := time.Duration(0)
	for i, step := range schedule.ExecutionChain {
		nodeIndex, ok := index[step.Key]
		if !ok {
			return fmt.Sprintf("execution chain contains unknown key %q", step.Key)
		}
		if i == 0 && step.Edge != dispatched.EdgeStart {
			return fmt.Sprintf("execution head edge = %q, want start", step.Edge)
		}
		if i > 0 && step.Edge != dispatched.EdgeDependency && step.Edge != dispatched.EdgeResource {
			return fmt.Sprintf("execution edge[%d] = %q", i, step.Edge)
		}
		executionTotal += graph.Nodes[nodeIndex].Duration
	}
	if executionTotal != schedule.Makespan {
		return fmt.Sprintf("execution chain duration = %s, want makespan %s", executionTotal, schedule.Makespan)
	}
	return ""
}

func walkGeneratedCorpus(yield func(string, dispatched.Graph, int) error) error {
	if err := yield("N0-D0-E0-C1", dispatched.Graph{}, 1); err != nil {
		return err
	}
	for nodeCount := 1; nodeCount <= 4; nodeCount++ {
		edges := make([][2]int, 0, nodeCount*(nodeCount-1)/2)
		for from := 0; from < nodeCount; from++ {
			for dependent := from + 1; dependent < nodeCount; dependent++ {
				edges = append(edges, [2]int{from, dependent})
			}
		}
		durationTuples := intPow(4, nodeCount)
		for durationCode := 0; durationCode < durationTuples; durationCode++ {
			durations := make([]int, nodeCount)
			remaining := durationCode
			for i := nodeCount - 1; i >= 0; i-- {
				durations[i] = remaining % 4
				remaining /= 4
			}
			for edgeMask := 0; edgeMask < 1<<len(edges); edgeMask++ {
				graph := dispatched.Graph{Nodes: make([]dispatched.Node, nodeCount)}
				for i := range graph.Nodes {
					graph.Nodes[i] = dispatched.Node{Key: strconv.Itoa(nodeCount - i), Duration: time.Duration(durations[i]) * time.Second}
				}
				for bit, edge := range edges {
					if edgeMask&(1<<bit) != 0 {
						from, dependent := edge[0], edge[1]
						graph.Nodes[dependent].BlockedBy = append(graph.Nodes[dependent].BlockedBy, graph.Nodes[from].Key)
					}
				}
				for maxParallel := 1; maxParallel <= nodeCount+1; maxParallel++ {
					id := fmt.Sprintf("N%d-D%d-E%d-C%d", nodeCount, durationCode, edgeMask, maxParallel)
					if err := yield(id, graph, maxParallel); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func intPow(base, exponent int) int {
	result := 1
	for range exponent {
		result *= base
	}
	return result
}

func testTickOracleZeroDrain(t *testing.T) {
	graph := dispatched.Graph{Nodes: []dispatched.Node{
		{Key: "A", Duration: 0},
		{Key: "B", Duration: 0, BlockedBy: []string{"A"}},
		{Key: "C", Duration: time.Second, BlockedBy: []string{"B"}},
	}}
	want := dispatched.Schedule{
		Makespan: time.Second,
		Trace: []dispatched.NodeTrace{
			{Key: "A", Start: 0, Finish: 0},
			{Key: "B", Start: 0, Finish: 0, BlockedBy: []string{"A"}},
			{Key: "C", Start: 0, Finish: time.Second, BlockedBy: []string{"B"}},
		},
		DependencyPath: []string{"A", "B", "C"},
		ExecutionChain: []dispatched.ChainStep{
			{Key: "A", Edge: dispatched.EdgeStart},
			{Key: "B", Edge: dispatched.EdgeDependency},
			{Key: "C", Edge: dispatched.EdgeDependency},
		},
	}
	if got := tickOracle(t, graph, 1); !schedulesEqual(got, want) {
		t.Fatalf("zero-drain oracle self-check\n got: %#v\nwant: %#v", got, want)
	}
}

func tickOracle(t *testing.T, graph dispatched.Graph, maxParallel int) dispatched.Schedule {
	t.Helper()
	if len(graph.Nodes) == 0 {
		return dispatched.Schedule{}
	}
	index := make(map[string]int, len(graph.Nodes))
	for i, node := range graph.Nodes {
		if node.Duration < 0 || node.Duration%time.Second != 0 {
			t.Fatalf("tickOracle received out-of-domain duration %s", node.Duration)
		}
		index[node.Key] = i
	}
	dependencies := make([][]int, len(graph.Nodes))
	for i, node := range graph.Nodes {
		seen := make(map[int]bool)
		for _, key := range node.BlockedBy {
			dependency, ok := index[key]
			if !ok {
				t.Fatalf("tickOracle received unknown dependency %q", key)
			}
			seen[dependency] = true
		}
		for dependency := range graph.Nodes {
			if seen[dependency] {
				dependencies[i] = append(dependencies[i], dependency)
			}
		}
	}
	pending := make([]bool, len(graph.Nodes))
	running := make([]bool, len(graph.Nodes))
	completed := make([]bool, len(graph.Nodes))
	remaining := make([]int, len(graph.Nodes))
	start := make([]int, len(graph.Nodes))
	finish := make([]int, len(graph.Nodes))
	for i := range pending {
		pending[i] = true
	}
	completedCount, runningCount, tick := 0, 0, 0
	for completedCount < len(graph.Nodes) {
		for i := range running {
			if running[i] && remaining[i] == 0 {
				running[i] = false
				completed[i] = true
				runningCount--
				completedCount++
			}
		}
		for i := range pending {
			if !pending[i] || runningCount >= maxParallel || !oracleReady(dependencies[i], completed) {
				continue
			}
			pending[i] = false
			running[i] = true
			runningCount++
			start[i] = tick
			remaining[i] = int(graph.Nodes[i].Duration / time.Second)
			finish[i] = tick + remaining[i]
		}
		zeroRunning := false
		for i := range running {
			zeroRunning = zeroRunning || running[i] && remaining[i] == 0
		}
		if zeroRunning {
			continue
		}
		if completedCount == len(graph.Nodes) {
			break
		}
		if runningCount == 0 {
			t.Fatal("tickOracle made no progress on its valid DAG domain")
		}
		for i := range running {
			if running[i] {
				remaining[i]--
			}
		}
		tick++
	}

	schedule := dispatched.Schedule{Trace: make([]dispatched.NodeTrace, len(graph.Nodes))}
	for i, node := range graph.Nodes {
		schedule.Trace[i] = dispatched.NodeTrace{
			Key:       node.Key,
			Start:     time.Duration(start[i]) * time.Second,
			Finish:    time.Duration(finish[i]) * time.Second,
			BlockedBy: oracleDependencyKeys(graph, dependencies[i]),
		}
		if schedule.Trace[i].Finish > schedule.Makespan {
			schedule.Makespan = schedule.Trace[i].Finish
		}
	}
	last := 0
	for i := 1; i < len(schedule.Trace); i++ {
		if schedule.Trace[i].Finish > schedule.Trace[last].Finish {
			last = i
		}
	}
	schedule.DependencyPath = oracleDependencyPath(schedule.Trace, dependencies, last)
	schedule.ExecutionChain = oracleExecutionChain(t, graph, schedule.Trace, dependencies, last)
	return schedule
}

func oracleReady(dependencies []int, completed []bool) bool {
	for _, dependency := range dependencies {
		if !completed[dependency] {
			return false
		}
	}
	return true
}

func oracleDependencyKeys(graph dispatched.Graph, dependencies []int) []string {
	if len(dependencies) == 0 {
		return nil
	}
	keys := make([]string, len(dependencies))
	for i, dependency := range dependencies {
		keys[i] = graph.Nodes[dependency].Key
	}
	return keys
}

func oracleDependencyPath(trace []dispatched.NodeTrace, dependencies [][]int, last int) []string {
	reverse := make([]string, 0, len(trace))
	current := last
	for {
		reverse = append(reverse, trace[current].Key)
		if len(dependencies[current]) == 0 {
			break
		}
		predecessor := dependencies[current][0]
		for _, dependency := range dependencies[current][1:] {
			if trace[dependency].Finish > trace[predecessor].Finish {
				predecessor = dependency
			}
		}
		current = predecessor
	}
	reverseStrings(reverse)
	return reverse
}

func oracleExecutionChain(t *testing.T, graph dispatched.Graph, trace []dispatched.NodeTrace, dependencies [][]int, last int) []dispatched.ChainStep {
	t.Helper()
	reverse := make([]dispatched.ChainStep, 0, len(trace))
	current := last
	for {
		deps := dependencies[current]
		depReady := time.Duration(0)
		for _, dependency := range deps {
			if trace[dependency].Finish > depReady {
				depReady = trace[dependency].Finish
			}
		}
		if len(deps) == 0 && trace[current].Start == 0 {
			reverse = append(reverse, dispatched.ChainStep{Key: trace[current].Key, Edge: dispatched.EdgeStart})
			break
		}
		if len(deps) > 0 && trace[current].Start == depReady {
			predecessor := -1
			for _, dependency := range deps {
				if trace[dependency].Finish == depReady {
					predecessor = dependency
					break
				}
			}
			if predecessor < 0 {
				t.Fatal("tickOracle could not find dependency predecessor")
			}
			reverse = append(reverse, dispatched.ChainStep{Key: trace[current].Key, Edge: dispatched.EdgeDependency})
			current = predecessor
			continue
		}
		if trace[current].Start > depReady {
			predecessor := -1
			for i, candidate := range trace {
				if candidate.Finish == trace[current].Start && graph.Nodes[i].Duration > 0 {
					predecessor = i
					break
				}
			}
			if predecessor < 0 {
				t.Fatal("tickOracle could not find resource predecessor")
			}
			reverse = append(reverse, dispatched.ChainStep{Key: trace[current].Key, Edge: dispatched.EdgeResource})
			current = predecessor
			continue
		}
		t.Fatalf("tickOracle execution predecessor cases are not exhaustive for %q", trace[current].Key)
	}
	for left, right := 0, len(reverse)-1; left < right; left, right = left+1, right-1 {
		reverse[left], reverse[right] = reverse[right], reverse[left]
	}
	return reverse
}

func reverseStrings(values []string) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func runLargeCapProbe(t *testing.T, armName string) {
	t.Helper()
	runFilter := largeCapChildRunFilter(armName)
	ctx, cancel := context.WithTimeout(context.Background(), largeCapParentTimeout)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		os.Args[0],
		"-test.run="+runFilter,
		"-test.count=1",
		"-test.timeout="+largeCapChildTimeout.String(),
		"-fc-schedule-large-cap-child="+armName,
	)
	configureLargeCapCancellation(cmd)
	cmd.WaitDelay = time.Second
	cmd.Env = childProcessEnv(armName)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("large-cap child timed out and was killed: %s", output)
	}
	if err != nil {
		t.Fatalf("large-cap child reported refusal, panic, allocation excess, or crash: %v\n%s", err, output)
	}
	if !largeCapHandshakePresent(output, armName) {
		t.Fatalf("large-cap child exited without the authenticated success handshake; skip, empty selection, and unarmed child output are failures:\n%s", output)
	}
}

func largeCapChildRunFilter(armName string) string {
	return "^TestFCSchedule" + armName + "Contract$/^large-cap-child-only$"
}

func largeCapSuccessToken(armName string) string {
	return "FC_SCHEDULE_LARGE_CAP_OK:" + armName
}

func largeCapHandshakePresent(output []byte, armName string) bool {
	want := largeCapSuccessToken(armName)
	for _, line := range bytes.Split(output, []byte{'\n'}) {
		if string(bytes.TrimSpace(line)) == want {
			return true
		}
	}
	return false
}

func isLargeCapChildProcess(armName string) bool {
	return isLargeCapChildInvocation(armName, os.Getenv(largeCapChildEnv), *largeCapChildArm, os.Args[1:])
}

func largeCapStorageDecision(armName, environmentArm, markerArm string, args []string) (runParent bool, failure string) {
	if isLargeCapChildInvocation(armName, environmentArm, markerArm, args) {
		return false, ""
	}
	if isLargeCapMarkedInvocation(armName, environmentArm, markerArm) {
		return false, fmt.Sprintf("large-cap child markers for arm %s require exact selector %q", armName, largeCapChildRunFilter(armName))
	}
	return true, ""
}

func isLargeCapChildInvocation(armName, environmentArm, markerArm string, args []string) bool {
	if !isLargeCapMarkedInvocation(armName, environmentArm, markerArm) {
		return false
	}
	wantRunArg := "-test.run=" + largeCapChildRunFilter(armName)
	for _, arg := range args {
		if arg == wantRunArg {
			return true
		}
	}
	return false
}

func isLargeCapMarkedInvocation(armName, environmentArm, markerArm string) bool {
	return environmentArm == armName && markerArm == armName
}

func childProcessEnv(armName string) []string {
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, largeCapChildEnv+"=") {
			environment = append(environment, entry)
		}
	}
	return append(environment, largeCapChildEnv+"="+armName)
}

func runLargeCapChild(t *testing.T, arm scheduleArm) {
	t.Helper()
	debug.SetMemoryLimit(largeCapMemoryLimit)
	graph := dispatched.Graph{Nodes: []dispatched.Node{{Key: "only", Duration: time.Nanosecond}}}
	input := cloneGraph(graph)
	want := dispatched.Schedule{
		Makespan:       time.Nanosecond,
		Trace:          []dispatched.NodeTrace{{Key: "only", Start: 0, Finish: time.Nanosecond}},
		DependencyPath: []string{"only"},
		ExecutionChain: []dispatched.ChainStep{{Key: "only", Edge: dispatched.EdgeStart}},
	}
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("large-cap scheduler panic: %v", recovered)
		}
	}()
	got, err := arm(input, math.MaxInt)
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("large-cap scheduler refusal: %v", err)
	}
	if !schedulesEqual(got, want) {
		t.Fatalf("large-cap schedule mismatch\n got: %#v\nwant: %#v", got, want)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > largeCapAllocationLimit {
		t.Fatalf("large-cap scheduler allocated %d bytes, want at most %d", allocated, largeCapAllocationLimit)
	}
	if heapGrowth := int64(after.HeapAlloc) - int64(before.HeapAlloc); heapGrowth > largeCapAllocationLimit {
		t.Fatalf("large-cap scheduler retained %d heap bytes, want at most %d", heapGrowth, largeCapAllocationLimit)
	}
}
