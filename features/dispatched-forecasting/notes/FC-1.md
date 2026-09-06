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
