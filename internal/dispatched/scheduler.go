package dispatched

import "time"

// F5 scheduling contract (docs/plans/forecasting-contracts.md, section F5).
//
// Scheduling is a pure deterministic function of a Graph whose durations
// are already sampled and a concurrency cap. No random sampling, no clock,
// no reference-class access: sampling belongs to FC-3. Two independent arms,
// internal/dispatched/schedulea and internal/dispatched/scheduleb, implement
// Scheduler from this file and features/dispatched-forecasting/notes/
// FC-SCHED-SCAFFOLD.md alone. This package holds the shared types, the
// sentinel errors (scheduler_errors.go) and the canonical fixture format; it
// deliberately holds no validation, normalization or scheduling algorithm,
// so that the two arms share nothing an error could hide in.

// Node is one row of a dependency graph with a duration already sampled.
//
// Key is compared byte-exact; a key that is empty or only whitespace is
// blank. Duration is a sampled time.Duration at nanosecond resolution and
// is used exactly: no rounding, quantizing to ticks or seconds, or float
// conversion anywhere in scheduling. BlockedBy is the raw declared
// dependency list. Its meaning is a SET: repeated entries are one
// dependency, order carries no information, and an entry naming the node
// itself is a cycle. An entry may name a node declared LATER in the Graph
// (declaration order is not a topological order); such a forward reference
// is legal, not a cycle, and is scheduled by the same process.
// Implementations must not mutate BlockedBy (or any other input slice)
// while normalizing it.
type Node struct {
	Key       string
	Duration  time.Duration
	BlockedBy []string
}

// Graph is a set of nodes in declaration order. Declaration order (the index
// in Nodes) is the only tie-break the contract uses: it decides which ready
// node takes a free slot, which of several last-finishing nodes anchors the
// explanations, which of several equal candidates becomes a chain edge, and
// how NodeTrace.BlockedBy is ordered. Keys are never sorted: a Graph whose
// keys are declared out of lexical order (B before A) resolves every tie in
// favour of B, and an implementation that sorts a dependency set by key is
// wrong even where it agrees on lexically ordered inputs.
//
// A Graph is valid when every key is nonblank and unique, every BlockedBy
// entry names a node in the graph, no Duration is negative, the dependency
// relation is acyclic, and the sum of all durations is representable as a
// time.Duration (computed with a per-add checked increment, never a
// wrapping +=; see ErrScheduleOverflow). Validation belongs to each arm;
// see the precedence order documented on SchedulerSentinels.
type Graph struct {
	Nodes []Node
}

// NodeTrace is the scheduled interval of one node.
//
// Start is the instant the node took a slot and Finish is Start+Duration,
// both measured from 0, the start of the schedule. BlockedBy is the
// normalized dependency set: duplicates removed, ordered by the referenced
// node's declaration index, nil when the node has no dependencies.
type NodeTrace struct {
	Key       string
	Start     time.Duration
	Finish    time.Duration
	BlockedBy []string
}

// EdgeKind labels how a node on the execution chain came to start when it
// did. The string values are frozen because fixtures carry them.
type EdgeKind string

const (
	// EdgeStart marks the chain head: a node with no dependencies that
	// started at 0. Exactly the first ChainStep carries it.
	EdgeStart EdgeKind = "start"

	// EdgeDependency: the node started the instant its latest-finishing
	// dependency finished. The previous chain step is that dependency.
	EdgeDependency EdgeKind = "dependency"

	// EdgeResource: every dependency had finished earlier, but no slot was
	// free until the previous chain step, an unrelated node, finished and
	// released one. This edge exists only because the concurrency cap bound
	// for this specific schedule.
	EdgeResource EdgeKind = "resource"
)

// ChainStep is one node of an execution chain together with the kind of
// edge that leads INTO it from the previous step.
type ChainStep struct {
	Key  string
	Edge EdgeKind
}

// Schedule is the result of running a Graph under a concurrency cap.
//
// Makespan is the largest Finish over all nodes (0 for an empty graph).
//
// Trace has one entry per node in declaration order.
//
// DependencyPath and ExecutionChain are two different explanations of the
// makespan, both listed head first in execution order and both anchored on
// the same last node L: the node with the largest Finish, ties broken by
// earliest declaration.
//
// DependencyPath uses original dependency edges only. From L, step to the
// dependency with the largest Finish (ties: earliest declaration) until a
// node without dependencies is reached. The sum of its durations never
// exceeds Makespan and equals it whenever no node waited for a slot (for
// example when maxParallel >= len(Nodes)). It is NOT the cap-constrained
// critical path and must not be labeled as one.
//
// ExecutionChain adds resource-slot waits. From L, step to the predecessor
// chosen by the frozen rule (documented on Scheduler) until a node that
// started at 0 with no dependencies is reached. Every edge is contiguous
// (the predecessor's Finish equals the node's Start), so the sum of its
// durations always equals Makespan. Independent A=2 and B=3 under cap 1
// take 5: the dependency path is [B] and cannot explain 5, the execution
// chain [A start, B resource] can.
//
// An empty Graph yields Makespan 0 and nil Trace, DependencyPath and
// ExecutionChain. Comparisons treat a nil slice and an empty slice as equal.
type Schedule struct {
	Makespan       time.Duration
	Trace          []NodeTrace
	DependencyPath []string
	ExecutionChain []ChainStep
}

// Scheduler computes when a Graph completes with at most maxParallel nodes
// running at once. It is the single interface both arms expose; the arms
// are otherwise independent and must not read each other's code.
//
// Validation (before any scheduling; see SchedulerSentinels for the frozen
// precedence). An invalid input returns the zero Schedule and an error that
// wraps exactly one scheduler sentinel, the highest-precedence violation
// present. Within one sentinel the message names the first offending node
// in declaration order; message text is not frozen, sentinel identity is.
//
// Process (list scheduling, frozen):
//
//   - Time starts at 0 with no node running. Every node is pending.
//   - At each timestamp t, first COMPLETE every running node whose Finish
//     is t, all of them, before any node starts at t.
//   - Then FILL: walk the pending nodes in declaration order; a node is
//     ready when every dependency has finished (Finish <= t). Start each
//     ready node while a slot is free: Start=t, Finish=t+Duration. A node
//     that is ready but finds no free slot waits; a later-declared node
//     never overtakes it.
//   - A zero-duration node takes a slot like any other and finishes at the
//     instant it starts. Its completion is processed at the same t, which
//     frees the slot and may make dependents ready, so COMPLETE and FILL
//     repeat at t until a pass starts nothing. This drains zero-duration
//     work deterministically in declaration order without advancing time.
//   - Advance t to the smallest Finish among running nodes and repeat until
//     nothing is pending or running.
//
// Chain predecessor rule for a node n (D = its normalized dependency set,
// depReady = the largest Finish in D, or 0 when D is empty):
//
//   - D nonempty and Start(n) == depReady: EdgeDependency to the dependency
//     with Finish == depReady, ties to the earliest declared.
//   - Start(n) > depReady: EdgeResource to the node r with Finish(r) ==
//     Start(n) and Duration(r) > 0 (it held a slot up to that instant),
//     ties to the earliest declared. Such an r always exists under the
//     process above; a dependency of n never qualifies because every
//     dependency finished before Start(n).
//   - D empty and Start(n) == 0: n is the head and carries EdgeStart.
//
// Properties an implementation must satisfy for every valid input:
//
//   - The result is a pure function of (g, maxParallel): no randomness, no
//     clock, no dependence on map iteration order, and g is not mutated.
//   - Makespan is at least the longest dependency chain and at most the sum
//     of all durations; with maxParallel >= len(Nodes) it equals the longest
//     dependency chain and ExecutionChain carries only EdgeStart and
//     EdgeDependency edges whose keys equal DependencyPath.
//   - Resource ordering is this list-scheduling policy, not a claim to find
//     an optimal schedule.
type Scheduler interface {
	Schedule(g Graph, maxParallel int) (Schedule, error)
}

// Canonical fixture format.
//
// One case per file under internal/dispatched/testdata/scheduler/cases/
// named <name>.json where <name> equals the "name" field. Files are UTF-8
// JSON objects with these keys (unknown keys are an error for the loader):
//
//	{
//	  "name": "F5-CAP-BINDS",
//	  "provenance": "synthetic",
//	  "note": "free text for the adjudicator",
//	  "concurrency": 1,
//	  "nodes": [
//	    {"key": "A", "duration": "2s", "blocked_by": []},
//	    {"key": "B", "duration": "3s", "blocked_by": []}
//	  ],
//	  "expect": {
//	    "error": "",
//	    "makespan": "5s",
//	    "trace": [
//	      {"key": "A", "start": "0s", "finish": "2s", "blocked_by": []},
//	      {"key": "B", "start": "2s", "finish": "5s", "blocked_by": []}
//	    ],
//	    "dependency_path": ["B"],
//	    "execution_chain": [
//	      {"key": "A", "edge": "start"},
//	      {"key": "B", "edge": "resource"}
//	    ]
//	  }
//	}
//
// Durations are Go duration strings as accepted by time.ParseDuration
// ("0s", "2s", "1m30s", "-1s", "4611686018427387904ns"); comparison is by
// parsed value, never by string. "expect.error" is either "" (a schedule is
// expected and every other expect field is compared in full) or the exact
// Go identifier of one scheduler sentinel from SchedulerSentinels (then the
// error must satisfy errors.Is for that sentinel and no other, and the
// remaining expect fields must be absent or empty). A null or absent list
// equals an empty list. The same file drives both arms and the oracle.
type ScheduleFixture struct {
	Name        string             `json:"name"`
	Provenance  string             `json:"provenance"`
	Note        string             `json:"note,omitempty"`
	Concurrency int                `json:"concurrency"`
	Nodes       []FixtureNode      `json:"nodes"`
	Expect      FixtureExpectation `json:"expect"`
}

// FixtureNode is one node of a ScheduleFixture.
type FixtureNode struct {
	Key       string   `json:"key"`
	Duration  string   `json:"duration"`
	BlockedBy []string `json:"blocked_by"`
}

// FixtureExpectation is the expected outcome of a ScheduleFixture.
type FixtureExpectation struct {
	Error          string             `json:"error"`
	Makespan       string             `json:"makespan,omitempty"`
	Trace          []FixtureTrace     `json:"trace,omitempty"`
	DependencyPath []string           `json:"dependency_path,omitempty"`
	ExecutionChain []FixtureChainStep `json:"execution_chain,omitempty"`
}

// FixtureTrace is the expected NodeTrace of one node, durations as strings.
type FixtureTrace struct {
	Key       string   `json:"key"`
	Start     string   `json:"start"`
	Finish    string   `json:"finish"`
	BlockedBy []string `json:"blocked_by"`
}

// FixtureChainStep is the expected ChainStep of one execution-chain node.
type FixtureChainStep struct {
	Key  string   `json:"key"`
	Edge EdgeKind `json:"edge"`
}
