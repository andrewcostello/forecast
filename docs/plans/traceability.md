# Acceptance and finding traceability

This is a plan of required proof, not a claim that any listed defect is fixed.
Old worklists and documents are archived verbatim. The revised contracts state
all intentional policy amendments; task notes carry subsequent deviations.

## Original rows and acceptance

| Original row | Revised implementation and proof |
|---|---|
| FC-GATE | Already Done on main; preserve build/vet/race and known-red support. |
| FC-SCAFFOLD | Reopened for F1–F4; scheduler, sampler and CLI have later dedicated scaffolds/seals. Rules remain distinct from measured counts. |
| FC-1 | FC-JOURNAL, FC-SOURCES, bounded FC-1 integration, FC-OBS-ADJ; all nine original edge cases retained below. |
| FC-2A / FC-2B | Independent bodies after FC-SCHED-SCAFFOLD/SEALS, full differential trace ruled by FC-SCHED-ADJ. Sampling moved to FC-3; cap explanation corrected in F5. |
| FC-3 | FC-SAMPLE-SCAFFOLD/SEALS → FC-3 → FC-SAMPLE-ADJ. Joint empirical samples replace independent round multiplication and thin-cell log-normal fallback (F4). |
| FC-4 | FC-PREDICT-SCAFFOLD/SEALS → FC-4. Preserve tasks/reference/concurrency/iterations CLI, p50/p80/p95 time and available cost, row-level execution explanation and limitations. |
| FC-ADJ | Terminal integrated proof, predeclared dogfood-go/model-matrix training and wallet holdout, honest upper-p80 outcome with no tuning or leakage. |
| YT-SCAFFOLD | Neutral DTO/capability contract and stubs. YT-JIRA implements the existing-provider adapter separately; compile-time satisfaction applies to the adapter rather than requiring a concrete client rewrite. |
| YT-SEALS | Original argv, equivalence and fail-closed seals, plus dedicated later wiring/command seals. New behavior red; existing Jira regression green and mutation-proven (explicit amendment). |
| YT-CONFIG | One resolver, legacy no-kind Jira default and unknown-prefix fallback, distinct YouTrack settings, malformed kind error. |
| YT-READ | YT-TRANSPORT + YT-HISTORY + YT-READ, checked by YT-ADAPTER-ADJ. Native queries, pagination, field-by-field Item equivalence, named custom fields, cycle-time/closed-by, empty-vs-auth distinction. |
| YT-WRITE | YT-WRITE + YT-TRANSITION after transport; readable key, Markdown, complete optional-field matrix, caller status mapping and explicit replay/crash-window guarantees. |
| YT-WIRE | Dedicated scaffold/seals, CLI/factory body plus YT-WIRE-WEB, then YT-WIRE-ADJ. Refresh actual constructor inventory; one factory exception replaces contradictory zero-outside requirement. |
| YT-CMD | Dedicated scaffold/seals/body/adjudication; exact legacy argv/output retained, neutral create and transition, explicit refusal under jira for YouTrack. |
| YT-ADJ | All deviations ruled, real create/read/history forecast, full gate, precise external bridge requirements; remains externally Blocked until prerequisites exist. |

All original agent/model/effort pins remain attached to their original keys.
New rows do not replace or relabel past cost/review evidence.

## FC-1 original edge cases

| Case | Contract and proof owner |
|---|---|
| Started without completion | F2; FC-JOURNAL censoring seals, never a completed duration. |
| Blocked elapsed | F2/F4; journal/reference and sampling seals, mutation must fail if included in duration mean. |
| Cascade versus authored pin | F1; actual final stamp with cascade disclosure, no pin substitution. |
| Repeated revisions/conflicting stamps | F1; same run/key/start dedupe, separate runs retained, authoritative conflict counted. |
| Zero-observation cell | F4; present n=0, coverage reported, prediction refused. |
| Missing stamped model | F1/F3; absent-stamp disposition, no guessed attribution. |
| Journal with no recoverable YAML | F3; match-based counters and unmatched sibling attempts. |
| UTC offsets | F1; exact instant normalization, no nearest-start match. |
| Hand-finished rows | F2/F4; limitation populated by Build and rendered, not just a formatter fixture. |

FC-OBS-ADJ retains the old assignment's >=80% statement-coverage requirement
for the observation/extraction package, measured on the corrected head. It is
additional evidence, not a substitute for behavioral seals or mutation proof.
All source errors remain typed/wrapped for errors.Is classification.

## FC-1 panel: all 19 findings

Source: `../dispatcher-runs/2026-09-04T22-34-59Z-FC-1-panel/panel.json`,
reviewed head `d529265c5e2d510657feb13a701b2b622daeb48e`. IDs below are family and
one-based finding order. Final dispositions belong in FC-OBS-ADJ's note.

| ID | Severity / finding | Required owner and proof |
|---|---|---|
| Claude-1 | High: overlapping development/review | FC-JOURNAL; production event ordering, disjoint intervals bounded by elapsed (F2). |
| Claude-2 | Medium: simplified Git history loses side branch | FC-SOURCES; superseded merge fixture, full reachable history (F3). |
| Claude-3 | Medium: empty target passes gates | FC-1; each coverage gate rejects zero target rows (F4). |
| Claude-4 | Medium: inherited ingest comments/attachments replay | Separate ingest follow-up below; no claim fixed by these epics. |
| Claude-5 | Medium: all blobs buffered | FC-SOURCES; streamed/capped bytes and cancellation (F3). |
| Claude-6 | Low: redundant target-cell counter and stale prose | FC-1; report matches validated target states (F4). |
| Claude-7 | Low: evidence tie order-dependent | FC-1; evidence permutations and deterministic authority ties (F1). |
| Claude-8 | Low: personal source-repository default | FC-1; explicit repeatable repositories, updated command docs (F3). |
| Codex-1 | High: attempt identity omits run ID | FC-SCAFFOLD/SEALS/FC-1; distinct-run same-start regression (F1). |
| Codex-2 | Medium: stale YAML marked recoverable too early | FC-1; no recovery credit until exact unambiguous match (F3). |
| Codex-3 | Medium: hand-finished test only checks formatter | FC-SEALS/FC-1; Build-to-report behavior and mutation (F2/F4). |
| Grok-1 | High: journal discovery silently empty | FC-SOURCES/FC-1; missing/unreadable/empty source refusals (F3). |
| Grok-2 | High: single-source default and stale brief | FC-1; explicit source manifest; old brief replaced by authority pointer. |
| Grok-3 | Medium: shallow/grafted history appears complete | FC-SOURCES; partial-state detection and gate refusal (F3/F4). |
| Grok-4 | Medium: inherited Git environment redirects source | FC-SOURCES; adversarial environment fixture (F3). |
| Grok-5 | Medium: diff/blob bytes unbounded, partial progress lost | FC-SOURCES; bounded streaming and explicit error/partial status. Exact diagnostic persistence behavior frozen in scaffold (F3). |
| Grok-6 | Medium: YAML with no journal disappears | FC-1; named no-run/key disposition and reconciliation counts (F3). |
| Grok-7 | Low: two flags absent from registration assertion | FC-SEALS; timeout and fail-on-uncovered-required argv assertions. |
| Grok-8 | Low: data errors print usage | FC-1; parsed CLI data-error fixture, no Cobra usage. |

Claude-4 concerns existing `forecast ingest --apply`, inherited into the FC
review diff. Track it as `INGEST-REPLAY`: deterministic action identity,
partial-failure rerun, comment and attachment replay proof, and explicit remote
success/local-ledger crash semantics before claiming retry safety. It belongs
to a separate ingest contract/seals/bodies review, not a forecasting or tracker
read refactor. It remains open; YT create replay work does not resolve it.

## YouTrack constraints checked at each handoff

- Exact bridge-generated create AND transition argv plus readable-key parser
  round trip; do not rename persisted jira_key in another repository.
- No ADF in neutral types or YouTrack code; Markdown preserved end to end.
- All seals offline, no permanent token, fixture provenance honest. Original
  recorded-response requirement remains a live-acceptance obligation: synthetic
  or documentation-derived fixtures are labeled as such and checked against
  real sanitized responses during YT-ADJ, never mislabeled as recordings.
- Full optional-field preflight before mutation; error names unsupported field.
- Existing Jira output, config, Item/cycle-time behavior preserved.
- Shared provider factory used by every inventoried CLI and web site.
- Successful-key replay distinct from ambiguous remote success; no unproven
  exactly-once claim, no automatic fresh-create retry after uncertain outcome.
- All body deviations explicitly accepted with reason or corrected under seals;
  live proof and required external bridge changes cannot be waived for Done.
