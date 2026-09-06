# FC-OBS-ADJ: observation adjudication on the accepted FC-1 head

## Scope and reviewed head

This row adjudicates the completed observation/extraction unit (FC-SCAFFOLD,
FC-SEALS, FC-JOURNAL, FC-SOURCES, FC-1) under F1–F4 of
`docs/plans/forecasting-contracts.md`, using `docs/plans/traceability.md` for
the original FC-1 panel and the operator rulings for every later panel.

- Adjudicated head: `c3ec1180d034cf0f63427dc54d88a77377c9d309`, the dispatcher
  merge of accepted FC-1 head `bf7163b7755b82ecadc11881f1de82d0a99245f3` into
  this branch. `git rev-parse` shows both commits have the **same tree**, so
  every result below applies byte-for-byte to the exact head the sixth panel
  (`2026-09-06T09-32-02Z-FC-1-corrected-panel`, APPROVE ×3) and verifier
  (`2026-09-06T09-32-02Z-FC-1-verification`, VERIFIED) reviewed.
- This row changed **no** source, test, fixture, schema, scaffold, known-red
  or worklist file. `internal/dispatched/observation.go` and
  `docs/plans/forecasting-contracts.md` are owned by this row and were left
  unchanged: no ruling below amends a contract, and no implementation is hidden
  in adjudication. Every finding that would need code is assigned to a named
  later row or accepted with its reason.
- Evidence directory:
  `/home/andrew/Project/dispatcher-runs/2026-09-05T00-59-44Z-forecasting-v2/FC-OBS-ADJ/evidence`
  (`gate.sh`, `results.txt`, logs, `seals.jsonl`, `coverage.out`,
  `coverage.json`, `line-diagnostic-proof/`, `after-audit-relocation/`).
- Not in this row's power: the pinned-head cross-family panel and verifier for
  this adjudication row are launched by the dispatcher after this note; this
  worker did not spawn reviewers. The accepted unit's own pinned-head panel is
  the one cited above.

## Gate evidence on this head (no exclusions)

All commands ran from the worktree with `DISPATCHER_KNOWN_RED_FILE` unset, so
no reserved group was hidden.

| Check | Result |
|---|---|
| `go build ./...` | pass |
| `go vet ./...` | pass |
| Four observation seal groups (`TestFCJournalContract`, `TestFCSourcesContract`, `TestFCEvidenceContract`, `TestFCReferenceCLIContract`) with `-race -count=1 -json` | 466 pass, 0 fail, 0 skip, 0 panics, 0 data races (leaf passes: Evidence 292, Sources 98, Journal 51, CLI 21) |
| `env -u DISPATCHER_KNOWN_RED_FILE go test ./... -race -count=1` | pass, all 12 packages with tests, no exclusions |
| Statement coverage, `internal/dispatched` (the observation/extraction package, same scope as the fifth-panel 84.01% measurement) | **83.88%** (3300 / 3934 statements) — above the retained ≥80% requirement |
| Working tree after all runs | clean (`git status --porcelain` empty) |

Per-file coverage: `build.go` 87.1%, `evidence.go` 86.5%, `extract.go` 93.5%,
`journal.go` 83.0%, `observation.go` 92.2%, `referenceclass.go` 97.4%,
`sourcefile_unix.go` 77.3%, `sources.go` 80.4%. The 0.13-point difference from
the fifth-panel figure is five timing-dependent statements in `sources.go`
(Git lifecycle/readiness branches); both measurements exceed the threshold.

Typed errors: every source/data failure surfaces one of the 33 exported
sentinels in `internal/dispatched/errors.go`, wrapped with `%w`; the four seal
groups assert `errors.Is` identity in 69 places. The unwrapped `fmt.Errorf`
strings inside `build.go` are inner reasons that the eligibility gate wraps in
`ErrNotEligible` plus `ErrSourceIncomplete` (sealed by `F4-ELIGIBLE-*`), and
`errJournalTotalBound` is private and always surfaces as `ErrBoundExceeded`.

## Reproduced manifest-based real-data report

Command (binary built from this head, receipt in the evidence directory):

```
forecast dispatched-reference build \
  --runs-dir /home/andrew/Project/dispatcher-runs \
  --features-repo /home/andrew/Project/claude-workflow \
  --task-root features/dogfood-go --task-root features/model-matrix \
  --tasks features/dispatched-forecasting/tasks.yaml --timeout 20m --out <evidence>/coverage.json
```

Selection matches the FC-1 note: journals root, one explicit repository at
HEAD `0d3c5cda59890728784bb5fbbfb60c0f7bdd2c33` (moved on from the FC-1 run's
`fa949ca7`; two uncommitted files present, live source read as-is), the two
minimal inventory roots, this repository's 20-row target, no holdout IDs. The
wallet YAML roots were not selected and no wallet outcome file was opened.

Two runs were made; both are preserved.

1. **First run, cutoff `2026-09-06T09:42:50Z`.** Journals source PARTIAL: 106
   files, 424 malformed records, two journals with no producer
   (`2026-09-06T08-22-02Z-FC-1-corrective-seals/journal.jsonl` and
   `2026-09-06T09-12-27Z-FC-1-corrective-seals/journal.jsonl`). Those two files
   were operator `go test -json` audit logs saved under the reserved
   direct-child filename, not dispatcher event files. The code behaved exactly
   as F3 requires: malformed records counted, producer refusal typed as
   `ErrJournalSource`, source and aggregate marked PARTIAL with named reasons,
   diagnostic artifact retained, no usage output, exit 1.
2. **Second run after the operator relocated only those two audit logs**
   (`evidence/operator-audit-log-relocation.json`, SHA-256 receipts; no
   dispatcher event file or task data touched), cutoff
   `2026-09-06T09:44:46Z`, same argv, receipt in
   `evidence/after-audit-relocation/`. Journals source COMPLETE: 104 journals,
   11,827 retained events, 0 malformed, 0 unreadable. Live YAML COMPLETE (112
   records, 6 files). Git history COMPLETE (3,309 records, 84 recorded refs, not
   shallow/grafted/replaced). Aggregate **PARTIAL** for exactly one reason: the
   equal-rank terminal conflict in `2026-08-31T23-54-47Z-tasks/GO-1-1`
   (`blocked` vs `done` at the same instant, 34 conflicting readings).

Counts (second run; the recovered set is identical in both runs and identical
to the FC-1 historical report):

| Quantity | Value |
|---|---|
| Journal start attempts / unique run-task rows | 311 / 297 |
| Recovered attempts (one joint record each) | 44 |
| Not-recovered attempts, individually retained | 267 (across 91 runs) |
| YAML readings examined / dispositions | 3,421: recovered 44, duplicate_reading 367, missing_join_keys 2,711, no_matching_start 227, absent_stamp 38, conflicting_evidence 34, everything else 0 |
| Conflicting / ambiguous attempts, YAML-only terminals, after-cutoff starts | 1 / 0 / 0 / 0 |

Target coverage (threshold `min_completed=2`, 20/20 rows name a valid cell):

| Required cell | target rows | n | n_done | state |
|---|---|---|---|---|
| adjudicate / claude-fable-5-1 | 4 | 0 | 0 | empty, uncovered |
| bodies / gpt-5.6-sol | 6 | 0 | 0 | empty, uncovered |
| bodies / grok-4.6 | 1 | 0 | 0 | empty, uncovered |
| scaffold / claude-fable-5-1 | 4 | 0 | 0 | empty, uncovered |
| scaffold / claude-opus-5 | 1 | 0 | 0 | empty, uncovered |
| seals / gpt-5.6-sol | 2 | 7 | 0 | thin (all 7 blocked), uncovered |
| seals / grok-4.6 | 2 | 1 | 1 | thin, uncovered |

**All seven required cells remain uncovered; five are empty; 0/20 target rows
are covered.** The 44 recovered attempts are 32 blocked, 2 unfinished
(censored) and 10 done, spread over 18 (role, model) cells, none of which
reaches two completed samples for a required cell.

Reading this report **releases the synthetic scheduler rows** (FC-SCHED-*
depend only on F5 fixtures) and **does not declare prediction data
sufficient**: the artifact is PARTIAL, `PredictionEligibility` would refuse it,
and no wallet holdout evaluation or training population is claimed.

Wallet handling in this reproduction: the CLI has no holdout flag (deferred
below), so the 18 `*-wallet-v2-tasks` journals in the shared runs root were
read as journal starts. They contribute only to the not-recovered denominator
(93 lost attempts); **no wallet attempt entered any observation or cell**
because no wallet YAML root was selected. FC-ADJ must freeze its holdout run
IDs in `Selection.HoldoutRunIDs` before extraction as F6 requires; this
diagnostic report is not that frozen population.

Two cosmetic observations, neither a contract deviation: the error line is
printed twice on stderr (once by Cobra, once by `main.go`), with no usage text;
and a diagnostic build without gate flags exits 1 when the manifest is PARTIAL
while still writing the artifact and report. Both belong to the
FC-PREDICT-SCAFFOLD/FC-4 command surface if anyone wants them changed.

## Original FC-1 panel: final dispositions for all 19 findings

Head reviewed by that panel: `d529265`. Seal names are cases that pass on this
head in the runs above.

| ID | Final disposition |
|---|---|
| Claude-1 (High, overlapping dev/review) | **Corrected under seals.** Producer-order reducer with disjoint classified intervals bounded by elapsed; `F2-PHASES-DISJOINT`, `F2-PHASES-ITERATE-AFTER-SPAWN`, `F2-PHASES-PANEL-WALL`, `F2-UNCLASSIFIED-RESIDUAL-ONLY`. |
| Claude-2 (Medium, simplified history) | **Corrected.** Full reachable history over all captured refs; `F3-GIT-FULL-HISTORY`, `F3-GIT-DELETED-RENAMED`, `F3-DETACHED-HEAD-ALL-REFS`. |
| Claude-3 (Medium, empty target passes gates) | **Corrected.** `F4-TARGET-ZERO-ROWS`, `F4-GATE-REQUIRES-TASKS`, `F4-GATE-BYPASS-EMPTY-TARGET` (library and CLI). |
| Claude-4 (Medium, ingest replay) | **Out of scope, still open as `INGEST-REPLAY`** per traceability; no FC row claims it fixed. |
| Claude-5 (Medium, all blobs buffered) | **Corrected.** Streamed/capped bytes, processes and cancellation; `F3-BOUND-BYTES`, `F3-BOUND-TOTAL-CONCURRENT`, `F3-BOUND-PROCESSES`, `F3-CANCELLED`. |
| Claude-6 (Low, redundant counter/stale prose) | **Corrected.** Schema-4 report prints validated target states and manifest facts; `F4-TARGET-INPUT`, `F4-ARTIFACT-CELL`; legacy counters are named as uncomputed in `Limits`. |
| Claude-7 (Low, order-dependent ties) | **Corrected.** `F1-EV-PERMUTATION`, `F1-EV-MERGE-PERMUTATION`, `F1-READING-TOTAL-ORDER`, `F1-CONFLICT-TOTAL-ORDER`. |
| Claude-8 (Low, personal repo default) | **Corrected.** `--features-repo` required, repeatable; `F3-SRC-EXPLICIT-ONLY`; `docs/COMMAND_REFERENCE.md` updated. |
| Codex-1 (High, identity omits run ID) | **Corrected.** `AttemptID` is (run, key, UTC start); `F1-ID-DISTINCT-RUNS`, `F1-ID-SAME-RUN-REVISIONS`, `F1-ID-UTC-OFFSET`. |
| Codex-2 (Medium, premature recoverable credit) | **Corrected.** Recovery only after exact match; `F1-ID-NEAREST-NOT-MATCHED`, `F3-ROWS-VS-ATTEMPTS`, `F3-LOST-NOT-HIDDEN`. |
| Codex-3 (Medium, hand-finished formatter-only test) | **Corrected.** Build-to-report limitation; `F4-HAND-FINISHED-LIMIT` in both groups. |
| Grok-1 (High, silent empty discovery) | **Corrected.** `F3-SRC-MISSING`, `F3-SRC-ZERO-JOURNALS`, `F3-ALLOW-EMPTY-REASONS`, `F3-BUILD-ALLOW-EMPTY-STATE`. |
| Grok-2 (High, single-source default, stale brief) | **Corrected.** Explicit manifest in every artifact; `docs/BRIEF-dispatched-work-forecasting.md` is now an authority pointer to the plans, old brief archived. |
| Grok-3 (Medium, shallow/grafted looks complete) | **Corrected.** `F3-GIT-SHALLOW`, `F3-GIT-WORKTREE-GRAFTS`, `F4-NOT-ELIGIBLE-PARTIAL`. |
| Grok-4 (Medium, inherited Git env) | **Corrected.** `F3-GIT-ENV-STRIPPED`, `F3-GIT-INSTALLATION-ENV` (the `GIT_EXEC_PATH` retention is a frozen, sealed decision; see FC-SOURCES residuals). |
| Grok-5 (Medium, unbounded diff/blob, lost progress) | **Corrected.** Bounded streaming with explicit `ErrBoundExceeded`/PARTIAL and retained diagnostics; `F3-BOUND-*`, `F3-CANCEL-PERSIST-PARSE`, `F3-SRC-MALFORMED-PARTIAL`. |
| Grok-6 (Medium, YAML with no journal vanishes) | **Corrected.** Named dispositions and reconciliation counts; `F3-DISPOSITION-NO-RUN`, `F3-DISPOSITION-MISSING-JOIN-KEYS`, `F3-DISPOSITION-EVERY-SNAPSHOT`. |
| Grok-7 (Low, flags absent from registration) | **Corrected.** `F4-MISSING-FLAGS` asserts timeout and both gate flags. |
| Grok-8 (Low, data errors print usage) | **Corrected.** `F4-DATA-ERROR-NO-USAGE`, `F4-UNKNOWN-FLAG-USAGE`, `F4-TIMEOUT-USAGE`. |

## FC-JOURNAL panel findings (inherited, approve with one dissent)

Panel on `4bee4ed`: consensus approve; two uncorroborated Codex Highs. Ruled on
this head:

| Finding | Disposition |
|---|---|
| Codex High: `ReduceAttempts` returns nil error with retained conflicts | **Accepted with reason, controlled at Build.** F1 says a conflict is a named, counted exclusion. Amended `Build` treats any retained `AttemptSet.Conflicts` as `ErrEvidenceConflict` and marks the aggregate PARTIAL; the reproduced real report shows exactly that for `GO-1-1`. Direct reducer callers must read `Conflicts`; that is documented, not hidden. |
| Codex High: keyless non-`run_started` candidates dropped without `LinesUnparsed` | **Accepted with reason.** In the measured producer, keyless events are run-level (`heartbeat`, `run_complete`, `resume_started`, `notify_sent`, `preflight`; 366 of 488 events in this run's own journal). Counting them as unparsed would misreport healthy journals. A malformed keyless `task_started` is a hypothetical the 0.1.0 producer never emits; if a producer amendment adds keyed-type validation it goes through FC-SCAFFOLD/SEALS. Nonblocking residual. |
| Claude Medium: `physicalEventLess` ties on Type before At for direct inputs | **Accepted, unreachable for real input.** `ParseEvents` output has unique lines; direct `ParsedJournal` callers are validated by `validateReducerEvents` and sealed by `F2-EVENT-IDENTITY`/`F2-LINE-ENVELOPE`. |
| Codex Medium: `validateReducerEvents` relabels `EventRef.Journal` | **Accepted nonblocking residual.** Direct-input provenance is FC-JOURNAL-owned; Build always supplies parser output, and FC-1 eligibility independently binds every carried citation to the selected journal (`F4-ELIGIBLE-PROVENANCE*`). |
| Codex Medium: quadratic `buildWall` | **Accepted.** Bounded by the byte cap; 11,827 real events reduce in seconds. No performance contract was frozen. |
| Codex Medium: `F1-EV-PROVENANCE-KEPT` under-asserted | **Superseded by later seals.** Fourth/fifth-panel measurement seals (`F4-ELIGIBLE-MEASUREMENT`, `F1-EV-TOKENS-CITED-SEPARATELY`) assert review/wall/token citations. |
| Grok Medium: total-cap mid-line surfaces as a full line | **Sealed.** `F2-PARSER-TOTAL-CAP` requires `ErrBoundExceeded` without decoding the fragment; passes here. |
| Grok Medium: reversed verdict drops whole attempt | **Accepted as contract.** F2: no malformed measurement reaches a mean silently; `F2-MEASURE-REVERSED` pins the refusal, and the attempt stays in the counted denominator as lost. |
| Grok Medium: read failures wrapped with `%v` | **Accepted nonblocking.** `ErrJournalSource` identity is preserved (`errors.Is` sealed); the underlying `io` error is lost from the chain only on the non-cancel path. Diagnostics only. |
| Claude Low: marker-deficit completeness; Claude Low: count-only discard diagnostics; Grok Low: duplicated spawn-kind lists | **Accepted nonblocking maintenance residuals** in FC-JOURNAL-owned code; no behavior requirement is affected and any change routes through scaffold/seals. |

## FC-SOURCES cumulative history and accepted-panel residuals

Sources: the five deviation-history snapshots
(`ab03f0a`, `1a2975c`, `4ee43ab`, `55a552d`, `f9c726f`) indexed at
`2026-09-06T04-03-09Z-FC-SOURCES-verification-retry/deviation-history/index.json`,
the current note, and the accepted panel `2026-09-06T04-25-45Z-FC-SOURCES-corrected-panel`
(APPROVE ×3 on `f9c726f`, VERIFIED). Historical Blocked/uncommitted statements
and the superseded first-body policies are history, not current claims.

| Item | Disposition |
|---|---|
| Claude-1 / Codex-1 (Medium): present-but-unreadable held-out journal reported as "not discovered" | **Accepted with reason; corrective seal required before holdout use, assigned downstream.** Direction is fail-closed (no holdout leak; extraction refuses). Appending before open would invent journals for empty run directories, contradicting `F3-JOURNAL-MISSING-CHILD`. The diagnostic inaccuracy is real. FC-PREDICT-SCAFFOLD/FC-PREDICT-SEALS, which own the frozen holdout surface, must add a metadata-confirmed unreadable-journal disposition and seal before FC-ADJ's wallet holdout run so a permission fault is not misread as a misspelling. Nonblocking for this row. |
| Claude-2 (Medium): run directory vanishing between `Info()` and open yields either `ErrSourceMissing` or a silent skip | **Accepted nonblocking with reason.** The shared runs root is an append-only archive by plan policy (nothing is deleted or requeued), the `Info()` path fails closed, and the open-path ENOENT skip is the sealed pending-run case. A parent-component ENOENT should become a named PARTIAL reason if a rotating root is ever selected; that is a narrow FC-SOURCES seals-plus-body amendment, not required for Done. |
| Claude-3 (Low): Windows `PathError` carries only the component; Unix returns bare `Errno` | **Accepted.** Diagnostics only; `errors.Is(fs.ErrNotExist)` identity holds on both platforms, and the returned journal-open error already carries source/repository/run context. Cross-platform runtime remains compile-checked only, as disclosed. |
| Earlier retained residuals: `GIT_EXEC_PATH` inherited (sealed by `F3-GIT-INSTALLATION-ENV`); capped `MaxCommits` is a deterministic traversal-order subset, not newest-N (operator ruling); repeated metadata charged to the byte budget; common-dir graft inspection is one local `os.Stat` with no deadline; `codex-6` 100 ms concurrency seal cannot prove simultaneous acquisition; cross-builds are not runtime proof | **Accepted with the recorded reasons.** Each is either a frozen sealed policy or a disclosed limitation with no contract requirement to the contrary. |
| Rejected findings (`grok-1` capped subset, raw-journal cutoff, `codex-1` descriptor leak, `grok-1` `DT_UNKNOWN`) | **Rejections stand**; each was disproved by ruling or by cited code and fixtures. |
| Contract deviation | **None.** Public seams, typed errors, bounds and `Selection` semantics match the scaffold. |

## FC-1 panels one through six: rulings adjudicated on the final head

All five operator rulings were applied by independent seals followed by pinned
body corrections; every corrected item is re-proven here because the four seal
groups pass with zero failures on this exact tree. Dispositions below cover the
items the task names explicitly; the FC-1 note tables enumerate every ID.

| Ruling item | Final adjudication |
|---|---|
| First panel: missing-role Unrecoverable clarification | **Accepted as ruled and sealed** (`F1-ROLE-ABSENCE`, `F1-ROLE-CITATION`): empty role is unknown, a valid sibling supplies it, nonempty invalid role is malformed, all-missing role stays Unrecoverable with PARTIAL aggregate; no inference from authored model or key. |
| First panel: positive eligibility fixture enrichment | **Accepted.** The synthetic `recoveredArtifact` was enriched by independent seal commits only, preserving n/IDs/outcomes/thresholds; a real Build→Eligibility control (`F4-ELIGIBLE-LOCAL-FIXTURE`) exists separately. |
| First panel: evidence-integrity corrections and false-positive controls | **Accepted.** Structured eligibility seals (`F4-ELIGIBLE-STRUCTURE`, `-AUDIT-BINDING`, `-LEGITIMATE-LOSS`) and the four controls (nil manifest, interval immutability, `OutcomeDone=iota+1`, CLI missing-`--tasks` refusal) all pass. No exactly-one-reduced-set-per-journal requirement exists (`F1-ATTEMPT-UNIVERSE`). |
| First panel deferrals: cutoff/holdout/allow-empty/ref CLI flags | **Accepted deferral to FC-PREDICT-SCAFFOLD/FC-4.** Library `Selection` already supports reproducible builds; consequence for this report recorded above. |
| Second panel: audit binding, selected provenance, total ordering, numeric domain, timeout usage | **Accepted as corrected and sealed** (`F1-ENVELOPE-ASSOCIATION`, `F4-ELIGIBLE-PROVENANCE`, `F1-SAME-REF-PERMUTATION`, `F4-ELIGIBLE-NUMERIC-DOMAIN`, `F4-TIMEOUT-USAGE`). |
| Second panel Grok-1: direct malformed `AttemptSet` salvage | **Accepted deferral, not implemented.** `JoinEvidence` stays fail-closed on invalid input; `Build` retains reducer-produced usable attempts plus aggregate diagnostics. No partial-salvage API is promised. |
| Second panel Claude-3: all-or-nothing legacy projection | **Accepted boundary.** Validated Build input cannot reach it with malformed walls; defensive failure leaves the artifact PARTIAL and ineligible. |
| Second panel Grok-2: bounded target reading | **Accepted deferral** to FC-PREDICT-SCAFFOLD's target snapshot contract. |
| Direct-child journal-layout follow-up | **Accepted as corrected**: nested and dot-cleaned run IDs refused, ordinary and internal-space components valid (`F4-ELIGIBLE-PROVENANCE`). |
| Third panel: drive-qualified provenance, total conflict comparator | **Accepted as corrected** (`F4-ELIGIBLE-PROVENANCE` drive cases, `F1-CONFLICT-TOTAL-ORDER`, `F1-CONFLICT-PORTABLE`). |
| Third panel Claude-2: future-field provenance enumeration | **Accepted with reason.** Today's schema fields are enumerated and checked; a future `Attempt`/`AttemptEvidence` field must amend its scaffold, seals and both enumerations. No reflection or shared enumerator. |
| Third panel Claude-3: carried role/source-value structural limit | **Accepted, disclosed.** Eligibility proves internal consistency and citation binding of carried values, not original snapshot truth or source-byte authenticity; stated in `Artifact.Limits` and printed by the real report. |
| Third panel Claude-4: validator factoring | **Accepted nonblocking maintainability note**; no behavior requirement. |
| Third panel Claude-5 / Grok-2: one-shot Cobra invocation | **Accepted with reason.** Reusing the mutated singleton after `RunE` is not a frozen embedding promise; fresh constructors are the tested path; an in-`RunE` reset could not fix pre-`RunE` parse failures. |
| Fourth panel: count/list correction | **Adopted as the current policy.** Frozen Journal counts equal complete unique event-list lengths; known totals, including known zero, cite contributors; the earlier operator exemption and the notes repeating it are superseded history. Sealed by 35 measurement leaves plus repaired positive fixtures. |
| Fourth panel: fixture repairs, ambiguous-ref determinism, actionable provenance reasons, root-guard ordering | **Accepted as corrected** (`F4-ELIGIBLE-MEASUREMENT`, `F1-AMBIGUOUS-REF-PERMUTATION`, `F4-ELIGIBLE-PROVENANCE-DIAGNOSTICS`, `drive-dotdot-root` controls). |
| Fourth panel Codex-1: mutation limitations | **Accepted as exactly recorded.** Replacing the final comparator proves total ordering is needed there; removing the final sort proves merge ordering; neither proves the other; no second valid conflict code is invented. |
| Fifth panel Codex-1: physical journal+line uniqueness | **Accepted as corrected and independently reproduced.** Per-list seen-line set separate from total `EventRef` order; nine physical-identity leaves and the distinct-line/cross-list controls pass. |
| Fifth panel Codex-2 / Claude-1: journal mismatch and attempt-context diagnostics | **Accepted as corrected** (`selected-run-b-journal-on-run-a-attempt`). |
| Fifth panel Codex-3: rule-token assertion strengthening; Claude-2 redundant guard removal | **Accepted**; overlay mutation recorded by the seal author, behavior unchanged. |
| Sixth panel (Claude Low, Codex Medium): bare-number physical-line assertion | **Accepted nonblocking test-maintenance limitation, dispositioned with a reproduced mutation proof.** See next section. |

### Sixth-panel bare-number assertion: supplemental proof reproduced on this head

`requireRepeatedPhysicalLineRefused` checks the line with a bare
`strconv.Itoa(line)` over the joined reasons; the wrapper's RFC3339 timestamp
already contains `2026`, so a `6` can be satisfied by the year. This row
reproduced the operator's external-overlay proof against this tree using
`go test -overlay`, without editing any tracked file
(`evidence/line-diagnostic-proof/`, tracked `build.go` SHA-256
`22a93caa…613c` unchanged before and after):

| Run | Result |
|---|---|
| Probe requiring `cascades` and `physical line 6` on current code | pass |
| Same probe with production message changed to drop only the line number | **fails** with the coupled-diagnostic message, while rejection, both sentinels and the attempt wrapper survive |
| Nine tracked physical-identity leaves under that same mutation | 7 fail; `input-tokens-same-line-seq` and `output-tokens-same-line-prevhash` still pass (both use line 6) |

Ruling: the production guarantee (field plus physical line named) is proven on
this head by the stronger external assertion; the tracked seal proves rejection
and field naming for all nine leaves and line naming for seven. The two weak
leaves are a seals-maintenance gap in an FC-SEALS-owned file. It is
**accepted with reason and nonblocking**: no acceptance requirement is waived,
because the diagnostic behavior itself is demonstrated by a failing mutation.
A narrow follow-up seals row may replace the bare integer with a
`physical line %d` token (or lines outside the timestamp digits); this row does
not edit the test.

## Ruled deviations summary

| Deviation | Ruling |
|---|---|
| Aggregate real corpus PARTIAL (`GO-1-1` equal-rank terminal conflict) | Accepted: correct F1 behavior; the artifact is diagnostic and prediction-ineligible. |
| Non-dispatcher `journal.jsonl` files in the shared runs root (first run) | Accepted: environmental; code responded per F3 with named reasons and retained artifact; operator relocated the audit logs with receipts. Runs-root hygiene: only dispatcher event files may use the reserved direct-child name. |
| Wallet journals read as starts in the CLI diagnostic report | Accepted: no wallet observation or cell; frozen holdout selection is FC-ADJ's predeclared responsibility via `Selection`/future flags. |
| Statement coverage 83.88% vs 84.01% earlier | Accepted: timing-dependent Git lifecycle branches; both above 80%. |
| Unreadable held-out journal diagnosed as unmatched | Accepted, fail-closed; corrective seal assigned to FC-PREDICT-SCAFFOLD/SEALS before holdout use. |
| Directory-rotation ENOENT asymmetry; platform diagnostic asymmetry | Accepted nonblocking (see FC-SOURCES table). |
| Deferred CLI surface (cutoff/holdout/allow-empty/ref, bounded target) | Accepted deferral to FC-PREDICT-SCAFFOLD/FC-4. |
| Legacy projection, direct malformed-set salvage, future-field enumeration, validator factoring, one-shot Cobra, carried-value authenticity | Accepted boundaries with the reasons above. |
| Bare-number seal assertion | Accepted nonblocking with reproduced mutation proof; optional seals-maintenance follow-up. |
| FC-JOURNAL uncorroborated findings | Accepted with reasons above; none reaches a mean or hides an attempt. |
| `INGEST-REPLAY` | Remains open outside the forecasting epic. |

No deviation requires a corrective row before this adjudication closes. Two
items are handed to named downstream rows (unreadable-holdout diagnostic seal;
optional line-token seal maintenance). No contract text was amended.

## Residual limitations

The observation unit proves carried structural consistency, typed refusals,
bounded complete-or-PARTIAL sourcing and deterministic reconciliation. It does
not authenticate source bytes, recompute payload sums, test non-Linux runtimes,
or supply enough completed samples for any required cell of the current target.
Prediction eligibility for this target is refused on data grounds, which is the
required honest result, not a defect.
