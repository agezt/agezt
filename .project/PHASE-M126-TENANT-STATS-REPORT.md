# M126 — `agt tenant stats`: cross-tenant usage view

## Why
Multitenancy (M14/M38–39) gives each tenant a fully isolated kernel — its own
journal, state, vault, budget. The operator can `tenant create / list / token /
release / rm`, and route work with `--tenant <id>`. But that surface is
**asymmetric**: you can *manage* tenants yet have no way to *see what they're
doing*. "Which tenant is busy? Which is burning budget? Which is failing?" had no
answer short of running `agt runs stats --tenant <id>` once per tenant by hand.

The data was already there — each tenant's own journal — just never aggregated
across tenants for the primary operator.

## What
`agt tenant stats [--json]` — for every tenant on disk, fold that tenant's journal
into run count / completed / failed / active / spend / last-activity, plus grand
totals. Reuses the exact `collectRuns` fold behind `agt runs`, so the per-tenant
numbers match what each tenant sees for itself.

Two correctness properties:
- **Residency-preserving.** A tenant that was *closed* before the call is acquired
  (opened on demand, as any tenant-routed command does), folded, then **released
  again** — so a read-only stats query never silently leaves every tenant resident
  in memory. A tenant already open stays open.
- **Primary-token only.** `CmdTenantStats` is deliberately absent from
  `tenantTokenAllows`, so a tenant credential is denied by default (a tenant sees
  only its own runs via `runs stats`); only the primary operator gets the
  cross-tenant view.

Per-tenant failures (e.g. a corrupt tenant journal) are reported in that tenant's
row, not fatal to the whole command.

## Files
- `kernel/controlplane/protocol.go` — `CmdTenantStats` constant + doc.
- `kernel/controlplane/server.go` — dispatch case.
- `kernel/controlplane/tenant.go` — `handleTenantStats` (acquire → `collectRuns` →
  aggregate → restore residency).
- `cmd/agt/tenant.go` — `cmdTenantStats` (table + totals), `stats` subcommand, help.
- `kernel/controlplane/tenant_test.go` — `TestTenantStats_AggregatesPerTenant`
  (alpha 1 / beta 2 / total 3) + `CmdTenantStats` added to the disabled-registry
  guard test.

## Live proof (offline mock, AGEZT_MULTITENANT=on)
Created alpha + beta, ran 1 task in alpha and 2 in beta, then **released alpha**
(closed it) before the stats call:
```
=== tenant stats ===
  alpha    1 run(s)  (1 ok, 0 failed, 0 active)  last: 2026-06-02 09:00
  beta     2 run(s)  (0 ok, 2 failed, 0 active)  last: 2026-06-02 09:00
  2 tenant(s), 3 run(s) total

=== tenant list (after stats) ===
  alpha    [closed]   …/tenants/alpha     ← stats read it, then re-closed it
  beta     [open]     …/tenants/beta
```
alpha is `[closed]` again after stats opened it to read — the residency-preserving
property holds. (beta's "2 failed" is the offline mock's scripted-response
exhaustion across repeated daemon calls; the run counts and attribution are
correct.)

## Verification
- 55 packages `ok`, **FAIL 0**; **1415 tests**.
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: every touched file is clean under LF (0 complaints); the working-tree
  CRLF flag is the repo-wide line-ending artifact, normalized to LF in the commit
  blob.
