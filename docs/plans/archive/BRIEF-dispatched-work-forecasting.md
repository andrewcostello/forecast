# Brief: forecast dispatched agent work, not human tickets

**For another agent. Self-contained — everything needed to start is here.**

## The problem

`forecast` predicts well for human JIRA work and predicts badly for dispatched
agent work, for four specific reasons in `internal/montecarlo/simulator.go`.
The next large project (wallet v2, `dispatcher/wallet-v2-tasks.yaml` in
evenplay-mono) is **73 dispatched rows across 13 waves**, and the current model
would be confidently wrong about it.

## Four defects, in priority order

### 1. There is no dependency graph. `simulateOnce` is a capacity model.

```go
for size, count := range s.Remaining {
    for i := 0; i < count; i++ {
        hours := s.Rand.NormFloat64()*dist.StdDev + dist.Mean
        totalHours += hours
    }
}
days := totalHours / s.TeamCapacity
```

Every item is summed and divided by capacity, so the model assumes work is
perfectly parallelisable. Dispatched work is not: `wallet-v2-tasks.yaml` is 73
rows in **13 waves**, and within each unit `scaffold → seals → bodies →
adjudicate` is strictly serial. Four of the waves contain a single task.

**The critical path dominates, and the current model cannot see it.** A run of
73 rows at 40 minutes each is 48 hours of work but ~13 sequential steps; those
are different predictions and only one is right.

**Fix:** take the dependency graph as input and simulate it — per iteration,
sample a duration for each row, then compute the longest path subject to a
concurrency cap. The dispatcher already computes waves
(`dispatcher.cli run --mode dry-run` prints them); read `blockedBy` from the
tasks YAML rather than re-deriving.

### 2. It buckets by `Size`. Role is the far tighter predictor.

`calculateDistributions` groups cycle times by `item.Size` (S/M/L/XL). For
dispatched rows, `size:L` covers both a scaffold that writes a 300-line contract
and a bodies row that fills it — behaviourally unrelated. Measured on one unit,
same `size:L`:

| role | dev time | review time | rounds |
|---|---|---|---|
| scaffold (opus) | 17m | 11m | 0 |
| bodies (codex) | 175m | 65m | 8 |

**Fix:** bucket by `(role, model)`. Roles are `scaffold|seals|bodies|adjudicate`
and both fields are already on every dispatcher row.

### 3. Review time is charged PER ROUND and is not modelled at all.

Measured across seven arms, review was **27–50% of wall clock**, and it is paid
by three reviewer seats on every iterate round — whichever model implements. One
arm spent 163 minutes being reviewed across 20 rounds and delivered nothing.

So round count is the dominant cost driver, and it is a random variable, not a
constant. Model it explicitly:

```
row_wall_clock = dev_time + (rounds × review_time_per_round)
```

Sample `rounds` from history per `(role, model)`. This is the single highest-value
change after the graph, because it is what makes "a cheaper model that needs more
rounds" comparable to "an expensive one that needs fewer" — a question the
current model cannot express.

### 4. It samples from a normal distribution, which cannot be right.

```go
hours := s.Rand.NormFloat64()*dist.StdDev + dist.Mean
if hours < 0.5 { hours = 0.5 }
```

That clamp is the tell: a normal distribution puts mass below zero, so the code
truncates it. Task durations are right-skewed and strictly positive.

**Fix:** empirical bootstrap — resample observed durations directly. With few
samples per bucket it is better behaved than fitting, needs no distributional
assumption, and the clamp disappears. Fall back to log-normal only if a bucket
is too thin to resample.

## The data source

Dispatcher journals: `~/Project/dispatcher-runs/<run-id>/journal.jsonl`, one JSON
object per line, `event_type` + `payload` + `task_key` + `timestamp`.

Events that matter:

| event_type | carries |
|---|---|
| `task_started` | row begins; `payload.model` |
| `task_spawn_finished` | `payload.spawn_kind` (`implementer`/`panel-iterate`/`verifier`), `cost_usd`, `input_tokens`, `output_tokens`, `model` |
| `panel_started` / `panel_verdict` | bracket ONE review; the gap is review time |
| `panel_iterate` | one round; count these per `task_key` |
| `task_done` / `task_blocked` | terminal |

Reference implementation for the extraction already exists and is known to work:
`claude-workflow/features/model-matrix/report.py`, function `journal_facts()` —
it computes dev vs review split, round count and token totals per row. Port or
reuse rather than rewriting.

`agent` / `model` / `effort` / `role` come from the tasks YAML. **Read the model
the dispatcher STAMPED, not the one authored** — on cascade the dispatcher
rewrites the row with what actually ran, and an earlier comparison was invalid
for exactly this reason.

## Suggested shape

```
forecast dispatched-reference build --runs-dir ~/Project/dispatcher-runs \
    --out .forecast/dispatched-reference.json
forecast dispatched-predict --tasks dispatcher/wallet-v2-tasks.yaml \
    --reference .forecast/dispatched-reference.json \
    --max-parallel 4 --iterations 10000
```

Output: p50/p80/p95 **wall clock** and **cost**, plus the critical path — which
rows are on it, because that is what a reader can act on. A p95 nobody can
shorten is a number; a p95 plus "these 13 rows are the chain" is a plan.

## Validation, and it is the part to not skip

The reference class is built from completed runs, so **hold one out**. Build from
the earlier dogfood-go and model-matrix runs, predict the wallet run, then
compare to actual once it lands. If the p80 does not contain the outcome, say so
rather than tuning until it does.

## Honest limits to state in the output, not bury

* **n is small.** At time of writing there is roughly one sample per
  `(role, model)` cell. Two arms of the SAME model on the same task differed
  1.9× in output and inverted on the quality metric. A prediction from n=1 is a
  guess with error bars drawn around a single point, and should be labelled that
  way. A replicate study (7 models × 5 runs) is running and will thicken it.
* **Blocked rows have no duration.** A row that blocks after 8 rounds consumed
  real time and produced no completion. Decide explicitly whether it is a
  censored observation or excluded — do not let it silently become a fast row.
* **Human intervention is invisible.** Several rows in past runs were finished by
  hand after blocking. Those wall clocks are not agent throughput.
