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
| FC-JOURNAL | ParseEvents, ReduceAttempts, SummarizeWall; canonical attempt/time/cost evidence. |
| FC-SOURCES | Source/bound/selection validation, parseReadings, ReadSources, manifest validation. Depends on FC-JOURNAL because ReadSources consumes its parser. |
| FC-1 | JoinEvidence, amended Build/PredictionEligibility, version 4 ArtifactEvidence and CLI. |

Decision functions above return ErrNotImplemented in this scaffold. Validation
method descriptions state the required body behavior; they are not claims of
implemented validation. Simple enum membership and data constructors remain;
they do not implement the extraction/join policy. No code in a dependent body
needs to edit observation.go or extract.go to fill these seams.

## Contract rulings

- RecoveredAttempt contains the joint Attempt, its Cell and contributing YAML
  readings. The legacy max-fold table is never used for amended reconciliation.
  Select each value and its full citation together. Terminal outcome, terminal
  instant and elapsed are one unit; journal outranks YAML, and equal-authority
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
  identity-only AfterCutoff. This prevents a stripped journal terminal reappearing
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
- ReadSources retains identity-only excluded YAML envelopes and separately
  records excluded journal identities. JoinEvidence audits every envelope and
  excludes it from contribution. Source marks are rechecked against validated
  Selection; the reducer ignores all post-cutoff journal evidence.
- A non-task YAML document is neither a malformed task nor a lost attempt.
  Decode task rows independently. Invalid siblings cannot erase valid rows.
- ReadSources validates before IO, applies zero-bound defaults, and reports
  shallow/capped/malformed reads as PARTIAL with reasons. Only cancellation or
  actual discovery/read failure returns a read error. Completeness is refused
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

The exact producer version string is `0.1.0`. Fixtures must cite an actual
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
| F1-ID-AMBIGUOUS-START | Two `task_started` for one key at one instant in one run | Neither chosen; `AttemptSet.Ambiguous=[{id, Starts:2}]`; readings → `DispositionAmbiguousStart`. | `ErrAmbiguousAttempt` | FC-JOURNAL / FC-1 |
| F1-ID-NEAREST-NOT-MATCHED | YAML start 1 s from the only `task_started` | `DispositionNoMatchingStart`; attempt in `LostAttempts`. | `EvidenceJoin` | FC-1 |
| F1-EV-PROVENANCE-KEPT | Any recovered reading | Every field of `ObservationEvidence` (model, start, terminal, elapsed, wall, corrections, cascades, reviews, verifications, input tokens, output tokens, cost) carries a `FieldEvidence` naming its `ReadingRef` or `EventRef`; a summed field cites its least event and `Attempt.CostEvents`/`InputTokenEvents`/`OutputTokenEvents` list the rest; an unknown value has `EvidenceNone`. | `ObservationEvidence` | FC-JOURNAL / FC-1 |
| F1-EV-TOKENS-CITED-SEPARATELY | Spawn with `output_tokens` but no `input_tokens` | Event in `OutputTokenEvents` only; `InputTokens` unaffected; `Evidence.InputTokens` and `Evidence.OutputTokens` differ. | `Attempt.InputTokenEvents` | FC-JOURNAL |
| F1-EV-MERGE-PERMUTATION | YAML done at 12m, journal done at 10m; permutations | Journal outcome/time/elapsed/citations selected together (10m); equal-authority incompatible values conflict. All other values travel with their citations. | JoinEvidence, RecoveredAttempt | FC-1 |
| F1-ROW-EQUALITY-BY-CONTENT | Equivalent event instants and unordered input citations/intervals | Join output has canonical timestamps/list order; structural contents and serialized values agree under permutations. Legacy Observation equality is unchanged. | RecoveredAttempt | FC-JOURNAL / FC-1 |
| F1-JOURNAL-PRODUCER-RESOLVED | Journal with `run_started` and no task events | `ParsedJournal.Journal.Producer == "0.1.0"`, `Events` empty, `MissingProducer=false`; a journal without `run_started` → `Producer==""`, `MissingProducer=true`. | `ParsedJournal` | FC-JOURNAL |
| F1-EV-NO-MANUFACTURED-ROW | Journal terminal 10 m and implementing spawn cost 1; reading X: elapsed 10 m cost 1; reading Y (same attempt, different revision): elapsed 12 m cost unknown | Terminal/elapsed from journal; cost `Known(1)` cited to its spawn events; no field takes an independent max attributed to X or Y. | `Attempt.CostEvents` | FC-1 |
| F1-EV-JOURNAL-OVER-YAML | Journal terminal T1, differing YAML terminal T2/outcome | Choose journal terminal tuple; only equal-authority contradictions are conflicts. | Attempt.Evidence | FC-1 |
| F1-EV-YAML-ONLY-TERMINAL | No journal terminal; YAML `Done`+`completed_at`, both revision and terminal at/before cutoff | Outcome done, `Terminal.Source=EvidenceYAML`, counted in `RowsWithYAMLOnlyTerminalEvidence`. | `FieldEvidence` | FC-1 |
| F1-EV-UNKNOWN-STAYS-UNKNOWN | No terminal anywhere | `OutcomeUnfinished`, `Terminal.Source=EvidenceNone`, elapsed to cutoff, censored. | `Attempt.Censored` | FC-JOURNAL |
| F1-EV-MODEL-CONFLICT | Two implementing spawns in one attempt with different models and no cascade ordering | Last recorded implementing stamp wins (closing model); a cascade is `Cascades≥1`. Equal-authority contradiction that cannot be ordered → `AttemptConflict{Field:"model"}`. | `ErrEvidenceConflict` | FC-JOURNAL |
| F1-EV-TERMINAL-CONFLICT | `task_done` and `task_blocked` in one attempt | `AttemptConflict{Field:"terminal"}`; excluded, `DispositionConflictingEvidence`. | `ErrEvidenceConflict` | FC-JOURNAL / FC-1 |
| F1-EV-PERMUTATION | Inputs/corroborating citations reordered | Same canonical recovered samples/audit; tie order includes every journal and reading member, including Producer and UTC instant. | JoinEvidence | FC-1 |
| F1-MODEL-CLOSING-STAMP | implementer `opus` → fallback → panel-iterate `sol` | `Model=Known("sol")`, `Cascades=1`, disclosed; not pooled with `opus`. | `Attempt.Model` | FC-JOURNAL |
| F1-MODEL-NO-ALIAS-POOL | `claude-opus-5` and `opus-5` | Two cells. | `Cell` | FC-1 |
| F1-MODEL-ABSENT-STAMP | No implementing spawn carries a model; YAML `model: opus` | `Model=Unknown`, `DispositionAbsentStamp`; authored model never substituted. | `ErrUnattributable` | FC-JOURNAL / FC-1 |
| F2-ELAPSED-TERMINAL | start T0, `task_done` T1 | `Elapsed=T1−T0`, not censored. | `Attempt` | FC-JOURNAL |
| F2-ELAPSED-CUTOFF | start T0, no terminal, cutoff C | `Elapsed=C−T0`, censored; never in a duration mean. | `Attempt.Censored` | FC-JOURNAL |
| F2-ELAPSED-BLOCKED-CENSORED | `task_blocked` | Censored lower bound; excluded from completed samples; counted in `NBlocked`. | `Observation.Duration` | FC-JOURNAL / FC-1 |
| F2-ELAPSED-SURVIVES-UNKNOWN-PHASES | Terminal present, no panel events | `Elapsed` known, `Wall.Complete=false`, `Wall.Intervals` empty, `WallSummary.Unclassified=Elapsed`. | `WallBreakdown` | FC-JOURNAL |
| F2-PHASES-DISJOINT | Production order above | Intervals disjoint, contained, classified sum ≤ `Elapsed`; residual `Unclassified`, never development. | `SummarizeWall` | FC-JOURNAL |
| F2-PHASES-PANEL-WALL | Three reviewer seats in one panel | One `panel_review` interval `panel_started→panel_verdict`. | `Interval` | FC-JOURNAL |
| F2-PHASES-ITERATE-AFTER-SPAWN | `spawn_finished(panel-iterate)` then `panel_iterate` | Corrective work interval ends at the spawn finish; `panel_iterate` is not a start boundary. | `Interval.Evidence` | FC-JOURNAL |
| F2-PHASES-INFERRED-LABELED | Boundary not recorded but derivable | `Inferred=true`; ambiguous attribution → no interval, `Complete=false`. | `Interval.Inferred` | FC-JOURNAL |
| F2-ROUNDS-VS-REVIEWS | first review, then two corrections | `Reviews=3`, `Corrections=2`. | `Attempt` | FC-JOURNAL |
| F2-COST-NULL-VS-ZERO | spawn `cost_usd: null` vs `cost_usd: 0` | `Unknown` vs `Known(0)`. | `Measured[float64]` | FC-JOURNAL |
| F2-COST-NO-DOUBLE-SUM | Same spawn seen twice (duplicate line) | Summed once; `CostEvents` lists one ref. | `Attempt.CostEvents` | FC-JOURNAL |
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
| F3-GIT-ENV-STRIPPED | GIT_DIR or GIT_CONFIG_GLOBAL/COUNT/KEY_n/VALUE_n redirects location/config/helper | Strip inherited Git overrides, ignore global/system config, pin repository, invoke no configured helper; bounded enumeration stays in selected repository. | ReadSources | FC-SOURCES |
| F3-GIT-SHALLOW | Shallow/grafted/replaced clone | Shallow=true, PARTIAL, named reason; nil read error solely for this condition; no fetching. Eligibility refuses. | SourceReport | FC-SOURCES / FC-1 |
| F3-GIT-FULL-HISTORY | Side branch superseded at merge, ref deleted | Its blob enumerated and read. | `ReadSources` | FC-SOURCES |
| F3-GIT-DELETED-RENAMED | File deleted or renamed in history | Old content still read. | `ReadSources` | FC-SOURCES |
| F3-BOUND-COMMITS | MaxCommits=3, five reachable commits | Limit enumeration before collecting; report bound/PARTIAL, nil read error solely for cap. Gate refuses. | ReadSources | FC-SOURCES |
| F3-BOUND-BYTES | Blob over `MaxBlobBytes`, line over `MaxLineBytes`, or total over `MaxTotalBytes` | Not read; `BoundsExceeded` counted; `SourcePartial`. | `ErrBoundExceeded` | FC-SOURCES |
| F3-BOUND-PROCESSES | `MaxProcesses=1`, two git reads of one source concurrently | Serializer only: at most one git child per source in flight; the second read waits for a slot rather than spawning. The cap never stops a read, never increments `BoundsExceeded`, never wraps `ErrBoundExceeded`, never changes counts, order or `SourceState`; a busy host completes COMPLETE. `MaxProcesses<0` → `ErrInvalidSourceSpec`. | `ReadBounds.MaxProcesses` | FC-SOURCES |
| F3-CANCELLED | Context cancelled mid-read | `ErrSourceCancelled` (also `context.Canceled`); partial manifest with `Cancelled=true`, never COMPLETE. | `SourceReport.Cancelled` | FC-SOURCES |
| F3-HOLDOUT-EXCLUDED | Held-out R in live/history/journals | Keep identity-only YAML envelopes marked HeldOut and journal identities in ExcludedJournals; no held-out predictive payload/events enter reducer/join. Audit includes the excluded envelopes once. | SourceReadings, Reading.Excluded, EvidenceJoin | FC-SOURCES / FC-1 |
| F3-CUTOFF-EXCLUDED | Row starts after cutoff; journal terminal after cutoff | Identity-only row marker AfterCutoff; reducer ignores post-cutoff events, preserving a prior attempt as censored at cutoff. No later outcome/model leakage. | Reading.Excluded, ReduceAttempts | FC-JOURNAL / FC-SOURCES / FC-1 |
| F3-SELECTION-INVALID | Blank, padded or duplicate held-out ID | ErrInvalidSelection before IO/reconciliation, even if validation helper was not called by the caller. | ReadSources, JoinEvidence | FC-SOURCES / FC-1 |
| F3-HOLDOUT-PADDED-STILL-EXCLUDES | Padded ID passed straight to ReadSources or JoinEvidence | Reject ErrInvalidSelection; no observations. Validation is required at the entry point, no permissive matching helper. | ReadSources, JoinEvidence | FC-SOURCES / FC-1 |
| F3-HOLDOUT-UNMATCHED | Named holdout not among discovered journal run IDs | ErrInvalidSelection before reduction. A matched journal need not have any task events/YAML; ExcludedJournals records its identity. | Selection.UnmatchedHoldouts | FC-SOURCES |
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
| F2-CANONICAL-ORDER | Same spans in different raw orders | Reducer emits canonical order; wall validator rejects noncanonical input instead of accepting a different equality convention. | ReduceAttempts, SummarizeWall | FC-JOURNAL |
| F2-PARTIAL-SUM-UNKNOWN | One spawn has cost/tokens, another lacks them | Total unknown with available citations retained; never present the observed partial sum as a complete measurement. | Attempt | FC-JOURNAL |
| F3-NON-TASK-DOCUMENT | Ordinary known-red/config YAML under selected root | NonTaskDocuments increments, DocumentNotTasks and matching disposition; no malformed count/PARTIAL solely from this file. | Reading.Kind, SourceCounts | FC-SOURCES / FC-1 |
| F3-COMPLETE-CONSISTENCY | Nil manifest or COMPLETE label with shallow/cancelled/bound/malformed flags | ErrSourceIncomplete; labels cannot override contradictory facts. | SourceManifest.ValidateComplete | FC-SOURCES |
| F3-DEFAULT-BOUNDS | Zero fields or negative fields | Zero uses frozen defaults; negative rejected before IO. MaxProcesses queues work, never truncates data. | ReadSources, ParseEvents (line bound) | FC-SOURCES / FC-JOURNAL |
| F4-THRESHOLD-NONPOSITIVE | minCompleted zero/negative | DefaultMinObservations applied; effective positive threshold returned. | PredictionEligibility | FC-1 |
| F4-SCHEMA-ROUNDTRIP | Full joint attempt with evidence, wall, review/verifier counts and cost/token event lists | Version 4 Evidence round-trip retains all fields; nil Evidence means unavailable, zero counts inside payload remain present. Legacy version 3 cannot license amended sampling. | ArtifactEvidence | FC-1 |

## Additional seals from the corrected-head review

| Name | Input | Expected | Owner |
|---|---|---|---|
| F2-SPAWN-WIRE | Actual producer payload with scalar cost/token/iteration and duration_ms 1250; repeat null/missing/zero | Pointers retain absence vs zero; duration converts to 1.25s. No scalar-to-Measured decode. Overflow/negative/type errors count LinesUnparsed and cannot reach samples. | FC-JOURNAL |
| F2-CORRECTIVE-BOUNDARY | panel-iterate spawn finishes at T with valid duration D | Development span [T-D,T), Inferred=false because producer records duration. If missing D, no interval; retain elapsed and Complete=false. Do not use preceding panel_verdict as an assumed start: it may include queue/setup time. If the span overlaps a review/verifier or is outside the attempt, omit the ambiguous span, retain elapsed, Complete=false; SummarizeWall rejects such spans supplied directly. | FC-JOURNAL |
| F1-CITATION-ROW | Two rows at one source/path/revision | Ref.Row differs. Full field evidence and serialized citations identify each; exact duplicate envelopes remain separately audited. Compatible readings of one AttemptID yield one recovered and remaining duplicates. | FC-SOURCES / FC-1 |
| F3-CUTOFF-REPLAY | Fixed cutoff C; rebuild with different host time; live/git revision or terminal later than C | Same elapsed/outcomes from eligible evidence. Later envelopes AfterCutoff; no later YAML terminal restores a post-cutoff journal terminal. Zero cutoff at a direct seam or mismatched Attempt.Cutoff => ErrInvalidSelection. Build captures one instant before reading. | FC-JOURNAL / FC-SOURCES / FC-1 |
| F3-RESOLVED-BOUNDS | Requested zero bounds | Manifest stores positive effective defaults, including DefaultMaxCommits; it can be replayed without applying a future version's defaults. | FC-SOURCES |
| F3-CANCEL-PERSIST | Cancellation after some records | ParseEvents retains parsed events; ReadSources returns PARTIAL manifest/readings with wrapping context error; Build returns diagnostic result plus error; CLI writes/report manifest before failing. | FC-JOURNAL / FC-SOURCES / FC-1 |
| F4-TARGET-INPUT | Duplicate/blank keys in original TargetRow slice but apparently valid aggregate Coverage | Gate rejects ErrInvalidTarget before source/sample checks. It validates original rows, not reconstructed aggregates; no file IO in the gate. | FC-1 |
| F4-VERSION-EXACT | Legacy schema 3, unknown schema 5, or schema 4 missing Evidence/manifest | Refusal; only exact EvidenceSchemaVersion=4 with required valid payload can pass. | FC-1 |

## Review disposition and validation

The prior 26 findings (14 High) are mapped below. Removal means the premature
implementation was removed from this scaffold; the required behavior remains
in the independent seals/body handoff above. This is not a waiver of that proof.

| Finding | Resolution / remaining proof |
|---|---|
| claude-1 (HIGH) | Legacy merge restored; amended RecoveredAttempt and atomic-value contract belong to FC-1. |
| claude-2 (HIGH) | DocumentNotTasks and NonTaskDocuments separate ordinary YAML; decoder is a FC-SOURCES stub. |
| claude-3 (HIGH) | Unauthorized new test removed; existing test restored exactly to baseline; all cases handed to FC-SEALS. |
| claude-4 (MEDIUM) | Premature validator removed; SummarizeWall requires a nonzero start and named error. |
| claude-5 (MEDIUM) | Premature preferEvidence removed; complete citation tie order frozen for FC-1. |
| claude-6 (MEDIUM) | New Observation.Equal removed; legacy row unchanged; amended output uses canonical instants/citations. |
| claude-7 (MEDIUM) | Incomplete Git filter removed; ReadSources requires stripping all inherited Git location/config overrides. |
| claude-8 (MEDIUM) | Unsafe Complete boolean removed; nil-safe ValidateComplete error seam frozen. |
| claude-9 (LOW) | Disposition.Valid uses an allocation-free switch. |
| codex-1 (HIGH) | Legacy merge restored; amended atomic terminal tuple and field citations frozen separately. |
| codex-2 (HIGH) | Excluded YAML audit envelopes retained with markers; no predictive values; join audits once. |
| codex-3 (HIGH) | Premature whole-document decoder removed; per-row decoding specified on the FC-SOURCES stub. |
| codex-4 (HIGH) | Build guard detects non-nil empty source and holdout lists. |
| codex-5 (HIGH) | Read is diagnostic PARTIAL on incompleteness; explicit gate error/default policy is frozen. |
| codex-6 (HIGH) | Version 4 ArtifactEvidence carries full recovered attempts and citations/verification counts. |
| codex-7 (HIGH) | All scaffold test modifications removed; decision implementations replaced by named stubs. |
| codex-8 (HIGH) | Unclassified is residual only; SummarizeWall rejects explicit unclassified spans. |
| codex-9 (MEDIUM) | ErrEvidenceConflict applies to incompatible measurements/terminal units; premature mergeWall removed. |
| grok-1 (HIGH) | Source exclusions keep audit markers and journal identities instead of deleting audit evidence. |
| grok-2 (HIGH) | New equality helper removed; canonical instants/citation order specified for amended outputs. |
| grok-3 (HIGH) | Nonpositive thresholds explicitly default to DefaultMinObservations. |
| grok-4 (MEDIUM) | Zero reconciliation counters live in required version 4 Evidence payload without omitempty. |
| grok-5 (MEDIUM) | ValidateComplete must check nil/aggregate state/source state and inconsistent quality flags/counters. |
| grok-6 (MEDIUM) | Canonical interval/citation order specified; no conflicting implemented equality/validation helpers. |
| grok-7 (MEDIUM) | ReadSources explicitly validates source IDs, selection, bounds/defaults and cancellation semantics. |
| grok-8 (LOW) | ErrInvalidPhase separates phase errors from outcomes; unimplemented SummarizeWall will enforce it. |

Validation: go build ./..., go vet ./..., and the full go test ./... -race
suite pass without exclusions after correction. The head-pinned cross-family
panel is required before releasing FC-SEALS. Baseline
code/test equivalence is checked independently from the historical panel results.

The legacy extraction defects remain visible until FC-JOURNAL/FC-SOURCES/FC-1
land, as the worklist requires. No budget, test exclusion, or reviewer rule is
relaxed by this correction.

### Corrected-head panel follow-up (a4bf3e8)

The 20 findings from the panel at 2026-09-05T03-55-18Z are addressed in this
follow-up; its BLOCK remains preserved as historical evidence.

| Finding | Disposition |
|---|---|
| claude-1, claude-2; grok-6 | Scalar nullable producer EventPayload, duration_ms, checked conversion and explicit wire seal; no Measured JSON workaround. |
| claude-3 | Evidence version advances from actual legacy 3 to 4; exact-equality gate. |
| claude-4; codex-5; grok-3 | Baseline readTargetTasks comment now states only ErrYAMLSource. |
| claude-5; codex-2; grok-2 | Matching Corrections value/evidence names and JSON tag. |
| claude-6 | Correction starts at finish minus recorded duration; missing/ambiguous duration means unavailable phase, not an invented boundary. |
| claude-7 | Every validator stub names its receiver. |
| claude-8 | Wire/boundary examples centralized here and cited from parser godoc; inline seam invariants retained where needed by callers. |
| codex-1 | Row position and revision instant in canonical ReadingRef; one recovered envelope per attempt, all further compatible envelopes duplicates. |
| codex-3 | Explicit original TargetRow slice at gate; target validation before aggregates. |
| codex-4 | ErrSourceEmpty consistently means zero journals only. |
| grok-1 | Single cutoff; revision/terminal exclusion, no JoinEvidence now parameter; replay seal. |
| grok-4 | Resolved positive manifest bounds and exported DefaultMaxCommits. |
| grok-5 | Legacy scanner restored to exact private literal baseline, independent of amended defaults. |
| grok-7 | Partial-on-error obligations frozen end-to-end, including Build result and CLI reporting. |

Validation and exact-head panel must be rerun for this follow-up before release.
