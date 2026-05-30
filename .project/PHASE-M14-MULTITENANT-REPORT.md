# Phase Report — Milestone M14 (Multi-tenant isolation)

> Status: **Phases 1–7 shipped** · Date: 2026-05-31
> ROADMAP P6-MULTI. Phase 1: the storage + lifecycle isolation core
> (`kernel/tenant`). Phase 2: the daemon wiring + operator surface (`agt
> tenant`). Phase 3: per-request **run routing** (`agt run --tenant <id>`).
> Phase 4: **tenant-routed REST API** (`X-Agezt-Tenant` header). Phase 5:
> **tenant-routed OpenAI-compatible API** (same header on `/v1/chat/completions`,
> `/v1/responses`, `/v1/models`). Phase 6: **tenant-routed ACP** (`agt acp
> --tenant <id>`), completing the API-surface routing. Phase 7: **per-tenant
> budget quotas** (each tenant its own governor + spend ledger + ceiling).
> Per-tenant auth is the remaining work.

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

## Phase 2 — daemon wiring + `agt tenant`

The registry is now mounted in the daemon and managed by operators:

- **Daemon mount (opt-in).** With `AGEZT_MULTITENANT=on`, the daemon builds a
  `tenant.Registry` rooted at `<baseDir>/tenants` whose `OpenFunc` clones the
  *primary* kernel config (same provider, tools, model, catalog) with a
  per-tenant base dir and a **fresh per-tenant Warden and Edict** (so a tenant's
  HALT or policy state is its own, not shared) and no reload hook. The primary
  single-tenant kernel is untouched; off by default. A banner line
  (`tenancy: enabled (root=…)` / `disabled`) reports the state, and `CloseAll`
  runs on shutdown.
- **Control plane.** Four token-authed commands — `tenant_create` (Acquire),
  `tenant_list` (List), `tenant_release` (Release), `tenant_remove` (Remove) —
  injected via `Server.SetTenants`. When no registry is configured they return a
  clear "multi-tenancy is disabled" error instead of touching anything.
- **CLI.** `agt tenant create|list|release|rm <id>` (with `--json`), distinct
  exit codes (3 for not-open / not-found), and a help subcommand.

**Proven live (daemon + CLI, `AGEZT_MULTITENANT=on`):** `tenant create alpha` /
`beta` each materialised an isolated base dir with its own
`journal/state/memory/worldmodel/skills/cadence`; `tenant list` showed both
open; `release alpha` closed it (→ `[closed]`, state kept on disk); `rm beta`
deleted *only* beta's tree (alpha untouched); a `../evil` id was rejected; `rm`
of a missing id returned exit 3. Plus a control-plane integration test
(create/list/release/remove + idempotent re-create + traversal rejection) and
a "disabled without a registry" test.

## Phase 3 — per-request run routing

Runs can now choose their tenant; everything else about a run is unchanged.

- **Resolver.** `Server.kernelFor(tenantID)` returns the primary kernel for an
  empty id (the untouched single-tenant path) or the named tenant's kernel,
  opening it on demand via the registry (type-asserting the `io.Closer` back to
  `*runtime.Kernel`). A tenant id with no registry configured, or an invalid id,
  errors cleanly.
- **`CmdRun`.** `handleRun` reads an optional `tenant` arg and routes the whole
  run — correlation, bus subscription, `RunWith` — through the resolved kernel,
  so the run executes under *that tenant's* governance and lands in *that
  tenant's* journal. Absent the arg, byte-for-byte the previous behavior.
- **CLI.** `agt run "<intent>" --tenant <id>` (composes with `--json`).

**Proven live:** `run --tenant alpha` executed on alpha's kernel and its
correlation appears in `tenants/alpha/journal` but **not** in the primary
journal; `run` without the flag stayed on the primary; `run --tenant beta`
**auto-created** beta on demand (lazy Acquire) and ran there. A control-plane
test asserts a routed run's intent is in the tenant journal and absent from the
primary's, and that an invalid tenant id errors.

## Phase 4 — tenant-routed REST API

The native REST surface (`kernel/restapi`) now routes per tenant, the same way
`CmdRun` does — external HTTP clients, not just `agt run`, can target a tenant.

- **Header-driven.** A request carrying `X-Agezt-Tenant: <id>` is served by that
  tenant's Engine **and bus**; absent the header (or with no resolver wired) it
  uses the primary engine/bus — byte-for-byte the previous single-tenant path.
  The bus, not just the engine, is per-tenant: streaming runs subscribe to the
  resolved tenant kernel's bus so SSE tokens come from the right journal.
- **Resolver seam.** `Server.SetTenantResolver(func(id) (Engine, *bus.Bus,
  error))` injects the mapping; `Server.bind(r)` reads the header and returns the
  pair (or the primary). All three run paths — sync `POST /api/v1/runs`,
  streaming `POST /api/v1/runs` (SSE), and `GET /api/v1/runs/{corr}` — go through
  `bind`. A resolver error is a clean `400 invalid_tenant`.
- **Daemon wiring.** When `AGEZT_MULTITENANT=on`, `buildRESTAPI` installs a
  resolver backed by the registry: `Acquire(id)` → the tenant kernel's
  `kernelAPIEngine` adapter + its `Bus()`. Off by default; nil resolver = today's
  behavior.

**Proven live (`AGEZT_MULTITENANT=on`, `AGEZT_REST_ADDR=127.0.0.1:8810`):** a
`curl -H "X-Agezt-Tenant: alpha"` POST to `/api/v1/runs` landed in
`tenants/alpha/journal` ("YES in alpha") and was **absent** from the primary
journal ("NO (isolated)"); a header-less POST stayed on the primary. Plus a unit
test (`TestRun_TenantRouting`): header-less → primary engine, `X-Agezt-Tenant:
alpha` → alpha's answer with the run recorded only on alpha, unknown tenant
`ghost` → 400.

## Phase 5 — tenant-routed OpenAI-compatible API

The drop-in OpenAI surface (`kernel/openaiapi`) now routes per tenant too, so any
OpenAI SDK / IDE / client can target a tenant by setting one extra header — the
same `X-Agezt-Tenant` seam as the native REST surface.

- **All three routes.** `POST /v1/chat/completions`, `POST /v1/responses`, and
  `GET /v1/models` resolve their Engine + bus through `Server.bind(r)`. Both
  streaming forms (Chat `chat.completion.chunk` SSE and the Responses
  `response.*` SSE sequence) subscribe to the **resolved tenant's bus** so token
  deltas come from that tenant's journal, not the primary's.
- **Same resolver shape.** `Server.SetTenantResolver(func(id) (Engine, *bus.Bus,
  error))`; header-less requests (or a nil resolver) stay on the primary —
  byte-for-byte the prior single-tenant path. A resolver error is a clean `400
  invalid_request_error`.
- **Daemon wiring.** `buildOpenAIAPI` takes the registry and installs the same
  `Acquire(id)` → tenant `kernelAPIEngine` + `Bus()` resolver as `buildRESTAPI`,
  guarded by `AGEZT_MULTITENANT=on`.

**Proven:** a unit test (`TestChat_TenantRouting`) drives `/v1/chat/completions`
header-less (→ primary engine), with `X-Agezt-Tenant: alpha` (→ alpha's answer,
run recorded only on alpha, primary untouched), and with an unknown tenant
`ghost` (→ 400). The streaming paths reuse the same `bind`-resolved bus the
non-streaming path proves.

## Phase 6 — tenant-routed ACP (editor backend)

The Agent Client Protocol bridge (`agt acp`, the stdio JSON-RPC agent an IDE like
Zed drives) can now bind a whole editor session to a tenant. Unlike the HTTP
surfaces, ACP is **not** a daemon resident — it runs in the `agt` client process
and forwards each prompt to the daemon over the control plane. So routing reuses
the Phase 3 seam directly: the control-plane `CmdRun` already honours a `tenant`
arg; ACP just supplies it.

- **One flag.** `agt acp --tenant <id>` stores the id on the runner; every prompt
  it forwards adds `tenant: <id>` to the `CmdRun` args, so the run executes on —
  and streams chunks from — that tenant's kernel. Omit the flag and the `tenant`
  key is absent entirely (byte-for-byte the prior request; the daemon's
  `kernelFor("")` stays on the primary).
- **No new daemon code.** The whole feature is the CLI flag + a one-line arg; the
  routing it triggers is the control-plane path already proven in Phase 3.

**Proven:** unit tests on the runner (`TestACPRunner_ForwardsTenant` asserts the
`tenant` arg is forwarded with the intent; `TestACPRunner_OmitsTenantWhenUnset`
asserts the key is absent when no tenant is set) via a fake streamer — no daemon
needed. The downstream routing (`tenant` arg → isolated kernel + journal) is the
control-plane behaviour Phase 3's `TestRun_RoutesToTenantKernel` already proves.

With Phases 3–6, **every** way to start a run — `agt run`, the native REST API,
the OpenAI-compatible API, and an ACP editor session — can target a tenant
through one consistent seam (an arg/header resolved to an isolated kernel + bus).

## Phase 7 — per-tenant budget quotas

Isolation wasn't complete while tenants shared one spend ledger: until now every
tenant kernel was opened with the **primary's** governor, so they shared its
single in-memory daily-spend counter and ceiling — one tenant could exhaust the
global cap and silently starve every other tenant (and the primary). Phase 7
gives each tenant its **own** governor.

- **Sibling governor.** `Governor.WithDailyCeiling(microcents)` returns a sibling
  that shares the parent's provider **registry** (same provider pool, same
  credentials — tenants don't each need their own keys) and routing config, but
  keeps an **independent spend ledger** (its own `spentToday`, its own day
  rollover) and its own global ceiling. The daemon's tenant `OpenFunc` builds one
  per tenant, sets it as the tenant kernel's `Provider`, and re-points its bus to
  the tenant's own kernel bus (`SetBus`) so budget events land in *that* tenant's
  journal.
- **Operator control.** Each tenant's daily ceiling defaults to the primary's;
  `AGEZT_TENANT_DAILY_CEILING=<usd>` overrides it for every tenant (a malformed
  or negative value is a hard startup error). The banner reports it
  (`tenancy: enabled (root=…, ceiling=$5.00/day)` or `ceiling=inherited`).

**Proven:** a governor unit test (`TestWithDailyCeiling_IndependentLedgers`)
shows a sibling with a tiny ceiling getting its **second** call blocked with
`ErrBudgetExceeded` while the parent's ledger stays at 0 and the parent keeps its
full headroom — spend does not bleed across the boundary — and the sibling
reports its own ceiling, not the parent's. Live: the daemon boots with
`AGEZT_MULTITENANT=on AGEZT_TENANT_DAILY_CEILING=5.00` and the banner shows the
per-tenant ceiling.

## Engineering notes

- **Stdlib only** (`os`, `path/filepath`, `regexp`, `sort`, `sync`). The
  `io.Closer` seam keeps the package free of a runtime dependency.
- **Additive, zero-risk.** Nothing in the existing single-tenant daemon path
  changed; the registry is new surface area. The current daemon remains a
  single-tenant kernel — the registry is the substrate the multi-tenant daemon
  will sit on.

## Deferred — the later phases (named, not yet built)

- **Tenant-routed API surfaces — done.** All run entry points route per tenant:
  the control plane (`CmdRun` tenant arg, Phase 3), the native REST surface
  (Phase 4), the OpenAI-compatible surface (Phase 5), and ACP (Phase 6).
- **Per-tenant auth.** A token (or token scope) per tenant so a caller proves
  which tenant it may target (today the daemon token gates the surface and the
  `X-Agezt-Tenant`/`--tenant` selector is trusted). Per-tenant budget *ceilings*
  shipped in Phase 7; per-tenant **rate limits** (calls/min, not just $/day) are
  the natural companion still to do.
- **Shared vs. per-tenant catalog/credentials** policy (today each tenant base
  dir would carry its own vault; some deployments want a shared provider pool).
- **Tenant-scoped Pulse / cadence residents** (each tenant's autonomous timers
  and proactivity running under its own governance).
