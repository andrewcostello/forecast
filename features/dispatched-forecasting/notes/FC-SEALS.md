# FC-SEALS: F1–F4 contract seals

Independent seals against the frozen FC-SCAFFOLD handoff. No implementation
edits, no `t.Skip`, no xfail, no `config/known-red.yaml` edits.

Operator correction author: Codex Sol. The dispatched task/model pin remains
Grok; this note records the substantive correction model separately and does
not amend the worklist.

## Groups (predeclared)

| Group | Body | File |
|---|---|---|
| `TestFCJournalContract` | FC-JOURNAL | `internal/dispatched/journal_contract_test.go` |
| `TestFCSourcesContract` | FC-SOURCES | `internal/dispatched/sources_contract_test.go` |
| `TestFCEvidenceContract` | FC-1 | `internal/dispatched/evidence_contract_test.go` |
| `TestFCReferenceCLIContract` | FC-1 | `cmd/forecast/dispatched_reference_cli_contract_test.go` |

Mismatch vs `config/known-red.yaml`: none. Those four names are used as
top-level tests; no additional failing top-level names were introduced.

## Fixture provenance

Recorded/sanitized:

- `internal/dispatched/testdata/journals/recorded-panel-iterate.jsonl`
- Producer `dispatcher_version=0.1.0`
- Orchestrator revision cited by the scaffold: `df771516b905355995d03313b470b06e1aea4e06`
- Event order taken from `dispatcher-runs/2026-09-02T16-21-45Z-convergence/journal.jsonl`
- Preserved: `task_spawn_finished(spawn_kind=panel-iterate)` then `panel_iterate`,
  uncached vs cache token split, `duration_ms`, invocation-shaped `panel_started`
- Sanitized: synthetic keys/hashes, rounded cost, no hostnames/prompts/credentials

All other `testdata/journals/synthetic-*.jsonl` and `testdata/yaml/*` files are
explicitly synthetic, including `testdata/yaml/holdout-held-and-keep.yaml`
(keep + held-out rows for live and git-history joins). Git history fixtures
are built in-process from those YAML files; no network and no real credentials.
Replace-ref fixtures are created with `git replace` against those in-process
repos (F3-HISTORY-FACTS).

Fixture Git author/committer dates come from an explicit monotonic clock per
fixture repository starting at `2025-12-01T00:00:00Z`; the clock does not read
reachable refs and therefore cannot move backward after branch deletion,
replacement, or graft setup. Every created commit is checked before
`contractCutoff` and strictly after every parent. Fixture Git commands discard
inherited `GIT_*` location/config state and use null system/global config. The
FC-SEALS and CLI fixture constructors initialize an explicit `main` branch;
unchanged baseline fixtures address only `HEAD` and have no default-branch-name
assumption. Copied/generated live files receive deterministic content-derived
mtimes in December 2025, with checkout/merge paths re-freezing worktree files
through the same helper.
Explicitly later journal/YAML timestamps and direct `ReadingRef` cases retain
their deliberate later values. The malformed-document fixture is
`testdata/yaml/malformed-document.yaml`; unused
stale-start/missing-run/conflicting-role/text fixtures and the unreferenced
`valid-tasks.yaml` fixture were removed.

CLI targets live under `cmd/forecast/testdata/dispatched/targets/`.

## Green vs red

Existing regression tests remain green, including baseline `Table.Add` join
(identity still omits run ID on that path) and `journal_facts` overlapping
timing. Two additive green amendments:

- `TestDispatchedReferenceBuildCommandIsRegistered` now lists `--timeout` and
  `--fail-on-uncovered-required` (FC-1 panel Grok-7).
- `TestCoverageStatesHandFinishedLimit` asserts `Build` itself populates
  `Limits` (FC-1 panel Codex-3). This remains green because the accepted
  baseline constructor already emits `HandFinishedLimit`; the regression gate
  confirms it is not a new FC-1 body dependency. Mutation: delete that value
  from the baseline artifact constructor.

New capability cases in the four reserved groups are red against
`ErrNotImplemented` seams until the named body lands.

## Deviations

None. Seals assert the accepted scaffold contract as written. Nonblocking
follow-ups that would change that contract are recorded below, not adopted.

## Inherited FC-SCAFFOLD findings pinned by these seals

| Finding | Pin |
|---|---|
| Terminal/elapsed not atomic; independent max | `F1-EV-MERGE-PERMUTATION`, `F1-EV-JOURNAL-OVER-YAML`, `F1-EV-NO-MANUFACTURED-ROW` |
| Non-task YAML treated as malformed | `F3-NON-TASK-DOCUMENT` |
| Scaffold authored `*_test.go` | Not present on this head; seals own the glob |
| `WallBreakdown` containment skipped on zero start | `F2-WALL-ABSENT-START` |
| Evidence ties not commutative | `F1-EV-PERMUTATION`, `F1-READING-TOTAL-ORDER` |
| `GIT_CONFIG*` family omitted from isolation | `F3-GIT-ENV-STRIPPED` |
| Nil manifest completeness panic | `F3-COMPLETE-CONSISTENCY` (`ValidateComplete` nil-safe) |
| Held-out readings stripped before join audit | `F3-HOLDOUT-EXCLUDED`, `F3-HOLDOUT-EXCLUDED-JOIN`, `F3-DISPOSITION-EVERY-SNAPSHOT` |
| Typed YAML decode collapses the document | `F3-READING-ENVELOPE`, `F3-SRC-MALFORMED-PARTIAL` |
| Distinct runs collapsed | `F1-ID-DISTINCT-RUNS` |
| Hand-finished only a formatter fixture | green Build assertion + `F4-HAND-FINISHED-LIMIT` |
| Empty journal discovery silent | `F3-SRC-ZERO-JOURNALS`, CLI `F3-SRC-ZERO-JOURNALS` |
| Usage spam on data errors | CLI `F4-DATA-ERROR-NO-USAGE` |

## Cases, expected failure, mutation

Expected failure today is the named assertion failing because the owning seam
returns `ErrNotImplemented` (or the baseline CLI/sentinel). A passing body that
drops the assertion's predicate fails the same test.

Mutation execution is intentionally split: the unchanged green regressions
have current-body evidence; the four owner groups are verified red against the
scaffold but their proposed green-path mutations cannot be executed until the
owners land. Those mutations are obligations for body adjudication, not claims
of proof against stubs. In particular, the fixed `ReadSources` seam exposes the
retained commit count but no injectable commit iterator, so F3-BOUND-COMMITS
cannot independently distinguish bounded collection from walk-then-trim without
pinning an arbitrary Git command. It now requires exactly three retained commits
and a bound diagnostic; the iterator-access mutation is explicitly deferred.
Byte pre-buffering is observable by the controlled-child harness once an owner
body reaches it because the child withholds EOF after byte 65. Process/source
probes record actual Git entry, output, ordering, overlap and cancellation with
absolute paths embedded in the wrapper, but the frozen seams expose no hook for
an attempted slot/source acquisition. Absence of an overlap marker is not proof
under arbitrary goroutine scheduling. Journal-source entry is not instrumented.
Full process-slot and all-source-kind fan-out proof is deferred to
body/adjudication code review and green-body mutation measurement; current
scaffold-red termination does not exercise these controlled-child paths.

### Journal (`TestFCJournalContract`)

| Case | Expected | Mutation that must fail |
|---|---|---|
| F1-ID-UTC-OFFSET | One UTC instant, no monotonic data | Treat `-08:00` and `Z` as different starts |
| F1-ID-AMBIGUOUS-START | No attempt; `Starts=2` | Pick nearest or first start |
| F1-JOURNAL-PRODUCER-RESOLVED | `0.1.0` from `run_started`; missing producer flagged | Trust `JournalIdentity.Producer` input |
| F1-EV-TOKENS-CITED-SEPARATELY | Output-only spawn does not cite input | Copy output events into input |
| F1-EV-PROVENANCE-KEPT | Every field has `FieldEvidence`; sums cite least event | Drop `Evidence.Cost` while keeping `CostEvents` |
| F1-EV-UNKNOWN-STAYS-UNKNOWN | Unfinished, `EvidenceNone`, elapsed to cutoff, censored | Invent YAML/journal terminal |
| F1-EV-MODEL-CONFLICT | Last implementing spawn wins; no cascade | Count two models as `ErrEvidenceConflict` |
| F1-EV-TERMINAL-CONFLICT | `task_done`+`task_blocked` excluded | Prefer done |
| F1-MODEL-CLOSING-STAMP | Model `sol`, Cascades=1 | Keep `opus` after fallback |
| F1-MODEL-ABSENT-STAMP | Model unknown | Substitute planned/YAML model |
| F1-HASH-PROVENANCE | First `task_started` and panel-iterate spawn retain fixture `hash`/`prev_hash` | Drop first-event hash/prev_hash; strip hash on EventRef |
| F1-CONFLICT-PORTABLE | Tagged JSON candidates with outcome | Store only citations |
| F1-ROW-EQUALITY-BY-CONTENT | Permuted events, same canonical JSON | Keep input order |
| F2-ELAPSED-TERMINAL | 10m, not censored | Use cutoff |
| F2-ELAPSED-CUTOFF | Censored lower bound to cutoff | Drop elapsed |
| F2-ELAPSED-BLOCKED-CENSORED | Blocked 8m censored | Treat blocked as duration |
| F2-ELAPSED-SURVIVES-UNKNOWN-PHASES | Elapsed known, intervals empty, Complete=false | Zero elapsed when phases missing |
| F2-PHASES-DISJOINT | Disjoint, sum ≤ elapsed, residual unclassified | Copy overlapping journal_facts windows |
| F2-PHASES-PANEL-WALL | One invocation interval; gate opens none | One interval per reviewer seat |
| F2-PHASES-ITERATE-AFTER-SPAWN | Development ends at spawn finish; `panel_iterate` is not a start | Use `panel_iterate` as start (legacy inverted order) |
| F2-PHASES-INFERRED-LABELED | Caller `Inferred=true` preserved; reducer emits false | Flip the flag |
| F2-ROUNDS-VS-REVIEWS | Reviews=2, Corrections=1 | Count first review as a correction |
| F2-COST-NULL-VS-ZERO | Unknown vs Known(0) | Coerce null to 0 |
| F2-COST-NO-DOUBLE-SUM | Retransmission summed once | Double-count seq |
| F2-MEASURE-NONFINITE | LinesUnparsed for negative/type errors | Let -1 cost reach samples |
| F2-MEASURE-REVERSED | `ErrReversedInterval` | Clamp to zero |
| F2-WALL-ABSENT-START | `ErrUnattributable` | Skip containment when start is zero |
| F2-UNCLASSIFIED-RESIDUAL-ONLY | `ErrInvalidPhase` on unclassified interval | Accept it as a span |
| F2-CANONICAL-ORDER | `ErrNonCanonicalEvidence` | Sort silently |
| F2-PARTIAL-SUM-UNKNOWN | Total unknown, citations retained | Report partial sum as complete |
| F2-SPAWN-WIRE | `duration_ms` 1250 → exact cited `[T-1.25s,T)` development span; zero vs null | Drop the reducer span or label it inferred |
| F2-CORRECTIVE-BOUNDARY | Overlap component withheld; elapsed survives | Keep overlapping panel+iterate spans |
| F2-PRODUCER-SHAPES | Unknown `panel_started` → LinesUnparsed | Guess a review start |
| F2-CORRECTION-KINDS | 6 counted kinds | Also count paired panel spawn finishes |
| F2-VERIFICATIONS | Count `verification_started` only | Count skipped/mechanical |
| F2-EVENT-IDENTITY | Retransmission once; colliding seq discarded | Merge collisions |
| F2-ALL-SPAWN-COST | All kinds including verifier/design; recorded cache-heavy fixture still totals 242 uncached input tokens | Add cache creation/read tokens; leave tokens unknown |
| F2-ARITHMETIC-ERRORS | Wire duration at max-ms+1 is `LinesUnparsed` with no whole-file error; finite cost-sum overflow is reducer `ErrMeasurementOverflow` | Return parser overflow error; saturate reducer cost |
| F2-LATER-START | StartsAfterCutoff=1, no attempt | Negative elapsed |
| F2-INITIAL-SPAWN | `[T1-D,T1)`, residual includes setup | Use `task_started` as development start |
| F2-ALL-KINDS | Closing stamp from implementing kinds; design does not stamp | Stamp designer |
| F2-CITATION-MEMBERSHIP | Evidence cites list[0]; lists complete | Evidence cites a leftover ref |
| F2-SUMMARY-AVAILABILITY | Same numbers, different Complete | Drop Complete |
| F4-OUTCOME-WIRE | `done/blocked/unfinished`; invalid → `ErrInvalidOutcome` | Decode missing outcome as unfinished |
| F2-DESIGN-SPAWN | Cost included; exact design `[00:01,00:02)` remains unclassified residual | Classify the design window into any phase |
| F2-PRODUCER-DECLARATIONS | Conflict clears Producer | Pick last version |
| F2-LINE-ENVELOPE | HasSeq from presence; explicit 0 distinct from null | Treat missing seq as 0 present |
| F2-PARSER-TOTAL-CAP | TotalBoundExceeded, retained data | Unbounded parse |
| F2-WALL-PARENT-CONSISTENCY | `ErrEvidenceConflict` on wall/parent mismatch | Serialize mismatch |
| F3-CANCEL-PERSIST-PARSE | `ErrSourceCancelled` and `context.Canceled`, diagnostics kept | Drop parsed lines |
| F3-REDUCE-ZERO-CUTOFF | `ErrInvalidSelection` | Read a clock |

### Sources (`TestFCSourcesContract`)

| Case | Expected | Mutation |
|---|---|---|
| F3-SRC-EXPLICIT-ONLY | `ErrInvalidSourceSpec`; no HOME default | Scan `$HOME/Project/claude-workflow` |
| F3-SRC-ROOT-OUTSIDE-FEATURES | Single `dispatcher/` root: scanned, `features/` and `unrelated/` not. Two declared roots `dispatcher`+`features` (live and history): both scanned, undeclared `unrelated/` not | Walk the whole tree; scan only the first root |
| F3-SRC-ROOT-ESCAPES | Reject `../`, `/abs`, symlink escape | Follow the symlink |
| F3-SRC-MISSING | `ErrSourceMissing` | Return EMPTY |
| F3-SRC-ZERO-JOURNALS | `ErrSourceEmpty`; AllowEmpty → EMPTY, not eligible | Succeed with zero journals |
| F3-SRC-MALFORMED-PARTIAL | Valid sibling kept; row and document errors retained; PARTIAL; malformed count includes both | Abort the document or skip malformed-document diagnostics |
| F3-READING-ENVELOPE | Four independent task envelopes plus row-zero `DocumentMalformed` envelope | Collapse siblings or classify invalid YAML as non-task |
| F3-NON-TASK-DOCUMENT | `DocumentNotTasks`, not Malformed/PARTIAL | Count as malformed |
| F3-SRC-RESOLVED-REF | SHA in ResolvedRef | Leave branch name |
| F3-GIT-ENV-STRIPPED | Drop `GIT_DIR`/`GIT_CONFIG*` family; pin `/dev/null` | Honour `GIT_CONFIG_COUNT` |
| F3-GIT-INSTALLATION-ENV | Keep `GIT_EXEC_PATH` and PATH | Strip exec-path too |
| F3-GIT-SHALLOW | Shallow+PARTIAL; ValidateComplete wraps both incompleteness sentinels | Report COMPLETE |
| F3-GIT-FULL-HISTORY | Reading revision exactly equals the superseded side-parent SHA | First-parent only |
| F3-GIT-DELETED-RENAMED | Old path still read | Skip deleted blobs |
| F3-BOUND-COMMITS | Exactly three retained commits; bound/PARTIAL diagnostics | Retained over-cap commit fails now; iterator walk-then-trim mutation deferred until the body exists |
| F3-BOUND-BYTES | PARTIAL plus controlled blob reader rejects unique byte 65 before EOF | Buffer the whole blob before applying the cap |
| F3-BOUND-PROCESSES | Negative invalid; MaxProcesses=1 preserves COMPLETE/counts; after the second call launches, first release stays absent during a bounded positive-presence observation; first entry and both controlled outputs are observed, and any actual pre-release second entry/overlap is rejected | Eager overlap is caught; marker absence is not proof of attempted acquisition/serialization, which remains deferred to body review/mutation |
| F3-CANCELLED | Both cancel sentinels; not COMPLETE | Ignore ctx |
| F3-HOLDOUT-EXCLUDED | Held-out journal in ExcludedJournals; live AND git-history YAML rows with `dispatcher_run_id: held` marked HeldOut with snapshot cleared; keep rows remain in-sample | Feed held-out events to reducer; ignore history YAML |
| F3-CUTOFF-EXCLUDED | AfterCutoff audit envelope | Sample later completed_at |
| F3-SELECTION-INVALID | Padded/duplicate/zero cutoff rejected before IO | Trim then match |
| F3-HOLDOUT-PADDED-STILL-EXCLUDES | `ErrInvalidSelection` | Match after trim |
| F3-HOLDOUT-UNMATCHED | `ErrInvalidSelection` | Ignore unknown IDs |
| F3-MISSING-REVISION-TIME | Direct zero-RecordedAt envelope: Malformed=1, AfterCutoff=0, no recovery | Treat zero as AfterCutoff |
| F3-COMPLETE-CONSISTENCY | Nil receiver does not panic; COMPLETE+shallow/grafted/replaced each wrap ErrShallowHistory+ErrSourceIncomplete | Panic on nil; ignore Replaced |
| F3-DEFAULT-BOUNDS | Zeros become stored defaults; negatives invalid | Persist zeros |
| F3-RESOLVED-BOUNDS | Positive DefaultMaxCommits stored | Re-apply future defaults |
| F3-REF-IDENTITY | All-refs: empty ResolvedRef, full list | Put HEAD in ResolvedRef |
| F3-REVISION-CANONICAL | live / git:full lower SHA only | Accept abbreviated SHA |
| F3-UNSUPPORTED-REF | Ref on live/journal → invalid spec | Ignore Ref |
| F3-AMENDED-GIT-HELPER | Poisoned GIT_DIR does not redirect | Call legacy gitLines |
| F3-GIT-RUNNER | Nil budget invalid; `git status` invalid; cancel wraps | Shell out unbounded |
| F3-HISTORY-FACTS | Grafted-only: Grafted, not Replaced, ErrShallowHistory+ErrSourceIncomplete. Replaced-only: `git replace` ref present, Replaced, not Grafted, PARTIAL, same sentinels | Hide grafts/replace by disabling replace objects only; set one flag for both |
| F3-EXCLUDED-QUALITY | Held-out malformed uses MalformedExcluded | Degrade in-sample completeness |
| F3-MALFORMED-HELDOUT-IDENTITY | Independent RunID still proves holdout | Require valid Snapshot |
| F3-ALL-JOURNALS-HELDOUT | COMPLETE, Journals=1, excluded=1 | Mark EMPTY |
| F3-SOURCE-CONCURRENCY | After the first marker, its release stays absent during a bounded positive-presence observation for second-source entry, overlap, or second-first ordering; any observed violation fails, then reports remain SourceID-ordered with unchanged bound counts | Eager cross-Git fan-out/order is caught; marker absence is not proof of acquisition, and journal-kind fan-out remains deferred |
| F3-DUPLICATE-JOURNAL-RUN | `ErrDuplicateJournalRun` | Merge replicas |
| F3-COMPLETENESS-CAUSES | Bound PARTIAL wraps both incompleteness sentinels | COMPLETE with BoundsExceeded>0 |
| F3-EXCLUSION-ORDER | NotTaskDocument first | Classify as malformed |
| F4-MANIFEST-EMPTY-LISTS | `[]` not null | Omit empty keys |
| F3-CITATION-ROW | Distinct Ref.Row | Row=0 for every task |
| F3-OPEN-SOURCE-NIL-BUDGET | `ErrInvalidSourceSpec` | Panic / unbounded read |

### Evidence / CLI (`TestFCEvidenceContract`, `TestFCReferenceCLIContract`)

| Case | Expected | Mutation |
|---|---|---|
| F1-ID-DISTINCT-RUNS | Attempts=2, UniqueRows=2 | Dedupe by key+time |
| F1-ID-SAME-RUN-REVISIONS | 1 recovered + 2 duplicate readings | One sample per commit |
| F1-ID-NEAREST-NOT-MATCHED | NoMatchingStart; lost attempt listed | Match 1s offset |
| F1-ID-UTC-OFFSET-YAML | Offset YAML joins UTC journal start | Require identical strings |
| F1-EV-MERGE-PERMUTATION | Two YAML citation orders yield byte-identical complete join; journal 10m terminal and cost retain event citations | Elapsed 12m with journal label; first-reading tie |
| F1-EV-NO-MANUFACTURED-ROW | YAML has no cost carrier: terminal/elapsed and cost remain separate journal event-cited fields while both readings are retained | Attribute cost to a YAML `ReadingRef`; take YAML 12m terminal |
| F1-EV-JOURNAL-OVER-YAML | Journal done beats YAML blocked | Conflict or YAML wins |
| F1-EV-YAML-ONLY-TERMINAL | YAML source; counter=1 | Leave unfinished |
| F1-EV-PERMUTATION | Canonical JSON across set order | First-write-wins |
| F1-MODEL-NO-ALIAS-POOL | Two cells | Pool opus aliases |
| F1-ROLE-CITATION | Both conflicting rows are audited/cited; zero recovery/observations; `ErrEvidenceConflict` | Pick first role or omit either conflict disposition |
| F1-READING-TOTAL-ORDER | Examined canonical | Input order |
| F1-ATTEMPT-RUN-CONSISTENCY | `ErrInvalidSelection` | Join anyway |
| F3-SRC-READ-OK-ZERO-MATCH | Recovered=0, `DispositionNoMatchingRun`; distinct from zero-journal EMPTY | Invent a sample; treat as `F3-SRC-ZERO-JOURNALS` |
| F3-DISPOSITION-* | Every snapshot; no-run; missing keys | Drop unmatched YAML |
| F3-ROWS-VS-ATTEMPTS | UniqueRows=1 Attempts=2 | Collapse restarts |
| F3-LOST-NOT-HIDDEN | Lost sibling listed | Count recovered as covering it |
| F3-MANIFEST-CUTOFF-STORED | `SourceManifest.Cutoff` set; per-source counts present | Omit cutoff; leave zero |
| F3-DIRECT-HOLDOUT* | Unmatched/held-out attempt sets refused | Produce observations |
| F3-EXCLUDED-JOURNAL-AUDIT | Full identity survives | Reduce to run-id strings |
| F3-HOLDOUT-EXCLUDED-JOIN | HeldOut envelope audited, keep recovered, held snapshot unused | Feed held-out snapshot into join |
| F2-YAML-TERMINAL-WALL | Rebase elapsed; withhold outside spans | Clip intervals |
| F4-TARGET-* | Empty → ErrEmptyTarget; malformed → ErrInvalidTarget | Gate on aggregates |
| F4-TARGET-INPUT | Duplicate original keys → ErrInvalidTarget despite valid aggregate | Gate on aggregates |
| F4-ELIGIBLE-THRESHOLD | Eligible at n=2 | Eligible at n=1 |
| F4-NOT-ELIGIBLE-THIN | ErrNotEligible only | Also wrap incomplete |
| F4-NOT-ELIGIBLE-PARTIAL | Both sentinels when refuse | Eligible diagnostic |
| F4-HAND-FINISHED-LIMIT | Limits from amended Build; report prints it | Formatter-only |
| F4-BUILD-AMENDED-OPTIONS | No silent v3 legacy path | Ignore Sources/holdout |
| F4-THRESHOLD-NONPOSITIVE | DefaultMinObservations | Keep 0 |
| F4-SCHEMA-ROUNDTRIP | Version 4 round-trip retains counts and mandatory observations key/array | Drop Reviews/Corrections; emit null or omit observations |
| F4-VERSION-EXACT | v3/v5/missing Evidence refused | `>=` comparison |
| F4-MIXED-OPTIONS | ErrInvalidSourceSpec | Ignore one side |
| F4-CANONICAL-LISTS | Every mandated empty v4 list key exists as `[]` | Emit null or omit keys |
| F4-ONE-ARTIFACT-INSTANT | GeneratedAt == manifest.Cutoff == explicit cutoff, not opts.Now | Use opts.Now |
| F4-AGGREGATE-REASON | Successful reads followed by real reducer and join errors yield retained PARTIAL results with sorted unique `reduce:`/`join:` reasons surviving JSON; the induced named prefix must be present while other legitimate aggregate diagnostics are allowed | Emit null/omitted reasons; omit the induced reducer/join diagnostic |
| F4-PROJECTION-MAPPING | Successful Build projects the recovered all-kinds attempt as rounds=6, outcome=done, terminal_evidence=journal | Map done→none; rounds from Reviews |
| F4-ARTIFACT-HOLDOUT | Held-out run in artifact refused | Predict anyway |
| F4-ARTIFACT-CUTOFF | Attempt/manifest cutoff mismatch refuses with both eligibility/incomplete sentinels | Ignore replay cutoff mismatch |
| F4-ARTIFACT-CELL | Structured Cell model contradicting stamped Attempt model refuses with both sentinels | Trust the contradictory cell |
| F4-ARTIFACT-JOINT-RECORD | Wall/parent elapsed mismatch refuses with both eligibility/incomplete sentinels | Accept `Attempt.Elapsed` and ignore contradictory `Wall.Elapsed` |
| F4-STRUCTURED-THIN-CELL | Cells name completed/threshold | Reasons only |
| F4-CELL-EMPTY-N0 | Empty required cell present n=0 | Omit it |
| CLI F3-SRC-EXPLICIT-ONLY | No HOME default; no usage | Default `~/Project/claude-workflow` |
| CLI F4-MISSING-FLAGS | Missing `--runs-dir` / `--out` fails | Succeed with empty argv; default `--out` |
| CLI empty/malformed targets | ErrEmptyTarget / ErrInvalidTarget, no usage | Legacy ErrYAMLSource + usage |
| CLI empty corpus | ErrSourceEmpty, no usage | Succeed with zero journals |
| CLI partial refusal | NotEligible/Incomplete, no usage | Coverage gate on PARTIAL |
| CLI data error | No Usage: banner | Cobra default usage |

## scaffold_review_followups

Nonblocking comments from `/home/andrew/Project/dispatcher-runs/2026-09-05T03-44-21Z-FC-SCAFFOLD-correction/nonblocking-review-followups.json`.
They do not amend the accepted contract.

| ID | Owners | Disposition |
|---|---|---|
| claude-1 | FC-SOURCES | **Not adopted.** Accepted F3-GIT-INSTALLATION-ENV keeps inherited `GIT_EXEC_PATH`. Pin: `F3-GIT-INSTALLATION-ENV`. Deriving exec-path from the binary needs a contract amendment. |
| claude-2 | FC-JOURNAL, FC-1 | **Not adopted.** Accepted F2-PHASES-INFERRED-LABELED preserves caller `Inferred=true`. Pin: that row. Requiring EventRef or excluding inferred spans from projections is a new named row. |
| claude-3 | FC-SOURCES | **Not adopted.** Accepted F3-SRC-ROOT-ESCAPES is Validate/ReadSources before read. Pin: `F3-SRC-ROOT-ESCAPES` (includes symlink). `O_NOFOLLOW` on `openSourceFile` is a contract amendment. |
| claude-4 | FC-SOURCES | **Recorded, documentation/stub.** Bodies must not emit illegal empty-state manifests. Pin: `F3-COMPLETE-CONSISTENCY`, `F3-DEFAULT-BOUNDS`. |
| claude-5 | FC-SOURCES | **Documentation only.** Godoc hedge/`ctx.Err.` is not a behavioral row. No silent reinterpretation. |
| claude-6 | FC-JOURNAL, FC-1 | **Documentation only.** Diagnostics combination rule stays in the handoff. |
| claude-7 | FC-1, FC-SOURCES | **Pinned mapping, not the extra Limits sentence.** `F4-PROJECTION-MAPPING` / schema-4 `rounds` ← `Attempt.Corrections`. Disclosing the rename in Limits is a follow-up, not a change to the mapping. |
| grok-1 | FC-SOURCES | **Not adopted.** Accepted isolation list does not install `GIT_PAGER=cat` or `safe.directory`. Pin: `F3-GIT-ENV-STRIPPED` as written. |
| grok-2 | FC-SOURCES | **Documentation only.** Same `ctx.Err()` typo as claude-5. |

## FC-SEALS panel finding dispositions

Family/index values refer to the blocked panel at
`2026-09-05T16-15-07Z-FC-SEALS-corrected-panel`. Repeated reviewers are grouped
only where the same correction answers each finding; every finding is named.

| Finding(s) | Disposition and concrete evidence |
|---|---|
| `claude/1`, `codex/8`, `grok/12` | Corrected F4-AGGREGATE-REASON with one valid-read reducer reversal and one valid-read role join conflict. Both require retained nonnil results, COMPLETE individual source reports, PARTIAL aggregate state, sorted unique structured reasons including the induced named prefix, and a present JSON array; other legitimate reduce/join reasons are allowed. Projection requires one recovered flat row exactly. |
| `claude/2`, `codex/11`, `grok/8` | Corrected F2-DESIGN-SPAWN by asserting the exact recorded design window `[00:01,00:02)` intersects no classified interval, remains at least one minute of unclassified residual, and its cost remains included. No nonexistent spawn kind is inferred from `Journal.Path`. |
| `claude/3`, `grok/9` | Input tokens are unconditionally Known(15) for all-kinds and Known(242) for the recorded cache-heavy fixture. Cost 1.10 uses tolerance, not raw decimal equality. |
| `claude/4`, `codex/4`, `grok/10` | F1-EV-MERGE-PERMUTATION now uses two distinct YAML citations in both orders and compares the complete join JSON. F1-EV-NO-MANUFACTURED-ROW reflects the real carrier: no YAML cost field; journal terminal/elapsed/cost each retain their event citation and both YAML readings remain listed. |
| `claude/5`, `grok/7` | Superseded in measurement strength by the follow-up panel below. The probes retain actual Git entry/order/overlap observations and SourceID-ordered reports, but no longer claim that marker absence proves a waiting acquisition or all-kind serialization. |
| `claude/6`, `codex/13`, `grok/5` | F3-MISSING-REVISION-TIME feeds the zero-time parse envelope directly to JoinEvidence: Malformed=1, AfterCutoff=0, no observation, and the zero citation remains in Examined. |
| `claude/7` | Added F4-ARTIFACT-CUTOFF and F4-ARTIFACT-CELL; each requires an ineligible result plus both ErrNotEligible and ErrSourceIncomplete. |
| `claude/8` | Added malformed-document parse and discovery assertions (`DocumentMalformed`, row 0, Err, PARTIAL/count), renamed the fixture to discoverable `.yaml`, and removed three unused YAML fixtures. |
| `claude/9`, `codex/14`, `grok/13` | Corrected PROVENANCE.txt to the real `synthetic-` convention and sole recorded fixture; removed the other dead fixtures noted by Grok. |
| `claude/10` | Terminal conflict now unconditionally requires its in-process ErrEvidenceConflict sentinel. |
| `claude/11` | Removed the unreachable disposition loop and require every disposition in exact `Dispositions()` order, including zero counts. |
| `claude/12` | Removed unused `ptr`, `testdataPath`, `copyTestdata`, and `usageSpam` helpers. |
| `codex/1` | Strengthened after follow-up: fixture commits use explicit per-repository monotonic clocks rather than reachability counts; setup Git environments strip inherited location/config variables, owned repos select `main`, and every commit is checked before cutoff and strictly later than each parent. Content-derived live mtimes remain frozen before cutoff and checkout/merge paths re-freeze files. Deliberately later payload/citation times are untouched. |
| `codex/2`, `grok/2` | Symlink escape must return exactly one of the two accepted sentinels (ErrInvalidSourceSpec or ErrSourceMissing). Neither `features/link.yaml` nor the unique external task key may appear in readings. No third sentinel was added. |
| `codex/3`, `grok/4` | Full-history proof now requires `Reading.Ref.Revision == "git:" + sideParentSHA`; merge-tree content and abbreviated SHA cannot satisfy it. |
| `codex/5` | Role conflict independently requires zero recovery, zero observations, two conflict dispositions, two Examined citations, and both portable conflict-side row citations. |
| `codex/6` | CLI missing-source case uses a valid journal corpus and a valid HOME-default Git repository with a distinctive decoy row; it requires a pre-processing explicit-source error, no artifact, no decoy-path disclosure, and no usage spam. |
| `codex/7` | F4 schema/canonical-list assertions decode `map[string]json.RawMessage`; omission, null, non-array, and wrong cardinality all fail. Manifest list seals use the same helper. |
| `codex/9`, `grok/3` | F2-SPAWN-WIRE unconditionally requires the exact 1.25-second development boundaries, recorded citation, and `Inferred=false`; Wall.Complete cannot mask a dropped span. |
| `codex/10` | Parser duration uses max representable milliseconds + 1 and must become exactly one LinesUnparsed diagnostic with no spawn/interval and no whole-file error. A separate finite MaxFloat cost-sum fixture requires reducer ErrMeasurementOverflow and forbids saturation. |
| `codex/12` | Byte cap uses a controlled child that writes unique byte 65 and withholds EOF; bounded read must fail before release without retaining that byte. Commit retention is exactly capped, but pre-collection iterator access is not observable through the frozen seam. Process/source markers now prove only events they positively observe; attempted acquisition and full fan-out mutations are likewise deferred rather than falsely certified. |
| `grok/1` | Proposed revert not adopted after checking evidence: accepted baseline Build already initializes `Limits` with HandFinishedLimit, and the excluded-group regression gate remains green. The assertion is a useful green regression, not an unnamed FC-1 red. |
| `grok/6` | Shallow fixture keeps file-URL depth semantics but opts into `protocol.file.allow=always` only in the test helper; production isolation remains `never`. |
| `grok/11` | F3-MANIFEST-CUTOFF-STORED, F4-HAND-FINISHED-LIMIT, F4-BUILD-AMENDED-OPTIONS, and F4-ONE-ARTIFACT-INSTANT now use nonempty valid journal corpora. Only F4-CANONICAL-LISTS deliberately uses AllowEmpty. |

## Follow-up panel finding dispositions

Family/index values refer to the latest attached cross-family panel (consensus
APPROVE; Claude and Grok approved, Codex dissented with two uncorroborated high
findings). Every current family/index disposition is recorded here.

| Finding | Disposition and concrete evidence |
|---|---|
| `claude/verdict` | APPROVE with no indexed findings; no separate code change required. |
| `grok/verdict` | APPROVE with no indexed findings; no separate code change required. |
| `codex/1` | **Corrected and narrowed.** F3-BOUND-PROCESSES keeps first release absent for a bounded positive-presence observation after the second launch. Wrapper paths and the absolute real-Git path are shell-quoted, waits/cleanup are bounded, and actual pre-release child entry/overlap fails. Absence does not prove attempted slot acquisition or serialization; that remains deferred. |
| `codex/2` | **Corrected and narrowed.** F3-SOURCE-CONCURRENCY keeps first release absent for a bounded positive-presence observation after first entry and rejects observed second entry, overlap, or second-first ordering. Reports remain canonically SourceID-ordered. Absence does not prove acquisition, and the uninstrumented journal-kind fan-out remains deferred. Cleanup/results stay bounded. |
| `panel/request-4` | Fixture reachability clocks were replaced by explicit per-repo clocks, fixture Git environments are isolated, and commit parent ordering is asserted in both Go test packages. The tiny helpers remain duplicated because separate `dispatched` and `main` test packages cannot share unexported test code without a production/exported seam; no such scope expansion is justified. Explicit initial branches remove host default-branch dependence. |
| `panel/request-5` | Added F4-ARTIFACT-JOINT-RECORD for a wall/parent elapsed mismatch, requiring both ErrNotEligible and ErrSourceIncomplete. Aggregate-reason checks require the induced named prefix but permit other valid `reduce:`/`join:` diagnostics. |
| `panel/request-6` | PROVENANCE now distinguishes the sole recorded/sanitized fixture from wholly synthetic companions. `valid-tasks.yaml` was deleted after repository-wide reference search found no consumer; duplicate in-package mtime calculation was folded into `setFixtureMTime`. |

## Latest panel correction dispositions

| Finding | Disposition |
|---|---|
| Bounded process/source probes | Restored a `fixtureHarnessWait` positive-presence observation before first release in both probes. Positive markers fail and trigger bounded release/cleanup; marker absence is not treated as negative proof. Attempted acquisition, serialization adjudication, and journal-kind fan-out remain deferred. |
| Controlled shell portability/bytes | All controlled release loops use portable `sleep 1`. The byte probe writes exactly 64 `a` bytes plus `Z` with `printf '%s'`, without a newline, and remains alive until release. |
| Fixture clock wording | Narrowed the helper godoc: commit-producing `runGit` calls advance the explicit per-repository clock; read-only helpers using the isolated environment do not. The clock implementation is unchanged. |
| Claude malformed-record HIGH | **Not adopted; false positive.** `PredictionEligibility` expressly treats malformed joint records as invalid payloads and, when refusing, requires `ErrNotEligible` plus `ErrSourceIncomplete` (`internal/dispatched/build.go:149-167`, especially 154-166). `ErrEvidenceConflict` applies at the distinct `ReduceAttempts`/Attempt JSON/`JoinEvidence` entry points. The prior panel at `2026-09-05T17-05-25Z` requested this exact negative case and pair. Mutation: accept `Attempt.Elapsed` and ignore contradictory `Wall.Elapsed`. |

## Residual limitation

New groups are red until FC-JOURNAL / FC-SOURCES / FC-1 replace `ErrNotImplemented`.
Baseline extraction defects remain visible on the legacy path, as required.
The frozen seams cannot reveal that a competing goroutine has reached a process
slot or source-acquisition wait, and they expose no journal-source entry event.
Consequently F3-BOUND-PROCESSES and F3-SOURCE-CONCURRENCY reject positively
observed overlap/order violations but do not mathematically prove serialization
under arbitrary scheduling. Full acquisition/all-source-kind fan-out validation
is a body/adjudication code-review and mutation-measurement obligation.

## Correction validation

On the corrected tree, `gofmt` and `git diff --check` are clean; `go build
./...` and `go vet ./...` pass. The unaffected regression command `go test
./... -race -skip
'^(TestFCJournalContract|TestFCSourcesContract|TestFCEvidenceContract|TestFCReferenceCLIContract)$'`
passes. Each of the four reserved groups exits 1 under `-race` on named
scaffold/baseline assertions inside a 90-second external bound; all completed
far below the bound with no panic, fatal runtime error, or data race.
`F4-ARTIFACT-JOINT-RECORD` is among those named scaffold failures. These
no-hang runs stop at `ErrNotImplemented` and do not enter or empirically exercise
the controlled-child harness paths; their termination is not evidence that
those paths ran. Body/adjudication mutation proof remains deferred. The parent
operator's IP-socket-denied check remains a separate independent verification
step.

## Operator ownership ruling: missing revision time

FC-SOURCES implementation `ab03f0a` exposed an accidental forward dependency:
F3-MISSING-REVISION-TIME parsed a reading and then called FC-1-owned JoinEvidence.
The source group now retains the original parser envelope assertions. The full
original setup and join assertions also run as F3-MISSING-REVISION-TIME-JOIN under
TestFCEvidenceContract (FC-1 owner). No assertion, fixture, or contract was removed
or weakened; no new known-red exclusion was added. The join case must remain red
until FC-1 lands and must reject a mutation that samples zero RecordedAt evidence.
This is an explicit operator seal-ownership amendment, not a body-authored change.

## FC-SOURCES corrective seals

Independent seals for frozen-contract defects from
`2026-09-06T01-18-41Z-FC-SOURCES-corrected-panel`. Existing cases, fixtures,
the operator split, and FC-1 ownership are unchanged. New cases register inside
`TestFCSourcesContract` only (`sources_panel_contract_test.go`). Bodies cannot
edit these seals.

### Panel dispositions

| Finding | Disposition |
|---|---|
| Codex raw journal cutoff | **False positive.** `SourceReadings` godoc and F3-CUTOFF-EXCLUDED assign post-cutoff journal event filtering to ReduceAttempts. Do not filter raw `ParsedJournal.Events` in ReadSources. Existing raw parse and reducer cutoff cases stay. |
| Live `os.Stat` vs `openSourceFile`, path walk vs `os.Root` | Body must tie discovery/mtime to the confined descriptor. Frozen seams cannot expose that race deterministically; **body code-review obligation**, no sleep-based TOCTOU test. |
| Shared `MaxTotalBytes` check-then-charge | `F3-BOUND-TOTAL-CONCURRENT`. Cap is inclusive; one overflow probe byte may be consumed and must not be retained. Readiness-before-reservation means the frozen seam cannot prove both readers acquired allowance; the old-body `bytesRead=130` failure against cap 64 is positive mutation evidence, not a negative serialization proof. No new public acquisition hook. |
| `Close` discards finish | `F3-GIT-CLOSE-ERRORS`. Close without Read surfaces cancel/bound/nonzero-child errors. EOF then Close is nil. Close after Read already delivered an error stays nil so existing `closeFixtureReader` assertions remain. |
| `ValidateComplete` refs/reasons | `F3-COMPLETE-RESOLUTION` counterfactual manifests: blank/padded/malformed/duplicate all-ref entries, Git-invalid all-ref names (`a..b`, `.lock`, control chars, outside `refs/`), resolution metadata on live/journal, COMPLETE with incomplete reasons. Sentinel `ErrSourceIncomplete`. All-ref `HEAD` is the allowed pseudoref; explicit requested `HEAD` stays separately valid. |
| Empty history COMPLETE vs validator | `F3-EMPTY-HISTORY-CONSISTENT`. No resolved commits ⇒ PARTIAL with a diagnostic, canonical empty lists, `ValidateComplete` `ErrSourceIncomplete`. Do not invent a new returned sentinel. |
| Unbounded `rev-list` | `F3-BOUND-COMMITS-LIMITER` requires provider-side `MaxCommits+1` (`--max-count=4` or `-n4`) on the metadata traversal command. Retains exactly-three-plus-bound semantics. `F3-GIT-FULL-HISTORY` keeps superseded merge-parent coverage. Not a process-count or timing threshold. Eager topo laziness and argv-vs-ref-count bounds are a **body code-review obligation** unless a compact deterministic seam can prove them. |
| N+1 show/ls-tree batching | **Advisory optimization**, not a new total-process-count contract. No acceptance threshold. |
| Linked-worktree grafts | `F3-GIT-WORKTREE-GRAFTS` (fixture worktree). `F3-GIT-INSTALLATION-ENV` unchanged. |
| AllowEmpty skips finalize | `F3-ALLOW-EMPTY-REASONS`: EMPTY keeps canonical lists and copies partial source reasons. |
| Non-mapping YAML as malformed | `F3-NON-TASK-SHAPES`. Empty/list/scalar without `tasks` is `DocumentNotTasks`. Invalid syntax stays malformed. `tasks: []` still yields no row envelopes. |
| Structural identity/time | `F3-IDENTITY-STRUCTURE`. Sequence/map/alias identity or time nodes carry `Err`, stay auditable, and degrade completeness when exclusion cannot be proved. Genuine absent fields stay absent. |
| Zero RecordedAt ingest unsealed | `F3-MISSING-REVISION-TIME-INGEST` uses `ingestReadings` at the frozen source boundary. Join remains `F3-MISSING-REVISION-TIME-JOIN` under Evidence. |
| Write-capable metadata args | `F3-GIT-REQUEST-READONLY` rejects `--output` and external-helper options; current read-only forms including `--git-common-dir` and `rev-list --max-count` remain valid. |
| Unsupported refs | No change. `F3-UNSUPPORTED-REF` stands. |

### Corrective cases, expected failure, mutation

| Case | Expected | Mutation that must fail |
|---|---|---|
| F3-GIT-CLOSE-ERRORS | Close after successful EOF is nil. Close without Read on nonzero exit wraps `ErrGitHistory`; on cancel wraps `ErrSourceCancelled` and `context.Canceled`; after stderr charges the total cap wraps `ErrBoundExceeded`. Controlled children use `sleep 1`, bounded cleanup, minimal env, no network. | `Close` always nil; EOF Close returns `io.EOF`; kill-without-wait hides exit |
| F3-BOUND-TOTAL-CONCURRENT | Two concurrent `runSourceGit` streams against one budget: physical `bytesRead() <= MaxTotalBytes+1`, retained payload `<= MaxTotalBytes`, overflow `Z` not retained, at least one `ErrBoundExceeded`. Readiness-before-reservation cannot prove both readers hold allowance; old-body `bytesRead=130` remains the mutation evidence. | Check remaining then read without reserving; allow two full collections (`bytesRead=130` vs cap 64) |
| F3-COMPLETE-RESOLUTION | Otherwise-valid COMPLETE fixtures with blank/padded/malformed/duplicate all-ref entries, Git-invalid all-ref names (`refs/heads/a..b`, `refs/heads/main.lock`, control chars, names outside `refs/`), live/journal resolution metadata, or COMPLETE reasons wrap `ErrSourceIncomplete`. Control fixture stays complete. All-ref `HEAD` plus `refs/heads/main` is accepted; explicit requested `HEAD` is accepted. | Accept invalid all-ref names; reject `HEAD` as the all-ref pseudoref; reject explicit requested `HEAD`; panic; wrong sentinel |
| F3-EMPTY-HISTORY-CONSISTENT | Unborn all-refs history is PARTIAL with a diagnostic reason, canonical empty `resolved_refs`/`reasons`/`roots` lists, and `ValidateComplete` `ErrSourceIncomplete`. Returned diagnostic error semantics stay as already accepted; no new sentinel. | Return COMPLETE that `ValidateComplete` rejects; invent refs/commits; omit the reason; emit null lists; guess `ErrSourceEmpty` as the only legal outcome |
| F3-BOUND-COMMITS-LIMITER | History `rev-list` includes `MaxCommits+1`. Five-commit fixture still retains 3 and counts a bound. | Unlimited `rev-list` plus kill; `--max-count=MaxCommits` with no overflow line; drop merge-parent coverage (`F3-GIT-FULL-HISTORY`) |
| F3-GIT-WORKTREE-GRAFTS | Fixture linked worktree with grafts in the common dir reports Grafted, not Replaced, PARTIAL, `ErrShallowHistory+ErrSourceIncomplete`. Installation env policy unchanged. | Stat only `--absolute-git-dir`; treat graft as replace; strip `GIT_EXEC_PATH` |
| F3-ALLOW-EMPTY-REASONS | AllowEmpty + zero journals + grafted history: `EMPTY`, non-empty aggregate reasons, JSON `[]` lists not null, `ValidateComplete` still `ErrSourceIncomplete`. | Skip reason copy; emit null lists; mark eligible |
| F3-NON-TASK-SHAPES | Empty, `[]`, scalar, `null` YAML → `DocumentNotTasks`, not malformed/PARTIAL. Invalid syntax → `DocumentMalformed`. Empty `tasks: []` stays zero row envelopes. | Classify non-mapping success as malformed; treat syntax errors as not-tasks |
| F3-IDENTITY-STRUCTURE | Sequence/map/alias identity or time fields set `Err`, remain listed, cannot prove holdout, increment Malformed/PARTIAL. Missing `started_at` stays absent without that error. | Treat structural nodes as absent; hold out on non-scalar run ID; error on genuine absence |
| F3-MISSING-REVISION-TIME-INGEST | Zero-`RecordedAt` parse envelope ingested as in-sample Malformed=1, AfterCutoff=0, PARTIAL, snapshot not cleared as excluded. | Drop ingest `Err`; count AfterCutoff; only fail in the FC-1 join case |
| F3-GIT-REQUEST-READONLY | `--output` / `--output=path` and external-helper options wrap `ErrInvalidSourceSpec`. Current read-only rev-parse/rev-list/show/ls-tree/for-each-ref/cat-file blob forms stay valid. Bounded `rev-list` without `--topo-order` (`--max-count` + `--format=%cI` with `--all` or a snapshot SHA, optionally `--parents`) is accepted so snapshot-tip batches need not change public `SourceGitRequest`. | Allow `--output`; reject `--git-common-dir` or `--max-count`; reject current blob form; reject bounded non-topo `rev-list` |

Descriptor-tied live/journal discovery remains a **body code-review obligation** (no flaky race test). Performance batching remains advisory. Ref snapshot consistency and bounded traversal remain required by the existing history seals plus `F3-EMPTY-HISTORY-CONSISTENT` / `F3-BOUND-COMMITS-LIMITER`.

## FC-SOURCES follow-up corrective seals

Independent seals for frozen-contract defects from
`2026-09-06T02-15-10Z-FC-SOURCES-corrected-panel` and the operator F3
clarification at the end of `FC-SCAFFOLD.md`. Existing assertions, fixtures,
the operator split, and FC-1 ownership are unchanged. New cases register inside
`TestFCSourcesContract` only. Bodies cannot edit these seals.

### Follow-up panel dispositions

| Finding | Disposition |
|---|---|
| Detached HEAD omitted from `for-each-ref` traversal | `F3-DETACHED-HEAD-ALL-REFS`. A commit reachable only from detached HEAD is enumerated and `ResolvedRefs` records the captured `HEAD` commit. COMPLETE may not silently drop it. |
| All-ref names not Git ref syntax | `F3-COMPLETE-RESOLUTION` adds `a..b`, `.lock`, control-char, and outside-`refs/` counterfactuals. `HEAD` is the allowed all-ref pseudoref; explicit requested `HEAD` remains separately valid. |
| Successful Git child closed after one-second cleanup | `F3-GIT-BUFFERED-EXIT-READ`. Later read retains the full payload; forced closure must not become a clean truncated EOF. Descendant/cancel cleanup stays bounded. |
| Byte cap reclassified as bad Git syntax | `F3-BOUND-METADATA-FRAGMENT` cuts a `rev-list` commit header and a timestamp through the current private `readHistoryCommits` seam. `ErrBoundExceeded` is retained. The helper signature is not a public contract. |
| Noncommit fallback peels a mutable name | `F3-NONCOMMIT-REF-PEEL` observes wrapper argv: peel `<captured-id>^{commit}`, never `refs/odd/blob^{commit}`. Cancellation/bound identity preservation on that fallback is a **body code-review obligation** (no extra public hook). |
| Graft `Stat` errors treated as missing | `F3-GIT-GRAFT-INSPECT-ERROR`. A deterministic self-symlink graft path cannot yield COMPLETE. Ignore only `NotExist`. |
| Final-component symlink TOCTOU | Existing `F3-SRC-ROOT-ESCAPES` static refusal stays. Atomic no-follow open relative to the confined parent is a **body code-review obligation**; no flaky rename-race test. |
| Empty-history under-specified | `F3-EMPTY-HISTORY-CONSISTENT` now requires PARTIAL, a diagnostic, canonical lists, and `ValidateComplete` `ErrSourceIncomplete`. |
| Eager `--topo-order` / argv grows with refs | `F3-BOUND-COMMITS-LIMITER` still requires `MaxCommits+1`. `F3-GIT-FULL-HISTORY` still requires every reachable merge parent. Unique-commit accounting stays in `F3-BOUND-COMMITS`. Bounded snapshot-tip batches are permitted; public `SourceGitRequest` is unchanged. Provider laziness and argv-vs-ref-count bounds are a **body code-review obligation**. Per-commit `ls-tree` remains advisory. |
| `F3-BOUND-TOTAL-CONCURRENT` 100ms comment | Comment corrected only. Cap assertion unchanged. Old-body `bytesRead=130` retained as mutation evidence. No new acquisition hook. |

### Follow-up cases, expected failure, mutation

| Case | Expected | Mutation that must fail |
|---|---|---|
| F3-DETACHED-HEAD-ALL-REFS | Detached-only commit is enumerated (`git:<sha>`). `ResolvedRefs` records `HEAD` at that SHA and `refs/heads/main` at the branch tip. COMPLETE cannot omit it. Fixture uses the existing per-repo clock. | Walk only `for-each-ref` tips; omit `HEAD`; claim COMPLETE without the detached commit |
| F3-GIT-BUFFERED-EXIT-READ | Controlled child writes a unique payload and exits 0. The test waits on the private `sourceGitReadCloser.processDone` channel, then crosses the old one-second post-Wait cleanup deadline, then `Read`s. Exact full payload and a nil terminal read error are both required. The wait is `processDone` then a wall delay, not a proof of concurrent reader/Wait interleaving. Bounded waits/cleanup (`fixtureHarnessWait`), minimal env, no network. | Close stdout one second after `Wait` and map `os.ErrClosed` to `io.EOF`; stall, return short data, or return the full payload with a non-nil terminal error |
| F3-BOUND-METADATA-FRAGMENT | Metadata byte cap cutting a commit header or its timestamp returns `ErrBoundExceeded`. Do not reclassify the fragment as malformed Git syntax. | Parse the truncated line first and wrap only `ErrGitHistory` |
| F3-NONCOMMIT-REF-PEEL | Fallback `rev-parse --verify --end-of-options` peels the captured object ID. Argv must contain `<id>^{commit}` and must not contain `refs/odd/blob^{commit}`. Noncommit refs still cannot produce COMPLETE. | Peel the live ref name; skip the blob ref and mark COMPLETE |
| F3-GIT-GRAFT-INSPECT-ERROR | Self-symlink/uninspectable `info/grafts` cannot yield COMPLETE. `ValidateComplete` wraps `ErrSourceIncomplete`. NotExist remains the only missing-graft case. | Treat every `Stat` failure as absent grafts and return COMPLETE |
| F3-GIT-REQUEST-READONLY/bounded-rev-list-without-topo | Bounded `rev-list --max-count=4 --format=%cI` with `--all` or a SHA, optionally `--parents`, is a valid read-only form. Existing `--topo-order` forms stay valid. | Keep the grammar pinned to `--topo-order` as args[1] so snapshot-tip batches cannot run |

Atomic final-component open, fallback cancel/bound wrapping, provider-side walk laziness, and argv independence from ref count remain **body code-review obligations**. No flaky TOCTOU test, no giant timed walk benchmark, and no new public Git request field. Per-commit `ls-tree` stays advisory.

## FC-SOURCES incomplete-discovery corrective seals

Independent seals for frozen-contract defects from
`2026-09-06T03-12-17Z-FC-SOURCES-corrected-panel` and the operator F3
ruling at the end of `FC-SCAFFOLD.md` ("incomplete discovery and capped
subsets"). Existing assertions, fixtures, the operator split, and FC-1
ownership are unchanged. New cases register inside `TestFCSourcesContract`
only. Bodies cannot edit these seals.

### Latest panel dispositions

| Finding | Disposition |
|---|---|
| Journal discovery silently drops direct-child symlinks | `F3-JOURNAL-SYMLINK-CHILD`. One real run plus a direct-child symlink (named `latest`, targeting the in-root real run) must be PARTIAL with a reason naming `latest`, never COMPLETE. No traversal. No `latest` exemption. Local fixture target only. |
| Invalid/corrupt HEAD treated as unborn | `F3-HEAD-SYMBOLIC-INVALID`. `ref: refs/heads/main..bad` and a simple missing symbolic target in a repo that still has `refs/heads/main` must be typed `ErrGitHistory`/PARTIAL, never COMPLETE. Genuine unborn symbolic HEAD remains a valid absence control. Missing detached object is a GREEN typed-`ErrGitHistory` control: `rev-parse --verify --quiet HEAD` returns the missing ID with exit 0 and the existing captured-ID peel already fails; that is not a new red defect. |
| Final journal parent follows in-root directory symlink | `F3-OPEN-SOURCE-SYMLINK-PARENT`. Direct `openSourceFile` of `journal.jsonl` under an in-root symlink parent is refused. Deterministic static layout, not a rename race. Held confined root uses existing budget helpers. Legitimate `real-run/journal.jsonl` still opens. |
| Caller `Close` after successful exit becomes incomplete-stderr | `F3-GIT-CLOSE-SELF-CANCEL`. Close without Read after a 0-exit child must not return `ErrGitHistory` incomplete-stderr caused solely by `ioCtx` cancellation. Controlled inherited stderr pipe, bounded wait on private `processDone`, release of the descendant. Existing `F3-GIT-CLOSE-ERRORS` nonzero-exit, parent-cancel, and bound seals stay. |
| Buffered-exit read not synchronized on `processDone` | `F3-GIT-BUFFERED-EXIT-READ` tightened: wait on private `processDone` before crossing the old cleanup deadline; require exact full payload **and** nil terminal read error. No public hook. No stronger scheduling claim than `processDone` then a wall delay. |
| Newest-N / HEAD-first capped subset | **Rejected as acceptance.** Operator: a binding `MaxCommits` retains a deterministic traversal-order subset and PARTIAL. No global newest-N or HEAD-within-cap promise. COMPLETE uncapped history must still include every captured tip/HEAD (`F3-DETACHED-HEAD-ALL-REFS`, `F3-GIT-FULL-HISTORY`). Bounded snapshot-tip batches remain permitted. Recorded as a contract assumption, not unimplemented acceptance. |
| Remaining-capacity batch metadata | **Optimization request, not acceptance.** Repeated metadata reads consume the actual total-byte budget and may cause PARTIAL. Duplicate metadata charges are permitted. Remaining-capacity batching that miscounts duplicates is not required. |
| Unix build-tag restoration | **Body/code-review.** No new platform-specific imports in shared tests. |
| `O_NONBLOCK` on accepted regular files | **Body/code-review.** Nonblocking may remain on the atomic type-probe open to avoid a FIFO-open hang. No unbounded FIFO fixture. Clearing nonblocking on accepted regular files is not sealed here. |

### Latest cases, expected failure, mutation

| Case | Expected | Mutation that must fail |
|---|---|---|
| F3-HEAD-SYMBOLIC-INVALID/invalid-double-dot-target | Repo with valid `refs/heads/main` and HEAD `ref: refs/heads/main..bad` is typed `ErrGitHistory` and not COMPLETE. | Treat `rev-parse --verify --quiet HEAD` exit 1 as unborn and return COMPLETE |
| F3-HEAD-SYMBOLIC-INVALID/missing-symbolic-target | Symbolic HEAD to `refs/heads/does-not-exist` while `main` exists is typed `ErrGitHistory` and not COMPLETE. | Omit HEAD and walk only `main` as COMPLETE |
| F3-HEAD-SYMBOLIC-INVALID/unborn-absence-control | Verified unborn symbolic HEAD is omitted, PARTIAL, and not a typed `ErrGitHistory` fault. | Record HEAD, mark COMPLETE, or classify unborn as corrupt Git history |
| F3-HEAD-SYMBOLIC-INVALID/missing-detached-peel-control | Missing detached object ID is typed `ErrGitHistory`/not COMPLETE via the existing captured-ID peel. Green control. | Ignore peel failure and return COMPLETE |
| F3-GIT-REQUEST-READONLY/symbolic-ref-readonly | Exact `symbolic-ref --quiet HEAD` and `show-ref --verify --quiet refs/heads/main` are valid. Write/delete `symbolic-ref` forms wrap `ErrInvalidSourceSpec`. | Reject the read-only inspections; allow `symbolic-ref HEAD <ref>` / `-d` / `--delete` |
| F3-JOURNAL-SYMLINK-CHILD | Real run plus direct-child symlink `latest` is PARTIAL with a reason naming `latest`, never COMPLETE, and `latest` is not traversed. | `continue` with no reason so one real run becomes COMPLETE; traverse the alias; exempt `latest` |
| F3-OPEN-SOURCE-SYMLINK-PARENT | `openSourceFile` on `alias-run/journal.jsonl` (in-root symlink parent) is refused (`ErrSourceMissing` or `ErrInvalidSourceSpec`). `real-run/journal.jsonl` still opens on the same held root. | `root.Open(parent)` follows the in-root directory symlink and reads the file |
| F3-GIT-CLOSE-SELF-CANCEL | After a 0-exit child whose inherited stderr is still held, caller Close without Read returns nil, not incomplete-stderr `ErrGitHistory` from `ioCtx` cancel. Descendant released; no sleeper leak. | Classify self-cancel of the stderr drainer as `ErrGitHistory` incomplete stderr |
| F3-GIT-BUFFERED-EXIT-READ | After `processDone` and the old one-second cleanup deadline, delayed Read returns the exact payload and a nil terminal error. | Timer-close stdout; return short data; return full data with a non-nil read error |

Exact allowlisted inspections are `symbolic-ref --quiet HEAD` and `show-ref --verify --quiet <canonical-ref>`. Directory-component no-follow for actual journal opens is sealed by the static `F3-OPEN-SOURCE-SYMLINK-PARENT` case, not a flaky rename race. Unix tag restoration and regular-file blocking-mode restoration remain **body code-review**. No newest-N/HEAD-first cap test. No remaining-capacity batching test.
