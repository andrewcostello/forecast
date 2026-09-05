# FC-SEALS: F1–F4 contract seals

Independent seals against the frozen FC-SCAFFOLD handoff. No implementation
edits, no `t.Skip`, no xfail, no `config/known-red.yaml` edits.

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

CLI targets live under `cmd/forecast/testdata/dispatched/targets/`.

## Green vs red

Existing regression tests remain green, including baseline `Table.Add` join
(identity still omits run ID on that path) and `journal_facts` overlapping
timing. Two additive green amendments:

- `TestDispatchedReferenceBuildCommandIsRegistered` now lists `--timeout` and
  `--fail-on-uncovered-required` (FC-1 panel Grok-7).
- `TestCoverageStatesHandFinishedLimit` now asserts `Build` itself populates
  `Limits` (FC-1 panel Codex-3). Mutation: delete `HandFinishedLimit` from
  `Build`'s artifact constructor.

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
| F1-HASH-PROVENANCE | Hash/prev_hash round-trip | Strip hash on EventRef |
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
| F2-SPAWN-WIRE | `duration_ms` 1250 → 1.25s; zero vs null | Decode scalars into `Measured` |
| F2-CORRECTIVE-BOUNDARY | Overlap component withheld; elapsed survives | Keep overlapping panel+iterate spans |
| F2-PRODUCER-SHAPES | Unknown `panel_started` → LinesUnparsed | Guess a review start |
| F2-CORRECTION-KINDS | 6 counted kinds | Also count paired panel spawn finishes |
| F2-VERIFICATIONS | Count `verification_started` only | Count skipped/mechanical |
| F2-EVENT-IDENTITY | Retransmission once; colliding seq discarded | Merge collisions |
| F2-ALL-SPAWN-COST | All kinds including verifier/design; uncached input only | Add cache tokens |
| F2-ARITHMETIC-ERRORS | Overflow named, no saturation | Saturate duration |
| F2-LATER-START | StartsAfterCutoff=1, no attempt | Negative elapsed |
| F2-INITIAL-SPAWN | `[T1-D,T1)`, residual includes setup | Use `task_started` as development start |
| F2-ALL-KINDS | Closing stamp from implementing kinds; design does not stamp | Stamp designer |
| F2-CITATION-MEMBERSHIP | Evidence cites list[0]; lists complete | Evidence cites a leftover ref |
| F2-SUMMARY-AVAILABILITY | Same numbers, different Complete | Drop Complete |
| F4-OUTCOME-WIRE | `done/blocked/unfinished`; invalid → `ErrInvalidOutcome` | Decode missing outcome as unfinished |
| F2-DESIGN-SPAWN | Cost included; incomplete wall | Classify design as development |
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
| F3-SRC-MALFORMED-PARTIAL | Valid sibling kept; PARTIAL; Reading.Err set | Abort the document |
| F3-READING-ENVELOPE | Four independent envelopes | One row-zero error for typed mismatch |
| F3-NON-TASK-DOCUMENT | `DocumentNotTasks`, not Malformed/PARTIAL | Count as malformed |
| F3-SRC-RESOLVED-REF | SHA in ResolvedRef | Leave branch name |
| F3-GIT-ENV-STRIPPED | Drop `GIT_DIR`/`GIT_CONFIG*` family; pin `/dev/null` | Honour `GIT_CONFIG_COUNT` |
| F3-GIT-INSTALLATION-ENV | Keep `GIT_EXEC_PATH` and PATH | Strip exec-path too |
| F3-GIT-SHALLOW | Shallow+PARTIAL; ValidateComplete wraps both incompleteness sentinels | Report COMPLETE |
| F3-GIT-FULL-HISTORY | Superseded merge parent blob present | First-parent only |
| F3-GIT-DELETED-RENAMED | Old path still read | Skip deleted blobs |
| F3-BOUND-COMMITS | Cap before collection; PARTIAL | Walk then trim |
| F3-BOUND-BYTES | PARTIAL on blob cap | Buffer the whole blob |
| F3-BOUND-PROCESSES | Negative invalid; cap does not mark PARTIAL | Treat as data cap |
| F3-CANCELLED | Both cancel sentinels; not COMPLETE | Ignore ctx |
| F3-HOLDOUT-EXCLUDED | Held-out journal in ExcludedJournals; live AND git-history YAML rows with `dispatcher_run_id: held` marked HeldOut with snapshot cleared; keep rows remain in-sample | Feed held-out events to reducer; ignore history YAML |
| F3-CUTOFF-EXCLUDED | AfterCutoff audit envelope | Sample later completed_at |
| F3-SELECTION-INVALID | Padded/duplicate/zero cutoff rejected before IO | Trim then match |
| F3-HOLDOUT-PADDED-STILL-EXCLUDES | `ErrInvalidSelection` | Match after trim |
| F3-HOLDOUT-UNMATCHED | `ErrInvalidSelection` | Ignore unknown IDs |
| F3-MISSING-REVISION-TIME | Zero RecordedAt is malformed in-sample | Treat as AfterCutoff |
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
| F3-SOURCE-CONCURRENCY | Sequential sources, per-source slots | Fan-out across sources |
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
| F1-EV-MERGE-PERMUTATION | Journal 10m+journal citation together | Elapsed 12m with journal label |
| F1-EV-NO-MANUFACTURED-ROW | No independent max across YAML X/Y | Max elapsed from Y, cost from X |
| F1-EV-JOURNAL-OVER-YAML | Journal done beats YAML blocked | Conflict or YAML wins |
| F1-EV-YAML-ONLY-TERMINAL | YAML source; counter=1 | Leave unfinished |
| F1-EV-PERMUTATION | Canonical JSON across set order | First-write-wins |
| F1-MODEL-NO-ALIAS-POOL | Two cells | Pool opus aliases |
| F1-ROLE-CITATION | Conflicting roles not recovered | Pick first role |
| F1-READING-TOTAL-ORDER | Examined canonical | Input order |
| F1-ATTEMPT-RUN-CONSISTENCY | `ErrInvalidSelection` | Join anyway |
| F3-DISPOSITION-* | Every snapshot; no-run; missing keys | Drop unmatched YAML |
| F3-ROWS-VS-ATTEMPTS | UniqueRows=1 Attempts=2 | Collapse restarts |
| F3-LOST-NOT-HIDDEN | Lost sibling listed | Count recovered as covering it |
| F3-DIRECT-HOLDOUT* | Unmatched/held-out attempt sets refused | Produce observations |
| F3-EXCLUDED-JOURNAL-AUDIT | Full identity survives | Reduce to run-id strings |
| F2-YAML-TERMINAL-WALL | Rebase elapsed; withhold outside spans | Clip intervals |
| F4-TARGET-* | Empty → ErrEmptyTarget; malformed → ErrInvalidTarget | Gate on aggregates |
| F4-ELIGIBLE-THRESHOLD | Eligible at n=2 | Eligible at n=1 |
| F4-NOT-ELIGIBLE-THIN | ErrNotEligible only | Also wrap incomplete |
| F4-NOT-ELIGIBLE-PARTIAL | Both sentinels when refuse | Eligible diagnostic |
| F4-HAND-FINISHED-LIMIT | Limits from amended Build; report prints it | Formatter-only |
| F4-BUILD-AMENDED-OPTIONS | No silent v3 legacy path | Ignore Sources/holdout |
| F4-THRESHOLD-NONPOSITIVE | DefaultMinObservations | Keep 0 |
| F4-VERSION-EXACT | v3/v5/missing Evidence refused | `>=` comparison |
| F4-MIXED-OPTIONS | ErrInvalidSourceSpec | Ignore one side |
| F4-ARTIFACT-HOLDOUT | Held-out run in artifact refused | Predict anyway |
| F4-STRUCTURED-THIN-CELL | Cells name completed/threshold | Reasons only |
| F4-CELL-EMPTY-N0 | Empty required cell present n=0 | Omit it |
| CLI F3-SRC-EXPLICIT-ONLY | No HOME default; no usage | Default `~/Project/claude-workflow` |
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

## Residual limitation

New groups are red until FC-JOURNAL / FC-SOURCES / FC-1 replace `ErrNotImplemented`.
Baseline extraction defects remain visible on the legacy path, as required.
