# FC-SOURCES: explicit bounded source implementation

## Contract section

Implemented the frozen F3 seams in `internal/dispatched/sources.go`:

- `SourceSpec`, `Selection`, and `ReadBounds` validate explicit, repeatable
  repositories, roots, refs, cutoff, holdouts, and positive resolved bounds
  before content reads.
- `ReadSources` processes sources sequentially by source ID, discovers journals
  strictly, records requested and resolved refs, traverses every reachable commit
  and selected tree root, streams distinct blobs, and retains per-source counts,
  states, reasons, and audit envelopes.
- `sourceBudget`, `runSourceGit`, and `openSourceFile` enforce per-source process
  serialization plus shared total-byte and per-blob bounds. Git uses only the
  frozen read-only commands and isolated environment; local files are opened
  beneath an `os.Root` descriptor and final-component symlinks are refused.
- `parseReadings` independently decodes string-tagged join identities and cutoff
  timestamps before predictive fields, retaining valid siblings, non-task
  documents, and malformed row envelopes.
- Holdout and cutoff evidence is removed at the source boundary while its
  identity, times, citation, exclusion marker, and excluded-quality diagnostics
  remain available for audit. `ValidateComplete` rejects every frozen partial,
  empty, contradictory, shallow, replaced, grafted, cancelled, malformed,
  unreadable, or bounded shape with the required sentinels.

## Reason

This replaces every FC-SOURCES-owned `ErrNotImplemented` body without changing
its scaffolded signature or parameter names. Sources are explicit and bounded so
an artifact cannot claim evidence outside its manifest, silently use personal
defaults, buffer an unbounded Git child, or train on held-out/post-cutoff rows.

## Affected callers

- FC-1 receives canonical `SourceManifest` and `SourceReadings` carriers for the
  amended build, reducer, join, and eligibility pipeline.
- Direct callers can validate manifest completeness with typed shallow/bound
  causes and can classify cancellation through both `ErrSourceCancelled` and the
  context error.

## Seal evidence

- `go test ./internal/dispatched -run <all TestFCSourcesContract cases except F3-MISSING-REVISION-TIME> -count=1` — pass (42 cases).
- The same 42-case selection with `-race -count=1` — pass.
- `go test ./internal/dispatched -run '^TestFCSourcesContract/(F3-BOUND-BYTES|F3-BOUND-PROCESSES|F3-SOURCE-CONCURRENCY|F3-CANCELLED)$' -race -count=1` — pass.
- `go test ./internal/dispatched -run '^TestFCSourcesContract$' -count=1` — 42
  cases pass; `F3-MISSING-REVISION-TIME` reaches the implemented parser, then
  fails on the separately owned `JoinEvidence` `ErrNotImplemented` stub.
- `go build ./...` — pass.
- `go vet ./...` — pass.
- `go test ./...` — fail only in the registered unfinished FC-1 evidence/CLI
  groups and the cross-owner `F3-MISSING-REVISION-TIME` call described above.
- `go test ./... -race -skip '^(TestFCEvidenceContract|TestFCReferenceCLIContract)$' -count=1`
  — all packages and the owned source cases pass under race except the same
  cross-owner `F3-MISSING-REVISION-TIME` call to the FC-1 stub.

## Exact commit

No implementation commit was created in this session. `git commit` failed because
the managed worktree's shared Git metadata could not create `index.lock` on the
read-only filesystem. The implementation remains in the working tree for the
dispatcher to commit, as its instructions permit. Starting HEAD was `f4e7b56`.

## scaffold_review_followups

| Finding | Disposition |
|---|---|
| `claude-1` | **Deferred contract amendment.** The accepted handoff and `F3-GIT-INSTALLATION-ENV` seal explicitly require preserving caller `GIT_EXEC_PATH`; the body does so and strips every other ambient `GIT_*` override. Deriving or dropping it would contradict the immutable seal, so the security change needs scaffold/seal adjudication. |
| `claude-3` | **Adopted in the body.** `ReadSources` installs an `os.Root` descriptor on the per-source budget, `openSourceFile` opens relative to that descriptor, and final symlink entries are rejected. This prevents an opened path from escaping the selected repository even if a parent path is raced. |
| `claude-4` | **Adopted.** The completed seam returns nonnil/list-initialized carriers, resolved bounds, canonical cutoff/holdouts, and an explicit COMPLETE/PARTIAL/EMPTY state appropriate to the reached outcome. |
| `claude-5` | **Adopted.** The `ctx.Err()` typo is fixed and the seam comment names the remaining F3 error classes while retaining its pointer to the authoritative rows. |
| `claude-7` | **No FC-SOURCES behavioral change.** Every amended live/journal read goes through `openSourceFile`; the explicitly retained baseline readers remain untouched. The schema-4 `Rounds`/`Limits` projection is FC-1-owned and is not reinterpreted here. |
| `grok-1` | **Partially adopted without changing the freeze.** Every command carries `--no-pager`, uses pipes rather than a TTY, disables config/helpers/protocols, and pins `-C` to the selected repository. The suggested additional environment/config entries are not in the accepted affirmative list and require contract/seal adjudication before adoption. |
| `grok-2` | **Adopted.** The source seam comment now spells `ctx.Err()` correctly. |

## Inherited dependency findings

- The body treats `MaxBlobBytes` inclusively: it retains exactly the configured
  number of bytes and performs a one-byte EOF probe, so only byte N+1 raises
  `ErrBoundExceeded`. The inherited seal observation is correct that its fixtures
  do not independently pin the exact-N case; this task did not edit tests.
- The process and source-concurrency observation windows are now reached and pass
  repeatedly, including under `-race`; the remaining comments about redundant
  marker paths, final-tick polling, and fixture timeout slack concern immutable
  FC-SEALS helpers rather than the source body.
- FC-JOURNAL review findings remain in that dependency. This source body does not
  duplicate its parser/reducer logic; it preserves the parser diagnostics and
  source completeness consequences returned through the frozen seam.

## Deviation

None in the implementation: all frozen signatures and parameter names are
unchanged. The owned seal group cannot be wholly green on this dependency head
because `F3-MISSING-REVISION-TIME` directly invokes FC-1-owned `JoinEvidence`,
which is still the scaffold stub. Crossing that source-ownership boundary would
violate the dispatch contract; the seal dependency requires adjudication.

## Residual limitation

The accepted contract preserves ambient `GIT_EXEC_PATH`, as its seal requires;
the assigned security follow-up recommends deriving or dropping it instead.
That contradiction remains for scaffold/seal adjudication. Known FC-JOURNAL
review findings remain owned by that dependency and are not modified here.
