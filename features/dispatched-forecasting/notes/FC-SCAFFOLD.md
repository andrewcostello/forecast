# FC-SCAFFOLD — amend the observation and source contracts (F1–F4)

Scaffold row. Freezes types, error states, ownership and behavior for
FC-SEALS, FC-JOURNAL, FC-SOURCES and FC-1. Reuses the reviewed FC-1 baseline
(`d529265`, implementation head `2604bb5`); nothing was re-extracted.
Contract authority: `docs/plans/forecasting-contracts.md` F1–F4;
protocol: `docs/plans/execution.md`.

## Status

- Build, vet and the full race suite pass on the amended head (see Evidence).
- No acceptance tests were authored (scaffold rule); `contract_test.go` holds unit tests of scaffold-implemented helpers only (panel iteration 1).
- Baseline behavior is preserved under the file moves: every moved function
  is byte-identical apart from godoc "Superseded:" notes and the two named
  constant substitutions in the Deviations table.

## File layout and ownership

| File | Content after this row | Owner of bodies |
|---|---|---|
| `observation.go` | Baseline `Role/Outcome/Cell/Revision/Provenance/Observation` unchanged; NEW `AttemptID`, `Measured[T]`, `Phase`, `Interval` (+`Equal`), `WallBreakdown` (+`Equal`), `EvidenceSource`, `FieldEvidence`, `ObservationEvidence` (11 fields, `merge`); `Observation.AttemptID()`, `Observation.Wall`, `Observation.Evidence`, `Observation.Equal`. | FC-OBS-ADJ (disputed path) |
| `errors.go` | Baseline sentinels unchanged; NEW F1–F4 sentinels (table below). | scaffold (frozen) |
| `journal.go` | NEW frozen: producer constants, `JournalIdentity`, `EventRef` (+`Equal`), `EventPayload`, `Event`, `ParsedJournal`, `JournalDiagnostics`, `Attempt`, `AttemptSet`, `AmbiguousAttempt`, `AttemptConflict`, holes `ParseEvents`, `ReduceAttempts`. MOVED from `extract.go`: `runTask`, `JournalFacts`, `JournalRow` (+methods), `journalEvent`, `journalPayload`, `journalSources`, `scanJournal`, both `observe` methods. | FC-JOURNAL |
| `sources.go` | NEW frozen: bound constants, `ReadBounds`, `SourceKind`, `SourceSpec`, `SourceState`, `SourceCounts`, `SourceReport`, `Selection` (`Validate`, `HeldOut`, `UnmatchedHoldouts`), `SourceManifest`, `ReadingRef`, `SourceReadings`, `gitEnvironment`, hole `ReadSources`. MOVED from `extract.go`: `readJournals`, `readJournal`, `readLiveSnapshots`, `gitSources`, `blobRef`, `readGitSnapshots`, `gitCatFileBatch`, `gitLines`, `readTargetTasks`, `defaultMaxHistoryCommits`. | FC-SOURCES |
| `evidence.go` | NEW frozen: `Disposition` (+`Dispositions()`), `Examined`, `DispositionCount`, `EvidenceJoin`, `preferEvidence`, `eventRefLess`, `sortExamined`, hole `JoinEvidence`. MOVED from `build.go`: `joinReadings`, `terminalEvidence*` consts, `observationFrom`, `terminalStatus`, `isUnrecoverableObservationError`. | FC-1 |
| `extract.go` | Retains only YAML decoding: `yamlTask`, `yamlDocument`, `taskSnapshot` (+NEW `SourceID`, `Ref()`), `yamlSources`, `parseSnapshots` (baseline, verbatim), NEW `RowFields`, `Reading` envelope, `parseReadings`, `isTaskYAML`, `decodeTaskDocument`. | FC-1 (via `build.go` boundary); `parseReadings` called by FC-SOURCES |
| `referenceclass.go` | FC-1 join/coverage boundary. `merge` extended to fold `Evidence` (per field, `preferEvidence`) and `Wall` (content join, nil identity, differing → refusal); `mergeWall` added. Baseline fields unchanged. | FC-1 |
| `build.go` | Baseline `Build` unchanged except the amended-options guard; NEW `BuildOptions.Sources/Selection/Bounds`, `Artifact.SourceManifest`, `Coverage.Dispositions/UniqueRows/Attempts/LostAttempts`, `Eligibility`, hole `PredictionEligibility`. | FC-1 |
| `cmd/forecast/dispatched_reference_cmd.go` | Not touched (not owned by this row). | FC-1 |

Holes return `ErrNotImplemented` and name every parameter, so a body can
read each value without a signature change.

## Frozen types (summary)

- `AttemptID{RunID, Key, StartedAt}`; `NewAttemptID` normalises to UTC;
  `Valid`, `Equal`, `Less`, `String`. Supersedes `identity{Key, StartedAt}`.
- `Measured[T]{Value, Known}`; `Known(v)`, `Unknown[T]()`, `Get`, `Must`
  (wraps `ErrUnknownMeasurement`). Money and tokens are separate `Measured`
  values from wall time.
- `Phase`: `development`, `panel_review`, `verifier`, `unclassified`.
- `Interval{Phase, Start, End, Inferred, Evidence []EventRef}`; `Validate`
  wraps `ErrReversedInterval`.
- `WallBreakdown{StartedAt, Elapsed, Intervals, Complete}`; `Classified`,
  `Unclassified`, `Validate` (disjoint, contained, sum ≤ elapsed;
  wraps `ErrOverlappingIntervals`).
- `EvidenceSource`: `EvidenceNone < EvidenceYAML < EvidenceJournal`.
  `FieldEvidence{Source, Event, Reading}`; `ObservationEvidence{Model, Start,
  Terminal, Elapsed, Wall, Rounds, Cascades, Reviews, InputTokens,
  OutputTokens, Cost}`: one `FieldEvidence` per derived row value. For a
  summed field `Event` is the least summed event; the full lists live on
  `Attempt`. `ObservationEvidence.merge` joins per field with `preferEvidence`.
- `Observation.Equal`: row equality by content (`Wall` via
  `WallBreakdown.Equal`, nil equals only nil). `==` on rows is not the
  contract because `Wall` is a pointer.
- `ParsedJournal{Journal, Events, Diagnostics}`: what `ParseEvents` returns;
  `Journal.Producer` is the raw `dispatcher_version` (`"0.1.0"`), resolved
  even when the journal has no task events.
- `Attempt`: joint completed-attempt record (id, start ref, `Model
  Measured[string]`, cascades, outcome, terminal/cutoff instants, elapsed,
  `Wall`, `Corrections`, `Reviews`, `Verifications`, tokens/cost as
  `Measured` with `CostEvents`, `InputTokenEvents`, `OutputTokenEvents`, and
  one `Evidence ObservationEvidence`). There is no duplicate
  `ModelEvidence`/`Terminal` field; the invariants between `Evidence` and the
  values are stated on the type.
- `Reading{Ref, Row, Present RowFields, Snapshot, Err}`: the envelope for
  every discovered YAML row, parsed or not; `RowFields{Key, RunID,
  StartedAt}` records raw presence so a missing `started_at` and a malformed
  one stay distinguishable. `SourceReadings{Journals []ParsedJournal,
  Readings []Reading}`; `JoinEvidence` takes `[]Reading`.
- `AttemptSet{Journal, Attempts, Ambiguous, Conflicts, Diagnostics, LeadingEvents}`.
- `SourceSpec{ID, Kind, Repository, Roots, Ref}`; `SourceState`
  `COMPLETE|PARTIAL|EMPTY`; `SourceReport`; `Selection{Cutoff, HoldoutRunIDs,
  AllowEmpty}` with `Validate` (blank, padded or duplicate ID), `HeldOut`
  (trimmed comparison) and `UnmatchedHoldouts` (holdout naming no discovered
  run); `SourceManifest`; `ReadBounds` with defaults (commits 5000, line
  16 MiB, blob 16 MiB, total 512 MiB, processes 2). Commit/line/blob/total
  caps stop a read and mark PARTIAL; `MaxProcesses` is a serializer only.
- `Disposition` (12 values, listed by `Dispositions()`), `Examined`,
  `EvidenceJoin`.
- `Eligibility{Eligible, MinCompleted, Reasons}`.

## Error states

| Sentinel | Raised when | Section |
|---|---|---|
| `ErrNotImplemented` | A scaffold hole is called before its body lands. Never a finished-unit outcome. | protocol |
| `ErrAmbiguousAttempt` | Two `task_started` share one `AttemptID`. | F1 |
| `ErrEvidenceConflict` | Equal-authority readings of one attempt disagree on model or terminal. | F1 |
| `ErrUnknownMeasurement` | A `Measured` read via `Must` while unknown. | F2 |
| `ErrReversedInterval` | Interval or attempt ends before it starts. | F2 |
| `ErrOverlappingIntervals` | Classified intervals overlap, leave the attempt or exceed elapsed. | F2 |
| `ErrInvalidSourceSpec` | Blank ID/repository, undeclared kind, absolute or escaping root, negative bound. | F3 |
| `ErrSourceMissing` | Requested repository/root/runs dir missing or unreadable. | F3 |
| `ErrSourceEmpty` | Zero journals or zero readings without `AllowEmpty`. | F3 |
| `ErrSourceIncomplete` | PARTIAL/EMPTY manifest reaches a gate needing COMPLETE. | F3/F4 |
| `ErrShallowHistory` | Shallow, grafted or replaced history where complete history is demanded. | F3 |
| `ErrBoundExceeded` | Commit, byte or line cap stopped a read. Never raised by `MaxProcesses`. | F3 |
| `ErrSourceCancelled` | Context ended mid-read (also wraps the context error). | F3 |
| `ErrInvalidSelection` | Held-out run ID blank, whitespace-padded or duplicate (`Selection.Validate`); holdout naming no discovered run (`Selection.UnmatchedHoldouts`, called by `ReadSources`). | F3/F6 |
| `ErrInvalidTarget` | Target row lacks key/valid role/model or repeats a key. | F4 |
| `ErrEmptyTarget` | Gate asked to check a zero-row target. | F4 |
| `ErrNotEligible` | Prediction gate refused. | F4 |

Baseline sentinels (`ErrUnattributable`, `ErrStampConflict`,
`ErrJournalSource`, `ErrYAMLSource`, `ErrGitHistory`, …) are unchanged.
`ErrStampConflict` remains what `Table.Add`/`merge` raise until FC-1 rebases
the table on `AttemptID`; `ErrEvidenceConflict` is the amended name for the
same situation inside one attempt.

## Superseded baseline rules (named rulings)

| Ruling | Superseded rule | Replacement | Body |
|---|---|---|---|
| R1 cross-run dedupe | `identity{Key, StartedAt}` folded different runs into one row (panel Codex-1). | `AttemptID(run, key, start)`; `Observation.AttemptID()`. | FC-1 |
| R2 faithful-copy timing | `JournalFacts.observe` copies `journal_facts()`; dev and review windows overlap in production order (panel Claude-1). | `Attempt.Wall` from `ReduceAttempts`: disjoint validated intervals; `DevElapsed`/`ReviewElapsed` retained only for the baseline artifact. | FC-JOURNAL |
| R3 rounds = max(iteration_count, panel_iterate) | `observationFrom` takes the max of two different counters. | `Attempt.Corrections` (recorded `panel_iterate`) distinct from `Attempt.Reviews` (`panel_started`); YAML `iteration_count` is a reading, not a max operand. | FC-JOURNAL / FC-1 |
| R4 silent empty journals | `readJournals` glob with zero hits is success (panel Grok-1). | `ErrSourceEmpty` unless `Selection.AllowEmpty`, then `SourceEmpty`. | FC-SOURCES |
| R5 simplified history | `rev-list --all -- features` without `--full-history` (panel Claude-2). | Full reachable history over explicit roots incl. deleted/renamed. | FC-SOURCES |
| R6 personal default repo | CLI default `~/Project/claude-workflow` (panel Claude-8/Grok-2). | `SourceSpec` only; no default. | FC-1 (CLI) |
| R7 order-dependent evidence tie | `merge` keeps `a.TerminalEvidence` on ties (panel Claude-7). | `preferEvidence` total order; `merge` already folds `Evidence` with it and sets `TerminalEvidence` from the joined `Evidence.Terminal` when that is not none (scaffold-implemented). | FC-1 |
| R8 buffered blobs / inherited git env / shallow looks complete | `gitCatFileBatch` buffers all; parent env inherited; `Truncated` only from count (panel Claude-5, Grok-3/4/5). | `ReadBounds`, `gitEnvironment`, `SourceReport.Shallow`, PARTIAL state. | FC-SOURCES |
| R9 recovered-credit before match | `joinedRunTasks` set before `Match` (panel Codex-2). | `DispositionNoMatchingStart`; recovery only on `DispositionRecovered`. | FC-1 |

## Producer semantics frozen for fixtures

Recorded from `~/Project/dispatcher-runs/2026-09-04T21-11-28Z-wallet-v2-tasks/journal.jsonl`
(`run_started.payload.dispatcher_version = "0.1.0"`, constant
`ProducerDispatcherV0_1_0 = "0.1.0"`, the exact wire value; `Producer` is
never prefixed or normalised). Per task, seq order:

```
task_started{model: planned}
task_spawn_finished{spawn_kind: implementer, model, cost_usd|null, duration_ms|null}
verification_started{iteration:0} … task_spawn_finished{spawn_kind: verifier} verification_verdict{duration_seconds}
panel_started{iteration:0} … panel_verdict
task_spawn_finished{spawn_kind: panel-iterate, model}   <- corrective spawn returns
panel_iterate{iteration:1}                             <- emitted AFTER that spawn
panel_started{iteration:1} … panel_verdict
task_done | task_blocked
agent_fallback{from_agent, to_agent, reason}            (anywhere; counts a cascade)
```

`journal_facts()` (model-matrix) is reference material only.

## Behavior/example table (every F1–F4 refusal and reconciliation)

Names are stable identifiers for seal subtests. "Carrier" is the type,
field or sentinel that expresses the outcome.

| Name | Input | Expected | Carrier | Body |
|---|---|---|---|---|
| F1-ID-UTC-OFFSET | `task_started` at `08:00-07:00`; YAML `started_at: 15:00Z` | One attempt, one reading. | `AttemptID.Equal` | FC-1 |
| F1-ID-DISTINCT-RUNS | Runs A and B, same key and instant | Two attempts, two observations, `Attempts=2`, `UniqueRows=2`. | `AttemptID`, `EvidenceJoin` | FC-1 |
| F1-ID-SAME-RUN-REVISIONS | Three YAML commits of one (run,key,start) | One attempt, one observation, `DispositionRecovered`×1 + `DispositionDuplicateReading`×2. | `EvidenceJoin.Dispositions` | FC-1 |
| F1-ID-AMBIGUOUS-START | Two `task_started` for one key at one instant in one run | Neither chosen; `AttemptSet.Ambiguous=[{id, Starts:2}]`; readings → `DispositionAmbiguousStart`. | `ErrAmbiguousAttempt` | FC-JOURNAL / FC-1 |
| F1-ID-NEAREST-NOT-MATCHED | YAML start 1 s from the only `task_started` | `DispositionNoMatchingStart`; attempt in `LostAttempts`. | `EvidenceJoin` | FC-1 |
| F1-EV-PROVENANCE-KEPT | Any recovered reading | Every field of `ObservationEvidence` (model, start, terminal, elapsed, wall, rounds, cascades, reviews, input tokens, output tokens, cost) carries a `FieldEvidence` naming its `ReadingRef` or `EventRef`; a summed field cites its least event and `Attempt.CostEvents`/`InputTokenEvents`/`OutputTokenEvents` list the rest; an unknown value has `EvidenceNone`. | `ObservationEvidence` | FC-JOURNAL / FC-1 |
| F1-EV-TOKENS-CITED-SEPARATELY | Spawn with `output_tokens` but no `input_tokens` | Event in `OutputTokenEvents` only; `InputTokens` unaffected; `Evidence.InputTokens` and `Evidence.OutputTokens` differ. | `Attempt.InputTokenEvents` | FC-JOURNAL |
| F1-EV-MERGE-PERMUTATION | Two readings of one row with different `Evidence` (yaml terminal vs journal terminal) and/or `Wall` | `merge(a,b).Equal(merge(b,a))`; each `Evidence` field is `preferEvidence` of the pair; `TerminalEvidence` equals the joined `Evidence.Terminal.Source.String()`; `Wall`: nil is identity, equal content kept, two different breakdowns → `ErrStampConflict` and `ErrEvidenceConflict` (never "whichever arrived first"). | `merge`, `mergeWall` | scaffold (implemented) / FC-1 |
| F1-ROW-EQUALITY-BY-CONTENT | Two rows with structurally equal `Wall` at distinct pointers | `Observation.Equal` true; `==` is not the contract. Seals compare rows with `Equal`. | `Observation.Equal` | scaffold (implemented) |
| F1-JOURNAL-PRODUCER-RESOLVED | Journal with `run_started` and no task events | `ParsedJournal.Journal.Producer == "0.1.0"`, `Events` empty, `MissingProducer=false`; a journal without `run_started` → `Producer==""`, `MissingProducer=true`. | `ParsedJournal` | FC-JOURNAL |
| F1-EV-NO-MANUFACTURED-ROW | Reading X: elapsed 10 m cost 1; reading Y (same attempt, different revision): elapsed 12 m cost unknown | Terminal/elapsed from journal; cost `Known(1)` cited to its spawn events; no field takes an independent max attributed to X or Y. | `Attempt.CostEvents` | FC-1 |
| F1-EV-JOURNAL-OVER-YAML | Journal `task_done` at T1; YAML `status: Done completed_at: T2` | Outcome done, elapsed to T1, `Terminal.Source=EvidenceJournal`. | `preferEvidence` | FC-1 |
| F1-EV-YAML-ONLY-TERMINAL | No journal terminal; YAML `Done`+`completed_at` | Outcome done, `Terminal.Source=EvidenceYAML`, counted in `RowsWithYAMLOnlyTerminalEvidence`. | `FieldEvidence` | FC-1 |
| F1-EV-UNKNOWN-STAYS-UNKNOWN | No terminal anywhere | `OutcomeUnfinished`, `Terminal.Source=EvidenceNone`, elapsed to cutoff, censored. | `Attempt.Censored` | FC-JOURNAL |
| F1-EV-MODEL-CONFLICT | Two implementing spawns in one attempt with different models and no cascade ordering | Last recorded implementing stamp wins (closing model); a cascade is `Cascades≥1`. Equal-authority contradiction that cannot be ordered → `AttemptConflict{Field:"model"}`. | `ErrEvidenceConflict` | FC-JOURNAL |
| F1-EV-TERMINAL-CONFLICT | `task_done` and `task_blocked` in one attempt | `AttemptConflict{Field:"terminal"}`; excluded, `DispositionConflictingEvidence`. | `ErrEvidenceConflict` | FC-JOURNAL / FC-1 |
| F1-EV-PERMUTATION | Any readings/events reordered (stable seq) | Identical `EvidenceJoin`. | `preferEvidence`, `eventRefLess` | FC-1 |
| F1-MODEL-CLOSING-STAMP | implementer `opus` → fallback → panel-iterate `sol` | `Model=Known("sol")`, `Cascades=1`, disclosed; not pooled with `opus`. | `Attempt.Model` | FC-JOURNAL |
| F1-MODEL-NO-ALIAS-POOL | `claude-opus-5` and `opus-5` | Two cells. | `Cell` | FC-1 |
| F1-MODEL-ABSENT-STAMP | No implementing spawn carries a model; YAML `model: opus` | `Model=Unknown`, `DispositionAbsentStamp`; authored model never substituted. | `ErrUnattributable` | FC-JOURNAL / FC-1 |
| F2-ELAPSED-TERMINAL | start T0, `task_done` T1 | `Elapsed=T1−T0`, not censored. | `Attempt` | FC-JOURNAL |
| F2-ELAPSED-CUTOFF | start T0, no terminal, cutoff C | `Elapsed=C−T0`, censored; never in a duration mean. | `Attempt.Censored` | FC-JOURNAL |
| F2-ELAPSED-BLOCKED-CENSORED | `task_blocked` | Censored lower bound; excluded from completed samples; counted in `NBlocked`. | `Observation.Duration` | FC-JOURNAL / FC-1 |
| F2-ELAPSED-SURVIVES-UNKNOWN-PHASES | Terminal present, no panel events | `Elapsed` known, `Wall.Complete=false`, `Wall.Intervals` empty, `Unclassified()=Elapsed`. | `WallBreakdown` | FC-JOURNAL |
| F2-PHASES-DISJOINT | Production order above | Intervals disjoint, contained, `Classified` sum ≤ `Elapsed`; residual `Unclassified`, never development. | `WallBreakdown.Validate` | FC-JOURNAL |
| F2-PHASES-PANEL-WALL | Three reviewer seats in one panel | One `panel_review` interval `panel_started→panel_verdict`. | `Interval` | FC-JOURNAL |
| F2-PHASES-ITERATE-AFTER-SPAWN | `spawn_finished(panel-iterate)` then `panel_iterate` | Corrective work interval ends at the spawn finish; `panel_iterate` is not a start boundary. | `Interval.Evidence` | FC-JOURNAL |
| F2-PHASES-INFERRED-LABELED | Boundary not recorded but derivable | `Inferred=true`; ambiguous attribution → no interval, `Complete=false`. | `Interval.Inferred` | FC-JOURNAL |
| F2-ROUNDS-VS-REVIEWS | first review, then two corrections | `Reviews=3`, `Corrections=2`. | `Attempt` | FC-JOURNAL |
| F2-COST-NULL-VS-ZERO | spawn `cost_usd: null` vs `cost_usd: 0` | `Unknown` vs `Known(0)`. | `Measured[float64]` | FC-JOURNAL |
| F2-COST-NO-DOUBLE-SUM | Same spawn seen twice (duplicate line) | Summed once; `CostEvents` lists one ref. | `Attempt.CostEvents` | FC-JOURNAL |
| F2-MEASURE-NONFINITE | `cost_usd: NaN`/`Inf`/negative | `ErrNegativeValue`; row not stored. | `Observation.Validate` | baseline |
| F2-MEASURE-REVERSED | terminal before start | `ErrReversedInterval`; `DispositionUnrecoverable`. | `Interval.Validate` | FC-JOURNAL |
| F3-SRC-EXPLICIT-ONLY | No `Sources`, no repos | `ErrInvalidSourceSpec`/`ErrYAMLSource`; never a home-directory default. | `SourceSpec.Validate` | FC-SOURCES / FC-1 |
| F3-SRC-ROOT-OUTSIDE-FEATURES | root `dispatcher/` | Scanned; sibling roots untouched. | `SourceSpec.Roots` | FC-SOURCES |
| F3-SRC-ROOT-ESCAPES | root `../x` or `/abs` | `ErrInvalidSourceSpec`. | `SourceSpec.Validate` | scaffold (implemented) |
| F3-SRC-MISSING | Repository path absent | `ErrSourceMissing`. | `ReadSources` | FC-SOURCES |
| F3-SRC-ZERO-JOURNALS | Runs dir exists, no `journal.jsonl` | `ErrSourceEmpty`; with `AllowEmpty`: `SourceEmpty`, never eligible. | `Selection.AllowEmpty` | FC-SOURCES |
| F3-SRC-READ-OK-ZERO-MATCH | Sources read, no reading matches an attempt | `SourceComplete` with `Records>0`; join reports dispositions; distinct from F3-SRC-ZERO-JOURNALS. | `SourceReport` | FC-1 |
| F3-SRC-MALFORMED-PARTIAL | One unreadable YAML among valid ones | Valid retained, `Counts.Malformed=1`, `SourcePartial`; the malformed row is present in `SourceReadings.Readings` with `Err` set. | `SourceCounts`, `Reading.Err` | FC-SOURCES |
| F3-READING-ENVELOPE | Document with a valid row, a row lacking `started_at`, and a row with `started_at: not-a-time` | Three `Reading`s: `{Present complete, Err nil}`, `{Present.StartedAt=false, Err nil}`, `{Present.StartedAt=true, Err set}`; the join gives them `DispositionRecovered`/…, `DispositionMissingJoinKeys`, `DispositionMalformed` respectively. An undecodable document is one `Reading{Row:0, Err set}` → `DispositionMalformed`. | `parseReadings`, `Reading` | scaffold (implemented) / FC-1 |
| F3-SRC-RESOLVED-REF | `Ref: main` | `ResolvedRef` = commit SHA. | `SourceReport` | FC-SOURCES |
| F3-GIT-ENV-STRIPPED | `GIT_DIR` pointing elsewhere | Selected repository read; override ignored. | `gitEnvironment` | FC-SOURCES |
| F3-GIT-SHALLOW | Shallow/grafted/replaced clone | `Shallow=true`, `SourcePartial` (or `ErrShallowHistory` when complete demanded); no auto-deepen. | `SourceReport.Shallow` | FC-SOURCES |
| F3-GIT-FULL-HISTORY | Side branch superseded at merge, ref deleted | Its blob enumerated and read. | `ReadSources` | FC-SOURCES |
| F3-GIT-DELETED-RENAMED | File deleted or renamed in history | Old content still read. | `ReadSources` | FC-SOURCES |
| F3-BOUND-COMMITS | `MaxCommits=3`, 5 commits | Enumeration stops at 3 before collection; `BoundsExceeded=1`, `SourcePartial`, `ErrBoundExceeded` when complete history is demanded. | `ReadBounds.MaxCommits` | FC-SOURCES |
| F3-BOUND-BYTES | Blob over `MaxBlobBytes`, line over `MaxLineBytes`, or total over `MaxTotalBytes` | Not read; `BoundsExceeded` counted; `SourcePartial`. | `ErrBoundExceeded` | FC-SOURCES |
| F3-BOUND-PROCESSES | `MaxProcesses=1`, two git reads of one source concurrently | Serializer only: at most one git child per source in flight; the second read waits for a slot rather than spawning. The cap never stops a read, never increments `BoundsExceeded`, never wraps `ErrBoundExceeded`, never changes counts, order or `SourceState`; a busy host completes COMPLETE. `MaxProcesses<0` → `ErrInvalidSourceSpec`. | `ReadBounds.MaxProcesses` | FC-SOURCES |
| F3-CANCELLED | Context cancelled mid-read | `ErrSourceCancelled` (also `context.Canceled`); partial manifest with `Cancelled=true`, never COMPLETE. | `SourceReport.Cancelled` | FC-SOURCES |
| F3-HOLDOUT-EXCLUDED | `HoldoutRunIDs=[R]`, live and historical readings for R | Excluded at source boundary before both joins; `ExcludedByRun` counted; `DispositionHeldOut`. | `Selection.HeldOut` | FC-SOURCES / FC-1 |
| F3-CUTOFF-EXCLUDED | Attempt started after `Cutoff` | Excluded; `AfterCutoff` counted; `DispositionAfterCutoff`. | `Selection.Cutoff` | FC-SOURCES / FC-1 |
| F3-SELECTION-INVALID | Blank, whitespace-padded (`" R1\n"`) or duplicate holdout ID | `ErrInvalidSelection`; the ID is rejected, not silently trimmed. | `Selection.Validate` | scaffold (implemented) |
| F3-HOLDOUT-PADDED-STILL-EXCLUDES | `HoldoutRunIDs=[" R1\n"]` used without `Validate` | `HeldOut("R1")` is true: comparison is on trimmed forms, so a padded holdout can never leak its run into the corpus. | `Selection.HeldOut` | scaffold (implemented) |
| F3-HOLDOUT-UNMATCHED | `HoldoutRunIDs=[R-misspelt]`, journals for R1 and R2 only | `ReadSources` calls `Selection.UnmatchedHoldouts` with every discovered run ID (before exclusion) and returns `ErrInvalidSelection`; nothing is extracted. Per-source `ExcludedByRun` is therefore ≥1 for some source whenever a holdout is named. | `Selection.UnmatchedHoldouts`, `SourceCounts.ExcludedByRun` | scaffold (helper implemented) / FC-SOURCES |
| F3-DISPOSITION-EVERY-SNAPSHOT | Any corpus | `len(Examined)` = snapshots examined; `Dispositions` sums to it. | `EvidenceJoin` | FC-1 |
| F3-DISPOSITION-NO-RUN | YAML run ID absent from journals | `DispositionNoMatchingRun` (not silently dropped). | `Disposition` | FC-1 |
| F3-DISPOSITION-MISSING-JOIN-KEYS | YAML snapshot lacking any of key, run ID or `started_at` | `AttemptID.Valid()=false`; snapshot listed in `Examined` with `DispositionMissingJoinKeys`; never matched by nearest start or run alone. | `AttemptID.Valid` | FC-1 |
| F3-ROWS-VS-ATTEMPTS | One run restarts a task | `UniqueRows=1`, `Attempts=2`, both listed. | `EvidenceJoin` | FC-1 |
| F3-LOST-NOT-HIDDEN | Two attempts, one recovered | `LostAttempts=[other]`; recovered sibling does not count for it. | `EvidenceJoin.LostAttempts` | FC-1 |
| F3-MANIFEST-CUTOFF-STORED | Any build | `SourceManifest.Cutoff` set; per-source counts present. | `SourceManifest` | FC-1 |
| F4-CELL-EMPTY-N0 | Required cell with no rows | Present, `n=0`, reported; not covered. | `RequiredCell.Empty` | baseline |
| F4-TARGET-MALFORMED | Row without model or with blank role | `ErrInvalidTarget` (and `ErrYAMLSource`). | `readTargetTasks` | scaffold (implemented) |
| F4-TARGET-ZERO-ROWS | `tasks: []` under any gate | `ErrEmptyTarget`; gate fails closed. | `PredictionEligibility` | FC-1 |
| F4-ELIGIBLE-THRESHOLD | Every required cell `NDone≥MinCompleted`, manifest COMPLETE | `Eligible=true`, `MinCompleted` reported as threshold. | `Eligibility` | FC-1 |
| F4-NOT-ELIGIBLE-THIN | A required cell `NDone<MinCompleted` | `Eligible=false`, reason names the cell; `ErrNotEligible` when refusing. | `ErrNotEligible` | FC-1 |
| F4-NOT-ELIGIBLE-PARTIAL | Manifest PARTIAL or EMPTY | `Eligible=false` (diagnostic artifact allowed); `ErrSourceIncomplete`. | `ErrSourceIncomplete` | FC-1 |
| F4-HAND-FINISHED-LIMIT | Any build | `Artifact.Limits` contains `HandFinishedLimit` and the report prints it. | `HandFinishedLimit` | baseline / FC-1 |
| F4-BUILD-AMENDED-OPTIONS | `BuildOptions.Sources`/`Selection`/`Bounds` set before FC-1 | `ErrNotImplemented`, never a silently-ignored holdout. | `BuildOptions.amended` | scaffold (implemented) |

## Deviations from the contract text

| Item | Deviation | Reason |
|---|---|---|
| `readTargetTasks` error | Now wraps `ErrInvalidTarget` in addition to `ErrYAMLSource` (message gains one segment). | Gives F4 its named error without breaking baseline `errors.Is(ErrYAMLSource)`. Existing tests pass unchanged. |
| `Build` | Returns `ErrNotImplemented` when the amended options are set. | Preserves the legacy call shape exactly; refuses to ignore a holdout. |
| `Observation` | Gains `Wall *WallBreakdown` and `Evidence ObservationEvidence`; `Validate` does not inspect them; `merge` folds both. | Freezes the amended row without changing baseline validation. `ObservationEvidence` is kept comparable so `Observation` stays a `==`-able key type; row equality for seals is `Observation.Equal` (content), and the permutation test in `dispatched_test.go` now uses it. |
| `merge` | Now joins `Evidence` and `Wall`; sets `TerminalEvidence` from joined `Evidence.Terminal` when not none; refuses two different `Wall`s. | No artifact change today: the baseline path never sets either field. Closes the reopened R7 on the amended fields. |
| Unit tests | `contract_test.go` added (Selection, merge over amended fields, `parseReadings`, producer constant, hole signatures). | Panel iteration required tests that fail under the named defects; these are unit tests of scaffold-implemented code, not acceptance seals over the behavior table, which remain FC-SEALS'. |
| Timing rule | Baseline `JournalFacts.observe` retained verbatim (marked superseded). | Preserve baseline artifact until FC-JOURNAL; deleting it would change `development_seconds`. |
| Scheduler | `scheduler.go` untouched. | F5 belongs to FC-SCHED-SCAFFOLD. |
| Otherwise | None. | |

## Evidence

Commands run on this head (2026-09-04, worktree `feat/FC-SCAFFOLD-scaffold-amend-the-observation-and`):

```
go build ./...
go vet ./...
env -u DISPATCHER_KNOWN_RED_FILE go test ./... -race     # all packages ok
gofmt -l internal/dispatched                              # empty
```

Producer ordering recorded from a real journal as listed above. No fixture
files were added (seals own `testdata/`).

## Residual limitations

- Every hole (`ParseEvents`, `ReduceAttempts`, `ReadSources`,
  `JoinEvidence`, `PredictionEligibility`) returns `ErrNotImplemented`; the
  artifact produced today is the FC-1 baseline artifact with the panel's
  defects still present, by design, until the bodies land.
- `gitEnvironment`, `sortExamined`, `parseReadings` and
  `Selection.UnmatchedHoldouts` are implemented but not yet called by the
  baseline path; `preferEvidence` and `eventRefLess` are reached through
  `merge` but the baseline never populates `Evidence`.
- The CLI default repository (R6) is outside this row's ownership and is
  still present until FC-1.

## Panel iteration 1 (cross-family review, 7 HIGH)

| # | Finding | Change |
|---|---|---|
| 1 | Holdout gate failed open on padded IDs; unmatched holdout had no carrier. | `Selection.Validate` rejects an ID that differs from its trimmed form; `HeldOut` compares trimmed forms; new `Selection.UnmatchedHoldouts` wraps `ErrInvalidSelection`; rows F3-SELECTION-INVALID (amended), F3-HOLDOUT-PADDED-STILL-EXCLUDES, F3-HOLDOUT-UNMATCHED. |
| 2 | `merge` ignored `Evidence`/`Wall`; pointer `Wall` degraded the `!=` seal. | `merge` folds `Evidence` per field via `preferEvidence` and `Wall` via `mergeWall` (nil identity, equal content kept, different → `ErrStampConflict`+`ErrEvidenceConflict`); godocs corrected; `Observation.Equal`, `WallBreakdown.Equal`, `Interval.Equal`, `EventRef.Equal`; rows F1-EV-MERGE-PERMUTATION, F1-ROW-EQUALITY-BY-CONTENT; permutation test compares with `Equal`. Deviation from the suggested "prefer Complete else nil": that rule is not associative (two differing incomplete breakdowns plus one complete one give different results by order), so the refusal join was chosen; it is order-independent. |
| 3 | Producer constant was `"dispatcher 0.1.0"`. | `ProducerDispatcherV0_1_0 = "0.1.0"`, documented as the raw wire value; no normalisation layer. |
| 4 | `ParseEvents` could not return the resolved producer. | Returns `ParsedJournal{Journal, Events, Diagnostics}`; `ReduceAttempts(parsed ParsedJournal, cutoff)`; `SourceReadings.Journals []ParsedJournal`; row F1-JOURNAL-PRODUCER-RESOLVED. |
| 5 | `SourceReadings` carried only parsed snapshots, so malformed/missing rows were indistinguishable. | `Reading{Ref, Row, Present RowFields, Snapshot, Err}` envelope and `parseReadings`; `SourceReadings.Readings []Reading`; `JoinEvidence(…, readings []Reading, …)`; `Examined.Row`; row F3-READING-ENVELOPE. `parseSnapshots` untouched for the baseline path. |
| 6 | Provenance only for model and terminal; `TokenEvents` conflated; duplicate evidence fields on `Attempt`. | `ObservationEvidence` has 11 fields (model, start, terminal, elapsed, wall, rounds, cascades, reviews, input tokens, output tokens, cost); `Attempt.InputTokenEvents`/`OutputTokenEvents` replace `TokenEvents`; `Attempt.ModelEvidence` and `Attempt.Terminal` removed in favour of `Attempt.Evidence` with stated invariants; rows F1-EV-PROVENANCE-KEPT (amended), F1-EV-TOKENS-CITED-SEPARATELY. |
| 7 | Process cap pointed both ways. | `MaxProcesses` is a serializer only: struck from `ErrBoundExceeded`, the error table and the `ReadBounds` preamble; `SourceCounts.BoundsExceeded` documents the exclusion; row F3-BOUND-PROCESSES rewritten. |

Evidence after iteration 1: `go build ./...`, `go vet ./...`,
`gofmt -l internal/dispatched` (empty), `env -u DISPATCHER_KNOWN_RED_FILE go test ./... -race` pass.
