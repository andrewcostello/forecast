package dispatched

import "time"

// Node is one row of a dependency graph with a duration already sampled.
// BlockedBy holds the keys that must finish before this node may start.
type Node struct {
	Key       string
	Duration  time.Duration
	BlockedBy []string
}

// Graph is a set of nodes in declaration order. Keys are unique, every
// BlockedBy entry names a node in the graph, no Duration is negative, and
// the graph is acyclic.
type Graph struct {
	Nodes []Node
}

// Schedule is the result of running a Graph under a concurrency cap.
//
// Completion is the wall clock at which the last node finishes.
// CriticalPath is built from the node that finishes last by stepping to the
// BlockedBy node that finished last until a node with no BlockedBy is
// reached, and is listed in execution order. Ties in finishing time go to
// the earlier node in Graph.Nodes. The sum of its durations never exceeds
// Completion, and equals it when the cap is not binding.
type Schedule struct {
	Completion   time.Duration
	CriticalPath []string
}

// Scheduler computes when a Graph completes with at most maxParallel nodes
// running at once.
//
// Rules an implementation must honour:
//
//   - A node starts at the earliest instant when every BlockedBy node has
//     finished and fewer than maxParallel nodes are running.
//   - When more nodes are ready than slots free, they start in Graph.Nodes
//     order. The result is therefore deterministic for a given input.
//   - Completion is never less than the longest dependency chain, and never
//     more than the sum of all durations.
//   - With maxParallel at least len(Nodes), Completion equals the longest
//     dependency chain.
//   - An empty Graph yields Completion == 0 and a nil CriticalPath.
//   - Errors wrap ErrCycle, ErrUnknownDependency, ErrDuplicateKey,
//     ErrNegativeValue (a Duration below zero) or ErrInvalidConcurrency
//     (maxParallel < 1). An input that breaks several rules reports the first
//     in THIS order, so two implementations of this interface agree:
//     ErrInvalidConcurrency, ErrDuplicateKey, ErrUnknownDependency,
//     ErrNegativeValue, ErrCycle.
type Scheduler interface {
	Schedule(g Graph, maxParallel int) (Schedule, error)
}
