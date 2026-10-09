# W2.36a typed tenant management

The six operator-only tenant operations now run on the shared dispatcher, in the
new package `kernel/app/tenants`.

Audited registry changes:

- `tenant_create` opens a tenant, creating it on first use, and returns its
  token;
- `tenant_release` closes a tenant's kernel and keeps its data;
- `tenant_remove` deletes a tenant and its data;
- `tenant_token` returns an existing tenant's credential.

Read-only operations:

- `tenant_list` lists every tenant on disk and whether it is open;
- `tenant_stats` summarises each tenant's runs.

`Service` works over a `Registry` port (the daemon's `tenant.Registry`) and an
`Activity` port. `Activity` opens one tenant's kernel with `kernelFor` and reads
its runs with `collectRuns`. The native `tenantService` binds a nil registry
when multi-tenancy is disabled, never a typed nil.

`Operations` declares six primary-only specs with unknown input allowed and no
Web UI route. `tenant_stats` keeps its caller-tenant routing, so a named tenant
is still resolved first, as before. The other five use primary tenancy.

The native `tenant_handlers.go` and `registerTenantCommands` are removed.
`tenant.go` now holds only the binding.

## Preserved behavior

- Every operation first reports a disabled registry with
  `multi-tenancy is disabled (no tenant registry configured)`, before reading
  any argument.
- The `id` argument is a strict string (`null` included) and blank is required.
  The id passes untrimmed, and the registry validates it.
- `tenant_create` reports `created` only for a tenant that did not exist. Its
  acquisition is stamped by the daemon clock.
- `tenant_stats` reports, for each tenant:
  - runs, completed, failed and active, where completed takes precedence over
    failed;
  - spend;
  - the latest start, completion or failure time.

  It also reports totals and the count.
- A tenant whose kernel cannot be opened reports only its error. A tenant whose
  runs cannot be read reports its error too, and is released again if it was
  closed. Every successfully summarised tenant that was closed is released
  again, so the summary leaves residency as it found it.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any audit or registry change. Routing may
still open a tenant named on `tenant_stats`, because that happens before
admission on both sides.

## Runnable comparison and regression evidence

Every cloned fixture has three tenants:

- `acme`, open, with run history;
- `beta`, closed, with run history;
- `empty`, closed, with no runs.

The harness runs the pre-slice handlers on one copy and the registered
operations on another. It hides the clone's own path and freshly minted tokens,
and checks separately that `tenant_token` returns `acme`'s own token.

222 steps repeated twenty times cover six sequences under primary, wrong and
tenant tokens, in normal and canceled contexts:

- list, stats, token and re-create of an existing tenant;
- create, token, release and remove of a new tenant;
- release and removal of a closed tenant, and stats after;
- unknown ids;
- every id error, invalid ids and a padded id;
- operator-named and invalid routing tenants;
- secret-named arguments.

The comparison checks three things:

- responses are byte-exact;
- the registry state after each sequence is equal;
- both kernels' journals, grouped by correlation, are equal, including the
  operation audit records.

The harness asserts that the registry and the stats were read, and that create,
release and remove took effect. The exceptions are 36 canceled primary-token
steps. These return the admission error, leave no audit record, and let no
create or remove take effect.

Permanent tests cover:

- the disabled check before arguments;
- every id error, with no registry call on error;
- the untrimmed id, `created`, the clock and every response;
- registry errors;
- the stats rows, the precedence, totals, error rows and exactly which
  tenants are released again;
- the empty wires;
- the specs, including `tenant_stats`' caller-tenant routing, and the output
  schemas.

Native tests cover the registry flags and the disabled daemon. The existing
tenant and tenant-auth suites pass unchanged through the typed path.

Thirty-one independent mutations fail tests. These include the native
registration, the typed-nil guard and the activity conversion. Mutation testing
found one gap, now closed: no native test read a tenant's spend and failures
through the activity port.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
