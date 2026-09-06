# FC-SCHED-SCAFFOLD: frozen F5 scheduling handoff

Contract section: F5 of `docs/plans/forecasting-contracts.md`. This is a
contract scaffold. It freezes types, sentinels, the list-scheduling process,
both explanation kinds, the fixture format and the oracle. It implements no
scheduling: both arm entry points return `ErrNotImplemented`. No test or
fixture is included; FC-SCHED-SEALS authors them independently.

## Scope and ownership

| File | Owner after this row | Content |
|---|---|---|
| `internal/dispatched/scheduler.go` | FC-SCHED-ADJ (disputed path) | `Node`, `Graph`, `NodeTrace`, `EdgeKind`, `ChainStep`, `Schedule`, `Scheduler`, fixture types. No algorithm. |
| `internal/dispatched/scheduler_errors.go` | frozen | `ErrBlankKey`, `ErrScheduleOverflow`, `SchedulerSentinels` (precedence), `LookupSchedulerSentinel`. |
| `internal/dispatched/schedulea/scheduler.go` | FC-2A | `Scheduler` type, `Schedule(g, maxParallel)`; hole returns `ErrNotImplemented`. |
| `internal/dispatched/scheduleb/scheduler.go` | FC-2B | Same interface, same hole, separate package. |
| `internal/dispatched/scheduler_contract_test.go`, `internal/dispatched/testdata/scheduler/**` | FC-SCHED-SEALS, then FC-SCHED-ADJ | Corpus, oracle, `TestFCScheduleAContract`, `TestFCScheduleBContract`. |

Both arms expose exactly one shape: package function
`Schedule(g dispatched.Graph, maxParallel int) (dispatched.Schedule, error)`
and a zero-size `Scheduler` struct whose method delegates to it. Every
parameter of the hole is named so a body can use it without a signature
change. The arms share the `dispatched` types and sentinels and nothing
else: no validation helper, no normalization helper, no scheduling code
lives in `dispatched`. Each arm validates, normalizes and schedules on its
own, so the differential comparison covers errors as well as schedules.
Neither arm may read, import or reason from the other package, its notes or
its traces. Production adapter selection happens in FC-SCHED-ADJ after both
land; this scaffold exports no default scheduler.

Import shape for seals: `schedulea` and `scheduleb` import `dispatched`, so
a test that imports an arm must be `package dispatched_test` (external test
package), not `package dispatched` like the existing F1–F4 seal files. Both
files may coexist in `internal/dispatched/`.

## Declared changes to the baseline

The baseline `scheduler.go` (interface only, no implementation, no caller,
no test anywhere in the module; verified by grep for `Schedule`, `Graph`,
`CriticalPath` and the four scheduler sentinels) is amended as follows.

| Baseline | Amended | Reason |
|---|---|---|
| `Schedule.Completion` | `Schedule.Makespan` | F5 vocabulary. |
| `Schedule.CriticalPath` | `Schedule.DependencyPath` | F5: dependency-only information keeps its proper label; it is not the cap-constrained critical path. |
| — | `Schedule.Trace`, `Schedule.ExecutionChain` | F5 requires per-node start/finish and the resource-aware explanation. |
| precedence `ErrInvalidConcurrency, ErrDuplicateKey, ErrUnknownDependency, ErrNegativeValue, ErrCycle` | `ErrInvalidConcurrency, ErrBlankKey, ErrDuplicateKey, ErrUnknownDependency, ErrNegativeValue, ErrCycle, ErrScheduleOverflow` | Two new F5 rules inserted; relative order of the five baseline sentinels preserved. |
| "Ties in finishing time go to the earlier node in Graph.Nodes" | unchanged | Same tie-break, now applied to both explanations. |
| empty graph: `Completion == 0`, nil path | unchanged: `Makespan == 0`, nil `Trace`/`DependencyPath`/`ExecutionChain` | Preserved. |

No other file changed. `errors.go` is not owned by this row and is untouched;
the five baseline scheduler sentinels keep their identities there.

## Contract rulings

Each ruling below resolves a point F5 leaves open or states it precisely
enough for two isolated implementers to agree. Deviations from F5: none.

- Input durations are already sampled. Neither arm reads a clock, a random
  source or the reference class. The result is a pure function of
  `(g, maxParallel)`; `g` and its slices are not mutated.
- Declaration order (index in `Graph.Nodes`) is the only tie-break. Keys are
  compared byte-exact and never sorted.
- Ready ordering: at each timestamp, pending nodes are walked in
  declaration order and every ready node starts while a slot is free. A
  ready node that finds no slot waits; a later-declared node never overtakes
  it.
- Simultaneous completions: all nodes finishing at `t` are completed before
  any node starts at `t`. Filling after each individual completion is a
  distinct, wrong process (see F5-SIMULTANEOUS-COMPLETIONS).
- Zero-duration work takes a slot for zero time. It starts only when a slot
  is free, finishes at its start instant, and its completion is processed at
  the same timestamp, freeing the slot and possibly readying dependents.
  COMPLETE and FILL repeat at that timestamp until a pass starts nothing.
  Zero-duration work is therefore not exempt from the cap and does not jump
  the declaration order.
- Dependencies are sets: repeated `BlockedBy` entries are one dependency;
  `NodeTrace.BlockedBy` reports the set ordered by the referenced node's
  declaration index, nil when empty. A self-reference is a cycle.
- Validation precedence is the order of `SchedulerSentinels`. Every rule is
  evaluated over the whole graph before the next rule; within one rule the
  first offending node in declaration order is named. The returned error
  satisfies `errors.Is` for exactly one scheduler sentinel. Message text is
  not frozen. `maxParallel < 1` is refused even for an empty graph. A blank
  `BlockedBy` entry is `ErrUnknownDependency`, not `ErrBlankKey`.
- Overflow is defined on the bound, not the outcome: if the sum of all
  durations exceeds `math.MaxInt64` nanoseconds the graph is refused with
  `ErrScheduleOverflow` before scheduling, even if the requested cap would
  have produced a representable makespan. A representable sum guarantees
  every start, finish and the makespan are representable, so no arm needs
  checked arithmetic inside the scheduling loop.
- Last node `L` for both explanations: largest `Finish`, ties to the
  earliest declared. Both lists are head first.
- `DependencyPath`: from `L`, step to the dependency with the largest
  `Finish` (ties: earliest declared) until a node with no dependencies. Its
  duration sum is `<= Makespan`, and `== Makespan` when no node waited for a
  slot.
- `ExecutionChain` predecessor of node `n` with normalized dependency set
  `D` and `depReady = max Finish over D` (0 when `D` is empty):
  `D` nonempty and `Start(n) == depReady` → `EdgeDependency` to the
  dependency with `Finish == depReady` (ties: earliest declared);
  `Start(n) > depReady` → `EdgeResource` to the node `r` with
  `Finish(r) == Start(n)` and `Duration(r) > 0` (ties: earliest declared);
  `D` empty and `Start(n) == 0` → head, `EdgeStart`. These cases are
  exhaustive. Every edge is contiguous, so the chain's duration sum always
  equals `Makespan`. A zero-duration node is never a resource predecessor:
  it released no time. A dependency of `n` never qualifies as a resource
  predecessor because under the rule all dependencies finished strictly
  before `Start(n)`.
- With `maxParallel >= len(Nodes)` no node waits: `Makespan` equals the
  longest dependency chain, the execution chain carries only `EdgeStart` and
  `EdgeDependency`, and its keys equal `DependencyPath`.
- Resource ordering is this list-scheduling policy. It is not a claim to
  find an optimal schedule, and the seals must not assert optimality.

## Behavioral examples for independent seals

Durations are seconds unless stated. Traces are `key start..finish`. Each
example was computed by hand against the process above; a seal that
disagrees with one of these rows is a contract dispute for FC-SCHED-ADJ,
not a reason to edit an arm. The same rows are expected of both arms.

| Name | Input (cap; nodes in declaration order) | Expected |
|---|---|---|
| F5-CAP-BINDS | cap 1; A=2, B=3 | A 0..2, B 2..5; makespan 5; DependencyPath `[B]`; ExecutionChain `[A start, B resource]`. The dependency path sums to 3 and cannot explain 5. |
| F5-CAP-FREE | cap 2; A=2, B=3 | A 0..2, B 0..3; makespan 3; `[B]`; `[B start]`. |
| F5-FORK-JOIN-FREE | cap 2; A=1, B=2 (A), C=3 (A), D=1 (B, C) | A 0..1, B 1..3, C 1..4, D 4..5; makespan 5; `[A, C, D]`; `[A start, C dependency, D dependency]`. |
| F5-FORK-JOIN-CAP1 | cap 1; same graph | A 0..1, B 1..3, C 3..6, D 6..7; makespan 7; DependencyPath `[A, C, D]` (sum 5); ExecutionChain `[A start, B dependency, C resource, D dependency]` (sum 7). |
| F5-CAP-OVER-N | cap 10; same graph | Identical to F5-FORK-JOIN-FREE. |
| F5-READY-DECLARATION-ORDER | cap 1; B=3, A=2 | B 0..3, A 3..5; makespan 5; `[A]`; `[B start, A resource]`. Compare F5-CAP-BINDS: same keys and durations, different declaration order, different trace; key order is never consulted. |
| F5-SIMULTANEOUS-COMPLETIONS | cap 2; X=2, Y=2, P=1 (Y), Q=1 (X), R=1 (X) | X 0..2, Y 0..2, P 2..3, Q 2..3, R 3..4; makespan 4; `[X, R]`; `[Y start, P dependency, R resource]`. Mutation: completing X and filling before completing Y starts Q and R at 2 and P at 3. |
| F5-ZERO-DRAIN | cap 1; A=0, B=0 (A), C=1 (B) | A 0..0, B 0..0, C 0..1; makespan 1; `[A, B, C]`; `[A start, B dependency, C dependency]`. |
| F5-ZERO-WAITS-FOR-SLOT | cap 1; A=2, Z=0 | A 0..2, Z 2..2; makespan 2; `L` is A (tie at 2, earlier declared); `[A]`; `[A start]`. |
| F5-ZERO-DECLARATION-ORDER | cap 1; P=0, Q=3, R=0 | P 0..0, Q 0..3, R 3..3; makespan 3; `[Q]`; `[Q start]`. R does not overtake Q. |
| F5-RESOURCE-SKIPS-ZERO | cap 1; A=2, Z=0, B=1 | A 0..2, Z 2..2, B 2..3; makespan 3; `[B]`; `[A start, B resource]`. Z finishes at B's start but has zero duration and is not the predecessor. |
| F5-RESOURCE-TIE | cap 2; A=3, B=3, C=1 | A 0..3, B 0..3, C 3..4; makespan 4; `[C]`; `[A start, C resource]` (A and B both qualify; earliest declared). |
| F5-DUP-DEPENDENCY | cap 1; A=1, B=1 (A, A) | A 0..1, B 1..2; `Trace[B].BlockedBy == [A]`; makespan 2; `[A, B]`; `[A start, B dependency]`. Input `BlockedBy` still `[A, A]` after the call. |
| F5-DEP-SET-ORDER | cap 2; A=1, B=1, C=1 (B, A) | `Trace[C].BlockedBy == [A, B]`; A 0..1, B 0..1, C 1..2; makespan 2; `[A, C]` (A and B tie at 1; earliest declared); `[A start, C dependency]`. |
| F5-EMPTY | cap 1; no nodes | makespan 0; nil `Trace`, `DependencyPath`, `ExecutionChain`; nil error. |
| F5-EMPTY-BAD-CAP | cap 0; no nodes | `ErrInvalidConcurrency`. |
| F5-BLANK-KEY | cap 1; key `"  "` | `ErrBlankKey`. |
| F5-BLANK-DEPENDENCY | cap 1; A=1 (`""`) | `ErrUnknownDependency`. |
| F5-DUPLICATE-KEY | cap 1; A=1, A=1 | `ErrDuplicateKey`. |
| F5-UNKNOWN-DEPENDENCY | cap 1; A=1 (B) | `ErrUnknownDependency`. |
| F5-NEGATIVE | cap 1; A=-1s | `ErrNegativeValue`. |
| F5-SELF-DEPENDENCY | cap 1; A=1 (A) | `ErrCycle`. |
| F5-CYCLE | cap 1; A=1 (B), B=1 (A) | `ErrCycle`. |
| F5-OVERFLOW | cap 2; A=4611686018427387904ns, B=4611686018427387904ns | `ErrScheduleOverflow` although a cap-2 makespan of 2^62 ns would fit. |
| F5-PRECEDENCE-LADDER | cap 0; `"  "`=-1s (Z), A=1 (A), A=1 | `ErrInvalidConcurrency`. Then with cap 1: `ErrBlankKey`; rename the blank key to K: `ErrDuplicateKey`; rename the third node to C: `ErrUnknownDependency` (Z); drop Z: `ErrNegativeValue`; set K=1s: `ErrCycle` (A self). Each step reports exactly one sentinel. |
| F5-EXACTLY-ONE-SENTINEL | any invalid input | `errors.Is` is true for exactly one entry of `SchedulerSentinels` and false for the other six and for `ErrNotImplemented`. |
| F5-PURE | any valid input, called twice, in parallel goroutines under `-race` | Byte-identical `Schedule` values; input graph unchanged. |

## Canonical fixture format

One case per file: `internal/dispatched/testdata/scheduler/cases/<name>.json`,
`<name>` equal to the `"name"` field. The Go shape is `ScheduleFixture` in
`scheduler.go`; the JSON keys are `name`, `provenance`, `note` (optional),
`concurrency`, `nodes[]{key, duration, blocked_by}` and
`expect{error, makespan, trace[]{key, start, finish, blocked_by},
dependency_path, execution_chain[]{key, edge}}`. Durations are
`time.ParseDuration` strings compared by parsed value. `expect.error` is
`""` for a schedule or the Go identifier of one `SchedulerSentinels` entry
(`LookupSchedulerSentinel` resolves it). A null or absent list equals an
empty list. Unknown keys are a loader error. `provenance` is `synthetic` for
every scheduler fixture: no recorded schedule exists and none is claimed.
The same file feeds arm A, arm B and the oracle; nothing in a fixture is
arm-specific.

## Independent small-graph oracle

Name: `tickOracle`, written by FC-SCHED-SEALS inside
`scheduler_contract_test.go`, imported by neither arm, and never derived
from either arm's code. It advances time one tick at a time instead of
jumping between events: durations are integer ticks (seconds in fixtures),
each node keeps a remaining-tick counter, and at every tick the oracle
retires every running node whose counter is zero, then repeatedly starts
the earliest-declared ready pending node while a slot is free (retiring
zero-tick nodes in the same tick), then decrements the running counters.
It derives `DependencyPath` and `ExecutionChain` from its own trace by the
definitional rules above, and checks invalid graphs by direct definition in
the frozen precedence. Bounded generated corpus: nodes `<= 5`, durations in
`{0,1,2,3}` ticks, cap `1..N+1`, dependency edges only from earlier-declared
to later-declared nodes (so generated graphs are acyclic by construction and
cycle cases stay in the hand-written corpus), enumerated deterministically
with no random source; FC-SCHED-SEALS records the exact enumeration and
count. Every generated case compares the complete `Schedule` (makespan,
trace, both paths) and the error, not makespan alone. Disagreement between
an arm and the oracle, or between the arms, is minimized and adjudicated by
FC-SCHED-ADJ; no arm is edited to agree.

## Guidance for FC-SCHED-SEALS drawn from inherited findings

The inherited FC-OBS-ADJ findings concern evidence and reference-class code
and name no scheduler file; none is reproduced here. Two of their shapes
are relevant to writing these seals and are pre-empted by the rulings:

- The F4-NOT-ELIGIBLE-PARTIAL finding (a disjunctive `errors.Is` assertion
  that passes when either sentinel is present). F5-EXACTLY-ONE-SENTINEL
  requires asserting the expected sentinel is present AND the other six are
  absent, so a body that wraps two sentinels, or the wrong one, fails.
- The bare-number search finding (a line-number assertion satisfied by the
  year `2026`). Fixture comparison is by parsed duration value and by whole
  `Schedule` equality, never by substring of a formatted trace.

## Validation

- `go build ./...` and `go vet ./...` pass on this head.
- `env -u DISPATCHER_KNOWN_RED_FILE go test ./... -race -count=1`: exit 0,
  all 12 tested packages `ok`, zero `--- FAIL` lines, no exclusions. The
  scheduler seal groups do not exist yet, so nothing is red on this head.
- Stub check (throwaway test, run once and deleted, not committed): from
  `package dispatched_test`, both `schedulea.Scheduler{}` and
  `scheduleb.Scheduler{}` return a zero `Schedule` and an error satisfying
  `errors.Is(err, ErrNotImplemented)`; `LookupSchedulerSentinel` resolves
  `ErrScheduleOverflow` and rejects `""`. This also confirms the import
  shape (external test package required) works without a cycle.
- No acceptance test authored. No fixture authored. No behavior of any
  existing package changed; the only touched package is `internal/dispatched`
  (types and sentinels) plus the two new stub packages.

Residual limitation: the rulings above are checked by hand, not by a
running implementation; the first executable check is the oracle FC-SCHED-SEALS
writes, and any row it contradicts goes to FC-SCHED-ADJ as a contract
dispute rather than being silently corrected in an arm.
