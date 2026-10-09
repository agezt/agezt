# W2.31b typed targeted run cancel

`cancel_run` now runs on the shared dispatcher, next to the steering commands in
`kernel/app/steer`. `Service.Cancel` works over the routed kernel's `Runs` port,
which gains `CancelRun`. `Operations` adds a tenant-owned, caller-tenant-routed
spec that is not read-only (`POST /api/cancel_run`, matching the Web UI write
route), with unknown input allowed. Shared dispatch therefore requires a
successful operation audit admission before the run is cancelled.

The native `handleCancelRun` and its registration in `registerCoreCommands` are
removed. The native wire entry is `AppOwned`, not `ReadOnly`, and tenant-allowed
and routed.

## Preserved behavior

- `correlation` is required. It must be a non-blank string, and it is passed on
  untrimmed.
- `tenant` must be a string when present, `null` included. It is checked after
  the correlation, as before, while routing still follows the trimmed tenant.
- An unknown run, or a run of another kernel, reports `cancelled:false`.
- Cancelling leaves the kernel un-halted and every other run untouched.
- An invalid tenant id fails native routing before any argument check, on both
  sides alike.

## Audit label fix

The common adapter passed the raw tenant argument as the caller tenant while
routing trimmed it. An operator-selected padded tenant (`" acme "`) therefore
wrote its audit record into the `acme` journal labelled `" acme "`. The adapter
now passes the trimmed tenant, so the label names the kernel the write reached.
This affects every caller-tenant-routed operation.

The tenant-token path was already right: its argument is pinned to the
authorized tenant, trimmed. `TestAppHostTenantAuditUsesRoutedJournal` gains the
padded-tenant case. With the adapter change disabled it fails with
`tenant: acme ` in the record; with the change it passes.

## Runnable comparison and regression evidence

The W2.31a harness runs the pre-slice `handleCancelRun` on one cloned kernel and
the registered operation on another. Each clone has a live run blocked inside its
model call in both the primary kernel and the `acme` tenant kernel. Three things
are compared:

- decoded and normalized raw responses;
- both kernels' journals, grouped by correlation;
- the provider call counts.

156 steps repeated twenty times cover 19 sequences under primary, wrong and tenant
tokens and normal and canceled contexts:

- a cancel and its repeat;
- a tenant-routed cancel;
- cross-kernel and foreign-tenant attempts;
- a cancel between a pause and a resume;
- unknown runs;
- every correlation and tenant codec error;
- padded, empty and invalid tenants;
- a padded correlation and secret-named arguments.

The harness asserts that the primary and the tenant cancel really cancelled. All
responses, journals and call counts are equal except the following:

- 29 canceled authorized, routable steps return the admission error, and neither
  journal holds an op audit.
- The five operator-selected tenant audit records carry the W2.1 tenant label,
  now trimmed.

Permanent tests cover:

- the codec errors before any port call, including the tenant after the
  correlation;
- the untrimmed correlation and the exact wire;
- the spec with its route, audit admission and tenant routing;
- the native registry entry;
- the two-kernel native socket test, which adds a tenant-token cancel that cannot
  reach a primary run and the strict tenant error.

The existing `TestCancelRunViaControlPlane` suite still runs end to end through
the new path.

Nine independent mutations fail tests, including the codec order, the port, the
wire key, the spec name and route, and the adapter trim. Sources are restored
byte-for-byte. Fixtures use isolated temporary kernels and scripted mock providers
only.
