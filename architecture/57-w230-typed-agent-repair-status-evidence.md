# W2.30g typed agent_repair_status and cursor current-state fix

`agent_repair_status` is now a shared application operation.
`approster.RepairStatusService` folds the selected journal into one agent's
autonomous self-repair history over roster lookup, journal range, cooldown and clock
ports; the cooldown port is the native `AGEZT_AUTO_REPAIR_COOLDOWN` reader, which
the status collector still shares. `RepairStatusOperations` declares one
primary-only, read-only spec with unknown input allowed and
`GET /api/agents/repair_status` metadata, matching the Web UI read route. The native
handler, its manual `commandSpec` row, the contract/next-action/decision view
helpers (`roster_activity_views.go`) and the row view helpers are removed, and the
now-empty `roster_activity.go` is deleted. The native wire entry is `AppOwned`,
`ReadOnly`, not tenant-allowed or routed. Arguments share W2.30f's `RefPageRequest`
(ref → unknown agent → limit order, lenient sequence cursor) with this command's 20
default and 100 cap.

## Bug fixed: paging moved the agent's current state

The legacy handler paged history with `filtered := rows[:0]`, which writes the kept
rows into the full row list's own backing array. `latest`, top-level
`next_eligible_ms` and `next_action` were then read from that overwritten list,
although the handler's own comment says they must describe the full list and not
move with the page. With a cursor that skips newer rows, `latest` reported an older
repair, and the cooldown decision pointed at the wrong fingerprint and eligibility
time — the agent page showed a wrong current repair state while paging.

`TestAgentRepairStatus_CursorKeepsCurrentState` (native socket, five repair events,
second page) failed on the unchanged native handler three times out of three
(page 2 reported `fp-2` instead of `fp-4` for `latest`, `next_eligible_ms` and the
cooldown `next_action`), and passes now. The service filters the page into its own
slice. A mutation that restores the in-place filter fails the permanent tests.

## Typed output

`RepairStatusOutput` types every field: slug, cooldown seconds, the repair contract
(retry, doctor, self-repair, escalation and the authority boundary text), history
and inflight rows (always arrays, `[]` when empty), counts, `next_cursor` only when
truncated, `latest` and `next_eligible_ms` only when any row exists, and
`next_action`. `RepairRowOutput` keeps all 31 row fields with nil string lists as
`null`. `RepairNextAction` always carries action, label, detail and tone; its
optional fields are pointers so the decisions that report them keep present-but-empty
values (an inflight row without a correlation id still emits `"correlation_id":""`),
exactly as the legacy maps did.

## Runnable comparison and regression evidence

The native harness runs the pre-slice handler and views (reconstructed from full-path
snapshots) against the registered operation on the same initialized kernels. 756
cases repeated twenty times cover three kernel states (empty; two agents; journaled
queued, completed, rollback-queued, failed and foreign-agent repair events with HTML
text, lists and large integers), normal and canceled contexts, primary, wrong and
tenant tokens, twenty-one argument shapes and two reads. The populated history is
checked to contain the agent's rows, one inflight fingerprint and none of the other
agent's. 628 complete raw socket responses are byte-equal; 126 differences are
exactly the primary-token canceled admissions; and 2 are the cursor fix — the only
cursor that skips newer rows. For those, every field except `latest`,
`next_eligible_ms` and `next_action` is equal, and those three equal the same
request without a cursor. Journal head/hash and provider call counts stay unchanged.

Permanent tests pin row and inflight folding (subject, kind, agent and undecodable
payload filters; latest queued row per fingerprint), history order, totals, paging,
cursor/limit edges with the 20/100 limits, the current state under a cursor, the
empty shape, the contract defaults and policy overrides, every next-action decision
and its present-empty fields, escalation-target precedence, decision detail parts,
spec metadata, admission and the native wire entry. Twenty-nine independent mutations
fail tests and sources are restored byte-for-byte. One further candidate — swapping
the target-agent and target-correlation row fields — is an equivalent mutant on this
path, because repair-status rows never set either field; it is recorded and
excluded. Fixtures use isolated temporary kernels and do not mutate owner profiles,
wake agents, run a provider or send channel messages.

The first full gate run stopped only at staticcheck: `parseSeqCursor` lost its last
callers when activity and repair status moved, so the dead helper and its import were
removed and the full gate run was repeated from the start on the changed inputs.
