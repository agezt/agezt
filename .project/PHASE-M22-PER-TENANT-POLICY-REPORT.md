# Phase Report — Milestone M22 (Per-tenant policy management)

> Status: **shipped** · Date: 2026-05-31
> ROADMAP P6-MULTI · DECISIONS F3/F4 · B1. The runtime policy surface (M18–M21)
> operated only on the primary kernel. M22 routes every `agt edict` command to a
> named tenant's isolated engine, so multi-tenant deployments can manage — and
> durably persist — each tenant's policy independently.

## Why

Two prior arcs had not yet met: **M14** gave each tenant its own isolated kernel
(storage, identity, cost, rate, and a fresh Edict engine), and **M18–M21** made
the policy engine fully runtime-manageable and durable. But all the `agt edict`
commands hard-coded `s.k.Edict()` — the primary. There was no way to say "lock
shell down *for tenant acme*" or "add this deny rule to *that* tenant." The
recurring "per-tenant policy" deferral named in every report since M18 was
exactly this gap. M22 closes it by fusing the two arcs.

The control plane already had the seam: `kernelFor(tenantID)` (used by `CmdRun`)
resolves an empty id to the primary kernel and a non-empty id to the tenant's
lazily-opened kernel. M22 simply routes the policy handlers through it.

## What shipped

- **Handler routing (`kernel/controlplane/edict.go`)** — a small `edictFor(conn,
  req)` helper reads the optional `req.Args["tenant"]`, resolves it via
  `kernelFor`, and returns the right engine *and* its owning kernel (so writes
  journal to that kernel's bus). All seven handlers (`show`, `test`, `deny
  list/add/rm`, `level`, `mode`) now route through it; zero `s.k.Edict()` /
  `s.k.Bus()` references remain. An unknown-tenant / registry-disabled error is
  surfaced cleanly, not swallowed.
- **CLI (`cmd/agt/edict.go`)** — every `agt edict` subcommand accepts `--tenant
  <id>` (and `--tenant=<id>`), parsed by a shared `extractTenantFlag` and folded
  into the request by `withTenant` (empty → omitted → primary). Help text on
  every subcommand advertises the flag.
- **Per-tenant durability (`cmd/agezt/main.go`)** — the tenant `OpenFunc` now, when
  `AGEZT_EDICT_DURABLE=on`, replays the just-opened tenant's OWN journal overlay
  onto its engine (the same `replayPolicyOverlay` the primary uses, M20). Each
  tenant's journal is its own source of truth; best-effort so a read error leaves
  the tenant on its boot policy rather than failing the lazy open.

No `go.mod` change. No new control-plane command (the existing edict commands gain
an optional arg) and no new event kind (tenant changes are ordinary
`policy.changed` events in the tenant's journal).

## Proven

- **Unit (control plane):** deny-rule isolation — a rule added to tenant `alpha`
  hard-denies for `alpha` but NOT for `beta` or the primary, and `deny list`
  shows one runtime rule on `alpha`, none on `beta`; level isolation — `shell=L0`
  on `alpha` leaves `beta` and the primary unchanged; naming a tenant with no
  registry wired errors.
- **Unit (CLI):** `extractTenantFlag` over both flag forms + the no-flag and
  trailing-flag cases with order-preserving remainder; `withTenant` injection;
  `--tenant` advertised in help.
- **Live (multi-tenant daemon + restart, mock provider):**
  - `AGEZT_MULTITENANT=on AGEZT_EDICT_DURABLE=on`; `edict deny add --tenant alpha
    "shell:kubectl delete"` + `edict level --tenant alpha http.post L0`.
  - **Isolation:** `kubectl delete` is denied for `alpha`, **allowed** for `beta`
    and the primary; `deny list --tenant alpha` shows the `[runtime]` rule,
    `--tenant beta` shows none.
  - **Restart:** alpha's deny rule and `http.post=L0` are both restored — from
    **alpha's own journal** — while `beta` stays clean. The tenant's journal
    holds its two `policy.changed` events; the **primary journal holds zero** —
    confirming per-tenant journal isolation.

6 new tests; suite **1184** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Arc — the security/policy line, M14 → M22

| M | Frontier |
|---|---|
| 14 | Tenant isolation (storage · identity · cost · rate · engine) |
| 15 | Secret redaction at the journal boundary |
| 16 | Network egress guard (SSRF / metadata) |
| 17 | Operator-extensible hard-deny floor |
| 18 | Runtime deny-rule management |
| 19 | Runtime trust-level changes |
| 20 | Durable runtime policy (journal replay) |
| 21 | Runtime approval-mode changes |
| **22** | **Per-tenant policy management (fuses 14 + 18–21)** |

Every governed tenant now has the *full* runtime-and-durable policy surface over
its own token and its own journal, isolated from every other tenant and from the
primary. The two longest threads of the arc are joined.

## Deferred — named

- **Per-tenant `--tenant` for the rest of the read/inspect commands** (`state`,
  `memory`, `world`, …) — the same `kernelFor` routing would generalise; M22
  scoped it to the policy surface where management matters most.
- **Tenant-authenticated policy management** — today policy commands use the
  primary control-plane token; a future mode could require the *tenant's* token
  (M14 minted one per tenant) so a tenant operator manages only their own policy.
- **Compaction** of `policy.changed` history (shared with M20), now per-tenant.
