# W2.30h typed agent_escalations

`agent_escalations` is now a shared application operation.
`approster.EscalationService` reports the help requests addressed to one agent or
to everyone, with their doctor/delegation wake metadata, over three ports: roster
lookup, a narrow `EscalationBoard` read view (help topic and per-message replies)
and the selected journal range. The native adapter `nativeEscalationBoard` wraps the
host's `*board.Store` and keeps the existing 500-message read cap, so the shared
board instance and its open-on-read fallback are unchanged. `EscalationOperations`
declares one primary-only, read-only spec with unknown input allowed and
`GET /api/agents/escalations` metadata, matching the Web UI read route.

The native handler, its manual `commandSpec` row, the row fold, the text source
parser, the `agentEscalationRow` type and the now-empty `roster_escalation_rows.go`
are removed. `BoardMessageAckedBy` is exported from the application package and the
two remaining native callers (status escalation load and lifecycle helpers) use it.
`argLimit` lost its last caller with this move, so `args_specialized.go` is deleted.
The native wire entry is `AppOwned`, `ReadOnly`, not tenant-allowed or routed.

## Preserved behavior

Arguments use the shared `RefPageRequest` with this command's 20 default and 100
cap, and the legacy error order: ref, unknown agent, limit, then opening the board
(its error is returned), and only then the strict cursor — a present non-string
cursor, including null, is `args.cursor must be a string`. A string cursor decodes as
`"<ts_unix_ms>:<message_id>"`; an unparsable timestamp means no cursor, while
`0:<id>` still filters by id. The journal fold keeps the newest metadata per mailbox
message for this target agent and ignores undecodable payloads and blank message ids.

Rows keep the legacy status precedence (answered over acked over open),
case-insensitive acknowledgement, delegated versus doctor origin, source agent from
metadata or else parsed from "Doctor … for agent X." text, root agent defaulting to
the source, newest-first stable order, the same-timestamp message-id tie-break and
`next_cursor` only when truncated. `open_count` counts the returned page, as before,
and an empty result is `[]`. The intended wire change is the shared already-canceled
admission, which now rejects before the board or journal is read.

## Runnable comparison and regression evidence

The native harness runs the pre-slice handler, fold, text parser, row type and
`argLimit` (reconstructed from full-path snapshots) against the registered operation
on the same initialized kernels and one shared board. 756 cases repeated twenty
times cover three kernel states (empty; two agents; help requests to the agent, to
everyone, to another agent, a plain post, a reply, an acknowledgement, journaled
doctor and delegation metadata including a foreign target), normal and canceled
contexts, primary, wrong and tenant tokens, twenty-one argument shapes (every ref,
limit and cursor edge including typed and null cursors) and two reads. The populated
result is checked for open, answered, acked and delegated rows and no foreign help
request. 630 complete raw socket responses are byte-equal; the 126 differences are
exactly the primary-token canceled admissions. Journal head/hash and provider call
counts stay unchanged.

Permanent tests pin the fold (help-only, recipients, statuses, reply counts,
acknowledgement case, latest metadata per message, every metadata field, origins,
text-derived source and root, the text parser edges), argument and error order with
board-open before cursor validation, limits, cursor tie-breaks and lenient decoding,
the empty shape, spec metadata, admission and the native wire entry. Twenty-eight
independent mutations, including the native help-topic adapter and registration,
fail tests and sources are restored byte-for-byte. One further candidate — not
trimming the mailbox message id — is an equivalent mutant, because generated board
ids are never blank; it is recorded and excluded. Fixtures use isolated temporary
kernels and boards and do not mutate owner profiles, wake agents, run a provider or
send channel messages.
