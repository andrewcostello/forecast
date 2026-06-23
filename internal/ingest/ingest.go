// Package ingest projects a claude-dispatcher run's artifacts onto JIRA.
//
// The dispatcher stays pure: it only writes canonical local artifacts — a
// tasks.yaml (per-task outcome rows, including the jira_key that `forecast sync`
// maps) and a hash-chained journal.jsonl. `forecast ingest` reads those and
// projects the run-specific richness (per-task outcome + transcript attachment,
// and the whole-feature review) onto the mapped JIRA issues as comments and
// attachments. Planning is a pure function (testable, dry-runnable); only Apply
// touches JIRA, and the command gates it behind --apply.
package ingest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// TaskOutcome is one dispatcher task's run result, read from the run's
// tasks.yaml row. Only the fields ingest projects are modelled; unknown YAML
// keys are ignored.
type TaskOutcome struct {
	Key           string  `yaml:"key"`
	JiraKey       string  `yaml:"jira_key"`
	Summary       string  `yaml:"summary"`
	Status        string  `yaml:"status"`
	CostUSD       float64 `yaml:"cost_usd"`
	PRURL         string  `yaml:"pr_url"`
	TranscriptLog string  `yaml:"transcript_log"`
	HaikuSummary  string  `yaml:"haiku_summary"` // path to the haiku markdown
	// HaikuText is the haiku file's contents, loaded by LoadRun so Plan stays
	// pure (no filesystem reads).
	HaikuText string `yaml:"-"`
}

type tasksFile struct {
	Project string        `yaml:"project"`
	Epic    string        `yaml:"epic"`
	Tasks   []TaskOutcome `yaml:"tasks"`
}

// Disposition is one finding's recorded decision from the feature-review.
type Disposition struct {
	Disposition string
	Severity    string
	Location    string
	Reason      string
}

// FinalReview is the whole-feature review verdict + dispositions from the LAST
// review round in the journal. Present is false when the run had no feature
// review (the default dispatcher path).
type FinalReview struct {
	Present      bool
	Consensus    string
	Blocking     int
	Dispositions []Disposition
}

// RunArtifacts is everything ingest reads from a dispatcher run.
type RunArtifacts struct {
	Project     string
	Epic        string
	Tasks       []TaskOutcome
	FinalReview FinalReview
}

// ActionKind discriminates the projected JIRA writes.
type ActionKind string

const (
	ActionComment ActionKind = "comment"
	ActionAttach  ActionKind = "attach"
)

// Action is one projected JIRA write. The plan is pure data; Apply executes it.
type Action struct {
	Kind     ActionKind
	IssueKey string
	Body     string // comment body (ActionComment)
	FilePath string // file to attach (ActionAttach)
	Label    string // human-readable description for dry-run / logs
}

// LoadRun reads a dispatcher run's tasks.yaml (required) and, when given, its
// journal.jsonl (for the final feature-review). Haiku summaries referenced by
// rows are read into TaskOutcome.HaikuText so Plan can stay pure.
func LoadRun(tasksPath, journalPath string) (RunArtifacts, error) {
	var ra RunArtifacts
	raw, err := os.ReadFile(tasksPath)
	if err != nil {
		return ra, fmt.Errorf("read tasks yaml: %w", err)
	}
	var tf tasksFile
	if err := yaml.Unmarshal(raw, &tf); err != nil {
		return ra, fmt.Errorf("parse tasks yaml: %w", err)
	}
	ra.Project, ra.Epic, ra.Tasks = tf.Project, tf.Epic, tf.Tasks
	for i := range ra.Tasks {
		if p := ra.Tasks[i].HaikuSummary; p != "" {
			if b, err := os.ReadFile(p); err == nil {
				ra.Tasks[i].HaikuText = strings.TrimSpace(string(b))
			}
		}
	}
	if journalPath != "" {
		fr, err := parseFinalReview(journalPath)
		if err != nil {
			return ra, fmt.Errorf("parse journal: %w", err)
		}
		ra.FinalReview = fr
	}
	return ra, nil
}

// parseFinalReview scans the hash-chained journal and returns the LAST review
// round's verdict + dispositions (a multi-round fix loop re-reviews; the final
// round is the one worth projecting). Malformed lines are skipped — the journal
// is an audit aid, not a strict schema we should hard-fail on.
func parseFinalReview(path string) (FinalReview, error) {
	var fr FinalReview
	f, err := os.Open(path)
	if err != nil {
		return fr, err
	}
	defer f.Close()

	type event struct {
		EventType string         `json:"event_type"`
		Payload   map[string]any `json:"payload"`
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024) // journal lines can be large
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e event
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		switch e.EventType {
		case "feature_review_started":
			// New round — reset so we keep only the final round's findings.
			fr = FinalReview{}
		case "feature_review_verdict":
			fr.Present = true
			fr.Consensus, _ = e.Payload["consensus"].(string)
			if b, ok := e.Payload["blocking"].(float64); ok {
				fr.Blocking = int(b)
			}
		case "disposition_recorded":
			d := Disposition{}
			d.Disposition, _ = e.Payload["disposition"].(string)
			d.Severity, _ = e.Payload["severity"].(string)
			d.Location, _ = e.Payload["location"].(string)
			d.Reason, _ = e.Payload["reason"].(string)
			fr.Dispositions = append(fr.Dispositions, d)
		}
	}
	return fr, sc.Err()
}

// Plan computes the JIRA writes for a run, purely (no IO). For each task mapped
// to a JIRA issue (jira_key set) it plans an outcome comment and, if a
// transcript was logged, an attachment. When the run had a feature review and
// an epicKey is given, it plans one final-review comment on the epic.
func Plan(ra RunArtifacts, epicKey string) []Action {
	var actions []Action
	for _, t := range ra.Tasks {
		if t.JiraKey == "" {
			continue // not mapped to JIRA (run `forecast sync` first) — skip
		}
		actions = append(actions, Action{
			Kind:     ActionComment,
			IssueKey: t.JiraKey,
			Body:     renderTaskComment(t),
			Label:    fmt.Sprintf("comment → %s (%s, %s)", t.JiraKey, t.Key, t.Status),
		})
		if t.TranscriptLog != "" {
			actions = append(actions, Action{
				Kind:     ActionAttach,
				IssueKey: t.JiraKey,
				FilePath: t.TranscriptLog,
				Label:    fmt.Sprintf("attach transcript → %s", t.JiraKey),
			})
		}
	}
	if ra.FinalReview.Present && epicKey != "" {
		actions = append(actions, Action{
			Kind:     ActionComment,
			IssueKey: epicKey,
			Body:     renderFinalReview(ra),
			Label:    fmt.Sprintf("final-review comment → %s", epicKey),
		})
	}
	return actions
}

func renderTaskComment(t TaskOutcome) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**Dispatcher run outcome — `%s`**\n\n", t.Key)
	fmt.Fprintf(&b, "- Status: %s\n", orDash(t.Status))
	if t.PRURL != "" {
		fmt.Fprintf(&b, "- PR: %s\n", t.PRURL)
	}
	if t.CostUSD > 0 {
		fmt.Fprintf(&b, "- Cost: $%.2f\n", t.CostUSD)
	}
	if t.HaikuText != "" {
		fmt.Fprintf(&b, "\n%s\n", t.HaikuText)
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderFinalReview(ra RunArtifacts) string {
	fr := ra.FinalReview
	var b strings.Builder
	fmt.Fprintf(&b, "**Feature review — consensus: %s (%d blocking)**\n",
		orDash(fr.Consensus), fr.Blocking)
	if ra.Epic != "" {
		fmt.Fprintf(&b, "\nEpic: %s\n", ra.Epic)
	}
	if len(fr.Dispositions) > 0 {
		b.WriteString("\n| disposition | severity | location | reason |\n")
		b.WriteString("|---|---|---|---|\n")
		for _, d := range fr.Dispositions {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
				orDash(d.Disposition), orDash(d.Severity),
				orDash(d.Location), orDash(d.Reason))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// Writer is the minimal JIRA surface Apply needs. The command adapts
// *jira.Client to it (and routes per issue key across JIRA instances).
type Writer interface {
	AddComment(issueKey, comment string) error
	UploadAttachment(issueKey, filePath string) error
}

// Apply executes the planned actions, routing each to the Writer for its issue
// key (clientFor). It is fault-tolerant: a failed action is collected and the
// rest still run, so one bad issue key doesn't abort the whole projection.
// Returns the count applied and the per-action errors.
func Apply(actions []Action, clientFor func(issueKey string) (Writer, error)) (int, []error) {
	done := 0
	var errs []error
	for _, a := range actions {
		w, err := clientFor(a.IssueKey)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", a.Label, err))
			continue
		}
		switch a.Kind {
		case ActionComment:
			err = w.AddComment(a.IssueKey, a.Body)
		case ActionAttach:
			err = w.UploadAttachment(a.IssueKey, a.FilePath)
		default:
			err = fmt.Errorf("unknown action kind %q", a.Kind)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", a.Label, err))
			continue
		}
		done++
	}
	return done, errs
}
