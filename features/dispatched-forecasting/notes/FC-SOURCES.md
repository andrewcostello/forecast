# FC-SOURCES: corrected explicit bounded source implementation

## Contract and final implementation

`internal/dispatched/sources.go` implements the accepted F3 source seams without
changing frozen public signatures or parameter names and without implementing
FC-1 `JoinEvidence` or `Build`.

- `ReadSources` validates explicit repositories, roots, refs, cutoff, holdouts
  and resolved positive bounds before content reads. Sources remain sequential
  in canonical source-ID order.
- Journal and live discovery use the same opened `os.Root` identity used for
  later confined opens. Final symlinks are refused. Live mtime and stability
  checks come from the exact file descriptor whose bytes were read; a normal
  size or mtime change during the read is rejected rather than admitting bytes
  under stale metadata. The recorded mtime remains evidence, not a tamper-proof
  clock, as the scaffold specifies.
- The shared byte budget reserves allowance atomically before each physical
  read across files, Git stdout, Git stderr and concurrent children. Readiness
  is established before reservation, so an idle stderr pipe cannot reserve the
  corpus or deadlock stdout. One source-wide overflow probe is bounded,
  physically counted and never retained.
- Git uses owned pipes and starts `Wait` immediately. EOF observes exit status;
  Close surfaces undelivered exit, cancellation and bound errors; a terminal
  Read followed by Close is benign. The process and inherited-pipe cleanup
  deadline does not depend on `Cmd.WaitDelay` starting before a later Wait call,
  and stderr is drained before its pipe is released.
- `ValidateComplete` verifies resolved-ref names, full lowercase object IDs,
  unique/canonical all-ref entries, explicit-ref consistency, non-history
  resolution absence and diagnostic/state consistency. An unborn all-ref
  history is PARTIAL with a reason, never a COMPLETE manifest that rejects
  itself. AllowEmpty and defensive returns retain canonical lists and aggregate
  source reasons.
- All-ref tips are resolved in one `for-each-ref` snapshot when their object
  shape permits, and the resolved commit IDs—not moving ref names—drive the
  traversal. A provider-side overflow-safe `MaxCommits+1` limit bounds
  `rev-list`; the same command supplies commit times. Every reachable retained
  DAG snapshot is still read, including superseded merge parents. One
  `ls-tree` per retained commit remains necessary for that snapshot semantics,
  followed by one bounded `cat-file blob` per distinct object.
- Linked-worktree graft detection uses the Git common metadata directory.
  Noncommit or otherwise unsupported refs are not silently omitted: resolution
  fails the source with `ErrGitHistory`, so no manifest can become falsely
  COMPLETE while claiming a complete all-ref tip list.
- Successfully decoded YAML without a `tasks` mapping—including empty, null,
  list and scalar documents—is `DocumentNotTasks`. Invalid syntax or malformed
  `tasks` shape remains `DocumentMalformed`. Structural identity/time nodes
  retain row-local errors; exclusion is never inferred from a non-string run
  identity, while genuinely absent fields retain their absent semantics.
- The Git request validator accepts only the precise read-only forms required
  by the body and seals. Output, external helper, filter, batch and write-capable
  option space remains unreachable. The frozen installation policy still
  preserves `PATH` and `GIT_EXEC_PATH` while stripping other ambient `GIT_*`
  state and disabling helpers/protocols.

The journal cutoff boundary is intentionally unchanged: `SourceReadings`
retains raw parsed in-sample journals, and `ReduceAttempts` removes events
strictly after the cutoff. `ReadSources` must not silently filter
`ParsedJournal.Events`; F3-CUTOFF-EXCLUDED and the `SourceReadings` godoc assign
that work to the reducer. Holdout journals remain identity-only in
`ExcludedJournals`.

## Reason and affected callers

The correction closes descriptor/metadata races, physical-read over-allocation,
child cleanup/error loss, self-contradictory manifests, provider-side traversal
overrun, linked-worktree metadata loss and YAML classification gaps. FC-1 and
direct callers receive canonical `SourceManifest` and `SourceReadings` values
whose eligibility claims do not exceed the exact selected sources.

## Final seal evidence

- Red-before: the corrective Sources cases failed at committed HEAD `8f35efc`
  on Git Close errors, concurrent total-byte accounting, resolution validation,
  empty history, provider limiting, linked-worktree grafts, AllowEmpty reasons,
  non-task shapes, structural fields and write-capable Git options.
- `go test ./internal/dispatched -run '^TestFCSourcesContract$' -race -count=1`
  passes the full amended 54-case owned source group.
- `go test ./internal/dispatched -run '^(TestFCSourcesContract|TestFCJournalContract)$'
  -race -count=3` passes all
  54 Sources cases and all 51 completed Journal cases on each repetition.
- `go build ./...` passes.
- `go vet ./...` passes.
- `go test ./... -race -skip '^(TestFCEvidenceContract|TestFCReferenceCLIContract)$'
  -count=1` passes every
  included package. The only registered unfinished groups excluded are FC-1
  Evidence and FC-1 Reference CLI. The zero-revision source ingest case is green
  in Sources; the relocated join case remains correctly owned by FC-1.

The starting corrective HEAD is `8f35efc`. The immutable hash of the corrective
commit containing this note is recorded in the dispatcher run `summary.md`,
because a commit cannot embed its own hash.

## Latest panel dispositions

Every finding below is indexed in its review-family order from
`2026-09-06T01-18-41Z-FC-SOURCES-corrected-panel`.

| Finding | Disposition |
|---|---|
| `claude-1` | **Fixed.** `parseReadings` classifies valid empty/list/scalar/null YAML as `DocumentNotTasks`; F3-NON-TASK-SHAPES preserves malformed syntax/tasks shapes. |
| `claude-2` | **Fixed.** `decodeIdentityField` and `decodeTimeField` attach row errors to sequence/map/alias nodes; F3-IDENTITY-STRUCTURE proves the holdout and completeness consequences. |
| `claude-3` | **Partly optimized, advisory remainder documented.** One `for-each-ref` resolves ordinary tips and bounded `rev-list --format=%cI` supplies all retained commit times. Per-commit `ls-tree` remains to preserve full DAG snapshots; there is no frozen total-process threshold. |
| `claude-4` | **Fixed.** AllowEmpty calls `finalizeManifest` before setting aggregate EMPTY; F3-ALLOW-EMPTY-REASONS proves reasons and nonnull lists survive. |
| `claude-5` | **Fixed defensively.** The otherwise unreachable per-source budget-construction error now records/canonicalizes diagnostics before returning. |
| `claude-6` | **Fixed.** Owned pipes allow immediate concurrent Wait and stderr drain without `StderrPipe`/Wait truncation; explicit post-exit pipe cleanup handles inherited descriptors. F3-GIT-CLOSE-ERRORS covers the public error surface. |
| `claude-7` | **Fixed.** The dead command-only allowlist was replaced by precise argument grammars; F3-GIT-REQUEST-READONLY covers accepted and rejected forms. |
| `claude-8` | **Fixed/sealed.** F3-MISSING-REVISION-TIME-INGEST directly proves in-sample Malformed/PARTIAL handling. The FC-1 join case remains separate. |
| `codex-1` | **False positive by adjudication.** Raw journal events stay in `SourceReadings`; `ReduceAttempts` owns strict post-cutoff event filtering. No source filter was added. |
| `codex-2` | **Fixed.** Live evidence metadata is descriptor-derived and stable across the read; confined discovery and opening share the same `os.Root`. This is also the unflaky body-review obligation recorded by FC-SEALS. |
| `codex-3` | **Fixed.** Readiness-aware atomic reservation enforces physical shared-byte accounting with one global probe; F3-BOUND-TOTAL-CONCURRENT passes. |
| `codex-4` | **Fixed.** Close returns undelivered Git exit/cancel/bound failures and reaps/releases every path; EOF-then-Close and error-Read-then-Close remain benign. |
| `codex-5` | **Fixed.** `ValidateComplete` now rejects malformed, padded, duplicate or inconsistent ref metadata, non-history resolution metadata and COMPLETE diagnostics; F3-COMPLETE-RESOLUTION passes. |
| `codex-6` | **Fixed.** Empty all-ref history is explicitly PARTIAL and validator-consistent; F3-EMPTY-HISTORY-CONSISTENT passes. |
| `codex-7` | **Fixed.** Precise read-only grammars reject `--output` and external-helper forms while retaining required operations; F3-GIT-REQUEST-READONLY passes. |
| `grok-1` | **Fixed.** `rev-list` receives provider-side `MaxCommits+1` (without integer overflow) and retains Go-side cap verification; F3-BOUND-COMMITS-LIMITER passes. |
| `grok-2` | **Advisory optimization applied where semantics are safe.** Ref and timestamp calls are batched. Consecutive linear `diff-tree` substitution was rejected because it loses independent DAG snapshots; bounded per-commit `ls-tree` remains. |
| `grok-3` | **Adjudicated alternative rejected.** Unsupported/noncommit refs are not skipped. They produce `ErrGitHistory` and PARTIAL diagnostics, preserving the accepted claim that `ResolvedRefs` is the complete resolved ref-tip list. |
| `grok-4` | **Fixed.** Grafts are read from `--git-common-dir`; F3-GIT-WORKTREE-GRAFTS covers a linked worktree. |
| `grok-5` | **Fixed.** Journal directory reads, live walks and local opens all use the same `os.Root` directory identity; symlinks remain refused. |
| `grok-6` | **Fixed.** AllowEmpty always canonicalizes and aggregates before EMPTY; F3-ALLOW-EMPTY-REASONS passes. |
| `grok-7` | **Fixed/sealed.** Source-owned zero-RecordedAt ingest remains malformed, in-sample and PARTIAL under F3-MISSING-REVISION-TIME-INGEST. |
| `grok-8` | **Fixed.** This note replaces the stale 42-case and Sources-owned JoinEvidence-block claims with the amended 54-case result and FC-1-only red ownership. |

## Preserved scaffold follow-ups

| Finding | Disposition |
|---|---|
| `scaffold claude-1` | **Deferred contract amendment.** F3-GIT-INSTALLATION-ENV requires inherited `GIT_EXEC_PATH`; the body preserves it. |
| `scaffold claude-3` | **Adopted and strengthened.** Confined opens, same-root discovery and descriptor metadata are implemented; final symlinks are refused. |
| `scaffold claude-4` | **Adopted.** Carriers, bounds, cutoff, holdouts and lists remain initialized/canonical on reached outcomes. |
| `scaffold claude-5` | **Adopted.** The seam exposes the documented typed F3 errors and correct `ctx.Err()` wrapping. |
| `scaffold claude-7` | **No FC-SOURCES projection change.** Schema-4 `Rounds`/`Limits` remains FC-1-owned; baseline readers remain untouched. |
| `scaffold grok-1` | **Frozen policy retained.** No unaccepted Git environment/config additions were introduced. |
| `scaffold grok-2` | **Adopted.** Cancellation documentation and wrapping use `ctx.Err()` correctly. |

## Historical blocked record, deviations and residual limits

Historically, the first body session at starting HEAD `f4e7b56` could not create
the shared Git `index.lock` and reported a 42-case Sources group blocked by a
cross-owner `JoinEvidence` call. The dispatcher later committed that body as
`ab03f0a`; operator commit `f3a3e39` moved the unchanged join assertions to the
FC-1 group, and seal commit `8f35efc` added the corrective cases. That blocked
account is retained here as history only; it is not the state or evidence of
this corrected body.

Contract deviation: none. Public signatures, parameter names, typed errors,
legacy extraction paths and the `GIT_EXEC_PATH` seal are preserved. Remaining
performance cost is realistic and bounded: one tree snapshot per retained
commit and one blob process per distinct object. Replacing this with linear
consecutive diffs would violate full reachable DAG history. Content-addressed
archiving and tamper-proof timestamps remain outside the frozen contract.

This body must not be marked runtime Done. Its exact corrective commit still
requires independent parent verification and panel review.
