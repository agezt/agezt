# W2.62 the boundary rows have one declaration

Three rows crossed the boundary between the host (the runtime, the tenant
registry, the kernel) and the application, and each of them was described
**twice** — once where it is produced and once where it is published — and each
copy was filled in **field by field**:

| where | the row | the copy |
|---|---|---|
| `kernel/runtime.Opinion` — the panel's position in one round | seat, model, round, text, error | — |
| `kernel/app/council.OpinionRow` — the council operation's answer | seat, model, round, text, error | five fields, per opinion |
| `kernel/tenant.Info` — the registry's listing snapshot | id, base dir, open | — |
| `kernel/app/tenants.Row` — the tenant operation's listing | id, base dir, open | three fields, per tenant |
| `kernel/contract/types.SubAgentLimits` — the delegation ceilings (M46–M48) | enabled, depth, fan-out, spend, total | — |
| `kernel/app/system.DelegationStatus` — the status operation's row | enabled, depth, fan-out, spend, total | five fields |
| `plugins/tools/introspecttool.Delegation` — the introspection tool's row | enabled, depth, fan-out, spend, total | five fields |

With this slice each of the three rows is declared **once**, in the contract
layer (`kernel/contract/types`), and every other side is an alias of it. The
three field-by-field copies are gone.

## What moved

- **the council opinion** — `types.Opinion` (new) carries the row's json names.
  `runtime.Opinion` is an alias of it (the runtime keeps its public name), and
  `app/council.OpinionRow` is an alias of it, so the panel's opinions reach the
  operator **without a projection**: the answer's opinion list is now a copy of
  the runtime's own slice.
- **the tenant row** — `types.TenantInfo` (new) carries the row's json names.
  `tenant.Info` is an alias of it (the registry keeps its public name) and
  `app/tenants.Row` is an alias of it, so the listing publishes the registry's
  rows **unchanged**.
- **the delegation ceilings** — `types.SubAgentLimits` (already the shared
  value type since W2.58) now carries the row's json names. `app/system.DelegationStatus`
  and `introspecttool.Delegation` are aliases of it, so the status operation and
  the introspection tool publish the kernel's ceilings **as they are**.

Three imports were added, all downward (module → contract, application →
contract, plugin → contract), all legal under the layer rule:

| package | layer | imports |
|---|---|---|
| `kernel/tenant` | L2 (platform/tenancy) | `kernel/contract/types` (L1) |
| `kernel/app/council`, `kernel/app/system` | L4 | `kernel/contract/types` (L1) |
| `plugins/tools/introspecttool` | L6 | `kernel/contract/types` (L1) |

## What did not change

**No wire changed.** Every row keeps the same json names, in the same order,
with the same `omitempty` behaviour, and every value is the value it was:

- the council answer's `opinions` — `seat`, `model`, `round`, `text` and the
  omitted `error`;
- the tenant listing's `tenants` — `id`, `base_dir`, `open`;
- the status operation's `delegation` and the introspection tool's `delegation`
  — `enabled`, `max_depth`, `max_fanout`, `max_spend_microcents`, `max_total`.

The **empty-list rule** is kept as well: an empty panel, an empty registry and
an empty deliberation are still empty arrays (`[]`), never `null` — the copies
are `make([]T, len(src))` plus `copy`, so the destination is non-nil even when
the source is empty.

The `SubAgentLimits` type is now a **wire** row as well as a value type. It was
never marshalled before this slice (the runtime hands it over as a value and
both sides read its fields), so the tags are a promise the wire already kept.

## Runnable evidence

Permanent tests cover:

- the **identity** of every side that now shares a declaration: the runtime's
  opinion, the application's opinion row, the registry's row, the application's
  tenant row, the application's delegation row and the tool's delegation row are
  the contract types (`reflect.TypeOf` comparisons, so a copy that drifts is
  caught, not just a renamed field);
- the **wire** of each row: the exact json keys, the omitted empty `error` and
  the round-trip (`kernel/contract/types/types_test.go`);
- that the values **flow unchanged**: the council answer carries the panel's
  opinions (seat, model, round, text and the failed seat's error), the tenant
  listing carries the registry's rows (id, base dir and the open flag), the
  status operation carries the kernel's effective ceilings, and the live
  kernel's adapter carries them into the introspection tool's overview.

The mutations fail tests: **14 mutations, 14 caught** — the renamed wire keys
(`seat`, `model`, `error`, `base_dir`, `open`, `enabled`, `max_depth`,
`max_spend_microcents`), the `error` that stops being omitted, the answer that
drops the opinions, the answer that keeps only the first one, the listing that
drops the tenants, the listing that keeps only the first one, the status row
that reports no ceilings and the introspection tool that reports no ceilings.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (271 packages, 129 allowlisted import violations and 13
allowlisted call sites — the ratchet is unchanged), `tools/structure-md` (no
new package), `tools/changelog-lint`, `tools/docclaimscheck` and staticcheck.
The affected suites — the contract types, the council, the tenants, the status
operation, the tenant registry, the introspection tool and the council tool —
pass at count=20.

## Not done here

The **application's own artifact row** (`kernel/app/artifacts.Entry`) and the
**data-lake's records** (`kernel/datalake`, `kernel/app/data`) still have their
own declarations and their own spellings. They are their own wires: the
application's artifact row spells the correlation `CorrelationID` and keeps the
empty keys, and the data-lake's rows are always-present keys on the store's
side and omitempty on the application's — unifying those changes a wire or a
disk format, which is a separate decision.

The **reaper's rows** (`kernel/app/reaper` against `kernel/runtime`) still have
two declarations: the runtime's rows omit the empty optional fields and the
application's rows always carry every field. Same shape, different promise.

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of this
slice, as in W2.44b through W2.61. The runtime and control-plane suites are the
parity proof here.
