package dispatched

import "errors"

// Sentinels. Errors from this package and from implementations of its
// interfaces must wrap one of these with %w so callers classify with errors.Is.
//
// The first block is the FC-1 baseline. The second block is the F1–F4
// amendment (docs/plans/forecasting-contracts.md); every refusal named in the
// behavior table of features/dispatched-forecasting/notes/FC-SCAFFOLD.md
// carries a documented sentinel (some refusals wrap more than one).
var (
	// ErrUnattributable: the row cannot be placed in a (role, model) cell.
	ErrUnattributable = errors.New("dispatched: row cannot be attributed to a cell")

	// ErrUnparseableRevision: a source revision is not in a recognised form.
	ErrUnparseableRevision = errors.New("dispatched: source revision cannot be parsed")

	// ErrInvalidOutcome: an Outcome value outside the declared constants.
	ErrInvalidOutcome = errors.New("dispatched: outcome is not a declared value")

	// ErrNegativeValue: a duration, count or token total below zero, or a
	// cost that is not finite (negative, NaN or infinite).
	ErrNegativeValue = errors.New("dispatched: value must not be negative")

	// ErrStampConflict: one (Key, StartedAt) observed with two different
	// cells, or with two different terminal outcomes. Both mean a re-run
	// reused a key; neither may be resolved by last-write-wins.
	//
	// Baseline name. The amended contract raises ErrEvidenceConflict for the
	// same situation scoped to one AttemptID; Table.Add keeps this sentinel
	// on the legacy path; amended reconciliation uses RecoveredAttempt.
	ErrStampConflict = errors.New("dispatched: same row observed with different stamps")

	// ErrCycle: the dependency graph is not acyclic.
	ErrCycle = errors.New("dispatched: dependency graph has a cycle")

	// ErrUnknownDependency: a node is blocked by a key that is not in the graph.
	ErrUnknownDependency = errors.New("dispatched: dependency refers to an unknown node")

	// ErrDuplicateKey: two nodes in one graph share a key.
	ErrDuplicateKey = errors.New("dispatched: duplicate node key")

	// ErrInvalidConcurrency: a concurrency cap below one.
	ErrInvalidConcurrency = errors.New("dispatched: concurrency cap must be at least one")

	// ErrJournalSource: a dispatcher journal cannot be discovered or read.
	ErrJournalSource = errors.New("dispatched: journal source cannot be read")

	// ErrYAMLSource: a tasks YAML source cannot be discovered or parsed.
	ErrYAMLSource = errors.New("dispatched: tasks YAML source cannot be read")

	// ErrGitHistory: historical tasks YAML cannot be enumerated or read.
	ErrGitHistory = errors.New("dispatched: git history cannot be read")

	// ErrReferenceOutput: a reference-class artifact cannot be written.
	ErrReferenceOutput = errors.New("dispatched: reference output cannot be written")
)

// F1–F4 amendment sentinels.
var (
	// ErrNotImplemented: a frozen contract seam whose body has not landed.
	// Only scaffold holes return it; a body replaces every occurrence in the
	// file it owns. It is never a legitimate runtime outcome of a finished
	// unit.
	ErrNotImplemented = errors.New("dispatched: contract body not implemented")

	// ErrAmbiguousAttempt (F1): two or more journal task_started events carry
	// an identical AttemptID. Neither is chosen; the attempt is excluded and
	// counted under DispositionAmbiguousStart.
	ErrAmbiguousAttempt = errors.New("dispatched: attempt identity is ambiguous")

	// ErrEvidenceConflict (F1): within one AttemptID, two authoritative
	// readings disagree on a measurement or a terminal selection unit.
	// Precedence cannot resolve it because both sides rank equally.
	ErrEvidenceConflict = errors.New("dispatched: conflicting authoritative evidence within one attempt")

	// ErrUnknownMeasurement (F2): a Measured value was read as if known.
	ErrUnknownMeasurement = errors.New("dispatched: measurement is unknown")

	// ErrInvalidPhase (F2): an undeclared phase or unclassified interval.
	ErrInvalidPhase = errors.New("dispatched: invalid classified phase")

	// ErrReversedInterval (F2): an interval or attempt whose end precedes its
	// start, or a terminal event before the task_started it closes.
	ErrReversedInterval = errors.New("dispatched: interval ends before it starts")

	// ErrOverlappingIntervals (F2): classified wall intervals overlap, fall
	// outside the attempt, or sum to more than elapsed.
	ErrOverlappingIntervals = errors.New("dispatched: classified intervals overlap or exceed elapsed")

	// ErrInvalidSourceSpec (F3): a source has no identity, a blank root or
	// repository, an undeclared kind, or a duplicate identity.
	ErrInvalidSourceSpec = errors.New("dispatched: source specification is invalid")

	// ErrSourceMissing (F3): a requested repository, root or runs directory
	// does not exist or cannot be read. Missing is an error by default.
	ErrSourceMissing = errors.New("dispatched: requested source is missing or unreadable")

	// ErrSourceEmpty (F3): discovery succeeded and found zero journals,
	// and Selection.AllowEmpty was not set. Zero task readings alone is valid.
	ErrSourceEmpty = errors.New("dispatched: source discovered no records")

	// ErrSourceIncomplete (F3/F4): a PARTIAL or EMPTY source manifest reached
	// a gate that requires complete selected sources.
	ErrSourceIncomplete = errors.New("dispatched: selected sources are not complete")

	// ErrShallowHistory (F3): the repository's history is shallow, grafted or
	// replaced, so reachable history cannot be enumerated in full.
	ErrShallowHistory = errors.New("dispatched: git history is shallow, grafted or replaced")

	// ErrBoundExceeded (F3): a commit, byte or line bound stopped a read
	// before the source was exhausted. ReadBounds.MaxProcesses never raises
	// it: the process cap serialises git children, it does not stop reads.
	ErrBoundExceeded = errors.New("dispatched: read bound exceeded")

	// ErrSourceCancelled (F3): the context ended while a source was being
	// read. Wraps the context error as well so errors.Is(err,
	// context.Canceled) still holds.
	ErrSourceCancelled = errors.New("dispatched: source read cancelled")

	// ErrInvalidSelection (F3/F6): a cutoff or holdout selection is malformed:
	// a blank or whitespace-padded held-out run ID, a duplicate
	// (Selection.Validate), or a holdout that names no discovered run
	// (Selection.UnmatchedHoldouts, called by ReadSources).
	ErrInvalidSelection = errors.New("dispatched: cutoff or holdout selection is invalid")

	// ErrInvalidTarget (F4): a target row lacks a key, a valid role or a
	// nonblank model, or repeats a key. Legacy readTargetTasks keeps its
	// ErrYAMLSource contract; the amended FC-1 path uses this sentinel.
	ErrInvalidTarget = errors.New("dispatched: target task is malformed")

	// ErrEmptyTarget (F4): a coverage or prediction gate was asked to check a
	// target with zero rows. A gate that measures nothing fails closed.
	ErrEmptyTarget = errors.New("dispatched: target has no rows")

	// ErrNotEligible (F4): the artifact cannot license a prediction: a
	// required cell is below the completed-sample threshold, or the sources
	// are not complete.
	ErrNotEligible = errors.New("dispatched: reference class is not prediction-eligible")
)
