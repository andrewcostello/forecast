# FC-SOURCES: second bounded source-body correction

## Scope and result

Starting HEAD was `870c9e7ead5577ce07cfcd683b526fae3d887718`, containing the
operator clarification and independent follow-up seals. This correction changes
only `internal/dispatched/sources.go`, private
`internal/dispatched/sourcefile_*.go` platform helpers, and this note. The exact
correction commit is recorded in the dispatcher handoff because a commit cannot
embed its own hash.

The frozen public source seams and parameter names are unchanged. FC-1
`JoinEvidence`/`Build`, the `GIT_EXEC_PATH` policy, existing assertions, tests,
fixtures, scaffold, module files, worklist, and known-red register are unchanged.

The corrected body now:

- opens the confined parent through `os.Root`, then atomically opens only the
  final basename relative to the held parent descriptor/handle. Linux and Darwin
  use `openat` with `O_NOFOLLOW`; Windows uses `NtCreateFile` with
  `RootDirectory`, `FILE_OPEN_REPARSE_POINT`, and a handle-attribute reparse
  check. Every failure path closes its descriptor or handle. Initial and final
  metadata still come from the opened file descriptor;
- keeps readiness ahead of shared-byte reservation without raw `File.Fd()` or a
  blocking-mode change. Platform helpers hold the descriptor with
  `SyscallConn`; cancellation is rechecked on bounded readiness intervals;
- never timer-closes delayed healthy Git output. Buffered output remains
  readable after child exit, while a post-exit pipe held idle by a descendant is
  diagnosed as typed incomplete Git data. Read/Close deliver terminal errors
  once, cancellation reaps descendants, and later Close is benign;
- captures canonical `refs/...` tips and `HEAD`, including detached HEAD, and
  peels only captured object IDs. Implicit validation accepts `HEAD` as the sole
  pseudoref and otherwise applies full Git ref-name restrictions;
- enumerates sorted, deduplicated captured tip IDs in fixed 128-tip batches.
  Every batch passes provider `MaxCommits+1`, omits eager `--topo-order`, and
  uses the default all-parent walk. Global accounting counts each commit once;
  any provider-truncated batch or extra unique commit remains PARTIAL;
- checks non-EOF metadata read failures before parsing a partial commit header
  or timestamp, preserving bound and cancellation identities;
- treats only absent graft metadata as absent. Every other inspection failure or
  unsupported present shape returns typed `ErrGitHistory` and leaves the source
  PARTIAL/error; and
- considers only real direct-child run directories as journal candidates.
  Unrelated files and symlink aliases are not traversed; a symlink-only run is
  therefore unsupported rather than discovered. A candidate directory whose
  `journal.jsonl` is a symlink/non-regular file is still refused, including at
  the final atomic open.

Raw journal events remain intentionally unfiltered in `SourceReadings`;
`ReduceAttempts` owns strict post-cutoff event filtering. Historical reading
cutoff and holdout exclusion remain at the source boundary. Full DAG snapshot
semantics and one `ls-tree` per retained commit are preserved.

## Correction evidence

- `gofmt` and `git diff --check`: pass.
- `go test ./internal/dispatched -run '^TestFCSourcesContract$' -count=1`:
  pass, all 59 old and new Sources cases.
- `go build ./...`: pass.
- `go vet ./...`: pass.
- `go test ./... -race -skip '^(TestFCEvidenceContract|TestFCReferenceCLIContract)$' -count=1`:
  pass, including all 59 Sources cases and all 51 completed Journal cases. Only
  the two named FC-1-owned groups were excluded.
- `GOOS=windows GOARCH=amd64 go build ./internal/dispatched`: pass.
- `GOOS=darwin GOARCH=amd64 go build ./internal/dispatched`: pass.

The final-component descriptor-race evidence is body review of the relative
`openat`/`NtCreateFile` paths plus the immutable static
`F3-SRC-ROOT-ESCAPES` symlink case. No flaky rename-race claim is made.
`F3-BOUND-TOTAL-CONCURRENT` is positive demonstrated old-body mutation evidence:
the old check-then-charge body physically read 130 bytes against a 64-byte cap.
The public seam has no reservation-acquisition hook, so its timing pause is not
claimed as negative proof that two readers simultaneously acquired allowance.

## Latest panel dispositions

These indices are the per-family order from
`2026-09-06T02-15-10Z-FC-SOURCES-corrected-panel`.

| Finding | Disposition |
|---|---|
| `claude-1` | **Fixed.** Implicit snapshots record captured `HEAD` and all canonical `refs/...` tips. Detached-only history is walked. Deduplicated fixed-size tip batches bound argv independently of ref count. |
| `claude-2` | **Fixed.** The unconditional post-Wait pipe-close timer is gone. Readable buffers drain without a wall-clock consumer deadline; post-exit idle inherited pipes produce typed incomplete data, never synthetic EOF. |
| `claude-3` | **Fixed with supported-shape boundary documented.** Non-directory entries, including convenience symlink aliases, are skipped without traversal. Only real direct-child run directories are candidates; their symlink/non-regular final journals are refused. A run available only through a directory symlink is deliberately unsupported and undiscovered. |
| `claude-4` | **Fixed.** Raw `File.Fd()` polling is replaced by `SyscallConn`-held platform readiness checks. No global budget mutex or reservation is held across an idle pipe read. |
| `claude-5` | **Fixed under the operator amendment.** Unix-only code moved to tagged private helpers, with Windows and fail-closed other-platform implementations. The operator had already promoted existing `x/sys v0.40.0` to a direct dependency; no module edit or upgrade was made. Linux, Windows, and Darwin package builds pass. |
| `codex-1` | **Fixed.** The final basename is atomically opened no-follow relative to the confined held parent. Descriptor-derived regular-file and stability validation remains in place. Proof is code review plus the static symlink seal, not a claimed rename-race reproduction. |
| `codex-2` | **Fixed.** Graft inspection ignores only `fs.ErrNotExist`; loops, permission/I/O failures, and non-regular present metadata return typed `ErrGitHistory`, producing retained PARTIAL/error diagnostics. Linked-worktree common-dir behavior remains correct. |
| `codex-3` | **Fixed.** Noncommit fallback peels `<captured-object-id>^{commit}`, never the mutable name, and bound/cancellation errors remain discoverable through wrapping. |
| `codex-4` | **Fixed.** A non-EOF read error is returned before a partial rev-list header/timestamp is interpreted. `F3-BOUND-METADATA-FRAGMENT` passes both cuts as `ErrBoundExceeded`. |
| `codex-5` | **Fixed.** Implicit resolved names use full canonical `refs/...` syntax, with only explicit `HEAD` pseudoref authorization. Explicit requested expressions keep their separate equality/object-ID semantics. |
| `codex-6` | **Immutable seal limitation recorded.** The 100 ms pause cannot prove simultaneous allowance acquisition because the public seam has no hook. The body adds no hook and makes no negative-proof claim; the seal remains useful as the recorded positive 130-byte old-body mutation. |
| `codex-7` | **Corrected independently, unchanged here.** Seal commit `870c9e7` strengthened unborn history to require PARTIAL state, a reason, canonical empty lists, and `ErrSourceIncomplete`. The body passes that immutable case. |
| `grok-1` | **Fixed.** Bounded enumeration drops eager `--topo-order`, retains provider `MaxCommits+1`, uses bounded captured-tip batches, and accounts unique commits globally. A truncated batch remains PARTIAL even if cross-batch duplicates leave retained count below the cap. |
| `grok-2` | **Fixed.** Readiness no longer changes descriptor blocking mode or polls a potentially recycled raw integer. Cancellation interrupts through bounded `SyscallConn` readiness checks; OS-specific APIs are isolated. |
| `grok-3` | **Advisory residual retained.** One `ls-tree` per retained commit and one `cat-file blob` per distinct object remain. Consecutive diffs would not represent every independent merge/side-parent snapshot, and no process-count threshold was frozen. |
| `grok-4` | **Fixed with `claude-1`.** Captured HEAD is explicit in `ResolvedRefs`, its commit participates in deduplicated traversal, and the detached-HEAD seal passes. |
| `grok-5` | **Residual documented.** Common-dir graft inspection is a direct local `os.Stat` after the isolated Git common-dir query; it is not charged as Git bytes/processes and a pathological network filesystem can block an OS metadata call. Adding a leaking goroutine timeout or rejecting linked common dirs would be incorrect. Non-NotExist outcomes now fail closed. |

## Preserved scaffold follow-ups and earlier rulings

| Finding | Disposition |
|---|---|
| `scaffold claude-1` | `GIT_EXEC_PATH` remains inherited exactly as required; other ambient `GIT_*` state remains stripped. |
| `scaffold claude-3` | Strengthened to descriptor-relative atomic final opens and descriptor metadata consistency. |
| `scaffold claude-4` | Carriers, effective bounds, cutoff, holdouts, reports, reasons, and lists remain initialized/canonical on reached outcomes. |
| `scaffold claude-5` | Typed F3 and `ctx.Err()` identities remain wrapped and inspectable. |
| `scaffold claude-7` | Schema-4 `Rounds`/`Limits` remains FC-1-owned; baseline readers are unchanged. |
| `scaffold grok-1` | Frozen Git environment/config policy is retained. |
| `scaffold grok-2` | Cancellation remains context-causal and inspectable. |

The earlier raw-journal-cutoff finding remains rejected by operator ruling. The
first correction's all-ref batching, completeness validation, AllowEmpty,
non-task YAML, structural identity/time, common-dir graft, read-only grammar,
and shared-byte fixes are preserved rather than redesigned.

## Deviations, limitations, and status

Frozen-contract deviation: none. Existing `x/sys v0.40.0` was used without a
module change. Platforms other than Linux, Darwin, and Windows compile against a
private fail-closed helper and reject atomic source opens at runtime rather than
silently use a racy fallback. Journal directory symlink aliases are not source
candidates. Per-commit tree snapshots and direct local common-dir metadata
inspection remain the explicit residuals above.

During local API inspection, `go doc` attempted automatic module proxy metadata
lookups; the restricted sandbox denied every DNS/network operation, so no live
network access succeeded. All build/test commands thereafter explicitly used
`GOPROXY=off` and local toolchains/caches. No credentials, external messages,
subagents, push, fixture/test/scaffold/module edits, or runtime status mutation
were used.

This body is **not Done**. Parent must independently inspect, test, and panel the
exact correction commit before any status transition.
