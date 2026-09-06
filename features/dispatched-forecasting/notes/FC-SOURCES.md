# FC-SOURCES: third bounded source-body correction

## Scope and result

Starting HEAD was `a669f36da57fbd0176ab0799dce928611ca79013`, containing the
operator-corrected orphan-HEAD seal. This correction changes only
`internal/dispatched/sources.go`, private
`internal/dispatched/sourcefile_*.go` helpers, and this note. The exact
correction commit is recorded in the dispatcher handoff because a commit cannot
embed its own hash.

The public source seams, model and ownership pins, FC-1 raw-journal cutoff,
`GIT_EXEC_PATH` policy, tests, fixtures, scaffold, module files, worklist and
known-red register are unchanged.

The corrected body now:

- treats `rev-parse --verify --quiet HEAD` exit 1 only as a request for more
  evidence. The exact admitted `symbolic-ref --quiet HEAD` form must produce a
  syntactically valid canonical `refs/...` target, and exact
  `show-ref --verify --quiet <canonical-ref>` must prove that target absent.
  Only that state is verified unborn. Invalid symbolic syntax, a detached
  unresolved HEAD, and an existing target that still cannot resolve are typed
  `ErrGitHistory` failures. A missing/zero full detached ID continues through
  the captured-ID peel and fails there as it did before;
- retains valid refs and readings when a canonical symbolic HEAD target is
  absent even if other refs exist. Both no-ref and other-ref orphan states are
  PARTIAL with an explicit unborn-HEAD reason. Other refs do not turn a valid
  orphan branch into corruption;
- admits only the two exact new read-only Git argv forms above. Symbolic-ref
  write/delete forms and every other show-ref shape remain
  `ErrInvalidSourceSpec`;
- derives a starting directory descriptor from the held confined `os.Root`,
  then atomically opens every relative directory component against the
  preceding held descriptor. Unix uses `openat` with
  `O_DIRECTORY|O_NOFOLLOW`; Windows uses relative `NtCreateFile` directory
  handles with reparse constraints. The final file is opened no-follow against
  the retained parent handle. Every intermediate handle is closed on success
  and error paths;
- compares a journal run directory opened for the final journal against the
  descriptor identity observed during discovery. A run cannot switch into a
  symlink alias between discovery and open. The static in-root symlink-parent
  seal is the contract evidence; no probabilistic rename-race proof is claimed;
- keeps the Unix final-file type-probe open nonblocking, so regular-file-to-FIFO
  substitution cannot hang the initial open. After descriptor `fstat` accepts a
  regular evidence file, it clears `O_NONBLOCK` before ordinary reads. Git pipes
  retain their runtime-managed readiness behavior;
- reports every direct-child symlink of a journal-runs root as a named source
  reason and PARTIAL, and never traverses it. There is no `latest` exception.
  Ordinary nonsymlink non-directory entries remain ignored; and
- treats caller Close after an already-successful child exit as intentional
  cleanup when its own private I/O cancellation interrupts a held inherited
  stderr pipe. Parent cancellation, byte bounds, nonzero child exit and genuine
  inherited-pipe faults keep their existing precedence and typed errors.
  Exactly-once delivery, delayed buffered stdout, real EOF and bounded cleanup
  remain unchanged.

The cheap local-file metadata advisory was also addressed: already-validated
regular evidence files bypass pipe readiness/fstat checks. Only Git pipes enter
the readiness path.

## Latest panel dispositions

These indices are the per-family order in
`2026-09-06T03-12-17Z-FC-SOURCES-corrected-panel`.

| Finding | Disposition |
|---|---|
| `claude-1` | **Fixed.** Every direct-child journal-root symlink is untraversed, named in the source reasons, and makes that source PARTIAL. No convenience-alias exception was introduced. |
| `claude-2` | **Fixed.** A healthy child followed by caller cleanup no longer reports private `ioCtx` cancellation as incomplete stderr. Genuine process, parent, bound and pipe errors retain precedence. |
| `claude-3` | **Fixed.** The safe helper now uses the `unix` build set; the fallback is `!unix && !windows`. Linux, Darwin, FreeBSD and the additional checked Unix targets compile. |
| `claude-4` | **Advisory addressed cheaply.** Local evidence is known regular at construction and now skips repeated readiness `fstat`; no reader redesign or public metadata seam was added. |
| `codex-1` | **Fixed with the operator distinction.** Exit 1 alone is not unborn proof. Symbolic target syntax and canonical-target absence are verified by exact isolated read-only commands. A canonical absent target is legitimate unborn even with existing refs; invalid syntax or an existing corrupt target is `ErrGitHistory`. The missing detached-object peel was already green and was not misreported as a body fix. |
| `codex-2` | **Fixed.** Every parent component and the final file are opened no-follow from held directory handles, and discovered journal parent identity is checked before the final relative open. |
| `codex-3` | **Corrected independently; unchanged here.** The immutable seal now waits for `processDone`, crosses the former cleanup deadline, and requires both the full payload and nil read error. The corrected body passes it. |
| `grok-1` | **Rejected by explicit operator ruling.** A PARTIAL `MaxCommits` result promises a deterministic bounded traversal-order subset, not global newest-N or HEAD-first retention. Existing bounded tip batches are unchanged. Uncapped COMPLETE still walks all captured refs and detached HEAD. |
| `grok-2` | **Fixed under the operator refinement.** Removing nonblocking from the initial evidence open would reintroduce FIFO-open hangs. The initial no-follow open stays nonblocking, but accepted regular files are switched to blocking mode before reads. |
| `grok-3` | **Advisory residual retained by ruling.** Repeated metadata consumes the actual source byte budget and may make the source PARTIAL. A remaining-unique per-batch cutoff was not added because duplicate prefixes could consume it and lose history behind them. No optimal metadata-cost or total-process threshold is claimed. |

## Corrective seal dispositions

The body passes all independent cases from
`2026-09-06T03-32-34Z-FC-SOURCES-corrective-seals`, including the operator's
later corrected orphan assertion:

| Seal/control | Disposition |
|---|---|
| `F3-HEAD-SYMBOLIC-INVALID/invalid-double-dot-target` | Fixed: typed `ErrGitHistory`, never COMPLETE. |
| `F3-HEAD-SYMBOLIC-INVALID/unborn-with-existing-refs` | Fixed: PARTIAL with reason, no `ErrGitHistory`, valid main ref/readings retained. |
| `F3-HEAD-SYMBOLIC-INVALID/unborn-absence-control` | Preserved green: no resolved HEAD, PARTIAL with reason. |
| `F3-HEAD-SYMBOLIC-INVALID/missing-detached-peel-control` | Preserved green empirical counterexample: rev-parse yields the captured full ID and the peel returns typed `ErrGitHistory`. |
| `F3-GIT-REQUEST-READONLY` | Fixed: exact inspections admitted; write/delete and other variants rejected. |
| `F3-JOURNAL-SYMLINK-CHILD` | Fixed: `latest` is named PARTIAL evidence and is not traversed. |
| `F3-OPEN-SOURCE-SYMLINK-PARENT` | Fixed: the alias parent is atomically refused and the real parent remains readable. |
| `F3-GIT-CLOSE-SELF-CANCEL` | Fixed: Close after healthy exit returns nil despite its own cleanup cancellation. |
| `F3-GIT-BUFFERED-EXIT-READ` | Preserved green: after `processDone` and the delayed read, the full payload and nil read error are delivered. |
| `F3-GIT-CLOSE-ERRORS`, `F3-EMPTY-HISTORY-CONSISTENT`, `F3-DETACHED-HEAD-ALL-REFS` | Preserved green, along with every previously passing Sources case. |

No independent seal contradicts an accepted operator ruling.

## Validation

- `gofmt` and `git diff --check`: pass.
- `GOPROXY=off go test ./internal/dispatched -run '^TestFCSourcesContract$' -count=1`:
  pass, including every Sources and corrective-seal case.
- `GOPROXY=off go build ./...`: pass.
- `GOPROXY=off go vet ./...`: pass.
- `GOPROXY=off go test ./... -race -skip '^(TestFCEvidenceContract|TestFCReferenceCLIContract)$' -count=1`:
  pass, including all owned Sources and completed Journal cases. Only the two
  named FC-1 groups were excluded.
- `CGO_ENABLED=0 GOPROXY=off go build ./internal/dispatched`: pass for
  `windows/amd64`, `darwin/amd64`, `freebsd/amd64`, `aix/ppc64`,
  `dragonfly/amd64`, `netbsd/amd64`, `openbsd/amd64`, and `solaris/amd64`.

No live network, credentials, external messages, subagents, push, cross-target
test execution, or immutable-file edits were used. This body is **not Done**;
the parent must independently inspect and verify the correction commit.

## Preserved scaffold follow-ups and earlier rulings

These IDs are from `2026-09-05T03-44-21Z-FC-SCAFFOLD-correction/nonblocking-review-followups.json`, not the later source panel. This section restores the dispositions accidentally removed by the third-body note rewrite.

| Finding | Disposition |
|---|---|
| `scaffold claude-1` | `GIT_EXEC_PATH` remains inherited exactly as required; other ambient `GIT_*` state remains stripped. |
| `scaffold claude-3` | Strengthened to descriptor-relative atomic parent-component and final opens, with descriptor metadata consistency. |
| `scaffold claude-4` | Carriers, effective bounds, cutoff, holdouts, reports, reasons, and lists remain initialized/canonical on reached outcomes. |
| `scaffold claude-5` | Typed F3 and `ctx.Err()` identities remain wrapped and inspectable. |
| `scaffold claude-7` | Schema-4 `Rounds`/`Limits` remains FC-1-owned; baseline readers are unchanged. |
| `scaffold grok-1` | Frozen Git environment/config policy is retained. |
| `scaffold grok-2` | Cancellation remains context-causal and inspectable. |

The earlier raw-journal-cutoff finding remains rejected by operator ruling. The
first correction's all-ref batching, completeness validation, AllowEmpty,
non-task YAML, structural identity/time, common-dir graft, read-only grammar,
and shared-byte fixes are preserved rather than redesigned.


## Cumulative deviations and remaining limitations

The listed symlink, unborn-HEAD, deterministic capped-subset, platform-helper,
and exact read-only metadata-command rulings are implemented as described. A
separate conservative diagnostic/known-universe limitation remains for
FC-OBS-ADJ and is recorded in the fourth-round section below; it is not hidden
by a blanket no-deviation claim. The original body and correction notes are
preserved by commit and in the operator deviation-history index attached to
FC-OBS-ADJ. Earlier policies superseded by those rulings are historical, not
current promises.

Per-commit tree snapshots and repeated metadata charges remain permitted costs.
Local common-dir graft metadata inspection is an ordinary OS filesystem call;
no hard deadline on a pathological filesystem is claimed. Linux runtime tests
and the listed cross-platform package builds are the portability evidence;
cross-build success is not a claim of runtime testing on those other platforms.

## Fourth bounded source-body correction

Starting HEAD was `3662401d64739a7781fe357d76a590ec66bf95e6`, containing
the parent-owned seal and fixture follow-up. This correction remains limited to
`internal/dispatched/sources.go`, the private Windows source-file helper, and
this note. Tests, fixtures, scaffold, module files, worklist, known-red register,
public seams, capped-subset/HEAD behavior, and Git lifecycle behavior are
unchanged.

The fourth correction:

- translates only `windows.NTStatus` returned by `NtCreateFile` through its
  existing `Errno()` mapping and returns an `os.PathError` with operation and
  component path. Standard Windows not-found identities therefore remain
  reachable through `errors.Is(..., fs.ErrNotExist)`. Non-NT errors remain the
  wrapped cause; reparse refusal, parent identity mismatch, and access errors
  are not converted to successful absence;
- preserves the journal run `Info` error identity with `%w` while keeping the
  existing fail-closed inspection policy; and
- adds source ID, repository, and run context to returned journal-open errors
  while preserving the original typed error with `%w`.

### Fourth corrected-panel dispositions

Finding indices below are the per-family order from panel
`/home/andrew/Project/dispatcher-runs/2026-09-06T04-03-09Z-FC-SOURCES-corrected-panel/panel.md`.
The adjacent per-seat records contain the nonblocking findings omitted from the
short cross-family rendering.

| Finding | Disposition |
|---|---|
| `claude-1` (MEDIUM) | **Residual for FC-OBS-ADJ; no policy change here.** A present but unreadable held-out journal can surface as an unmatched selection because discovery bookkeeping follows successful open. This conservatively refuses extraction rather than admitting held-out evidence to training. Unconditionally appending before open was rejected because it would invent a journal for every empty run directory, contradicting `F3-JOURNAL-MISSING-CHILD` and the known-universe contract. A correction requires a metadata-confirmed unreadable-journal seal and contract decision. Final adjudication must either accept this limitation with a reason or require a corrective row. |
| `claude-2` (MEDIUM) | **Corrected independently; body already correct.** The parent strengthened the immutable symlink seal to require a nil returned error, the exact retained real-run journal, and a report reason naming the symlink. Its hard-refusal mutation fails. No test change is included here. |
| `claude-3` (LOW) | **Fixed.** Returned journal-open failures now identify source ID, repository, and run, with `%w` preserving typed errors. This is diagnostic context only. |
| `claude-4` (LOW) | **Documented independently.** The seal author recorded `processDone` as an explicit structural dependency of the two Git lifecycle seals. A future reader redesign must re-derive that synchronization; no body or public-seam change is made here. |
| `codex-1` (MEDIUM) | **False; no body change.** At reviewed commit `55a552d`, lines 1812-1819 record the duplicate-run error but do not return; execution falls through to `ParseEvents` and `reader.Close` at lines 1820-1821. The alleged descriptor-leak path does not exist. |
| `codex-2` (MEDIUM) | **Fixture defect corrected independently by the parent.** The immutable fixture now uses finite loops, exits if its state directory vanishes, and acknowledges leaving the loop and closing inherited pipes before `TempDir` cleanup. This proves fixture cleanup, not that `Close` kills arbitrary non-child descendants; direct Git child reaping remains the frozen body duty. The behavioral `Close` assertion is unchanged, and the parent re-ran the old-body mutation to show it still fails. No test change is included here. |
| `grok-1` (HIGH) | **False for the production reader; no runtime filesystem policy added.** Local Go 1.26 primary code at `/usr/lib/go-1.26/src/os/dirent_linux.go:28-50` maps `DT_UNKNOWN` to `^FileMode(0)`; `/usr/lib/go-1.26/src/os/dir_unix.go:138-150` calls `newUnixDirent`; and `/usr/lib/go-1.26/src/os/file_unix.go:468-485` falls back to `lstat` and fills `typ` and `info` before returning. Thus `Type()==0` is regular, not unresolved unknown, for this real `os.Root.FS` reader. There is no injected arbitrary-FS seam. |
| `grok-2` (HIGH) | **Fixed.** The native `NtCreateFile` status is converted with `windows.NTStatus.Errno()` and wrapped in `os.PathError`; `F3-JOURNAL-MISSING-CHILD` remains green on Linux. Windows runtime behavior was established by code correction and compile checking only, not runtime execution. |
| `grok-3` (MEDIUM) | **Fixed at the reviewer-permitted minimum.** Journal run inspection now wraps `infoErr` with `%w`, retaining both `ErrSourceMissing` and the underlying identity. Arbitrary unreadable candidates are still fail-closed rather than silently skipped. |

### Fourth correction validation

- `gofmt` and `git diff --check`: pass.
- `GOPROXY=off go build ./...`: pass.
- `GOPROXY=off go vet ./...`: pass.
- `GOPROXY=off go test ./... -race -skip '^(TestFCEvidenceContract|TestFCReferenceCLIContract)$' -count=1`: pass, including `internal/dispatched`; only the two named FC-1 groups were excluded.
- `CGO_ENABLED=0 GOPROXY=off go build ./internal/dispatched`: pass for
  `windows/amd64`, `darwin/amd64`, and `freebsd/amd64`.

No Windows, Darwin, or FreeBSD runtime was available. Cross-builds do not prove
runtime behavior on those platforms. No network, credentials, external
messages, subagents, push, immutable-file edits, or runtime Done transition
were used.
