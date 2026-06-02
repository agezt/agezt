# M128 — Tenant self-observability authorization

## Why
This is the same class of latent bug as M127 (an inventory that must stay in sync
with the code but silently drifted) — here in a **security-sensitive** allowlist.

`tenantTokenAllows` is the deny-by-default gate for what a tenant token may do.
Its own comment promises it holds *"exactly the commands that route to the
caller's kernel via kernelFor/edictFor."* But it had drifted: over many milestones
new tenant-routed observability commands were added, and only **some** were added
to the allowlist. The result — a tenant operating its own fully-isolated kernel
could read its runs / tools / edict / webhooks / rate-limit / netguard, but was
**denied** its own:

- memory log, world log
- approvals log + stats
- plan history + stats
- provider log + stats + rejections
- schedule fires + stats
- warden log + stats

All 13 are read-only handlers that fold the tenant's **own** journal via
`kernelFor(tenantOf(req))` — exactly the shape already allowlisted for runs/tool/
edict. Excluding them was an oversight, not a policy: a tenant was wrongly denied
its own data.

## What
- **Audited each handler** (memory_log / world_log / approvals_log / plan_history /
  provider_log / schedule_fires / warden_log) to confirm it reads **only** the
  kernel returned by `kernelFor(tenantOf(req))` — never `s.k` (the primary) or
  shared state — so granting access exposes only that tenant's own data. This is
  the load-bearing safety check; verified by grep + reading each handler body.
- **Granted** the 13 read-only commands in `tenantTokenAllows`, grouped and
  commented as tenant self-observability.
- **Held the boundary**: cross-tenant `CmdTenantStats` (M126) and durable-policy
  compaction `CmdEdictCompact` (a mutation) stay primary-only. Tenant-registry
  management and daemon-global halt/resume/shutdown remain primary-only as before.

The existing isolation guarantees carry over unchanged: every granted handler uses
the same per-tenant routing already proven by `TestRunsAreTenantScoped` /
`TestWhyIsTenantScoped` (a tenant sees only its own journal, never the primary's).

## Files
- `kernel/controlplane/tenant.go` — `tenantTokenAllows` expanded; comment rewritten
  to document the principle and the boundary.
- `kernel/controlplane/tenant_auth_test.go` — `TestTenantToken_AllowsOwnObservability`
  (all 13 reach their handler, no "forbidden"); `TestTenantToken_ForbidsNonAllowlistedCmd`
  extended to lock `CmdTenantStats` + `CmdEdictCompact` as primary-only.

## Proof
- Unit test through the **real token-auth path** (a tenant `Client` with its own
  token): all 13 commands now succeed on the tenant's own kernel; `tenant_stats`
  and `edict compact` are forbidden.
- Live (multitenant daemon, tenant token): `agt tenant stats` with a tenant token →
  `unauthorized` (boundary holds). NOTE: the daemon authorization is now correct,
  but the agt CLI does not yet expose `--tenant` on these specific observability
  subcommands (memory/world/provider/… log/stats) — that CLI surface is the
  immediate follow-up (M129). The grant is reachable today by any control-plane
  client that sends `{tenant: <id>}` with a tenant token (as the unit test does)
  and by the API/header-routed tenant paths.

## Verification
- 55 packages `ok`, **FAIL 0**; **1417 tests**.
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: both touched files clean under LF.
