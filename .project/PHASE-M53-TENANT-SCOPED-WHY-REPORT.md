# Phase Report — Milestone M53 (Tenant-scoped `agt why`)

> Status: **shipped** · Date: 2026-06-01
> SPEC-08 (event journal) × SPEC-14 (multi-tenancy). Closes the last
> non-tenant-aware control surface: `agt why` now traces a tenant's own journal.

## Why

M38 made control-plane access per-tenant authenticated; M39 made the run views
(`runs list`/`stats`) tenant-scoped. But `agt why` — the event-chain tracer —
still read the primary journal unconditionally (`s.k.Why` / `s.k.ParentOf`). So a
tenant couldn't trace its own events, and the primary `why` could see across the
isolation boundary into any journal. `why` was the one observability surface left
behind by M39. M53 routes it through the same `kernelFor(tenantOf(req))` seam, so
each scope traces only its own journal.

## What shipped

- **`handleWhy` routed via `kernelFor` (`kernel/controlplane/server.go`)** — an
  empty tenant traces the primary journal; a named tenant traces its own isolated
  journal. Both `k.Why(id)` and `k.ParentOf(corr)` now run on the resolved kernel,
  not `s.k`. One-line semantic change, isolation enforced.
- **`CmdWhy` added to `tenantTokenAllows` (`kernel/controlplane/tenant.go`)** — a
  tenant token may now trace its own events, alongside the M39 run views. Daemon-
  global and registry-management commands stay primary-only.
- **`--tenant` on `agt why` (`cmd/agt/why.go`)** — `agt why <event_id> --tenant
  <id>` traces a tenant's journal (needs that tenant's token). Both `--tenant X`
  and `--tenant=X` forms, mirroring `agt runs list/stats`. The arg loop was
  converted from a range to an index loop to accept the two-token form.

## Design decisions

- **Reuse the M39 seam, don't invent.** `kernelFor(tenantOf(req))` is the exact
  pattern M39 used for `runs list/stats` and M22 for `edict`. Routing `why`
  through it makes the third observability surface tenant-aware with no new
  mechanism — empty tenant → primary, named tenant → its registry kernel.
- **Isolation is mutual.** A tenant can't trace a primary event (it's not in the
  tenant journal → "not found"), and the primary scope can't trace a tenant event.
  Neither side leaks across the boundary — proven both ways in the test and live.
- **Allowlist, not open.** `why` joins the deny-by-default tenant allowlist
  explicitly; a tenant token still can't run `tenant_list`, `halt`, or any
  daemon-global command. Least privilege preserved.
- **"Not found" is the right failure.** Tracing an event that isn't in the scoped
  journal returns the kernel's existing "event … not found" error — no special
  cross-tenant error, no information leak about whether the id exists elsewhere.

## Tests

- `kernel/controlplane/tenant_auth_test.go::TestWhyIsTenantScoped` — a tenant
  token traces its OWN event (found, correct correlation) but not a primary event
  (not found); symmetrically, the primary scope sees its own event but not the
  tenant's. Exercises the real Server/Client round-trip + the allowlist.
- `cmd/agt/why_test.go::TestCmdWhy_HelpExitsCleanly` extended to assert `--tenant`
  is documented.
- The existing tenant-auth suite (`ForbidsNonAllowlistedCmd`, etc.) is unchanged
  and still passes — `why` was not in the forbidden examples.

Test count: **1284 → 1285**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, added lines gofmt-clean.

## Live proof (offline daemon, AGEZT_MULTITENANT=on)

```
$ agt tenant create acme
$ agt run "hello world"          # primary scope
$ agt why <event-id>             # primary: found
  11 events in correlation:
    seq=0  kind=task.received  …

$ agt why <event-id> --tenant acme   # tenant scope: isolated
  agt why: controlplane: runtime: event … not found
```

The same event id resolves under the primary scope but not under `--tenant acme`
— `why` now reads the tenant's own journal, completing tenant isolation across
the observability surface.

## What's next

Tenant isolation is now complete across execution (M14), control (M38), and
observability (M39 runs + M53 why). The multi-agent axis is complete. Remaining
work is small polish or a fresh axis:

1. **Boot-banner the delegation caps** (LOW) — echo the active depth / fan-out /
   spend ceilings at daemon startup, alongside the model-advisory / recovery banners.
2. **`agt runs stats` spend percentiles** (LOW) — extend the M47 spend aggregate
   with a per-run cost distribution (avg/p50/p95), mirroring the duration block.
3. **`agt runs list` answer preview column** (LOW) — `answer_preview` is on every
   row (M52); show it in the flat list too, not only the `↳` line.
4. **Open a fresh axis** (MED-HIGH) — autonomy/scheduler observability
   (`cadence.Store` exists; its surfacing may be thin) or revisiting
   vision/attachment enforcement — worth a read-only `Explore` to pick.
