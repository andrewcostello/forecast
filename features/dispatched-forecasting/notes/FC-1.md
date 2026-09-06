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
