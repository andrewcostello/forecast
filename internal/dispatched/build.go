package dispatched

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// HandFinishedLimit is edge case 9: no field marks a row that a human
// finished after it blocked, so its wall clock cannot be separated from agent
// throughput. Stated in the report rather than modelled around.
const HandFinishedLimit = "Hand-finished rows have no identifying field; their wall clock cannot be distinguished from agent throughput."

// DefaultMinObservations is the number of completed observations a required
// cell needs before the report calls it covered. One completed row is a
// sample, not a distribution, and the refuse-to-predict ruling is worth
// nothing if a single row silently licenses a forecast.
const DefaultMinObservations = 2

// BuildOptions identifies the journal tree, YAML history repository and,
// optionally, the task list whose coverage should be measured.
type BuildOptions struct {
	RunsDir      string
	FeaturesRepo string
	TargetTasks  string
	Now          time.Time
	// MinObservations is the completed-observation threshold for calling a
	// required cell covered. Zero means DefaultMinObservations.
	MinObservations int
	// MaxHistoryCommits bounds the git history walk. Zero means
	// defaultMaxHistoryCommits.
	MaxHistoryCommits int
}

// BuildResult holds both the queryable table and its portable artifact.
type BuildResult struct {
	Table    *Table
	Artifact Artifact
}

type Artifact struct {
	SchemaVersion int                    `json:"schema_version"`
	GeneratedAt   time.Time              `json:"generated_at"`
	Observations  []ReferenceObservation `json:"observations"`
	Cells         []CellSummary          `json:"cells"`
	Coverage      Coverage               `json:"coverage"`
	Conflicts     []Conflict             `json:"conflicts"`
	Limits        []string               `json:"limits"`
}

// ReferenceObservation is one stored row as the artifact serialises it.
// CostUSD is null when no spawn payload recorded a cost, so a downstream mean
// can exclude the row instead of averaging in a zero nobody measured.
type ReferenceObservation struct {
	Key                string    `json:"key"`
	Role               Role      `json:"role"`
	Model              string    `json:"model"`
	Outcome            string    `json:"outcome"`
	TerminalEvidence   string    `json:"terminal_evidence"`
	Censored           bool      `json:"censored"`
	StartedAt          time.Time `json:"started_at"`
	ElapsedSeconds     float64   `json:"elapsed_seconds"`
	DevelopmentSeconds float64   `json:"development_seconds"`
	ReviewSeconds      float64   `json:"review_seconds"`
	Rounds             int       `json:"rounds"`
	Cascades           int       `json:"cascades"`
	InputTokens        int64     `json:"input_tokens"`
	OutputTokens       int64     `json:"output_tokens"`
	CostUSD            *float64  `json:"cost_usd"`
	DispatcherRunID    string    `json:"dispatcher_run_id"`
	SourceRevision     string    `json:"source_revision"`
}

type NumericSummary struct {
	N      int     `json:"n"`
	Min    float64 `json:"min"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	Max    float64 `json:"max"`
}

// CellSummary is the per-(role, model) tally. Duration summarises COMPLETED
// rows only: a censored row contributes to N, NBlocked and NCensored and to
// the rounds summary, and never to Duration.
type CellSummary struct {
	Role      Role           `json:"role"`
	Model     string         `json:"model"`
	N         int            `json:"n"`
	NDone     int            `json:"n_done"`
	NBlocked  int            `json:"n_blocked"`
	NCensored int            `json:"n_censored"`
	Duration  NumericSummary `json:"duration_seconds"`
	Rounds    NumericSummary `json:"rounds"`
}

type RequiredCell struct {
	Role       Role   `json:"role"`
	Model      string `json:"model"`
	TargetRows int    `json:"target_rows"`
	N          int    `json:"n"`
	NDone      int    `json:"n_done"`
	Empty      bool   `json:"empty"`
	Covered    bool   `json:"covered"`
}

type Coverage struct {
	TargetTasks            string         `json:"target_tasks,omitempty"`
	MinObservations        int            `json:"min_observations"`
	RequiredCells          []RequiredCell `json:"required_cells"`
	EmptyRequiredCells     []Cell         `json:"empty_required_cells"`
	UncoveredRequiredCells []Cell         `json:"uncovered_required_cells"`
	TargetRows             int            `json:"target_rows"`
	TargetRowsWithCell     int            `json:"target_rows_with_cell"`
	TargetRowsCovered      int            `json:"target_rows_covered"`
	TargetCoveredShare     *float64       `json:"target_covered_share"`

	// JournalStartedRows counts DISTINCT (run, task key) pairs that started,
	// which is the denominator a recovered row can be compared against.
	// JournalStartAttempts counts task_started events, so the two differ by
	// JournalRestarts.
	JournalStartedRows   int `json:"journal_started_rows"`
	JournalStartAttempts int `json:"journal_start_attempts"`
	JournalRestarts      int `json:"journal_restarts"`
	// RecoveredObservations counts stored rows, which is one per recovered
	// ATTEMPT; RecoveredRows counts the distinct (run, task key) pairs behind
	// them, so it is comparable with JournalStartedRows. A run that restarts
	// a task can yield two observations from one started row.
	RecoveredObservations int `json:"recovered_observations"`
	RecoveredRows         int `json:"recovered_rows"`
	RecoveryShortfall     int `json:"recovery_shortfall"`

	JournalRunTasksWithoutYAML       int `json:"journal_run_tasks_without_yaml"`
	JournalLinesUnparsed             int `json:"journal_lines_unparsed"`
	JournalEventsWithBadTimestamp    int `json:"journal_events_with_bad_timestamp"`
	TaskStartsWithoutModel           int `json:"task_starts_without_model"`
	JournalAttemptsWithoutStamp      int `json:"journal_attempts_without_stamped_model"`
	UnattributableJoinedRows         int `json:"unattributable_joined_rows"`
	UnrecoverableJoinedRows          int `json:"unrecoverable_joined_rows"`
	StampConflictRows                int `json:"stamp_conflict_rows"`
	AuthoredStampMismatches          int `json:"authored_stamp_mismatches"`
	RowsWithCascade                  int `json:"rows_with_cascade"`
	RowsWithoutRecordedCost          int `json:"rows_without_recorded_cost"`
	RowsWithYAMLOnlyTerminalEvidence int `json:"rows_with_yaml_only_terminal_evidence"`

	LiveYAMLReadings       int  `json:"live_yaml_readings"`
	HistoricalYAMLReadings int  `json:"historical_yaml_readings"`
	HistoryCommits         int  `json:"history_commits"`
	HistoryBlobs           int  `json:"history_blobs"`
	HistoryTruncated       bool `json:"history_truncated"`
	UnparseableYAMLDocs    int  `json:"unparseable_yaml_documents"`
	MalformedYAMLRows      int  `json:"malformed_yaml_rows"`
}

type Conflict struct {
	Key       string    `json:"key"`
	StartedAt time.Time `json:"started_at"`
	Cells     []Cell    `json:"cells"`
	Reason    string    `json:"reason"`
}

// Build constructs the union reference class. The stamped journal model is
// the only model used for attribution; the authored YAML model is retained
// only to count disagreements.
func Build(ctx context.Context, opts BuildOptions) (*BuildResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	if opts.MinObservations <= 0 {
		opts.MinObservations = DefaultMinObservations
	}
	journals, err := readJournals(ctx, opts.RunsDir)
	if err != nil {
		return nil, err
	}
	live, err := readLiveSnapshots(ctx, opts.FeaturesRepo)
	if err != nil {
		return nil, err
	}
	historical, err := readGitSnapshots(ctx, opts.FeaturesRepo, opts.MaxHistoryCommits)
	if err != nil {
		return nil, err
	}

	coverage := Coverage{
		TargetTasks:            opts.TargetTasks,
		MinObservations:        opts.MinObservations,
		LiveYAMLReadings:       len(live.Snapshots),
		HistoricalYAMLReadings: len(historical.Snapshots),
		HistoryCommits:         historical.Commits,
		HistoryBlobs:           historical.Blobs,
		HistoryTruncated:       historical.Truncated,
		UnparseableYAMLDocs:    live.UnparseableDocuments + historical.UnparseableDocuments,
		MalformedYAMLRows:      live.MalformedRows + historical.MalformedRows,
		JournalLinesUnparsed:   journals.LinesUnparsed,

		JournalEventsWithBadTimestamp: journals.BadTimestamps,
	}
	var target []yamlTask
	if opts.TargetTasks != "" {
		target, err = readTargetTasks(opts.TargetTasks)
		if err != nil {
			return nil, err
		}
	}
	required := make(map[Cell]int)
	coverage.TargetRows = len(target)
	for _, task := range target {
		cell := Cell{Role: task.Role, Model: task.Model}
		if !cell.Role.Valid() || cell.Model == "" {
			continue
		}
		required[cell]++
		coverage.TargetRowsWithCell++
	}
	declared := make([]Cell, 0, len(required))
	for cell := range required {
		declared = append(declared, cell)
	}
	table := NewTable(declared...)

	for _, row := range journals.Rows {
		starts := row.Starts()
		if starts > 0 {
			coverage.JournalStartedRows++
		}
		coverage.JournalStartAttempts += starts
		coverage.JournalRestarts += row.Restarts()
		coverage.TaskStartsWithoutModel += row.StartsWithoutModel()
		coverage.JournalAttemptsWithoutStamp += row.AttemptsWithoutStampedModel()
	}

	readings := make(map[identity][]Observation)
	joinedRunTasks := make(map[runTask]bool)
	unattributable := make(map[identity]bool)
	unrecoverable := make(map[identity]bool)
	mismatches := make(map[identity]bool)
	for _, snapshot := range append(append([]taskSnapshot{}, live.Snapshots...), historical.Snapshots...) {
		key := runTask{RunID: snapshot.DispatcherRunID, Key: snapshot.Key}
		row, ok := journals.Rows[key]
		if !ok {
			continue
		}
		joinedRunTasks[key] = true
		facts := row.Match(snapshot.StartedAt)
		if facts == nil {
			facts = &JournalFacts{}
		}
		id := identity{Key: snapshot.Key, StartedAt: snapshot.StartedAt.UTC()}
		observation, err := observationFrom(snapshot, facts, opts.Now)
		if err != nil {
			// Only the two documented degraded classes are tolerated, and only
			// as a classification; anything else is a defect in this package
			// and must not be swallowed because some other revision happened to
			// parse.
			switch {
			case errors.Is(err, ErrUnattributable):
				if len(readings[id]) == 0 && !unrecoverable[id] {
					unattributable[id] = true
				}
			case isUnrecoverableObservationError(err):
				if len(readings[id]) == 0 {
					unrecoverable[id] = true
					delete(unattributable, id)
				}
			default:
				return nil, err
			}
			continue
		}
		delete(unattributable, id)
		delete(unrecoverable, id)
		if snapshot.AuthoredModel != "" && snapshot.AuthoredModel != facts.Model {
			mismatches[id] = true
		}
		readings[id] = append(readings[id], observation)
	}
	coverage.UnattributableJoinedRows = len(unattributable)
	coverage.UnrecoverableJoinedRows = len(unrecoverable)
	coverage.AuthoredStampMismatches = len(mismatches)
	for key, row := range journals.Rows {
		if row.Starts() > 0 && !joinedRunTasks[key] {
			coverage.JournalRunTasksWithoutYAML++
		}
	}

	ids := make([]identity, 0, len(readings))
	for id := range readings {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if ids[i].StartedAt.Equal(ids[j].StartedAt) {
			return ids[i].Key < ids[j].Key
		}
		return ids[i].StartedAt.Before(ids[j].StartedAt)
	})
	var conflicts []Conflict
	recoveredRuns := make(map[runTask]bool)
	for _, id := range ids {
		rows := readings[id]
		// Merge every reading of the identity BEFORE touching the table, so a
		// conflict discovered on the last reading cannot leave the first ones
		// stored in an artifact that also reports them as excluded.
		joined, err := joinReadings(rows)
		if err != nil {
			conflicts = append(conflicts, Conflict{
				Key: id.Key, StartedAt: id.StartedAt, Cells: distinctCells(rows), Reason: err.Error(),
			})
			continue
		}
		if err := table.Add(joined); err != nil {
			return nil, err
		}
		coverage.RecoveredObservations++
		recoveredRuns[runTask{RunID: joined.Provenance.RunID, Key: joined.Key}] = true
	}
	coverage.RecoveredRows = len(recoveredRuns)
	coverage.StampConflictRows = len(conflicts)
	coverage.RecoveryShortfall = max(coverage.JournalStartedRows-coverage.RecoveredRows, 0)

	observations := flattenObservations(table)
	for _, row := range observations {
		if row.Cascades > 0 {
			coverage.RowsWithCascade++
		}
		if row.CostUSD == nil {
			coverage.RowsWithoutRecordedCost++
		}
		if row.TerminalEvidence == terminalEvidenceYAML {
			coverage.RowsWithYAMLOnlyTerminalEvidence++
		}
	}

	cells := summarizeCells(table)
	coverage.finishTarget(required, cells, opts.MinObservations)
	artifact := Artifact{
		SchemaVersion: 2,
		GeneratedAt:   opts.Now.UTC(),
		Observations:  observations,
		Cells:         cells,
		Coverage:      coverage,
		Conflicts:     conflicts,
		Limits:        []string{HandFinishedLimit},
	}
	return &BuildResult{Table: table, Artifact: artifact}, nil
}

// joinReadings folds every reading of one identity into a single row using
// the same rules Table.Add applies, and reports the conflict rather than
// picking a winner. It is total: the fold is over a commutative operation, so
// the order readings arrive in does not change the result.
func joinReadings(rows []Observation) (Observation, error) {
	joined := rows[0]
	for _, row := range rows[1:] {
		next, err := merge(joined, row)
		if err != nil {
			return Observation{}, err
		}
		joined = next
	}
	return joined, nil
}

// Terminal evidence values. A journal terminal event is the dispatcher's own
// record; a YAML status is a mutable file a human may have edited, and edge
// case 9 says a hand-finished row is indistinguishable from an agent one.
const (
	terminalEvidenceJournal = "journal"
	terminalEvidenceYAML    = "yaml"
	terminalEvidenceNone    = "none"
)

// observationFrom joins one YAML snapshot to the journal attempt it names.
//
// The terminal event, when the journal has one, decides both the outcome and
// the instant elapsed time is measured to. A YAML completed_at is used ONLY
// when the YAML status is itself terminal: a row still marked in progress
// carrying a stale completed_at is censored, and its lower bound runs to now,
// not back to a timestamp from a previous attempt.
func observationFrom(snapshot taskSnapshot, facts *JournalFacts, now time.Time) (Observation, error) {
	if !snapshot.Role.Valid() || facts.Model == "" {
		return Observation{}, fmt.Errorf("%w: row %s has role %q and stamped model %q", ErrUnattributable, snapshot.Key, snapshot.Role, facts.Model)
	}
	outcome, end, evidence := OutcomeUnfinished, now, terminalEvidenceNone
	if facts.TerminalOutcome.Valid() {
		outcome, end, evidence = facts.TerminalOutcome, facts.TerminalAt, terminalEvidenceJournal
	} else if yamlOutcome, ok := terminalStatus(snapshot.Status); ok && !snapshot.CompletedAt.IsZero() {
		outcome, end, evidence = yamlOutcome, snapshot.CompletedAt, terminalEvidenceYAML
	}
	if end.Before(snapshot.StartedAt) {
		return Observation{}, fmt.Errorf("build observation: %w: row %s ends at %s before it starts at %s", ErrNegativeValue, snapshot.Key, end.Format(time.RFC3339Nano), snapshot.StartedAt.Format(time.RFC3339Nano))
	}
	observation := Observation{
		Key:              snapshot.Key,
		Cell:             Cell{Role: snapshot.Role, Model: facts.Model},
		Outcome:          outcome,
		TerminalEvidence: evidence,
		StartedAt:        snapshot.StartedAt,
		Elapsed:          end.Sub(snapshot.StartedAt),
		DevElapsed:       facts.DevElapsed,
		ReviewElapsed:    facts.ReviewElapsed,
		Rounds:           max(snapshot.IterationCount, facts.Rounds),
		Cascades:         facts.Fallbacks,
		InputTokens:      facts.InputTokens,
		OutputTokens:     facts.OutputTokens,
		CostUSD:          facts.CostUSD,
		CostKnown:        facts.CostKnown,
		Provenance:       Provenance{RunID: snapshot.DispatcherRunID, Revision: snapshot.Revision},
	}
	if err := observation.Validate(); err != nil {
		return Observation{}, fmt.Errorf("build observation: %w", err)
	}
	return observation, nil
}

func terminalStatus(status string) (Outcome, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done":
		return OutcomeDone, true
	case "blocked":
		return OutcomeBlocked, true
	}
	return 0, false
}

func isUnrecoverableObservationError(err error) bool {
	return errors.Is(err, ErrNegativeValue) ||
		errors.Is(err, ErrInvalidOutcome) ||
		errors.Is(err, ErrUnparseableRevision)
}

func distinctCells(rows []Observation) []Cell {
	set := make(map[Cell]bool)
	for _, row := range rows {
		set[row.Cell] = true
	}
	out := make([]Cell, 0, len(set))
	for cell := range set {
		out = append(out, cell)
	}
	sortCells(out)
	return out
}

func sortCells(cells []Cell) {
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].Role != cells[j].Role {
			return cells[i].Role < cells[j].Role
		}
		return cells[i].Model < cells[j].Model
	})
}

// summarizeCells builds the per-cell tally. Duration is fed ONLY by
// Observation.Duration, which refuses to return a value for a censored row —
// the single place the right-censoring ruling is enforced for statistics.
func summarizeCells(table *Table) []CellSummary {
	cells := table.Cells()
	out := make([]CellSummary, 0, len(cells))
	for _, cell := range cells {
		observations := table.Observations(cell)
		var durations, rounds []float64
		var done, blocked int
		for _, observation := range observations {
			if duration, ok := observation.Duration(); ok {
				done++
				durations = append(durations, duration.Seconds())
			}
			if observation.Outcome == OutcomeBlocked {
				blocked++
			}
			rounds = append(rounds, float64(observation.Rounds))
		}
		out = append(out, CellSummary{
			Role: cell.Role, Model: cell.Model, N: len(observations),
			NDone: done, NBlocked: blocked, NCensored: len(observations) - done,
			Duration: summarize(durations), Rounds: summarize(rounds),
		})
	}
	return out
}

func summarize(values []float64) NumericSummary {
	if len(values) == 0 {
		return NumericSummary{}
	}
	sorted := append([]float64{}, values...)
	sort.Float64s(sorted)
	var sum float64
	for _, value := range sorted {
		sum += value
	}
	middle := len(sorted) / 2
	median := sorted[middle]
	if len(sorted)%2 == 0 {
		median = (sorted[middle-1] + sorted[middle]) / 2
	}
	return NumericSummary{N: len(sorted), Min: sorted[0], Mean: sum / float64(len(sorted)), Median: median, Max: sorted[len(sorted)-1]}
}

func flattenObservations(table *Table) []ReferenceObservation {
	var out []ReferenceObservation
	for _, cell := range table.Cells() {
		observations := table.Observations(cell)
		sort.Slice(observations, func(i, j int) bool {
			if observations[i].StartedAt.Equal(observations[j].StartedAt) {
				return observations[i].Key < observations[j].Key
			}
			return observations[i].StartedAt.Before(observations[j].StartedAt)
		})
		for _, observation := range observations {
			var cost *float64
			if observation.CostKnown {
				value := observation.CostUSD
				cost = &value
			}
			evidence := observation.TerminalEvidence
			if evidence == "" {
				evidence = terminalEvidenceNone
			}
			out = append(out, ReferenceObservation{
				Key: observation.Key, Role: cell.Role, Model: cell.Model,
				Outcome: observation.Outcome.String(), TerminalEvidence: evidence,
				Censored:  observation.Censored(),
				StartedAt: observation.StartedAt, ElapsedSeconds: observation.Elapsed.Seconds(),
				DevelopmentSeconds: observation.DevElapsed.Seconds(), ReviewSeconds: observation.ReviewElapsed.Seconds(),
				Rounds: observation.Rounds, Cascades: observation.Cascades,
				InputTokens: observation.InputTokens, OutputTokens: observation.OutputTokens,
				CostUSD: cost, DispatcherRunID: observation.Provenance.RunID,
				SourceRevision: observation.Provenance.Revision.String(),
			})
		}
	}
	return out
}

// finishTarget reports which required cells the class can and cannot speak
// for. A cell is covered only when it holds at least minObservations
// COMPLETED rows: a cell of censored rows knows a lower bound and no
// duration, and a cell of one row is a sample.
func (c *Coverage) finishTarget(required map[Cell]int, cells []CellSummary, minObservations int) {
	byCell := make(map[Cell]CellSummary, len(cells))
	for _, summary := range cells {
		byCell[Cell{Role: summary.Role, Model: summary.Model}] = summary
	}
	ordered := make([]Cell, 0, len(required))
	for cell := range required {
		ordered = append(ordered, cell)
	}
	sortCells(ordered)
	for _, cell := range ordered {
		summary := byCell[cell]
		item := RequiredCell{
			Role: cell.Role, Model: cell.Model, TargetRows: required[cell],
			N: summary.N, NDone: summary.NDone,
			Empty:   summary.N == 0,
			Covered: summary.NDone >= minObservations,
		}
		c.RequiredCells = append(c.RequiredCells, item)
		if item.Empty {
			c.EmptyRequiredCells = append(c.EmptyRequiredCells, cell)
		}
		if !item.Covered {
			c.UncoveredRequiredCells = append(c.UncoveredRequiredCells, cell)
		} else {
			c.TargetRowsCovered += item.TargetRows
		}
	}
	if c.TargetRows > 0 {
		share := float64(c.TargetRowsCovered) / float64(c.TargetRows)
		c.TargetCoveredShare = &share
	}
}

// WriteArtifact atomically replaces path with indented JSON.
func WriteArtifact(path string, artifact Artifact) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("%w: create parent for %s: %v", ErrReferenceOutput, path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dispatched-reference-*.tmp")
	if err != nil {
		return fmt.Errorf("%w: create temporary output for %s: %v", ErrReferenceOutput, path, err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(artifact); err != nil {
		return fmt.Errorf("%w: encode %s: %v", ErrReferenceOutput, path, err)
	}
	// CreateTemp makes the file 0600; a shared analysis artifact is read by
	// other accounts and CI steps.
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("%w: chmod temporary output for %s: %v", ErrReferenceOutput, path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%w: close temporary output for %s: %v", ErrReferenceOutput, path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("%w: replace %s: %v", ErrReferenceOutput, path, err)
	}
	ok = true
	return nil
}

// WriteCoverage writes the human-readable coverage report delivered by FC-1.
// Every shortfall is a printed number: a reader must be able to see what the
// class does not cover without opening the JSON.
func WriteCoverage(w io.Writer, artifact Artifact) {
	fmt.Fprintln(w, "Dispatched reference-class coverage")
	fmt.Fprintln(w, "role/model\tn\tn_done\tn_blocked\tn_censored\tduration mean/median/min/max\trounds mean/min/max")
	for _, cell := range artifact.Cells {
		fmt.Fprintf(w, "%s/%s\t%d\t%d\t%d\t%d\t%s\t%s\n",
			cell.Role, cell.Model, cell.N, cell.NDone, cell.NBlocked, cell.NCensored,
			formatDurationSummary(cell.Duration), formatRoundsSummary(cell.Rounds))
	}
	c := artifact.Coverage
	if c.TargetTasks != "" {
		fmt.Fprintf(w, "\nTarget tasks: %s (a cell is covered at n_done >= %d)\n", c.TargetTasks, c.MinObservations)
		for _, cell := range c.RequiredCells {
			fmt.Fprintf(w, "  required %s/%s: target_rows=%d n=%d n_done=%d empty=%t covered=%t\n", cell.Role, cell.Model, cell.TargetRows, cell.N, cell.NDone, cell.Empty, cell.Covered)
		}
		fmt.Fprintf(w, "Required cells empty: %d; required cells not covered: %d\n", len(c.EmptyRequiredCells), len(c.UncoveredRequiredCells))
		// A target row with no role or no model cannot be asked about at all,
		// which is a wider hole than an empty cell and belongs in the same
		// paragraph rather than only in the JSON.
		fmt.Fprintf(w, "Target rows naming a (role, model) cell: %d/%d\n", c.TargetRowsWithCell, c.TargetRows)
		if c.TargetCoveredShare != nil {
			fmt.Fprintf(w, "Target rows in a covered cell: %d/%d (%.1f%%)\n", c.TargetRowsCovered, c.TargetRows, *c.TargetCoveredShare*100)
		}
	}
	fmt.Fprintf(w, "Rows recovered vs journal starts: %d/%d; recovery shortfall=%d\n", c.RecoveredRows, c.JournalStartedRows, c.RecoveryShortfall)
	fmt.Fprintf(w, "Observations stored (one per recovered attempt): %d\n", c.RecoveredObservations)
	fmt.Fprintf(w, "task_started events: %d across %d rows (%d restarts)\n", c.JournalStartAttempts, c.JournalStartedRows, c.JournalRestarts)
	fmt.Fprintf(w, "task_started events carrying no planned model: %d\n", c.TaskStartsWithoutModel)
	fmt.Fprintf(w, "Attempts no implementing spawn stamped a model on: %d\n", c.JournalAttemptsWithoutStamp)
	fmt.Fprintf(w, "Joined rows still unattributable: %d\n", c.UnattributableJoinedRows)
	fmt.Fprintf(w, "Joined rows unrecoverable after validation: %d\n", c.UnrecoverableJoinedRows)
	fmt.Fprintf(w, "Journal run/task keys with no recoverable YAML: %d\n", c.JournalRunTasksWithoutYAML)
	fmt.Fprintf(w, "Journal lines unparsed: %d; events with an unreadable timestamp: %d\n", c.JournalLinesUnparsed, c.JournalEventsWithBadTimestamp)
	fmt.Fprintf(w, "Authored/stamped model disagreements (attributed to stamp): %d\n", c.AuthoredStampMismatches)
	fmt.Fprintf(w, "Recovered rows that cascaded to another agent: %d\n", c.RowsWithCascade)
	fmt.Fprintf(w, "Recovered rows with no recorded cost: %d\n", c.RowsWithoutRecordedCost)
	fmt.Fprintf(w, "Recovered rows whose terminal evidence is YAML only: %d\n", c.RowsWithYAMLOnlyTerminalEvidence)
	fmt.Fprintf(w, "Stamp conflicts excluded: %d\n", c.StampConflictRows)
	fmt.Fprintf(w, "YAML readings: live=%d historical=%d over %d commits / %d blobs (truncated=%t)\n",
		c.LiveYAMLReadings, c.HistoricalYAMLReadings, c.HistoryCommits, c.HistoryBlobs, c.HistoryTruncated)
	fmt.Fprintf(w, "YAML documents that would not parse: %d; rows with an unreadable timestamp: %d\n", c.UnparseableYAMLDocs, c.MalformedYAMLRows)
	for _, limit := range artifact.Limits {
		fmt.Fprintf(w, "Limit: %s\n", limit)
	}
}

func formatDurationSummary(s NumericSummary) string {
	if s.N == 0 {
		return "n=0"
	}
	return fmt.Sprintf("%.0fs/%.0fs/%.0fs/%.0fs", s.Mean, s.Median, s.Min, s.Max)
}

func formatRoundsSummary(s NumericSummary) string {
	if s.N == 0 {
		return "n=0"
	}
	return fmt.Sprintf("%.2f/%.0f/%.0f", s.Mean, s.Min, s.Max)
}
