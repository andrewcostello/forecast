# FC-SCAFFOLD: corrected F1–F4 handoff

This is a contract scaffold, not a completed extraction implementation. The
latest dispatcher verdict remains Blocked until this corrected head is reviewed.
Prior notes, patches and summary are retained in the dated FC-SCAFFOLD-correction
run artifact. This document supersedes the previous "scaffold implemented"
rulings, including the earlier justification for writing tests.

## Scope and ownership

The legacy Observation, Table/merge, target parser, CLI and all existing tests
retain baseline behavior at b4d7ab4. Journal/filesystem/evidence source moves
remain. No new test or fixture is included in the scaffold's cumulative diff.

| Owner | Frozen seam to implement after independent FC-SEALS |
|---|---|
| FC-JOURNAL | ParseEvents, ReduceAttempts, SummarizeWall, Attempt JSON/Censored methods; canonical attempt/time/cost evidence. |
| FC-SOURCES | Source/bound/selection validation, parseReadings, ReadSources, newSourceBudget/runSourceGit/openSourceFile/sourceGitCommand, ValidateReadingRevision, manifest validation. Depends on FC-JOURNAL because ReadSources consumes its parser. |
| FC-1 | JoinEvidence, amended Build/PredictionEligibility, version 4 ArtifactEvidence and CLI. |

Decision functions above return ErrNotImplemented in this scaffold. Validation
method descriptions state the required body behavior; they are not claims of
implemented validation. Simple enum membership and data constructors remain;
they do not implement the extraction/join policy. No code in a dependent body
needs to edit observation.go or extract.go to fill these seams.

## Contract rulings

- RecoveredAttempt contains the joint Attempt, its Cell and contributing YAML
  readings. The legacy max-fold table is never used for amended reconciliation.
  Select each value and its full citation together. Cell role has Evidence.Role; conflicting valid YAML roles are an evidence conflict.
  Terminal outcome, terminal instant and elapsed are one unit; journal outranks YAML, and equal-authority
  incompatible values are conflicts. Evidence tie order covers every member.
- Normalize instants to UTC without monotonic state. Reducers emit canonical
  interval and event-citation lists. Raw slice/pointer/time.Time equality is
  not the semantic identity rule. No newly implemented equality helper is frozen.
- Classified intervals exclude the unclassified phase. Residual alone represents
  unclassified time. Reject absent start, reversed/outside/overlapping spans,
  unknown phases, invalid order and overflow through named errors. Preserve
  known elapsed when a breakdown is unavailable.
- Selection.Cutoff is the sole nonzero extraction instant. Build resolves it once
  from explicit cutoff, else opts.Now, else one clock read. Standalone ReadSources,
  ReduceAttempts and JoinEvidence reject zero; none reads a clock. Every Attempt
  cutoff must equal Selection.Cutoff. Revision time and YAML start/terminal must
  be at/before cutoff before predictive values contribute; later envelopes are
  excluded audit AfterCutoff. This prevents a stripped journal terminal reappearing
  through later YAML. Git committer time/live mtime are recorded evidence, not
  tamper-proof clocks. Source bounds in the manifest are resolved positive values.
- ReadingRef contains its row and recorded revision instant. There is one portable
  citation, including source/path/revision/row/time. For a reconciled attempt the
  least canonical envelope is recovered and every additional compatible envelope
  is a duplicate reading; exact duplicate envelopes remain counted. Distinct row
  positions remain distinct citations. No conflicting attempt is recovered.
- EventPayload is a scalar-wire carrier with nullable pointers. Measured is the
  normalized artifact carrier. ParseEvents rejects malformed/negative/non-finite
  payloads and overflowing duration conversion with a LinesUnparsed diagnostic;
  it does not unmarshal scalar JSON directly into Measured.
- ReadSources retains excluded audit excluded YAML envelopes and separately
  records excluded journal identities. JoinEvidence audits every envelope and
  excludes it from contribution. Source marks are rechecked against validated
  Selection; the reducer ignores all post-cutoff journal evidence.
- A non-task YAML document is neither a malformed task nor a lost attempt.
  Decode task rows independently. Invalid siblings cannot erase valid rows.
- ReadSources validates before IO, applies zero-bound defaults, and reports
  shallow/capped/malformed reads as PARTIAL with reasons. Cancellation, invalid/duplicate source identity, or
  actual discovery/read failure returns an error with retained diagnostics. Completeness is refused
  by ValidateComplete/PredictionEligibility, with no hidden read policy flag.
- Version 4 requires SourceManifest plus ArtifactEvidence; the baseline emits version 3. Gate version comparison is equality, never >=. The Evidence payload
  holds full RecoveredAttempt records and audit data with stable JSON names;
  durations are nanoseconds. Its zero counters serialize explicitly. A missing
  payload is unavailable, never interpreted as measured zero. Legacy flat
  observations/cells are compatibility projections, not amended sampling input.
- PredictionEligibility accepts original []TargetRow records before aggregation.
  It validates them first (empty, then row errors in declaration order), computes
  counts from Evidence.Observations, and never trusts legacy coverage counters.
- PredictionEligibility defaults nonpositive thresholds to DefaultMinObservations.
  Schema/manifest failure with refuse=true wraps ErrNotEligible AND
  ErrSourceIncomplete; thin cells wrap ErrNotEligible. refuse=false reports
  insufficiency diagnostically. Empty/malformed targets are always named errors.

The exact producer version string is `0.1.0`. Source verification for the rules
below is dispatcher orchestrator.py at `df771516b905355995d03313b470b06e1aea4e06`.
A version string alone cannot identify a producer code revision. Fixtures must cite an actual
producer revision; `panel_iterate` follows the corrective spawn finish. This
ordering is evidence for the reducer, not a license to copy a defective oracle.

## Behavioral examples for independent seals

These are expected completed-body outcomes. Each new capability is red against
the named ErrNotImplemented seam. FC-SEALS writes the fixtures and assertions
independently. Existing regression tests remain unchanged unless that seals
row explicitly amends a legacy expectation under the revised contracts.

| Name | Input | Expected | Carrier | Body |
|---|---|---|---|---|
| F1-ID-UTC-OFFSET | task/YAML starts with different offsets for the same instant | One run/key/UTC instant; output timestamps normalized without monotonic data. | AttemptID | FC-JOURNAL / FC-1 |
| F1-ID-DISTINCT-RUNS | Runs A and B, same key and instant | Two attempts, two observations, `Attempts=2`, `UniqueRows=2`. | `AttemptID`, `EvidenceJoin` | FC-1 |
| F1-ID-SAME-RUN-REVISIONS | Three compatible YAML commits of one (run,key,start), all at/before cutoff | Least canonical row citation is recovered; others are duplicate readings regardless of revision. One attempt, one observation, `DispositionRecovered`×1 + `DispositionDuplicateReading`×2. | `EvidenceJoin.Dispositions` | FC-1 |
| F1-ID-AMBIGUOUS-START | Two distinct `task_started` sequences for one key at one instant in one run | Neither chosen; `AttemptSet.Ambiguous=[{id, Starts:2}]`; readings → `DispositionAmbiguousStart`. | `ErrAmbiguousAttempt` | FC-JOURNAL / FC-1 |
| F1-ID-NEAREST-NOT-MATCHED | YAML start 1 s from the only `task_started` | `DispositionNoMatchingStart`; attempt in `LostAttempts`. | `EvidenceJoin` | FC-1 |
| F1-EV-PROVENANCE-KEPT | Any recovered reading | Every field of `ObservationEvidence` (role, model, start, terminal, elapsed, wall, corrections, cascades, reviews, verifications, input tokens, output tokens, cost) carries a `FieldEvidence` naming its `ReadingRef` or `EventRef`; a summed field cites its least event and `Attempt.CostEvents`/`InputTokenEvents`/`OutputTokenEvents` list every contributing ref including that least ref; an unknown value has `EvidenceNone`. | `ObservationEvidence` | FC-JOURNAL / FC-1 |
| F1-EV-TOKENS-CITED-SEPARATELY | Spawn with `output_tokens` but no `input_tokens` | Event in `OutputTokenEvents` only; `InputTokens` unaffected; `Evidence.InputTokens` and `Evidence.OutputTokens` differ. | `Attempt.InputTokenEvents` | FC-JOURNAL |
| F1-EV-MERGE-PERMUTATION | YAML done at 12m, journal done at 10m; permutations | Journal outcome/time/elapsed/citations selected together (10m); equal-authority incompatible values conflict. All other values travel with their citations. | JoinEvidence, RecoveredAttempt | FC-1 |
| F1-ROW-EQUALITY-BY-CONTENT | Equivalent event instants and unordered input citations/intervals | Join output has canonical timestamps/list order; structural contents and serialized values agree under permutations. Legacy Observation equality is unchanged. | RecoveredAttempt | FC-JOURNAL / FC-1 |
| F1-JOURNAL-PRODUCER-RESOLVED | Journal with `run_started` and no task events | `ParsedJournal.Journal.Producer == "0.1.0"`, `Events` empty, `MissingProducer=false`; a journal without `run_started` → `Producer==""`, `MissingProducer=true`. | `ParsedJournal` | FC-JOURNAL |
| F1-EV-NO-MANUFACTURED-ROW | Journal terminal 10 m and implementing spawn cost 1; reading X: elapsed 10 m cost 1; reading Y (same attempt, different revision): elapsed 12 m cost unknown | Terminal/elapsed from journal; cost `Known(1)` cited to its spawn events; no field takes an independent max attributed to X or Y. | `Attempt.CostEvents` | FC-1 |
| F1-EV-JOURNAL-OVER-YAML | Journal terminal T1, differing YAML terminal T2/outcome | Choose journal terminal tuple; only equal-authority contradictions are conflicts. | Attempt.Evidence | FC-1 |
| F1-EV-YAML-ONLY-TERMINAL | No journal terminal; YAML `Done`+`completed_at`, both revision and terminal at/before cutoff | Outcome done, `Terminal.Source=EvidenceYAML`, counted in `EvidenceJoin.RowsWithYAMLOnlyTerminal`. | FieldEvidence; EvidenceJoin.RowsWithYAMLOnlyTerminal | FC-1 |
| F1-EV-UNKNOWN-STAYS-UNKNOWN | No terminal anywhere | `OutcomeUnfinished`, `Terminal.Source=EvidenceNone`, elapsed to cutoff, censored. | `Attempt.Censored` | FC-JOURNAL |
| F1-EV-MODEL-CONFLICT | Two ordered implementing spawns in one attempt with different models and no cascade marker | Last physical implementing event with a recorded model wins after retransmission/collision removal; Cascades counts only recorded agent_fallback events. No extra unordered-model case: conflicting same-sequence payloads are parser-malformed by F2-EVENT-IDENTITY. | Attempt.Model | FC-JOURNAL |
| F1-EV-TERMINAL-CONFLICT | `task_done` and `task_blocked` in one attempt | `AttemptConflict{Field:"terminal"}`; excluded, `DispositionConflictingEvidence`. | `ErrEvidenceConflict` | FC-JOURNAL / FC-1 |
| F1-EV-PERMUTATION | Inputs/corroborating citations reordered | Same canonical recovered samples/audit; tie order includes every journal and reading member, including Producer and UTC instant. | JoinEvidence | FC-1 |
| F1-MODEL-CLOSING-STAMP | implementer `opus` → fallback → panel-iterate `sol` | `Model=Known("sol")`, `Cascades=1`, disclosed; not pooled with `opus`. | `Attempt.Model` | FC-JOURNAL |
| F1-MODEL-NO-ALIAS-POOL | `claude-opus-5` and `opus-5` | Two cells. | `Cell` | FC-1 |
| F1-MODEL-ABSENT-STAMP | No implementing spawn carries a model; YAML `model: opus` | `Model=Unknown`, `DispositionAbsentStamp`; authored model never substituted. | `ErrUnattributable` | FC-JOURNAL / FC-1 |
| F2-ELAPSED-TERMINAL | start T0, `task_done` T1 | `Elapsed=T1−T0`, not censored. | `Attempt` | FC-JOURNAL |
| F2-ELAPSED-CUTOFF | start T0, no terminal, cutoff C | `Elapsed=C−T0`, censored; never in a duration mean. | `Attempt.Censored` | FC-JOURNAL |
| F2-ELAPSED-BLOCKED-CENSORED | `task_blocked` | Censored lower bound; excluded from completed samples; counted in `NBlocked`. | Attempt.Elapsed and Attempt.Censored | FC-JOURNAL / FC-1 |
| F2-ELAPSED-SURVIVES-UNKNOWN-PHASES | Terminal present, no panel events | `Elapsed` known, `Wall.Complete=false`, `Wall.Intervals` empty, `WallSummary.Unclassified=Elapsed, WallSummary.Complete=false`. | `WallBreakdown` | FC-JOURNAL |
| F2-PHASES-DISJOINT | Production order above | Intervals disjoint, contained, classified sum ≤ `Elapsed`; residual `Unclassified`, never development. | `SummarizeWall` | FC-JOURNAL |
| F2-PHASES-PANEL-WALL | Three reviewer seats in one panel | One panel_review interval from invocation-shaped panel_started to panel_verdict; path-classification gate records never open a review interval. | `Interval` | FC-JOURNAL |
| F2-PHASES-ITERATE-AFTER-SPAWN | `spawn_finished(panel-iterate)` then `panel_iterate` | Corrective work interval ends at the spawn finish; `panel_iterate` is not a start boundary. | `Interval.Evidence` | FC-JOURNAL |
| F2-PHASES-INFERRED-LABELED | Direct valid WallBreakdown interval marked Inferred=true | Validator preserves the flag; it does not infer or authenticate caller attribution. The 0.1.0 reducer emits only explicit-event or recorded-duration spans with Inferred=false; missing/ambiguous boundaries are withheld with Complete=false. | Interval.Inferred; SummarizeWall | FC-JOURNAL |
| F2-ROUNDS-VS-REVIEWS | first review invocation, then two panel corrections each followed by another invocation | `Reviews=3`, `Corrections=2`. | `Attempt` | FC-JOURNAL |
| F2-COST-NULL-VS-ZERO | spawn `cost_usd: null` vs `cost_usd: 0` | `Unknown` vs `Known(0)`. | `Measured[float64]` | FC-JOURNAL |
| F2-COST-NO-DOUBLE-SUM | Same spawn seen twice with the same present producer sequence and equivalent payload | Summed once; `CostEvents` lists one ref. | `Attempt.CostEvents` | FC-JOURNAL |
| F2-MEASURE-NONFINITE | Negative/non-finite quantity or out-of-range duration-ms | ParseEvents counts LinesUnparsed and skips invalid event; ReadSources marks PARTIAL. Direct invalid join inputs are unrecoverable, never sampled. | JournalDiagnostics, JoinEvidence | FC-JOURNAL / FC-1 |
| F2-MEASURE-REVERSED | terminal before start | `ErrReversedInterval`; `DispositionUnrecoverable`. | `SummarizeWall` | FC-JOURNAL |
| F3-SRC-EXPLICIT-ONLY | No `Sources`, no repos | `ErrInvalidSourceSpec`/`ErrYAMLSource`; never a home-directory default. | `SourceSpec.Validate` | FC-SOURCES / FC-1 |
| F3-SRC-ROOT-OUTSIDE-FEATURES | root `dispatcher/` | Scanned; sibling roots untouched. | `SourceSpec.Roots` | FC-SOURCES |
| F3-SRC-ROOT-ESCAPES | Relative ../x, absolute /x, symlink escaping declared root | ErrInvalidSourceSpec or ErrSourceMissing before reading outside selection. | SourceSpec.Validate, ReadSources | FC-SOURCES |
| F3-SRC-MISSING | Repository path absent | `ErrSourceMissing`. | `ReadSources` | FC-SOURCES |
| F3-SRC-ZERO-JOURNALS | Runs dir exists, no `journal.jsonl` | `ErrSourceEmpty`; with `AllowEmpty`: `SourceEmpty`, never eligible. | `Selection.AllowEmpty` | FC-SOURCES |
| F3-SRC-READ-OK-ZERO-MATCH | Sources read, no reading matches an attempt | `SourceComplete` with `Records>0`; join reports dispositions; distinct from F3-SRC-ZERO-JOURNALS. | `SourceReport` | FC-1 |
| F3-SRC-MALFORMED-PARTIAL | One unreadable YAML among valid ones | Valid retained, `Counts.Malformed=1`, `SourcePartial`; the malformed row is present in `SourceReadings.Readings` with `Err` set. | `SourceCounts`, `Reading.Err` | FC-SOURCES |
| F3-READING-ENVELOPE | Valid row, missing start, invalid timestamp, typed YAML field mismatch | Four independent envelopes; invalid rows retain their own Err and do not erase valid siblings. Non-task documents have DocumentNotTasks; malformed syntax has DocumentMalformed. | parseReadings, Reading | FC-SOURCES |
| F3-SRC-RESOLVED-REF | `Ref: main` | `ResolvedRef` = commit SHA. | `SourceReport` | FC-SOURCES |
| F3-GIT-ENV-STRIPPED | GIT_DIR or GIT_CONFIG_GLOBAL/COUNT/KEY_n/VALUE_n redirects location/config/helper | Clear the named override list in Entry-point contracts; preserve PATH/GIT_EXEC_PATH, ignore global/system config, pin repository, invoke no configured helper; bounded enumeration stays in selected repository. | ReadSources | FC-SOURCES |
| F3-GIT-SHALLOW | Shallow/grafted/replaced clone | Shallow=true, PARTIAL, named reason; nil read error solely for this condition; no fetching. Eligibility refuses. | SourceReport | FC-SOURCES / FC-1 |
| F3-GIT-FULL-HISTORY | Side branch superseded at merge, ref deleted | Its blob enumerated and read. | `ReadSources` | FC-SOURCES |
| F3-GIT-DELETED-RENAMED | File deleted or renamed in history | Old content still read. | `ReadSources` | FC-SOURCES |
| F3-BOUND-COMMITS | MaxCommits=3, five reachable commits | Limit enumeration before collecting; report bound/PARTIAL, nil read error solely for cap. Gate refuses. | ReadSources | FC-SOURCES |
| F3-BOUND-BYTES | Blob over `MaxBlobBytes`, line over `MaxLineBytes`, or total over `MaxTotalBytes` | Retain bounded data only, withhold overflow; `BoundsExceeded` counted; `SourcePartial`. | SourceCounts.BoundsExceeded; ValidateComplete wraps ErrBoundExceeded | FC-SOURCES |
| F3-BOUND-PROCESSES | `MaxProcesses=1`, two git reads of one source concurrently | Serializer only: at most one git child per source in flight; the second read waits for a slot rather than spawning. The cap never stops a read, never increments `BoundsExceeded`, never wraps `ErrBoundExceeded`, never changes counts, order or `SourceState`; a busy host completes COMPLETE. `MaxProcesses<0` → `ErrInvalidSourceSpec`. | `ReadBounds.MaxProcesses` | FC-SOURCES |
| F3-CANCELLED | Context cancelled mid-read | `ErrSourceCancelled` (also `context.Canceled`); partial manifest with `Cancelled=true`, never COMPLETE. | `SourceReport.Cancelled` | FC-SOURCES |
| F3-HOLDOUT-EXCLUDED | Held-out R in live/history/journals | Keep excluded audit YAML envelopes marked HeldOut and journal identities in ExcludedJournals; no held-out predictive payload/events enter reducer/join. Audit includes the excluded envelopes once. | SourceReadings, Reading.Excluded, EvidenceJoin | FC-SOURCES / FC-1 |
| F3-CUTOFF-EXCLUDED | YAML RecordedAt OR started_at OR completed_at after cutoff | Identity-only AfterCutoff unless HeldOut (which wins). Missing RecordedAt sets Err and Malformed/PARTIAL for source quality; join emits Malformed for an otherwise in-sample row. Journal post-cutoff events are separately ignored; a later start creates no attempt. No later outcome/model contribution; exclusion times are audit evidence only. | Reading.Excluded, ReduceAttempts | FC-JOURNAL / FC-SOURCES / FC-1 |
| F3-SELECTION-INVALID | Blank, padded or duplicate held-out ID | ErrInvalidSelection before IO/reconciliation, even if validation helper was not called by the caller. | ReadSources, JoinEvidence | FC-SOURCES / FC-1 |
| F3-HOLDOUT-PADDED-STILL-EXCLUDES | Padded ID passed straight to ReadSources or JoinEvidence | Reject ErrInvalidSelection; no observations. Validation is required at the entry point, no permissive matching helper. | ReadSources, JoinEvidence | FC-SOURCES / FC-1 |
| F3-HOLDOUT-UNMATCHED | Named holdout not among discovered journal run IDs | ErrInvalidSelection at ReadSources and JoinEvidence entry points. Join receives the full discovered journal universe, including ExcludedJournals, so a matched journal needs no task events/YAML. | Selection.UnmatchedHoldouts, JoinEvidence journal universe | FC-SOURCES / FC-1 |
| F3-DISPOSITION-EVERY-SNAPSHOT | Any corpus | `len(Examined)` = snapshots examined; `Dispositions` sums to it. | `EvidenceJoin` | FC-1 |
| F3-DISPOSITION-NO-RUN | YAML run ID absent from journals | `DispositionNoMatchingRun` (not silently dropped). | `Disposition` | FC-1 |
| F3-DISPOSITION-MISSING-JOIN-KEYS | YAML snapshot lacking any of key, run ID or `started_at` | identity incomplete; snapshot listed in `Examined` with `DispositionMissingJoinKeys`; never matched by nearest start or run alone. | `AttemptID` | FC-1 |
| F3-ROWS-VS-ATTEMPTS | One run restarts a task | `UniqueRows=1`, `Attempts=2`, both listed. | `EvidenceJoin` | FC-1 |
| F3-LOST-NOT-HIDDEN | Two attempts, one recovered | `LostAttempts=[other]`; recovered sibling does not count for it. | `EvidenceJoin.LostAttempts` | FC-1 |
| F3-MANIFEST-CUTOFF-STORED | Any build | `SourceManifest.Cutoff` set; per-source counts present. | `SourceManifest` | FC-1 |
| F4-CELL-EMPTY-N0 | Required cell with no rows | Present, `n=0`, reported; not covered. | `RequiredCell.Empty` | baseline |
| F4-TARGET-MALFORMED | Blank key/model, bad role, repeated key | Amended gate ErrInvalidTarget; legacy readTargetTasks remains byte-for-byte baseline. | PredictionEligibility | FC-1 |
| F4-TARGET-ZERO-ROWS | `tasks: []` under any gate | `ErrEmptyTarget`; gate fails closed. | `PredictionEligibility` | FC-1 |
| F4-ELIGIBLE-THRESHOLD | Every required cell `NDone≥MinCompleted`, manifest COMPLETE | `Eligible=true`, `MinCompleted` reported as threshold. | `Eligibility` | FC-1 |
| F4-NOT-ELIGIBLE-THIN | A required cell `NDone<MinCompleted` | `Eligible=false`, reason names the cell; `ErrNotEligible` when refusing. | `ErrNotEligible` | FC-1 |
| F4-NOT-ELIGIBLE-PARTIAL | Absent/invalid/partial/empty manifest or absent/wrong-version Evidence payload | Eligible=false; refuse=true wraps both ErrNotEligible and ErrSourceIncomplete, otherwise diagnostic result. | PredictionEligibility | FC-1 |
| F4-HAND-FINISHED-LIMIT | Any build | `Artifact.Limits` contains `HandFinishedLimit` and the report prints it. | `HandFinishedLimit` | baseline / FC-1 |
| F4-BUILD-AMENDED-OPTIONS | Non-nil Sources (including []), non-nil holdout list, cutoff/allow-empty or nonzero bounds before FC-1 | ErrNotImplemented; no silent legacy-source fallback. | BuildOptions.amended | scaffold refusal seam / FC-1 |
| F2-WALL-ABSENT-START | Wall start zero | ErrUnattributable; no invented containment or duration. | SummarizeWall | FC-JOURNAL |
| F2-UNCLASSIFIED-RESIDUAL-ONLY | Interval phase unclassified or undeclared | ErrInvalidPhase; missing attribution stays residual. | SummarizeWall | FC-JOURNAL |
| F2-CANONICAL-ORDER | Same spans in different raw orders | Reducer emits canonical order; wall validator rejects noncanonical input with ErrNonCanonicalEvidence. | ReduceAttempts, SummarizeWall | FC-JOURNAL |
| F2-PARTIAL-SUM-UNKNOWN | One spawn has cost/tokens, another lacks them | Total unknown with available citations retained; never present the observed partial sum as a complete measurement. | Attempt | FC-JOURNAL |
| F3-NON-TASK-DOCUMENT | Ordinary known-red/config YAML under selected root | NonTaskDocuments increments, DocumentNotTasks and matching disposition; no malformed count/PARTIAL solely from this file. | Reading.Kind, SourceCounts | FC-SOURCES / FC-1 |
| F3-COMPLETE-CONSISTENCY | Nil manifest or COMPLETE label with shallow/cancelled/bound/malformed flags | ErrSourceIncomplete; labels cannot override contradictory facts. | SourceManifest.ValidateComplete | FC-SOURCES |
| F3-DEFAULT-BOUNDS | Zero fields or negative fields | Zero uses frozen defaults; negative rejected before IO. MaxProcesses queues work, never truncates data. | ReadSources, ParseEvents (line bound) | FC-SOURCES / FC-JOURNAL |
| F4-THRESHOLD-NONPOSITIVE | minCompleted zero/negative | DefaultMinObservations applied; effective positive threshold returned. | PredictionEligibility | FC-1 |
| F4-SCHEMA-ROUNDTRIP | Full joint attempt with evidence, wall, review/verifier counts and cost/token event lists | Version 4 Evidence round-trip retains all fields; nil Evidence means unavailable, zero counts inside payload remain present. Legacy version 3 cannot license amended sampling. | ArtifactEvidence | FC-1 |

## Additional behavioral examples

| Name | Input | Expected | Owner |
|---|---|---|---|
| F2-SPAWN-WIRE | Actual producer payload with scalar cost/token/iteration and duration_ms 1250; repeat null/missing/zero | Pointers retain absence vs zero; duration converts to 1.25s. No scalar-to-Measured decode. Overflow/negative/type errors count LinesUnparsed and cannot reach samples. | FC-JOURNAL |
| F2-CORRECTIVE-BOUNDARY | panel-iterate spawn finishes at T with valid duration D | Development span [T-D,T), Inferred=false because producer records duration. If missing D, no interval; retain elapsed and Complete=false. Do not use preceding panel_verdict as an assumed start: it may include queue/setup time. Outside spans and every member of any overlapping candidate component are withheld, retaining elapsed with Complete=false; SummarizeWall rejects such spans supplied directly. | FC-JOURNAL |
| F1-CITATION-ROW | Two rows at one source/path/revision | Ref.Row differs. Full field evidence and serialized citations identify each; exact duplicate envelopes remain separately audited. Compatible readings of one AttemptID yield one recovered and remaining duplicates. | FC-SOURCES / FC-1 |
| F3-CUTOFF-REPLAY | Fixed cutoff C; rebuild with different host time; live/git revision or terminal later than C | Same elapsed/outcomes from the same frozen source snapshots/resolved refs and eligible evidence. Later envelopes AfterCutoff; no later YAML terminal restores a post-cutoff journal terminal. Zero cutoff at a direct seam or mismatched Attempt.Cutoff => ErrInvalidSelection. Build captures one instant before reading. | FC-JOURNAL / FC-SOURCES / FC-1 |
| F3-RESOLVED-BOUNDS | Requested zero bounds | Manifest stores positive effective defaults, including DefaultMaxCommits; it can be replayed without applying a future version's defaults. | FC-SOURCES |
| F3-CANCEL-PERSIST | Cancellation after some records | ParseEvents retains parsed events; ReadSources returns PARTIAL manifest/readings with wrapping context error; Build returns diagnostic result plus error; CLI writes/report manifest before failing. | FC-JOURNAL / FC-SOURCES / FC-1 |
| F4-TARGET-INPUT | Duplicate/blank keys in original TargetRow slice but apparently valid aggregate Coverage | Gate rejects ErrInvalidTarget before source/sample checks. It validates original rows, not reconstructed aggregates; no file IO in the gate. | FC-1 |
| F4-VERSION-EXACT | Legacy schema 3, unknown schema 5, or schema 4 missing Evidence/manifest | Refusal; only exact AmendedEvidenceSchemaVersion=4 with required valid payload can pass. | FC-1 |

## Validation

Validation: go build ./..., go vet ./..., and the full go test ./... -race
suite pass without exclusions after correction. The head-pinned cross-family
panel is required before releasing FC-SEALS. Baseline
code/test equivalence is checked independently from the historical panel results.

The legacy extraction defects remain visible until FC-JOURNAL/FC-SOURCES/FC-1
land, as the worklist requires. No budget, test exclusion, or reviewer rule is
relaxed by this correction.

Build/vet/full race validation passed on the follow-up; an exact-head panel remains required before release.

## Producer, identity and exclusion behavior

These are completed-body expectations; the scaffold adds no acceptance implementation or tests.

| Name | Input | Frozen outcome | Owner |
|---|---|---|---|
| F2-PRODUCER-SHAPES | panel_started with forced_by=path_classification, then panel_started with iteration and iterations_remaining, then one panel_verdict | One Reviews and one panel wall interval from the invocation-shaped start. Gate record opens no interval. Unknown or contradictory start shapes are LinesUnparsed/PARTIAL. Evidence.Reviews cites invocation; ReviewEvents retains every counted invocation. | FC-JOURNAL |
| F2-CORRECTION-KINDS | panel-iterate, verifier-iterate, test-fix-retry, commit-retry, push-retry and summary-recovery spawn finishes | All are implementing work for closing-model selection and development spans when duration is usable. Corrections counts panel_iterate + verification_iterate plus test-fix-retry/commit-retry/push-retry/summary-recovery finishes. Do not also count paired panel/verifier spawn finishes. CorrectionEvents retains every counted event. Unknown spawn kinds never supply a model/phase. | FC-JOURNAL |
| F2-VERIFICATIONS | verification_started/verdict, skipped, mechanical and iterate records | Verifications counts verification_started only, even if no verdict follows. Skipped/mechanical/iterate/verdict add no invocation. Zero has EvidenceNone; otherwise least counted event, also included in the complete VerificationEvents list. No completed phase span when end is missing; Complete=false. | FC-JOURNAL |
| F2-EVENT-IDENTITY | Copied event with HasSeq=true and equal Seq; copied sequence-less events on different lines | Equivalent sequence retransmission retained once using least line. Sequence-less lines remain distinct. HasSeq=false stores Seq=0; explicit zero is distinguishable. Same sequence with conflicting task/type/instant/payload: every colliding line counted LinesUnparsed and discarded, PARTIAL. Reducer validates/directly deduplicates its ParsedJournal inputs by the same rule rather than trusting a caller to have parsed them. Distinct-start ambiguity applies after retransmission removal. | FC-JOURNAL |
| F2-ALL-SPAWN-COST | Implementer $1.20 and verifier/corrective/retry spawns $0.90 | CostUSD Known(2.10) when every recorded spawn contributes. Any missing cost => Unknown with available citations; no spawns => Unknown. CostScope=recorded_task_spawns. Separate unjournaled reviewer/operator spend is outside this measurement and must be disclosed; never label it total process cost. InputTokens is uncached input_tokens only; cache tokens are excluded and labeled. | FC-JOURNAL / FC-1 |
| F2-ARITHMETIC-ERRORS | Noncanonical interval/citation order; overflowing duration/token/count/finite cost sum | SummarizeWall returns ErrNonCanonicalEvidence or ErrMeasurementOverflow as applicable. ReduceAttempts/JoinEvidence checked arithmetic returns ErrMeasurementOverflow with retained diagnostics, never saturation. Build marks any reduction/reconciliation error PARTIAL and refuses prediction. Invalid wire numbers are separately counted/skipped by ParseEvents as LinesUnparsed. | FC-JOURNAL / FC-1 |
| F2-LATER-START | task_started strictly after C | No Attempt, no LostAttempt and no negative elapsed; increment AttemptSet.StartsAfterCutoff, carried through EvidenceJoin to ArtifactEvidence. Start exactly at C is included. | FC-JOURNAL |
| F1-ROLE-CITATION | Compatible readings versus two different valid roles for one AttemptID | Evidence.Role cites selected YAML role; different valid roles are ErrEvidenceConflict, no recovered sample. | FC-1 |
| F1-CONFLICT-PORTABLE | Two incompatible models/roles/terminals or measurements | AttemptConflict serializes code=evidence_conflict and both tagged canonical JSON candidate values beside A/B citations. Terminal value is {outcome,terminal_at,elapsed_ns}. No source lookup is needed to see the disagreement. | FC-JOURNAL / FC-1 |
| F1-HASH-PROVENANCE | Event has hash/prev_hash | EventRef round-trip retains both. This scope preserves provenance, not a claim of cryptographic verification or detection of an omitted tail. Full citation tie order is WallBreakdown EventRef order: journal run/source/path/producer, HasSeq false-first, Seq, Line, Type, UTC instant, Hash, PrevHash. | FC-JOURNAL / FC-1 |
| F3-EXCLUSION-ORDER | Non-task document; heldout plus later revision/start/terminal; malformed in-sample row | Order: NotTaskDocument, HeldOut, AfterCutoff, Malformed, MissingJoinKeys, match outcome. HeldOut wins over cutoff and errors. Source quality reports excluded malformed/unreadable facts in separate diagnostic counters; only in-sample quality degrades completeness. AfterCutoff means RecordedAt OR started_at OR completed_at > C. Both source and join apply it. Join honors source exclusion markers while retaining independently decoded selection times; a recheck can add exclusion, never undo it. Inconsistent HeldOut marker => ErrInvalidSelection. | FC-SOURCES / FC-1 |
| F3-MISSING-REVISION-TIME | In-sample row Ref.RecordedAt zero | ReadSources sets Reading.Err, counts Malformed and PARTIAL; JoinEvidence maps it to DispositionMalformed, also for direct inputs missing Err. Never treat it as merely unrecoverable COMPLETE input. | FC-SOURCES / FC-1 |
| F3-COMPLETENESS-CAUSES | Shallow or data-bound PARTIAL manifest | Read returns nil error solely for these conditions. ValidateComplete wraps ErrSourceIncomplete plus ErrShallowHistory and/or ErrBoundExceeded for the matching facts. MaxProcesses is never such a cause. | FC-SOURCES |
| F3-REF-IDENTITY | Explicit ref versus all refs | Explicit: ResolvedRef=commit and ResolvedRefs has the one requested name/commit. All refs: ResolvedRef empty and ResolvedRefs sorted complete list. ReadingRef.Revision is stable live/git:<commit> text, parsed by ParseRevision. | FC-SOURCES |
| F4-MIXED-OPTIONS | Any legacy RunsDir/FeaturesRepo/non-nil FeaturesRepos/nonzero MaxHistoryCommits together with amended Sources/Selection/Bounds | Completed amended Build returns ErrInvalidSourceSpec before IO. FC-1 CLI maps legacy flag spellings into explicit sources/bounds and clears compatibility fields. No silent ignored location/cap. The scaffold itself still refuses all amended inputs with ErrNotImplemented. | FC-1 |
| F4-CANONICAL-LISTS | Empty version 4 payload lists or nested citations | Bodies serialize [] not null; JSON-value equality is the replay criterion. Nil Evidence still means unavailable. Reused Cell intentionally retains stable legacy Role/Model keys. | FC-1 |

Replay equality is conditional on the caller preserving the same source bytes,
live mtime and resolved Git objects. The artifact stores citations and resolved
refs, not live bytes, a content archive or a cryptographic snapshot identity.
A changed live file with preserved mtime is not distinguishable from these
citations alone. Portable audit verifies the recorded internal evidence, not
source authenticity or recovery of old live files. Content-addressed archiving
is outside F1-F4; no body or report may claim self-contained source replay.
All in-sample missing revision times are malformed in the source and join; excluded
quality counters retain malformed facts separately without degrading the in-sample corpus.

## Entry-point contracts

This section and the named examples are the authoritative behavioral handoff.
Seam godocs name inputs/errors and point here; baseline comments describe only
unchanged baseline behavior. No downstream row may amend these rules casually.

- **ParseEvents:** validate both JournalBounds fields/defaults before reading; enforce total bytes
  as well as line bytes, preserve
  producer identity and per-line diagnostics, use exact wire scalar names. Resolve
  sequence retransmissions/collisions by F2-EVENT-IDENTITY and panel shapes by
  F2-PRODUCER-SHAPES. Keep parsed data on read/cancellation errors; errors wrap
  ErrJournalSource or ErrSourceCancelled plus ctx.Err(). Invalid wire values and
  overflow are counted LinesUnparsed rather than returned as a whole-file error.
- **ReduceAttempts:** require a nonzero cutoff (ErrInvalidSelection), normalize UTC,
  and validate/deduplicate direct ParsedJournal inputs as ParseEvents does. Events
  strictly after cutoff supply no measurements; later starts are counted and
  omitted, not censored negatively. Exact IDs, ambiguity/conflicts and closing
  stamp rules follow F1. All nine producer spawn kinds are explicit; design is known auxiliary work. Every
  implementing/retry spawn uses the same duration-boundary rule in F2-INITIAL-SPAWN;
  only invocation-shaped panel_started opens a panel interval. Verifier wall uses
  verification_started→verification_verdict, not a sum of verifier spawns. Missing
  boundaries and all members of an overlap component are withheld, Complete=false,
  while total elapsed survives. Corrections/review/verifier/cascade event lists
  contain every counted ref INCLUDING the least FieldEvidence ref. Cost/token
  lists contain every available contributing ref including the least; missing
  contributions make the quantity unknown. Checked duration/count/token/cost
  overflow returns ErrMeasurementOverflow with retained valid data; reversed
  attempt elapsed returns ErrReversedInterval. Propagate added diagnostics to
  PARTIAL during Build. AttemptSet.Diagnostics retains parser counts plus reducer
  additions; Build augments aggregate quality only from those additions because
  ReadSources already counted parser facts. Never add both full totals. No saturation and no implicit clock read. Attempt.Wall.StartedAt must equal
  Attempt.ID.StartedAt and Wall.Elapsed must equal Attempt.Elapsed. Reduction,
  Attempt JSON encode/decode and joining reject an INPUT mismatch with ErrEvidenceConflict;
  the standalone wall validator only checks the wall it receives. When a valid
  YAML-only terminal is adopted, JoinEvidence is explicitly authorized to atomically
  recompute terminal/elapsed and Wall.Elapsed, withhold every interval extending
  outside the new bounds (no clipping), and set Wall.Complete=false if any span is
  withheld. Wall.StartedAt remains the attempt start. The resulting consistent wall
  can serialize. This is terminal reconciliation, not repair of an invalid input.
- **ReadSources:** validate a nonempty SourceSpec list including at least one journal-kind source,
  unique IDs, Selection, bounds and roots
  before IO. Non-history kinds require Ref empty; unsupported kind/field combinations
  wrap ErrInvalidSourceSpec. Reject duplicate discovered run IDs across all journal
  sources with ErrDuplicateJournalRun and aggregate PARTIAL diagnostics. Resolve zero bounds to positive defaults stored on SourceManifest.
  Require cutoff; validate all named holdouts against the full discovered run-ID
  universe before exclusion. Missing/unreadable requested repository/root/runs-dir
  is ErrSourceMissing; zero discovered journal files across all journal-kind sources is ErrSourceEmpty
  unless AllowEmpty (aggregate EMPTY, never eligible). Counts.Journals is discovery
  BEFORE exclusion; JournalsExcludedByHoldout counts held-out files separately,
  while ExcludedByHoldout counts only YAML envelopes. Holding out every discovered
  run remains COMPLETE if quality permits; the gate then refuses thin cells only.
  SourceReport states are COMPLETE/PARTIAL, including a zero-record source. EMPTY
  is the aggregate zero-journal diagnostic state. Partial/failed enumeration is
  never called successful zero discovery. Successfully scanned YAML with zero
  task rows can be COMPLETE.
  Decode string-tagged identity scalars (no numeric/bool coercion) and temporal
  selection nodes independently before typed predictive
  fields, preserving valid siblings and Reading.Identity despite unrelated errors.
  Source disposition precedence is NotTaskDocument→HeldOut→AfterCutoff→Malformed→
  MissingJoinKeys→matching result. Only valid independent identity proves a
  holdout. AfterCutoff uses Ref.RecordedAt or independently decoded start/terminal;
  all later envelopes are excluded audit. Missing revision time is malformed.
  Excluded malformed/unreadable facts are MalformedExcluded/UnreadableExcluded;
  they are diagnostic and do not make the selected corpus PARTIAL. If exclusion
  cannot be proved (for example invalid run identity), the fact stays in-sample
  and degrades completeness. Required-source discovery failure is never reclassified
  as an excluded-record failure. Preserve markers after predictive Snapshot fields are cleared; retain Identity/Ref/Err/CompletedAt for selection audit.
  Keep held-out journal identities in ExcludedJournals without their task payload.
  Git traversal enumerates full reachable history, including superseded merge
  parents/deletions/renames under explicit roots. Enforce streamed metadata,
  line/blob/total-byte/commit caps before collection; caps/shallow/grafted/replaced
  history are PARTIAL with reasons, not read errors solely for incompleteness.
  Process bounds serialize per-source children, never truncate data. Sources execute sequentially (in SourceID order), with per-source child slots
  only queueing work; no cross-source fan-out. Metadata/blobs stream under byte
  caps before buffering. Legacy gitLines/gitCatFileBatch reuse is prohibited.
  All amended Git execution uses runSourceGit with one shared sourceBudget per
  source, including journal/live reads in that budget. sourceGitCommand is private
  construction used only by runSourceGit. The bounded runner applies process slots
  and source-total bytes internally and each blob bound before buffering; no legacy
  gitLines/gitCatFileBatch calls are allowed. Discard ALL inherited GIT_* variables
  except GIT_EXEC_PATH; preserve PATH for the selected trusted Git installation.
  Install GIT_LITERAL_PATHSPECS=1, GIT_CONFIG_NOSYSTEM=1,
  GIT_CONFIG_SYSTEM=/dev/null, GIT_CONFIG_GLOBAL=/dev/null,
  GIT_TERMINAL_PROMPT=0, GIT_NO_LAZY_FETCH=1, GIT_NO_REPLACE_OBJECTS=1,
  GIT_ALLOW_PROTOCOL="", GIT_SSH_COMMAND=/bin/false, GIT_ASKPASS=/bin/false,
  and GIT_PROXY_COMMAND=/bin/false. Inherited location, namespace, config,
  alternate-object, pathspec, HTTP, SSH, proxy and external-diff overrides cannot
  survive that allowlist. Every command adds -c core.fsmonitor=false,
  -c credential.helper=, -c protocol.allow=never, and -c protocol.file.allow=never.
  Use only raw metadata/blob modes, never checkout, --filters, --textconv,
  external diff or user aliases; diff-tree explicitly disables ext-diff/textconv.
  Thus repository filter/diff/credential helpers have no execution path.
  Pin the repository and pass plain repository-relative roots after --;
  GIT_LITERAL_PATHSPECS=1 makes them literal. Do NOT combine that setting with
  :(literal) prefixes, which would themselves become literal path text. Use
  fixed read-only Git subcommands. Detect/report raw replace/graft metadata while disabling their
  interpretation; do not hide that evidence by merely disabling replacements. Refs resolve before traversal; all-ref tips are
  sorted and recorded. ValidateReadingRevision accepts only live or git:<full
  lowercase 40/64-hex object ID>; RecordedAt separately carries commit time/mtime.
  Cancellation returns retained data/PARTIAL plus ErrSourceCancelled and ctx.Err().
- **ValidateComplete:** nil/empty manifest, zero cutoff, invalid/duplicate source
  identities, nonpositive resolved bounds, non-COMPLETE aggregate/source states,
  Shallow/Grafted/Replaced flags, ProducerUncertain, cancellation, positive IN-SAMPLE malformed,
  unreadable or data-bound counters, invalid/negative counters, or no discovered journal across journal-kind sources
  wrap ErrSourceIncomplete. Counts.Journals is pre-exclusion, so all held out is
  still complete when quality permits. Excluded-quality counters do not degrade completeness.
  Any Shallow/Grafted/Replaced flag and data-bound facts additionally wrap ErrShallowHistory/ErrBoundExceeded.
- **JoinEvidence:** receive the full discovered JournalIdentity universe, including
  excluded journals. Duplicate RunID values across that universe, even identical
  replicas, wrap ErrDuplicateJournalRun before reduction/joining; select one journal
  source per run, never merge replicas. ReadSources returns aggregate PARTIAL
  metadata and that error; JoinEvidence returns no observations or LostAttempts.
  Refuse ANY supplied AttemptSet or individual Attempt naming a
  held-out run with ErrInvalidSelection before reconciliation; return no observations
  or LostAttempts. Every Attempt.ID.RunID and start-event journal/run must agree with
  its AttemptSet.Journal, also ErrInvalidSelection on mismatch. ReadSources.Journals
  contains only in-sample journals; excluded identities occur solely in ExcludedJournals.
  Validate every AttemptSet belongs to the universe, validate Selection
  and UnmatchedHoldouts at this entry point, and require each attempt cutoff to
  equal Selection.Cutoff. Marker mismatches or invalid universe wrap
  ErrInvalidSelection before reconciliation. Use independently valid Reading.Identity
  for holdout checks even when predictive fields failed. Identity owns run/key/start;
  ReadingSnapshot contains only role/authored-model/status/iteration count, so
  there is no second conflicting copy of the join keys. CompletedAt, when Known,
  remains usable for cutoff even if an unrelated predictive field failed. Verify every existing exclusion: HeldOut against independently valid run identity,
  AfterCutoff against Ref.RecordedAt, Identity.StartedAt or retained CompletedAt.
  An unsupported marker wraps ErrInvalidSelection. Rechecks may add exclusions
  but never silently remove a source exclusion. Match exact IDs, no proximity;
  precedence, atomic terminal tuple, role/model conflicts and canonical output are
  frozen in F1 rows. EvidenceJoin.ExcludedJournals contains the full canonical excluded identities
  from the validated universe; Build copies that list into ArtifactEvidence
  without reducing it to HeldOutRuns strings. Every envelope gets one disposition. Least compatible citation
  is Recovered; remaining compatible envelopes for the attempt are duplicates,
  including exact repeated envelopes. Excluded payload never contributes. Unknown
  stays unknown; invalid outcomes/roles/citations/measurements are not sampled.
  Missing/invalid ReadingRef revision or RecordedAt is Malformed (including direct
  inputs), not Unrecoverable. In-sample malformed citation evidence makes the build
  PARTIAL. Unrecoverable is reserved for normalized-attempt measurement/outcome errors.
  Overflow returns ErrMeasurementOverflow; noncanonical input submitted to wall
  validation returns ErrNonCanonicalEvidence. Full portable conflicts include both
  values and citations. Canonical ReadingRef order is SourceID, Repository, Path,
  Revision (bytewise strings), Row (numeric), RecordedAt (UTC instant). It controls
  recovered envelope choice, reading lists and YAML FieldEvidence ties. Identical
  refs compare equal; indistinguishable repeated envelopes still yield one recovered
  and remaining duplicate dispositions, with canonical audit ordering including
  disposition so input order cannot choose the serialized output. EvidenceNone and compatibility projection mapping follow
  F4-PROJECTION-MAPPING. Full event lists include the least ref, never just the rest.
- **Build / serialization:** resolve one extraction instant, set BOTH Artifact.GeneratedAt and
  SourceManifest.Cutoff to its Round(0).UTC() value, and reject mixed legacy
  and amended location/history options as already frozen. Retain diagnostic results
  on source/reducer/join errors; aggregate SourceManifest.Reasons is sorted unique
  text formatted "reduce: <sentinel-name>: <detail>" / "join: <sentinel-name>: <detail>"
  for reduction/join errors, with aggregate State=PARTIAL. Individual source reports
  continue to describe only their reads. Preserve full canonical payload and []
  lists. BaselineSchemaVersion=3 remains current; only the completed amended Build
  emits 4 with both payloads populated. Attempt's named JSON methods serialize
  textual outcome and reject missing/unknown outcome on decode; Censored returns
  ErrInvalidOutcome instead of treating invalid input as a plausible censoring flag.
  PredictionEligibility validates original targets and artifact records before use.

## Further independent seal cases

| Name | Input | Expected | Owner |
|---|---|---|---|
| F2-INITIAL-SPAWN | task_started T0, implementer finish T1, recorded duration D differs from T1-T0 | Development is [T1-D,T1), Inferred=false, residual includes setup/queue gap. Same rule for all seven implementing/retry kinds. Missing duration means no span and Complete=false; never substitute task_started. Outside spans and all overlapping candidate components are withheld, elapsed retained; direct SummarizeWall on invalid spans still errors. | FC-JOURNAL |
| F2-ALL-KINDS | Enumerate _account_spawn AND direct task_spawn_finished emission at the recorded revision | design, implementer, panel-iterate, verifier, verifier-iterate, test-fix-retry, commit-retry, push-retry, summary-recovery. Verifier is emitted directly, not through _account_spawn. The seven kinds other than verifier/design can stamp implementing model/development. Design never stamps implementing model or counts a correction; its time stays disclosed unclassified residual and makes the breakdown incomplete. Standalone retry finishes count corrections; paired panel/verifier finishes use their iterate marker once. | FC-JOURNAL |
| F2-CITATION-MEMBERSHIP | One cost-bearing spawn / two fallbacks | Evidence.Cost.Event==CostEvents[0], len=1. Cascades==len(CascadeEvents)==2; Evidence.Cascades cites element 0. Every aggregate list includes its least ref, not only remaining refs. Empty lists []. | FC-JOURNAL |
| F2-SUMMARY-AVAILABILITY | Identical numeric WallSummary parts from complete and incomplete inputs | Summary.Complete differs; a consumer can distinguish unavailable phases without consulting the original wall. | FC-JOURNAL |
| F3-MALFORMED-HELDOUT-IDENTITY | Valid run-ID scalar names holdout; role or cost field malformed | Independently decoded Identity.RunID remains Known; HeldOut wins and marker is rechecked without reading invalid Snapshot. Invalid run identity cannot prove holdout and remains malformed/PARTIAL unless another exclusion is independently proved. | FC-SOURCES / FC-1 |
| F3-EXCLUDED-QUALITY | Malformed excluded row versus malformed in-sample row | MalformedExcluded versus Malformed; only the latter degrades selected-source completeness. Unknown/unreadable identity cannot invent exclusion. | FC-SOURCES |
| F3-DIRECT-HOLDOUT | Direct JoinEvidence call with misspelled holdout | ErrInvalidSelection using full supplied journal universe. Real held-out journal with no task events remains valid through ExcludedJournals. | FC-1 |
| F3-AMENDED-GIT-HELPER | Environment redirects Git directory/config/helper | ReadSources executes Git through runSourceGit and its sourceGitCommand constructor only; no fallback to inherited-env baseline helpers. | FC-SOURCES |
| F3-REVISION-CANONICAL | live; git:full lower SHA; bare/abbreviated SHA; live:mtime | First two valid; latter forms ErrUnparseableRevision. Legacy ParseRevision behavior is unchanged. | FC-SOURCES |
| F4-OUTCOME-WIRE | Attempt done/blocked/unfinished, invalid direct outcome, missing/unknown JSON outcome | Marshal/unmarshal uses stable text. Invalid/missing outcome => ErrInvalidOutcome; Censored returns error on invalid value. Conflict terminal outcome uses same text. Baseline Outcome enum untouched. | FC-JOURNAL |
| F4-AGGREGATE-REASON | Cross-source join or reducer error with successful source reads | Aggregate manifest PARTIAL with sorted named Reasons surviving artifact serialization; individual source reports remain truthful. | FC-1 |
| F4-PROJECTION-MAPPING | EvidenceNone / YAML / Journal in schema 4 | Flat TerminalEvidence none/yaml/journal respectively; structured source stays empty/yaml/journal. Amended sampling reads structured attempts only. | FC-1 |

## Selection and replay cases

| Name | Input | Expected | Owner |
|---|---|---|---|
| F3-DIRECT-HOLDOUT-ATTEMPTS | Correctly named holdout supplied as any AttemptSet/Attempt | ErrInvalidSelection before reconciliation; no observations and no LostAttempts. Full journal universe may contain the held-out identity, but attempt sets may not. | FC-1 |
| F1-ATTEMPT-RUN-CONSISTENCY | Attempt/start run ID disagrees with its AttemptSet.Journal | ErrInvalidSelection before matching; cannot disguise held-out events under a different run. | FC-1 |
| F2-WALL-PARENT-CONSISTENCY | Wall start/elapsed differs from parent Attempt | ErrEvidenceConflict from reducer, Attempt JSON methods and join; standalone SummarizeWall validates only its input. | FC-JOURNAL / FC-1 |
| F1-READING-TOTAL-ORDER | Permuted refs differing in each member; exact duplicate refs | Compare SourceID, Repository, Path, Revision bytes; Row numeric; UTC RecordedAt. Equal refs compare equal. Recovered/duplicate audit output is canonical including disposition for otherwise identical envelopes. | FC-1 |
| F4-MANIFEST-EMPTY-LISTS | Empty manifest roots/reasons/refs/holdouts | [] keys retained, not omitted; bodies initialize nonnil lists. | FC-SOURCES / FC-1 |

| Name | Input | Expected | Owner |
|---|---|---|---|
| F3-ALL-JOURNALS-HELDOUT | One discovered journal, that run held out, otherwise valid sources | Journals=1, JournalsExcludedByHoldout=1; COMPLETE manifest. No samples; prediction refuses thin cells with ErrNotEligible, not ErrSourceIncomplete. | FC-SOURCES / FC-1 |
| F4-ONE-ARTIFACT-INSTANT | Explicit cutoff differs from opts.Now | GeneratedAt==manifest.Cutoff==captured UTC instant without monotonic data. opts.Now is fallback only. | FC-1 |
| F3-SOURCE-CONCURRENCY | Multiple sources, MaxProcesses=2 | Sources execute sequentially in SourceID order; at most two git children within the current source. No cross-source fan-out and no legacy buffered helper reuse. | FC-SOURCES |
| F3-EXCLUDED-JOURNAL-AUDIT | Held-out journal has path/source/producer but no events | Full identity survives universe→EvidenceJoin.ExcludedJournals→ArtifactEvidence.ExcludedJournals. | FC-1 |
| F3-GIT-INSTALLATION-ENV | Custom PATH/GIT_EXEC_PATH with poisoned location/config vars | Installation vars remain; named location/config/pathspec overrides are cleared, explicit no-helper/no-network settings apply. | FC-SOURCES |

### Canonical reconciliation totals and list order

Counted attempt IDs are the distinct IDs in the in-sample AttemptSet.Attempts,
Ambiguous and Conflicts collections; after-cutoff starts and held-out runs are
excluded. Attempts is that set's size; UniqueRows is its distinct (run,key)
count. Recovered is len(Observations), one per recovered ID. LostAttempts is the
sorted counted-ID set minus recovered IDs, including ambiguous/conflicting IDs;
Ambiguous.Starts separately preserves how many distinct starts shared an ID.
A stale/unmatched YAML identity never increases the journal attempt denominator.
Dispositions includes every declared value, including zero, in Dispositions()
report order; counts sum to len(Examined), with one recovered envelope per sample.

Attempt IDs sort by RunID, Key, UTC start. Observations/LostAttempts/Ambiguous sort
by ID. Reading lists use ReadingRef's exact order. Examined sorts by ReadingRef,
AttemptID, disposition string, then reason. Conflict sides are ordered by complete
FieldEvidence (source then the corresponding canonical event/reading citation),
then Kind and canonical candidate JSON bytes for equal citations. Conflicts sort
by ID, field and the ordered candidates. Journal identities sort by run/source/
path/producer; held-out run IDs and aggregate reasons sort lexically. Canonical
strings compare bytewise; timestamps compare UTC instants without monotonic data.
No nonfinite measurement may enter candidate JSON. These rules apply to portable
output; they do not give input slice order evidentiary authority.

## Final wire and execution rules

- ParseEvents decodes JournalLine's exact producer keys. Seq pointer presence
  determines HasSeq; EventRef's type/at/has_seq names are portable-output keys only.
  DispatcherVersion is read only from run_started payloads. Missing/null declarations
  add no candidate; malformed type or blank/padded value counts LinesUnparsed.
  Compatible repeated nonblank versions resolve once; distinct versions set
  ProducerConflict and resolve Producer to empty. ProducerVersions retains the
  sorted unique valid declarations; MissingProducer is true when no valid candidate
  exists. Never trust an input JournalIdentity.Producer instead of the wire.
  Parsed task events are retained for diagnostics with the resolved identity.
  In-sample missing/conflicting/unsupported producer makes ProducerUncertain and
  PARTIAL on the source report. ReduceAttempts refuses conflicts with ErrEvidenceConflict,
  and missing/unsupported producer with ErrJournalSource; it never guesses event
  semantics outside the recorded 0.1.0 contract. Excluded journals do not degrade
  in-sample quality. SourceReport flags preserve shallow/graft/replace separately.
- JournalBounds.MaxLineBytes excludes LF/CRLF terminators; MaxTotalBytes counts raw
  consumed bytes, including terminators and skipped lines. Defaults are 16MiB/512MiB;
  a cap retains parsed data and sets LinesOverBound or TotalBoundExceeded. The parser
  uses bounded lookahead of at most one byte to distinguish exact EOF from overflow,
  never parses/stores that probe, and includes it in Diagnostics.Bytes. ReadSources
  maps each exceeded line and the total cap to BoundsExceeded and PARTIAL,
  counting the same shared total-cap occurrence only once. Source
  budget accounting counts physical reads once, not again from parser diagnostics.
- newSourceBudget resolves positive defaults; private state may implement shared
  counters and semaphores. runSourceGit validates budget/request before spawning.
  Blob requests are exactly cat-file blob <full-object-id>; metadata requests are
  fixed read-only rev-parse, rev-list, diff-tree, ls-tree, for-each-ref or show-ref
  operations. Arbitrary subcommands, batch/filter modes and shell invocation are
  rejected with ErrInvalidSourceSpec. Metadata streams under shared total bytes;
  each Blob also has MaxBlobBytes. Byte budgets allow at most one bounded EOF probe
  and count it as consumed, never retained data. stderr is bounded by remaining
  source-total bytes. Read surfaces nonzero exit as ErrGitHistory, cancellation as
  ErrSourceCancelled plus ctx.Err(), and data caps as ErrBoundExceeded. EOF waits for
  exit so it cannot hide failure. Close cancels/reaps/releases slots even on early
  abandonment and is idempotent. Baseline helpers remain in their required source
  ownership file; only the bounded runner may execute amended Git requests.
- Empty contexts at context-taking entry points are normalized to Background;
  cancellation retains data/diagnostics. A nil sourceBudget is ErrInvalidSourceSpec,
  never a panic or unbounded default.
- Excluded readings retain independently decoded CompletedAt for cutoff proof;
  predictive Snapshot fields are cleared. JoinEvidence validates markers against
  retained times and rejects forged/inconsistent AfterCutoff markers. Examined
  preserves Identity and CompletedAt alongside the citation/disposition so the
  portable audit can verify the exclusion without reopening YAML.
- PredictionEligibility refuses observations whose run is in manifest holdouts or
  whose cutoff differs from manifest cutoff, using the invalid-payload error rule.
  Eligibility.Cells is computed from joint evidence, sorted by role/model, and
  reports completed count, effective threshold and eligibility for each required
  cell. Reasons renders these structured facts; an empty cell has completed=0.
  RowsWithYAMLOnlyTerminal counts recovered attempts with YAML terminal evidence,
  not readings, in EvidenceJoin and ArtifactEvidence; the flat legacy counter is
  just a projection. Unrecorded design work timing stays explicitly unclassified,
  never an implementing-model stamp or an invented correction.

| Name | Input | Expected | Owner |
|---|---|---|---|
| F2-YAML-TERMINAL-WALL | Consistent unfinished attempt to C, YAML terminal T<C | Atomically adopt terminal/elapsed and Wall.Elapsed. Withhold out-of-bounds intervals without clipping, Complete=false if any withheld. Output wall/parent agree and serialize; preexisting mismatch still ErrEvidenceConflict. | FC-1 |
| F2-DESIGN-SPAWN | Accounted design spawn | Recorded cost/tokens included; no implementing model/correction. Duration remains disclosed unclassified residual and Wall.Complete=false. | FC-JOURNAL |
| F2-PRODUCER-DECLARATIONS | Missing/null, malformed, repeated-compatible or conflicting run_started versions | Named diagnostics and deterministic resolution as above; uncertain in-sample producer makes source PARTIAL. | FC-JOURNAL / FC-SOURCES |
| F2-LINE-ENVELOPE | seq absent/null/zero and hash/prev_hash | HasSeq exact presence; exact producer keys preserved, portable EventRef names never used as input wire keys. | FC-JOURNAL |
| F2-PARSER-TOTAL-CAP | Many short lines exceed standalone JournalBounds.MaxTotalBytes | Bounded parse, retained data and TotalBoundExceeded; no unbounded standalone path. | FC-JOURNAL |
| F3-EXCLUSION-PROOF | AfterCutoff marker, only completion time supplies cause | Completion proof survives source and Examined audit; marker with no retained time after cutoff => ErrInvalidSelection. | FC-SOURCES / FC-1 |
| F3-DUPLICATE-JOURNAL-RUN | Two journal identities share run ID, including identical replicas | ErrDuplicateJournalRun; no replica reconciliation/double counting. ReadSources retains PARTIAL metadata; direct Join returns no samples/lost attempts. | FC-SOURCES / FC-1 |
| F3-UNSUPPORTED-REF | Ref on live-YAML or journal source | ErrInvalidSourceSpec before IO. | FC-SOURCES |
| F3-GIT-RUNNER | Poisoned env, long blob/metadata output, concurrent child requests, cancellation/early close | Mandatory isolated streaming runner enforces shared budget and blob bound; process slots released/reaped; no legacy helper execution. | FC-SOURCES |
| F3-HISTORY-FACTS | Shallow versus grafted versus replaced | Separate flags persist and each causes ErrShallowHistory with ErrSourceIncomplete at validation. | FC-SOURCES |
| F4-ARTIFACT-HOLDOUT | Hand-built artifact contains a held-out run or wrong cutoff | Invalid-payload refusal: ErrNotEligible plus ErrSourceIncomplete when refusing. | FC-1 |
| F4-STRUCTURED-THIN-CELL | Required cell below threshold | Eligibility.Cells names role/model/completed/threshold; Reasons is only its human rendering. | FC-1 |

Historical reviewer dispositions are preserved in the correction audit directory
and in prior commits, not repeated in this behavioral handoff.

## Compatibility projection and bounded execution details

Version 4 creates one ReferenceObservation per recovered AttemptID in canonical
ID order, without Table.Add or any legacy max-fold/dedupe. BuildResult.Table is
nil on the amended path: that legacy table cannot represent distinct run IDs.
Consumers of amended results use Artifact/Evidence. Baseline Table remains unchanged.

| Flat field | Exact version 4 mapping |
|---|---|
| Key, DispatcherRunID, StartedAt | Attempt.ID key/run/UTC start |
| Role, Model | RecoveredAttempt.Cell; must agree with the validated stamped model |
| Outcome, Censored | Attempt.Outcome.String(); outcome != done |
| TerminalEvidence | structured none/yaml/journal maps to legacy none/yaml/journal |
| ElapsedSeconds | Attempt.Elapsed.Seconds(), including censored lower bounds |
| DevelopmentSeconds, ReviewSeconds | sums of retained disjoint development and panel_review spans in seconds; verifier is separate in Evidence, never folded into review |
| Rounds, Cascades | Attempt.Corrections, Attempt.Cascades; never YAML iteration_count or review invocation count |
| InputTokens, OutputTokens | measured value when Known, otherwise legacy zero placeholder; Evidence carries availability |
| CostUSD | pointer to known value, including zero; otherwise null |
| SourceRevision, SourceRepository, SourcePath | least canonical recovered ReadingRef's respective members; citation of the reading, not per-field authority |

Legacy phase sums and unknown-token zero placeholders cannot express availability;
Limits must disclose this, and amended sampling/eligibility never reads them.
CellSummary groups these same distinct recovered IDs by role/model, plus empty
requested target cells. N counts all, NDone counts done, NBlocked counts blocked,
NCensored counts blocked+unfinished. Duration uses done elapsed seconds only;
Rounds uses correction counts of all outcomes. NumericSummary uses N/min/mean/
median/max, empty all-zero; cells sort role/model. Coverage's target and recovery
projections use these same joint counts and the named disposition/manifest facts,
never a second extraction or Table merge. Evidence is the authoritative diagnostic
payload; legacy counters without an exact amended counterpart remain zero and are
explicitly labeled unavailable in Limits, rather than guessed from unrelated counts.

All live/journal file reads use openSourceFile with the same sourceBudget as Git.
Byte limits apply before retaining bytes; journal line limits remain ParseEvents'
responsibility and physical bytes are never charged twice. Slot acquisition in
runSourceGit is context-cancellable and returns the same wrapped cancellation as
an active child. Bounds are per explicitly requested source, not a total-RSS claim.
Pure ReduceAttempts/JoinEvidence/SummarizeWall are finite in-memory operations over
that bounded input and do not spawn children. They have no separate deadline; Build
checks ctx between source/reduce/join phases and refuses cancelled results. A global
corpus/RSS cap or required caller deadline is outside this contract and must not be
advertised. Callers control how many explicit sources they request.

SummarizeWall is the phase checker: the exhaustive classified values are development,
panel_review and verifier. Unclassified is a report key only; all other interval
values wrap ErrInvalidPhase. Equivalent enum helpers are optional body details.

## Operator F3 clarification after source follow-up review

All-ref history retains Git --all semantics: every captured canonical refs/...
tip AND the captured HEAD commit, including detached HEAD. Record HEAD explicitly
in ResolvedRefs when it resolves; it is the one allowed pseudoref name in an
implicit-ref snapshot. Other implicit names must follow full Git refs/... syntax.
Explicit requested expressions retain their existing separate validation. An
unborn/no-resolved-commit history is PARTIAL with a reason; ValidateComplete
refuses it. Capture IDs once and peel captured object IDs, never reread mutable
names as fallback. No COMPLETE claim may silently omit detached-HEAD evidence.

The commit cap must stop traversal work, not only truncate emitted lines after
an eager topological walk. Uncapped complete history still includes every
reachable parent. Bound traversal argv independently of ref count (e.g. bounded
snapshot-tip batches); no arbitrary total-process-count performance threshold
is introduced. Preserve source completeness, deterministic retained evidence and
unique commit counting under the cap.

A forcibly closed/truncated Git stream is an error, never a synthesized clean EOF.
Cleanup must not discard buffered successful output just because its consumer
waited more than one second. Cancellation and inherited-pipe cleanup remain bounded.
Final-component symlinks require atomic refusal relative to the confined parent
descriptor; a pre-open Lstat alone is not proof. Platform-specific private file
open helpers are permitted to preserve portable package builds; frozen public
seams and test ownership do not change. Their implementations belong to FC-SOURCES.
