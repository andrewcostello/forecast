# Execution and evidence protocol

## Baselines and preserved work

The redesign is preparation, not permission to call a blocked live service.
The existing dirty `forecast` checkout and reviewed FC-1 worktree are preserved.
Exact prior worklists live under each feature's `archive/`; old FC brief and
assignment live under `docs/plans/archive/`. Do not dispatch an archive or
resume the old run against a redesigned worklist.

Start FC from `feature/dispatched-forecasting-v2`, retaining reviewed code at
`d529265` plus this plan and current main. Start YouTrack from current `main`
with this plan. Read the worklist top to bottom. Use new worktree parents below
because reused FC keys otherwise reopen old worktrees; also verify generated
branch names do not collide with old task branches. Never delete/requeue an
old branch to make a fresh run start. The dispatcher merges completed dependency
branches before spawning each child; check those merge SHAs in its journal.
A merge failure remains Blocked. No implementation auto-integration or push
is part of this planning change.

Do not run YouTrack concurrently with wallet v2 in evenplay-mono. The earlier
permission to run the FC-1 review panel was specific to that panel. Check live
run/process state immediately before launching another Claude-consuming run;
an old idle log or a timed pause is not proof that the accounts are free.
Run these epics sequentially: their command and documentation changes overlap.

## Dry runs and launch shapes

The revised graph has 20 FC rows (1 Done, 19 remaining) and 13 active dependency
waves. YouTrack has 20 rows (19 To Do, 1 externally Blocked), 16 active waves
and a final live-adjudication wave once unblocked. A graph wave can contain 3
rows; the runtime cap remains 2. These replace the archived 8-task/7-wave check.

From the checkout containing the corresponding revised worklist:

```sh
dispatcher run features/dispatched-forecasting/tasks.yaml \
  --mode dry-run --base-branch feature/dispatched-forecasting-v2 --max-parallel 2

dispatcher run features/youtrack-support/tasks.yaml \
  --mode dry-run --base-branch main --max-parallel 2
```

Once the relevant account/prerequisite checks pass, retain the authorized
execution controls. These are launch recipes, not records of runs performed:

```sh
dispatcher run features/dispatched-forecasting/tasks.yaml \
  --mode unattended --base-branch feature/dispatched-forecasting-v2 \
  --worktree-base "$HOME/Project/forecast-v2-worktrees" \
  --max-parallel 2 --max-cost-usd 200 --cross-family-panel-iterate 5 \
  --runs-dir "$HOME/Project/dispatcher-runs" \
  --claude-extra-args "--permission-mode bypassPermissions --allow-dangerously-skip-permissions"

dispatcher run features/youtrack-support/tasks.yaml \
  --mode unattended --base-branch main \
  --worktree-base "$HOME/Project/youtrack-v2-worktrees" \
  --max-parallel 2 --max-cost-usd 200 --cross-family-panel-iterate 5 \
  --runs-dir "$HOME/Project/dispatcher-runs" \
  --claude-extra-args "--permission-mode bypassPermissions --allow-dangerously-skip-permissions"
```

Keep existing row agent/model/effort pins. New rows use the same permitted
fable/opus/sol/grok floor. Verify actual implementer, verifier, reviewer and
fallback models before spending; do not enable cheap-first or a cheaper
summary model. Record actual stamps when a permitted cascade occurs. A budget
stop holds work; it does not authorize another $200 restart. The CLI's cost
limit is a dispatch guardrail, not an exact total: in-flight work and reviewer
spend can exceed its accounting. The redesign does not raise the limit.

## Red-first gates and immutable seals

Every phase runs build and vet. New missing-capability seals must compile and
fail on the named contract assertion, not a crash. Existing Jira/CLI regression
seals stay green and must have a demonstrated failing mutation. Record fixture
provenance and mutation evidence. Re-run the mutations against completed
bodies during adjudication when a missing implementation previously prevented
a meaningful mutation demonstration. No xfail, skip or real credentials.

`config/known-red.yaml` contains plan-author reservations for unique top-level
contract test groups, each with one seals task and one body owner. No existing
test matches a reserved name at plan creation. Reservations are not evidence
that tests have been written or proven red. The dispatcher protects this file
from all task roles, so registration is done here by the operator before
seals run. A mismatched/new name requires a narrow operator amendment with
failure evidence; a task must not bypass the protected-path rule.

Each body directly depends on its seals row. Its own group is NEVER excluded.
Only another unfinished body's registered groups may be hidden from its gate;
Done-body groups are restored by the dispatcher. Ordinary `go test` runs all
seals. Adjudication runs its whole completed unit without exclusions; final
epic adjudication runs every seal on the integrated epic tree without any
known-red exclusions. Reconcile unused reservations rather than leave an
unowned failing test. The shared configured gate is build, vet, and race tests.
For the final gate use:

```sh
go build ./...
go vet ./...
env -u DISPATCHER_KNOWN_RED_FILE go test ./... -race
```

The risk table currently has zero financial rules, so the dispatcher derives
`**` with its stated fail-closed reason. This plan does not suppress that
classification or disable review/preflight/verification.

## Handoffs, ownership and deviations

The scaffold records types, errors, ownership, and behavior/example tables in
its per-task notes. Contract amendments require a named ruling before seals;
a scaffold is not a placeholder-only claim that behavior is already defined.
Preserve existing behavior and compiling legacy call shapes during file moves.
No acceptance tests in scaffolds. Seal authors place temporarily red amended
legacy cases under the appropriate registered owner group; they do not leave
an unregistered old test failing in unrelated bodies. Unaffected regressions
stay active.

Seal authors are independent sessions. Bodies edit only owned source files;
seals and fixtures stay immutable. Scheduler arms have separate packages and
must not inspect each other's code. Adjudication compares both only after
both land. A shared-file requirement discovered during parallel work blocks
that change until ownership/dependencies are amended.

Every bodies note records contract section, deviation (or explicit none),
reason, affected callers, seal evidence, exact commit, and residual limitation.
Adjudication records accepted-with-reason or corrective-row-required for each.
Final reports include these dispositions, not just test pass counts. A new
implementation fix must return through an amended scaffold/seals/body sequence;
adjudication cannot silently weaken acceptance or become an unbounded body.

## External prerequisites and final reports

YT-ADJ stays Blocked until a real YouTrack URL/project and a locally stored
permanent-token reference are supplied. Never put a token in a worklist or
run log. Only that row calls live YouTrack. It needs enough real history to
produce the required forecast; creating one fresh ticket does not supply that
history. Any required bridge change is a separate-repo release dependency.

FC-ADJ predeclares dogfood-go/model-matrix training sources and wallet holdout,
including exact run IDs, cutoff/refs and pre-run target pins. If those sources
lack sufficient independent completed observations, report the shortfall and
hold validation; do not substitute the holdout into training or quietly pick
another successful run. A changed evaluation population requires an explicit
recorded adjudication before evaluation.

The final FC report includes all panel-finding dispositions, coverage and
holdout result, source/seed manifest and limitations. The final YouTrack report
includes every bodies deviation, the real readable ticket key and forecast,
and a precise dispatcher change matrix for create, transition, key parsing,
field compatibility and replay. “Only select the subcommand” is a conclusion
to prove, not the required answer.
