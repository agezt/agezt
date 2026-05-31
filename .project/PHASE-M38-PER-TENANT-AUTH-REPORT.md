# Phase Report — Milestone M38 (Per-tenant authenticated control-plane access)

> Status: **shipped** · Date: 2026-05-31
> SPEC-14 (tenancy/security). A fresh-axis turn back to the tenant-isolation
> story (M14, M22): isolation existed for *execution* (per-tenant kernel,
> journal, governor, Edict) but not for *control* — every operation still
> required the primary token. M38 closes that gap.

## Why

`agt tenant create` already mints a persistent per-tenant token
(`.tenant-token`, 0600), and `tenant.Registry.Authorize(id, token)` already does
a constant-time check of it. The design intent — "the daemon admin token
authorizes any tenant; the per-tenant token authorizes that tenant" — was even
documented in the registry. But the control-plane surface never wired the tenant
half: `handleConn` only accepted `req.Token == s.Token()` (the primary token), so
a tenant token was a credential that unlocked nothing. A multi-tenant operator had
to hand out the **primary** token — which authorizes *everything, on every
tenant* — to let a tenant manage its own policy. That breaks isolation on the
control side.

## What shipped

- **Tenant-token authentication + authorization (`kernel/controlplane/server.go`,
  `handleConn`)** — when `req.Token` is not the primary token, the request must
  name a tenant AND present that tenant's own token; `s.tenants.Authorize(tenant,
  token)` (constant-time) gates it. On success the principal is that tenant and is:
  1. **confined to an allowlist** of tenant-routed commands — `tenantTokenAllows`
     (deny-by-default): `run`, `cancel_run`, and all seven `edict` subcommands;
  2. **pinned to its own tenant** — the `tenant` arg is forced to the authorized id
     so no handler can be tricked into acting elsewhere.
  Anything else (a different tenant, tenant-registry management, daemon-global
  halt/resume/shutdown/pulse, or the primary-journal run stats) is denied.
- **`tenantTokenAllows` (`kernel/controlplane/tenant.go`)** — the explicit
  allowlist. It contains exactly the commands that route to the caller's kernel via
  `kernelFor`/`edictFor`; a new tenant-routed command must be added here
  deliberately, and forgetting is the *safe* failure (deny, not over-grant).
- **`AGEZT_TOKEN` client override (`kernel/controlplane/client.go`)** — `NewClient`
  honours `AGEZT_TOKEN`, so `AGEZT_TOKEN=<tenant-token> agt --tenant X edict show`
  connects to the primary control plane but authenticates as tenant X. Falls back
  to the on-disk primary token (single-tenant default unchanged).

The primary token path is **untouched**: it authorizes every command on every
tenant exactly as before. M38 only *adds* a second, strictly-scoped credential
class.

## Design decisions

- **Deny-by-default allowlist, not a denylist.** A forgotten new command must fail
  closed (tenant denied), never fail open (tenant over-granted). The allowlist is
  the minimal set that is both tenant-routed *and* safe; run stats are excluded
  because they read the **primary** journal (`collectRuns` walks `s.k`), and
  halt/shutdown are daemon-global.
- **Client asserts identity (tenant id + token), server verifies — no reverse
  index.** Rather than a token→tenant lookup table, the request carries the tenant
  id and the server checks the presented token against *that* tenant's token with
  the existing constant-time `Authorize`. A tenant trying to target another tenant
  presents its own token against the wrong id → `Authorize` fails → unauthorized.
- **Pin the tenant arg.** Even though the arg was the thing authorized, the handler
  is given a freshly-pinned value (defense in depth) so a future code path can't
  observe a divergent tenant.
- **`unauthorized` vs `forbidden`.** A bad/empty/mismatched credential →
  `unauthorized` (authentication failed). A *valid* tenant credential invoking a
  non-allowlisted command → `forbidden` (authenticated, not permitted). In the CLI,
  commands that send no `--tenant` arg under a tenant token deny as `unauthorized`
  (no tenant to authorize against); both are correct denials.

## Tests

`kernel/controlplane/tenant_auth_test.go` (real server + registry + isolated
tenant kernels, tenant token presented via `AGEZT_TOKEN`):
- own-tenant allowlisted command (edict show) → authorized;
- same token targeting a *different* tenant → unauthorized;
- non-allowlisted commands (`runs_list`, `tenant_list`) with a valid tenant arg →
  forbidden;
- a bogus token → unauthorized;
- the **primary** token still manages any tenant's edict AND the tenant registry.

Test count: **1248 → 1253**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, all touched files gofmt-clean.

## Live proof (multi-tenant daemon)

```
$ agt tenant create acme        # mints acme's token (primary token)
$ agt tenant create beta

# tenant token manages its OWN edict:
$ AGEZT_TOKEN=<acme> agt edict show --tenant acme   → ask_policy: allow ✓

# … and is denied everywhere else:
$ AGEZT_TOKEN=<acme> agt edict show --tenant beta   → unauthorized
$ AGEZT_TOKEN=<acme> agt runs list                  → unauthorized (primary-only)
$ AGEZT_TOKEN=<acme> agt tenant list                → unauthorized (registry mgmt)

# primary keeps full reach:
$ agt tenant list                                   → acme [open], beta [open] ✓
```

Every cross-tenant / primary-only / registry attempt under the tenant token was
denied; the tenant managed only its own policy; the primary token was unaffected —
isolation now holds on the control side as well as in execution.

## What's next

Tenant isolation is now complete across execution and control. Remaining
candidates:

1. **`agt --token <tok>` flag** (LOW) — a first-class flag mirroring the
   `AGEZT_TOKEN` env, for ergonomics; plus an `agt whoami` that reports the
   authenticated principal.
2. **Tenant-scoped run stats** (MED) — make `runs list`/`stats` tenant-routable
   (walk the tenant kernel's journal) so a tenant can observe *its own* runs;
   then add them to `tenantTokenAllows`.
3. **Cross-provider down-routing** (MED) — the deferred M37 follow-on.
4. **`policy.changed` compaction** (LOW) — bound durable-policy boot replay.
