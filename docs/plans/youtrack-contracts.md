# YouTrack support contracts

## Y1 — compatibility and architecture

`forecast [--config PATH] jira create` and every existing flag/output remain a
contract. Preserve existing Jira behavior and configuration defaults. Add
`forecast issue create` and `forecast issue transition` as neutral operations;
`issue sync` can compose the transition path. The bridge currently invokes
`jira transition`, not `jira sync`.

A legacy `jira` command selecting a YouTrack instance fails clearly before any
mutation. Its help points to the neutral command. Jira-only capabilities stay
explicitly Jira-only; the epic does not implement every Jira administration,
sprint, attachment, or worklog operation on YouTrack.

Put provider-neutral request/result types and small reader/creator/transition
capabilities in `internal/tracker`. Keep JQL, ADF and provider wire types out of
that domain package. Derive needed methods from callers, including web and
status/sync, and record the call-site/capability inventory. Preserve public
Jira request/response compatibility using aliases/adapters where needed.

Use an importable factory under `internal/tracker/provider`, shared by CLI and
web. It may import Jira and YouTrack; neither implementation imports it. The
neutral contract package imports neither concrete provider. This prevents an
import cycle while keeping shared DTOs independent. `cmd/forecast/helpers.go`
wraps that factory; web cannot import a Go main package.

Allow `jira.NewClient` in the Jira implementation/tests and the one named
provider factory. No other production caller constructs directly. This amends
the former zero-outside-Jira assertion, which contradicted the shared-factory
requirement. Do not hide construction behind a renamed function to pass grep.

## Y2 — configuration and routing

One resolver handles instance selection, project selection and key-prefix
routing. Missing kind preserves Jira for every legacy config fixture. A typo
in kind is an error. Separate YouTrack URL/permanent-token/project settings
from Jira email/API-token settings. Preserve the old unknown-prefix fallback;
reject ambiguous duplicate prefix ownership rather than relying on map order.
An explicit project/instance and a supplied ticket key that disagree fail
before writing. Document how no-key creates select the project.

YouTrack custom fields are mapped by configured name and validated against
the selected project's metadata/type. Units and precedence are explicit:
cycle-time override is hours; no silent conversion from an unrelated field.

## Y3 — transport, read path, and history

YouTrack authentication uses a bearer permanent token. Keep raw Markdown
through the entire description path. DO NOT PORT ADF. Request selected fields
explicitly, distinguish internal IDs from UI-readable IDs, and return the
readable ID to callers. Issue collection pagination and activity-history
pagination have different contracts; the selected endpoints determine which
pagination mechanism is used. Detect repeated pages/cursors and malformed
responses. Cancellation, finite timeouts, and bounded response handling apply.
Authentication failure, missing resource, valid empty result and malformed
response remain distinct; no error becomes an empty successful forecast.

Transport and shared wire structures land before concurrent read/write bodies.
Bodies own separate files: transport, history, read mapping, transition, create.
All seals run without network or real credentials using injected transports,
not localhost servers requiring network access. Fixtures identify their source
(API documentation or sanitized recorded response) and schema/version; never
claim synthetic fixtures were recorded from a live project.

Freeze one field-by-field `forecast.Item` equivalence table with the same
logical readable key on both sides. Preserve compatibility fields such as
`JiraKey` while adding a neutral provider; renaming persisted fields is not
part of this epic. Normalize times and statuses explicitly. Include missing
history, timezone offsets, reopened issues, custom-time override precedence,
and closed-by attribution. Match the frozen Jira behavior for equivalent
fixtures; any discovered Jira defect is adjudicated separately, not changed
incidentally in the YouTrack adapter. Fetch complete history before calculating
cycle time; partial history is a named failure/quality state.

## Y4 — writes and retry semantics

Freeze a capability matrix for summary, type, Markdown description, epic,
parent, labels, priority, story points, due date, assignee, fix versions and
components. Mapped fields must have fixture-proven YouTrack semantics. A field
without a safe mapping is rejected by NAME before any mutating request; do not
invent a mapping simply to make a test green. Validate all supplied fields
before creating, so a post-create validation error cannot leave an orphan.

Return readable key plus canonical URL. Returning an empty key is failure.
Do not retry a create after an ambiguous timeout/connection loss. Return a
named uncertain-write result and the evidence needed for reconciliation.

There are two distinct replay requirements:
1. With a successfully persisted valid tracker key, a rerun creates nothing.
2. After remote success but before local key persistence, the current bridge
   has no exactly-once protocol. Ordinary POST plus a key check cannot prove
   no duplicates in that crash window.

Preserve (1) as the existing Jira contract. Treat (2) as a release blocker for
any stronger retry guarantee: YT-SCAFFOLD must document supported server
idempotency or propose stable operation IDs plus durable reconciliation and
name the required dispatcher change. YT-ADJ cannot assert universal duplicate
prevention without a fault-injection test of that protocol. No automatic
retry is an interim safety behavior, not a claim of exactly-once delivery.

Status transition uses the caller's target status and optional resolution/
comment; no second hardcoded dispatcher status map. Already-at-target is a
no-op for the state change. Comment replay is a separate mutation and must
have a defined dedupe policy or be refused explicitly. Unsupported resolution
or comment semantics are named errors before state mutation.

## Y5 — actual dispatcher bridge contract

The old opaque-key assumption is false in the inspected
`claude_dispatcher/forecast_bridge.py`:
- `jira_key_of` validates `_JIRA_KEY_RE`;
- `parse_create_output` uses `_CREATED_RE`, which can accept a matching prefix;
- `create_missing_tickets` persists `jira_key` after processing the loop;
- `sync_terminal_statuses` builds `jira transition KEY --to STATUS`, optionally
  `--resolution` and `--comment`.

Copy generated create/transition argv and parser fixtures from a recorded
bridge revision. Do not infer flags from prose or rename the persisted
`jira_key` field in another repo. The forecast adapter's readable ID must
round-trip exactly, not truncate to a parser prefix.

The final bridge report is a compatibility matrix, not a predetermined answer
that only a subcommand needs changing. Selecting `issue` must cover create
AND transition. Additional changes may be required for key validation/parsing,
resolution/comments, or crash-safe create replay. Any dispatcher implementation
lands in its own repo/review. A required external change remains an explicit
release prerequisite, never quietly omitted from YT-ADJ's verdict.

## Y6 — offline proof and live adjudication

All new-capability seals are red against the contract stubs; existing Jira
regression seals remain green and are mutation-verified. No xfail, t.Skip,
credential-dependent skip, real token, or live HTTP call belongs in a seal.
Token redaction and transport injection are exercised as behaviors.

YT-ADJ is the only task allowed to touch a real YouTrack instance. Before it
runs, the operator supplies URL, project, and a locally stored permanent token
reference. Log the reference name, never the token. Create one authorized
ticket, record its readable key, read it back, exercise agreed transitions,
and forecast from that project's history. Insufficient project history blocks
that acceptance item; it does not license generated historical data. Keep the
created ticket as evidence unless cleanup is separately authorized.

During YT-ADJ, reconcile documentation/synthetic fixtures against sanitized
real responses and archive those recordings; do not rewrite sealed assertions
to hide an API mismatch. A mismatch requires a contract ruling and corrective
seals/body work.

Final acceptance requires all offline seals green without exclusions, every
bodies deviation explicitly ruled, and the live/bridge evidence complete.

## API sources checked for this redesign

JetBrains documents bearer-token authentication in
[Permanent Token Authorization](https://www.jetbrains.com/help/youtrack/devportal/authentication-with-permanent-token.html),
issue versus activity pagination in
[Pagination](https://www.jetbrains.com/help/youtrack/devportal/api-concept-pagination.html),
and typed, named custom fields in
[Create an Issue and Set Custom Fields](https://www.jetbrains.com/help/youtrack/devportal/api-howto-create-issue-with-fields.html).
Use the exact selected endpoint documentation when recording each fixture.
