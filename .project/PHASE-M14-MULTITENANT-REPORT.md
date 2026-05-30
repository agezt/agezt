# Phase Report — Milestone M14 (Multi-tenant isolation: the foundation)

> Status: **Phase 1 shipped** (foundation) · Date: 2026-05-31
> ROADMAP P6-MULTI. The storage + lifecycle core that lets one daemon host many
> fully-isolated tenants. Control-plane / API routing per tenant is an explicit
> later phase; this is the isolation primitive those build on.

## Why this milestone

Every prior milestone assumed a single tenant: one `BaseDir` → one journal, one
state store, one vault, one set of memory / world / skills / schedules, one Edict
scope. A Jarvis that serves a team, a household, or multiple projects needs
**hard isolation** between tenants — tenant A must never see, touch, or collide
with tenant B's events, secrets, or schedules — without standing up a separate
daemon per tenant.

The kernel was already well-positioned: `runtime.Open(cfg)` derives *every*
subsystem path from `cfg.BaseDir`, holds no global state, and binds no fixed
ports (those live in the daemon's resident layer). So multiple kernels under
distinct base dirs can coexist in one process. M14 Phase 1 turns that latent
property into an explicit, tested **tenant registry**.

## What shipped — `kernel/tenant`

A `Registry` that manages isolated tenants under one root directory:

- **Isolation by construction.** Each tenant gets its own base dir
  `<root>/<id>`, so its journal, state, memory, world model, skills, vault, and
  schedules are physically separate. Tenant ids are validated against a strict
  pattern (`^[a-z0-9][a-z0-9_-]{0,63}$`) — a single safe path segment with no
  dots or separators — so an id can neither traverse out of the root (`..`,
  `../evil`, `a/b`) nor collide with a sibling. `baseDir` additionally re-checks
  that the cleaned path is still directly under root (defense in depth).
- **Lazy lifecycle.** `Acquire(id)` opens a tenant's kernel on first use and
  caches it; a second `Acquire` of the same id reuses it (idempotent). `Release`
  closes the kernel but keeps the on-disk state (a later `Acquire` reopens it);
  `Remove` deletes the tenant's dir entirely (destructive); `CloseAll` shuts all
  loaded kernels for daemon shutdown. `List` reflects on-disk tenants and which
  are currently open.
- **Decoupled from runtime.** The registry opens tenants through an injected
  `OpenFunc(id, baseDir) (io.Closer, error)`, so its lifecycle logic is
  unit-testable without a provider, and the daemon supplies a real
  `runtime.Open`-backed factory. No import of `kernel/runtime` from the package
  core.

## Proven

- **Unit (fake opener):** id validation (good + traversal/unicode/length-bad
  cases), idempotent acquire (opened once despite repeated `Acquire`), distinct
  contained base dirs, traversal ids rejected *before any side effect* (no dir
  created, no opener called), release-keeps-disk + re-acquire-reopens,
  remove-deletes-only-its-own-dir, list open/closed state, open-error
  propagation (failed open does not register), `CloseAll`.
- **Integration (real kernels, mock provider):** two tenants (`alpha`, `beta`)
  each run an intent through their **own governed loop**, then each tenant's
  journal on disk is asserted to contain **only its own** intent —
  `alpha-secret-intent` appears in alpha's journal and never in beta's, and vice
  versa. This is the end-to-end isolation proof.

8 new tests; suite **1116** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Engineering notes

- **Stdlib only** (`os`, `path/filepath`, `regexp`, `sort`, `sync`). The
  `io.Closer` seam keeps the package free of a runtime dependency.
- **Additive, zero-risk.** Nothing in the existing single-tenant daemon path
  changed; the registry is new surface area. The current daemon remains a
  single-tenant kernel — the registry is the substrate the multi-tenant daemon
  will sit on.

## Deferred — the later phases (named, not yet built)

- **Daemon routing.** Wire the control plane and the OpenAI/REST/ACP surfaces to
  select a tenant per request (`X-Agezt-Tenant` header / a control-plane `tenant`
  arg), dispatching to that tenant's kernel via the registry. The big,
  behavior-changing phase.
- **`agt tenant` CLI** — create / list / release / remove tenants over the
  control plane.
- **Per-tenant auth + quotas.** A token (or token scope) per tenant; per-tenant
  budget ceilings and rate limits, so one tenant can't exhaust another's spend.
- **Shared vs. per-tenant catalog/credentials** policy (today each tenant base
  dir would carry its own vault; some deployments want a shared provider pool).
- **Tenant-scoped Pulse / cadence residents** (each tenant's autonomous timers
  and proactivity running under its own governance).
