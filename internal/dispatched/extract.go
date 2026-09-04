package dispatched

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// defaultMaxHistoryCommits bounds the history walk. The walk is linear in
// commits and, after blob de-duplication, in distinct file contents; the cap
// exists so an unexpectedly large repository fails loudly and countably
// rather than running until it is killed.
const defaultMaxHistoryCommits = 5000

type runTask struct {
	RunID string
	Key   string
}

// JournalFacts is the Go port of model-matrix/report.py::journal_facts, for a
// single attempt at one task.
//
// Model is taken ONLY from payloads whose spawn_kind is implementer or
// panel-iterate, exactly as the reference does. The model on task_started is
// the model the dispatcher PLANNED, recorded before any cascade; using it
// would attribute a cascaded row to a model that never ran. An attempt with
// no such payload has no Model and cannot be placed in a cell.
type JournalFacts struct {
	// StartedAt is the timestamp of the task_started that opened this
	// attempt; zero for events seen before any task_started.
	StartedAt          time.Time
	Model              string
	Started            int
	StartsWithoutModel int
	Rounds             int
	InputTokens        int64
	OutputTokens       int64
	// CostUSD is summed from spawn payloads. CostKnown distinguishes "the
	// spawns cost nothing that was recorded" from "no spawn recorded a cost";
	// the reference implementation reads cost from the tasks YAML instead and
	// so has no equivalent.
	CostUSD         float64
	CostKnown       bool
	DevElapsed      time.Duration
	ReviewElapsed   time.Duration
	TerminalOutcome Outcome
	TerminalAt      time.Time
	Fallbacks       int

	reviewOpen time.Time
	devOpen    time.Time
}

// JournalRow holds every attempt at one (run, task key). A run that restarts
// a task emits a second task_started; folding both into one set of facts
// double-counts its tokens and lets two YAML snapshots with different
// started_at values each claim the same terminal event.
type JournalRow struct {
	Attempts []*JournalFacts
}

// Starts is the number of task_started events seen for the row.
func (r *JournalRow) Starts() int {
	n := 0
	for _, f := range r.Attempts {
		n += f.Started
	}
	return n
}

// Restarts is the number of task_started events beyond the first.
func (r *JournalRow) Restarts() int { return max(r.Starts()-1, 0) }

// StartsWithoutModel is the number of task_started events carrying no model.
func (r *JournalRow) StartsWithoutModel() int {
	n := 0
	for _, f := range r.Attempts {
		n += f.StartsWithoutModel
	}
	return n
}

// AttemptsWithoutStampedModel counts started attempts that no implementing
// spawn stamped a model on. Such an attempt is unattributable by rule.
func (r *JournalRow) AttemptsWithoutStampedModel() int {
	n := 0
	for _, f := range r.Attempts {
		if f.Started > 0 && f.Model == "" {
			n++
		}
	}
	return n
}

// Match binds a snapshot only to an attempt with the same start instant.
// Multiple starts at that instant are ambiguous and cannot supply evidence.
func (r *JournalRow) Match(startedAt time.Time) *JournalFacts {
	var match *JournalFacts
	for _, f := range r.Attempts {
		if !f.StartedAt.IsZero() && f.StartedAt.Equal(startedAt) {
			if match != nil {
				return nil
			}
			match = f
		}
	}
	return match
}

// observe routes one event to the attempt it belongs to. A task_started opens
// a new attempt; events seen before any task_started are kept in a leading
// attempt with no start instant rather than discarded.
func (r *JournalRow) observe(eventType string, at time.Time, p journalPayload) {
	if eventType == "task_started" {
		r.Attempts = append(r.Attempts, &JournalFacts{StartedAt: at})
	} else if len(r.Attempts) == 0 {
		r.Attempts = append(r.Attempts, &JournalFacts{})
	}
	r.Attempts[len(r.Attempts)-1].observe(eventType, at, p)
}

type journalEvent struct {
	EventType string          `json:"event_type"`
	TaskKey   string          `json:"task_key"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type journalPayload struct {
	Model        string   `json:"model"`
	SpawnKind    string   `json:"spawn_kind"`
	InputTokens  int64    `json:"input_tokens"`
	OutputTokens int64    `json:"output_tokens"`
	CostUSD      *float64 `json:"cost_usd"`
}

// implementing reports whether a payload describes the spawn that actually
// did the work, which is the only payload the reference trusts for a model.
func (p journalPayload) implementing() bool {
	return p.SpawnKind == "implementer" || p.SpawnKind == "panel-iterate"
}

// journalSources is what readJournals recovered, with the shortfalls it hit.
// Unreadable input is counted rather than dropped: recovered-versus-started
// is only an honest denominator if the lines nobody could read are named.
type journalSources struct {
	Rows          map[runTask]*JournalRow
	LinesUnparsed int
	BadTimestamps int
}

// readJournals reads direct child run directories of runsDir. A line that is
// not JSON, and an event whose timestamp is not RFC 3339, are counted and
// skipped; a journal that cannot be opened or scanned is an error.
func readJournals(ctx context.Context, runsDir string) (*journalSources, error) {
	info, err := os.Stat(runsDir)
	if err != nil {
		return nil, fmt.Errorf("%w: stat %s: %v", ErrJournalSource, runsDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrJournalSource, runsDir)
	}
	pattern := filepath.Join(runsDir, "*", "journal.jsonl")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: glob %s: %v", ErrJournalSource, pattern, err)
	}
	sort.Strings(paths)
	sources := &journalSources{Rows: make(map[runTask]*JournalRow)}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("%w: reading %s: %v", ErrJournalSource, path, err)
		}
		if err := readJournal(ctx, path, filepath.Base(filepath.Dir(path)), sources); err != nil {
			return nil, err
		}
	}
	return sources, nil
}

func readJournal(ctx context.Context, path, runID string, out *journalSources) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%w: open %s: %v", ErrJournalSource, path, err)
	}
	defer f.Close()

	return scanJournal(ctx, f, path, runID, out)
}

func scanJournal(ctx context.Context, reader io.Reader, path, runID string, out *journalSources) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w: scan %s: %w", ErrJournalSource, path, err)
		}
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var event journalEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			out.LinesUnparsed++
			continue
		}
		if event.TaskKey == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, event.Timestamp)
		if err != nil {
			out.BadTimestamps++
			continue
		}
		var payload journalPayload
		if len(event.Payload) != 0 && string(event.Payload) != "null" {
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				out.LinesUnparsed++
				continue
			}
		}

		key := runTask{RunID: runID, Key: event.TaskKey}
		row := out.Rows[key]
		if row == nil {
			row = &JournalRow{}
			out.Rows[key] = row
		}
		row.observe(event.EventType, at, payload)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%w: scan %s: %v", ErrJournalSource, path, err)
	}
	return nil
}

// observe deliberately mirrors journal_facts' state machine, event for event.
func (f *JournalFacts) observe(eventType string, at time.Time, p journalPayload) {
	switch eventType {
	case "panel_started":
		f.reviewOpen = at
	case "panel_verdict":
		if !f.reviewOpen.IsZero() {
			f.ReviewElapsed += at.Sub(f.reviewOpen)
			f.reviewOpen = time.Time{}
		}
	}

	if eventType == "task_started" || eventType == "panel_iterate" {
		f.devOpen = at
	} else if eventType == "task_spawn_finished" && p.implementing() && !f.devOpen.IsZero() {
		f.DevElapsed += at.Sub(f.devOpen)
		f.devOpen = time.Time{}
	}

	switch eventType {
	case "task_started":
		f.Started++
		// Counted, never used to attribute: this model is the plan, not the
		// stamp. See the JournalFacts godoc.
		if p.Model == "" {
			f.StartsWithoutModel++
		}
	case "panel_iterate":
		f.Rounds++
	case "agent_fallback":
		f.Fallbacks++
	case "task_done":
		f.TerminalOutcome, f.TerminalAt = OutcomeDone, at
	case "task_blocked":
		f.TerminalOutcome, f.TerminalAt = OutcomeBlocked, at
	}

	if p.implementing() {
		if p.Model != "" {
			f.Model = p.Model
		}
		f.InputTokens += p.InputTokens
		f.OutputTokens += p.OutputTokens
		if p.CostUSD != nil {
			f.CostUSD += *p.CostUSD
			f.CostKnown = true
		}
	}
}

type yamlTask struct {
	Key             string `yaml:"key"`
	Role            Role   `yaml:"role"`
	Model           string `yaml:"model"`
	Status          string `yaml:"status"`
	StartedAt       string `yaml:"started_at"`
	CompletedAt     string `yaml:"completed_at"`
	IterationCount  int    `yaml:"iteration_count"`
	DispatcherRunID string `yaml:"dispatcher_run_id"`
}

type yamlDocument struct {
	Tasks []yamlTask `yaml:"tasks"`
}

type taskSnapshot struct {
	Key             string
	Role            Role
	AuthoredModel   string
	Status          string
	StartedAt       time.Time
	CompletedAt     time.Time
	IterationCount  int
	DispatcherRunID string
	Revision        Revision
	Repository      string
	Path            string
}

// yamlSources is what one YAML source yielded, plus what it could not yield.
// A document that is not a task list, and a row with an unreadable timestamp,
// are counted and skipped: features/ holds YAML that is not a tasks file, and
// one such file must not abort the union.
type yamlSources struct {
	Snapshots            []taskSnapshot
	Documents            int
	UnparseableDocuments int
	MalformedRows        int
	MissingJoinKeys      int
}

func (s *yamlSources) absorb(other yamlSources) {
	s.Snapshots = append(s.Snapshots, other.Snapshots...)
	s.Documents += other.Documents
	s.UnparseableDocuments += other.UnparseableDocuments
	s.MalformedRows += other.MalformedRows
	s.MissingJoinKeys += other.MissingJoinKeys
}

// parseSnapshots reads one YAML document. It never returns an error: an
// unreadable document or row is reported through the returned counters so the
// caller can name the shortfall instead of failing the whole build.
func parseSnapshots(data []byte, revision Revision) yamlSources {
	out := yamlSources{Documents: 1}
	doc, err := decodeTaskDocument(data)
	if err != nil {
		out.UnparseableDocuments++
		return out
	}
	for _, task := range doc.Tasks {
		if task.Key == "" || task.DispatcherRunID == "" || task.StartedAt == "" {
			out.MissingJoinKeys++
			continue
		}
		started, err := time.Parse(time.RFC3339Nano, task.StartedAt)
		if err != nil {
			out.MalformedRows++
			continue
		}
		var completed time.Time
		if task.CompletedAt != "" {
			completed, err = time.Parse(time.RFC3339Nano, task.CompletedAt)
			if err != nil {
				out.MalformedRows++
				continue
			}
		}
		out.Snapshots = append(out.Snapshots, taskSnapshot{
			Key: task.Key, Role: task.Role, AuthoredModel: task.Model,
			Status: task.Status, StartedAt: started, CompletedAt: completed,
			IterationCount: task.IterationCount, DispatcherRunID: task.DispatcherRunID,
			Revision: revision,
		})
	}
	return out
}

func isTaskYAML(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

func readLiveSnapshots(ctx context.Context, featuresRepo string) (yamlSources, error) {
	featuresDir := filepath.Join(featuresRepo, "features")
	var paths []string
	err := filepath.WalkDir(featuresDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if !entry.IsDir() && isTaskYAML(entry.Name()) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return yamlSources{}, fmt.Errorf("%w: walk %s: %v", ErrYAMLSource, featuresDir, err)
	}
	sort.Strings(paths)
	var out yamlSources
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return yamlSources{}, fmt.Errorf("%w: read %s: %v", ErrYAMLSource, path, err)
		}
		source := parseSnapshots(data, Revision{Source: SourceLive})
		relative, _ := filepath.Rel(featuresRepo, path)
		for i := range source.Snapshots {
			source.Snapshots[i].Repository = featuresRepo
			source.Snapshots[i].Path = filepath.ToSlash(relative)
		}
		out.absorb(source)
	}
	return out, nil
}

// gitSources adds history-walk shape to what the blobs yielded.
type gitSources struct {
	yamlSources
	Commits   int
	Blobs     int
	Truncated bool
}

// blobRef is one distinct file content, attributed to the first commit in
// walk order that holds it.
type blobRef struct {
	oid    string
	path   string
	commit string
}

// readGitSnapshots reads changed YAML blobs in a bounded commit walk. Git
// enumerates at most maxCommits+1 commits; the extra commit detects truncation.
// One diff-tree process lists changes, and one cat-file process reads blobs.
func readGitSnapshots(ctx context.Context, featuresRepo string, maxCommits int) (gitSources, error) {
	if maxCommits <= 0 {
		maxCommits = defaultMaxHistoryCommits
	}
	// Avoid overflow for the lookahead used to detect truncation.
	limit := maxCommits
	if limit < int(^uint(0)>>1) {
		limit++
	}
	commits, err := gitLines(ctx, featuresRepo, "rev-list", "--max-count="+strconv.Itoa(limit), "--all", "--", "features")
	if err != nil {
		return gitSources{}, err
	}
	out := gitSources{}
	if len(commits) > maxCommits {
		commits, out.Truncated = commits[:maxCommits], true
	}
	out.Commits = len(commits)
	if len(commits) == 0 {
		return out, nil
	}
	cmd := exec.CommandContext(ctx, "git", "-C", featuresRepo, "diff-tree", "--stdin", "--root", "-r", "-m", "--raw", "-z", "--no-abbrev", "--no-renames", "--", "features")
	cmd.Stdin = strings.NewReader(strings.Join(commits, "\n") + "\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return gitSources{}, fmt.Errorf("%w: git diff-tree in %s: %v: %s", ErrGitHistory, featuresRepo, err, strings.TrimSpace(stderr.String()))
	}
	records := strings.Split(stdout.String(), "\x00")
	seen := make(map[string]blobRef)
	var order []string
	commit := ""
	for i := 0; i < len(records); i++ {
		record := records[i]
		if record == "" {
			continue
		}
		if !strings.HasPrefix(record, ":") {
			commit = record
			continue
		}
		fields := strings.Fields(record)
		if len(fields) != 5 || i+1 >= len(records) || commit == "" {
			return gitSources{}, fmt.Errorf("%w: malformed git diff-tree record %q", ErrGitHistory, record)
		}
		i++
		path, oid := records[i], fields[3]
		if fields[4] == "D" || !strings.HasPrefix(fields[1], "100") || !isTaskYAML(path) {
			continue
		}
		if _, ok := seen[oid]; ok {
			continue
		}
		seen[oid] = blobRef{oid: oid, path: path, commit: commit}
		order = append(order, oid)
	}
	out.Blobs = len(order)
	if len(order) == 0 {
		return out, nil
	}
	blobs, err := gitCatFileBatch(ctx, featuresRepo, order)
	if err != nil {
		return gitSources{}, err
	}
	for _, oid := range order {
		data, ok := blobs[oid]
		if !ok {
			return gitSources{}, fmt.Errorf("%w: git cat-file did not return blob %s (%s)", ErrGitHistory, oid, seen[oid].path)
		}
		source := parseSnapshots(data, Revision{Source: SourceGit, Commit: seen[oid].commit})
		for i := range source.Snapshots {
			source.Snapshots[i].Repository = featuresRepo
			source.Snapshots[i].Path = seen[oid].path
		}
		out.absorb(source)
	}
	return out, nil
}

// gitCatFileBatch reads every requested blob through one child process.
func gitCatFileBatch(ctx context.Context, repo string, oids []string) (map[string][]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repo, "cat-file", "--batch")
	cmd.Stdin = strings.NewReader(strings.Join(oids, "\n") + "\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: git cat-file --batch in %s: %v: %s", ErrGitHistory, repo, err, strings.TrimSpace(stderr.String()))
	}
	out := make(map[string][]byte, len(oids))
	rest := stdout.Bytes()
	for len(rest) > 0 {
		newline := bytes.IndexByte(rest, '\n')
		if newline < 0 {
			return nil, fmt.Errorf("%w: git cat-file --batch produced a truncated header", ErrGitHistory)
		}
		header := string(rest[:newline])
		rest = rest[newline+1:]
		fields := strings.Fields(header)
		if len(fields) != 3 {
			return nil, fmt.Errorf("%w: git cat-file --batch header %q", ErrGitHistory, header)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || size > len(rest) {
			return nil, fmt.Errorf("%w: git cat-file --batch header %q", ErrGitHistory, header)
		}
		out[fields[0]] = append([]byte(nil), rest[:size]...)
		rest = rest[size:]
		if len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}
	}
	return out, nil
}

// gitLines runs git and splits its output. Records are NUL-separated when -z
// is passed and newline-separated otherwise. stderr is carried into the error
// because "exit status 128" alone names neither the repository nor the cause.
func gitLines(ctx context.Context, repo string, args ...string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: git %s in %s: %v: %s",
			ErrGitHistory, strings.Join(args, " "), repo, err, strings.TrimSpace(stderr.String()))
	}
	separator := "\n"
	for _, arg := range args {
		if arg == "-z" {
			separator = "\x00"
		}
	}
	var lines []string
	for _, line := range strings.Split(stdout.String(), separator) {
		if line = strings.Trim(line, "\n"); line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func readTargetTasks(path string) ([]yamlTask, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read target %s: %v", ErrYAMLSource, path, err)
	}
	doc, err := decodeTaskDocument(data)
	if err != nil {
		return nil, fmt.Errorf("%w: parse target %s: %v", ErrYAMLSource, path, err)
	}
	seen := make(map[string]bool)
	for i, task := range doc.Tasks {
		if strings.TrimSpace(task.Key) == "" || !task.Role.Valid() || strings.TrimSpace(task.Model) == "" {
			return nil, fmt.Errorf("%w: target %s row %d (%q) requires a key, valid role and model", ErrYAMLSource, path, i+1, task.Key)
		}
		if seen[task.Key] {
			return nil, fmt.Errorf("%w: target %s repeats key %q", ErrYAMLSource, path, task.Key)
		}
		seen[task.Key] = true
	}
	return doc.Tasks, nil
}

func decodeTaskDocument(data []byte) (yamlDocument, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return yamlDocument{}, err
	}
	if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return yamlDocument{}, fmt.Errorf("expected a mapping with a tasks sequence")
	}
	mapping := root.Content[0]
	found := false
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "tasks" {
			if mapping.Content[i+1].Kind != yaml.SequenceNode {
				return yamlDocument{}, fmt.Errorf("tasks must be a sequence")
			}
			found = true
		}
	}
	if !found {
		return yamlDocument{}, fmt.Errorf("missing tasks sequence")
	}
	var doc yamlDocument
	if err := root.Decode(&doc); err != nil {
		return yamlDocument{}, err
	}
	return doc, nil
}
