# W2.31a typed live run steering

The five live run steering commands now run on the shared dispatcher:

- `run_pause`
- `run_resume`
- `run_step`
- `run_steer`
- `run_intervene`

The new package `kernel/app/steer` owns their strict codecs, the `note`/`steer`
mode, the intervention request mapping (lease in milliseconds, scope, idempotency
key) and the typed outputs. They work over a `Runs` port, which is the routed
kernel's live-run control surface.

`Operations` declares five tenant-owned, caller-tenant-routed specs that are not
read-only, with unknown input allowed:

- `POST /api/run/pause`, `/api/run/resume`, `/api/run/step` and `/api/run/steer`,
  matching the Web UI write routes;
- `run_intervene`, which has no HTTP route, as before.

Shared dispatch therefore requires a successful operation audit admission before
any run is touched. The control-plane binding hands the service the kernel the
dispatcher routed to. A tenant still steers its own runs without the primary
token, and the primary token reaches a tenant's run only by naming the tenant.

`steer.go`, `registerSteerCommands` with its registry call and the now-unused
`argFloat64` are removed. The native wire entries are `AppOwned`, not `ReadOnly`,
and tenant-allowed and routed.

## Preserved behavior

- `correlation` is required on every command. It must be a non-blank string, and
  it is passed on untrimmed.
- `run_steer` requires a non-blank, untrimmed `directive`. `mode` must be a string
  when present. Only `"note"` is a soft BTW; anything else, `"NOTE"` included, is
  a forceful steer. The response always echoes `steer` or `note`.
- `run_intervene` reads `primitive`, `directive`, `scope` and `idempotency_key` as
  strict optional strings, in that order. `lease_ms` must be a number, and only a
  positive value becomes a lease. The kernel validates the primitive, and its
  errors are returned unchanged. `lease_expires_unix` appears only when the kernel
  reports a lease expiry.
- An unknown run reports `ok:false` (or `accepted:false`) rather than an error.
- The primary token without a tenant argument steers only primary runs. A tenant
  token is accepted only for its own tenant.

Two differences are intended:

- The shared already-canceled admission rejects before audit and before any run
  is touched.
- An operator-selected tenant write is audited in the tenant journal with its
  tenant label. This is the W2.1 host behavior that
  `TestAppHostTenantAuditUsesRoutedJournal` pins; the legacy native audit left the
  label off for the primary token.

## Runnable comparison and regression evidence

The harness runs the pre-slice `runCorr`, the five handlers and `argFloat64` on
one cloned kernel and the registered operations on another. Each clone has a live
run blocked inside its model call in both the primary kernel and the `acme`
tenant kernel. Placeholders resolve to each side's own run correlations. After
the steps, both sides cancel the runs the same way and let them finish. Three
things are compared:

- decoded and normalized raw responses;
- the journals of both kernels, grouped by correlation, covering the op audit, the
  run control and intervention events, and the runs through cancellation;
- the provider call counts.

222 steps repeated twenty times cover 21 sequences under primary, wrong and tenant
tokens and normal and canceled contexts:

- pause and resume twice, and stepping twice;
- forceful and note steering;
- tenant-routed pause, steer (`NOTE`) and resume;
- cross-kernel and foreign-tenant attempts;
- halt with a lease and its idempotent replay, plus query, redirect, adjust,
  abort and an invalid primitive;
- unknown runs;
- every codec error, a secret-named argument, and an unknown tenant combined with
  argument errors.

All responses, journals and call counts are equal except the following:

- 42 canceled authorized steps return the admission error, and neither journal
  holds an op audit or a control event.
- The five operator-selected tenant audit records carry the tenant label. The
  harness adds the label to the legacy record, asserts that it was absent there,
  and requires at least one such case.

Permanent tests cover:

- every control codec error before any port call, and the untrimmed correlation;
- the directive and mode rules;
- the intervention field order and errors, the lease conversion, request
  passthrough, the result mapping and the optional expiry;
- the five specs with their routes and schemas, audit admission blocking a steer,
  and tenant dispatch;
- the native registry entries;
- a native socket test with live blocked runs in the primary and tenant kernels.
  It proves each token reaches only its routed kernel, the primary token reaches
  the tenant run only by naming the tenant, and the final pause and pending
  states are right.

The existing native steering suites still run end to end through the new path.

Thirty-three independent mutations fail tests. They include the native
registration, the routed-kernel binding, each spec's tenancy, authorization and
unknown-input flags, and two routes. One equivalent was recorded: labelling the
note branch with the raw mode, which is always `"note"` there. Sources are
restored byte-for-byte.

Fixtures use isolated temporary kernels and scripted mock providers only.
