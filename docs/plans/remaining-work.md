# Remaining forecast work: contract, seals, implementation, adjudication

This revision covers all remaining dispatched-forecasting and YouTrack work.
They remain separate epics. This is a worklist redesign, not a declaration that
any implementation or review has passed.

## Authority and existing work

The active worklists are `features/dispatched-forecasting/tasks.yaml` and
`features/youtrack-support/tasks.yaml`. Each has an exact pre-redesign snapshot
under `archive/`, including the FC-1 Blocked verdict, review head, and cost.
The archived files are evidence, not dispatch inputs.

FC-1 code at `d529265` is retained as the forecasting baseline. Its panel at
`../dispatcher-runs/2026-09-04T22-34-59Z-FC-1-panel/` blocked with four High
findings. No historical status or verdict is relabeled as a pass. The active
FC-SCAFFOLD and FC-1 rows are explicitly reopened for revised work, with links
to their prior evidence. All original agent/model/effort pins are retained.

The contracts linked below supersede contradictory instructions in the old
brief and assignment. Measurements are dated evidence, never expected results
an implementation must produce. Bodies must record every contract deviation;
adjudication accepts it with a reason or requires a correction.

- [Observation, scheduling, sampling, prediction](forecasting-contracts.md)
- [Tracker compatibility, adapters, wiring, commands](youtrack-contracts.md)
- [Acceptance and finding traceability](traceability.md)
- [Execution and external prerequisites](execution.md)

## Working sequence

Each unit has a contract/scaffold row, an independent seals row, bounded bodies
rows, and adjudication. A scaffold defines behavior, data ownership, error
states, and compilation seams; it does not implement its own acceptance tests.
A seals row uses frozen contracts and real event/API fixtures, rather than
copying the body's assumptions into mocks. No new live API calls belong in a
seal. Independent sessions matter; different model families alone are not
proof of independence.

Existing regression tests stay green unless a named contract amendment changes
the expected behavior. The seal author makes that exact amendment and records
why. New missing-capability cases are red; passing legacy compatibility checks
are mutation-verified, not artificially broken to make every test red.
Bodies never weaken seals or edit their fixtures. A blocked body returns to
contract adjudication when the required behavior is ambiguous; it does not
invent a new policy during a fix loop.

Parallel work is allowed only for disjoint implementation files with frozen
shared types. Every row declares its files and direct seals dependency. Each
phase owns its own notes file under the feature's `notes/` directory. Tests use
unique top-level names from the known-red register. Final adjudication runs
all seals without exclusions before declaring an epic complete.

## Boundaries that changed

| Previous ambiguity | Revised ruling |
|---|---|
| Join by run/key but dedupe by key/start | An attempt is run ID + task key + normalized start instant. |
| Copy a supposedly correct timing reference | Event-producer semantics and recorded sequences are authoritative; overlapping accounting is invalid. |
| Thin-cell log-normal fallback | No fitted fallback or pooling; report insufficient evidence. |
| Independently sample time parts and rounds | Resample joint completed-attempt records; preserve their dependence. |
| Dependency chain called the cap-constrained critical path | Report dependency path and a separate execution chain including resource waits. |
| Missing data quietly becomes an empty successful corpus | Explicit source manifest, completeness state, and named refusal at prediction gates. |
| The dispatcher knows no key syntax | Its create parser, key validator, and status-transition argv are explicit compatibility surfaces. |
| CLI helpers shared directly with web | Put the provider factory in an importable internal package. |
| Constructor count must be zero outside Jira | One named provider-factory exception; no distributed construction or renamed-constructor evasion. |
| Create is inherently exactly-once | Preserve existing successful-key replay behavior; surface uncertain writes and gate any stronger guarantee on an explicit protocol. |
| All seals must be individually red | New-capability seals red; existing compatibility seals green and mutation-verified. |

The task count and waves intentionally change. The former YouTrack 8-task /
7-wave check describes the archived plan only. Budget, model floor, Jira argv
compatibility, Markdown-native YouTrack, and the wallet account constraint
remain in force.
