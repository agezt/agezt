# Phase Report — Milestone M10 (Web UI v1: Live Monitor + Explorers)

> Status: **shipped** · Date: 2026-05-30
> ROADMAP product-M4 / SPEC-07 — the first **visual surface** over the
> system. Everything Agezt knows and does was already queryable (`--json`)
> and streamable (the bus); M10 adds eyes.

## Scope

A server-rendered, SSE-driven **dashboard**: open
`http://127.0.0.1:<port>/?token=…` and watch the system live — a streaming
event feed (the Live Monitor) plus read panels for status, world model,
skills, memory, inbox, and the latest reflection. Every panel is a
projection of the same journal/bus/control-plane the CLI uses; the UI holds
no authoritative state (SPEC-07 §0).

**Stdlib-first MVP cut.** SPEC-07 §2 specifies React 19 + Tailwind + React
Flow. As every prior milestone cut its SPEC to a stdlib slice (memory
deferred CobaltDB; channels used `net/http`, not an SDK), the Web UI v1 is
**`net/http` + `embed` + vanilla JS — no React, no npm, no build step.**
This is faithful to SPEC-07 §0 ("one event truth, many views; the UI never
holds authoritative state — it subscribes and renders") and §5.2 ("Live
Monitor driven entirely by events — the journal is the telemetry"). Flow
Studio's React canvas (§3) stays the full-project direction; v1 ships the
event-truth surfaces under it.

## What shipped

### New `kernel/webui`
A stateless HTTP surface (`net/http` only) with two data paths, both
reusing what already exists:
- **`GET /events` (SSE)** — subscribes to `bus.Subscribe(">", 256)` (the
  exact firehose the daemon tees to stdout) and relays each event as one
  `data: {json}\n\n` frame, flushing per event, with a 20s heartbeat;
  closes on client disconnect (request ctx) or bus close. This is the Live
  Monitor's spine (§5.2).
- **`GET /api/{status,memory,world,skills,inbox,reflect}`** — thin JSON
  proxies that `Call` the matching control-plane command
  (`CmdStatus`/`CmdMemoryList`/`CmdWorldList`/`CmdSkillList`/`CmdInbox`/
  `CmdReflectShow`) through the **same `controlplane.Client` `agt` uses**.
  Zero query duplication; guaranteed CLI/Web parity.
- **`GET /`** — the embedded (`//go:embed`) single-page dark dashboard:
  vanilla JS (`EventSource` + `fetch`), no build chain.

The client is taken as a `Caller` interface (`Call(ctx, cmd, args)`),
satisfied by `*controlplane.Client` — which keeps the package unit-testable
against a fake, no live daemon required.

### Security (SPEC-06)
- **Token on every request.** A fresh 32-byte token is minted at start
  (`crypto/rand` → hex, mirroring the control plane). The browser passes it
  as `?token=` (an `EventSource` can't set headers); API callers may use
  `Authorization: Bearer`. Mismatch → 401. An unconfigured token **fails
  closed** (serves nothing).
- **Read-only in v1.** Every `/api` route maps to a read command — there is
  no path that mutates (a test asserts the proxy only ever issues the known
  read set). Halt/approve/forget from the browser are deferred.
- **Never binds `0.0.0.0` implicitly.** The operator supplies the host via
  `AGEZT_WEB_ADDR`; the banner **warns** if it isn't loopback (public
  exposure is their explicit choice — a tunnel, §9).

### Daemon wiring (`cmd/agezt/main.go`)
`buildWebUI(ctx, k, baseDir, stdout)` — an in-process resident like
Pulse/Telegram: gated by `AGEZT_WEB_ADDR`, runs on the daemon ctx (so `agt
halt`/SIGTERM/`agt shutdown` stop it via `http.Server.Shutdown`), binds with
`net.Listen` so a bad address fails the banner synchronously. Banner prints
the tokenized URL (+ loopback warning when applicable); absent → "disabled".

| Env var | Meaning |
|---|---|
| `AGEZT_WEB_ADDR` | `host:port` to serve the Web UI on (e.g. `127.0.0.1:8787`); unset = disabled |

## Design rules followed

- **No new external dependency** — `net/http`/`embed`/`encoding/json`/
  `crypto/rand` are stdlib; `go.mod` unchanged (POLICY).
- **No new event kinds** — a read surface emits nothing; it only observes.
- **No new query logic** — read panels proxy existing `Cmd*` handlers; the
  live feed reuses the existing bus subscription. The CLI and the Web UI are
  the same data, two renderers.
- **Resident lifecycle reuse** — start-on-ctx / stop-on-cancel / env-gated
  banner, identical to the Pulse/Telegram/reflect-ticker wiring.
- **XSS-safe by construction** — all bus/control-plane values render via
  `textContent` + DOM construction; no `innerHTML` with dynamic data.

## Test coverage

7 new `httptest` tests (`kernel/webui`); `go test ./...` green on host
(windows) + `GOOS=linux` cross-compile; `go vet` + `gofmt -l` clean.
Package count 46 → 47.

- `/` serves the embedded HTML (200, `text/html`).
- Auth matrix: no-token / wrong-token / wrong-bearer → 401; query-token /
  bearer-token → 200; **empty-token server rejects everything** (fail
  closed).
- `/api/status` proxies the control-plane result and issues exactly one
  `CmdStatus`.
- **Read-only invariant:** every advertised `/api` route issues a command in
  the known read set.
- Upstream error → 502 with the cause relayed.
- **SSE:** a published bus event arrives on `/events` as a `data:` frame
  carrying its kind + subject.

### Manual end-to-end (mock provider)
Started the daemon with `AGEZT_WEB_ADDR=127.0.0.1:8799`; the banner printed
the tokenized URL. Verified:
- `GET /api/status?token=…` → `{"active_runs":0,…,"world_entities":0}`
  (the same numbers `agt status` shows).
- `GET /api/world?token=…` → `{"count":0,"entities":[],"relation_count":0}`.
- `GET /api/status` (no token) → **401**.
- `GET /?token=…` → **200 `text/html`** (the dashboard).
- **Live feed:** with `curl -N /events?token=…` streaming, firing `agt run
  "…"` produced **15 SSE `data:` frames** spanning the full run —
  `task.received → llm.request/response → routing.decision →
  policy.decision → tool.invoked/result → warden.executed →
  budget.consumed → task.completed`. The Live Monitor renders the journal in
  real time. (Browser rendering of the embedded page is documented; `curl`
  exercises the identical handlers.)

## Deferred (named for later)

- **React 19 + React Flow Flow Studio** (design/run/replay canvas, §3) and
  the TS SDK — v1 is the event-truth dashboard those build on.
- **Write actions** from the browser (halt/approve/forget/skill-revert) —
  read-only in v1.
- **WebSocket transport** (SSE suffices for one-way telemetry v1).
- **The gateway + remote auth/OAuth + tunnels** (§9, SPEC-06) for non-local
  access; richer per-surface panels (graph node-link world view, DAG
  replay, the full Web Inbox triage).

## Closes / next

Agezt now has a **visual surface**: a single page that streams the live
event truth and renders every read projection the control plane exposes,
localhost-bound and token-authed. Combined with Phase 2's cognitive loop,
an operator can now *watch* the system remember, model, learn, reflect, and
act — not just query it after the fact.

Next per ROADMAP: harden toward a tagged release, or begin the React Flow
Studio (§3) on top of this event-truth foundation.
