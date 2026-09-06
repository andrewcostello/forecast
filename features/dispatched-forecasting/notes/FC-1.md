# FC-1: corrected reconciliation and coverage body

## Contract section

Implemented the frozen F1–F4 seams owned by FC-1:

- `JoinEvidence` validates the selected journal universe, rejects duplicate or
  held-out attempt sets, joins only exact canonical `AttemptID` values, keeps
  all YAML citations, and emits a deterministic disposition for every reading.
  Multiple revisions remain one joint attempt; distinct runs and unmatched
  restart siblings remain separate. Journal terminal tuples outrank YAML,
  YAML-only terminals are cited and rebase wall bounds atomically, and
  equal-authority role/terminal conflicts are excluded with portable evidence.
- amended `Build` resolves one UTC cutoff, invokes `ReadSources`, reduces each
  selected journal, reconciles through `JoinEvidence`, and emits schema 4 with
  a complete manifest/evidence audit. Source, reducer, and join failures retain
  a diagnostic artifact and mark only the aggregate manifest PARTIAL. The flat
  observations/cells are compatibility projections of joint attempts, not an
  independent-max merge.
- `PredictionEligibility` validates original target rows and structured joint
  records. It requires exact schema 4, a COMPLETE manifest, no held-out or
  cutoff-inconsistent samples, and the effective completed-sample threshold in
  every required cell. Empty target cells remain visible at `n=0`.
- the CLI removes the home-directory repository default, maps its legacy flags
  into explicit journal/live/history sources and bounds, supports repeatable
  repository-relative `--task-root` directories, writes coverage-only
  diagnostics before refusing a gate, and suppresses Cobra usage for data
  errors. The schema-4 report prints actual manifest sources, reasons,
  dispositions, lost attempts, ambiguity, and conflicts instead of stale
  unreachable schema-3 claims.

Starting HEAD was `d7abce0`. The exact implementation commit is recorded by
the dispatcher handoff because a commit cannot embed its own hash. The requested
commit was attempted with subject
`feat(dispatched): reconcile corrected evidence coverage [FC-1]`, but Git could
not create the shared worktree `index.lock` on the read-only parent metadata;
all changes remain in the working tree for the dispatcher to commit.

## Reason and affected callers

This replaces only the FC-1 `ErrNotImplemented` seams and connects the already
landed FC-JOURNAL and FC-SOURCES bodies. Legacy `BuildOptions` continue through
the schema-3 path; amended and mixed options cannot be silently confused.
Downstream forecasting consumes `Artifact.Evidence.Observations` and calls
`PredictionEligibility`; it does not infer completeness from legacy counters.

## Real coverage evidence

The rebuilt artifact is
`/home/andrew/Project/dispatcher-runs/2026-09-05T00-59-44Z-forecasting-v2/FC-1/coverage.json`.
The successful selection used cutoff `2026-09-06T04:49:49.076639495Z` and:

- journals: `/home/andrew/Project/dispatcher-runs`;
- repository: `/home/andrew/Project/claude-workflow`;
- YAML roots: `features/dogfood-go` and `features/model-matrix`, the minimal
  containing directories for the six files in the existence-only inventory;
- diagnostic target: this repository's
  `features/dispatched-forecasting/tasks.yaml`;
- no holdout IDs. The wallet target and its outcomes were not opened.

All three source reports are COMPLETE: 103 journals / 11,545 retained journal
events, 112 live YAML records, and 3,085 historical records from 319 commits and
28 unique blobs. The aggregate is honestly PARTIAL because 32 readings of
`GO-1-1` in run `2026-08-31T23-54-47Z-tasks` provide contradictory equal-rank
terminal evidence. The audit reports 303 attempts across 289 run/task rows, 44
recovered attempts, and 259 individually retained lost attempts. Of 3,197 YAML
readings, 44 are recovered envelopes, 329 duplicate readings, 2,561 lack join
keys, 197 have no exact start, 34 match attempts without a stamped model, and
32 are conflicting evidence.

For the 20-row diagnostic target, five of seven required cells are empty and
all seven are below `min_completed=2`; coverage is 0/20 target rows. This is a
coverage result, not a prediction or a wallet holdout evaluation.

The first real-data invocation used the six inventory file paths as roots and
failed closed with `ErrSourceMissing` because the frozen source contract accepts
directory roots. No coverage claim is based on that failed selection; the
artifact above was overwritten by the successful minimal-directory run.

## scaffold_review_followups

| Finding | Disposition |
|---|---|
| `claude-2` | **Not adopted, consistent with FC-JOURNAL.** Caller-supplied `Inferred=true` intervals remain valid under the accepted scaffold. Schema-4 limits disclose that legacy phase projections cannot express availability; eligibility uses total elapsed from the structured attempt. Adding a citation requirement or excluding inferred spans would amend the frozen contract. |
| `claude-6` | **Documentation only, no reinterpretation.** Build consumes reducer output without double-counting parser facts already represented by source reports. Reducer/join errors become aggregate manifest reasons; boolean/list diagnostics remain idempotent carriers. |
| `claude-7` | **Adopted in the FC-1 projection.** `Rounds` maps only from `Attempt.Corrections`, and `Artifact.Limits` explicitly states that schema-4 rounds are correction events rather than review invocation count. CLI documentation now describes only the amended manifest path; baseline readers remain compatibility code. |

## Inherited findings disposition

- FC-JOURNAL's nil top-level error for terminal conflicts is not reproduced:
  amended Build treats any retained `AttemptSet.Conflicts` as
  `ErrEvidenceConflict`, marks the aggregate PARTIAL, and preserves the
  conflicts in the artifact.
- The direct-`ParsedJournal` physical tie-order, marker-deficit completeness,
  total-cap truncation, underlying I/O wrapping, and quadratic wall-component
  findings remain in FC-JOURNAL-owned code. Normal CLI input has physical line
  identities, but these remain adjudication limitations rather than claims
  silently fixed here.
- FC-SOURCES' unreadable held-out journal diagnostic remains conservative and
  fail-closed. FC-1 neither relabels that failure nor manufactures discovery
  from the inventory.

## Seal and regression evidence

- `go test ./internal/dispatched -run '^TestFCEvidenceContract$' -race -count=1` — pass.
- `go test ./cmd/forecast -run '^TestFCReferenceCLIContract$' -race -count=1` — pass.
- `go build ./...` — pass.
- `go vet ./...` — pass.
- `go test ./internal/dispatched -count=1` — pass.
- `env -u DISPATCHER_KNOWN_RED_FILE go test ./... -race -count=1` — fail only
  in two pre-correction CLI regressions described under Deviation; all other
  packages and both owned seal groups pass.

## Deviation

No implementation signature or accepted F1–F4 behavior was changed. Full-suite
completion is blocked by an immutable regression/contract conflict:
`TestDispatchedReferenceBuildRunEWiresFlagsToTheBuilder` and
`TestBuildCommandAcceptsRepeatedSourceRepositories` construct journals without
the mandatory `run_started.payload.dispatcher_version` declaration, yet require
successful recovered observations. The authoritative corrected contract and
FC-JOURNAL body require missing producer evidence to remain PARTIAL and make
`ReduceAttempts` return `ErrJournalSource`; FC-1 cannot infer version `0.1.0`
without violating that rule. Editing the fixtures is outside body ownership,
and adding a producer guess or schema-3 fallback would silently weaken the
accepted contract. This requires adjudication or a seals-owned fixture update.

## Residual limitation

The real artifact is prediction-ineligible because of its named conflict and
thin/empty target cells. It must remain a diagnostic coverage artifact. No
wallet holdout, network, credentials, external messages, subagents, push, test
fixture edits, known-red edits, or unowned source edits were used.

## Operator completion evidence after independent fixture repair

The Blocked and uncommitted statements above describe the historical native
attempt, not the current checkout. The dispatcher saved the implementation as
`ab165c7bb490baa321641b4961da2ec33bae98f3`. Operator seal commit
`820dbe12d0c0f3af194d0ef046d9f1530ddd3d55` adds only an explicit synthetic
dispatcher 0.1.0 producer declaration to the successful CLI fixture, preserving
every prior task event, measurement and assertion. No implementation changed.

The full build/vet/race gate now passes without exclusions. All four observation
contract groups pass with IPv4/IPv6 networking denied and credential environment
removed (214 cases, no failures, skips, panics or races). Removing only the new
producer header makes both original positive CLI assertions fail with unknown
producer refusal; the fixture was restored afterward. The independent missing-
producer refusal cases remain active and unchanged. Exact evidence is at
`/home/andrew/Project/dispatcher-runs/2026-09-06T04-57-54Z-FC-1-resolution`.

The real coverage artifact and its conflict/insufficient-cell limitations remain
as recorded above. Source and Journal residual findings still require explicit
FC-OBS-ADJ dispositions. This resolves the fixture ownership block; runtime Done
still requires the separate verifier and pinned-head cross-family panel.

## Corrective F1/F4 body after operator panel ruling

The authoritative 2026-09-06 Operator F1/F4 ruling was implemented without
changing tests, fixtures, public signatures, source-selection flags, worklists,
or known-red registrations. `JoinEvidence` now retains the exact classified
reading, indexes run/key misses, treats absent role as unknown, classifies a
nonempty invalid role as malformed, refuses repeated sets and duplicate attempt
categories without requiring a set for every discovered journal, and copies
ambiguous refs before canonical sorting. Repeated identical compatible reading
citations remain legal and each retains one disposition.

Build preserves EMPTY, records early source and pre-join cancellation reasons,
and never projects a malformed wall breakdown as zero phases. Eligibility now
checks recovered reading/field citations, journal/attempt agreement, YAML
terminal/elapsed pairing, wall interval invariants, evidence counters and audit
arrays. Legitimate lost/no-YAML attempts and unavailable optional cost, token,
or phase measurements remain valid. The CLI keeps its early missing-`--tasks`
misuse guard and adds the ruled defensive post-report helper refusal.

The previously recorded real corpus report is historical diagnostic evidence:
it remains PARTIAL with 44 of 303 attempts recovered. Those are not post-fix
counts, not a data-sufficiency claim, and not a completed holdout evaluation.
No wallet or live-data rescan was performed for this correction, and all prior
source-report limitations above remain in force.

### First-body-panel final dispositions

| Finding ID | Final disposition |
|---|---|
| `Claude-1` | **Corrected.** Empty role is unknown; a valid sibling supplies role and its exact citation. |
| `Claude-2` | **Corrected.** Build preserves `SourceEmpty`; EMPTY remains prediction-ineligible. |
| `Claude-3` | **Corrected.** Early source errors add canonical aggregate source reasons and PARTIAL state. |
| `Claude-4` | **Corrected.** Exact classified readings are retained and run/key misses use an index; no public context parameter was added. |
| `Claude-5` | **Corrected.** Limits name uncomputed schema-4 legacy Coverage fields and the authoritative evidence/source equivalents or unavailable fields. |
| `Claude-6` | **Corrected.** `SummarizeWall` invariants are enforced before eligibility/projection; malformed intervals cannot fabricate zero phases. |
| `Claude-7` | **Corrected.** Construction-time `SilenceUsage` was removed; RunE still suppresses usage for data errors. |
| `Claude-8` | **Corrected.** Legacy `Artifact.Conflicts` is documented as compatibility-only; `Evidence.Conflicts` is authoritative. |
| `Claude-9` | **Deferred as ruled.** No cutoff/holdout/allow-empty/ref CLI flags were added; downstream scaffold owns that surface. |
| `Codex-1` | **Corrected/controlled.** The early CLI missing-target refusal remains; the direct helper writes diagnostics then returns `ErrEmptyTarget` and `ErrNotEligible`. |
| `Codex-2` | **Corrected.** Structured provenance, counters, dispositions, conflict/ambiguity state, and recovered audit links are validated. |
| `Codex-3` | **Corrected to ruling.** Repeated sets and duplicate categories are refused; missing selected sets remain legal diagnostic input and distinct conflict facts are retained. |
| `Codex-4` | **Corrected.** Reconciliation consumes the exact reading that classification accepted, independent of caller permutation. |
| `Codex-5` | **Corrected.** Unknown role may coexist with valid role; nonempty invalid role is malformed and never authoritative conflict evidence. |
| `Codex-6` | **False positive controlled plus gap corrected.** Existing nested interval copying remains; repeated joins stay immutable, and ambiguous refs now receive their own copy before sorting. |
| `Codex-7` | **Corrected.** Coverage-gate documentation now includes target and COMPLETE-source prerequisites. |
| `Grok-1` | **False positive, no relaxation.** Nil manifest validation was already safe; eligibility now also emits an explicit missing-manifest reason. |
| `Grok-2` | **Corrected.** Exact reading association and indexed run/key lookup remove the observed quadratic rescans. |
| `Grok-3` | **Deferred as ruled.** Reproducibility convenience flags remain owned by FC-PREDICT-SCAFFOLD/FC-4. |
| `Grok-4` | **False positive.** `OutcomeDone` remains `iota+1`; zero remains invalid. |
| `Grok-5` | **Corrected.** The report labels the aggregate as `not-recovered attempts`, not unmatched siblings. |

### Corrective verification evidence

- `go test ./internal/dispatched -run '^TestFCEvidenceContract$' -count=1` — pass.
- `go test ./cmd/forecast -run '^TestFCReferenceCLIContract$' -count=1` — pass.
- `go test ./internal/dispatched ./cmd/forecast -count=1` — pass.
- `go test ./... -count=1` — pass.
- `go build ./...` — pass.
- `go vet ./...` — pass.
- `go test ./... -race -count=1` — pass, all packages, no exclusions.

No residual implementation deviation from the operator F1/F4 ruling is known.
FC-1 remains Blocked pending the operator exact-head gate, independent verifier,
and cross-family panel; this body does not mark runtime work Done.

## Operator F4 cutoff-integrity follow-up

The parent findings against `a4736d75` are corrected at the existing schema-4
eligibility boundary. Every recovered `ReadingRef.RecordedAt`, and every known
`CompletedAt` on a Recovered or DuplicateReading audit envelope, must be at or
before the manifest cutoff. Every terminal attempt, including a YAML-terminal
attempt, must end at or before cutoff. An unfinished attempt must have elapsed
exactly from its start through cutoff, with the existing parent/wall alignment
checks still applied. Equality is accepted. Future timestamps on correctly
excluded AfterCutoff audit records remain valid diagnostic proof and are not
treated as samples.

### Parent-finding dispositions

| Finding | Final disposition |
|---|---|
| Recovered `ReadingRef.RecordedAt` after cutoff was eligible | **Corrected.** All recovered reading citations are now cutoff-bounded; exact-cutoff citations remain valid. |
| YAML terminal after cutoff was eligible | **Corrected.** All terminal attempts are cutoff-bounded, including YAML terminal/elapsed pairs; exact-cutoff terminals remain valid. |
| Recovered/DuplicateReading known completion proof after cutoff | **Corrected.** Both audit dispositions validate known `CompletedAt` against cutoff. |
| Unfinished elapsed shorter or longer than cutoff | **Corrected.** Unfinished elapsed must end exactly at cutoff while `Wall.Elapsed` remains aligned. |
| Proper AfterCutoff diagnostic with future proof | **Preserved.** The cutoff checks apply only to recovered sample proof; excluded future evidence remains legal. |
| Optional unknowns and legitimate losses | **Preserved.** No new optional-measurement or source-completeness requirement was introduced. |

### Follow-up verification

- Baseline on `dd9681e`: `F4-ARTIFACT-CUTOFF-PROOF` failed only the six
  expected invalid cases; its four equality/unfinished/AfterCutoff controls
  remained green.
- `go test ./internal/dispatched -run '^TestFCEvidenceContract$/F4-ARTIFACT-CUTOFF-PROOF' -count=1` — pass.
- `go test ./internal/dispatched -run '^TestFCEvidenceContract$' -count=1` — pass.
- `go build ./...` — pass.
- `go vet ./...` — pass.
- `go test ./... -race -count=1` — pass, all packages, no exclusions.

No residual implementation deviation from the Operator F4 cutoff-integrity
follow-up is known. The historical real corpus report remains diagnostic
PARTIAL at 44/303 recovered; it was not rescanned and is not a post-fix count,
data-sufficiency claim, or completed holdout evaluation. FC-1 remains Blocked
pending the parent exact-head gate, independent verifier, and full panel.

## Second-panel corrective body

The five High findings from the panel on `42c80ae` are corrected together at
the schema-4 reconciliation and eligibility boundary. Recovered and duplicate
audit envelopes now carry a complete run/key/start identity equal to their UTC
`AttemptID`; every citation occurrence has a disposition, and every same-ref
envelope remains indexed. A YAML terminal is supported when at least one
corresponding envelope has known `CompletedAt` equal to `TerminalAt`; compatible
duplicates with unknown completion remain legal and cannot overwrite that
support. The earlier recovered-reading, terminal, completion, and unfinished
cutoff checks remain in force, with equality allowed and AfterCutoff diagnostics
still excluded from sampling rather than rejected.

Recovered YAML citations now resolve structurally to the selected live/history
source, exact declared repository, source kind and declared root. Journal
citations resolve to the selected journals source, the direct-child
`run/journal.jsonl` layout and producer `0.1.0`. Stored paths are checked as
portable relative paths without filesystem access; legal spaces remain valid,
and a history reading may cite an ancestor rather than a captured tip. This is
source-namespace and carried-evidence integrity validation, not cryptographic
authentication, raw-event recomputation, source reopening, or a new corpus
completeness definition.

FC-1-private comparators retain `ReadingRef` as the primary order and then order
all reconciliation-relevant reading content. Portable `Examined` ties are
ordered by identity, optional completion, attempt, disposition and reason.
Compatible identical references remain distinct evidence with deterministic
Recovered/DuplicateReading assignments. Eligibility also refuses known
non-finite or negative cost, known negative token totals, and negative
correction, cascade, review or verification counts through the existing invalid
payload sentinels. Unknown optional quantities, known zero, and valid positive
measurements remain eligible without inventing citation/count equalities.

The CLI now validates a nonpositive timeout before enabling RunE usage
suppression. Unknown or parse-invalid flags and invalid timeouts therefore keep
usage output, while missing `--tasks` and source/coverage data refusals retain
the established silent behavior.

### Second-body-panel final dispositions

These IDs are from the second panel only and are separate from the first-panel
table above.

| Finding ID | Final disposition |
|---|---|
| `Claude-1` | **Corrected.** Known cost is finite/nonnegative, known token totals are nonnegative, and all four recorded attempt counts are nonnegative; unknown optionals remain allowed. |
| `Claude-2` | **Corrected.** Recovered YAML and journal citations bind to selected source kind, exact repository where represented, portable declared paths, and supported producer without I/O. |
| `Claude-3` | **Accepted boundary, not implemented.** The private legacy projection remains all-or-nothing; validated Build input cannot send malformed walls to it, and defensive failure leaves the artifact PARTIAL and ineligible. |
| `Claude-4` | **Corrected.** Every recovered/duplicate envelope identity binds to its UTC attempt and holdout selection; all same-ref envelopes are indexed for exact YAML completion support. |
| `Claude-5` | **Corrected.** Missing-task errors still name `--tasks` without usage; flag/timeout misuse retains usage and data errors remain silent. |
| `Codex-1` | **Corrected.** Complete audit identity and known matching YAML-terminal completion proof are required without treating unknown compatible duplicates as conflicts. |
| `Codex-2` | **Corrected.** Selected source namespace, repository, root, revision kind, journal layout, and producer are structurally bound; history ancestors remain valid. |
| `Codex-3` | **Corrected.** Matched same-ref readings and every portable Examined tie field now have a semantic total canonical order independent of caller permutation. |
| `Grok-1` | **Deferred as ruled.** Direct malformed normalized `AttemptSet` salvage is not added; `JoinEvidence` remains fail-closed while Build retains reducer-produced usable attempts and aggregate diagnostics. |
| `Grok-2` | **Deferred as ruled.** A bounded target snapshot/read contract remains FC-PREDICT-SCAFFOLD/FC-4 work; no target cap or new CLI surface was added here. |
| `Grok-3` | **Corrected.** Nonpositive timeout is rejected before RunE suppresses usage; the existing unknown-flag and data-error behaviors are preserved. |

### Second-panel corrective verification

- `go test ./internal/dispatched -run '^TestFCEvidenceContract$' -count=1` — pass.
- `go test ./cmd/forecast -run '^TestFCReferenceCLIContract$' -count=1` — pass.
- `go test ./internal/dispatched -count=1` — pass.
- `go test ./cmd/forecast -count=1` — pass.
- `go build ./...` — pass.
- `go vet ./...` — pass.
- `go test ./... -race -count=1` — pass, all packages, no exclusions.

No ruled second-panel body deviation is known. The accepted/deferred projection,
direct-set salvage and bounded-target items above are explicit future boundaries,
not hidden acceptance. The historical real corpus report remains diagnostic
PARTIAL at 44/303 recovered; it was not rescanned, is not a post-fix count or a
data-sufficiency claim, and is not a completed holdout evaluation. FC-1 remains
Blocked pending the parent exact-head gate, independent verifier and full panel;
this body does not mark runtime work Done.

## Direct-child journal-layout follow-up

The narrow parent finding after the second-panel correction is fixed within the
existing selected-provenance rule. A recovered journal `RunID` must now be one
non-dot portable path component before its stored path can match
`{RunID}/journal.jsonl`. Thus `parent/run-a` cannot masquerade as one selected
direct-child run, and `.` cannot be cleaned away to `journal.jsonl`. Ordinary
`run-a` and a legal internal-space component such as `run a` remain valid. The
selected source, supported producer, exact layout and all prior carried-evidence
checks remain unchanged; no new source policy or filesystem access was added.

Independent seal commit `6d9754abc980533f95bcd4ddf7d2cdc7c44147b0`
faithfully relinks complete positive artifacts without changing observation
counts, models, outcomes or thresholds. Before this body change, the focused
Evidence group failed only the new `nested-run-id` and `dot-cleaned-run-id`
leaves; the ordinary-direct-child and legal-internal-space controls passed. No
seal or existing assertion conflicted with the ruling.

### Direct-child follow-up verification

- `go test ./internal/dispatched -run '^TestFCEvidenceContract$' -count=1` — pass.
- `go test ./cmd/forecast -run '^TestFCReferenceCLIContract$' -count=1` — pass.
- `go build ./...` — pass.
- `go vet ./...` — pass.
- `go test ./... -race -count=1` — pass, all packages, no exclusions.
- `git diff --check` — pass.

Tests, fixtures, seals, Source/Journal implementations, scaffold, known-red and
worklists were not edited. Every second-panel correction and the historical
diagnostic PARTIAL 44/303 limitation remain as recorded above. FC-1 remains
Blocked pending the parent exact-head gate, independent verifier and panel; this
follow-up does not mark runtime work Done.

## Third-panel corrective body

The two High findings from the panel on `12a0e11` are corrected under the final
Operator F1/F4 third-panel ruling. Portable stored citation paths, journal run
components and declared-root matching now reject an ASCII letter followed by a
colon at the start, independent of the build host. This covers upper/lowercase
drive-absolute and drive-relative spellings. Existing absolute, backslash,
traversal, dot and nested-run refusals remain, while ordinary relative paths,
legal spaces and non-drive colons such as `features/archive:notes/tasks.yaml`
remain valid. No SourceSpec/ReadSources behavior, filesystem lookup or general
Windows filename policy was added.

An FC-1-private total comparator now governs both `EvidenceJoin.Conflicts` sort
sites. It preserves `attemptConflictLess` as the primary order and then compares
the complete A and B citations, both typed values, `Code` and `Reason`; only the
non-serialized `Err` is excluded. Different conflict facts for one identity
remain paired, retained and counted as one attempt denominator entry. No stable
input-order fallback, candidate swap, conflict deletion or caller mutation is
used.

Schema-4 `Artifact.Limits` now states the accepted carried-value boundary:
eligibility validates internal consistency and citations but does not re-derive
role or other values from original snapshots/events and does not authenticate
source bytes. Sources are not refetched, and neither Examined nor the public
schema was expanded.

### Third-body-panel final dispositions

These IDs belong only to the third panel and do not replace either earlier
panel-disposition table.

| Finding ID | Final disposition |
|---|---|
| `Claude-1` | **Corrected.** Both conflict output sorts use an FC-1-private total comparator over every serialized conflict field while retaining all distinct facts and pairings. |
| `Claude-2` | **Accepted nonblocking boundary.** Current event-citation fields remain explicitly enumerated; no reflection, hypothetical-field requirement or shared production/fixture enumerator was introduced. Future schema fields must amend their owning scaffold and seals. |
| `Claude-3` | **Disclosed as ruled.** Artifact Limits and these notes state that carried-value/citation consistency does not re-prove original role/event values or authenticate source bytes. |
| `Claude-4` | **Deferred nonblocking.** Validator factoring is a maintainability suggestion, not a behavior requirement; the tested indices and invariants remain intact without a broad refactor. |
| `Claude-5` | **Accepted with reason.** The shipped CLI is one-shot; repeated execution of a mutated Cobra singleton is not a frozen embedding promise, and no incomplete RunE-only reset was added. Fresh-constructor and data-refusal behavior remains tested. |
| `Codex-1` | **Corrected.** Host-independent ASCII drive prefixes are refused in stored YAML paths, journal run components and root interpretation, including absolute and drive-relative forms. |
| `Grok-1` | **Corrected under the parent-confirmed ruling.** Drive-qualified provenance can no longer become eligible merely because the current host treats it as relative; legal non-drive colons remain allowed. |
| `Grok-2` | **Accepted with reason.** No repeated-singleton Execute contract or partial state reset was invented; callers needing independent invocations use fresh constructors. |

### Third-panel corrective verification

On seal commit `ad255c937833bc03419486dfb993db5b0fe3ca81`, the
focused Evidence baseline failed only the eight drive-prefix leaves and ten
conflict-order permutation leaves recorded by the independent seals. The
field-primary conflict controls, legal colon/space/path controls and complete CLI
group remained green; no immutable assertion contradicted the ruling.

- `go test ./internal/dispatched -run '^TestFCEvidenceContract$' -count=1` — pass.
- `go test ./cmd/forecast -run '^TestFCReferenceCLIContract$' -count=1` — pass.
- `go test ./internal/dispatched -count=1` — pass.
- `go test ./cmd/forecast -count=1` — pass.
- `go build ./...` — pass.
- `go vet ./...` — pass.
- `go test ./... -race -count=1` — pass, all packages, no exclusions.
- `git diff --check` — pass.

No ruled third-panel body deviation is known. Tests, fixtures, seals, schema,
Source/Journal/shared helpers, scaffold, known-red and worklists were not edited.
The historical real corpus report remains diagnostic PARTIAL at 44/303
recovered; it was not rescanned and is not a post-fix count, holdout evaluation
or data-sufficiency claim. FC-1 remains Blocked pending the parent exact-head
gate, independent verifier and isolated panel; this body does not mark runtime
work Done.

## Fourth-panel corrective body

The final Operator F1/F4 fourth-panel ruling supersedes the earlier mistaken
count/list exemption. In particular, the second-panel sentence that positive
measurements remain eligible “without inventing citation/count equalities” is
historical and no longer current where the frozen `Attempt` schema already
declares complete counted-event lists and least-event citations. The former
known-zero-without-contributors and weighted one-event-for-count-2/3 fixture
interpretations are likewise superseded. The independent fixture repair keeps
the same values, IDs, models, outcomes and thresholds while supplying the
proof the Journal contract always required.

Eligibility now checks all carried measurement/list relationships. Corrections,
cascades, reviews and verifications equal their complete unique canonical list
lengths; zero uses `EvidenceNone`, and nonzero cites list element zero with
`EvidenceJournal`. Known cost and token totals, including known zero, require a
nonempty contributor list and its least-event journal citation. Unknown totals
remain unknown and may retain available partial contributor lists with
`EvidenceNone`. Each list uses the selected attempt journal, declared event
types, positive physical lines and nonzero timestamps at or before cutoff.
`CostScope` remains `recorded_task_spawns`. Events may appear in multiple
measurement lists; no payload sums, spawn kinds, source bytes or stricter
terminal-clock bounds are inferred.

Ambiguous diagnostic refs are copied, UTC-canonicalized and sorted with an
FC-1-private total comparator. All refs and `Starts` remain, inputs are not
mutated, the identity stays lost/ambiguous, and no diagnostic is admitted as a
sample. The existing total conflict comparator and both conflict-output sorts
remain unchanged and are exercised by the independent per-field and
post-reconciliation cases.

Private YAML and journal provenance validators now return field-, value- and
rule-specific reasons for selected-source, repository, revision, path/root,
cutoff, producer and journal-layout failures. Refusal still uses the existing
`ErrNotEligible` plus `ErrSourceIncomplete` gate path and never opens the cited
source. The portable root helper also documents why raw drive/absolute/backslash
guards precede cleaning: cleaning `C:/..` can erase the forbidden drive prefix
and produce `.`. Drive-dotdot remains refused while ordinary `a/..` and `.` roots
retain their ruled behavior.

### Fourth-body-panel final dispositions

These IDs are specific to the fourth panel. Earlier tables remain historical;
the count/list mistake is expressly superseded above.

| Finding ID | Final disposition |
|---|---|
| `Claude-1` | **Corrected.** Carried counts, complete unique canonical event lists, least citations, known/unknown totals, event domains, cutoff, selected journal and cost scope now enforce the frozen Attempt contract. |
| `Claude-2` | **Corrected.** Ambiguous refs are privately copied, UTC-canonicalized and totally sorted without changing Starts, lost status, denominator or caller inputs. |
| `Claude-3` | **Corrected.** YAML/journal refusals now name meaningful offending fields, stored values and structural rules while retaining both typed gate sentinels and no source I/O. |
| `Claude-4` | **Corrected/documented.** Raw nonportable-root guards explicitly precede cleaning; drive-dotdot refuses and the ordinary cleaned `a/..` and `.` controls remain valid. |
| `Codex-1` | **Preserved and independently strengthened.** The total conflict comparator remains at both output sorts; one-field ties and reconciliation-appended conflicts pass permutation, retention, pairing and immutability seals. Replacing only the final comparator proves total ordering is required there; removing only the final sort separately proves merge ordering. Neither mutation alone proves the other, and no second valid Code or exhaustive-branch claim is invented. |
| `Grok` | **No findings / approve.** No corrective disposition beyond preserving all ruled behavior. |

### Fourth-panel corrective verification

On independent seal commit `99724ab87b5db42a1b615964404c2ad0ab8df3de`,
the uncorrected body failed 42 new leaves: 35 measurement-integrity cases, one
ambiguous whole-output permutation case and six actionable-provenance-reason
cases. The repaired known-zero/positive fixtures, unknown-partial-list controls,
conflict cases, root controls and complete CLI group were green. No immutable
assertion contradicted the corrected ruling.

- `go test ./internal/dispatched -run '^TestFCEvidenceContract$' -count=1` — pass.
- `go test ./cmd/forecast -run '^TestFCReferenceCLIContract$' -count=1` — pass.
- `go build ./...` — pass.
- `go vet ./...` — pass.
- `go test ./... -race -count=1` — pass, all packages, no exclusions.
- `git diff --check` — pass.

The gate proves internal carried-evidence consistency, not raw-payload sum
recomputation, finer spawn-kind authentication, original source-value truth or
cryptographic source authenticity. Existing future-field/schema,
validator-factoring and one-shot CLI boundaries remain disclosed; no refactor,
reflection or repeated-singleton execution contract was added. Tests, fixtures,
seals, schema, Source/Journal/shared helpers, scaffold, known-red and worklists
were not edited by this body.

The real coverage report remains the historical diagnostic PARTIAL result of 44
recovered attempts out of 303. It was not rescanned and is not a post-fix count,
holdout evaluation or data-sufficiency claim. FC-1 remains Blocked pending the
parent exact-head gate and fifth isolated verifier/panel; this body does not mark
runtime work Done.
