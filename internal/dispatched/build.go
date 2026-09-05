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
	RunsDir       string
	FeaturesRepo  string // Compatibility for callers supplying one repository.
	FeaturesRepos []string
	TargetTasks   string
	Now           time.Time
	// MinObservations is the completed-observation threshold for calling a
	// required cell covered. Zero means DefaultMinObservations.
	MinObservations int
	// MaxHistoryCommits bounds the git history walk. Zero means
	// defaultMaxHistoryCommits.
	MaxHistoryCommits int

	// Amended contract inputs (F3/F6), frozen here for FC-1. Sources are the
	// explicit source specifications that replace RunsDir/FeaturesRepos;
	// Selection freezes holdout run IDs, cutoff and allow-empty; Bounds are
	// the byte/commit/process caps. Until FC-1 wires ReadSources and
	// JoinEvidence into Build, setting any of them makes Build return
	// ErrNotImplemented rather than silently ignoring an input; the legacy
	// shape (nil Sources, zero cutoff, nil holdouts, false AllowEmpty and zero
	// Bounds) runs the baseline unchanged.
	Sources   []SourceSpec
	Selection Selection
	Bounds    ReadBounds
}

// amended reports whether the options use the F3/F6 inputs.
func (o BuildOptions) amended() bool {
	return o.Sources != nil || !o.Selection.Cutoff.IsZero() || o.Selection.HoldoutRunIDs != nil ||
		o.Selection.AllowEmpty || o.Bounds != (ReadBounds{})
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
	// SourceManifest (F3) names every source the artifact rests on and its
	// completeness. Nil from the baseline path; FC-1 populates it and bumps
	// SchemaVersion when it does.
	SourceManifest *SourceManifest   `json:"source_manifest,omitempty"`
	Evidence       *ArtifactEvidence `json:"evidence,omitempty"`
}

// EvidenceSchemaVersion is emitted only once FC-1 implements the amended build.
// Legacy version 3 retains its fields. Version 4 requires SourceManifest and Evidence;
// consumers must reject missing payloads or unsupported versions, not default
// their absence to zero. Legacy Observations/Cells are compatibility projections
// only; amended sampling uses Evidence.Observations exclusively.
const EvidenceSchemaVersion = 4

// BaselineSchemaVersion is the current legacy writer version. Only the amended
// FC-1 Build may emit EvidenceSchemaVersion after filling both required payloads.
const BaselineSchemaVersion = 3

// ArtifactEvidence is the complete serializable join audit and joint sample.
// A nil payload means unavailable (legacy version 3); all counters inside a version 4
// payload are present, including zero. Durations on Attempt use nanoseconds.
// FC-1 initializes every list in version 4 evidence (including nested lists) to
// [] rather than null. Canonical JSON-value equality includes that distinction.
// Attempt outcome and terminal conflict outcome are done/blocked/unfinished text.
// Compatibility projections map amended EvidenceNone="" to legacy "none",
// leaving "yaml" and "journal" unchanged; amended sampling never reads projections.
// Reading revisions use stable strings; Cell retains legacy Role/Model JSON keys.
// Limits must disclose recorded_task_spawns cost scope, excluded cache tokens,
// and that unrecorded reviewer/operator spend is not total-process cost.
type ArtifactEvidence struct {
	StartsAfterCutoff int                `json:"starts_after_cutoff"`
	Observations      []RecoveredAttempt `json:"observations"`
	Examined          []Examined         `json:"examined"`
	Dispositions      []DispositionCount `json:"dispositions"`
	Conflicts         []AttemptConflict  `json:"conflicts"`
	Ambiguous         []AmbiguousAttempt `json:"ambiguous"`
	UniqueRows        int                `json:"unique_rows"`
	Attempts          int                `json:"attempts"`
	Recovered         int                `json:"recovered"`
	LostAttempts      []AttemptID        `json:"lost_attempts"`
	ExcludedJournals  []JournalIdentity  `json:"excluded_journals"`
}

// Eligibility is the F4 prediction gate result. Eligible is true only when
// the manifest is COMPLETE, the target has at least one row, every required
// cell is valid, and every required cell holds at least MinCompleted
// completed samples. Reasons lists every failed condition; MinCompleted is
// reported as a threshold, not as proof of calibration.
type Eligibility struct {
	Eligible     bool     `json:"eligible"`
	MinCompleted int      `json:"min_completed"`
	Reasons      []string `json:"reasons,omitempty"`
}

// TargetRow preserves each target identity and cell before coverage aggregation.
// Callers pass original rows, including invalid ones, so validation cannot hide
// blank/duplicate keys or roles/models that aggregation would discard.
type TargetRow struct {
	Key   string `json:"key"`
	Role  Role   `json:"role"`
	Model string `json:"model"`
}

// PredictionEligibility requires SchemaVersion == EvidenceSchemaVersion exactly,
// nonnil Evidence, a valid complete SourceManifest, a nonempty valid target
// argument and sufficient completed
// samples in every required cell. minCompleted<=0 uses DefaultMinObservations;
// Eligibility.MinCompleted records the effective positive threshold.
// Invalid schema/payload or incomplete sources yield Eligible=false and a
// reason; when refuse is true their error wraps BOTH ErrNotEligible and
// ErrSourceIncomplete. A thin cell wraps ErrNotEligible when refusing. Zero-row
// targets always wrap ErrEmptyTarget; malformed targets wrap ErrInvalidTarget.
// With refuse=false, ordinary insufficiency is a diagnostic result, not an
// error. The scaffold returns ErrNotImplemented regardless of inputs.
//
// Validate target rows first: empty -> ErrEmptyTarget, then declaration-order
// invalid/duplicate keys, invalid roles or blank models -> ErrInvalidTarget.
// Compute completed counts from valid Evidence.Observations, not legacy Cells/Coverage.
// Malformed joint records or cutoff/cell/model contradictions make the evidence
// payload invalid; refuse with ErrSourceIncomplete and ErrNotEligible as above.
// FC-1 body; this scaffold returns ErrNotImplemented.
func PredictionEligibility(artifact Artifact, target []TargetRow, minCompleted int, refuse bool) (Eligibility, error) {
	return Eligibility{}, fmt.Errorf("%w: PredictionEligibility(min %d)", ErrNotImplemented, minCompleted)
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
	SourceRepository   string    `json:"source_repository"`
	SourcePath         string    `json:"source_path"`
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

type RepositoryCoverage struct {
	Repository         string `json:"repository"`
	LiveReadings       int    `json:"live_readings"`
	HistoricalReadings int    `json:"historical_readings"`
	MatchedAttempts    int    `json:"matched_attempts"`
}

type Coverage struct {
	Repositories                    []RepositoryCoverage `json:"repositories"`
	YAMLRowsMissingJoinKeys         int                  `json:"yaml_rows_missing_join_keys"`
	SnapshotsWithoutMatchingAttempt int                  `json:"snapshots_without_matching_attempt"`
	AttemptsWithoutMatchingYAML     int                  `json:"attempts_without_matching_yaml"`
	RecoveredAttempts               int                  `json:"recovered_attempts"`
	AttemptRecoveryShortfall        int                  `json:"attempt_recovery_shortfall"`
	TargetTasks                     string               `json:"target_tasks,omitempty"`
	MinObservations                 int                  `json:"min_observations"`
	RequiredCells                   []RequiredCell       `json:"required_cells"`
	EmptyRequiredCells              []Cell               `json:"empty_required_cells"`
	UncoveredRequiredCells          []Cell               `json:"uncovered_required_cells"`
	TargetRows                      int                  `json:"target_rows"`
	TargetRowsWithCell              int                  `json:"target_rows_with_cell"`
	TargetRowsCovered               int                  `json:"target_rows_covered"`
	TargetCoveredShare              *float64             `json:"target_covered_share"`

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

// The amended FC-1 Build is frozen in notes/FC-SCAFFOLD.md "Entry-point
// contracts" (Build / serialization) and F4-MIXED-OPTIONS.

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
	if opts.amended() {
		// Fail loudly instead of running the baseline over inputs it would
		// ignore: an unapplied holdout or cutoff would leak evidence.
		return nil, fmt.Errorf("%w: Build with Sources, Selection or Bounds", ErrNotImplemented)
	}
	journals, err := readJournals(ctx, opts.RunsDir)
	if err != nil {
		return nil, err
	}
	coverage := Coverage{
		TargetTasks: opts.TargetTasks, MinObservations: opts.MinObservations,
		JournalLinesUnparsed: journals.LinesUnparsed, JournalEventsWithBadTimestamp: journals.BadTimestamps,
	}
	repos := append([]string{}, opts.FeaturesRepos...)
	if opts.FeaturesRepo != "" {
		repos = append(repos, opts.FeaturesRepo)
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("%w: at least one features repository is required", ErrYAMLSource)
	}
	var snapshots []taskSnapshot
	seenRepos := make(map[string]bool)
	for _, repo := range repos {
		if strings.TrimSpace(repo) == "" {
			return nil, fmt.Errorf("%w: empty features repository", ErrYAMLSource)
		}
		repo, err = filepath.Abs(repo)
		if err != nil {
			return nil, fmt.Errorf("%w: repository path: %v", ErrYAMLSource, err)
		}
		repo, err = filepath.EvalSymlinks(repo)
		if err != nil {
			return nil, fmt.Errorf("%w: repository path: %v", ErrYAMLSource, err)
		}
		if seenRepos[repo] {
			continue
		}
		seenRepos[repo] = true
		live, err := readLiveSnapshots(ctx, repo)
		if err != nil {
			return nil, err
		}
		historical, err := readGitSnapshots(ctx, repo, opts.MaxHistoryCommits)
		if err != nil {
			return nil, err
		}
		coverage.Repositories = append(coverage.Repositories, RepositoryCoverage{Repository: repo, LiveReadings: len(live.Snapshots), HistoricalReadings: len(historical.Snapshots)})
		coverage.LiveYAMLReadings += len(live.Snapshots)
		coverage.HistoricalYAMLReadings += len(historical.Snapshots)
		coverage.HistoryCommits += historical.Commits
		coverage.HistoryBlobs += historical.Blobs
		coverage.HistoryTruncated = coverage.HistoryTruncated || historical.Truncated
		coverage.UnparseableYAMLDocs += live.UnparseableDocuments + historical.UnparseableDocuments
		coverage.MalformedYAMLRows += live.MalformedRows + historical.MalformedRows
		coverage.YAMLRowsMissingJoinKeys += live.MissingJoinKeys + historical.MissingJoinKeys
		snapshots = append(snapshots, live.Snapshots...)
		snapshots = append(snapshots, historical.Snapshots...)
	}
	sort.Slice(coverage.Repositories, func(i, j int) bool { return coverage.Repositories[i].Repository < coverage.Repositories[j].Repository })
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
	readingAttempts := make(map[identity]map[*JournalFacts]runTask)
	matchedAttempts := make(map[*JournalFacts]bool)
	repoMatches := make(map[string]map[*JournalFacts]bool)
	joinedRunTasks := make(map[runTask]bool)
	unattributable := make(map[identity]bool)
	unrecoverable := make(map[identity]bool)
	mismatches := make(map[identity]bool)
	for _, snapshot := range snapshots {
		key := runTask{RunID: snapshot.DispatcherRunID, Key: snapshot.Key}
		row, ok := journals.Rows[key]
		if !ok {
			continue
		}
		joinedRunTasks[key] = true
		facts := row.Match(snapshot.StartedAt)
		if facts == nil {
			coverage.SnapshotsWithoutMatchingAttempt++
			continue
		}
		matchedAttempts[facts] = true
		if repoMatches[snapshot.Repository] == nil {
			repoMatches[snapshot.Repository] = make(map[*JournalFacts]bool)
		}
		repoMatches[snapshot.Repository][facts] = true
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
		if readingAttempts[id] == nil {
			readingAttempts[id] = make(map[*JournalFacts]runTask)
		}
		readingAttempts[id][facts] = key
	}
	for i := range coverage.Repositories {
		coverage.Repositories[i].MatchedAttempts = len(repoMatches[coverage.Repositories[i].Repository])
	}
	for _, row := range journals.Rows {
		for _, facts := range row.Attempts {
			if facts.Started > 0 && !matchedAttempts[facts] {
				coverage.AttemptsWithoutMatchingYAML++
			}
		}
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
	recoveredAttempts := make(map[*JournalFacts]bool)
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
		for facts, key := range readingAttempts[id] {
			recoveredRuns[key] = true
			recoveredAttempts[facts] = true
		}
	}
	coverage.RecoveredRows = len(recoveredRuns)
	coverage.RecoveredAttempts = len(recoveredAttempts)
	coverage.AttemptRecoveryShortfall = coverage.JournalStartAttempts - coverage.RecoveredAttempts
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
		SchemaVersion: BaselineSchemaVersion,
		GeneratedAt:   opts.Now.UTC(),
		Observations:  observations,
		Cells:         cells,
		Coverage:      coverage,
		Conflicts:     conflicts,
		Limits:        []string{HandFinishedLimit},
	}
	return &BuildResult{Table: table, Artifact: artifact}, nil
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
				SourceRevision:   observation.Provenance.Revision.String(),
				SourceRepository: observation.Provenance.Repository, SourcePath: observation.Provenance.Path,
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
	for _, repo := range c.Repositories {
		fmt.Fprintf(w, "Source repository %s: live=%d historical=%d matched_attempts=%d\n", repo.Repository, repo.LiveReadings, repo.HistoricalReadings, repo.MatchedAttempts)
	}
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
	fmt.Fprintf(w, "Attempts recovered vs journal starts: %d/%d; attempt shortfall=%d; attempts without matching YAML=%d\n", c.RecoveredAttempts, c.JournalStartAttempts, c.AttemptRecoveryShortfall, c.AttemptsWithoutMatchingYAML)
	fmt.Fprintf(w, "YAML snapshots without an exact unambiguous attempt: %d; rows missing join keys: %d\n", c.SnapshotsWithoutMatchingAttempt, c.YAMLRowsMissingJoinKeys)
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
