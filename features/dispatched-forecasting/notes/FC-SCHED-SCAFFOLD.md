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

## Contract authority

This note is the single normative source for F5 behavioral, fixture-loader
and oracle rules. Go declarations own type/signature/wire-key shape;
`SchedulerSentinels` owns the ordered error registry. Go comments point here
and do not restate the algorithm. Existing failed native panels are preserved;
this operator correction requires independent acceptance before seals start.

## Contract rulings

Each ruling below resolves a point F5 leaves open or states it precisely
enough for two isolated implementers to agree. Deviations from F5: none.

- Input durations are already sampled. Neither arm reads a clock, a random
  source or the reference class. The result is a pure function of
  `(g, maxParallel)`; `g` and its slices are not mutated.
- Declaration order (index in `Graph.Nodes`) is the only tie-break. Keys are
  compared byte-exact and never sorted. This applies in all four places a
  tie can arise: the FILL walk, the choice of last node `L`, the
  `DependencyPath` step and the `ExecutionChain` predecessor, and to the
  ordering of `NodeTrace.BlockedBy`. An arm that collects a dependency set
  in a map and sorts it by key is wrong even though it agrees with every row
  whose keys happen to be declared in lexical order; the rows
  F5-LAST-NODE-KEY-ORDER, F5-RESOURCE-TIE-KEY-ORDER and
  F5-BLOCKED-BY-KEY-ORDER declare keys out of lexical order to catch it.
- Readiness depends on a dependency's **completed state**, not its stored
  Finish. An unscheduled node or a running zero-duration node is not completed.
  Start at t=0 with every node pending. Each timestamp runs these phases:
  1. COMPLETE all running nodes with Finish==t, marking them completed and
     releasing their slots, before any FILL walk. No interleaving per completion.
  2. FILL walks pending nodes once in declaration order. A node is ready iff
     all its dependencies are completed. Start each ready node while a slot
     remains; Start=t, Finish=t+Duration. Newly started nodes enter running,
     including zeros. Do not retire them during this walk. A later ready node
     can start if an earlier node is not yet ready.
  3. If running contains any Finish==t (new zeros), repeat COMPLETE then FILL
     at the same t. Otherwise advance to the minimum running Finish (>t).
     Stop when pending and running are empty. Never take a minimum of an
     empty set or spin: pending with no running after these phases is an
     impossible-progress/cycle refusal, zero Schedule with ErrCycle. Validated
     DAGs cannot reach it; cycle validation still precedes overflow.
  Zero nodes occupy a slot until the next COMPLETE phase at the same instant.
  This resolves the former Finish<=t versus retirement ambiguity; see
  F5-ZERO-RETIRE-BEFORE-READY. It adds no positive duration for zero work.
- Key blankness means `strings.TrimSpace(key)==""` using Go Unicode
  whitespace semantics. This is a predicate only: never replace any key or
  dependency with its trimmed form. Nonblank padded keys remain distinct;
  lookup and duplicates use the original bytes. U+200B is not whitespace.
- Dependencies are sets: repeated `BlockedBy` entries are one dependency;
  `NodeTrace.BlockedBy` reports the set ordered by the referenced node's
  declaration index, nil when empty. A self-reference is a cycle.
- Declaration order is not a topological order. A `BlockedBy` entry may
  name a node declared later than the dependent (a reverse-declaration
  edge). Such an edge is legal once the uniqueness, unknown-dependency and
  cycle checks pass; it is not `ErrCycle`, not `ErrUnknownDependency`, and
  it is scheduled by the same process (the dependent simply is not ready
  during the FILL walk until the later-declared node has finished). An arm
  that only looks backwards for dependencies, or that treats a forward
  reference as a cycle, agrees with every row whose edges point to
  earlier-declared nodes and fails F5-REVERSE-DECLARATION.
- Durations are `time.Duration` at nanosecond resolution. Start, Finish and
  Makespan are exact sums of the input durations; no arm may round, quantize
  or convert through a float or a whole-second tick. The row grammar below
  uses whole seconds only for readability, and F5-MIXED-UNITS and
  F5-NANOSECOND-GRAIN pin values that no whole-second arithmetic reproduces.
- Validation precedence is the order of `SchedulerSentinels`. Every rule is
  evaluated over the whole graph before the next rule; within one rule the
  first offending node in declaration order is named. The returned error
  satisfies `errors.Is` for exactly one scheduler sentinel. Message text is
  not frozen. `maxParallel < 1` is refused even for an empty graph. A blank
  `BlockedBy` entry is `ErrUnknownDependency`, not `ErrBlankKey`.
- Overflow is defined on the bound, not the outcome: if the mathematical
  sum of all durations exceeds `math.MaxInt64` nanoseconds the graph is
  refused with `ErrScheduleOverflow` before scheduling, even if the
  requested cap would have produced a representable makespan. The sum MUST
  be computed with a per-add checked increment after negatives have been
  rejected: for each node, `if d > math.MaxInt64-sum { overflow }` before
  `sum += d`. `time.Duration` is `int64` and `+=` wraps silently; a wrapping
  sum followed by a final `sum < 0` test is not an implementation of this
  rule, because two wraps land on a small positive value (see
  F5-OVERFLOW-WRAP) and the scheduling loop would then emit wrapped
  timestamps. Wrap-to-positive is the case the check exists to catch. With
  the checked sum in place, a representable sum guarantees every start,
  finish and the makespan are representable, so no arm needs checked
  arithmetic inside the scheduling loop itself.
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

Durations are seconds unless stated (a bare `2` is `2s`; other units are
written as `time.ParseDuration` strings). Traces are `key start..finish`.
Each example was computed by hand against the process above; a seal that
disagrees with one of these rows is a contract dispute for FC-SCHED-ADJ,
not a reason to edit an arm. The same rows are expected of both arms.
Every success row below MUST be encoded as a fixture and asserted directly
against both arms; the tick oracle of the next section is a third check on
its own generated corpus and is never the source of truth for these rows
(several of them lie outside its bounds on purpose).

| Name | Input (cap; nodes in declaration order) | Expected |
|---|---|---|
| F5-CAP-BINDS | cap 1; A=2, B=3 | A 0..2, B 2..5; makespan 5; DependencyPath `[B]`; ExecutionChain `[A start, B resource]`. The dependency path sums to 3 and cannot explain 5. |
| F5-CAP-FREE | cap 2; A=2, B=3 | A 0..2, B 0..3; makespan 3; `[B]`; `[B start]`. |
| F5-FORK-JOIN-FREE | cap 2; A=1, B=2 (A), C=3 (A), D=1 (B, C) | A 0..1, B 1..3, C 1..4, D 4..5; makespan 5; `[A, C, D]`; `[A start, C dependency, D dependency]`. |
| F5-FORK-JOIN-CAP1 | cap 1; same graph | A 0..1, B 1..3, C 3..6, D 6..7; makespan 7; DependencyPath `[A, C, D]` (sum 5); ExecutionChain `[A start, B dependency, C resource, D dependency]` (sum 7). |
| F5-CAP-OVER-N | cap 10; same graph | Identical to F5-FORK-JOIN-FREE. |
| F5-READY-DECLARATION-ORDER | cap 1; B=3, A=2 | B 0..3, A 3..5; makespan 5; `[A]`; `[B start, A resource]`. Compare F5-CAP-BINDS: same keys and durations, different declaration order, different trace; key order is never consulted. |
| F5-REVERSE-DECLARATION | cap 1; A=2 (B), B=3 | B 0..3, A 3..5; makespan 5; DependencyPath `[B, A]`; ExecutionChain `[B start, A dependency]`; `Trace[A].BlockedBy == [B]`; nil error. A depends on a node declared after it. During the FILL walk at 0, A is walked first but is not ready, so B takes the slot. An arm that rejects forward references (as `ErrCycle` or `ErrUnknownDependency`) or that resolves dependencies only among earlier-declared nodes fails this row and also F5-RESOURCE-SKIPS-ZERO-DECLARED-FIRST. |
| F5-SIMULTANEOUS-COMPLETIONS | cap 2; X=2, Y=2, P=1 (Y), Q=1 (Y), R=1 (X) | X 0..2, Y 0..2, P 2..3, Q 2..3, R 3..4; makespan 4; `[X, R]`; `[Y start, P dependency, R resource]`. Mutation: completing X and filling before completing Y sees only R ready (P and Q still wait on Y), starts R 2..3, then completes Y and starts P 2..3 with no slot left for Q, giving Q 3..4, makespan 4, DependencyPath `[Y, Q]` and chain `[Y start, P dependency, Q resource]`; the frozen process starts P and Q at 2 and R at 3. |
| F5-ZERO-RETIRE-BEFORE-READY | cap 2; A=0, B=1 (A), C=1 (A), D=2 | A 0..0, B 0..1, C 1..2, D 0..2; makespan 2; last node C (tie with D); DependencyPath `[A, C]`; ExecutionChain `[A start, B dependency, C resource]`. The first FILL starts A and D; only after COMPLETE(A) does B become ready. A Finish<=t mutant starts A and B first, then C, postpones D to 1..3 and returns makespan 3. |
| F5-ZERO-DRAIN | cap 1; A=0, B=0 (A), C=1 (B) | A 0..0, B 0..0, C 0..1; makespan 1; `[A, B, C]`; `[A start, B dependency, C dependency]`. |
| F5-ZERO-WAITS-FOR-SLOT | cap 1; A=2, Z=0 | A 0..2, Z 2..2; makespan 2; `L` is A (tie at 2, earlier declared); `[A]`; `[A start]`. |
| F5-ZERO-DECLARATION-ORDER | cap 1; P=0, Q=3, R=0 | P 0..0, Q 0..3, R 3..3; makespan 3; `[Q]`; `[Q start]`. R does not overtake Q. |
| F5-RESOURCE-SKIPS-ZERO | cap 1; A=2, Z=0, B=1 | A 0..2, Z 2..2, B 2..3; makespan 3; `[B]`; `[A start, B resource]`. Z finishes at B's start with zero duration. This row does NOT pin the `Duration(r) > 0` filter: A is declared before Z, so the declaration tie-break alone also picks A, and an arm without the filter answers identically. It is kept as the plain zero-drain-then-resource case; the next row is the discriminating one. |
| F5-RESOURCE-SKIPS-ZERO-DECLARED-FIRST | cap 1; Z=0 (A), A=2, B=1 | Z 2..2, A 0..2, B 2..3 (trace in declaration order: Z, A, B); makespan 3; `[B]`; `[A start, B resource]`. Z is declared first, depends on A, and finishes at 2, the instant B starts. Without the `Duration(r) > 0` filter the earliest-declared candidate finishing at 2 is Z and the arm answers `[A start, Z dependency, B resource]` (three steps, duration sum 3, contiguous, and still wrong: Z held the slot for no time). The frozen rule skips Z and names A, the node that actually occupied the slot up to 2. A zero-duration predecessor is observable only when the zero node is declared before the positive node that released the slot; no earlier-to-later-edge graph can produce that, which is why this row is hand-written. |
| F5-RESOURCE-TIE | cap 2; A=3, B=3, C=1 | A 0..3, B 0..3, C 3..4; makespan 4; `[C]`; `[A start, C resource]` (A and B both qualify; earliest declared). |
| F5-DUP-DEPENDENCY | cap 1; A=1, B=1 (A, A) | A 0..1, B 1..2; `Trace[B].BlockedBy == [A]`; makespan 2; `[A, B]`; `[A start, B dependency]`. Input `BlockedBy` still `[A, A]` after the call. |
| F5-DEP-SET-ORDER | cap 2; A=1, B=1, C=1 (B, A) | `Trace[C].BlockedBy == [A, B]`; A 0..1, B 0..1, C 1..2; makespan 2; `[A, C]` (A and B tie at 1; earliest declared); `[A start, C dependency]`. |
| F5-LAST-NODE-KEY-ORDER | cap 2; B=2, A=2 | B 0..2, A 0..2; makespan 2; `L` is B (tie at 2, earlier declared although A sorts first); `[B]`; `[B start]`. A key-sorted arm answers `[A]` / `[A start]` and fails. |
| F5-RESOURCE-TIE-KEY-ORDER | cap 2; B=3, A=3, C=1 | B 0..3, A 0..3, C 3..4; makespan 4; `[C]`; `[B start, C resource]` (B and A both qualify; B is earlier declared although A sorts first). A key-sorted arm answers `[A start, C resource]` and fails. |
| F5-BLOCKED-BY-KEY-ORDER | cap 2; B=1, A=1, C=1 (A, B) | `Trace[C].BlockedBy == [B, A]` (declaration index, not key, not input order); B 0..1, A 0..1, C 1..2; makespan 2; `[B, C]` (B and A tie at 1; B earlier declared); `[B start, C dependency]`. A key-sorted arm answers `[A, B]`, `[A, C]` and `[A start, C dependency]` and fails. |
| F5-MIXED-UNITS | cap 1; A=`1500ms`, B=`2s` | A 0..1.5s, B 1.5s..3.5s; makespan `3.5s` (3500000000ns); `[B]`; `[A start, B resource]`. An arm that quantizes to whole seconds (A becomes 1s or 2s) answers makespan 3s or 4s and fails; a seal that encodes durations as integer seconds cannot even express this row. |
| F5-NANOSECOND-GRAIN | cap 1; A=`1m30s`, B=`250ns`, C=`1.5s` (B) | A 0..90s, B 90s..90.00000025s (90000000250ns), C 90.00000025s..91.50000025s; makespan 91500000250ns; DependencyPath `[B, C]`; ExecutionChain `[A start, B resource, C dependency]`. Three units on one graph, a duration below one microsecond, and a dependency edge whose contiguity is only visible at nanosecond resolution: a whole-second arm gives B 90..90, C 90..91 and makespan 91s; a float-seconds arm risks `90.00000025 + 1.5 != 91.50000025` bit-exactly. Comparison is by parsed `time.Duration` value, so `"91.50000025s"` and `"91500000250ns"` are the same expectation. |
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
| F5-OVERFLOW-WRAP | cap 3; A=9223372036854775807ns, B=9223372036854775807ns, C=5ns | `ErrScheduleOverflow` although a cap-3 makespan of `MaxInt64` ns would fit. The wrapping `int64` sum is `+3` (`MaxInt64 + MaxInt64` wraps to `-2`, `-2 + 5 == 3`), so an arm that adds with `+=` and tests `sum < 0` at the end accepts the graph and schedules with wrapped timestamps; the per-add check refuses it at B. |
| F5-PRECEDENCE-CAP | cap 0; `"  "`=-1s (Z), A=1 (A), A=1 | ErrInvalidConcurrency. |
| F5-PRECEDENCE-BLANK | cap 1; `"  "`=-1s (Z), A=1 (A), A=1 | ErrBlankKey. |
| F5-PRECEDENCE-DUPLICATE | cap 1; K=-1s (Z), A=1 (A), A=1 | ErrDuplicateKey. |
| F5-PRECEDENCE-UNKNOWN | cap 1; K=-1s (Z), A=1 (A), C=1 | ErrUnknownDependency. |
| F5-PRECEDENCE-NEGATIVE | cap 1; K=-1s, A=1 (A), C=1 | ErrNegativeValue. |
| F5-PRECEDENCE-CYCLE | cap 1; K=1, A=1 (A), C=1 | ErrCycle. |
| F5-NEGATIVE-BEFORE-OVERFLOW | cap 1; A=9223372036854775807ns, B=9223372036854775807ns, C=-1ns | ErrNegativeValue; the overflowing positive prefix cannot preempt a later negative. |
| F5-CYCLE-BEFORE-OVERFLOW | cap 1; A=9223372036854775807ns (B), B=9223372036854775807ns (A) | ErrCycle; overflow cannot preempt the cycle. |
| F5-UNICODE-BLANK | cap 1; key consists of tab, CR, LF, U+00A0 and U+2003; duration 0 | ErrBlankKey. Encode literal escapes in JSON. |
| F5-EMPTY-KEY | cap 1; key `""`, duration 0 | ErrBlankKey. |
| F5-PADDED-KEYS-DISTINCT | cap 1; key `"A"` duration 1, key `" A "` duration 2 with dependency `"A"` | Trace `"A"` 0..1 then `" A "` 1..3, latter BlockedBy `["A"]`; makespan 3; dependency path `["A", " A "]`; chain `["A" start, " A " dependency]`. Keep input bytes. |
| F5-PADDED-LOOKUP-EXACT | cap 1; A=1, B=1 with dependency `"A "` | ErrUnknownDependency; trimming would incorrectly find A. |
| F5-ZERO-WIDTH-NONBLANK | cap 1; key U+200B duration 0 | Success: trace U+200B 0..0 with no dependencies, makespan 0, dependency path `[U+200B]`, chain `[U+200B start]`. |
| F5-EXACTLY-ONE-SENTINEL | any invalid input | `errors.Is` is true for exactly one entry of `SchedulerSentinels` and false for the other six and for `ErrNotImplemented`. |
| F5-PURE | any valid input, called twice, in parallel goroutines under `-race` | Equal complete `Schedule` values under the fixture equality rule (nil and empty slices equivalent); input graph unchanged. Canonicalize list representations before any byte comparison. |

The F5-EXACTLY-ONE-SENTINEL and F5-PURE rows are harness properties applied
to their relevant cases, not single graph fixture files. All concrete graph
rows (success and refusal) above are mandatory named fixture files.

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
Every hand fixture feeds arm A and arm B; no fixture is arm-specific.
Hand fixtures never go through tickOracle. On success compare makespan,
all trace fields and both chains by parsed duration/value equality. On error
assert exactly the named sentinel, every other registry sentinel absent, and
zero Schedule. Unknown keys at every object depth, malformed durations, wrong
JSON types, or nonempty unknown `expect.error` names are loader errors, never
expect-success cases. Only the literal empty error name means success; false
from LookupSchedulerSentinel on a nonempty name must fail the loader.
FC-SCHED-SEALS must test loader refusal with an unknown name such as
ErrScheduleOverflowed and unknown nested keys, plus valid empty/sentinel controls.

## Independent small-graph oracle

Name: `tickOracle`, independently written by FC-SCHED-SEALS inside the
external scheduler test package and imported by neither arm. It advances one
whole-second tick at a time using remaining-duration counters rather than
jumping to the next event. At each tick it performs the same explicit
COMPLETE/FILL phase boundaries and completed-state readiness defined above;
zero counters started in FILL retire only in the next COMPLETE phase. It then
decrements positive running counters and advances one tick. It derives both
chains from its own trace. It is defined only for the generated valid corpus,
never hand fixtures, error cases, overflow cases or non-tick durations.

The mandatory deterministic corpus is exhaustive for N=1..4: durations in
{0,1,2,3} ticks; every subset of directed edges (i,j) with i<j; cap=1..N+1.
Enumerate N ascending, duration tuples lexicographically, edge bit masks in
ascending order over lexicographic (i,j), then cap ascending. Assign keys in
reverse lexical declaration order (N-i rendered as a single decimal digit).
Case counts by N are 8, 96, 2048 and 81920, totaling 84072; add the empty graph
at cap1 for 84073. An optional bounded N=5 extension is not a substitute and
must not silently turn this into an exhaustive multi-million-case product.
Record actual corpus counts and elapsed time in the seals/adjudication notes;
no machine-independent performance SLA is claimed. Validate fixture generation
before launching arm comparisons. Every case compares complete trace, paths,
makespan and error against each arm, not just makespan. Distinct event-driven
arms must not import or copy the ticking oracle. Disagreements are minimized
and adjudicated; matching the sibling is never itself a correctness argument.

The generated corpus is blind by construction to three frozen rules, and
the seals MUST NOT treat oracle agreement as evidence for them. Coverage of
each comes only from the hand-written row named, which the seals encode as
a fixture and assert directly against both arms:

- The `Duration(r) > 0` filter on `EdgeResource` predecessors. With edges
  restricted to earlier-declared to later-declared nodes, a zero-duration
  node finishing at instant `t` always has a positive-duration node
  finishing at `t` declared before it (the node whose completion made it
  ready or released its slot), so the declaration tie-break alone selects
  the same predecessor and the filter is never observable. Exhaustively
  checked for every corpus-shaped graph with `N <= 4` (84,072 graphs, zero
  discriminate). Covered by F5-RESOURCE-SKIPS-ZERO-DECLARED-FIRST, whose
  zero node is declared before the node that released the slot.
- Reverse-declaration dependency edges. The corpus never generates one, so
  an arm that refuses or mis-validates forward references matches the whole
  generated corpus. Covered by F5-REVERSE-DECLARATION. The bound stays as it
  is because it keeps generated graphs acyclic without a cycle detector in
  the oracle; the seals may additionally extend the oracle to accept
  reverse edges, but that is not a substitute for the fixture.
- Non-tick durations. The tick oracle is an integer-tick process and is
  defined ONLY over its own generated corpus, where every duration is a
  small whole number of ticks encoded as whole seconds. It is never applied
  to error rows, to overflow rows, or to hand fixtures whose durations are
  not whole seconds. Nanosecond arithmetic in the arms is covered by
  F5-MIXED-UNITS and F5-NANOSECOND-GRAIN, which the seals compare by parsed
  `time.Duration` value against both arms. A seal suite whose fixture
  encoder or comparison only handles whole seconds does not satisfy this
  section.

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

## Historical native validation (before operator correction)

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
running implementation (the panel-iteration rows F5-SIMULTANEOUS-COMPLETIONS,
F5-LAST-NODE-KEY-ORDER, F5-RESOURCE-TIE-KEY-ORDER, F5-BLOCKED-BY-KEY-ORDER,
F5-OVERFLOW-WRAP, F5-RESOURCE-SKIPS-ZERO-DECLARED-FIRST,
F5-REVERSE-DECLARATION, F5-MIXED-UNITS and F5-NANOSECOND-GRAIN were
additionally checked against the named mutant process, a no-zero-filter
mutant and a whole-second quantizing mutant, by a throwaway simulator of
the frozen process, and shown to differ where the row says they differ);
the first executable check is the oracle FC-SCHED-SEALS
writes, and any row it contradicts goes to FC-SCHED-ADJ as a contract
dispute rather than being silently corrected in an arm.


## Operator correction of the stalled panel

Parent correction starts at adb20005917c0f946e061282b5c860d6af11eafd.
This changes contract prose and examples only; public declarations, sentinel
identities/registry, arm stubs, tests and fixtures are unchanged. Native Blocked
verdicts remain historical. Independent review and then independent seals are
still required. Existing row model pins are unchanged; this correction is
operator-authored, not a claimed Fable session.

| Latest finding | Disposition |
|---|---|
| Claude High zero readiness | Completed-state readiness and COMPLETE/FILL phase boundaries are authoritative; discriminating trace and both paths added. |
| Claude Medium duplicate authority | Behavioral authority centralized here; Go comments now name shape and point here. |
| Claude Low ladder filename | Six distinct named fixtures replace the multi-case ladder; overflow combinations are separate. |
| Codex High overflow precedence | Negative-plus-overflow and cycle-plus-overflow refusals added. |
| Codex Medium fixture/oracle domain | Hand fixtures drive arms only; oracle restricted to its generated valid corpus. |
| Codex Low reverse-edge unique claim | Removed the false uniqueness claim. |
| Grok High key semantics | Unicode blankness predicate, byte-exact identity/lookup and discriminating padded/zero-width examples added. |
| Grok Medium no-progress loop | Explicit finite ErrCycle refusal on impossible progress; valid DAGs cannot reach it. |
| Grok Low corpus size | Exact exhaustive N<=4 corpus of 84073 cases; actual timings required, no invented SLA. |
| Earlier unknown sentinel Medium | Loader must reject every nonempty unknown name; independent loader controls/mutation required. |
| Earlier nil/empty purity Medium | Purity compares canonical-equivalent values; no conflicting raw-byte requirement. |

Remaining limit: no scheduling algorithm is implemented here. Fixture/oracle
acceptance and mutation demonstrations belong to the independent seals row.

Operator validation: full build/vet/race passed without exclusions; all 58 protected tests/fixtures/register files unchanged; noncomment Go code lines identical to the starting head. A local synthetic phase simulation reproduces makespan2 for completed-state readiness and makespan3 for the former Finish-only reading; proof archived under operator-scheduling-resolution/zero-readiness-proof.json. This explanatory probe is not a substitute for independent seals.
