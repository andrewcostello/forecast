package dispatched

import "errors"

// Sentinels. Errors from this package and from implementations of its
// interfaces must wrap one of these with %w so callers classify with errors.Is.
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
