package ingest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Plan (pure) ------------------------------------------------------------

func TestPlanSkipsTasksWithoutJiraKey(t *testing.T) {
	ra := RunArtifacts{Tasks: []TaskOutcome{
		{Key: "T1", Status: "Merged"},                  // no jira_key -> skipped
		{Key: "T2", JiraKey: "SMG-2", Status: "Merged"}, // mapped -> comment
	}}
	actions := Plan(ra, "")
	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d: %+v", len(actions), actions)
	}
	if actions[0].Kind != ActionComment || actions[0].IssueKey != "SMG-2" {
		t.Fatalf("unexpected action: %+v", actions[0])
	}
}

func TestPlanAddsTranscriptAttachment(t *testing.T) {
	ra := RunArtifacts{Tasks: []TaskOutcome{
		{Key: "T1", JiraKey: "SMG-1", Status: "Merged", TranscriptLog: "/runs/T1/transcript.json"},
	}}
	actions := Plan(ra, "")
	if len(actions) != 2 {
		t.Fatalf("want comment+attach (2), got %d", len(actions))
	}
	var kinds []ActionKind
	for _, a := range actions {
		kinds = append(kinds, a.Kind)
		if a.IssueKey != "SMG-1" {
			t.Fatalf("attachment routed to wrong key: %+v", a)
		}
	}
	if kinds[0] != ActionComment || kinds[1] != ActionAttach {
		t.Fatalf("want [comment, attach], got %v", kinds)
	}
	if actions[1].FilePath != "/runs/T1/transcript.json" {
		t.Fatalf("attach path = %q", actions[1].FilePath)
	}
}

func TestPlanTaskCommentBodyHasOutcome(t *testing.T) {
	ra := RunArtifacts{Tasks: []TaskOutcome{{
		Key: "T1", JiraKey: "SMG-1", Status: "Merged",
		PRURL: "https://gh/pr/9", CostUSD: 1.23, HaikuText: "Did the thing cleanly.",
	}}}
	body := Plan(ra, "")[0].Body
	for _, want := range []string{"T1", "Merged", "https://gh/pr/9", "$1.23", "Did the thing cleanly."} {
		if !strings.Contains(body, want) {
			t.Errorf("comment body missing %q:\n%s", want, body)
		}
	}
}

func TestPlanFinalReviewOnlyWithEpicKey(t *testing.T) {
	ra := RunArtifacts{
		Tasks:       []TaskOutcome{{Key: "T1", JiraKey: "SMG-1"}},
		FinalReview: FinalReview{Present: true, Consensus: "block", Blocking: 2},
	}
	// No epic key → no final-review action.
	if got := Plan(ra, ""); len(got) != 1 {
		t.Fatalf("without epic key want 1 action, got %d", len(got))
	}
	// With epic key → one extra comment on the epic.
	actions := Plan(ra, "SMG-100")
	last := actions[len(actions)-1]
	if last.Kind != ActionComment || last.IssueKey != "SMG-100" {
		t.Fatalf("final-review action wrong: %+v", last)
	}
	if !strings.Contains(last.Body, "block") || !strings.Contains(last.Body, "2 blocking") {
		t.Fatalf("final-review body missing verdict:\n%s", last.Body)
	}
}

func TestPlanFinalReviewSkippedWhenAbsent(t *testing.T) {
	ra := RunArtifacts{Tasks: []TaskOutcome{{Key: "T1", JiraKey: "SMG-1"}}}
	if got := Plan(ra, "SMG-100"); len(got) != 1 {
		t.Fatalf("no review present → no final-review action; got %d", len(got))
	}
}

// --- LoadRun ----------------------------------------------------------------

func TestLoadRunParsesTasksHaikuAndJournal(t *testing.T) {
	dir := t.TempDir()
	haiku := filepath.Join(dir, "haiku.md")
	if err := os.WriteFile(haiku, []byte("  one-line haiku.  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tasks := filepath.Join(dir, "tasks.yaml")
	os.WriteFile(tasks, []byte(`project: SMG
epic: wallet
tasks:
  - key: T1
    jira_key: SMG-1
    status: Merged
    cost_usd: 2.5
    pr_url: https://gh/pr/1
    transcript_log: /runs/T1/transcript.json
    haiku_summary: `+haiku+`
  - key: T2
    status: Blocked
`), 0o644)

	journal := filepath.Join(dir, "journal.jsonl")
	os.WriteFile(journal, []byte(strings.Join([]string{
		`{"event_type":"feature_review_started","payload":{}}`,
		`{"event_type":"disposition_recorded","payload":{"disposition":"defer","severity":"LOW","location":"a.py:1","reason":"non-blocking"}}`,
		// A second round supersedes the first (final round wins).
		`{"event_type":"feature_review_started","payload":{}}`,
		`{"event_type":"disposition_recorded","payload":{"disposition":"accept","severity":"HIGH","location":"b.py:2","reason":"corroborated"}}`,
		`{"event_type":"feature_review_verdict","payload":{"consensus":"block","blocking":1}}`,
		`garbage line that should be skipped`,
	}, "\n")), 0o644)

	ra, err := LoadRun(tasks, journal)
	if err != nil {
		t.Fatal(err)
	}
	if ra.Project != "SMG" || ra.Epic != "wallet" || len(ra.Tasks) != 2 {
		t.Fatalf("tasks header parse: %+v", ra)
	}
	if ra.Tasks[0].HaikuText != "one-line haiku." {
		t.Errorf("haiku text not loaded/trimmed: %q", ra.Tasks[0].HaikuText)
	}
	if !ra.FinalReview.Present || ra.FinalReview.Consensus != "block" || ra.FinalReview.Blocking != 1 {
		t.Fatalf("final review verdict: %+v", ra.FinalReview)
	}
	// Only the LAST round's dispositions are kept.
	if len(ra.FinalReview.Dispositions) != 1 || ra.FinalReview.Dispositions[0].Disposition != "accept" {
		t.Fatalf("expected only final-round dispositions, got %+v", ra.FinalReview.Dispositions)
	}
}

func TestLoadRunNoJournalIsFine(t *testing.T) {
	dir := t.TempDir()
	tasks := filepath.Join(dir, "tasks.yaml")
	os.WriteFile(tasks, []byte("project: SMG\ntasks:\n  - key: T1\n    jira_key: SMG-1\n"), 0o644)
	ra, err := LoadRun(tasks, "")
	if err != nil {
		t.Fatal(err)
	}
	if ra.FinalReview.Present {
		t.Fatal("no journal → FinalReview.Present should be false")
	}
}

func TestLoadRunMissingTasksErrors(t *testing.T) {
	if _, err := LoadRun(filepath.Join(t.TempDir(), "nope.yaml"), ""); err == nil {
		t.Fatal("expected error for missing tasks file")
	}
}

// --- Apply (mock writer) ----------------------------------------------------

type mockWriter struct {
	comments map[string][]string
	attaches map[string][]string
	failKey  string
}

func newMock() *mockWriter {
	return &mockWriter{comments: map[string][]string{}, attaches: map[string][]string{}}
}
func (m *mockWriter) AddComment(k, c string) error {
	if k == m.failKey {
		return fmt.Errorf("boom")
	}
	m.comments[k] = append(m.comments[k], c)
	return nil
}
func (m *mockWriter) UploadAttachment(k, p string) error {
	if k == m.failKey {
		return fmt.Errorf("boom")
	}
	m.attaches[k] = append(m.attaches[k], p)
	return nil
}

func TestApplyRoutesAndExecutes(t *testing.T) {
	m := newMock()
	actions := []Action{
		{Kind: ActionComment, IssueKey: "SMG-1", Body: "hi"},
		{Kind: ActionAttach, IssueKey: "SMG-1", FilePath: "/t.json"},
	}
	done, errs := Apply(actions, func(string) (Writer, error) { return m, nil })
	if done != 2 || len(errs) != 0 {
		t.Fatalf("want 2 done 0 errs, got %d / %v", done, errs)
	}
	if len(m.comments["SMG-1"]) != 1 || len(m.attaches["SMG-1"]) != 1 {
		t.Fatalf("writes not recorded: %+v %+v", m.comments, m.attaches)
	}
}

func TestApplyIsFaultTolerant(t *testing.T) {
	m := newMock()
	m.failKey = "SMG-BAD"
	actions := []Action{
		{Kind: ActionComment, IssueKey: "SMG-BAD", Body: "x"}, // fails
		{Kind: ActionComment, IssueKey: "SMG-OK", Body: "y"},  // still runs
	}
	done, errs := Apply(actions, func(string) (Writer, error) { return m, nil })
	if done != 1 || len(errs) != 1 {
		t.Fatalf("want 1 done 1 err, got %d / %v", done, errs)
	}
	if len(m.comments["SMG-OK"]) != 1 {
		t.Fatal("the good action should still have run")
	}
}

func TestApplySurfacesClientRoutingError(t *testing.T) {
	actions := []Action{{Kind: ActionComment, IssueKey: "SMG-1", Body: "x"}}
	done, errs := Apply(actions, func(string) (Writer, error) {
		return nil, fmt.Errorf("no instance for key")
	})
	if done != 0 || len(errs) != 1 {
		t.Fatalf("routing error should abort that action: %d / %v", done, errs)
	}
}
