# Task Assignment: forecast dispatched agent work

Tasker breakdown of `BRIEF-dispatched-work-forecasting.md`. The brief states the
four defects; this states what gets built, in what order, under whose rulings.

## Rulings on record

| question | ruling | by |
|---|---|---|
| Empty `(role, model)` cells | Refuse to predict; report coverage instead of a number | operator |
| Reference-class source | Union of live YAML and git-history YAML, joined to journals | operator |
| Blocked rows | Right-censored; block rate reported as its own figure, not folded into duration | tasker, operator deferred |
| First unit | Extraction + coverage report | tasker, operator deferred |

## Measured before assigning

**Historical snapshot:** these source counts predate multi-repository extraction.
The [2026-09-04 correction report](FC1-CORRECTIONS.md) supersedes the claim that
no bodies/adjudicate observations exist and records current wallet coverage.

Facts the brief did not have. Re-derive rather than trust; each is a
measurement, not a rule, and belongs here rather than in any contract.

* Wallet v2 is **20 dependency levels**, not 13. Wave sizes
  `[1,13,13,1,10,12,5,3,1,1,2,1,1,2,1,1,2,1,1,1]` — eleven single-row waves,
  and the last twelve waves are near-serial. Confirm against
  `dispatcher.cli run --mode dry-run` before relying on it.
* **`role` is absent from every journal event.** 187 `task_started` payloads
  carry `labels`, `model`, `summary`, `dependencies_unresolved` — no role. Role
  exists only in the tasks YAML.
* Reference class is **~40 timed rows**: 29 from live YAML, 35 from git
  history, overlapping. Not 150.
* **Zero observations of `bodies` and `adjudicate`** in either source, at any
  model. Those roles are 34 of wallet's 73 rows.
* Terminal outcomes across all runs: **42 done, 126 blocked.**
* 72 of 187 `task_started` events carry no model at all.
* Baseline gate is green: `go test ./...` passes, single module at repo root.

## Sub-tasks

XL splits along the seam. Contract-first: the scaffold is authoritative and a
change to it is a recorded DEVIATION, not a local fit.

| key | role | size | depends on | deliverable |
|---|---|---|---|---|
| FC-GATE | — (hand-done) | S | — | `.dispatcher.yaml`, `.agent/risk-paths.json` |
| FC-SCAFFOLD | scaffold | M | FC-GATE | Record type + reference-class and scheduler interfaces |
| FC-1 | bodies | M | FC-SCAFFOLD | Extraction, union source, coverage report |
| FC-2 | bodies | L | FC-SCAFFOLD | Graph simulator — longest path under a concurrency cap |
| FC-3 | bodies | M | FC-1 | Round/review-time model; empirical bootstrap sampling |
| FC-4 | bodies | M | FC-2, FC-3 | `dispatched-predict` CLI, percentiles, critical path |

FC-1 and FC-2 fan out in parallel once the scaffold lands — FC-2 depends on the
record *type*, not on any data.

**Differential testing: FC-2 only.** Longest-path-under-a-cap has no reference
implementation in this repo, and a correlated off-by-one survives a
self-written second version. Two coders, separate worktrees, same spec
verbatim, neither told the other exists. A disagreement is an ESCALATION for a
spec decision, not an iteration.

---

## FC-GATE — prerequisite, hand-done

**Risk:** Low. **Not dispatched:** nothing can be dispatched or classified from
`forecast` until it exists.

`forecast` has no `.dispatcher.yaml` and no `.agent/risk-paths.json`, so
`cmd/classify` cannot run against a diff here and a tasks list has nowhere to
live (the dispatcher derives the repo from the tasks file's location).

Unlike claude-workflow's seven modules, `forecast` is one module at the root, so
the gate is `go test ./...` directly. Include `-race`. Exclude `go build` —
`./forecast` is a tracked 26MB binary and building it dirties the worktree,
which the dispatcher refuses evidence from.

---

## FC-SCAFFOLD — the contract

**Risk:** Medium. **Objective:** define the record every later unit reads and
writes, and the two interfaces they meet at.

CLAUDE.md's most expensive lesson applies directly here, and this scaffold is
exactly the shape that has burned before. **State rules, not measurements.** No
sentence asserting how many rows the reference class holds, which cells are
empty, or that bodies has never been observed. Every one of those is true today
and false after the next run. They live in this document and in tests.

The test: *if someone fixed the defect this contract describes, would any
sentence here become false?* If yes, it is a measurement — move it.

### Required shape

A dispatched observation, whatever it is named, must distinguish:

* elapsed wall clock, and whether that elapsed time is a **completed duration
  or a lower bound**
* the terminal outcome, as a closed set
* the bucket key `(role, model)` — with the stamped model, not the authored one
* round count
* provenance: which run and which source revision it came from

A reference class must be able to answer "how many observations in this cell"
for a cell it has **none** of. A cell absent from a map is not the same as a
cell with n=0, and the refuse-to-predict ruling depends on telling them apart.

### Error semantics

Wrap with `%w` and a sentinel; never discard. An error a caller cannot match on
is an error the coverage report cannot classify. At minimum a sentinel for a
row that cannot be attributed to a cell, and one for a source revision that
cannot be parsed.

---

## FC-1 — extraction, union source, coverage report

**Risk:** Medium. **Objective:** build the reference class from every recoverable
dispatched row and report what it does and does not cover.

### Inputs

| source | holds | does not hold |
|---|---|---|
| `~/Project/dispatcher-runs/*/journal.jsonl` | per-event timestamps, stamped model, labels, rounds, cost, tokens | role |
| `claude-workflow/features/**/*.yaml` (live) | role, `started_at`, `completed_at`, `iteration_count`, `dispatcher_run_id` | rows since overwritten |
| the same files at every revision in git history | rows since overwritten | rows never committed while stamped |

Join on `dispatcher_run_id` + task key. Port
`claude-workflow/features/model-matrix/report.py::journal_facts()` rather than
rewriting it — it computes the dev/review split, round count and token totals
per row and is known to work.

### Edge cases — each needs explicit handling AND a test

1. **`started_at` with no `completed_at`** (in-progress, or a crashed run). Not
   a duration. Must not become one.
2. **A blocked row.** Right-censored: elapsed is a lower bound, terminal outcome
   is recorded, and it never enters a duration mean. A row that burned 8 rounds
   and produced nothing must never surface as a fast row.
3. **Cascade.** The journal carries `agent_fallback` events; on cascade the
   dispatcher rewrites the row with what actually ran. Read the **stamped**
   model. A row whose stamp and authored pin disagree is either attributed to
   the stamp or excluded — never recorded under the authored name. An earlier
   comparison was invalidated by exactly this.
4. **The same `(key, started_at)` in several git revisions.** Dedupe. Two
   revisions giving *different* stamps for one tuple is a re-run reusing a key —
   detect and surface it; last-write-wins silently discards a real observation.
5. **A cell with zero observations.** Present in the reference class with n=0.
   Omitting it makes the empty cell indistinguishable from a cell nobody asked
   about, and the refuse-to-predict ruling cannot be honoured.
6. **A `task_started` with no model** (72 of 187 observed). Unattributable to a
   `(role, model)` cell. Count it in the report; do not guess the model.
7. **Journal rows with no recoverable YAML revision.** The count of started rows
   exceeds what any join recovers. Report the shortfall as a number rather than
   dropping it silently — it is the honest denominator for the coverage claim.
8. **Timestamps carry UTC offsets** (`-07:00` observed). Compare as instants.
9. **A hand-finished row.** No field marks one, and several past rows were
   finished by hand after blocking. Their wall clock is not agent throughput and
   the data cannot distinguish it. State the limit in the report; do not model
   around it.

### Output

`forecast dispatched-reference build --runs-dir … --out …`, plus a coverage
report that is the unit's actual deliverable:

* per `(role, model)`: n_done, n_blocked, n_censored, duration summary, rounds
* the cells a named tasks YAML requires, and which of them are empty
* the share of target rows falling in a covered cell
* rows recovered vs rows started, with the shortfall named

A prediction is not in scope for this unit. The report is the answer to
"is the reference class good enough to predict wallet", and the expected answer
is no for 47% of it.

### Definition of Done

- [ ] Every edge case above has explicit handling and a test
- [ ] Censored rows provably excluded from duration statistics — a test that
      fails if a blocked row's elapsed time reaches a mean
- [ ] Empty cells representable and reported
- [ ] Error types wrap with `%w` and a sentinel
- [ ] `go test ./... -race` green on the committed tree
- [ ] Coverage ≥ 80%
- [ ] No stub where a real implementation was required

---

## Held for later units

**FC-2** takes the graph as input and simulates it: sample a duration per row,
compute the longest path under a concurrency cap. Read `blockedBy` from the
tasks YAML rather than re-deriving waves. Differential testing applies.

**FC-3** models `row_wall_clock = dev + (rounds × review_per_round)` with rounds
sampled per cell, and replaces the normal-plus-clamp with an empirical
bootstrap. The clamp at `simulator.go:100` is the tell that the distribution is
wrong; a resample makes it disappear.

**FC-4** is the CLI, the percentiles, and the critical path — which rows are on
the chain, because a p95 nobody can shorten is a number and a p95 plus the chain
is a plan.

## Validation, not to be skipped

Hold a run out. Build from dogfood-go and model-matrix, predict wallet, compare
when it lands. If p80 does not contain the outcome, say so rather than tuning
until it does.
