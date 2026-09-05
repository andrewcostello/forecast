package dispatched

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFCSourcesContract is the reserved FC-SOURCES group.
func TestFCSourcesContract(t *testing.T) {
	t.Run("F3-SRC-EXPLICIT-ONLY", testF3SrcExplicitOnly)
	t.Run("F3-SRC-ROOT-OUTSIDE-FEATURES", testF3SrcRootOutsideFeatures)
	t.Run("F3-SRC-ROOT-ESCAPES", testF3SrcRootEscapes)
	t.Run("F3-SRC-MISSING", testF3SrcMissing)
	t.Run("F3-SRC-ZERO-JOURNALS", testF3SrcZeroJournals)
	t.Run("F3-SRC-MALFORMED-PARTIAL", testF3SrcMalformedPartial)
	t.Run("F3-READING-ENVELOPE", testF3ReadingEnvelope)
	t.Run("F3-NON-TASK-DOCUMENT", testF3NonTaskDocument)
	t.Run("F3-SRC-RESOLVED-REF", testF3SrcResolvedRef)
	t.Run("F3-GIT-ENV-STRIPPED", testF3GitEnvStripped)
	t.Run("F3-GIT-INSTALLATION-ENV", testF3GitInstallationEnv)
	t.Run("F3-GIT-SHALLOW", testF3GitShallow)
	t.Run("F3-GIT-FULL-HISTORY", testF3GitFullHistory)
	t.Run("F3-GIT-DELETED-RENAMED", testF3GitDeletedRenamed)
	t.Run("F3-BOUND-COMMITS", testF3BoundCommits)
	t.Run("F3-BOUND-BYTES", testF3BoundBytes)
	t.Run("F3-BOUND-PROCESSES", testF3BoundProcesses)
	t.Run("F3-CANCELLED", testF3Cancelled)
	t.Run("F3-HOLDOUT-EXCLUDED", testF3HoldoutExcluded)
	t.Run("F3-CUTOFF-EXCLUDED", testF3CutoffExcluded)
	t.Run("F3-SELECTION-INVALID", testF3SelectionInvalid)
	t.Run("F3-HOLDOUT-PADDED-STILL-EXCLUDES", testF3HoldoutPadded)
	t.Run("F3-HOLDOUT-UNMATCHED", testF3HoldoutUnmatched)
	t.Run("F3-MISSING-REVISION-TIME", testF3MissingRevisionTime)
	t.Run("F3-COMPLETE-CONSISTENCY", testF3CompleteConsistency)
	t.Run("F3-DEFAULT-BOUNDS", testF3DefaultBounds)
	t.Run("F3-RESOLVED-BOUNDS", testF3ResolvedBounds)
	t.Run("F3-REF-IDENTITY", testF3RefIdentity)
	t.Run("F3-REVISION-CANONICAL", testF3RevisionCanonical)
	t.Run("F3-UNSUPPORTED-REF", testF3UnsupportedRef)
	t.Run("F3-AMENDED-GIT-HELPER", testF3AmendedGitHelper)
	t.Run("F3-GIT-RUNNER", testF3GitRunner)
	t.Run("F3-HISTORY-FACTS", testF3HistoryFacts)
	t.Run("F3-EXCLUDED-QUALITY", testF3ExcludedQuality)
	t.Run("F3-MALFORMED-HELDOUT-IDENTITY", testF3MalformedHeldoutIdentity)
	t.Run("F3-ALL-JOURNALS-HELDOUT", testF3AllJournalsHeldout)
	t.Run("F3-SOURCE-CONCURRENCY", testF3SourceConcurrency)
	t.Run("F3-DUPLICATE-JOURNAL-RUN", testF3DuplicateJournalRun)
	t.Run("F3-COMPLETENESS-CAUSES", testF3CompletenessCauses)
	t.Run("F3-EXCLUSION-ORDER", testF3ExclusionOrder)
	t.Run("F4-MANIFEST-EMPTY-LISTS", testF4ManifestEmptyLists)
	t.Run("F3-CITATION-ROW", testF3CitationRow)
	t.Run("F3-OPEN-SOURCE-NIL-BUDGET", testF3OpenSourceNilBudget)
}

func contractSourceTree(t *testing.T) (runs string, repo gitRepo) {
	t.Helper()
	runs = filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-root", "synthetic-utc-offset.jsonl")
	repo = initGitRepo(t)
	repo.writeTestdata("features/study/tasks.yaml", "yaml", "offset-equivalent.yaml")
	repo.writeTestdata("dispatcher/tasks.yaml", "yaml", "dispatcher-root.yaml")
	repo.writeTestdata("unrelated/secret.yaml", "yaml", "unrelated-secret.yaml")
	repo.commit("seed", "features", "dispatcher", "unrelated")
	return runs, repo
}

func testF3SrcExplicitOnly(t *testing.T) {
	if err := (SourceSpec{}).Validate(); !errors.Is(err, ErrInvalidSourceSpec) {
		t.Fatalf("empty spec = %v, want ErrInvalidSourceSpec", err)
	}
	_, _, err := ReadSources(context.Background(), nil, defaultSelection(), ReadBounds{})
	requireSentinel(t, err, ErrInvalidSourceSpec)
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustMkdirAllT(t, filepath.Join(home, "Project", "claude-workflow", "features"))
	_, _, err = ReadSources(context.Background(), []SourceSpec{}, defaultSelection(), ReadBounds{})
	requireSentinel(t, err, ErrInvalidSourceSpec)
	if err != nil && strings.Contains(err.Error(), home) {
		t.Fatalf("empty sources scanned HOME: %v", err)
	}
}

func testF3SrcRootOutsideFeatures(t *testing.T) {
	runs, repo := contractSourceTree(t)
	manifest, readings := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live", repo.path, "dispatcher"),
	}, defaultSelection(), ReadBounds{})
	if containPath(readings.Readings, "unrelated") {
		t.Fatalf("sibling unrelated/ was scanned: %+v", readings.Readings)
	}
	if !containPath(readings.Readings, "dispatcher") {
		t.Fatalf("dispatcher/ root not scanned: %+v", readings.Readings)
	}
	if containPath(readings.Readings, "features/study") {
		t.Fatal("undeclared features/ root was scanned")
	}
	_ = manifest

	// Two explicitly declared roots must both be scanned; the undeclared
	// sibling must not. Live and history each declare dispatcher + features.
	_, multi := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live-multi", repo.path, "dispatcher", "features"),
		historySpec("hist-multi", repo.path, "", "dispatcher", "features"),
	}, defaultSelection(), ReadBounds{})
	if containPath(multi.Readings, "unrelated") {
		t.Fatalf("undeclared unrelated/ sibling was scanned: %+v", multi.Readings)
	}
	var liveDispatcher, liveFeatures, histDispatcher, histFeatures bool
	for _, r := range multi.Readings {
		if r.Identity.Key.Value == "SHOULD-NOT-BE-READ" {
			t.Fatalf("undeclared sibling row leaked: %+v", r)
		}
		switch {
		case r.Ref.SourceID == "live-multi" && strings.Contains(r.Ref.Path, "dispatcher"):
			liveDispatcher = true
		case r.Ref.SourceID == "live-multi" && strings.Contains(r.Ref.Path, "features/study"):
			liveFeatures = true
		case r.Ref.SourceID == "hist-multi" && strings.Contains(r.Ref.Path, "dispatcher"):
			histDispatcher = true
		case r.Ref.SourceID == "hist-multi" && strings.Contains(r.Ref.Path, "features/study"):
			histFeatures = true
		}
	}
	if !liveDispatcher || !liveFeatures {
		t.Fatalf("live two-root scan incomplete: dispatcher=%v features=%v readings=%+v", liveDispatcher, liveFeatures, multi.Readings)
	}
	if !histDispatcher || !histFeatures {
		t.Fatalf("history two-root scan incomplete: dispatcher=%v features=%v readings=%+v", histDispatcher, histFeatures, multi.Readings)
	}
}

func testF3SrcRootEscapes(t *testing.T) {
	cases := []SourceSpec{
		liveSpec("rel", t.TempDir(), "../x"),
		{ID: "abs", Kind: SourceKindLiveYAML, Repository: t.TempDir(), Roots: []string{"/tmp/x"}},
	}
	for _, spec := range cases {
		if err := spec.Validate(); !errors.Is(err, ErrInvalidSourceSpec) {
			t.Fatalf("Validate(%+v) = %v, want ErrInvalidSourceSpec", spec, err)
		}
	}
	repo := initGitRepo(t)
	outside := t.TempDir()
	writeFileT(t, filepath.Join(outside, "escaped.yaml"), testdataFile(t, "yaml", "valid-tasks.yaml"))
	link := filepath.Join(repo.path, "features", "link.yaml")
	mustMkdirAllT(t, filepath.Dir(link))
	if err := os.Symlink(filepath.Join(outside, "escaped.yaml"), link); err != nil {
		t.Fatal(err)
	}
	repo.commit("symlink", "features")
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-root", "synthetic-utc-offset.jsonl")
	_, readings, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live", repo.path, "features"),
	}, defaultSelection(), ReadBounds{})
	if err != nil && !errors.Is(err, ErrSourceMissing) && !errors.Is(err, ErrInvalidSourceSpec) && !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("symlink escape = %v", err)
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if readings != nil && containPath(readings.Readings, "escaped") {
		t.Fatal("symlink escaped the declared root")
	}
}

func testF3SrcMissing(t *testing.T) {
	_, _, err := ReadSources(context.Background(), []SourceSpec{
		journalSpec("j", filepath.Join(t.TempDir(), "missing-runs")),
	}, defaultSelection(), ReadBounds{})
	requireSentinel(t, err, ErrSourceMissing)
}

func testF3SrcZeroJournals(t *testing.T) {
	empty := t.TempDir()
	_, _, err := ReadSources(context.Background(), []SourceSpec{journalSpec("j", empty)}, defaultSelection(), ReadBounds{})
	requireSentinel(t, err, ErrSourceEmpty)

	manifest, _, err := ReadSources(context.Background(), []SourceSpec{journalSpec("j", empty)}, Selection{
		Cutoff: contractCutoff(), AllowEmpty: true,
	}, ReadBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.State != SourceEmpty {
		t.Fatalf("allow-empty state = %q, want EMPTY", manifest.State)
	}
	if err := manifest.ValidateComplete(); !errors.Is(err, ErrSourceIncomplete) {
		t.Fatalf("EMPTY manifest eligible: %v", err)
	}
}

func testF3SrcMalformedPartial(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-a", "synthetic-utc-offset.jsonl")
	repo := initGitRepo(t)
	repo.writeTestdata("features/study/tasks.yaml", "yaml", "malformed-sibling.yaml")
	repo.commit("malformed", "features")
	manifest, readings, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live", repo.path, "features"),
	}, defaultSelection(), ReadBounds{})
	if err != nil && !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("malformed sibling aborted the read: %v", err)
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if manifest.State != SourcePartial {
		t.Fatalf("state = %q, want PARTIAL", manifest.State)
	}
	var valid, malformed int
	for _, r := range readings.Readings {
		if r.Kind == DocumentTaskRow && r.Err == nil && r.Identity.Key.Value == "VALID" {
			valid++
		}
		if r.Err != nil {
			malformed++
		}
	}
	if valid != 1 || malformed == 0 {
		t.Fatalf("valid=%d malformed=%d readings=%+v", valid, malformed, readings.Readings)
	}
	if manifest.Sources[1].Counts.Malformed < 1 {
		t.Fatalf("Malformed counter = %+v", manifest.Sources[1].Counts)
	}
}

func testF3ReadingEnvelope(t *testing.T) {
	ref := ReadingRef{SourceID: "live", Path: "features/study/tasks.yaml", Revision: "live", RecordedAt: contractCutoff()}
	got, err := parseReadings([]byte(testdataFile(t, "yaml", "malformed-sibling.yaml")), ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 4 {
		t.Fatalf("envelopes = %d, want independent rows", len(got))
	}
	byKey := map[string]Reading{}
	for _, r := range got {
		if r.Identity.Key.Known {
			byKey[r.Identity.Key.Value] = r
		}
		if r.Kind == DocumentTaskRow && r.Ref.Row < 1 {
			t.Fatalf("task row Ref.Row = %d", r.Ref.Row)
		}
	}
	if byKey["VALID"].Err != nil {
		t.Fatalf("valid sibling erased: %+v", byKey["VALID"])
	}
	if byKey["BADTS"].Err == nil || byKey["BADTS"].Present.StartedAt == false {
		t.Fatalf("invalid timestamp envelope = %+v", byKey["BADTS"])
	}
	if byKey["MISSINGSTART"].Present.StartedAt {
		t.Fatalf("missing start marked present: %+v", byKey["MISSINGSTART"])
	}
	if byKey["TYPED"].Err == nil {
		t.Fatalf("typed YAML mismatch retained as valid: %+v", byKey["TYPED"])
	}
}

func testF3NonTaskDocument(t *testing.T) {
	ref := ReadingRef{SourceID: "live", Path: "config/known-red.yaml", Revision: "live", RecordedAt: contractCutoff()}
	got, err := parseReadings([]byte(testdataFile(t, "yaml", "non-task-known-red.yaml")), ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != DocumentNotTasks || got[0].Err != nil || got[0].Ref.Row != 0 {
		t.Fatalf("non-task envelope = %+v", got)
	}

	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-root", "synthetic-utc-offset.jsonl")
	repo := initGitRepo(t)
	repo.writeTestdata("features/study/tasks.yaml", "yaml", "offset-equivalent.yaml")
	repo.writeTestdata("features/study/known-red.yaml", "yaml", "non-task-known-red.yaml")
	repo.commit("mixed", "features")
	manifest, readings := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live", repo.path, "features"),
	}, defaultSelection(), ReadBounds{})
	var nonTask int
	for _, r := range readings.Readings {
		if r.Kind == DocumentNotTasks {
			nonTask++
		}
	}
	if nonTask != 1 {
		t.Fatalf("NonTaskDocuments envelopes = %d", nonTask)
	}
	for _, src := range manifest.Sources {
		if src.Kind == SourceKindLiveYAML && src.Counts.NonTaskDocuments != 1 {
			t.Fatalf("NonTaskDocuments counter = %+v", src.Counts)
		}
		if src.Kind == SourceKindLiveYAML && src.State == SourcePartial && src.Counts.Malformed == 0 {
			t.Fatal("non-task document marked the source PARTIAL")
		}
	}
}

func testF3SrcResolvedRef(t *testing.T) {
	runs, repo := contractSourceTree(t)
	head := strings.TrimSpace(gitOutput(t, repo.path, "rev-parse", "HEAD"))
	manifest, _ := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", repo.path, "HEAD", "features"),
	}, defaultSelection(), ReadBounds{})
	var hist SourceReport
	for _, src := range manifest.Sources {
		if src.ID == "hist" {
			hist = src
		}
	}
	if hist.ResolvedRef != head {
		t.Fatalf("ResolvedRef = %q, want %s", hist.ResolvedRef, head)
	}
	if len(hist.ResolvedRefs) != 1 || hist.ResolvedRefs[0].Commit != head {
		t.Fatalf("ResolvedRefs = %+v", hist.ResolvedRefs)
	}
}

func testF3GitEnvStripped(t *testing.T) {
	poison := t.TempDir()
	t.Setenv("GIT_DIR", poison)
	t.Setenv("GIT_WORK_TREE", poison)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(poison, "config"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(poison, "system"))
	t.Setenv("GIT_CONFIG", filepath.Join(poison, "cfg"))
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.worktree")
	t.Setenv("GIT_CONFIG_VALUE_0", poison)
	t.Setenv("GIT_OBJECT_DIRECTORY", poison)
	t.Setenv("GIT_ALTERNATE_OBJECT_DIRECTORIES", poison)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(poison, "index"))
	t.Setenv("GIT_NAMESPACE", "evil")
	cmd, err := sourceGitCommand(context.Background(), t.TempDir(), "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	env := envMap(cmd.Env)
	for _, key := range []string{
		"GIT_DIR", "GIT_WORK_TREE", "GIT_OBJECT_DIRECTORY", "GIT_INDEX_FILE",
		"GIT_NAMESPACE", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
		"GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0", "GIT_CONFIG",
	} {
		if _, ok := env[key]; ok {
			t.Fatalf("inherited %s survived: %q", key, env[key])
		}
	}
	if env["GIT_CONFIG_GLOBAL"] != "/dev/null" || env["GIT_CONFIG_SYSTEM"] != "/dev/null" {
		t.Fatalf("config paths = global %q system %q", env["GIT_CONFIG_GLOBAL"], env["GIT_CONFIG_SYSTEM"])
	}
	if env["GIT_CONFIG_NOSYSTEM"] != "1" || env["GIT_LITERAL_PATHSPECS"] != "1" {
		t.Fatalf("affirmative isolation missing: %+v", env)
	}
	if env["GIT_SSH_COMMAND"] != "/bin/false" || env["GIT_ASKPASS"] != "/bin/false" || env["GIT_PROXY_COMMAND"] != "/bin/false" {
		t.Fatalf("helpers not pinned false: %+v", env)
	}
	joined := strings.Join(cmd.Args, "\x00")
	for _, flag := range []string{"core.fsmonitor=false", "credential.helper=", "protocol.allow=never", "protocol.file.allow=never"} {
		if !strings.Contains(joined, flag) {
			t.Fatalf("missing -c %s in %v", flag, cmd.Args)
		}
	}
}

func testF3GitInstallationEnv(t *testing.T) {
	t.Setenv("GIT_EXEC_PATH", "/trusted/libexec")
	t.Setenv("GIT_DIR", "/evil")
	cmd, err := sourceGitCommand(context.Background(), t.TempDir(), "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	env := envMap(cmd.Env)
	if env["GIT_EXEC_PATH"] != "/trusted/libexec" {
		t.Fatalf("GIT_EXEC_PATH not preserved: %q", env["GIT_EXEC_PATH"])
	}
	if _, ok := env["GIT_DIR"]; ok {
		t.Fatal("GIT_DIR survived beside preserved GIT_EXEC_PATH")
	}
	if env["PATH"] == "" {
		t.Fatal("PATH dropped")
	}
}

func testF3GitShallow(t *testing.T) {
	runs, repo := contractSourceTree(t)
	repo.write("features/study/other.txt", "1")
	repo.commit("second", "features")
	shallow := shallowClone(t, repo.path)
	manifest, _, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", shallow, "", "features"),
	}, defaultSelection(), ReadBounds{})
	if err != nil {
		t.Fatal(err)
	}
	var hist SourceReport
	for _, src := range manifest.Sources {
		if src.ID == "hist" {
			hist = src
		}
	}
	if !hist.Shallow || hist.State != SourcePartial {
		t.Fatalf("shallow report = %+v", hist)
	}
	if err := manifest.ValidateComplete(); !errors.Is(err, ErrShallowHistory) || !errors.Is(err, ErrSourceIncomplete) {
		t.Fatalf("ValidateComplete = %v, want ErrShallowHistory+ErrSourceIncomplete", err)
	}
}

func testF3GitFullHistory(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-root", "synthetic-utc-offset.jsonl")
	repo := initGitRepo(t)
	repo.write("features/study/tasks.yaml", testdataFile(t, "yaml", "offset-equivalent.yaml"))
	repo.commit("main-a", "features")
	runGit(t, repo.path, "checkout", "-q", "-b", "side")
	repo.write("features/study/tasks.yaml", testdataFile(t, "yaml", "dispatcher-root.yaml"))
	side := repo.commit("side-b", "features")
	runGit(t, repo.path, "checkout", "-q", "-")
	runGit(t, repo.path, "merge", "-q", "--no-ff", "-m", "merge-side", "side")
	runGit(t, repo.path, "branch", "-D", "side")
	manifest, readings := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", repo.path, "", "features"),
	}, defaultSelection(), ReadBounds{})
	_ = manifest
	foundSide := false
	for _, r := range readings.Readings {
		if r.Identity.Key.Value == "F3-DISPATCHER" {
			foundSide = true
		}
		if strings.Contains(r.Ref.Revision, side[:8]) || r.Ref.Revision == "git:"+side {
			foundSide = true
		}
	}
	if !foundSide {
		t.Fatalf("superseded merge parent blob not enumerated; readings=%d head-only?", len(readings.Readings))
	}
}

func testF3GitDeletedRenamed(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-root", "synthetic-utc-offset.jsonl")
	repo := initGitRepo(t)
	repo.writeTestdata("features/study/tasks.yaml", "yaml", "offset-equivalent.yaml")
	repo.commit("add", "features")
	runGit(t, repo.path, "mv", "features/study/tasks.yaml", "features/study/renamed.yaml")
	repo.commit("rename", "features")
	runGit(t, repo.path, "rm", "-q", "features/study/renamed.yaml")
	runGit(t, repo.path, "commit", "-q", "-m", "delete")
	_, readings := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", repo.path, "", "features"),
	}, defaultSelection(), ReadBounds{})
	if !containPath(readings.Readings, "tasks.yaml") && !containPath(readings.Readings, "renamed.yaml") {
		t.Fatalf("deleted/renamed content lost: %+v", readings.Readings)
	}
}

func testF3BoundCommits(t *testing.T) {
	runs, repo := contractSourceTree(t)
	for i := 0; i < 4; i++ {
		repo.write("features/study/n.txt", strings.Repeat("x", i+1))
		repo.commit("c", "features")
	}
	manifest, _, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", repo.path, "", "features"),
	}, defaultSelection(), ReadBounds{MaxCommits: 3})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.State != SourcePartial {
		t.Fatalf("state = %q", manifest.State)
	}
	var hist SourceReport
	for _, src := range manifest.Sources {
		if src.ID == "hist" {
			hist = src
		}
	}
	if hist.Counts.Commits > 3 || hist.Counts.BoundsExceeded < 1 {
		t.Fatalf("commit cap not applied before collection: %+v", hist.Counts)
	}
	if err := manifest.ValidateComplete(); !errors.Is(err, ErrBoundExceeded) {
		t.Fatalf("ValidateComplete = %v, want ErrBoundExceeded", err)
	}
}

func testF3BoundBytes(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-root", "synthetic-utc-offset.jsonl")
	repo := initGitRepo(t)
	repo.write("features/study/tasks.yaml", testdataFile(t, "yaml", "offset-equivalent.yaml")+strings.Repeat("x", 4096))
	repo.commit("big", "features")
	manifest, _, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live", repo.path, "features"),
	}, defaultSelection(), ReadBounds{MaxBlobBytes: 64})
	if err != nil && !errors.Is(err, ErrBoundExceeded) && !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("byte bound = %v", err)
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if manifest == nil {
		t.Fatal("nil manifest on byte cap")
	}
	if manifest.State != SourcePartial {
		t.Fatalf("state = %q", manifest.State)
	}
}

func testF3BoundProcesses(t *testing.T) {
	if err := (ReadBounds{MaxProcesses: -1}).Validate(); !errors.Is(err, ErrInvalidSourceSpec) {
		t.Fatalf("negative MaxProcesses = %v", err)
	}
	runs, repo := contractSourceTree(t)
	manifest, _, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", repo.path, "", "features"),
	}, defaultSelection(), ReadBounds{MaxProcesses: 1})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.State != SourceComplete {
		t.Fatalf("process serializer changed completeness: %s", manifest.State)
	}
	for _, src := range manifest.Sources {
		if src.Counts.BoundsExceeded != 0 {
			t.Fatalf("MaxProcesses counted as BoundsExceeded: %+v", src.Counts)
		}
	}
}

func testF3Cancelled(t *testing.T) {
	runs, repo := contractSourceTree(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	manifest, _, err := ReadSources(ctx, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live", repo.path, "features"),
	}, defaultSelection(), ReadBounds{})
	if !errors.Is(err, ErrSourceCancelled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	if manifest == nil || !func() bool {
		for _, src := range manifest.Sources {
			if src.Cancelled {
				return true
			}
		}
		return manifest.State != SourceComplete
	}() {
		t.Fatalf("cancelled read reported complete: %+v", manifest)
	}
	if manifest != nil && manifest.State == SourceComplete {
		t.Fatal("COMPLETE after cancellation")
	}
}

func testF3HoldoutExcluded(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "held", "synthetic-utc-offset.jsonl")
	writeJournalTree(t, runs, "keep", "synthetic-blocked.jsonl")
	repo := initGitRepo(t)
	repo.writeTestdata("features/study/tasks.yaml", "yaml", "holdout-held-and-keep.yaml")
	repo.commit("live-and-history", "features")
	sel := Selection{Cutoff: contractCutoff(), HoldoutRunIDs: []string{"held"}}
	manifest, readings := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live", repo.path, "features"),
		historySpec("hist", repo.path, "", "features"),
	}, sel, ReadBounds{})
	for _, j := range readings.Journals {
		if j.Journal.RunID == "held" {
			t.Fatal("held-out journal payload entered Journals")
		}
	}
	found := false
	for _, id := range readings.ExcludedJournals {
		if id.RunID == "held" {
			found = true
			if len(id.Path) == 0 {
				t.Fatal("excluded journal identity dropped path")
			}
		}
	}
	if !found {
		t.Fatalf("held-out identity missing: %+v", readings.ExcludedJournals)
	}
	var liveHeld, histHeld, liveKeep, histKeep bool
	for _, r := range readings.Readings {
		if !r.Identity.RunID.Known {
			continue
		}
		switch r.Identity.RunID.Value {
		case "held":
			if r.Excluded != DispositionHeldOut {
				t.Fatalf("held YAML not marked HeldOut: %+v", r)
			}
			if r.Snapshot.AuthoredModel != "" {
				t.Fatal("held-out snapshot still carries predictive fields")
			}
			switch r.Ref.SourceID {
			case "live":
				liveHeld = true
			case "hist":
				histHeld = true
				if !strings.HasPrefix(r.Ref.Revision, "git:") {
					t.Fatalf("history held-out row revision = %q, want git:", r.Ref.Revision)
				}
			}
		case "keep":
			if r.Excluded == DispositionHeldOut {
				t.Fatalf("in-sample keep YAML marked HeldOut: %+v", r)
			}
			switch r.Ref.SourceID {
			case "live":
				liveKeep = true
			case "hist":
				histKeep = true
			}
		}
	}
	if !liveHeld {
		t.Fatal("held-out YAML missing from live join")
	}
	if !histHeld {
		t.Fatal("held-out YAML missing from git-history join")
	}
	if !liveKeep || !histKeep {
		t.Fatalf("in-sample keep YAML dropped: live=%v hist=%v", liveKeep, histKeep)
	}
	_ = manifest
}

func testF3CutoffExcluded(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-root", "synthetic-utc-offset.jsonl")
	repo := initGitRepo(t)
	repo.write("features/study/tasks.yaml", `tasks:
  - key: LATE
    role: bodies
    model: stamp
    status: Done
    started_at: '2026-09-06T00:00:00Z'
    completed_at: '2026-09-06T00:10:00Z'
    dispatcher_run_id: run-root
`)
	repo.commit("late", "features")
	_, readings := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live", repo.path, "features"),
	}, defaultSelection(), ReadBounds{})
	found := false
	for _, r := range readings.Readings {
		if r.Identity.Key.Value == "LATE" {
			found = true
			if r.Excluded != DispositionAfterCutoff {
				t.Fatalf("late envelope excluded=%q", r.Excluded)
			}
			if r.Snapshot.AuthoredModel != "" {
				t.Fatal("after-cutoff snapshot not cleared")
			}
		}
	}
	if !found {
		t.Fatal("late envelope dropped instead of audited")
	}
}

func testF3SelectionInvalid(t *testing.T) {
	if err := (Selection{HoldoutRunIDs: []string{" "}}).Validate(); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("blank holdout = %v", err)
	}
	if err := (Selection{Cutoff: contractCutoff(), HoldoutRunIDs: []string{"a", "a"}}).Validate(); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("duplicate holdout = %v", err)
	}
	if err := (Selection{}).Validate(); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("zero cutoff = %v", err)
	}
	_, _, err := ReadSources(context.Background(), []SourceSpec{journalSpec("j", t.TempDir())}, Selection{
		Cutoff: contractCutoff(), HoldoutRunIDs: []string{" padded "},
	}, ReadBounds{})
	requireSentinel(t, err, ErrInvalidSelection)
}

func testF3HoldoutPadded(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "held", "synthetic-utc-offset.jsonl")
	_, _, err := ReadSources(context.Background(), []SourceSpec{journalSpec("j", runs)}, Selection{
		Cutoff: contractCutoff(), HoldoutRunIDs: []string{" held "},
	}, ReadBounds{})
	requireSentinel(t, err, ErrInvalidSelection)
}

func testF3HoldoutUnmatched(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "keep", "synthetic-utc-offset.jsonl")
	_, _, err := ReadSources(context.Background(), []SourceSpec{journalSpec("j", runs)}, Selection{
		Cutoff: contractCutoff(), HoldoutRunIDs: []string{"no-such-run"},
	}, ReadBounds{})
	requireSentinel(t, err, ErrInvalidSelection)
}

func testF3MissingRevisionTime(t *testing.T) {
	ref := ReadingRef{SourceID: "live", Path: "features/study/tasks.yaml", Revision: "live"}
	got, err := parseReadings([]byte(testdataFile(t, "yaml", "offset-equivalent.yaml")), ref)
	if err != nil {
		t.Fatal(err)
	}
	// parseReadings copies the citation; ReadSources must set Err when RecordedAt is zero.
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-offset", "synthetic-utc-offset.jsonl")
	repo := initGitRepo(t)
	repo.writeTestdata("features/study/tasks.yaml", "yaml", "offset-equivalent.yaml")
	repo.commit("t", "features")
	manifest, readings := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live", repo.path, "features"),
	}, defaultSelection(), ReadBounds{})
	_ = got
	for _, r := range readings.Readings {
		if r.Kind == DocumentTaskRow && r.Ref.RecordedAt.IsZero() && r.Err == nil && r.Excluded == "" {
			t.Fatal("in-sample row with zero RecordedAt not malformed")
		}
	}
	_ = manifest
}

func testF3CompleteConsistency(t *testing.T) {
	var nilManifest *SourceManifest
	err := nilManifest.ValidateComplete()
	requireSentinel(t, err, ErrSourceIncomplete)

	m := completeManifest(SourceComplete)
	m.Sources[0].Shallow = true
	if err := m.ValidateComplete(); !errors.Is(err, ErrSourceIncomplete) || !errors.Is(err, ErrShallowHistory) {
		t.Fatalf("COMPLETE+shallow = %v", err)
	}
	m = completeManifest(SourceComplete)
	m.Sources[0].Grafted = true
	if err := m.ValidateComplete(); !errors.Is(err, ErrSourceIncomplete) || !errors.Is(err, ErrShallowHistory) {
		t.Fatalf("COMPLETE+grafted = %v", err)
	}
	m = completeManifest(SourceComplete)
	m.Sources[0].Replaced = true
	if err := m.ValidateComplete(); !errors.Is(err, ErrSourceIncomplete) || !errors.Is(err, ErrShallowHistory) {
		t.Fatalf("COMPLETE+replaced = %v", err)
	}
	m = completeManifest(SourceComplete)
	m.Cutoff = time.Time{}
	requireSentinel(t, m.ValidateComplete(), ErrSourceIncomplete)
	m = completeManifest(SourceComplete)
	m.Bounds.MaxCommits = 0
	requireSentinel(t, m.ValidateComplete(), ErrSourceIncomplete)
}

func testF3DefaultBounds(t *testing.T) {
	if err := (ReadBounds{MaxCommits: -1}).Validate(); !errors.Is(err, ErrInvalidSourceSpec) {
		t.Fatalf("negative bound = %v", err)
	}
	runs, repo := contractSourceTree(t)
	manifest, _ := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live", repo.path, "features"),
	}, defaultSelection(), ReadBounds{})
	if manifest.Bounds.MaxCommits != DefaultMaxCommits || manifest.Bounds.MaxLineBytes != DefaultMaxLineBytes {
		t.Fatalf("defaults not stored: %+v", manifest.Bounds)
	}
	if manifest.Bounds.MaxProcesses != DefaultMaxProcesses || manifest.Bounds.MaxBlobBytes != DefaultMaxBlobBytes {
		t.Fatalf("process/blob defaults = %+v", manifest.Bounds)
	}
}

func testF3ResolvedBounds(t *testing.T) {
	runs, repo := contractSourceTree(t)
	manifest, _ := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live", repo.path, "features"),
	}, defaultSelection(), ReadBounds{})
	if manifest.Bounds.MaxCommits == 0 || manifest.Bounds.MaxTotalBytes == 0 {
		t.Fatalf("unresolved zero bounds stored: %+v", manifest.Bounds)
	}
}

func testF3RefIdentity(t *testing.T) {
	runs, repo := contractSourceTree(t)
	manifest, _ := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("all", repo.path, "", "features"),
	}, defaultSelection(), ReadBounds{})
	var all SourceReport
	for _, src := range manifest.Sources {
		if src.ID == "all" {
			all = src
		}
	}
	if all.ResolvedRef != "" {
		t.Fatalf("all-refs ResolvedRef = %q, want empty", all.ResolvedRef)
	}
	if len(all.ResolvedRefs) == 0 {
		t.Fatal("all-refs ResolvedRefs empty")
	}
}

func testF3RevisionCanonical(t *testing.T) {
	sha40 := strings.Repeat("ab", 20)
	sha64 := strings.Repeat("ab", 32)
	if err := ValidateReadingRevision("live"); err != nil {
		t.Fatalf("live: %v", err)
	}
	if err := ValidateReadingRevision("git:" + sha40); err != nil {
		t.Fatalf("git 40: %v", err)
	}
	if err := ValidateReadingRevision("git:" + sha64); err != nil {
		t.Fatalf("git 64: %v", err)
	}
	for _, bad := range []string{"git:abc", "abc", "live:mtime", "git:" + strings.ToUpper(sha40)} {
		if err := ValidateReadingRevision(bad); !errors.Is(err, ErrUnparseableRevision) {
			t.Fatalf("%q = %v, want ErrUnparseableRevision", bad, err)
		}
	}
}

func testF3UnsupportedRef(t *testing.T) {
	spec := liveSpec("live", t.TempDir(), "features")
	spec.Ref = "main"
	if err := spec.Validate(); !errors.Is(err, ErrInvalidSourceSpec) {
		t.Fatalf("live Ref = %v", err)
	}
	j := journalSpec("j", t.TempDir())
	j.Ref = "HEAD"
	if err := j.Validate(); !errors.Is(err, ErrInvalidSourceSpec) {
		t.Fatalf("journal Ref = %v", err)
	}
}

func testF3AmendedGitHelper(t *testing.T) {
	runs, repo := contractSourceTree(t)
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "not-the-repo"))
	_, _, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", repo.path, "HEAD", "features"),
	}, defaultSelection(), ReadBounds{})
	if err != nil {
		t.Fatal(err)
	}
}

func testF3GitRunner(t *testing.T) {
	_, err := runSourceGit(context.Background(), t.TempDir(), nil, SourceGitRequest{Args: []string{"rev-parse", "HEAD"}})
	requireSentinel(t, err, ErrInvalidSourceSpec)

	budget, err := newSourceBudget(ReadBounds{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = runSourceGit(context.Background(), t.TempDir(), budget, SourceGitRequest{Args: []string{"status"}})
	requireSentinel(t, err, ErrInvalidSourceSpec)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = runSourceGit(ctx, t.TempDir(), budget, SourceGitRequest{Args: []string{"rev-parse", "HEAD"}})
	if !errors.Is(err, ErrSourceCancelled) && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("cancelled runner = %v", err)
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
}

func testF3HistoryFacts(t *testing.T) {
	runs, repo := contractSourceTree(t)
	grafted := filepath.Join(repo.path, ".git", "info")
	mustMkdirAllT(t, grafted)
	writeFileT(t, filepath.Join(grafted, "grafts"), "deadbeef parent\n")
	manifest, _, err := readContractSources(t, []SourceSpec{
		journalSpec("j", runs),
		historySpec("hist", repo.path, "", "features"),
	}, defaultSelection(), ReadBounds{})
	if err != nil {
		t.Fatal(err)
	}
	var hist SourceReport
	for _, src := range manifest.Sources {
		if src.ID == "hist" {
			hist = src
		}
	}
	if !hist.Grafted {
		t.Fatalf("grafted flag not set: %+v", hist)
	}
	if hist.Replaced {
		t.Fatalf("grafted-only repo reported Replaced: %+v", hist)
	}
	if err := manifest.ValidateComplete(); !errors.Is(err, ErrShallowHistory) || !errors.Is(err, ErrSourceIncomplete) {
		t.Fatalf("grafted ValidateComplete = %v, want ErrShallowHistory+ErrSourceIncomplete", err)
	}

	replacedRepo := initGitRepo(t)
	replacedRepo.writeTestdata("features/study/tasks.yaml", "yaml", "offset-equivalent.yaml")
	original := replacedRepo.commit("original", "features")
	replacedRepo.writeTestdata("features/study/tasks.yaml", "yaml", "dispatcher-root.yaml")
	replacement := replacedRepo.commit("replacement", "features")
	runGit(t, replacedRepo.path, "replace", original, replacement)
	replaceListed := gitOutput(t, replacedRepo.path, "for-each-ref", "--format=%(refname)", "refs/replace")
	if !strings.Contains(replaceListed, "refs/replace/") {
		t.Fatalf("replace ref was not created: %q", replaceListed)
	}
	replaceRuns := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, replaceRuns, "run-root", "synthetic-utc-offset.jsonl")
	replacedManifest, _, err := readContractSources(t, []SourceSpec{
		journalSpec("j", replaceRuns),
		historySpec("hist", replacedRepo.path, "", "features"),
	}, defaultSelection(), ReadBounds{})
	if err != nil {
		t.Fatal(err)
	}
	var replaced SourceReport
	for _, src := range replacedManifest.Sources {
		if src.ID == "hist" {
			replaced = src
		}
	}
	if !replaced.Replaced {
		t.Fatalf("replaced flag not set: %+v", replaced)
	}
	if replaced.Grafted {
		t.Fatalf("replaced-only repo reported Grafted: %+v", replaced)
	}
	if replaced.State != SourcePartial {
		t.Fatalf("replaced state = %q, want PARTIAL", replaced.State)
	}
	if err := replacedManifest.ValidateComplete(); !errors.Is(err, ErrShallowHistory) || !errors.Is(err, ErrSourceIncomplete) {
		t.Fatalf("replaced ValidateComplete = %v, want ErrShallowHistory+ErrSourceIncomplete", err)
	}
}

func testF3ExcludedQuality(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "held", "synthetic-utc-offset.jsonl")
	repo := initGitRepo(t)
	repo.write("features/study/tasks.yaml", testdataFile(t, "yaml", "malformed-sibling.yaml"))
	repo.commit("x", "features")
	// rewrite dispatcher_run_id to held so the malformed file is holdout-excluded
	repo.write("features/study/tasks.yaml", strings.ReplaceAll(testdataFile(t, "yaml", "malformed-sibling.yaml"), "run-a", "held"))
	repo.commit("held", "features")
	manifest, _ := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live", repo.path, "features"),
	}, Selection{Cutoff: contractCutoff(), HoldoutRunIDs: []string{"held"}}, ReadBounds{})
	for _, src := range manifest.Sources {
		if src.Kind == SourceKindLiveYAML {
			if src.Counts.MalformedExcluded == 0 && src.Counts.Malformed > 0 {
				t.Fatalf("held-out malformed counted in-sample: %+v", src.Counts)
			}
		}
	}
}

func testF3MalformedHeldoutIdentity(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "held", "synthetic-utc-offset.jsonl")
	repo := initGitRepo(t)
	repo.write("features/study/tasks.yaml", `tasks:
  - key: H
    role: [bodies]
    model: stamp
    started_at: '2026-01-01T00:00:00Z'
    dispatcher_run_id: held
`)
	repo.commit("held-malformed-role", "features")
	_, readings := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live", repo.path, "features"),
	}, Selection{Cutoff: contractCutoff(), HoldoutRunIDs: []string{"held"}}, ReadBounds{})
	found := false
	for _, r := range readings.Readings {
		if r.Identity.RunID.Known && r.Identity.RunID.Value == "held" {
			found = true
			if r.Excluded != DispositionHeldOut {
				t.Fatalf("independently decoded holdout not HeldOut: %+v", r)
			}
		}
	}
	if !found {
		t.Fatal("malformed held-out identity lost")
	}
}

func testF3AllJournalsHeldout(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "held", "synthetic-utc-offset.jsonl")
	repo := initGitRepo(t)
	repo.writeTestdata("features/study/tasks.yaml", "yaml", "empty-tasks.yaml")
	repo.commit("empty yaml", "features")
	manifest, readings := mustReadSources(t, []SourceSpec{
		journalSpec("j", runs),
		liveSpec("live", repo.path, "features"),
	}, Selection{Cutoff: contractCutoff(), HoldoutRunIDs: []string{"held"}}, ReadBounds{})
	if manifest.State != SourceComplete {
		t.Fatalf("all-held-out state = %q, want COMPLETE", manifest.State)
	}
	var journals int
	for _, src := range manifest.Sources {
		if src.Kind == SourceKindJournals {
			journals = src.Counts.Journals
			if src.Counts.JournalsExcludedByHoldout != 1 {
				t.Fatalf("JournalsExcludedByHoldout = %d", src.Counts.JournalsExcludedByHoldout)
			}
		}
	}
	if journals != 1 {
		t.Fatalf("Journals pre-exclusion = %d", journals)
	}
	if len(readings.Journals) != 0 {
		t.Fatal("held-out payload still in Journals")
	}
}

func testF3SourceConcurrency(t *testing.T) {
	runs, repo := contractSourceTree(t)
	_, _, err := readContractSources(t, []SourceSpec{
		{ID: "b-hist", Kind: SourceKindGitHistory, Repository: repo.path, Roots: []string{"features"}},
		journalSpec("a-j", runs),
	}, defaultSelection(), ReadBounds{MaxProcesses: 2})
	if err != nil {
		t.Fatal(err)
	}
}

func testF3DuplicateJournalRun(t *testing.T) {
	a := filepath.Join(t.TempDir(), "a")
	b := filepath.Join(t.TempDir(), "b")
	writeJournalTree(t, a, "dup", "synthetic-utc-offset.jsonl")
	writeJournalTree(t, b, "dup", "synthetic-utc-offset.jsonl")
	manifest, _, err := readContractSources(t, []SourceSpec{
		journalSpec("j1", a),
		journalSpec("j2", b),
	}, defaultSelection(), ReadBounds{})
	requireSentinel(t, err, ErrDuplicateJournalRun)
	if manifest != nil && manifest.State == SourceComplete {
		t.Fatal("duplicate run reported COMPLETE")
	}
}

func testF3CompletenessCauses(t *testing.T) {
	m := completeManifest(SourcePartial)
	m.Sources[0].State = SourcePartial
	m.Sources[0].Counts.BoundsExceeded = 1
	err := m.ValidateComplete()
	if !errors.Is(err, ErrSourceIncomplete) || !errors.Is(err, ErrBoundExceeded) {
		t.Fatalf("bound PARTIAL = %v", err)
	}
}

func testF3ExclusionOrder(t *testing.T) {
	ref := ReadingRef{SourceID: "live", Path: "config.yaml", Revision: "live", RecordedAt: contractCutoff()}
	got, err := parseReadings([]byte(testdataFile(t, "yaml", "non-task-known-red.yaml")), ref)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Kind != DocumentNotTasks {
		t.Fatalf("Kind = %s, want not_tasks before field errors", got[0].Kind)
	}
}

func testF4ManifestEmptyLists(t *testing.T) {
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-root", "synthetic-utc-offset.jsonl")
	manifest, _ := mustReadSources(t, []SourceSpec{journalSpec("j", runs)}, defaultSelection(), ReadBounds{})
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"reasons"`, `"holdout_run_ids"`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("manifest omitted %s: %s", key, raw)
		}
	}
	if strings.Contains(string(raw), `"reasons":null`) || strings.Contains(string(raw), `"holdout_run_ids":null`) {
		t.Fatalf("null lists: %s", raw)
	}
}

func testF3CitationRow(t *testing.T) {
	ref := ReadingRef{SourceID: "live", Path: "features/study/tasks.yaml", Revision: "live", RecordedAt: contractCutoff()}
	got, err := parseReadings([]byte(testdataFile(t, "yaml", "malformed-sibling.yaml")), ref)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[int]bool{}
	for _, r := range got {
		if r.Kind == DocumentTaskRow {
			if rows[r.Ref.Row] {
				t.Fatalf("duplicate Ref.Row %d", r.Ref.Row)
			}
			rows[r.Ref.Row] = true
		}
	}
	if len(rows) < 2 {
		t.Fatal("rows not distinguished")
	}
}

func testF3OpenSourceNilBudget(t *testing.T) {
	_, err := openSourceFile(context.Background(), "x", nil, true)
	requireSentinel(t, err, ErrInvalidSourceSpec)
}
