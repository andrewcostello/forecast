# FC-SCHED-SEALS: independent F5 schedule seals

Contract section: F5 of `docs/plans/forecasting-contracts.md`. Deviation: none.
This row adds tests and synthetic fixtures only. It does not edit either
scheduler implementation, the shared declarations, or `config/known-red.yaml`.

## Deviation

None. The blocked findings exposed missing discriminators and harness checks;
they did not require changing the frozen F5 contract.

## Authority and panel disposition

The seal was written against accepted scaffold head
`80f5f7da5d24591a659aec8c495234e7a65ec395`, its exact-head panel at
`2026-09-06T12-54-03Z-FC-SCHED-SCAFFOLD-operator-panel`, and the mandatory
second-panel ruling under `operator-scheduling-resolution/`. The panel result
was consensus APPROVE with Grok dissent retained; this note does not claim
unanimity.

The parser High and named fractional-duration concern were independently
disproved on the installed Go 1.26 toolchain. The fixtures therefore preserve
`9223372036854775807ns`, `4611686018427387904ns`, and exact fractional values.
Loading all 49 fixtures is an executable round-trip check in both contract
groups. `ErrScheduleOverflowed` is intentionally an unknown near-miss in loader
mutation coverage. Generated edge `(i,j)` with `i<j` means node `j` lists node
`i` in `blocked_by`.

The first-offending-node name remains diagnostic guidance, not an observable
acceptance surface: no structured error carrier or message layout is frozen.
The seals assert exact single-sentinel identity and record this limitation
instead of inventing a public error type.

## Files and fixture provenance

- `internal/dispatched/scheduler_contract_test.go` is an external-package
  harness. Each top-level group calls only its named arm. The shared runner
  supplies identical fixtures and generated inputs independently to A and B.
- `internal/dispatched/testdata/scheduler/cases/*.json` contains 49 explicitly
  synthetic hand fixtures. No recorded schedule exists and none is claimed;
  every file carries `"provenance":"synthetic"` and no credential or network
  input.
- Fixture names and the complete sorted filename set are frozen in the test.
  Loading is presence-aware and rejects duplicate or unknown keys at every
  object depth, missing/null required values, wrong types, malformed durations,
  invalid edge/provenance, filename disagreement, nonempty error outputs, and
  unknown sentinel identifiers before an arm callback can run.

Success comparison is positional and covers exact `time.Duration` values,
every trace key/start/finish/normalized dependency, makespan, dependency path,
execution-chain key and edge. Nil and empty lists are canonical equivalents on
success. Refusals require an exact zero `Schedule`, the named sentinel, none of
the other six scheduler sentinels, and no `ErrNotImplemented` match. Every valid
hand graph is also called twice concurrently with its original cap; results
must be equal and the input graph must remain byte-for-byte equivalent.

## Prior-attempt disposition and cascade corrections

KEEP AND FIX. The reviewed attempt's external-fixture/oracle/isolated-child
shape matches F5, so discarding it would lose already validated coverage. This
correction retains that shape and closes the panel's discriminating gaps:

- F5-NANOSECOND-GRAIN now starts with `130841899126ns`; its complete expected
  trace differs from the conventional float-seconds finish mutant by one or
  more nanoseconds. The mutant is constructed and rejected by the executable
  `panel-discriminator-mutations` subtest.
- F5-NEGATIVE-CAP distinguishes `maxParallel < 1` from `maxParallel == 0`.
  F5-OVERFLOW-BOUNDARY accepts a total duration exactly equal to `MaxInt64` and
  distinguishes the required strict `>` guard from a `>=` guard.
- Fixture integrity now replays COMPLETE/FILL against every successful trace,
  including zero-duration slot occupancy, and independently reconstructs both
  explanation lists with declaration-order ties. Dedicated expectation mutants
  prove cap overlap, zero work bypassing an occupied slot, a shorter join
  predecessor, and a later resource tie are rejected.
- The large-cap subprocess emits an arm-specific success handshake only after
  the scheduler and allocation checks return. The parent rejects exit-zero
  skips, empty selections, unarmed runs, and wrong-arm handshakes; fixture I/O
  is inside filtered subtests and is not performed by the child.

F5-CAP-BINDS still records cap 1, independent A=2s/B=3s, makespan 5s, the full
trace, dependency path `[B]`, and execution chain `[A start, B resource]`. The
fixture-integrity subtest freezes the 49-file raw fingerprint
`b1bd84363de02d37a0852995200def2ace87cc9f0aa00e11a04dbe5b6857cda6`, the
26-success/23-refusal split, full schedule expectations, exact overflow strings,
and their parsed integer nanosecond values before either arm runs.

## Recorded cases and mutation ledger

Every success row below records trace intervals in declaration order followed
by `makespan; dependency path; execution chain`. Every refusal records its exact
sentinel. In addition to the semantic mutant named per row, the executable
`fixture-expectation-mutations` probe changes every success makespan by one
nanosecond and substitutes a wrong sentinel for every refusal; all 49 mutated
expectations are rejected. `comparison-mutations` separately proves trace
position/time, normalized `BlockedBy`, dependency-path, execution-edge, and
makespan changes are observed.

| Fixture | Frozen expected outcome | Concrete semantic mutation killed |
|---|---|---|
| F5-CAP-BINDS | `A 0..2, B 2..5; 5; [B]; [A start, B resource]` | Ignore the cap and start B at 0; trace, makespan, and resource edge differ. |
| F5-CAP-FREE | `A 0..2, B 0..3; 3; [B]; [B start]` | Serialize all ready work; B starts at 2 and a resource edge appears. |
| F5-FORK-JOIN-FREE | `A 0..1, B 1..3, C 1..4, D 4..5; 5; [A,C,D]; dependency-only chain` | Start only one fork despite a free slot; C and downstream times differ. |
| F5-FORK-JOIN-CAP1 | `A 0..1, B 1..3, C 3..6, D 6..7; 7; [A,C,D]; [A start,B dependency,C resource,D dependency]` | Label C's slot wait as dependency; the complete execution edge differs. |
| F5-CAP-OVER-N | Same full schedule as F5-FORK-JOIN-FREE at cap 10 | Reject `cap>N`; expected success fails. |
| F5-READY-DECLARATION-ORDER | `B 0..3, A 3..5; 5; [A]; [B start,A resource]` | Sort ready keys; A takes the first slot. |
| F5-REVERSE-DECLARATION | trace `A 3..5, B 0..3; 5; [B,A]; dependency chain` | Treat a dependency declared later as unknown/cyclic, or return trace in execution order. |
| F5-SIMULTANEOUS-COMPLETIONS | `X/Y 0..2, P/Q 2..3, R 3..4; 4; [X,R]; [Y start,P dependency,R resource]` | Complete X and fill before completing Y; R starts at 2 and Q moves to 3. |
| F5-ZERO-RETIRE-BEFORE-READY | `A 0..0, B 0..1, C 1..2, D 0..2; 2; [A,C]; [A start,B dependency,C resource]` | Treat stored `Finish<=t` as completion during FILL; C starts at 0 and D finishes at 3. |
| F5-ZERO-DRAIN | `A/B 0..0, C 0..1; 1; [A,B,C]; dependency chain` | Advance a positive tick between zero completions; B or C starts late. |
| F5-ZERO-WAITS-FOR-SLOT | `A 0..2, Z 2..2; 2; [A]; [A start]` | Let zero work bypass the occupied slot; Z starts at 0. |
| F5-ZERO-DECLARATION-ORDER | `P 0..0, Q 0..3, R 3..3; 3; [Q]; [Q start]` | Prioritize all zeros over declaration order; R starts at 0. |
| F5-RESOURCE-SKIPS-ZERO | `A 0..2, Z 2..2, B 2..3; 3; [B]; [A start,B resource]` | Fail to drain Z at time 2 before starting B; B starts later. |
| F5-RESOURCE-SKIPS-ZERO-DECLARED-FIRST | trace `Z 2..2, A 0..2, B 2..3; 3; [B]; [A start,B resource]` | Admit zero-duration resource predecessors; chain incorrectly includes Z. |
| F5-RESOURCE-TIE | `A/B 0..3, C 3..4; 4; [C]; [A start,C resource]` | Pick the later finisher declaration B as the resource predecessor. |
| F5-DUP-DEPENDENCY | `A 0..1, B 1..2` with trace dependency `[A]`; `2; [A,B]; dependency chain` | Preserve duplicate dependencies in `NodeTrace.BlockedBy`. |
| F5-DEP-SET-ORDER | `A/B 0..1, C 1..2` with trace dependencies `[A,B]`; `2; [A,C]` | Preserve input dependency order `[B,A]` rather than declaration order. |
| F5-LAST-NODE-KEY-ORDER | `B/A 0..2; 2; [B]; [B start]` | Lexically choose A for the finish tie. |
| F5-RESOURCE-TIE-KEY-ORDER | `B/A 0..3, C 3..4; 4; [C]; [B start,C resource]` | Lexically choose A as C's resource predecessor. |
| F5-BLOCKED-BY-KEY-ORDER | `B/A 0..1, C 1..2`, C dependencies `[B,A]`; `2; [B,C]; [B start,C dependency]` | Sort keys and return `[A,B]` / `[A,C]`. |
| F5-MIXED-UNITS | `A 0..1.5s, B 1.5..3.5s; 3.5s; [B]; resource chain` | Quantize to whole seconds; exact times and makespan differ. |
| F5-NANOSECOND-GRAIN | `A 0..130841899126ns, B ..130841899376ns, C ..132341899376ns; 132341899376ns; [B,C]; resource+dependency chain` | Compute finishes through float seconds; the executable mutant loses nanosecond precision and the full trace differs. |
| F5-EMPTY | exact zero schedule with nil/empty-equivalent lists | Return a nonempty trace/path or refuse the empty graph. |
| F5-PADDED-KEYS-DISTINCT | `A 0..1, " A " 1..3; 3; [A," A "]; dependency chain` | Trim keys before identity/lookup and collapse the two nodes. |
| F5-ZERO-WIDTH-NONBLANK | U+200B `0..0; 0; [U+200B]; [U+200B start]` | Treat U+200B as Unicode whitespace and return ErrBlankKey. |
| F5-EMPTY-BAD-CAP | `ErrInvalidConcurrency` | Validate the empty graph before its cap and accept it. |
| F5-BLANK-KEY | `ErrBlankKey` | Check only `key==""` and accept spaces. |
| F5-BLANK-DEPENDENCY | `ErrUnknownDependency` | Apply node blankness to dependencies and return ErrBlankKey. |
| F5-WHITESPACE-DEPENDENCY | `ErrUnknownDependency` | Trim dependency bytes and classify them as a blank node key. |
| F5-DUPLICATE-KEY | `ErrDuplicateKey` | Deduplicate nodes silently and schedule one A. |
| F5-UNKNOWN-DEPENDENCY | `ErrUnknownDependency` | Drop unknown edges and schedule A. |
| F5-NEGATIVE | `ErrNegativeValue` | Clamp negative duration to zero. |
| F5-NEGATIVE-CAP | `ErrInvalidConcurrency` | Check only `maxParallel == 0`; negative concurrency is accepted or reaches scheduling. |
| F5-SELF-DEPENDENCY | `ErrCycle` | Remove self edges while normalizing sets. |
| F5-CYCLE | `ErrCycle` | Use declaration order as a presumed topological order and miss the cycle. |
| F5-OVERFLOW | `ErrScheduleOverflow` | Check only realized cap-2 makespan, which remains representable. |
| F5-OVERFLOW-BOUNDARY | `A 0..9223372036854775807ns; MaxInt64; [A]; [A start]` | Use `d >= MaxInt64-sum`; the representable exact-boundary graph is incorrectly refused. |
| F5-OVERFLOW-WRAP | `ErrScheduleOverflow` | Sum in `int64` and test only final negativity; the sum wraps to positive 3. |
| F5-PRECEDENCE-CAP | `ErrInvalidConcurrency` | Scan graph blankness before concurrency. |
| F5-PRECEDENCE-BLANK | `ErrBlankKey` | Scan duplicates before blank keys. |
| F5-PRECEDENCE-DUPLICATE | `ErrDuplicateKey` | Scan unknown dependencies before duplicate keys. |
| F5-PRECEDENCE-UNKNOWN | `ErrUnknownDependency` | Scan negative durations before unknown dependencies. |
| F5-PRECEDENCE-NEGATIVE | `ErrNegativeValue` | Detect cycles before negative durations. |
| F5-PRECEDENCE-CYCLE | `ErrCycle` | Check overflow before cycle detection. |
| F5-NEGATIVE-BEFORE-OVERFLOW | `ErrNegativeValue` | Stop on the overflowing positive prefix before scanning all negatives. |
| F5-CYCLE-BEFORE-OVERFLOW | `ErrCycle` | Check the duration sum before graph cycles. |
| F5-UNICODE-BLANK | `ErrBlankKey` | Use ASCII-only trimming and accept NBSP/U+2003. |
| F5-EMPTY-KEY | `ErrBlankKey` | Omit blank-key validation. |
| F5-PADDED-LOOKUP-EXACT | `ErrUnknownDependency` | Trim `"A "` during lookup and incorrectly resolve A. |

## Independent bounded oracle

`tickOracle` is test-local and imported by neither arm. It uses remaining-tick
counters, explicit COMPLETE/FILL phase boundaries, completed-state readiness,
and repeated same-tick zero drains. It does not jump to event times. A direct
ZERO-DRAIN construction self-checks the sole hand-case exception before corpus
comparison.

The generated corpus is exhaustive and deterministic for N=1..4, durations
`{0,1,2,3}` seconds, every subset of earlier-to-later edges, and caps `1..N+1`,
plus the empty graph at cap 1. Counts are 1, 8, 96, 2,048 and 81,920 by N=0..4,
for exactly 84,073 cases. Enumeration is N, lexicographic duration tuple,
ascending edge mask over lexicographic `(i,j)`, then cap; keys are reverse
lexical declaration order. The frozen corpus fingerprint is
`9f0df57ac996a7326a3dffb7a58dd63394eeddb2e32517c106294e637b5125bf`.
The oracle domain is fully walked and checked for trace order/durations,
dependency readiness, cap occupancy, known path keys, dependency-path bound,
and execution-chain duration before arm comparisons begin. On this worker,
the non-race 84,073-case oracle-domain subtest completed in 0.09s (package
elapsed 0.099s); no machine-independent performance threshold is asserted.

The generated corpus deliberately does not prove reverse-declaration edges,
non-tick durations, or exclusion of zero-duration resource predecessors; the
three named hand fixtures remain the authority for those rules. Corpus arm
comparison includes every trace field, makespan, dependency path and execution
chain, never only makespan. It streams cases rather than retaining an unbounded
product; no N=5 extension is enabled.

## Large-cap storage seal

Each arm group starts a bounded child instance of the current test binary on a
one-node, one-nanosecond graph with `maxParallel=math.MaxInt`. An exact
slash-qualified test selector, explicit child flag, and matching environment
arm are all required before the child call can run; ambient environment values
or broad selectors cannot arm it or recurse. The child has a 256 MiB Go memory
limit, an 8 MiB per-call allocation/retained-heap ceiling, a
20-second child test timeout, and a 30-second parent context. The parent waits
for cleanup and reports refusal, recovered panic, timeout/kill, excessive
allocation, abnormal exit, or missing arm-specific handshake as failure. The
expected result is the same full one-node schedule as any cap `>=N`. This kills
a cap-sized slice/map buffer without exposing the parent test process to OOM.
POSIX cancellation kills the child process group. The non-POSIX fallback kills
the direct child; the probe is CPU-only scheduler code and starts no descendants.

## Red/green and mutation evidence

- Loader controls, all loader mutations, registry-copy isolation, oracle
  ZERO-DRAIN, the complete generated-domain validation, corpus fingerprint,
  per-fixture expectation mutations, and complete-schedule comparison mutations
  pass without calling either implementation.
- `TestFCScheduleAContract` is red: arm A returns `ErrNotImplemented` on the
  first capability calls, including generated and large-cap checks.
- `TestFCScheduleBContract` is red for the same independent reason. No
  capability or regression assertion is skipped, and there is no xfail,
  `t.Skip`, implementation change, or blanket exclusion. The internal
  child-only branch returns without work unless its exact isolated-subprocess
  protocol is present; its parent `large-cap-storage` assertion remains active.
- Existing non-scheduler regressions are run separately and remain green. Build
  and vet results are recorded after final verification below.

The blocked panel executed a throwaway faithful arm against the complete group
and demonstrated kills for zero-retire readiness; declaration-order last-node,
resource, BlockedBy, and FILL choices; wrap-sum overflow; reverse edges; whole-
second quantization; the positive-duration resource filter; precedence; duplicate
dependencies; cap-sized storage; execution-order trace; and caller-slice
mutation. Its original float mutant survived, which is why that fixture changed.
The corrected float mutant plus the negative-cap equality and strict-overflow-
boundary mutants are now executable harness subtests and pass only when the
fixture rejects them. All 49 per-case expectation mutations also pass. Ledger
rows outside those named body-level mutations have assertion-level mutation
evidence only; they were not executed against a complete scratch scheduler in
this correction and are not claimed as such. Adjudication must rerun every
semantic body mutant once the owned arm implementations exist.

## Verification

- `go build ./...`: PASS.
- `go vet ./...`: PASS.
- `go test ./internal/dispatched -race -count=1 -run
  '^TestFCSchedule(A|B)Contract$/^(fixture-corpus-integrity|fixture-loader|sentinel-registry|oracle-zero-drain|generated-corpus-shape|generated-oracle-domain|comparison-mutations|fixture-expectation-mutations|panel-discriminator-mutations)$'`:
  PASS in 3.961s. Both independent groups loaded the exact 49-file fingerprint,
  validated all 84,073 oracle cases, and rejected all per-case, full-schedule,
  integrity, and panel discriminator mutations without calling an arm.
- `go test ./... -race -count=1 -skip '^TestFCSchedule(A|B)Contract$'`: PASS in
  12.335s for `internal/dispatched`; all other module packages passed or had no
  tests. This also ran the standalone large-cap child-isolation regression.
- `env -u DISPATCHER_KNOWN_RED_FILE go test ./internal/dispatched -race -count=1
  -run '^TestFCSchedule(A|B)Contract$'`: expected RED in 2.976s. Both groups
  failed only when they reached the arm stubs: all 49 hand fixtures received
  `ErrNotImplemented`, the generated corpus stopped at its first arm call, and
  each authenticated large-cap child reported the same stub refusal. All
  pre-arm integrity and mutation subtests passed.
- `go test ./internal/dispatched -race -count=1 -run
  '^TestFCSchedule(A|B)Contract$/^large-cap-storage$'`: expected RED in 0.030s;
  both isolated children ran and reported only their arm's `ErrNotImplemented`
  refusal.
- `gofmt -l` on the three Go seal files: PASS (no output).
