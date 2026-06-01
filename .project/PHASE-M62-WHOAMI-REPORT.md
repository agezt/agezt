# Phase Report — Milestone M62 (`agt whoami`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-14 multi-tenancy.

## Why

M38/M39 added tenant tokens, but a client with `AGEZT_TOKEN` set had no way to
confirm WHICH identity it authenticates as — primary (admin) or a specific
tenant. When juggling multiple tokens, "who am I right now?" was unanswerable.

## What shipped

- **`CmdWhoami` + `handleWhoami` (`kernel/controlplane/server.go`)** — reports the
  authenticated principal. By the time a handler runs, `handleConn` (M38) has
  already verified the token, so identity is a pure read: `req.Token == s.Token()`
  → `{identity: primary, primary: true}`; otherwise the token authenticated as the
  tenant pinned in `req.Args["tenant"]` → `{identity: tenant, tenant: <id>}`. No
  new auth state.
- **Tenant allowlist** — `CmdWhoami` joins `tenantTokenAllows` so a tenant token
  can ask.
- **`agt whoami [--tenant <id>] [--json]`** — prints `primary (admin token …)` or
  `tenant "acme" (own token …)`.

## Design decisions

- **Derive, don't store.** The auth decision already happened in `handleConn`;
  whoami re-reads `req.Token` vs the primary token rather than threading a new
  "principal" field through the request — minimal and tamper-proof (a forged
  tenant arg never passed auth).

## Tests

- `TestWhoami_PrimaryAndTenant` — the primary token reports `primary=true`; a
  tenant token reports `primary=false, tenant=acme`.

Test count: **1300 → 1301**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof (AGEZT_MULTITENANT=on)

```
$ agt whoami
  agt: primary (admin token — full access)
$ AGEZT_TOKEN=<acme-token> agt whoami --tenant acme
  agt: tenant "acme" (own token — tenant-scoped access)
```
