# W2.34b typed live approvals

The two live approval operations now run on the shared dispatcher in
`kernel/app/approvals`:

- `approvals` lists the waiting requests;
- `decide` grants or denies one of them as the operator.

`Live` works over a `Broker` port (`Pending` and `Resolve`), bound to the primary
kernel's approval registry. `LiveOperations` declares two primary-only,
primary-tenancy specs with unknown input allowed:

- `approvals`: read-only and unaudited, on `GET /api/approvals`;
- `decide`: audited, on `POST /api/decide`.

Both routes match the Web UI routes.

Removed with the move:

- the native `handleApprovals` (`server_commands.go`);
- the native `handleDecide` (`server_handlers_plan.go`);
- their registrations.

With W2.34a, the whole approvals domain is typed.

## Preserved behavior

- `approvals` returns the registry's waiting requests oldest first. Each row
  carries the full intent and effect metadata. Times are Unix seconds. Unset
  effect lists and regret axes stay `null`. An empty list is `[]` with a count
  of 0.
- `decide` reads `id`, `decision` and `reason` leniently: a non-string is empty.
  The checks run in this order:
  1. an empty id is `args.id required`, before the decision is checked;
  2. the decision must be exactly `grant` or `deny`.
- The id passes untrimmed. The resolution is recorded as `operator`, and an
  unknown or already-resolved id returns the registry's error.
- Tenant tokens are refused both operations. Any `tenant` argument is ignored.
  This matches the native operator-only registration.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any audit or decision, so the waiting request
stays pending.

## Runnable comparison and regression evidence

Every cloned primary kernel holds three live waiting requests, submitted in a
fixed order:

- one with every metadata field set;
- a bare one;
- a third.

The harness runs the pre-slice handlers on one copy and the registered
operations on another. The waiting submitters are released the same way on both
sides. Approval ids and wall-clock times are normalized.

114 steps repeated twenty times cover five sequences under primary, wrong and
tenant tokens, in normal and canceled contexts:

- list, grant, re-list and a repeated decision;
- deny with a non-string reason;
- a wrongly cased decision;
- non-string, missing and blank ids, and a padded id;
- tenant arguments;
- secret-named arguments;
- an unknown id and a non-string decision.

The comparison checks two things:

- responses are equal, both decoded and as normalized raw bytes;
- both kernels' journals, grouped by correlation, are equal: the requests,
  their resolutions and the operation audit records.

The harness asserts the full metadata row, the grant and the shrinking list. It
also asserts the deny, the refusal of the padded id, and the error on a repeated
decision.

The only exceptions are 19 canceled primary-token steps. These return the
admission error and leave no audit record and no grant.

Permanent tests cover:

- the full and the bare pending rows, with the `null` wire, and the empty list;
- every `decide` error in order, with no resolution on error;
- the untrimmed id and the operator resolver;
- a non-string reason;
- the registry error;
- the specs and output schemas.

Native tests cover the registry flags. They also cover a live round trip:
a tenant token is refused both operations, the operator lists and denies a
waiting request, the submitter sees the operator's denial, and a repeated
decision returns the unknown-approval error.

Twenty-three independent mutations fail tests, including the native
registration and the broker binding. One equivalent mutant is recorded: echoing
the decision constant instead of the request's word is the same string, because
only `grant` and `deny` reach the echo. A broker bound to the server's own
kernel would also be equivalent, since primary tenancy always routes there.
Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
