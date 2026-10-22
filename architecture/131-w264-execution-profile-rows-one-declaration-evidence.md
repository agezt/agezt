# W2.64 the execution profile's rows have one declaration

The execution profile's rows — the profile, the health check and the secret
policy — were described **twice**: once in the module that builds them
(`kernel/executionprofile`) and once in the operation that publishes them
(`kernel/app/execprofile`) — and the operation's copy was filled in **field by
field**.

| the row | the module's declaration | the operation's declaration | the copy |
|---|---|---|---|
| the profile | `executionprofile.Profile` | `execprofile.Profile` | twenty-one fields |
| the health check | `executionprofile.HealthCheck` | `execprofile.Check` | ten fields |
| the secret policy | `executionprofile.SecretPolicy` | `execprofile.SecretPolicy` | six fields |
| the inventory | `executionprofile.Inventory` | `execprofile.ListOutput` | seven fields |
| the health report | `executionprofile.HealthReport` | `execprofile.CheckOutput` | eight fields |

With this slice the rows are declared **once**, in the module, and the
operation publishes them as they are.

## What moved

- **The operation's five types are the module's five types.** `Profile`,
  `Check`, `SecretPolicy`, `ListOutput` and `CheckOutput` are aliases of the
  module's `Profile`, `HealthCheck`, `SecretPolicy`, `Inventory` and
  `HealthReport`. The thirty-seven field copies are gone.
- **The module's rows carry every field.** The profile's `degrade_reason`,
  `policy_capability` and `notes` and the check's `next`,
  `backend_available` and `backend` lost their `omitempty`: the rows say what
  they are, and the empty ones are explicit rather than missing.
- **The profile row keeps the operator's key order.** The secret policy comes
  last on the wire, the way the operator reads it; the module's row now
  declares it last too.

## What did not change

**No wire changed.** The operation's answers keep the same keys, in the same
order, with the same values:

- the profile — every field present, the unset lists `null`, the secret policy
  last (and absent when the profile carries none);
- the health check — every field present, the empty optional fields explicit;
- the inventory and the health report — the same keys, the same counts, the
  routable profiles `null` when there are none.

The **list rule** stays the operation's own: an empty list is `null` on the
wire, and an empty profile list is an empty array (`[]`), never `null`. That
rule is now the only thing the operation does to a row — it normalises the
lists instead of rebuilding the row.

The **module's rows were never marshalled** — the inventory reaches the
operator through the operation's typed operation, and the run resolution and
the warden read the fields — so the `omitempty` change is invisible outside
the module.

## Runnable evidence

Permanent tests cover:

- the **identity** of every type that now shares a declaration: the
  operation's five types are the module's five types (`reflect.TypeOf`
  comparisons, so a copy that drifts is caught, not only a renamed field);
- the **wire** of every row in the module: the exact json of the profile, the
  health check and the secret policy, key by key, with the empty fields
  explicit and the secret policy last;
- the **values that flow unchanged**: the profile's identity, its tools and
  the list rule (an empty list is null), and the operation's exact answers
  (the pre-existing list, show and check tests, unchanged).

The mutations fail tests: **14 mutations, 14 caught** — the three omitted
profile keys, two swapped profile keys, the three omitted check keys, the
omitted secret-policy detail, the four lists that are not nulled and the two
rows that are emptied.

The mutation run found a **test gap** and it is fixed and named: the
operation's fixture exercised the null rule for the profile's tools and notes
only, so the backends and the limits were not covered. The fixture now carries
empty backends and limits too, and both mutations are caught.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (271 packages, 129 allowlisted import violations and 13
allowlisted call sites — the ratchet is unchanged), `tools/structure-md` (no
new package), `tools/changelog-lint`, `tools/docclaimscheck` and staticcheck.
The affected suites — the module, the operation and the control plane — pass
at count=20; the two commands pass at count=5.

## Not done here

The **application's own artifact row** (`kernel/app/artifacts.Entry`) and the
**data-lake's records** (`kernel/datalake`, `kernel/app/data`) still have
their own declarations and their own spellings — they are their own wires.

The **introspection tool's reaper rows** keep their compact form: the model
sees the empty optional fields omitted, which is the tool's wire.

The **skill's metrics row** (`kernel/app/skill` against
`kernel/skill`) still has two declarations: the store's row is the on-disk
format and omits the empty fields, the operation's row always carries them.
