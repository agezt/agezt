# W2.40c typed execution profiles

The three execution-profile reads now run on the shared dispatcher in the new
`kernel/app/execprofile` package. This completes the provider-config group.

- `execution_profiles` returns the inventory of profiles a kernel can route
  work to, with host and count totals.
- `execution_profile_show` returns one profile.
- `execution_profile_check` returns the health check of each profile and
  policy.

`Service` works over two ports:

- an inventory port, built on every call from the routed kernel's tools and
  warden and the host's SSH, K8s, Modal and Daytona configuration;
- the profile policy read from the environment.

`Operations` declares three read-only, unaudited specs with unknown input
allowed. They keep the native tenancy: owning-tenant authorization with
caller-tenant routing. A tenant token reads its own kernel; the operator reads
the primary or a named tenant. The inventory is on `GET
/api/execution_profiles` and the check on `GET /api/execution_profile_check`;
the single-profile read has no Web UI route, as before.

The inventory builder and `toolNames` move to `execution_inventory.go`. `run`'s
"not routable" error now builds its supported-profile list through the same
helper.

Removed with the move:

- `execution_profiles.go`, which held the three handlers and their row
  builders;
- `registerProviderConfigCommands`, which had no native command left.

The capability comparison's evidence list in `cmd/agt/compare_data.go` now
cites the new package and the shared inventory file in place of the removed
one; its existence check had caught the stale path.

## Preserved behavior

- **Profiles:**
  - Every field is always present, including an empty degrade reason, policy
    capability and notes.
  - Empty tool, backend, limit and note lists are null.
  - The secret policy appears only when the profile carries one.
- **Show:**
  - `id` is strict: absent or blank is "required", and a present non-string,
    null included, is "must be a string".
  - The id is checked before the inventory is built, then trimmed.
  - An unknown id keeps its error text.
- **Check:**
  - The inventory is diagnosed under the policy read at call time.
  - Every check field is always present, including an empty next step and
    backend.
  - The routable run profiles are null when there are none.
- **Tenancy:**
  - A tenant token must name its own tenant; otherwise it is refused.
  - A padded tenant is trimmed, and a mistyped one reads the primary.
  - An unknown tenant named with the primary token is provisioned by the
    registry, on both sides.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any read.

## Runnable comparison and regression evidence

The harness runs the pre-slice handlers on one kernel and the registered
operations on another. Each run opens a primary kernel with the shell tool and
a tenant whose kernel has none, so every read shows whose tools built the
inventory. 264 steps repeated twenty times cover two fixtures:

- no remote configuration;
- SSH, K8s, Modal and Daytona configured, with a deny policy for the warden
  profile and a metadata secret policy.

Each fixture runs 22 steps under primary, wrong and tenant tokens, in normal and
canceled contexts. The steps include:

- inventories with primary, named, padded, mistyped, empty and unknown
  tenants;
- every refused id form;
- padded, local, warden, SSH and Docker ids, including on a named and an
  unknown tenant;
- checks on the primary, a named and an unknown tenant.

The comparison checks two things:

- responses are byte-exact, with no masking;
- both journals, grouped by correlation, are equal.

The harness also asserts:

- that the primary reads its own kernel and each tenant read reads the
  tenant's;
- that a mistyped or empty tenant reads the primary and an unknown one is read
  after provisioning;
- that a tenant token without its tenant is refused;
- the id errors, the configured SSH profile, the secret policy and the
  routable profiles.

The exceptions are 52 canceled steps for authorized tokens. These return the
admission error.

Permanent tests cover:

- **List:** every field, null empty lists, the secret policy, and an empty
  inventory.
- **Show:**
  - every refused id;
  - that a refusal never builds the inventory;
  - the trimmed lookup.
- **Check:**
  - the report copied check by check, and the policy read;
  - every field present;
  - a backend found on a temporary PATH, gated by platform.
- **Operations:** the specs and input and output schemas.
- **Binding:** primary and named-tenant reads through the native adapter, with
  the primary and tenant tokens, and the registry flags.

The existing execution-profile and live-policy suites pass unchanged through
the typed path.

Thirty-five independent mutations fail tests. One first survived and closed a
real gap: dropping the check's `backend` field had no test, because no fixture
found a backend on PATH. It now has a test.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
