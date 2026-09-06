// F5 behavior is defined once in features/dispatched-forecasting/notes/
// FC-SCHED-SCAFFOLD.md; these declarations define the Go and fixture shapes.

package dispatched

import "errors"

var (
	ErrBlankKey = errors.New("dispatched: node key is blank")

	ErrScheduleOverflow = errors.New("dispatched: schedule duration overflow")
)

// SchedulerSentinel associates an error identifier with its sentinel.
type SchedulerSentinel struct {
	Name string
	Err  error
}

// schedulerSentinels is the authoritative ordered validation registry.
// ErrNotImplemented is a scaffold marker, not a completed-arm outcome.
var schedulerSentinels = [...]SchedulerSentinel{
	{Name: "ErrInvalidConcurrency", Err: ErrInvalidConcurrency},
	{Name: "ErrBlankKey", Err: ErrBlankKey},
	{Name: "ErrDuplicateKey", Err: ErrDuplicateKey},
	{Name: "ErrUnknownDependency", Err: ErrUnknownDependency},
	{Name: "ErrNegativeValue", Err: ErrNegativeValue},
	{Name: "ErrCycle", Err: ErrCycle},
	{Name: "ErrScheduleOverflow", Err: ErrScheduleOverflow},
}

// SchedulerSentinels returns an independent copy of the validation registry.
func SchedulerSentinels() []SchedulerSentinel {
	return append([]SchedulerSentinel(nil), schedulerSentinels[:]...)
}

// LookupSchedulerSentinel returns false for empty or unknown names.
// The handoff requires the fixture loader to distinguish those two cases.
func LookupSchedulerSentinel(name string) (sentinel error, ok bool) {
	for _, s := range schedulerSentinels {
		if s.Name == name {
			return s.Err, true
		}
	}
	return nil, false
}
