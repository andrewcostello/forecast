package dispatched

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type startNotifyReadCloser struct {
	io.ReadCloser
	started chan struct{}
	once    sync.Once
}

func (r *startNotifyReadCloser) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.ReadCloser.Read(p)
}

func closeSourceReader(t *testing.T, reader io.ReadCloser, label string) error {
	t.Helper()
	if reader == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- reader.Close() }()
	timer := time.NewTimer(fixtureHarnessWait)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		t.Fatalf("close %s did not return within %s", label, fixtureHarnessWait)
		return nil
	}
}

func waitClosedChan(t *testing.T, started <-chan struct{}, label string) {
	t.Helper()
	timer := time.NewTimer(fixtureHarnessWait)
	defer timer.Stop()
	select {
	case <-started:
	case <-timer.C:
		t.Fatalf("%s did not start reading within %s", label, fixtureHarnessWait)
	}
}

func panelSourceByID(t *testing.T, manifest *SourceManifest, id string) SourceReport {
	t.Helper()
	if manifest == nil {
		t.Fatal("nil manifest")
	}
	for _, src := range manifest.Sources {
		if src.ID == id {
			return src
		}
	}
	t.Fatalf("source %q missing: %+v", id, manifest.Sources)
	return SourceReport{}
}

func panelResolvedRef(refs []ResolvedRef, name string) (ResolvedRef, bool) {
	for _, ref := range refs {
		if ref.Name == name {
			return ref, true
		}
	}
	return ResolvedRef{}, false
}

func gitArgvHasCommitLimiter(argv string, n int) bool {
	want := fmt.Sprintf("%d", n)
	fields := strings.Fields(argv)
	for i, field := range fields {
		switch {
		case field == "--max-count="+want, field == "-n"+want:
			return true
		case (field == "--max-count" || field == "-n") && i+1 < len(fields) && fields[i+1] == want:
			return true
		}
	}
	return false
}

func validCompleteManifest() *SourceManifest {
	sha := strings.Repeat("ab", 20)
	m := completeManifest(SourceComplete)
	m.Sources = append(m.Sources, SourceReport{
		ID:           "hist",
		Kind:         SourceKindGitHistory,
		Repository:   "/tmp/repo",
		Roots:        []string{"features"},
		State:        SourceComplete,
		Reasons:      []string{},
		ResolvedRefs: []ResolvedRef{{Name: "refs/heads/main", Commit: sha}},
		Counts:       SourceCounts{Commits: 1, Records: 1},
	})
	return m
}

func requireJSONPresentArray(t *testing.T, object map[string]json.RawMessage, key string) []json.RawMessage {
	t.Helper()
	raw, ok := object[key]
	if !ok {
		t.Fatalf("JSON object omitted required %q key", key)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		t.Fatalf("JSON key %q is null", key)
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatalf("JSON key %q is not an array: %s (%v)", key, raw, err)
	}
	return values
}

func parseEnvelopeKind(t *testing.T, data []byte) Reading {
	t.Helper()
	got, err := parseReadings(data, ReadingRef{SourceID: "live", Path: "doc.yaml", Revision: "live", RecordedAt: contractCutoff()})
	if err != nil {
		t.Fatalf("parseReadings top-level error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("envelopes = %d, want 1: %+v", len(got), got)
	}
	return got[0]
}

// F3-GIT-CLOSE-ERRORS: Close without a prior Read must surface child/budget
// failures. Successful EOF then Close is nil. Close after Read already
// delivered an error stays nil so existing closeFixtureReader assertions remain.
func testF3GitCloseErrors(t *testing.T) {
	t.Run("eof-then-close", func(t *testing.T) {
		repo := initGitRepo(t)
		budget, err := newSourceBudget(ReadBounds{})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		reader, err := runSourceGit(ctx, repo.path, budget, SourceGitRequest{Args: []string{"rev-parse", "--is-shallow-repository"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadAll(reader); err != nil {
			t.Fatalf("successful git read: %v", err)
		}
		if err := closeSourceReader(t, reader, "successful git reader"); err != nil {
			t.Fatalf("EOF then Close = %v, want nil", err)
		}
	})

	t.Run("nonzero-exit", func(t *testing.T) {
		installFixtureGitWrapper(t, "exit 7")
		budget, err := newSourceBudget(ReadBounds{MaxProcesses: 1, MaxTotalBytes: 1024})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		call := startFixtureGit(ctx, t.TempDir(), budget, SourceGitRequest{Args: []string{"rev-parse", "HEAD"}})
		pending := true
		var reader io.ReadCloser
		t.Cleanup(func() {
			cancel()
			if pending {
				if completed, ok := waitFixtureGit(call, fixtureHarnessWait); ok {
					pending = false
					reader = completed.reader
				}
			}
			_ = closeSourceReader(t, reader, "nonzero git reader cleanup")
		})
		started, ok := waitFixtureGit(call, fixtureHarnessWait)
		if !ok {
			t.Fatal("runSourceGit did not return within the harness bound")
		}
		pending = false
		if started.err != nil {
			t.Fatal(started.err)
		}
		reader = started.reader
		if reader == nil {
			t.Fatal("nil git reader")
		}
		err = closeSourceReader(t, reader, "nonzero git reader")
		reader = nil
		if !errors.Is(err, ErrGitHistory) {
			t.Fatalf("Close after nonzero child = %v, want ErrGitHistory", err)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		state := t.TempDir()
		release := filepath.Join(state, "release")
		installFixtureGitWrapper(t, fmt.Sprintf("while [ ! -e %s ]; do sleep 1; done", shellQuote(release)))
		budget, err := newSourceBudget(ReadBounds{MaxProcesses: 1, MaxTotalBytes: 1024})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		call := startFixtureGit(ctx, t.TempDir(), budget, SourceGitRequest{Args: []string{"rev-parse", "HEAD"}})
		pending := true
		var reader io.ReadCloser
		t.Cleanup(func() {
			releaseFixturePath(t, release)
			cancel()
			if pending {
				if completed, ok := waitFixtureGit(call, fixtureHarnessWait); ok {
					pending = false
					reader = completed.reader
				}
			}
			_ = closeSourceReader(t, reader, "cancelled git reader cleanup")
		})
		started, ok := waitFixtureGit(call, fixtureHarnessWait)
		if !ok {
			t.Fatal("runSourceGit did not return a waiting child within the harness bound")
		}
		pending = false
		if started.err != nil {
			t.Fatal(started.err)
		}
		reader = started.reader
		if reader == nil {
			t.Fatal("nil git reader")
		}
		cancel()
		err = closeSourceReader(t, reader, "cancelled git reader")
		reader = nil
		if !errors.Is(err, ErrSourceCancelled) || !errors.Is(err, context.Canceled) {
			t.Fatalf("Close after cancel = %v, want ErrSourceCancelled and context.Canceled", err)
		}
	})

	t.Run("bound", func(t *testing.T) {
		state := t.TempDir()
		marker := filepath.Join(state, "wrote-stderr")
		release := filepath.Join(state, "release")
		installFixtureGitWrapper(t, fmt.Sprintf(
			"printf '%%s' %s >&2\n: > %s\nwhile [ ! -e %s ]; do sleep 1; done",
			shellQuote(strings.Repeat("e", 64)), shellQuote(marker), shellQuote(release),
		))
		budget, err := newSourceBudget(ReadBounds{MaxProcesses: 1, MaxTotalBytes: 8, MaxBlobBytes: 1024})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		call := startFixtureGit(ctx, t.TempDir(), budget, SourceGitRequest{Args: []string{"rev-parse", "HEAD"}})
		pending := true
		var reader io.ReadCloser
		t.Cleanup(func() {
			releaseFixturePath(t, release)
			cancel()
			if pending {
				if completed, ok := waitFixtureGit(call, fixtureHarnessWait); ok {
					pending = false
					reader = completed.reader
				}
			}
			_ = closeSourceReader(t, reader, "bound git reader cleanup")
		})
		started, ok := waitFixtureGit(call, fixtureHarnessWait)
		if !ok {
			t.Fatal("runSourceGit did not return a streaming reader within the harness bound")
		}
		pending = false
		if started.err != nil {
			t.Fatal(started.err)
		}
		reader = started.reader
		if reader == nil {
			t.Fatal("nil git reader")
		}
		if !fixturePathAppears(marker, fixtureHarnessWait) {
			t.Fatal("controlled child never wrote over-cap stderr")
		}
		deadline := time.NewTimer(fixtureHarnessWait)
		ticker := time.NewTicker(5 * time.Millisecond)
		hit := false
		for !hit {
			if budget.hitTotalBound() {
				hit = true
				break
			}
			select {
			case <-deadline.C:
				ticker.Stop()
				t.Fatal("shared total-byte bound was not charged from stderr before Close")
			case <-ticker.C:
			}
		}
		ticker.Stop()
		deadline.Stop()
		err = closeSourceReader(t, reader, "bound git reader")
		reader = nil
		if !errors.Is(err, ErrBoundExceeded) {
			t.Fatalf("Close after total-byte bound = %v, want ErrBoundExceeded", err)
		}
	})
}

// F3-BOUND-TOTAL-CONCURRENT: shared MaxTotalBytes is charged before collection
// across concurrent streams. One overflow probe byte is counted as consumed
// and is the only slack; retained payload stays at the inclusive cap.
//
// Readiness is established before reservation, so this frozen seam cannot
// prove both readers have acquired allowance. The short pause only keeps
// children from completing before release; it is not a serialization proof.
// The demonstrated old-body failure (bytesRead=130 against cap 64) is
// positive mutation evidence that check-then-charge over-allocates.
func testF3BoundTotalConcurrent(t *testing.T) {
	const capBytes int64 = 64
	payload := strings.Repeat("a", int(capBytes)) + "Z"
	state := t.TempDir()
	release := filepath.Join(state, "release")
	done := filepath.Join(state, "done")
	installFixtureGitWrapper(t, fmt.Sprintf(
		"while [ ! -e %s ]; do sleep 1; done\nprintf '%%s' %s\nwhile [ ! -e %s ]; do sleep 1; done",
		shellQuote(release), shellQuote(payload), shellQuote(done),
	))
	budget, err := newSourceBudget(ReadBounds{MaxTotalBytes: capBytes, MaxProcesses: 2, MaxBlobBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	repo := t.TempDir()
	firstCall := startFixtureGit(ctx, repo, budget, SourceGitRequest{Args: []string{"rev-parse", "HEAD"}})
	secondCall := startFixtureGit(ctx, repo, budget, SourceGitRequest{Args: []string{"rev-parse", "HEAD"}})
	firstPending, secondPending := true, true
	var first, second io.ReadCloser
	t.Cleanup(func() {
		releaseFixturePath(t, release)
		releaseFixturePath(t, done)
		cancel()
		if firstPending {
			if completed, ok := waitFixtureGit(firstCall, fixtureHarnessWait); ok {
				firstPending = false
				first = completed.reader
			}
		}
		if secondPending {
			if completed, ok := waitFixtureGit(secondCall, fixtureHarnessWait); ok {
				secondPending = false
				second = completed.reader
			}
		}
		_ = closeSourceReader(t, first, "concurrent reader A cleanup")
		_ = closeSourceReader(t, second, "concurrent reader B cleanup")
	})
	firstStarted, ok := waitFixtureGit(firstCall, fixtureHarnessWait)
	if !ok {
		t.Fatal("first concurrent runSourceGit did not return a reader")
	}
	firstPending = false
	secondStarted, ok := waitFixtureGit(secondCall, fixtureHarnessWait)
	if !ok {
		t.Fatal("second concurrent runSourceGit did not return a reader")
	}
	secondPending = false
	if firstStarted.err != nil {
		t.Fatal(firstStarted.err)
	}
	if secondStarted.err != nil {
		t.Fatal(secondStarted.err)
	}
	if firstStarted.reader == nil || secondStarted.reader == nil {
		t.Fatal("nil concurrent git readers")
	}
	notifyA := &startNotifyReadCloser{ReadCloser: firstStarted.reader, started: make(chan struct{})}
	notifyB := &startNotifyReadCloser{ReadCloser: secondStarted.reader, started: make(chan struct{})}
	first, second = notifyA, notifyB
	readA := startFixtureRead(notifyA)
	readB := startFixtureRead(notifyB)
	waitClosedChan(t, notifyA.started, "concurrent reader A")
	waitClosedChan(t, notifyB.started, "concurrent reader B")
	// Both Reads have entered waitSourceReadable. Because readiness is
	// checked before reservation, that is not proof both streams hold
	// allowance. Keep the pause so children stay blocked until release.
	time.Sleep(100 * time.Millisecond)
	if got, ok := waitFixtureRead(readA, 50*time.Millisecond); ok {
		t.Fatalf("reader A completed before release: len=%d err=%v", len(got.data), got.err)
	}
	if got, ok := waitFixtureRead(readB, 50*time.Millisecond); ok {
		t.Fatalf("reader B completed before release: len=%d err=%v", len(got.data), got.err)
	}
	releaseFixturePath(t, release)
	gotA, ok := waitFixtureRead(readA, fixtureHarnessWait)
	if !ok {
		t.Fatal("reader A did not finish after release")
	}
	gotB, ok := waitFixtureRead(readB, fixtureHarnessWait)
	if !ok {
		t.Fatal("reader B did not finish after release")
	}
	releaseFixturePath(t, done)
	if err := closeSourceReader(t, first, "concurrent reader A"); err != nil && !errors.Is(err, ErrBoundExceeded) {
		t.Errorf("close A after bound read = %v", err)
	}
	first = nil
	if err := closeSourceReader(t, second, "concurrent reader B"); err != nil && !errors.Is(err, ErrBoundExceeded) {
		t.Errorf("close B after bound read = %v", err)
	}
	second = nil
	cancel()

	retained := int64(len(gotA.data) + len(gotB.data))
	if retained > capBytes {
		t.Errorf("retained %d bytes across concurrent streams, inclusive cap is %d", retained, capBytes)
	}
	if strings.Contains(string(gotA.data)+string(gotB.data), "Z") {
		t.Errorf("overflow probe byte was retained: a=%q b=%q", gotA.data, gotB.data)
	}
	if !errors.Is(gotA.err, ErrBoundExceeded) && !errors.Is(gotB.err, ErrBoundExceeded) {
		t.Errorf("concurrent over-cap read errors a=%v b=%v, want ErrBoundExceeded on at least one stream", gotA.err, gotB.err)
	}
	// Physical accounting includes the single necessary overflow probe and
	// nothing more. Two full 64-byte collections against one remaining budget
	// is the defect.
	if got := budget.bytesRead(); got > capBytes+1 {
		t.Fatalf("shared MaxTotalBytes over-allocated before collection: bytesRead=%d cap=%d (one overflow probe allowed)", got, capBytes)
	}
}

func testF3CompleteResolution(t *testing.T) {
	if err := validCompleteManifest().ValidateComplete(); err != nil {
		t.Fatalf("control complete manifest rejected: %v", err)
	}

	sha := strings.Repeat("ab", 20)
	sha2 := strings.Repeat("cd", 20)
	cases := []struct {
		name   string
		mutate func(*SourceManifest)
	}{
		{
			name: "all-refs-blank-name",
			mutate: func(m *SourceManifest) {
				m.Sources[1].ResolvedRefs = []ResolvedRef{{Name: "", Commit: sha}}
			},
		},
		{
			name: "all-refs-padded-name",
			mutate: func(m *SourceManifest) {
				m.Sources[1].ResolvedRefs = []ResolvedRef{{Name: " refs/heads/main", Commit: sha}}
			},
		},
		{
			name: "all-refs-malformed-commit",
			mutate: func(m *SourceManifest) {
				m.Sources[1].ResolvedRefs = []ResolvedRef{{Name: "refs/heads/main", Commit: "not-a-commit"}}
			},
		},
		{
			name: "all-refs-uppercase-commit",
			mutate: func(m *SourceManifest) {
				m.Sources[1].ResolvedRefs = []ResolvedRef{{Name: "refs/heads/main", Commit: strings.ToUpper(sha)}}
			},
		},
		{
			name: "all-refs-duplicate-name",
			mutate: func(m *SourceManifest) {
				m.Sources[1].ResolvedRefs = []ResolvedRef{
					{Name: "refs/heads/main", Commit: sha},
					{Name: "refs/heads/main", Commit: sha2},
				}
			},
		},
		{
			name: "all-refs-duplicate-entry",
			mutate: func(m *SourceManifest) {
				m.Sources[1].ResolvedRefs = []ResolvedRef{
					{Name: "refs/heads/main", Commit: sha},
					{Name: "refs/heads/main", Commit: sha},
				}
			},
		},
		{
			name: "live-resolved-ref",
			mutate: func(m *SourceManifest) {
				m.Sources = append(m.Sources, SourceReport{
					ID: "live", Kind: SourceKindLiveYAML, Repository: "/tmp/repo",
					Roots: []string{"features"}, State: SourceComplete, Reasons: []string{},
					ResolvedRef: sha, ResolvedRefs: []ResolvedRef{},
					Counts: SourceCounts{Files: 1, Records: 1},
				})
			},
		},
		{
			name: "live-resolved-refs",
			mutate: func(m *SourceManifest) {
				m.Sources = append(m.Sources, SourceReport{
					ID: "live", Kind: SourceKindLiveYAML, Repository: "/tmp/repo",
					Roots: []string{"features"}, State: SourceComplete, Reasons: []string{},
					ResolvedRefs: []ResolvedRef{{Name: "HEAD", Commit: sha}},
					Counts:       SourceCounts{Files: 1, Records: 1},
				})
			},
		},
		{
			name: "journal-resolved-refs",
			mutate: func(m *SourceManifest) {
				m.Sources[0].ResolvedRefs = []ResolvedRef{{Name: "HEAD", Commit: sha}}
			},
		},
		{
			name: "journal-resolved-ref",
			mutate: func(m *SourceManifest) {
				m.Sources[0].ResolvedRef = sha
			},
		},
		{
			name: "complete-source-reasons",
			mutate: func(m *SourceManifest) {
				m.Sources[1].Reasons = []string{"Git history is shallow"}
				m.Reasons = []string{"hist: Git history is shallow"}
			},
		},
		{
			name: "complete-aggregate-reasons",
			mutate: func(m *SourceManifest) {
				m.Reasons = []string{"reduce: ErrReversedInterval: wall"}
			},
		},
		{
			name: "all-refs-double-dot-name",
			mutate: func(m *SourceManifest) {
				m.Sources[1].ResolvedRefs = []ResolvedRef{{Name: "refs/heads/a..b", Commit: sha}}
			},
		},
		{
			name: "all-refs-lock-suffix",
			mutate: func(m *SourceManifest) {
				m.Sources[1].ResolvedRefs = []ResolvedRef{{Name: "refs/heads/main.lock", Commit: sha}}
			},
		},
		{
			name: "all-refs-control-char",
			mutate: func(m *SourceManifest) {
				m.Sources[1].ResolvedRefs = []ResolvedRef{{Name: "refs/heads/main\x01", Commit: sha}}
			},
		},
		{
			name: "all-refs-outside-refs",
			mutate: func(m *SourceManifest) {
				m.Sources[1].ResolvedRefs = []ResolvedRef{{Name: "heads/main", Commit: sha}}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validCompleteManifest()
			tc.mutate(m)
			err := m.ValidateComplete()
			if !errors.Is(err, ErrSourceIncomplete) {
				t.Fatalf("ValidateComplete(%s) = %v, want ErrSourceIncomplete", tc.name, err)
			}
		})
	}

	t.Run("all-refs-head-pseudoref-accepted", func(t *testing.T) {
		m := validCompleteManifest()
		m.Sources[1].ResolvedRefs = []ResolvedRef{
			{Name: "HEAD", Commit: sha},
			{Name: "refs/heads/main", Commit: sha2},
		}
		if err := m.ValidateComplete(); err != nil {
			t.Fatalf("all-ref HEAD pseudoref rejected: %v", err)
		}
	})
	t.Run("explicit-requested-head-accepted", func(t *testing.T) {
		m := validCompleteManifest()
		m.Sources[1].RequestedRef = "HEAD"
		m.Sources[1].ResolvedRef = sha
		m.Sources[1].ResolvedRefs = []ResolvedRef{{Name: "HEAD", Commit: sha}}
		if err := m.ValidateComplete(); err != nil {
			t.Fatalf("explicit requested HEAD rejected: %v", err)
		}
	})
}

func testF3EmptyHistoryConsistent(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-root", "synthetic-utc-offset.jsonl")
	repo := initGitRepo(t)
	manifest, _, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", repo.path, "", "features"),
	}, defaultSelection(), ReadBounds{})
	if manifest == nil {
		t.Fatalf("nil manifest on empty history: %v", err)
	}
	hist := panelSourceByID(t, manifest, "hist")
	if hist.State == SourceComplete && len(hist.ResolvedRefs) == 0 && hist.RequestedRef == "" {
		t.Fatal("empty all-refs history reported COMPLETE with no resolved refs")
	}
	if hist.Counts.Commits != 0 {
		t.Fatalf("unborn repository reported commits=%d", hist.Counts.Commits)
	}
	validateErr := manifest.ValidateComplete()
	if manifest.State == SourceComplete && validateErr != nil {
		t.Fatalf("ReadSources returned COMPLETE (%v) but ValidateComplete rejects the same manifest: %v", err, validateErr)
	}
	if hist.RequestedRef == "" && len(hist.ResolvedRefs) == 0 && validateErr == nil {
		t.Fatal("ValidateComplete accepted an empty all-refs resolution")
	}
	if !errors.Is(validateErr, ErrSourceIncomplete) && manifest.State == SourceComplete {
		t.Fatalf("COMPLETE empty history ValidateComplete = %v", validateErr)
	}
	if hist.State != SourcePartial {
		t.Fatalf("empty all-refs history state = %q, want PARTIAL with a diagnostic", hist.State)
	}
	if len(hist.Reasons) == 0 {
		t.Fatal("empty all-refs history missing diagnostic reason")
	}
	if hist.ResolvedRefs == nil {
		t.Fatal("empty all-refs ResolvedRefs is nil, want canonical empty list")
	}
	if len(hist.ResolvedRefs) != 0 {
		t.Fatalf("unborn history invented resolved refs: %+v", hist.ResolvedRefs)
	}
	if !errors.Is(validateErr, ErrSourceIncomplete) {
		t.Fatalf("empty all-refs ValidateComplete = %v, want ErrSourceIncomplete", validateErr)
	}
	raw, marshalErr := json.Marshal(hist)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	var object map[string]json.RawMessage
	if unmarshalErr := json.Unmarshal(raw, &object); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	if len(requireJSONPresentArray(t, object, "resolved_refs")) != 0 {
		t.Fatalf("empty all-refs resolved_refs serialized nonempty: %s", raw)
	}
	if len(requireJSONPresentArray(t, object, "reasons")) == 0 {
		t.Fatalf("empty all-refs reasons serialized empty: %s", raw)
	}
	requireJSONPresentArray(t, object, "roots")
}

func testF3BoundCommitsLimiter(t *testing.T) {
	runs, repo := contractSourceTree(t)
	for i := 0; i < 4; i++ {
		repo.write("features/study/n.txt", strings.Repeat("x", i+1))
		repo.commit("c", "features")
	}
	logPath := filepath.Join(t.TempDir(), "git-argv.log")
	installFixtureGitWrapper(t, fmt.Sprintf("printf '%%s\\n' \"$*\" >> %s\nexec \"$real_git\" \"$@\"", shellQuote(logPath)))
	manifest, _, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", repo.path, "", "features"),
	}, defaultSelection(), ReadBounds{MaxCommits: 3})
	if err != nil {
		t.Fatal(err)
	}
	hist := panelSourceByID(t, manifest, "hist")
	if hist.Counts.Commits != 3 || hist.Counts.BoundsExceeded < 1 {
		t.Fatalf("commit cap semantics changed: %+v", hist.Counts)
	}
	raw, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read git argv log: %v", readErr)
	}
	foundRevList := false
	limited := false
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.Contains(line, "rev-list") {
			continue
		}
		foundRevList = true
		if gitArgvHasCommitLimiter(line, 4) {
			limited = true
			break
		}
	}
	if !foundRevList {
		t.Fatal("history read never invoked a rev-list metadata command")
	}
	if !limited {
		t.Fatalf("rev-list lacked provider-side MaxCommits+1 limiter (--max-count=4 or -n4); argv log:\n%s", raw)
	}
}

func testF3GitWorktreeGrafts(t *testing.T) {
	runs, repo := contractSourceTree(t)
	grafted := filepath.Join(repo.path, ".git", "info")
	mustMkdirAllT(t, grafted)
	writeFileT(t, filepath.Join(grafted, "grafts"), "deadbeef parent\n")
	worktree := filepath.Join(t.TempDir(), "linked")
	runGit(t, repo.path, "worktree", "add", worktree)
	gitfile, err := os.ReadFile(filepath.Join(worktree, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(gitfile, []byte("gitdir:")) {
		t.Fatalf("expected linked worktree gitfile, got %q", gitfile)
	}
	manifest, _, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", worktree, "", "features"),
	}, defaultSelection(), ReadBounds{})
	if err != nil {
		t.Fatal(err)
	}
	hist := panelSourceByID(t, manifest, "hist")
	if !hist.Grafted {
		t.Fatalf("linked worktree graft not reported: %+v", hist)
	}
	if hist.Replaced {
		t.Fatalf("grafted-only worktree reported Replaced: %+v", hist)
	}
	if hist.State != SourcePartial {
		t.Fatalf("grafted worktree state = %q, want PARTIAL", hist.State)
	}
	if err := manifest.ValidateComplete(); !errors.Is(err, ErrShallowHistory) || !errors.Is(err, ErrSourceIncomplete) {
		t.Fatalf("grafted worktree ValidateComplete = %v, want ErrShallowHistory+ErrSourceIncomplete", err)
	}
}

func testF3AllowEmptyReasons(t *testing.T) {
	empty := t.TempDir()
	repo := initGitRepo(t)
	repo.writeTestdata("features/study/tasks.yaml", "yaml", "offset-equivalent.yaml")
	repo.commit("seed", "features")
	mustMkdirAllT(t, filepath.Join(repo.path, ".git", "info"))
	writeFileT(t, filepath.Join(repo.path, ".git", "info", "grafts"), "deadbeef parent\n")
	manifest, _, err := ReadSources(context.Background(), []SourceSpec{
		journalSpec("j", empty),
		historySpec("hist", repo.path, "", "features"),
	}, Selection{Cutoff: contractCutoff(), AllowEmpty: true}, ReadBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.State != SourceEmpty {
		t.Fatalf("allow-empty state = %q, want EMPTY", manifest.State)
	}
	hist := panelSourceByID(t, manifest, "hist")
	if !hist.Grafted || hist.State != SourcePartial {
		t.Fatalf("partial history diagnostics dropped under AllowEmpty: %+v", hist)
	}
	if len(manifest.Reasons) == 0 {
		t.Fatal("AllowEmpty dropped aggregate reasons from partial source diagnostics")
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	if len(requireJSONPresentArray(t, object, "reasons")) == 0 {
		t.Fatalf("aggregate reasons serialized empty: %s", raw)
	}
	requireJSONPresentArray(t, object, "holdout_run_ids")
	sourcesRaw, ok := object["sources"]
	if !ok || string(sourcesRaw) == "null" {
		t.Fatalf("manifest sources omitted/null: %s", raw)
	}
	var sources []map[string]json.RawMessage
	if err := json.Unmarshal(sourcesRaw, &sources); err != nil {
		t.Fatalf("manifest sources missing/not array: %s (%v)", raw, err)
	}
	if len(sources) != 2 {
		t.Fatalf("manifest source count = %d, want 2", len(sources))
	}
	for _, source := range sources {
		requireJSONPresentArray(t, source, "reasons")
		requireJSONPresentArray(t, source, "resolved_refs")
		requireJSONPresentArray(t, source, "roots")
	}
	if err := manifest.ValidateComplete(); !errors.Is(err, ErrSourceIncomplete) {
		t.Fatalf("EMPTY+partial AllowEmpty eligible: %v", err)
	}
}

func testF3NonTaskShapes(t *testing.T) {
	ref := ReadingRef{SourceID: "live", Path: "doc.yaml", Revision: "live", RecordedAt: contractCutoff()}
	t.Run("parse", func(t *testing.T) {
		for _, data := range [][]byte{[]byte(""), []byte("[]\n"), []byte("just-a-scalar\n"), []byte("null\n")} {
			got, err := parseReadings(data, ref)
			if err != nil {
				t.Errorf("parseReadings(%q) top-level error: %v", data, err)
				continue
			}
			if len(got) != 1 || got[0].Kind != DocumentNotTasks || got[0].Err != nil || got[0].Ref.Row != 0 {
				t.Errorf("non-task shape %q envelope = %+v, want DocumentNotTasks", data, got)
			}
		}
		malformed := parseEnvelopeKind(t, []byte(testdataFile(t, "yaml", "malformed-document.yaml")))
		if malformed.Kind != DocumentMalformed || malformed.Err == nil || malformed.Ref.Row != 0 {
			t.Fatalf("invalid YAML syntax envelope = %+v, want DocumentMalformed", malformed)
		}
		emptyTasks, err := parseReadings([]byte(testdataFile(t, "yaml", "empty-tasks.yaml")), ref)
		if err != nil {
			t.Fatal(err)
		}
		if len(emptyTasks) != 0 {
			t.Fatalf("valid empty tasks list = %+v, want no row envelopes", emptyTasks)
		}
	})

	t.Run("ingest", func(t *testing.T) {
		runs := filepath.Join(t.TempDir(), "runs")
		writeJournalTree(t, runs, "run-root", "synthetic-utc-offset.jsonl")
		repo := initGitRepo(t)
		repo.write("features/study/tasks.yaml", testdataFile(t, "yaml", "offset-equivalent.yaml"))
		repo.write("features/study/empty.yaml", "")
		repo.write("features/study/list.yaml", "[]\n")
		repo.write("features/study/scalar.yaml", "just-a-scalar\n")
		repo.commit("shapes", "features")
		manifest, readings := mustReadSources(t, []SourceSpec{
			journalSpec("j", runs),
			liveSpec("live", repo.path, "features"),
		}, defaultSelection(), ReadBounds{})
		var nonTask, malformedCount int
		for _, r := range readings.Readings {
			if r.Kind == DocumentNotTasks {
				nonTask++
			}
			if r.Kind == DocumentMalformed {
				malformedCount++
			}
		}
		if nonTask != 3 {
			t.Fatalf("DocumentNotTasks envelopes = %d, want 3", nonTask)
		}
		if malformedCount != 0 {
			t.Fatalf("empty/list/scalar YAML counted malformed: %+v", readings.Readings)
		}
		live := panelSourceByID(t, manifest, "live")
		if live.Counts.NonTaskDocuments != 3 {
			t.Fatalf("NonTaskDocuments = %d, want 3", live.Counts.NonTaskDocuments)
		}
		if live.Counts.Malformed != 0 || live.State == SourcePartial {
			t.Fatalf("non-task shapes degraded completeness: %+v", live)
		}
	})
}

func testF3IdentityStructure(t *testing.T) {
	ref := ReadingRef{SourceID: "live", Path: "features/study/tasks.yaml", Revision: "live", RecordedAt: contractCutoff()}
	structural := []byte(`id: &run held
tasks:
  - key: SEQ
    role: bodies
    model: stamp
    started_at: '2026-01-01T00:00:00Z'
    dispatcher_run_id:
      - held
  - key: MAP
    role: bodies
    model: stamp
    started_at:
      at: '2026-01-01T00:00:00Z'
    dispatcher_run_id: run-root
  - key: ALIAS
    role: bodies
    model: stamp
    started_at: '2026-01-01T00:00:00Z'
    dispatcher_run_id: *run
  - key: ABSENT
    role: bodies
    model: stamp
    dispatcher_run_id: run-root
`)
	t.Run("parse", func(t *testing.T) {
		got, err := parseReadings(structural, ref)
		if err != nil {
			t.Fatal(err)
		}
		byKey := map[string]Reading{}
		for _, r := range got {
			if r.Identity.Key.Known {
				byKey[r.Identity.Key.Value] = r
			}
		}
		for _, key := range []string{"SEQ", "MAP", "ALIAS"} {
			row, ok := byKey[key]
			if !ok {
				t.Errorf("structural row %s dropped: %+v", key, got)
				continue
			}
			if row.Err == nil {
				t.Errorf("structural %s identity/time retained as valid: %+v", key, row)
			}
		}
		absent, ok := byKey["ABSENT"]
		if !ok {
			t.Fatalf("absent-field row dropped: %+v", got)
		}
		if absent.Present.StartedAt {
			t.Fatalf("genuine absent started_at marked present: %+v", absent)
		}
		if absent.Err != nil {
			t.Fatalf("genuine absent started_at became a structural error: %+v", absent)
		}
	})

	t.Run("ingest", func(t *testing.T) {
		runs := filepath.Join(t.TempDir(), "runs")
		writeJournalTree(t, runs, "held", "synthetic-utc-offset.jsonl")
		writeJournalTree(t, runs, "run-root", "synthetic-utc-offset.jsonl")
		repo := initGitRepo(t)
		repo.write("features/study/tasks.yaml", string(structural))
		repo.commit("structural", "features")
		manifest, readings := mustReadSources(t, []SourceSpec{
			journalSpec("j", runs),
			liveSpec("live", repo.path, "features"),
		}, Selection{Cutoff: contractCutoff(), HoldoutRunIDs: []string{"held"}}, ReadBounds{})
		var seq, alias Reading
		for _, r := range readings.Readings {
			if r.Identity.Key.Value == "SEQ" {
				seq = r
			}
			if r.Identity.Key.Value == "ALIAS" {
				alias = r
			}
		}
		if seq.Kind != DocumentTaskRow || seq.Err == nil {
			t.Errorf("sequence run id not auditable as malformed: %+v", seq)
		}
		if seq.Excluded == DispositionHeldOut {
			t.Error("sequence run id proved a holdout without a string scalar identity")
		}
		if alias.Err == nil {
			t.Errorf("alias run id not malformed: %+v", alias)
		}
		if alias.Excluded == DispositionHeldOut {
			t.Error("alias run id proved a holdout without a string scalar identity")
		}
		live := panelSourceByID(t, manifest, "live")
		if live.Counts.Malformed == 0 || live.State != SourcePartial {
			t.Fatalf("unprovable structural identity did not degrade completeness: %+v", live)
		}
	})
}

func testF3MissingRevisionTimeIngest(t *testing.T) {
	ref := ReadingRef{SourceID: "live", Repository: "repo", Path: "features/study/tasks.yaml", Revision: "live"}
	got, err := parseReadings([]byte(testdataFile(t, "yaml", "offset-equivalent.yaml")), ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != DocumentTaskRow || !got[0].Ref.RecordedAt.IsZero() || got[0].Err != nil || got[0].Excluded != "" {
		t.Fatalf("zero-time parser envelope = %+v", got)
	}
	report := &SourceReport{ID: "live", Kind: SourceKindLiveYAML, State: SourceComplete, Reasons: []string{}}
	readings := &SourceReadings{}
	ingestReadings(got, defaultSelection(), report, readings)
	if len(readings.Readings) != 1 {
		t.Fatalf("ingest dropped the zero-time envelope: %+v", readings.Readings)
	}
	row := readings.Readings[0]
	if row.Err == nil {
		t.Fatal("zero RecordedAt ingest dropped Reading.Err")
	}
	if row.Excluded != "" {
		t.Fatalf("zero RecordedAt excluded=%q, want in-sample malformed", row.Excluded)
	}
	if report.Counts.Malformed != 1 {
		t.Fatalf("Malformed = %d, want 1", report.Counts.Malformed)
	}
	if report.Counts.ExcludedAfterCutoff != 0 {
		t.Fatalf("AfterCutoff = %d, want 0", report.Counts.ExcludedAfterCutoff)
	}
	if report.State != SourcePartial {
		t.Fatalf("zero RecordedAt ingest state = %q, want PARTIAL", report.State)
	}
	if row.Snapshot.AuthoredModel == "" {
		t.Fatal("in-sample zero RecordedAt cleared snapshot as if excluded")
	}
}

func testF3GitRequestReadonly(t *testing.T) {
	sha := strings.Repeat("ab", 20)
	allowed := []SourceGitRequest{
		{Args: []string{"rev-parse", "--is-shallow-repository"}},
		{Args: []string{"rev-parse", "--absolute-git-dir"}},
		{Args: []string{"rev-parse", "--git-common-dir"}},
		{Args: []string{"rev-parse", "--verify", "--end-of-options", "HEAD^{commit}"}},
		{Args: []string{"for-each-ref", "--format=%(refname)"}},
		{Args: []string{"for-each-ref", "--format=%(refname)", "refs/replace"}},
		{Args: []string{"show", "-s", "--format=%cI", sha, "--"}},
		{Args: []string{"ls-tree", "-r", "-z", "--full-tree", sha, "--", "features"}},
		{Args: []string{"rev-list", "--topo-order", "--all"}},
		{Args: []string{"rev-list", "--topo-order", "--max-count=4", "--all"}},
		{Args: []string{"diff-tree", "--no-ext-diff", "--no-textconv", "-r", sha}},
		{Args: []string{"cat-file", "blob", sha}, Blob: true},
	}
	for _, request := range allowed {
		if err := validateSourceGitRequest(request); err != nil {
			t.Fatalf("accepted read-only request %v rejected: %v", request.Args, err)
		}
	}

	rejected := []SourceGitRequest{
		{Args: []string{"show", "--output=/tmp/evil", "-s", "--format=%cI", sha}},
		{Args: []string{"show", "--output", "/tmp/evil", "-s", sha}},
		{Args: []string{"diff-tree", "--output=changes.diff", sha}},
		{Args: []string{"diff-tree", "--ext-diff", sha}},
		{Args: []string{"diff-tree", "--textconv", sha}},
		{Args: []string{"rev-list", "--upload-pack=evil", "--all"}},
		{Args: []string{"rev-parse", "--exec=evil"}},
		{Args: []string{"for-each-ref", "--config-env=core.editor=evil"}},
	}
	for _, request := range rejected {
		if err := validateSourceGitRequest(request); !errors.Is(err, ErrInvalidSourceSpec) {
			t.Fatalf("write/helper request %v = %v, want ErrInvalidSourceSpec", request.Args, err)
		}
	}

	budget, err := newSourceBudget(ReadBounds{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = runSourceGit(context.Background(), t.TempDir(), budget, SourceGitRequest{Args: []string{"show", "--output=/tmp/evil", "-s"}})
	requireSentinel(t, err, ErrInvalidSourceSpec)

	t.Run("bounded-rev-list-without-topo", func(t *testing.T) {
		for _, request := range []SourceGitRequest{
			{Args: []string{"rev-list", "--max-count=4", "--format=%cI", "--all"}},
			{Args: []string{"rev-list", "--max-count=4", "--format=%cI", sha}},
			{Args: []string{"rev-list", "--parents", "--max-count=4", "--format=%cI", "--all"}},
			{Args: []string{"rev-list", "--parents", "--max-count=4", "--format=%cI", sha}},
		} {
			if err := validateSourceGitRequest(request); err != nil {
				t.Errorf("bounded rev-list form %v rejected: %v", request.Args, err)
			}
		}
	})

	t.Run("symbolic-ref-readonly", func(t *testing.T) {
		for _, request := range []SourceGitRequest{
			{Args: []string{"symbolic-ref", "--quiet", "HEAD"}},
			{Args: []string{"show-ref", "--verify", "--quiet", "refs/heads/main"}},
		} {
			if err := validateSourceGitRequest(request); err != nil {
				t.Errorf("accepted read-only form %v rejected: %v", request.Args, err)
			}
		}
		for _, request := range []SourceGitRequest{
			{Args: []string{"symbolic-ref", "HEAD", "refs/heads/main"}},
			{Args: []string{"symbolic-ref", "-d", "HEAD"}},
			{Args: []string{"symbolic-ref", "--delete", "HEAD"}},
			{Args: []string{"symbolic-ref", "--quiet", "HEAD", "refs/heads/main"}},
			{Args: []string{"symbolic-ref", "-m", "reason", "HEAD", "refs/heads/main"}},
		} {
			if err := validateSourceGitRequest(request); !errors.Is(err, ErrInvalidSourceSpec) {
				t.Errorf("write/delete symbolic-ref %v = %v, want ErrInvalidSourceSpec", request.Args, err)
			}
		}
	})
}

func testF3DetachedHeadAllRefs(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-root", "synthetic-utc-offset.jsonl")
	repo := initGitRepo(t)
	repo.writeTestdata("features/study/tasks.yaml", "yaml", "offset-equivalent.yaml")
	mainCommit := repo.commit("on-main", "features")
	runGit(t, repo.path, "checkout", "-q", "--detach", "HEAD")
	repo.writeTestdata("features/study/tasks.yaml", "yaml", "dispatcher-root.yaml")
	detached := repo.commit("detached-only", "features")
	if detached == "" || detached == mainCommit {
		t.Fatalf("detached commit %q was not distinct from main %q", detached, mainCommit)
	}
	if head := strings.TrimSpace(gitOutput(t, repo.path, "rev-parse", "HEAD")); head != detached {
		t.Fatalf("fixture HEAD = %q, want detached %s", head, detached)
	}
	if branch := strings.TrimSpace(gitOutput(t, repo.path, "rev-parse", "refs/heads/main")); branch != mainCommit {
		t.Fatalf("fixture main = %q, want %s", branch, mainCommit)
	}

	manifest, readings, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", repo.path, "", "features"),
	}, defaultSelection(), ReadBounds{})
	if manifest == nil {
		t.Fatalf("nil manifest on detached HEAD history: %v", err)
	}
	hist := panelSourceByID(t, manifest, "hist")
	headRef, haveHEAD := panelResolvedRef(hist.ResolvedRefs, "HEAD")
	if !haveHEAD || headRef.Commit != detached {
		t.Fatalf("HEAD captured commit missing: refs=%+v want %s", hist.ResolvedRefs, detached)
	}
	mainRef, haveMain := panelResolvedRef(hist.ResolvedRefs, "refs/heads/main")
	if !haveMain || mainRef.Commit != mainCommit {
		t.Fatalf("main tip missing from all-ref snapshot: refs=%+v want %s", hist.ResolvedRefs, mainCommit)
	}

	foundDetached := false
	if readings != nil {
		for _, reading := range readings.Readings {
			if reading.Ref.Revision == "git:"+detached {
				foundDetached = true
				break
			}
		}
	}
	if !foundDetached {
		if hist.State == SourceComplete {
			t.Fatalf("COMPLETE all-ref history silently omitted a detached-HEAD commit; refs=%+v", hist.ResolvedRefs)
		}
		t.Fatalf("commit reachable only from detached HEAD was not enumerated; state=%s refs=%+v err=%v", hist.State, hist.ResolvedRefs, err)
	}
}

func testF3GitBufferedExitRead(t *testing.T) {
	payload := "BUFFERED-EXIT-PAYLOAD-" + strings.Repeat("b", 200)
	state := t.TempDir()
	marker := filepath.Join(state, "wrote")
	installFixtureGitWrapper(t, fmt.Sprintf(
		"printf '%%s' %s\n: > %s",
		shellQuote(payload), shellQuote(marker),
	))
	budget, err := newSourceBudget(ReadBounds{MaxProcesses: 1, MaxTotalBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	call := startFixtureGit(ctx, t.TempDir(), budget, SourceGitRequest{Args: []string{"rev-parse", "HEAD"}})
	pending := true
	var reader io.ReadCloser
	t.Cleanup(func() {
		cancel()
		if pending {
			if completed, ok := waitFixtureGit(call, fixtureHarnessWait); ok {
				pending = false
				reader = completed.reader
			}
		}
		_ = closeSourceReader(t, reader, "buffered-exit git reader cleanup")
	})
	started, ok := waitFixtureGit(call, fixtureHarnessWait)
	if !ok {
		t.Fatal("runSourceGit did not return a reader within the harness bound")
	}
	pending = false
	if started.err != nil {
		t.Fatal(started.err)
	}
	reader = started.reader
	if reader == nil {
		t.Fatal("nil git reader")
	}
	if !fixturePathAppears(marker, fixtureHarnessWait) {
		t.Fatal("controlled child never wrote its successful payload")
	}
	waitSourceGitProcessDone(t, reader)
	// Cross the old one-second post-Wait cleanup deadline after Wait has been
	// observed. This sequences processDone then a wall delay; it does not prove
	// a concurrent reader/Wait interleaving beyond that order.
	deadline := time.NewTimer(time.Second + 400*time.Millisecond)
	<-deadline.C
	deadline.Stop()
	got, ok := waitFixtureRead(startFixtureRead(reader), fixtureHarnessWait)
	if !ok {
		t.Fatal("delayed read after successful child exit did not finish within the harness bound")
	}
	if string(got.data) != payload {
		t.Fatalf("delayed read lost buffered git output after successful exit: got %q (%d bytes) err=%v, want full %d-byte payload", got.data, len(got.data), got.err, len(payload))
	}
	if got.err != nil {
		t.Fatalf("delayed read after successful child exit terminal err=%v, want nil with the full payload", got.err)
	}
	if err := closeSourceReader(t, reader, "buffered-exit git reader"); err != nil {
		t.Errorf("close after delayed successful read = %v, want nil", err)
	}
	reader = nil
}

func testF3BoundMetadataFragment(t *testing.T) {
	sha := strings.Repeat("ab", 20)
	header := "commit " + sha + "\n"
	timestamp := "2025-12-01T00:00:00Z\n"
	payload := header + timestamp
	cases := []struct {
		name     string
		maxTotal int64
	}{
		{name: "commit-header", maxTotal: 20},
		{name: "committer-timestamp", maxTotal: int64(len(header) + 5)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installFixtureGitWrapper(t, fmt.Sprintf("printf '%%s' %s", shellQuote(payload)))
			budget, err := newSourceBudget(ReadBounds{
				MaxCommits:    3,
				MaxTotalBytes: tc.maxTotal,
				MaxBlobBytes:  1024,
				MaxProcesses:  1,
			})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, _, readErr := readHistoryCommits(ctx, t.TempDir(), []string{sha}, budget, 3)
			if !errors.Is(readErr, ErrBoundExceeded) {
				t.Fatalf("metadata cap cutting %s = %v, want ErrBoundExceeded (not reclassified Git syntax)", tc.name, readErr)
			}
		})
	}
}

func testF3NoncommitRefPeel(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-root", "synthetic-utc-offset.jsonl")
	repo := initGitRepo(t)
	repo.writeTestdata("features/study/tasks.yaml", "yaml", "offset-equivalent.yaml")
	repo.commit("seed", "features")
	blob := strings.TrimSpace(gitOutput(t, repo.path, "hash-object", "-w", "features/study/tasks.yaml"))
	if !validObjectID(blob) {
		t.Fatalf("hash-object blob id %q is not a full object ID", blob)
	}
	runGit(t, repo.path, "update-ref", "refs/odd/blob", blob)

	logPath := filepath.Join(t.TempDir(), "git-argv.log")
	installFixtureGitWrapper(t, fmt.Sprintf("printf '%%s\\n' \"$*\" >> %s\nexec \"$real_git\" \"$@\"", shellQuote(logPath)))
	manifest, _, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", repo.path, "", "features"),
	}, defaultSelection(), ReadBounds{})
	if manifest != nil && (manifest.State == SourceComplete || panelSourceByID(t, manifest, "hist").State == SourceComplete) {
		t.Fatalf("noncommit ref produced COMPLETE: err=%v hist=%+v", err, panelSourceByID(t, manifest, "hist"))
	}
	raw, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read git argv log: %v", readErr)
	}
	argv := string(raw)
	if strings.Contains(argv, "refs/odd/blob^{commit}") {
		t.Fatalf("noncommit fallback peeled mutable ref name, not captured object ID; argv log:\n%s", argv)
	}
	if !strings.Contains(argv, blob+"^{commit}") {
		t.Fatalf("noncommit fallback never peeled captured object ID %s^{commit}; argv log:\n%s", blob, argv)
	}
}

func testF3GitGraftInspectError(t *testing.T) {
	runs, repo := contractSourceTree(t)
	infoDir := filepath.Join(repo.path, ".git", "info")
	mustMkdirAllT(t, infoDir)
	grafts := filepath.Join(infoDir, "grafts")
	_ = os.Remove(grafts)
	if err := os.Symlink("grafts", grafts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(grafts); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("graft symlink loop was inspectable as missing-or-present: %v", err)
	}

	manifest, _, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", repo.path, "", "features"),
	}, defaultSelection(), ReadBounds{})
	if manifest == nil {
		t.Fatalf("nil manifest on uninspectable grafts: %v", err)
	}
	hist := panelSourceByID(t, manifest, "hist")
	if hist.State == SourceComplete || manifest.State == SourceComplete {
		t.Fatalf("graft inspection error other than NotExist produced COMPLETE: hist=%+v err=%v", hist, err)
	}
	if validateErr := manifest.ValidateComplete(); !errors.Is(validateErr, ErrSourceIncomplete) {
		t.Fatalf("uninspectable grafts ValidateComplete = %v, want ErrSourceIncomplete", validateErr)
	}
}

func writeGitHEAD(t *testing.T, repo, contents string) {
	t.Helper()
	path := filepath.Join(repo, ".git", "HEAD")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func waitSourceGitProcessDone(t *testing.T, reader io.ReadCloser) {
	t.Helper()
	gitReader, ok := reader.(*sourceGitReadCloser)
	if !ok {
		t.Fatalf("runSourceGit reader is %T, want *sourceGitReadCloser", reader)
	}
	timer := time.NewTimer(fixtureHarnessWait)
	defer timer.Stop()
	select {
	case <-gitReader.processDone:
	case <-timer.C:
		t.Fatalf("git child did not exit within %s", fixtureHarnessWait)
	}
}

func panelHistoryWithJournal(t *testing.T, repo string) (*SourceManifest, *SourceReadings, error) {
	t.Helper()
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-root", "synthetic-utc-offset.jsonl")
	return readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", repo, "", "features"),
	}, defaultSelection(), ReadBounds{})
}

func reasonNamesEntry(report SourceReport, err error, name string) bool {
	if name == "" {
		return false
	}
	if err != nil && strings.Contains(err.Error(), name) {
		return true
	}
	for _, reason := range report.Reasons {
		if strings.Contains(reason, name) {
			return true
		}
	}
	return false
}

func testF3HeadSymbolicInvalid(t *testing.T) {
	t.Run("invalid-double-dot-target", func(t *testing.T) {
		repo := initGitRepo(t)
		repo.writeTestdata("features/study/tasks.yaml", "yaml", "offset-equivalent.yaml")
		mainCommit := repo.commit("seed", "features")
		writeGitHEAD(t, repo.path, "ref: refs/heads/main..bad\n")
		manifest, _, err := panelHistoryWithJournal(t, repo.path)
		if manifest == nil {
			t.Fatalf("nil manifest on invalid symbolic HEAD: %v", err)
		}
		hist := panelSourceByID(t, manifest, "hist")
		if hist.State == SourceComplete || manifest.State == SourceComplete {
			t.Fatalf("invalid symbolic HEAD %q with valid refs produced COMPLETE: hist=%+v main=%s err=%v", "refs/heads/main..bad", hist, mainCommit, err)
		}
		if !errors.Is(err, ErrGitHistory) {
			t.Fatalf("invalid symbolic HEAD = %v (state=%s), want typed ErrGitHistory", err, hist.State)
		}
	})

	t.Run("unborn-with-existing-refs", func(t *testing.T) {
		repo := initGitRepo(t)
		repo.writeTestdata("features/study/tasks.yaml", "yaml", "offset-equivalent.yaml")
		mainCommit := repo.commit("seed", "features")
		writeGitHEAD(t, repo.path, "ref: refs/heads/does-not-exist\n")
		manifest, _, err := panelHistoryWithJournal(t, repo.path)
		if manifest == nil {
			t.Fatalf("nil manifest on missing symbolic HEAD target: %v", err)
		}
		hist := panelSourceByID(t, manifest, "hist")
		if hist.State == SourceComplete || manifest.State == SourceComplete {
			t.Fatalf("symbolic HEAD to missing refs/heads/does-not-exist with valid main %s produced COMPLETE: hist=%+v err=%v", mainCommit, hist, err)
		}
		if errors.Is(err, ErrGitHistory) {
			t.Fatalf("valid unborn symbolic HEAD with existing refs was classified as corrupt: %v", err)
		}
		if hist.State != SourcePartial || len(hist.Reasons) == 0 {
			t.Fatalf("unborn history must be PARTIAL with a reason: %+v", hist)
		}
		if got, ok := panelResolvedRef(hist.ResolvedRefs, "refs/heads/main"); !ok || got != mainCommit {
			t.Fatalf("unborn HEAD discarded valid main ref: %+v", hist.ResolvedRefs)
		}
	})

	t.Run("unborn-absence-control", func(t *testing.T) {
		repo := initGitRepo(t)
		manifest, _, err := panelHistoryWithJournal(t, repo.path)
		if manifest == nil {
			t.Fatalf("nil manifest on unborn symbolic HEAD: %v", err)
		}
		hist := panelSourceByID(t, manifest, "hist")
		if _, haveHEAD := panelResolvedRef(hist.ResolvedRefs, "HEAD"); haveHEAD {
			t.Fatalf("verified unborn symbolic HEAD was recorded as a resolved ref: %+v", hist.ResolvedRefs)
		}
		if hist.State == SourceComplete || manifest.State == SourceComplete {
			t.Fatal("unborn symbolic HEAD produced COMPLETE")
		}
		if errors.Is(err, ErrGitHistory) {
			t.Fatalf("verified unborn symbolic HEAD was treated as a typed Git history fault: %v", err)
		}
		if hist.State != SourcePartial {
			t.Fatalf("unborn symbolic HEAD state = %q, want PARTIAL absence", hist.State)
		}
	})

	t.Run("missing-detached-peel-control", func(t *testing.T) {
		repo := initGitRepo(t)
		repo.writeTestdata("features/study/tasks.yaml", "yaml", "offset-equivalent.yaml")
		mainCommit := repo.commit("seed", "features")
		missing := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		if missing == mainCommit {
			t.Fatal("missing detached fixture ID collided with main")
		}
		writeGitHEAD(t, repo.path, missing+"\n")
		manifest, _, err := panelHistoryWithJournal(t, repo.path)
		if manifest == nil {
			t.Fatalf("nil manifest on missing detached HEAD: %v", err)
		}
		hist := panelSourceByID(t, manifest, "hist")
		if hist.State == SourceComplete || manifest.State == SourceComplete {
			t.Fatalf("missing detached HEAD object produced COMPLETE: hist=%+v err=%v", hist, err)
		}
		if !errors.Is(err, ErrGitHistory) {
			t.Fatalf("missing detached HEAD peel = %v, want typed ErrGitHistory", err)
		}
	})
}

func testF3JournalSymlinkChild(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-real", "synthetic-utc-offset.jsonl")
	if err := os.Symlink("run-real", filepath.Join(runs, "latest")); err != nil {
		t.Fatal(err)
	}

	manifest, readings, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
	}, defaultSelection(), ReadBounds{})
	if manifest != nil && (manifest.State == SourceComplete || panelSourceByID(t, manifest, "j").State == SourceComplete) {
		t.Fatalf("journal root with a real run plus direct-child symlink latest reported COMPLETE: err=%v sources=%+v", err, manifest.Sources)
	}
	var journal SourceReport
	if manifest != nil {
		journal = panelSourceByID(t, manifest, "j")
	}
	if journal.State != SourcePartial {
		t.Fatalf("journal symlink child state = %q, want PARTIAL; err=%v", journal.State, err)
	}
	if !reasonNamesEntry(journal, err, "latest") {
		t.Fatalf("PARTIAL journal reasons do not name omitted symlink entry latest: reasons=%v err=%v", journal.Reasons, err)
	}
	if readings != nil {
		for _, parsed := range readings.Journals {
			if parsed.Journal.RunID == "latest" || strings.Contains(parsed.Journal.Path, "latest") {
				t.Fatalf("traversed journal symlink alias latest: %+v", parsed.Journal)
			}
		}
		for _, reading := range readings.Readings {
			if strings.Contains(reading.Ref.Path, "latest") {
				t.Fatalf("consumed journal evidence through symlink alias latest: %+v", reading.Ref)
			}
		}
	}
	if manifest != nil {
		if validateErr := manifest.ValidateComplete(); !errors.Is(validateErr, ErrSourceIncomplete) {
			t.Fatalf("symlink-omitted journal ValidateComplete = %v, want ErrSourceIncomplete", validateErr)
		}
	}
}

func testF3OpenSourceSymlinkParent(t *testing.T) {
	root := t.TempDir()
	mustMkdirAllT(t, filepath.Join(root, "real-run"))
	legitPath := filepath.Join(root, "real-run", "journal.jsonl")
	const payload = "legit-confined-journal\n"
	writeFileT(t, legitPath, payload)
	if err := os.Symlink("real-run", filepath.Join(root, "alias-run")); err != nil {
		t.Fatal(err)
	}

	budget, err := newSourceBudget(ReadBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if err := budget.setFileRoot(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(budget.closeFileRoot)

	ctx := context.Background()
	reader, err := openSourceFile(ctx, "real-run/journal.jsonl", budget, true)
	if err != nil {
		t.Fatalf("legitimate journal open: %v", err)
	}
	data, readErr := io.ReadAll(reader)
	if closeErr := closeSourceReader(t, reader, "legitimate journal"); closeErr != nil {
		t.Errorf("close legitimate journal: %v", closeErr)
	}
	if readErr != nil {
		t.Fatalf("read legitimate journal: %v", readErr)
	}
	if string(data) != payload {
		t.Fatalf("legitimate journal payload = %q, want %q", data, payload)
	}

	reader, err = openSourceFile(ctx, "alias-run/journal.jsonl", budget, true)
	if reader != nil {
		_ = closeSourceReader(t, reader, "unexpected symlink-parent reader")
	}
	if err == nil {
		t.Fatal("openSourceFile followed an in-root symlink parent")
	}
	if !errors.Is(err, ErrSourceMissing) && !errors.Is(err, ErrInvalidSourceSpec) {
		t.Fatalf("symlink-parent journal open = %v, want ErrSourceMissing or ErrInvalidSourceSpec", err)
	}

	reader, err = openSourceFile(ctx, "real-run/journal.jsonl", budget, true)
	if err != nil {
		t.Fatalf("legitimate journal reopen after refused symlink parent: %v", err)
	}
	_ = closeSourceReader(t, reader, "legitimate journal reopen")
}

func testF3GitCloseSelfCancel(t *testing.T) {
	state := t.TempDir()
	marker := filepath.Join(state, "holding-stderr")
	release := filepath.Join(state, "release")
	installFixtureGitWrapper(t, fmt.Sprintf(
		"trap '' HUP\n( : > %s\n  while [ ! -e %s ]; do sleep 1; done\n) </dev/null >&2 &\nwhile [ ! -e %s ]; do sleep 1; done\nprintf 'OK'",
		shellQuote(marker), shellQuote(release), shellQuote(marker),
	))
	budget, err := newSourceBudget(ReadBounds{MaxProcesses: 1, MaxTotalBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	call := startFixtureGit(ctx, t.TempDir(), budget, SourceGitRequest{Args: []string{"rev-parse", "HEAD"}})
	pending := true
	var reader io.ReadCloser
	t.Cleanup(func() {
		releaseFixturePath(t, release)
		cancel()
		if pending {
			if completed, ok := waitFixtureGit(call, fixtureHarnessWait); ok {
				pending = false
				reader = completed.reader
			}
		}
		_ = closeSourceReader(t, reader, "self-cancel git reader cleanup")
	})
	started, ok := waitFixtureGit(call, fixtureHarnessWait)
	if !ok {
		t.Fatal("runSourceGit did not return a reader within the harness bound")
	}
	pending = false
	if started.err != nil {
		t.Fatal(started.err)
	}
	reader = started.reader
	if reader == nil {
		t.Fatal("nil git reader")
	}
	if !fixturePathAppears(marker, fixtureHarnessWait) {
		t.Fatal("controlled child never held the inherited stderr pipe")
	}
	waitSourceGitProcessDone(t, reader)
	err = closeSourceReader(t, reader, "caller-close after successful exit")
	reader = nil
	if err != nil && errors.Is(err, ErrGitHistory) && (strings.Contains(err.Error(), "incomplete stderr") || errors.Is(err, ErrSourceCancelled)) {
		t.Fatalf("Close after successful child exit returned a fake incomplete-stderr Git fault from ioCtx cancellation: %v", err)
	}
	if err != nil {
		t.Fatalf("Close after successful child exit = %v, want nil", err)
	}
}
