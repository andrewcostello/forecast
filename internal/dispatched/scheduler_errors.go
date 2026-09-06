package dispatched

import "errors"

// F5 scheduler sentinels added by FC-SCHED-SCAFFOLD. The baseline scheduler
// sentinels (ErrCycle, ErrUnknownDependency, ErrDuplicateKey,
// ErrInvalidConcurrency, ErrNegativeValue) stay in errors.go with their
// baseline identities; this file adds the two the F5 contract names that
// had no sentinel, and freezes the precedence order in one place.
var (
	// ErrBlankKey (F5): a node key is empty or only whitespace. A blank
	// BlockedBy entry is not this error: it can name no node and reports
	// ErrUnknownDependency.
	ErrBlankKey = errors.New("dispatched: node key is blank")

	// ErrScheduleOverflow (F5): the sum of all node durations is not
	// representable as a time.Duration. The sum bounds every Start, Finish
	// and the Makespan, so a representable sum guarantees representable
	// arithmetic; an unrepresentable sum is refused before scheduling even
	// when a particular cap would have produced a representable makespan.
	// Never saturation, never a wrapped negative finish.
	ErrScheduleOverflow = errors.New("dispatched: schedule duration overflow")
)

// SchedulerSentinel pairs the Go identifier of a scheduler sentinel with the
// sentinel itself so fixtures can name expected errors portably.
type SchedulerSentinel struct {
	Name string
	Err  error
}

// SchedulerSentinels lists every sentinel a Scheduler may wrap, in the
// frozen validation precedence. An input that violates several rules
// reports the FIRST violated entry of this list and no other, so the two
// arms and the oracle agree on multi-fault graphs:
//
//  1. ErrInvalidConcurrency  maxParallel < 1 (checked even for an empty graph)
//  2. ErrBlankKey            a node key is blank
//  3. ErrDuplicateKey        two nodes share a key
//  4. ErrUnknownDependency   a BlockedBy entry names no node (blank included)
//  5. ErrNegativeValue       a Duration below zero
//  6. ErrCycle               the dependency relation has a cycle (self-dependency included)
//  7. ErrScheduleOverflow    the sum of all durations exceeds math.MaxInt64 nanoseconds
//
// Each rule is evaluated over the whole graph before the next rule is
// considered; within a rule the first offending node in declaration order
// is the one named. ErrNotImplemented is not in this list: it is the
// scaffold hole, never a valid outcome of a finished arm.
var SchedulerSentinels = []SchedulerSentinel{
	{Name: "ErrInvalidConcurrency", Err: ErrInvalidConcurrency},
	{Name: "ErrBlankKey", Err: ErrBlankKey},
	{Name: "ErrDuplicateKey", Err: ErrDuplicateKey},
	{Name: "ErrUnknownDependency", Err: ErrUnknownDependency},
	{Name: "ErrNegativeValue", Err: ErrNegativeValue},
	{Name: "ErrCycle", Err: ErrCycle},
	{Name: "ErrScheduleOverflow", Err: ErrScheduleOverflow},
}

// LookupSchedulerSentinel resolves a fixture "expect.error" identifier to its
// sentinel. The second result is false for "" and for any name that is not
// in SchedulerSentinels.
func LookupSchedulerSentinel(name string) (error, bool) {
	for _, s := range SchedulerSentinels {
		if s.Name == name {
			return s.Err, true
		}
	}
	return nil, false
}
