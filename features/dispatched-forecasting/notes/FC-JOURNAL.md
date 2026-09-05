# FC-JOURNAL: journal attempt implementation

## Contract section

Implemented the frozen F1–F2 seams in `internal/dispatched/journal.go`:

- `ParseEvents` applies bounded streaming reads, producer declaration resolution,
  strict scalar validation, sequence retransmission/collision handling, and full
  physical event citations.
- `ReduceAttempts` uses producer line order for attempt membership and closing
  model selection, then emits canonical attempt/evidence order. It preserves
  censored elapsed independently of phase completeness, separates corrections
  from review/verifier invocations, keeps available citations for incomplete
  measurements, and withholds complete overlap components.
- `SummarizeWall` validates canonical, contained, disjoint classified intervals
  and computes unclassified time only as the elapsed residual.
- `Attempt.Censored` and the Attempt JSON methods enforce declared textual
  outcomes and wall/parent consistency.

## Reason

The implementation replaces the FC-SCAFFOLD `ErrNotImplemented` bodies without
changing their frozen signatures. Physical producer order and portable output
order are deliberately separate: line order determines event meaning, while the
handoff's total citation order makes output deterministic under input permutation.

## Affected callers

- FC-SOURCES can pass parsed journal streams to `ReduceAttempts` while retaining
  parser and reducer diagnostics.
- FC-1 can reconcile normalized attempts, inspect exact field/event provenance,
  exclude blocked and unfinished elapsed from completed-duration samples through
  `Attempt.Censored`, and serialize schema-4 textual outcomes.

## Seal evidence

- `go test ./internal/dispatched -run '^TestFCJournalContract$' -count=1` — pass.
- `go test ./internal/dispatched -run '^TestFCJournalContract$' -race -count=1` — pass.
- `go test ./internal/dispatched -run '^TestFCJournalContract$' -count=25` — pass.
- `go build ./...` — pass.
- `go vet ./...` — pass.
- `go test ./... -race -skip '^(TestFCSourcesContract|TestFCEvidenceContract|TestFCReferenceCLIContract)$' -count=1` — pass. The skipped groups belong to unfinished FC-SOURCES/FC-1 bodies; the owned FC-JOURNAL group ran.

## Exact commit

No implementation commit was created in this session. `git commit` failed because
the managed worktree's shared Git metadata could not create `index.lock` on the
read-only filesystem. The implementation remains in the working tree for the
dispatcher to commit, as its instructions permit. Starting HEAD was `34abd04`.

## scaffold_review_followups

| Finding | Disposition |
|---|---|
| `claude-2` | **Not adopted.** The accepted `F2-PHASES-INFERRED-LABELED` contract remains unchanged: caller-supplied `Inferred=true` is retained even with empty evidence, while the 0.1.0 reducer emits only recorded-boundary/duration intervals with `Inferred=false`. Requiring citations or excluding inferred spans would amend the frozen contract. |
| `claude-6` | **Documentation only.** `AttemptSet.Diagnostics` carries parser counts plus reducer additions. Numeric consumers must add only the reducer delta to already-counted source diagnostics; boolean/list facts combine idempotently. No behavioral reinterpretation was made. |

## Deviation

None.

## Residual limitation

Reduction intentionally supports only the frozen producer semantics for exact
dispatcher version `0.1.0`. Missing, conflicting, or unsupported producer evidence
is refused rather than guessed. Missing or ambiguous finer phase boundaries leave
`Wall.Complete=false`; known total elapsed and its censoring status remain usable.
