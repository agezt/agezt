# W2.30l typed agent_task_update

Editing an agent's durable tasklist now runs on the shared dispatcher.
`approster.TaskUpdateService` works over one native port — the kernel's journaled
`UpdateProfile` — and `TaskUpdateOperations` declares a primary-only spec that is
not read-only (`POST /api/agents/task`, matching the Web UI write route), with
unknown input allowed. Shared dispatch therefore requires a successful operation
audit admission before the write. The native handler, its `hasArg` and
`taskFieldPresent` helpers, the manual `commandSpec` row and the
`roster_task_update.go` file are removed. The native wire entry is `AppOwned`, not
`ReadOnly`, not tenant-allowed or routed.

## Preserved behavior

`ref` is required and must be a string. `op` must be a string; it is trimmed and
lowercased, defaults to `update`, and accepts `add`, `update`, `remove` and
`delete`. `args.task` decodes into the roster task with the same
`args.task: <decoder error>` text and type names. The flat `id`, `title`,
`description`, `scope` and `status` arguments must be strings and override the
matching `args.task` fields. A title is required for `add`, and for `update` when
one is provided. Scope and status are checked against the allowed values only when
provided, either flat or as an `args.task` key. Validation runs in that order.

The write goes through `UpdateProfile` even when it ends up changing nothing, so an
unknown or missing task still saves the profile and journals `roster.updated`
(`action: edited`) before the error is returned, as the legacy handler did. Errors
after the write are unchanged and keep their order: unknown agent, then
`args.id required`, then `args.title required` for `add`, then
`unknown agent task: <id>` with the caller's untrimmed id. An update changes a field
when the flat argument is present (which can clear it) or when the value is
non-empty, and reports the task as it was before normalization. A remove reports
the removed task and keeps its neighbours in order. An add reports the stored task,
with the id and timestamps the store assigned. The result is
`{updated, profile, task}`, with the legacy profile view. The legacy handler wrote
`task` as the roster struct, so the native host keeps that struct's declared member
order inside the otherwise sorted object envelope, the same way it handles the
`config_schema` sections and channel media capabilities.

The only intended difference is the shared already-canceled admission, which now
rejects before audit and before any write (the legacy path ignored the context).

## Runnable comparison and regression evidence

The cloned-kernel harness from W2.30j and W2.30k runs the pre-slice handler and
helpers on one copy of a closed base kernel and the registered operation on
another. That base has an agent with two tasks, a plain agent and a retired agent
with a task. The harness compares the decoded responses and the complete journal
delta after normalizing only times, durations, correlation ids and generated ids,
and also compares the raw response bytes after the same normalization, so member
order is checked. 192 steps repeated twenty times cover twenty-eight sequences
under primary, wrong and tenant tokens and normal and canceled contexts:

- add, update and delete in sequence, and repeated same-title adds;
- titles that are blank, flat, or given in `args.task`;
- updates that leave or clear fields, and the missing-id and unknown-task paths
  (with a secret-named argument, to check redaction);
- removing the same task twice;
- invalid, typed and uppercase-padded ops;
- missing, typed, padded and unknown refs;
- typed and invalid `args.task` values, including null and an array;
- invalid scope and status values;
- a retired agent, a duplicate task id, and client-supplied timestamps.

All responses and journals are equal except the 32 primary-token canceled steps,
which return the canceled admission error and leave the shared journal empty.
Removing the host member-order special case makes the harness fail on the first
add.

Permanent tests cover:

- every error before the write (no store call) and after it (store call made);
- that each allowed scope and status is accepted, flat and in `args.task`;
- add, update and remove against a real roster store, including the presence and
  clearing rules, the pre-normalized update result and same-title adds;
- the task's member order;
- spec metadata, and that a failed audit admission blocks the write;
- non-primary and canceled admission, and audited success and failure records;
- the native registry entry and the native wire order;
- that a missed task still journals the profile edit.

Thirty-one independent mutations fail tests, including the native registration and
the member-order special case. The first run showed that no test accepted `total`
as a scope; the acceptance cases above were added and the suite was rerun from the
start. Sources are restored byte-for-byte. Fixtures use isolated temporary kernels.
They do not touch the owner's home, wake agents, run a provider or send channel
messages.
