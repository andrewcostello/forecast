package dispatched

import "errors"

// Sentinels. Errors from this package and from implementations of its
// interfaces must wrap one of these with %w so callers classify with errors.Is.
var (
	// ErrUnattributable: the row cannot be placed in a (role, model) cell.
	ErrUnattributable = errors.New("dispatched: row cannot be attributed to a cell")

	// ErrUnparseableRevision: a source revision string is not in a recognised form.
	ErrUnparseableRevision = errors.New("dispatched: source revision cannot be parsed")

	// ErrInvalidOutcome: an Outcome value outside the declared constants.
	ErrInvalidOutcome = errors.New("dispatched: outcome is not a declared value")

	// ErrCycle: the dependency graph is not acyclic.
	ErrCycle = errors.New("dispatched: dependency graph has a cycle")

	// ErrUnknownDependency: a node is blocked by a key that is not in the graph.
	ErrUnknownDependency = errors.New("dispatched: dependency refers to an unknown node")

	// ErrDuplicateKey: two nodes in one graph share a key.
	ErrDuplicateKey = errors.New("dispatched: duplicate node key")

	// ErrInvalidConcurrency: a concurrency cap below one.
	ErrInvalidConcurrency = errors.New("dispatched: concurrency cap must be at least one")
)
