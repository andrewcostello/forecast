package dispatched

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
	const externalKey = "FC-SEALS-EXTERNAL-SYMLINK-PAYLOAD"
	writeFileT(t, filepath.Join(outside, "escaped.yaml"), `tasks:
  - key: FC-SEALS-EXTERNAL-SYMLINK-PAYLOAD
    role: bodies
    model: must-not-be-read
    status: Done
    started_at: '2026-01-01T00:00:00Z'
    completed_at: '2026-01-01T00:01:00Z'
    dispatcher_run_id: escaped-run
`)
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
	if err == nil || (!errors.Is(err, ErrSourceMissing) && !errors.Is(err, ErrInvalidSourceSpec)) {
		t.Fatalf("symlink escape = %v, want ErrInvalidSourceSpec or ErrSourceMissing", err)
	}
	if readings != nil {
		for _, reading := range readings.Readings {
			if filepath.Clean(reading.Ref.Path) == filepath.Clean("features/link.yaml") {
				t.Fatalf("symlink link path was consumed: %+v", reading)
			}
			if reading.Identity.Key.Known && reading.Identity.Key.Value == externalKey {
				t.Fatalf("unique external payload was consumed through the symlink: %+v", reading)
			}
		}
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
	repo.writeTestdata("features/study/malformed-document.yaml", "yaml", "malformed-document.yaml")
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
	var malformedDocument bool
	for _, r := range readings.Readings {
		if r.Kind == DocumentTaskRow && r.Err == nil && r.Identity.Key.Value == "VALID" {
			valid++
		}
		if r.Err != nil {
			malformed++
		}
		if r.Kind == DocumentMalformed && r.Ref.Row == 0 && r.Err != nil {
			malformedDocument = true
		}
	}
	if valid != 1 || malformed < 2 || !malformedDocument {
		t.Fatalf("valid=%d malformed=%d malformed_document=%v readings=%+v", valid, malformed, malformedDocument, readings.Readings)
	}
	if manifest.Sources[1].Counts.Malformed < 2 {
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

	document, err := parseReadings([]byte(testdataFile(t, "yaml", "malformed-document.yaml")), ref)
	if err != nil {
		t.Fatalf("ordinary malformed document returned a top-level error: %v", err)
	}
	if len(document) != 1 || document[0].Kind != DocumentMalformed || document[0].Ref.Row != 0 || document[0].Err == nil {
		t.Fatalf("malformed document envelope = %+v", document)
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
		if r.Ref.Revision == "git:"+side {
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
	if hist.Counts.Commits != 3 || hist.Counts.BoundsExceeded < 1 {
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
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("data bound should retain a PARTIAL diagnostic result with nil read error: %v", err)
	}
	if manifest == nil {
		t.Fatal("nil manifest on byte cap")
	}
	if manifest.State != SourcePartial {
		t.Fatalf("state = %q", manifest.State)
	}
	var boundedSource bool
	for _, source := range manifest.Sources {
		if source.ID == "live" && source.Counts.BoundsExceeded > 0 {
			boundedSource = true
		}
	}
	if !boundedSource {
		t.Fatalf("blob cap did not increment source bound diagnostics: %+v", manifest.Sources)
	}

	// Drive the mandatory streaming Git seam with a child that writes one
	// unique byte beyond the blob cap and then deliberately withholds EOF. A
	// bounded reader must return ErrBoundExceeded without waiting for release;
	// an implementation that buffers the whole child output cannot do so.
	marker := filepath.Join(t.TempDir(), "wrote-over-cap")
	release := filepath.Join(t.TempDir(), "release")
	t.Setenv("FC_SEALS_OUTPUT_MARKER", marker)
	t.Setenv("FC_SEALS_OUTPUT_RELEASE", release)
	installFixtureGitWrapper(t, "printf '"+strings.Repeat("a", 64)+"Z'\n: > \"$FC_SEALS_OUTPUT_MARKER\"\nwhile [ ! -e \"$FC_SEALS_OUTPUT_RELEASE\" ]; do :; done")
	t.Cleanup(func() { _ = os.WriteFile(release, []byte("release\n"), 0o600) })
	budget, err := newSourceBudget(ReadBounds{MaxBlobBytes: 64, MaxTotalBytes: 1024, MaxProcesses: 1})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := runSourceGit(context.Background(), t.TempDir(), budget, SourceGitRequest{
		Args: []string{"cat-file", "blob", strings.Repeat("a", 40)}, Blob: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	type readResult struct {
		data []byte
		err  error
	}
	readDone := make(chan readResult, 1)
	go func() {
		data, readErr := io.ReadAll(reader)
		readDone <- readResult{data: data, err: readErr}
	}()
	if !fixturePathAppears(marker, 3*time.Second) {
		signalFixturePath(t, release)
		t.Fatal("controlled Git child never wrote its over-cap marker")
	}
	var bounded readResult
	select {
	case bounded = <-readDone:
	case <-time.After(3 * time.Second):
		signalFixturePath(t, release)
		<-readDone
		t.Fatal("blob read waited for EOF after the over-cap probe; output was buffered before bounding")
	}
	signalFixturePath(t, release)
	if !errors.Is(bounded.err, ErrBoundExceeded) {
		t.Fatalf("bounded blob read = %v, want ErrBoundExceeded", bounded.err)
	}
	if len(bounded.data) > 64 || strings.Contains(string(bounded.data), "Z") {
		t.Fatalf("over-cap byte was retained: len=%d data=%q", len(bounded.data), bounded.data)
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

	state := t.TempDir()
	firstStarted := filepath.Join(state, "first-started")
	secondStarted := filepath.Join(state, "second-started")
	firstRelease := filepath.Join(state, "first-release")
	secondRelease := filepath.Join(state, "second-release")
	t.Setenv("FC_SEALS_PROCESS_STATE", state)
	installFixtureGitWrapper(t, `if mkdir "$FC_SEALS_PROCESS_STATE/first-claimed" 2>/dev/null; then
  : > "$FC_SEALS_PROCESS_STATE/first-started"
  while [ ! -e "$FC_SEALS_PROCESS_STATE/first-release" ]; do :; done
  printf 'first\n'
else
  : > "$FC_SEALS_PROCESS_STATE/second-started"
  while [ ! -e "$FC_SEALS_PROCESS_STATE/second-release" ]; do :; done
  printf 'second\n'
fi`)
	t.Cleanup(func() {
		_ = os.WriteFile(firstRelease, []byte("release\n"), 0o600)
		_ = os.WriteFile(secondRelease, []byte("release\n"), 0o600)
	})
	budget, err := newSourceBudget(ReadBounds{MaxProcesses: 1, MaxTotalBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	runnerRepo := t.TempDir()
	first, err := runSourceGit(context.Background(), runnerRepo, budget, SourceGitRequest{Args: []string{"rev-parse", "HEAD"}})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if !fixturePathAppears(firstStarted, 3*time.Second) {
		t.Fatal("first controlled Git child did not start")
	}
	type runnerResult struct {
		reader io.ReadCloser
		err    error
	}
	secondResult := make(chan runnerResult, 1)
	go func() {
		second, runErr := runSourceGit(context.Background(), runnerRepo, budget, SourceGitRequest{Args: []string{"rev-parse", "HEAD"}})
		secondResult <- runnerResult{reader: second, err: runErr}
	}()
	select {
	case early := <-secondResult:
		if early.reader != nil {
			_ = early.reader.Close()
		}
		t.Fatalf("second runSourceGit did not wait for the per-source slot: %v", early.err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := os.Stat(secondStarted); err == nil {
		t.Fatal("second Git child spawned while MaxProcesses=1 slot was occupied")
	}
	signalFixturePath(t, firstRelease)
	firstData, firstErr := io.ReadAll(first)
	if firstErr != nil || string(firstData) != "first\n" {
		t.Fatalf("first controlled Git child = %q, %v", firstData, firstErr)
	}
	if closeErr := first.Close(); closeErr != nil {
		t.Fatalf("close first controlled Git child: %v", closeErr)
	}
	var second runnerResult
	select {
	case second = <-secondResult:
	case <-time.After(3 * time.Second):
		t.Fatal("second runSourceGit did not acquire released slot")
	}
	if second.err != nil || second.reader == nil {
		t.Fatalf("second runSourceGit = %v", second.err)
	}
	defer second.reader.Close()
	if !fixturePathAppears(secondStarted, 3*time.Second) {
		t.Fatal("second controlled Git child never spawned after slot release")
	}
	signalFixturePath(t, secondRelease)
	secondData, secondErr := io.ReadAll(second.reader)
	if secondErr != nil || string(secondData) != "second\n" {
		t.Fatalf("second controlled Git child = %q, %v", secondData, secondErr)
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
	ref := ReadingRef{SourceID: "live", Repository: "repo", Path: "features/study/tasks.yaml", Revision: "live"}
	got, err := parseReadings([]byte(testdataFile(t, "yaml", "offset-equivalent.yaml")), ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != DocumentTaskRow || !got[0].Ref.RecordedAt.IsZero() || got[0].Err != nil || got[0].Excluded != "" {
		t.Fatalf("zero-time parser envelope = %+v", got)
	}
	start := mustTime(t, "2026-01-01T00:00:00Z")
	attempt := journalAttempt("run-offset", "F1-OFFSET", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("run-offset", attempt)
	joined, err := joinContract(t, []AttemptSet{set}, got, defaultSelection(), identityUniverse(set))
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	requireDisposition(t, joined, DispositionMalformed, 1)
	requireDisposition(t, joined, DispositionAfterCutoff, 0)
	if joined.Recovered != 0 || len(joined.Observations) != 0 || len(joined.Examined) != 1 || !joined.Examined[0].Reading.RecordedAt.IsZero() {
		t.Fatalf("zero RecordedAt was sampled or lost from malformed audit: %+v", joined)
	}
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
	runs, firstRepo := contractSourceTree(t)
	secondRepo := initGitRepo(t)
	secondRepo.writeTestdata("features/study/tasks.yaml", "yaml", "offset-equivalent.yaml")
	secondRepo.commit("second source", "features")
	state := t.TempDir()
	firstMarker := filepath.Join(state, "first-source-entered")
	secondMarker := filepath.Join(state, "second-source-entered")
	firstRelease := filepath.Join(state, "release-first-source")
	t.Setenv("FC_SEALS_FIRST_REPO", firstRepo.path)
	t.Setenv("FC_SEALS_SECOND_REPO", secondRepo.path)
	t.Setenv("FC_SEALS_FIRST_SOURCE_MARKER", firstMarker)
	t.Setenv("FC_SEALS_SECOND_SOURCE_MARKER", secondMarker)
	t.Setenv("FC_SEALS_FIRST_SOURCE_RELEASE", firstRelease)
	installFixtureGitWrapper(t, `case "$(pwd) $*" in
  *"$FC_SEALS_FIRST_REPO"*)
    : > "$FC_SEALS_FIRST_SOURCE_MARKER"
    while [ ! -e "$FC_SEALS_FIRST_SOURCE_RELEASE" ]; do :; done
    ;;
  *"$FC_SEALS_SECOND_REPO"*)
    : > "$FC_SEALS_SECOND_SOURCE_MARKER"
    ;;
esac
exec "$FC_SEALS_REAL_GIT" "$@"`)
	t.Cleanup(func() { _ = os.WriteFile(firstRelease, []byte("release\n"), 0o600) })
	type sourceResult struct {
		manifest *SourceManifest
		err      error
	}
	result := make(chan sourceResult, 1)
	go func() {
		manifest, _, err := ReadSources(context.Background(), []SourceSpec{
			{ID: "b-history", Kind: SourceKindGitHistory, Repository: secondRepo.path, Roots: []string{"features"}},
			journalSpec("z-journals", runs),
			{ID: "a-history", Kind: SourceKindGitHistory, Repository: firstRepo.path, Roots: []string{"features"}},
		}, defaultSelection(), ReadBounds{MaxProcesses: 2})
		result <- sourceResult{manifest: manifest, err: err}
	}()
	deadline := time.NewTimer(3 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	firstEntered := false
	for !firstEntered {
		if _, err := os.Stat(firstMarker); err == nil {
			firstEntered = true
			break
		}
		select {
		case early := <-result:
			ticker.Stop()
			deadline.Stop()
			t.Fatalf("ReadSources returned before first source marker: %v", early.err)
		case <-deadline.C:
			ticker.Stop()
			t.Fatal("first SourceID never entered the Git runner")
		case <-ticker.C:
		}
	}
	ticker.Stop()
	deadline.Stop()
	if fixturePathAppears(secondMarker, 300*time.Millisecond) {
		signalFixturePath(t, firstRelease)
		<-result
		t.Fatal("second source entered while the first SourceID was blocked")
	}
	signalFixturePath(t, firstRelease)
	var completed sourceResult
	select {
	case completed = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("sequential source read did not complete after release")
	}
	if completed.err != nil {
		t.Fatal(completed.err)
	}
	if !fixturePathAppears(secondMarker, time.Second) {
		t.Fatal("second source never entered after first source completed")
	}
	if completed.manifest == nil || completed.manifest.Bounds.MaxProcesses != 2 {
		t.Fatalf("resolved process bound missing: %+v", completed.manifest)
	}
	wantIDs := []string{"a-history", "b-history", "z-journals"}
	if len(completed.manifest.Sources) != len(wantIDs) {
		t.Fatalf("source reports = %+v", completed.manifest.Sources)
	}
	for i, want := range wantIDs {
		if completed.manifest.Sources[i].ID != want {
			t.Fatalf("source report order[%d] = %q, want %q", i, completed.manifest.Sources[i].ID, want)
		}
		if completed.manifest.Sources[i].Counts.BoundsExceeded != 0 {
			t.Fatalf("source/process sequencing changed bound counts: %+v", completed.manifest.Sources[i])
		}
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
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	requireJSONArray(t, object, "reasons", 0)
	requireJSONArray(t, object, "holdout_run_ids", 0)
	sourcesRaw, ok := object["sources"]
	if !ok || string(sourcesRaw) == "null" {
		t.Fatalf("manifest sources omitted/null: %s", raw)
	}
	var sources []map[string]json.RawMessage
	if err := json.Unmarshal(sourcesRaw, &sources); err != nil {
		t.Fatalf("manifest sources missing/not array: %s (%v)", raw, err)
	}
	if len(sources) != 1 {
		t.Fatalf("manifest source count = %d, want 1", len(sources))
	}
	for _, source := range sources {
		requireJSONArray(t, source, "reasons", 0)
		if string(source["kind"]) == `"journals"` {
			requireJSONArray(t, source, "roots", 0)
			requireJSONArray(t, source, "resolved_refs", 0)
		}
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
