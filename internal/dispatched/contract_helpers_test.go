package dispatched

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// contractCutoff is after every recorded/synthetic fixture event except
// synthetic-later-start.jsonl, whose task_started is 2026-09-06.
func contractCutoff() time.Time {
	return time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
}

// fixtureRecordedAt predates every selection cutoff used by these seals.
// File mtimes and fixture Git commits are frozen here so a checkout made after
// contractCutoff cannot silently turn otherwise in-sample evidence into an
// after-cutoff envelope.
func fixtureRecordedAt() time.Time {
	return time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
}

func setFixtureMTime(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture for mtime %s: %v", path, err)
	}
	hash := fnv.New32a()
	_, _ = hash.Write(data)
	when := fixtureRecordedAt().Add(time.Duration(hash.Sum32()%2_000_000) * time.Second)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("freeze fixture mtime %s: %v", path, err)
	}
}

func freezeFixtureTree(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(root); err != nil {
		return
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.Type().IsRegular() {
			setFixtureMTime(t, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("freeze fixture tree %s: %v", root, err)
	}
}

type fixtureGitClock struct {
	path     string
	identity os.FileInfo
	sequence uint64
}

var fixtureGitClocks = struct {
	sync.Mutex
	clocks []fixtureGitClock
}{}

// fixtureGitEnv supplies runGit with an explicit clock for each fixture
// repository, independent of reachable refs, deleted branches, grafts, and
// replacements. Commit-producing runGit calls advance that clock, so each
// commit is strictly later than its parents. Read-only helpers using
// fixtureGitIsolatedEnv do not advance it.
func fixtureGitEnv(repo string) []string {
	key := filepath.Clean(repo)
	identity, _ := os.Stat(filepath.Join(key, ".git"))
	fixtureGitClocks.Lock()
	clockIndex := -1
	for i := range fixtureGitClocks.clocks {
		clock := &fixtureGitClocks.clocks[i]
		if identity != nil && clock.identity != nil && os.SameFile(identity, clock.identity) {
			clockIndex = i
			break
		}
		if clock.path == key {
			clockIndex = i
		}
	}
	if clockIndex < 0 {
		fixtureGitClocks.clocks = append(fixtureGitClocks.clocks, fixtureGitClock{path: key, identity: identity})
		clockIndex = len(fixtureGitClocks.clocks) - 1
	}
	clock := &fixtureGitClocks.clocks[clockIndex]
	clock.path = key
	if identity != nil {
		clock.identity = identity
	}
	sequence := clock.sequence
	clock.sequence++
	fixtureGitClocks.Unlock()
	when := fixtureRecordedAt().Add(time.Duration(sequence) * time.Minute).Format(time.RFC3339)
	return fixtureGitIsolatedEnv("GIT_AUTHOR_DATE="+when, "GIT_COMMITTER_DATE="+when)
}

func fixtureGitIsolatedEnv(extra ...string) []string {
	env := make([]string, 0, len(os.Environ())+len(extra)+4)
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if ok && strings.HasPrefix(key, "GIT_") {
			continue
		}
		env = append(env, item)
	}
	env = append(env,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
	)
	return append(env, extra...)
}

func requireFixtureCommitBeforeCutoff(t *testing.T, repo string) {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "rev-list", "--parents", "-n", "1", "HEAD")
	cmd.Env = fixtureGitIsolatedEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("read fixture commit and parents: %v", err)
	}
	revisions := strings.Fields(string(out))
	if len(revisions) == 0 {
		t.Fatal("fixture HEAD did not resolve")
	}
	headTimes := fixtureCommitTimes(t, repo, revisions[0])
	for _, when := range headTimes {
		if !when.Before(contractCutoff()) {
			t.Fatalf("fixture commit date %s is not before cutoff %s", when, contractCutoff())
		}
	}
	for _, parent := range revisions[1:] {
		for _, parentWhen := range fixtureCommitTimes(t, repo, parent) {
			for _, headWhen := range headTimes {
				if !headWhen.After(parentWhen) {
					t.Fatalf("fixture commit time %s is not strictly later than parent %s time %s", headWhen, parent, parentWhen)
				}
			}
		}
	}
}

func fixtureCommitTimes(t *testing.T, repo, revision string) []time.Time {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "show", "-s", "--format=%aI%n%cI", revision)
	cmd.Env = fixtureGitIsolatedEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("read fixture commit dates for %s: %v", revision, err)
	}
	rawTimes := strings.Fields(string(out))
	if len(rawTimes) != 2 {
		t.Fatalf("fixture commit %s dates = %q, want author and committer", revision, out)
	}
	times := make([]time.Time, 0, len(rawTimes))
	for _, raw := range rawTimes {
		when, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			t.Fatalf("parse fixture commit date %q: %v", raw, parseErr)
		}
		times = append(times, when)
	}
	return times
}

func testdataFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"testdata"}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func journalID(run, path string) JournalIdentity {
	return JournalIdentity{RunID: run, SourceID: "journals", Path: path}
}

func parseFixture(t *testing.T, run, name string) ParsedJournal {
	t.Helper()
	data := testdataFile(t, "journals", name)
	parsed, err := ParseEvents(context.Background(), journalID(run, name), strings.NewReader(data), JournalBounds{})
	if err != nil {
		t.Fatalf("ParseEvents(%s): %v", name, err)
	}
	return parsed
}

func reduceFixture(t *testing.T, run, name string) AttemptSet {
	t.Helper()
	set, err := ReduceAttempts(parseFixture(t, run, name), contractCutoff())
	if err != nil {
		t.Fatalf("ReduceAttempts(%s): %v", name, err)
	}
	return set
}

func requireSentinel(t *testing.T, err error, sentinel error) {
	t.Helper()
	if err == nil || !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want errors.Is %v", err, sentinel)
	}
}

func requireNoSentinel(t *testing.T, err error, sentinel error) {
	t.Helper()
	if errors.Is(err, sentinel) {
		t.Fatalf("error unexpectedly wraps %v: %v", sentinel, err)
	}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		at, err = time.Parse(time.RFC3339, value)
	}
	if err != nil {
		t.Fatal(err)
	}
	return at.UTC()
}

func requireInstant(t *testing.T, got time.Time, want time.Time, label string) {
	t.Helper()
	if !got.Equal(want.UTC()) || got.Location() != time.UTC {
		t.Fatalf("%s = %s loc=%s, want UTC instant %s", label, got.Format(time.RFC3339Nano), got.Location(), want.UTC().Format(time.RFC3339Nano))
	}
	if got.Round(0) != got {
		t.Fatalf("%s retained monotonic data: %v", label, got)
	}
}

func requireKnown[T comparable](t *testing.T, m Measured[T], want T, label string) {
	t.Helper()
	got, ok := m.Get()
	if !ok {
		t.Fatalf("%s unknown, want %v", label, want)
	}
	if got != want {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

func requireUnknown[T any](t *testing.T, m Measured[T], label string) {
	t.Helper()
	if m.Known {
		t.Fatalf("%s known (%v), want unknown", label, m.Value)
	}
}

func oneAttempt(t *testing.T, set AttemptSet) Attempt {
	t.Helper()
	if len(set.Attempts) != 1 {
		t.Fatalf("attempts = %d ambiguous=%d conflicts=%d, want 1 attempt", len(set.Attempts), len(set.Ambiguous), len(set.Conflicts))
	}
	return set.Attempts[0]
}

func defaultSelection() Selection {
	return Selection{Cutoff: contractCutoff()}
}

func journalSpec(id, runsDir string) SourceSpec {
	return SourceSpec{ID: id, Kind: SourceKindJournals, Repository: runsDir}
}

func liveSpec(id, repo string, roots ...string) SourceSpec {
	return SourceSpec{ID: id, Kind: SourceKindLiveYAML, Repository: repo, Roots: roots}
}

func historySpec(id, repo, ref string, roots ...string) SourceSpec {
	return SourceSpec{ID: id, Kind: SourceKindGitHistory, Repository: repo, Ref: ref, Roots: roots}
}

func readContractSources(t *testing.T, specs []SourceSpec, selection Selection, bounds ReadBounds) (*SourceManifest, *SourceReadings, error) {
	t.Helper()
	return ReadSources(context.Background(), specs, selection, bounds)
}

func mustReadSources(t *testing.T, specs []SourceSpec, selection Selection, bounds ReadBounds) (*SourceManifest, *SourceReadings) {
	t.Helper()
	manifest, readings, err := readContractSources(t, specs, selection, bounds)
	if err != nil {
		t.Fatalf("ReadSources: %v", err)
	}
	if manifest == nil || readings == nil {
		t.Fatal("ReadSources returned nil carriers")
	}
	return manifest, readings
}

func writeJournalTree(t *testing.T, runs, run, fixture string) {
	t.Helper()
	dir := filepath.Join(runs, run)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := testdataFile(t, "journals", fixture)
	path := filepath.Join(dir, "journal.jsonl")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	setFixtureMTime(t, path)
}

func writeJournalData(t *testing.T, runs, run, data string) {
	t.Helper()
	path := filepath.Join(runs, run, "journal.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	setFixtureMTime(t, path)
}

type gitRepo struct {
	t    *testing.T
	path string
}

func initGitRepo(t *testing.T) gitRepo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, path, "init", "-q", "--initial-branch=main")
	runGit(t, path, "config", "user.email", "seals@example.com")
	runGit(t, path, "config", "user.name", "FC-SEALS")
	runGit(t, path, "config", "commit.gpgsign", "false")
	return gitRepo{t: t, path: path}
}

func (g gitRepo) write(rel, data string) {
	g.t.Helper()
	path := filepath.Join(g.path, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		g.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		g.t.Fatal(err)
	}
	setFixtureMTime(g.t, path)
}

func (g gitRepo) writeTestdata(rel string, parts ...string) {
	g.write(rel, testdataFile(g.t, parts...))
}

func (g gitRepo) commit(message string, paths ...string) string {
	g.t.Helper()
	args := append([]string{"add"}, paths...)
	runGit(g.t, g.path, args...)
	runGit(g.t, g.path, "commit", "-q", "-m", message)
	cmd := exec.Command("git", "-C", g.path, "rev-parse", "HEAD")
	cmd.Env = fixtureGitIsolatedEnv()
	out, err := cmd.Output()
	if err != nil {
		g.t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func envMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			out[k] = v
		}
	}
	return out
}

func syntheticReading(run, key string, start time.Time, row int, path string) Reading {
	start = start.UTC()
	return Reading{
		Kind: DocumentTaskRow,
		Identity: ReadingIdentity{
			RunID:     Known(run),
			Key:       Known(key),
			StartedAt: Known(start),
		},
		Present: RowFields{Key: true, RunID: true, StartedAt: true},
		Ref: ReadingRef{
			SourceID:   "live",
			Repository: "repo",
			Path:       path,
			Revision:   "live",
			Row:        row,
			RecordedAt: start,
		},
		Snapshot:    ReadingSnapshot{Role: RoleBodies, AuthoredModel: "authored", Status: "Done"},
		CompletedAt: Known(start.Add(10 * time.Minute)),
	}
}

func journalAttempt(run, key string, start time.Time, elapsed time.Duration, model string, outcome Outcome) Attempt {
	id := NewAttemptID(run, key, start)
	end := id.StartedAt.Add(elapsed)
	terminalSrc := EvidenceNone
	if outcome == OutcomeDone || outcome == OutcomeBlocked {
		terminalSrc = EvidenceJournal
	}
	modelKnown := model != ""
	modelSrc := EvidenceNone
	var measuredModel Measured[string]
	if modelKnown {
		measuredModel = Known(model)
		modelSrc = EvidenceJournal
	}
	startRef := EventRef{
		Journal: JournalIdentity{RunID: run, SourceID: "journals", Path: "journal.jsonl", Producer: ProducerDispatcherV0_1_0},
		Type:    EventTaskStarted,
		At:      id.StartedAt,
		HasSeq:  true,
		Seq:     1,
		Line:    2,
	}
	termRef := EventRef{
		Journal: startRef.Journal,
		Type:    EventTaskDone,
		At:      end,
		HasSeq:  true,
		Seq:     3,
		Line:    4,
	}
	if outcome == OutcomeBlocked {
		termRef.Type = EventTaskBlocked
	}
	modelRef := startRef
	modelRef.Type = EventTaskSpawnFinished
	modelRef.At = id.StartedAt.Add(elapsed / 2)
	modelRef.Seq = 2
	modelRef.Line = 3
	ev := ObservationEvidence{
		Start: FieldEvidence{Source: EvidenceJournal, Event: startRef},
		Model: FieldEvidence{Source: modelSrc},
	}
	if modelKnown {
		ev.Model.Event = modelRef
	}
	if terminalSrc != EvidenceNone {
		ev.Terminal = FieldEvidence{Source: terminalSrc, Event: termRef}
		ev.Elapsed = FieldEvidence{Source: terminalSrc, Event: termRef}
	}
	return Attempt{
		ID:         id,
		Start:      startRef,
		Model:      measuredModel,
		Outcome:    outcome,
		TerminalAt: end,
		Cutoff:     contractCutoff(),
		Elapsed:    elapsed,
		Wall:       WallBreakdown{StartedAt: id.StartedAt, Elapsed: elapsed, Intervals: []Interval{}, Complete: false},
		CostScope:  CostScopeRecordedSpawns,
		Evidence:   ev,
	}
}

func attemptSetOf(run string, attempts ...Attempt) AttemptSet {
	return AttemptSet{
		Journal:     JournalIdentity{RunID: run, SourceID: "journals", Path: "journal.jsonl", Producer: ProducerDispatcherV0_1_0},
		Attempts:    attempts,
		Ambiguous:   []AmbiguousAttempt{},
		Conflicts:   []AttemptConflict{},
		Diagnostics: JournalDiagnostics{},
	}
}

func joinContract(t *testing.T, sets []AttemptSet, readings []Reading, selection Selection, journals []JournalIdentity) (EvidenceJoin, error) {
	t.Helper()
	return JoinEvidence(sets, readings, selection, journals)
}

func mustJoin(t *testing.T, sets []AttemptSet, readings []Reading, selection Selection, journals []JournalIdentity) EvidenceJoin {
	t.Helper()
	got, err := joinContract(t, sets, readings, selection, journals)
	if err != nil {
		t.Fatalf("JoinEvidence: %v", err)
	}
	return got
}

func dispositionCount(join EvidenceJoin, d Disposition) int {
	for _, row := range join.Dispositions {
		if row.Disposition == d {
			return row.Count
		}
	}
	return 0
}

func requireDisposition(t *testing.T, join EvidenceJoin, d Disposition, want int) {
	t.Helper()
	if got := dispositionCount(join, d); got != want {
		t.Fatalf("disposition %s = %d, want %d (examined=%d dispositions=%+v)", d, got, want, len(join.Examined), join.Dispositions)
	}
}

func intervalsOverlap(a, b Interval) bool {
	return a.Start.Before(b.End) && b.Start.Before(a.End)
}

func requireDisjointIntervals(t *testing.T, wall WallBreakdown) {
	t.Helper()
	for i := range wall.Intervals {
		iv := wall.Intervals[i]
		if iv.Phase == PhaseUnclassified {
			t.Fatalf("unclassified interval present: %+v", iv)
		}
		if !iv.Start.Before(iv.End) && !iv.Start.Equal(iv.End) {
			t.Fatalf("reversed interval: %+v", iv)
		}
		if iv.Start.Before(wall.StartedAt) || iv.End.After(wall.StartedAt.Add(wall.Elapsed)) {
			t.Fatalf("interval leaves the attempt: %+v wall=%+v", iv, wall)
		}
		for j := i + 1; j < len(wall.Intervals); j++ {
			if intervalsOverlap(iv, wall.Intervals[j]) {
				t.Fatalf("overlapping intervals: %+v and %+v", iv, wall.Intervals[j])
			}
		}
	}
}

func classifiedSum(wall WallBreakdown) time.Duration {
	var sum time.Duration
	for _, iv := range wall.Intervals {
		sum += iv.End.Sub(iv.Start)
	}
	return sum
}

func encodeJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func journalLine(seq int, typ, key, at string, payload any) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	wire := struct {
		Seq       int             `json:"seq"`
		Hash      string          `json:"hash"`
		PrevHash  string          `json:"prev_hash"`
		EventType string          `json:"event_type"`
		TaskKey   string          `json:"task_key,omitempty"`
		Timestamp string          `json:"timestamp"`
		Payload   json.RawMessage `json:"payload"`
	}{
		Seq:       seq,
		Hash:      fmt.Sprintf("%064d", seq+1),
		PrevHash:  fmt.Sprintf("%064d", seq),
		EventType: typ,
		TaskKey:   key,
		Timestamp: at,
		Payload:   raw,
	}
	data, err := json.Marshal(wire)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func parseLines(t *testing.T, run string, lines ...string) ParsedJournal {
	t.Helper()
	body := strings.Join(lines, "\n") + "\n"
	parsed, err := ParseEvents(context.Background(), journalID(run, "inline.jsonl"), strings.NewReader(body), JournalBounds{})
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	return parsed
}

func reduceLines(t *testing.T, run string, lines ...string) AttemptSet {
	t.Helper()
	set, err := ReduceAttempts(parseLines(t, run, lines...), contractCutoff())
	if err != nil {
		t.Fatalf("ReduceAttempts: %v", err)
	}
	return set
}

func identityUniverse(sets ...AttemptSet) []JournalIdentity {
	out := make([]JournalIdentity, 0, len(sets))
	for _, set := range sets {
		out = append(out, set.Journal)
	}
	return out
}

func mustMkdirAllT(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFileT(t *testing.T, path, data string) {
	t.Helper()
	mustMkdirAllT(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	setFixtureMTime(t, path)
}

func shallowClone(t *testing.T, src string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "shallow")
	// The test fixture explicitly opts into local file transport. Production
	// sourceGitCommand remains pinned to protocol.file.allow=never.
	runGit(t, src, "-c", "protocol.file.allow=always", "clone", "--depth", "1", "file://"+src, dst)
	freezeFixtureTree(t, dst)
	return dst
}

func gitOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = fixtureGitIsolatedEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func installFixtureGitWrapper(t *testing.T, body string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	realGit, err = filepath.Abs(realGit)
	if err != nil {
		t.Fatalf("resolve absolute Git path: %v", err)
	}
	bin := t.TempDir()
	wrapper := filepath.Join(bin, "git")
	writeFileT(t, wrapper, "#!/bin/sh\nset -eu\nreal_git="+shellQuote(realGit)+"\n"+body+"\n")
	if err := os.Chmod(wrapper, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func fixturePathAppears(path string, within time.Duration) bool {
	deadline := time.NewTimer(within)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		select {
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}
}

func observeFixturePathPresence(within time.Duration, paths ...string) (string, error) {
	deadline := time.NewTimer(within)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, path := range paths {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			} else if !os.IsNotExist(err) {
				return "", fmt.Errorf("inspect fixture marker %s: %w", path, err)
			}
		}
		select {
		case <-deadline.C:
			return "", nil
		case <-ticker.C:
		}
	}
}

func containPath(readings []Reading, fragment string) bool {
	for _, r := range readings {
		if strings.Contains(r.Ref.Path, fragment) {
			return true
		}
	}
	return false
}

func amendedBuildOpts(specs []SourceSpec, selection Selection, bounds ReadBounds) BuildOptions {
	return BuildOptions{Sources: specs, Selection: selection, Bounds: bounds, Now: contractCutoff()}
}

func schema4Artifact(manifest *SourceManifest, evidence *ArtifactEvidence) Artifact {
	return Artifact{
		SchemaVersion:  AmendedEvidenceSchemaVersion,
		GeneratedAt:    contractCutoff(),
		Limits:         []string{HandFinishedLimit},
		SourceManifest: manifest,
		Evidence:       evidence,
		Observations:   []ReferenceObservation{},
		Cells:          []CellSummary{},
	}
}

func completeManifest(state SourceState) *SourceManifest {
	return &SourceManifest{
		State:         state,
		Cutoff:        contractCutoff(),
		HoldoutRunIDs: []string{},
		Reasons:       []string{},
		Bounds: ReadBounds{
			MaxCommits:    DefaultMaxCommits,
			MaxLineBytes:  DefaultMaxLineBytes,
			MaxBlobBytes:  DefaultMaxBlobBytes,
			MaxTotalBytes: DefaultMaxTotalBytes,
			MaxProcesses:  DefaultMaxProcesses,
		},
		Sources: []SourceReport{{
			ID:           "journals",
			Kind:         SourceKindJournals,
			Repository:   "/tmp/runs",
			Roots:        []string{},
			State:        state,
			Reasons:      []string{},
			ResolvedRefs: []ResolvedRef{},
			Counts:       SourceCounts{Journals: 1, Records: 1},
		}},
	}
}
