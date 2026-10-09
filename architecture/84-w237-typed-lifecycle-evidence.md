# W2.37a typed lifecycle operations

The four operator-only lifecycle operations now run on the shared dispatcher in
`kernel/app/system`, beside `status` and `version`:

- `halt` stops new runs, recording a reason;
- `resume` allows new runs again, recording a reason;
- `journal_verify` checks the primary journal's hash chain;
- `shutdown` acknowledges, then schedules the daemon's exit.

`Lifecycle` works over a `Kernel` port (`HaltWith`, `ResumeWith`, `Verify`) and a
shutdown function, both bound to the primary kernel and server.
`LifecycleOperations` declares four primary-only, primary-tenancy specs with
unknown input allowed:

- `halt` on `POST /api/halt`, audited;
- `resume` on `POST /api/resume`, audited;
- `journal_verify`, read-only, with no route;
- `shutdown`, audited, with no route.

The two routes match the Web UI write routes.

Removed with the move:

- the native `handleHalt`, `handleResume`, `handleVerify` and `handleShutdown`;
- their registrations.

`shutdown.go` now holds `scheduleShutdown`, which keeps the acknowledgement
grace delay before it closes `shutdownCh`.

## Preserved behavior

- `halt` and `resume` read `reason` strictly: it may be absent, but a present
  `null` or non-string is an error. The reason passes untrimmed. A bad reason
  changes nothing.
- `halt` reports `{"ok":true,"halted":true,"reason":…}` and `resume` reports
  `halted:false`. Halting does not gate operations, only runs, so `resume`
  works while the kernel is halted.
- `journal_verify` returns the verifier's error, or `{"ok":true}`.
- `shutdown` returns `{"ok":true}`. After the grace delay it closes
  `shutdownCh` once, however many requests arrive.
- Any `tenant` argument is ignored, and tenant tokens are refused. This matches
  the native operator-only registration.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any audit, halt, resume or exit.

## Runnable comparison and regression evidence

The harness clones fixture kernels, each with an open `acme` tenant. It runs the
pre-slice handlers on one copy and the registered operations on another. After
each sequence it waits past the grace delay and records three things: the
primary's halt state, `acme`'s halt state, and whether the exit fired.

108 steps repeated twenty times cover six sequences under primary, wrong and
tenant tokens, in normal and canceled contexts:

- halt and resume with and without reasons, then verify;
- every reason error;
- tenant arguments;
- secret-named arguments;
- a lone shutdown;
- a halt followed by two shutdowns.

The comparison checks three things:

- responses are byte-exact;
- the halt and exit state is equal;
- both kernels' journals, grouped by correlation, are equal: the halt and
  resume events and the operation audit records.

The harness asserts the untrimmed reason, the verification, the final unhalted
state and the scheduled exit. The exceptions are 18 canceled primary-token
steps. These return the admission error and leave no audit record, halt,
resume or exit.

Permanent tests cover:

- halt and resume with the untrimmed reason, every reason error, and no kernel
  call on error;
- verification success and the error passthrough;
- shutdown scheduling exactly once;
- the specs and output schemas.

Native tests cover the registry flags. The existing halt, resume and shutdown
suites pass unchanged through the typed path.

Sixteen independent mutations fail tests, including the native registration and
the shutdown binding. One equivalent is recorded: signalling the exit before the
grace sleep instead of after. It only moves when `shutdownCh` closes relative to
the client's read. Test servers do not exit on that close, and asserting "still
open right after the acknowledgement" would race the 50 ms delay on a slow
runner. The native handler had the same untested window.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
