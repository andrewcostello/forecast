# FC-1 correction pass — 2026-09-04

The implementation now accepts multiple source repositories and joins snapshots
only to journal attempts with the same start instant. The prior cross-family
panel remains the historical verdict; this correction has not rerun that panel.

## Blocking findings

| Finding | Resolution | Evidence |
|---|---|---|
| Single source repo omitted observations | Repeatable `--features-repo`; union live and historical readings; preserve source repository/path; print per-repo matched attempts | `TestBuildUnionsRepositoriesAndRetainsSource`, `TestBuildCommandAcceptsRepeatedSourceRepositories` |
| Nearest-start matching duplicated attempts | Require exact normalized timestamps and reject ambiguous starts; count unmatched snapshots and attempts | `TestHistoricalTimestampCannotClaimAnotherAttempt`, `TestAmbiguousStartCannotSupplyEvidence` |
| Invalid targets passed the coverage gate | Require a tasks sequence, unique nonblank keys, valid roles, and nonblank models | `TestTaskDocumentShapeAndTargetCellsAreValidated`, `TestCoverageGateRejectsMalformedTargets` |
| History enumeration was unbounded | Bound `rev-list` with one-commit lookahead; batch changed-file enumeration through `diff-tree`, then blob reads through `cat-file` | `TestHistoryUsesBoundedBatchedGitProcesses`, `TestHistoryRecoversDeletedAndRenamedTaskFiles` |

## Related findings

- `--fail-on-uncovered-required` rejects cells below the completed-observation
  threshold, including cells containing only blocked rows. The existing
  empty-cell flag retains its literal meaning. Both require `--tasks`.
- Recovered attempts and their shortfall are reported separately from unique
  run/task rows. Counts use matched journal identities rather than merged
  representative provenance.
- Non-task YAML documents and rows missing join keys have explicit counters.
- Conflict diagnostics in the builder compare original terminal readings,
  avoiding attribution to an unrelated earlier unfinished reading.
- Journal scans check cancellation while scanning. CLI extraction has a
  configurable five-minute timeout.
- Negative cascade counts fail validation.
- CLI tests use fresh commands and committed fixtures; history wiring is
  asserted with a nonzero commit count.
- History enumeration no longer requires `git ls-tree --format`.

## Recorded deviations

The source option now accepts multiple repositories; the single-repository Go
option remains compatible. Provenance adds repository/path fields. Artifact
schema version 3 adds those fields and attempt/source coverage diagnostics.
Exact start matching deliberately excludes snapshots whose edited or stale
start timestamps cannot identify an attempt. A capped history reads changed
YAML revisions within the bounded walk and reports truncation; it does not
claim complete historical coverage when truncated.

These extend the scaffold's provenance and report shape. No duration is
invented for missing cells, and censored observations never enter duration
statistics. FC-2 and later prediction tasks remain held.

## Measurement

This supersedes the earlier single-repository claim that bodies and adjudicate
observations do not exist. It measures only the two repositories listed below,
not every repository that could contribute evidence.

```sh
go run ./cmd/forecast dispatched-reference build \
  --runs-dir ~/Project/dispatcher-runs \
  --features-repo ~/Project/claude-workflow \
  --features-repo ~/Project/claude-dispatcher \
  --tasks ~/Project/evenplay-mono/dispatcher/wallet-v2-tasks.yaml \
  --out ~/Project/dispatcher-runs/fc1-reference-2026-09-04.json
```

At extraction: 110 observations, 108 of 258 distinct started run/task rows,
110 of 271 started attempts. There were 147 attempts without matching YAML,
100 unmatched YAML snapshots, and 14 matched identities without attributable
model/role evidence. History was not truncated. The supplied source repos
matched 50 and 74 attempts respectively.

The current wallet target had 77 rows across six required cells. Five cells
were empty; all six were below the two-completion threshold, yielding 0/77
covered target rows. These are measurements of those exact model strings;
`claude-fable-5` is not pooled into `claude-fable-5-1`.

Validation: `go build ./... && go vet ./... && go test ./... -race` passed;
`internal/dispatched` statement coverage was 92.9%.
