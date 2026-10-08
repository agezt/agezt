# W2.30j typed agent_set_enabled

`agent_set_enabled` — pause or resume an agent — is the first roster write on the
shared dispatcher. `approster.SetEnabledService` works over four native ports: the
kernel's journaled `SetProfileEnabled`, the two paused-trigger counters (standing
orders and schedules) and the list cache invalidation. `SetEnabledOperations`
declares one primary-only spec that is not read-only, with unknown input allowed and
`POST /api/agents/enable` metadata matching the Web UI write route. Because the spec
is not read-only, shared dispatch requires a successful operation audit admission
before the handler runs; a failed admission blocks the write. The native handler and
its manual `commandSpec` row are removed, and the native wire entry is `AppOwned`,
not `ReadOnly`, not tenant-allowed or routed. `roster_crud.go` drops to zero raw
argument casts, so its ratchet baseline is lowered to 0.

## Preserved behavior

`ref` is required and looked up untrimmed. `enabled` keeps the legacy leniency: a
JSON boolean is used as-is, the strings `"true"` (any case) and `"1"` resume, and
every other value — other strings, numbers, null or absence — pauses. A missing
agent is `unknown agent: <ref>`; a retired one is `agent <ref> is retired — revive it
first`; other store errors pass through. The response is the legacy profile view
(float64 profile numbers, `kind`, `managed`, no status) and, only when resuming, the
counts of the agent's still-paused standing orders and schedules. The list cache is
invalidated after a successful change.

Native and shared audits journal the same records: `op.invoked` with operation,
caller, tenant and redacted summarized arguments, then `op.completed` with duration
or `op.failed` with the error. The intended difference is the shared
already-canceled admission: the legacy path ignored the connection context and
still audited and applied the change, while the shared path rejects before audit and
before any write.

## Runnable comparison and regression evidence

Writes change state, so the native harness clones one closed base kernel directory
(three agents including a managed and a retired one, a paused standing order and a
paused schedule for the first agent) into a fresh pair of kernels for every case,
runs the pre-slice handler on one copy and the registered operation on the other,
and compares both the raw socket responses and the complete journal delta — op audit
records, `roster.updated` events and argument redaction — after normalizing only
wall-clock times, durations and fresh correlation ids. 102 steps repeated twenty
times cover sixteen sequences (pause then resume, every `enabled` encoding, managed,
retired, unknown, padded, missing, blank, typed and null refs, unknown and
secret-named arguments), primary, wrong and tenant tokens, and normal and canceled
contexts. All responses and journals are equal except the 17 primary-token canceled
steps, which return the canceled admission error and leave the shared kernel's
journal empty. The provider is never called.

Permanent tests pin the codec table, error mapping and that failures neither count
triggers nor invalidate, the resume-only counters and legacy profile wire, spec and
output schema, audit admission failure blocking the write, non-primary and canceled
admission, audited success and failure records, the native wire entry, and that the
very next `agent_list` reflects a pause and a resume. Sixteen independent mutations
fail tests and sources are restored byte-for-byte. One further candidate — dropping
the native invalidation port — is an equivalent mutant, because the list cache key
already hashes each profile's enabled flag and update time, so a pause or resume
misses the cache regardless; the cache test pins that observable behavior. Fixtures
use isolated temporary kernels and do not touch the owner's home, wake agents, run a
provider or send channel messages.
