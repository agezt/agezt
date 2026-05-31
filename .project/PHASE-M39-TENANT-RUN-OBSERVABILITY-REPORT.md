# Phase Report — Milestone M39 (Tenant-scoped run observability)

> Status: **shipped** · Date: 2026-05-31
> SPEC-14 (tenancy/observability). Direct follow-on to M38: now that a tenant can
> *authenticate*, give it something to authenticate *for* — visibility into its
> own runs.

## Why

M38 let a tenant authenticate with its own token and run/manage its own work. But
`agt runs list`/`stats` still read the **primary** journal unconditionally
(`collectRuns` walked `s.k`), so they were correctly excluded from the tenant
allowlist — a tenant could *run* work but not *see* its own run health. That's a
real gap: a tenant operating in isolation has no observability into the runs it
just submitted.

M39 makes the run views tenant-routable, mirroring how `agt run`/`runs cancel`/
`edict` already route by tenant, and then admits them to the M38 allowlist.

## What shipped

- **`collectRuns(k *runtime.Kernel)` (`kernel/controlplane/runs.go`)** — the shared
  journal fold is now parameterized by the kernel to read, instead of hard-wiring
  `s.k`. `handleRunsList` and `handleRunsStats` resolve the kernel via
  `s.kernelFor(tenantOf(req))` — primary for an empty tenant, the tenant's own
  isolated journal for a named one — then fold over it. The primary path is
  unchanged (empty tenant → `s.k`).
- **`tenantOf(req)` helper (`kernel/controlplane/tenant.go`)** — extracts the
  optional `tenant` routing arg (shared by the run handlers).
- **`tenantTokenAllows` now admits `CmdRunsList` + `CmdRunsStats`
  (`kernel/controlplane/tenant.go`)** — safe to allowlist precisely because they
  now read the *caller's* journal (and M38 pins the tenant arg to the authenticated
  tenant, so a tenant token can only ever read its own).
- **`agt runs list/stats --tenant <id>` (`cmd/agt/runs.go`)** — both commands learn
  the `--tenant <id>` / `--tenant=<id>` flag, passed through as the routing arg.
  Combined with `AGEZT_TOKEN` (M38), a tenant runs `AGEZT_TOKEN=<tok> agt runs
  stats --tenant X`.

## Design decisions

- **Parameterize the fold, don't fork it.** `collectRuns` stays the single source
  of truth for run status; M39 only changes *which* journal it reads. The list and
  stats surfaces therefore remain in lock-step across tenants too.
- **Allowlist only after routing.** Admitting `CmdRunsList`/`CmdRunsStats` to the
  tenant allowlist is sound *because* they became tenant-routed in the same change
  — and because M38 pins the tenant arg, a tenant token presenting tenant X can
  only read X's journal. The two changes are co-dependent and shipped together.
- **Primary unchanged.** An empty/absent tenant arg routes to `s.k` exactly as
  before, so every existing single-tenant call and test behaves identically.

## Tests

- `kernel/controlplane/tenant_auth_test.go`:
  - `TestRunsAreTenantScoped` — a run published to the primary journal and one to a
    tenant's journal: the primary view shows only the primary run, the
    `--tenant acme` view shows only the tenant run (true isolation, both directions).
  - `TestTenantToken_AllowsOwnRunStats` — a tenant token may read its own
    `runs stats`/`list` (newly allowlisted).
  - `TestTenantToken_ForbidsNonAllowlistedCmd` — updated: tenant-registry
    management (`tenant_list`) and daemon-global `halt` remain forbidden.
- Existing `runs_test.go` (primary-scope) all pass unchanged (empty tenant → `s.k`).

Test count: **1253 → 1255**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, all touched files gofmt-clean.

## Live proof (multi-tenant daemon)

```
$ agt run "primary job 1"; agt run "primary job 2"   # on primary
$ agt run "acme job" --tenant acme                   # on tenant acme

$ agt runs stats                                     # primary scope
  run stats (over 2 run(s)): …                        ← only the primary runs

$ AGEZT_TOKEN=<acme> agt runs stats --tenant acme    # tenant scope, tenant token
  run stats (over 1 run(s)): …                        ← only acme's run
$ AGEZT_TOKEN=<acme> agt runs list  --tenant acme
    intent  : acme job                                ← only acme's run

$ AGEZT_TOKEN=<acme> agt runs stats                  # no tenant arg
  → unauthorized                                      ← can't reach the primary
```

A tenant sees exactly its own runs through its own token; the primary sees only
its own; a tenant token cannot reach the primary view — isolation holds across the
observability surface, not just execution.

## What's next

Tenant isolation is now complete across execution, control, and observability.
Remaining candidates:

1. **Cross-provider down-routing** (MED) — the deferred M37 follow-on (route to a
   tool-capable model on a different registered+credentialed provider).
2. **`agt --token <tok>` flag + `agt whoami`** (LOW) — first-class auth flag and a
   principal-reporting command for the M38/M39 model.
3. **`policy.changed` compaction** (LOW) — bound durable-policy boot replay to
   O(active rules).
4. **Vision/attachment capability enforcement** (MED) — extend M25/M37 beyond
   tool-use once agent messages carry images.
