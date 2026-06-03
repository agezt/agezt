# M219 — Per-tenant mesh peer sets (federated mesh × multi-tenant)

## Why
ROADMAP defines v1.0 (M8) as "federated mesh + multi-tenant". Both halves existed
separately — the mesh (`remote_run`, M200–M218) and tenant isolation (M14/M38/M39) — but
they did not meet: the `remote_run` tool held one **global** peer set shared by every
tenant, so all tenants delegated to the same nodes. A real multi-tenant deployment needs
each tenant to federate to its **own** peers (different data residency, different trust
domains, different capability nodes). This milestone is that intersection.

## The leak-safe design
The hard part is doing this without a cross-tenant leak. Two decisions make it safe by
construction:

1. **Tenant identity comes from the KERNEL, not HTTP.** A tenant run can be triggered by
   a REST call, the OpenAI API, a schedule, or a channel message — only some carry an HTTP
   tenant header. Injecting identity at the HTTP layer would miss the others. Instead the
   tenant's kernel (which inherently *is* that tenant) stamps its id onto every run's
   context. So identity is correct on every path, and it inherits the correctness of the
   existing per-tenant kernel binding (the M14/M38 isolation foundation).
2. **Fallback is always to the GLOBAL set, never another tenant's.** A run with no tenant
   id (primary), or a tenant with no configured override, uses the global peers. A
   misattributed or absent id can therefore only ever degrade to the primary's peers —
   structurally impossible to reach a *different* tenant's set.

## What
- **`kernel/tenantctx`** (new package) — `WithTenant(ctx, id)` / `Tenant(ctx)`. An empty
  id is a no-op, so the primary kernel never tags a run.
- **`kernel/runtime`** — `Config.TenantID` (empty for primary); `RunWith` stamps it onto
  the run context (`ctx = tenantctx.WithTenant(ctx, k.cfg.TenantID)`) before deriving the
  run/timeout context, so it propagates through to every `tool.Invoke`.
- **`plugins/tools/peer`**:
  - `TenantPeers map[string]map[string]Peer` on the `Tool`; `NewWithTenants(global, tenant)`
    (and `New` delegates to it) — returns the tool when *either* global or tenant peers
    exist.
  - `peersFor(ctx)` selects the run's effective set: the tenant's own when it has one,
    else global. `routeCandidates` / `serversForModel` / `resolve` / error messages all use
    the effective set (resolve and peerNames are now set-parameterised).
  - The discovery cache is keyed by peer **URL** (not name), so the same peer name across
    different tenant sets can't return another set's models.
  - `ParseTenantPeers(spec)` decodes the `AGEZT_TENANT_PEERS` JSON map, parsing each value
    with `ParsePeers` (same per-tenant validation).
- **`cmd/agezt/main.go`** — the tenant `OpenFunc` sets `tcfg.TenantID = id`; the tool is
  built with `NewWithTenants(ParsePeers(AGEZT_PEERS), ParseTenantPeers(AGEZT_TENANT_PEERS))`;
  registration line notes the override count.
- **`kernel/controlplane/config.go`** — `AGEZT_TENANT_PEERS` registered for `agt config show`.

Single-tenant deployments are unchanged: empty `TenantID` ⇒ `Tenant(ctx)==""` ⇒ global
peers, byte-for-byte the prior behaviour.

## Tests (+8)
- `kernel/tenantctx/tenantctx_test.go` — round-trip, default empty, empty-id no-op.
- `plugins/tools/peer/peer_tenant_test.go`:
  - `TestParseTenantPeers` — JSON map parsed + per-tenant validated; bad JSON / bad peer
    spec error (naming the tenant).
  - `TestRemoteRun_TenantUsesOwnPeers` — alpha's run hits alpha's peer, beta's hits beta's.
  - `TestRemoteRun_UnknownTenantFallsBackToGlobal` — a tenant with no override **and** the
    primary both use the global peer, never another tenant's (the leak-safety guarantee).
  - `TestRemoteRun_TenantErrorScopedToTenantPeers` — an unknown-peer error lists the
    tenant's own peers, not the global/other set.
- `kernel/runtime/tenant_test.go` — `TestKernel_StampsTenantOnRunContext`: a tenant kernel
  stamps its id onto the run ctx (a tool invoked during the run reads it); the primary
  stamps "". Proves the end-to-end kernel→tool path.

## Verification
- `go test ./...` — 1693 passing (1685 + 8 new), 0 failing.
- `go vet` clean on all touched packages.
- `gofmt -l` (CRLF-normalized) clean on all touched/new files.
- `GOOS=linux go build ./...` — clean.
- `go.mod` / `go.sum` unchanged (stdlib-only).
- Local commit only (no push); standard trailer.

## Files
- `kernel/tenantctx/tenantctx.go` (+ test) — new.
- `kernel/runtime/runtime.go` — `Config.TenantID` + `RunWith` stamp.
- `kernel/runtime/tenant_test.go` — kernel-stamp test.
- `plugins/tools/peer/peer.go` — `TenantPeers`, `NewWithTenants`, `peersFor`,
  `ParseTenantPeers`, URL-keyed cache, set-parameterised resolve/serversForModel.
- `plugins/tools/peer/peer_tenant_test.go` — tenant-isolation tests.
- `cmd/agezt/main.go` — tenant `TenantID` + tool wiring + `AGEZT_TENANT_PEERS`.
- `kernel/controlplane/config.go` — register the env var.

## Mesh thread (M8) — capstone
M200–M218 built the mesh (discovery, routing, failover, loop-guard, observability, config
hygiene) and M14/M38/M39 the multi-tenant isolation; **M219 fuses them** — the federated
mesh now respects, and is partitioned by, the tenant boundary. The kernel-level
`tenantctx` is a reusable primitive any future tenant-aware tool can read.
