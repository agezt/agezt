# M212 — Audit a tenant's loop-refusal on the tenant's own bus

## Why
M210 publishes a `mesh.loop_refused` event when the M209 hop guard stops a federation
loop. But it always published to the server's **primary** bus. In a multi-tenant
deployment a delegated run can target a specific tenant (`X-Agezt-Tenant`); when such a
run is loop-refused, the event landed on the *primary* tenant's journal/pulse, not the
target tenant's. Two problems: the tenant operator watching their own `agt pulse` never
saw their own mesh refusal, and a tenant-scoped safety event leaked onto the primary's
stream — blurring the per-tenant isolation that M14/M38/M39 establish. This is squarely
the federated-mesh × multi-tenant intersection that v1.0 (M8) is about.

## What
`kernel/restapi/restapi.go` (`handleRunsRoot`, hop-limit refusal path):
- Before publishing the audit event, the handler now resolves the request's target bus
  via the existing `s.bind(r)` (which maps `X-Agezt-Tenant` → that tenant's Engine +
  bus). The event is published to the **resolved tenant bus** when the tenant resolves,
  and falls back to the **primary bus** for a header-less request or an unknown/invalid
  tenant.
- The `508 Loop Detected` outcome is unchanged — resolving the bus is purely for routing
  the audit event; a bad tenant header still yields the same refusal (the bind error is
  ignored for the fallback).

Net: each tenant's loop refusals appear on that tenant's own journal/pulse, consistent
with how the rest of a tenant's run events are already isolated.

## Tests (+1)
`kernel/restapi/mesh_hop_test.go`:
- `TestMeshHop_RefusalAuditedToTenantBus` — a multi-tenant server (primary + tenant
  `alpha` with its own bus via `SetTenantResolver`). A POST with `X-Agezt-Tenant: alpha`
  and a hop past the limit returns `508`, and the **tenant** bus receives the
  `mesh.loop_refused` event while the **primary** bus receives nothing (asserted by
  subscribing to both `mesh.>` streams).

The M210 single-bus audit test (`TestMeshHop_RefusalIsAudited`, no tenant header → primary
bus) and the rest of the hop suite remain and pass.

## Verification
- `go test ./...` — 1675 passing (1674 + 1 new), 0 failing.
- `go vet ./kernel/restapi/` — clean.
- `gofmt -l` (CRLF-normalized) clean on touched files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only).
- Local commit only (no push); standard trailer.

## Files
- `kernel/restapi/restapi.go` — resolve the tenant bus for the refusal audit event.
- `kernel/restapi/mesh_hop_test.go` — tenant-bus audit test.

## Mesh thread (M8) so far
- **M200** bounded peer health read · **M201** `agt peers models` · **M202**
  `remote_run {model}` · **M203** auto-route · **M204** `agt peers route` · **M205**
  discovery cache · **M206** failover · **M207** `agt doctor` mesh check · **M208**
  `agt status` mesh config · **M209** loop guard · **M210** loop-refusal audit · **M211**
  tunable hop limit · **M212** tenant-scoped loop audit (this milestone).

The mesh loop-guard is now enforced, tunable, and audited to the correct tenant — the
guard fully respects the multi-tenant boundary. Per-tenant *peer sets* (a tenant
federating to its own mesh) remain a larger, separately-scoped change deliberately left
for later.
