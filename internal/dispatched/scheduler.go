// F5 behavior is defined once in features/dispatched-forecasting/notes/
// FC-SCHED-SCAFFOLD.md; these declarations define the Go and fixture shapes.

package dispatched

import "time"

// Node carries an exact key, sampled duration and declared dependencies.
type Node struct {
	Key       string
	Duration  time.Duration
	BlockedBy []string
}

// Graph retains node declaration order.
type Graph struct {
	Nodes []Node
}

// NodeTrace records a node interval and its normalized dependency set.
type NodeTrace struct {
	Key       string
	Start     time.Duration
	Finish    time.Duration
	BlockedBy []string
}

// EdgeKind labels the edge entering an execution-chain node.
type EdgeKind string

const (
	EdgeStart EdgeKind = "start"

	EdgeDependency EdgeKind = "dependency"

	EdgeResource EdgeKind = "resource"
)

// ChainStep pairs a node with its incoming edge.
type ChainStep struct {
	Key  string
	Edge EdgeKind
}

// Schedule reports the trace, makespan and both explanation kinds.
type Schedule struct {
	Makespan       time.Duration
	Trace          []NodeTrace
	DependencyPath []string
	ExecutionChain []ChainStep
}

// Scheduler is the interface implemented independently by both arms.
type Scheduler interface {
	Schedule(g Graph, maxParallel int) (Schedule, error)
}

// ScheduleFixture is the hand-fixture wire shape; loader rules live in the handoff.
type ScheduleFixture struct {
	Name        string             `json:"name"`
	Provenance  string             `json:"provenance"`
	Note        string             `json:"note,omitempty"`
	Concurrency int                `json:"concurrency"`
	Nodes       []FixtureNode      `json:"nodes"`
	Expect      FixtureExpectation `json:"expect"`
}

// FixtureNode is an input node in a hand fixture.
type FixtureNode struct {
	Key       string   `json:"key"`
	Duration  string   `json:"duration"`
	BlockedBy []string `json:"blocked_by"`
}

// FixtureExpectation is a complete schedule or one expected sentinel.
type FixtureExpectation struct {
	Error          string             `json:"error"`
	Makespan       string             `json:"makespan,omitempty"`
	Trace          []FixtureTrace     `json:"trace,omitempty"`
	DependencyPath []string           `json:"dependency_path,omitempty"`
	ExecutionChain []FixtureChainStep `json:"execution_chain,omitempty"`
}

// FixtureTrace carries expected relative timestamps as duration strings.
type FixtureTrace struct {
	Key       string   `json:"key"`
	Start     string   `json:"start"`
	Finish    string   `json:"finish"`
	BlockedBy []string `json:"blocked_by"`
}

// FixtureChainStep is one expected explanation step.
type FixtureChainStep struct {
	Key  string   `json:"key"`
	Edge EdgeKind `json:"edge"`
}
