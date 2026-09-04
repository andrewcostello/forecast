# Dispatched forecasting contracts

These rules govern every remaining FC row. Amend the existing scaffold to
implement them; preserve existing useful code. The source model is:

`explicit sources -> recorded evidence -> attempts -> validated observations -> coverage -> conditional forecast`

All source/data failures have named typed errors, wrapped for errors.Is.

## F1 — attempt identity and evidence

An attempt ID is `(dispatcher_run_id, task_key, started_at as a UTC instant)`.
Different runs remain different attempts even if key and time coincide. YAML
revisions with that same triple are readings of one attempt, never additional
samples. Distinct journal starts with an identical triple are ambiguous and
excluded with a named count; do not choose the nearest timestamp.

Keep source repository, relative path, revision/commit, journal identity and
relevant event positions with each reading. Merging must not manufacture a row
by taking independent maxima of incompatible readings and attributing them to
one arbitrary source. Define field evidence explicitly: journal terminal and
implementer stamp take precedence over YAML; a YAML-only terminal is labeled;
unknown stays unknown. Within an attempt, conflicting authoritative model or
terminal evidence is a named conflict. Evidence precedence and conflict
selection must be deterministic under input permutations.

The final recorded implementing model is the bucket, as previously required.
A cascade is counted and disclosed: the bucket describes the closing model,
not an assertion that it performed every second of the attempt. Do not pool
model aliases or silently substitute an authored model for an absent stamp.

## F2 — time, outcomes, and missing measurements

Elapsed is wall time from the attempt start to its terminal event, or to a
recorded extraction cutoff while unfinished. Blocked and unfinished elapsed
values are right-censored lower bounds and never completed-duration samples.
Retain usable elapsed evidence even when a finer phase breakdown is unknown.

Development, panel review, verifier work, and unclassified time are distinct
concepts. Report panel wall time, not the sum of simultaneous reviewer seats.
Any additive wall-time breakdown must use disjoint intervals contained in the
attempt. Its classified sum cannot exceed elapsed; residual time is explicitly
unclassified, never invented development time. Missing/ambiguous phase data
makes the breakdown incomplete, not zero and not a fabricated precise split.

Producer semantics decide event meaning. In the measured producer,
`panel_iterate` is emitted after the corrective spawn returns, not when work
begins. Freeze actual journal sequences and producer revision in fixtures.
Use explicit boundaries/durations where the record supports them; inferred
intervals must be labeled and unavailable where attribution is ambiguous.
`journal_facts()` is reference material, not an oracle that overrides these
rules. Preserve round count as recorded corrections, separate from review
invocation count; a first review is not a correction round.

Money and token quantities are separate from wall time. Missing cost is null;
measured zero is zero. Never sum overlapping or repeated evidence twice.
Non-finite, negative, reversed, or conflicting measurements have named errors
or quality diagnostics. No malformed measurement reaches a mean silently.

## F3 — sources and completeness

Source repositories and task-file roots are explicit, repeatable inputs; no
personal-home default. Support paths outside `features/` via explicit roots
(e.g. `dispatcher/`), without scanning unrelated personal files. Each source
has an identity, requested path/ref, actual resolved ref, and read outcome.
Strip Git location overrides when invoking Git against a selected repository;
record shallow/grafted/replaced history as partial or reject it explicitly.

Enumerate full reachable history of selected roots, including superseded merge
parents and deleted/renamed files. Bound commit enumeration before collecting
it, and stream or cap metadata and blob bytes as well as process count. Caps,
shallow history, unreadable records, missing roots, and cancellations cannot
be reported as complete. Avoid automatically fetching deeper history.

Missing/unreadable requested sources and zero discovered journals are errors
by default. Explicit diagnostic `allow-empty` mode may produce an EMPTY
artifact, but never a prediction-eligible artifact. Successfully read sources
with zero matching observations are distinct from failed discovery. Malformed
records may be counted while valid records are retained, with PARTIAL status.

Every examined snapshot has a disposition, including no matching run/key, no
matching start, ambiguous start, missing join keys, absent stamp, conflicting
evidence, and recovered duplicate reading. Report unique rows and attempts
separately. A run/task has recoverable YAML only after an unambiguous attempt
match. Reconciliation counts may not hide lost attempts behind a recovered
sibling. Store per-source counts and an extraction cutoff; no claim extends
beyond the sources supplied.

## F4 — reference class and prediction eligibility

The report represents requested empty cells as n=0. A valid cell has a valid
role and nonblank exact model; malformed targets are errors. A zero-row target
may be inspected but cannot pass a coverage/prediction gate. Prediction needs
complete selected sources and at least the declared minimum completed samples
per required cell (default 2, reported as a threshold, not proof of calibration).
Partial sources can produce diagnostic artifacts, not silently eligible ones.

The sampler resamples joint completed-attempt records: total elapsed, round
count, available phase measurements, and cost provenance stay paired. Do not
independently multiply sampled rounds by sampled mean review duration. Missing
phase data does not invalidate known total elapsed. Missing cost cannot become
zero or be silently filtered into an apparently complete cost forecast: report
cost unavailable when the requested population lacks the required evidence.
No log-normal fallback, invented defaults, alias pooling, or censored duration
mean. Report observed blocked/completed counts beside duration results.

The initial forecast is explicitly conditional on successful completion under
the observed process. It does not promise an unconditional time to completion
for work that can remain blocked indefinitely. A survival/retry model would
require its own contract and observations; it is outside these remaining rows.

## F5 — scheduling under a concurrency cap

Scheduling is a pure deterministic function of a graph with already sampled
durations. Random sampling belongs to FC-3, not either scheduler arm. When
multiple nodes are ready, choose declaration order. Process all completions at
a timestamp before filling slots; drain zero-duration work deterministically.
Dependencies are sets. Validate blank/duplicate keys, unknown dependencies,
negative durations, cycles, nonpositive concurrency, and duration overflow
with documented deterministic error precedence in the scaffold.

Return each node's start/finish, makespan, and two different explanations:
- dependency path: original dependency edges only;
- execution chain: causal dependencies plus resource-slot waits for this
  specific schedule, with edge kind and deterministic tie-breaking.

The execution chain explains the makespan when the cap binds. For example,
independent A=2 and B=3 with cap=1 take 5; a dependency-only path cannot explain
that 5. Preserve dependency-only information under its proper label rather
than calling it the cap-constrained critical path. Resource ordering is the
specified list-scheduling policy, not a claim to find an optimal schedule.

FC-2A and FC-2B implement the same frozen contract in isolated packages and
sessions. Neither may inspect the other's implementation. Both receive the
same external fixtures and small-graph oracle. Compare complete schedules,
errors and both path kinds, not just makespan. Any disagreement is minimized
and adjudicated; neither arm is edited simply to agree with the other.

## F6 — prediction and held-out validation

Preserve `forecast dispatched-predict --tasks … --reference … --max-parallel N
--iterations N`, reporting p50/p80/p95 wall clock and available cost.
Use an explicit seed and stable task order. Quantile convention, units,
iteration count, concurrency, artifact version, source manifest and sample
threshold are recorded. Validate artifacts and dependency closure before
sampling. Keep wall-clock and cost distributions separate; never divide
makespan by team size after scheduling.

A percentile's explanation comes from a named representative simulation near
that percentile, with its own schedule and execution chain. Do not assemble
an impossible chain from independent marginal percentile durations. Report
coverage-only refusal as a data result without Cobra usage spam.

Holdout selection happens before extraction and sampling: freeze excluded run
IDs, source refs/cutoff, and the target's pre-run graph/model pins. Exclude held-
out records from live and historical joins. Do not train on later task stamps
or tune thresholds/seed after seeing the outcome. Record an upper-p80 check as
actual <= predicted p80; one held-out run is not a calibration study. If the
holdout is blocked, compare censoring honestly rather than inventing a terminal
duration. Missing evidence may make the required live validation BLOCKED; it
does not justify invented forecasts or a Done status.
