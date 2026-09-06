// Package schedulea is scheduler arm A of the F5 differential pair.
//
// FC-2A implements this file, and only this file, from the frozen contract
// in features/dispatched-forecasting/notes/FC-SCHED-SCAFFOLD.md;
// scheduler.go and scheduler_errors.go define the shared shapes and registry. It must not
// read, import or reason from internal/dispatched/scheduleb, its tests,
// traces or notes. The arms share the dispatched types and sentinels and
// nothing else: no validation helper, no normalization helper, no
// scheduling code. Durations arrive already sampled; this package performs
// no random sampling and reads no clock.
package schedulea

import (
	"fmt"

	"github.com/andrewcostello/forecast/internal/dispatched"
)

// Scheduler is arm A's implementation of dispatched.Scheduler.
type Scheduler struct{}

var _ dispatched.Scheduler = Scheduler{}

// Schedule satisfies dispatched.Scheduler by delegating to the package
// function so both call shapes are frozen.
func (Scheduler) Schedule(g dispatched.Graph, maxParallel int) (dispatched.Schedule, error) {
	return Schedule(g, maxParallel)
}

// Schedule validates g and maxParallel in the precedence frozen by
// dispatched.SchedulerSentinels, then runs the list-scheduling process
// defined in the authoritative handoff and returns the complete schedule:
// makespan, per-node trace, dependency path and execution chain.
//
// Scaffold hole: FC-2A replaces this body. The parameters are named even
// though this stub does not use them, because the body may not change the
// signature and cannot read a value bound to the blank identifier.
func Schedule(g dispatched.Graph, maxParallel int) (dispatched.Schedule, error) {
	return dispatched.Schedule{}, fmt.Errorf("schedulea: Schedule: %w", dispatched.ErrNotImplemented)
}
