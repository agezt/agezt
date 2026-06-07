# Phase M575 — Home Assistant control TOOL (read state + call services)

**Type:** Feature (agentic capability; eleventh in-process tool)
**Date:** 2026-06-07
**Branch:** `feat-homeassistant-tool`

## Goal

Make Home Assistant READABLE and ACTIONABLE from inside an agent run. The M573
Home Assistant *channel* is outbound-only (it pushes a brief to a notify
service). This adds the inverse: a tool the agent can call mid-run to read entity
state ("is the living-room light on?") and call services ("turn the porch light
off", "set the thermostat to 20°C"). It turns Agezt from something that announces
into the house into something that can act on it. Stdlib-only, no dependency,
fail-closed.

## What shipped

### `plugins/tools/homeassistant/homeassistant.go` (new)
An `agent.Tool` with two operations over the HA REST API on `net/http` only:
- **`get_states`** → GET `{base}/api/states` (all) or `/api/states/{entity_id}`
  (one). Read-only.
- **`call_service`** → POST `{base}/api/services/{domain}/{service}` with the
  service data (the model's `data` object, with `entity_id` merged in).

Security — fail-closed on two independent axes:
- The HA URL + token are OPERATOR-pinned config; the agent never supplies the
  host, so there is **no SSRF / arbitrary-egress surface** (which is why, unlike
  the `http` tool, this needs no netguard — the destination is fixed).
- **`call_service`** is gated by a SERVICE allowlist (`"light.turn_on"`,
  `"climate.*"`, `"*"`); empty → nothing callable.
- **`get_states`** is gated by a READ-entity allowlist; empty → nothing readable,
  and a BULK read is FILTERED to the allowlist so a prompt-injected agent can't
  enumerate the whole house through `/api/states`.
- The token is never logged; response bodies are size-capped (256 KiB) before
  the model sees them. HTTP client is injectable (unit-testable offline).

### `kernel/edict/` — two new capabilities
- `CapHomeAssistantRead = "homeassistant.read"` (DefaultLevel **Allow** — reads
  are low-risk and allowlist-filtered) and `CapHomeAssistantCall =
  "homeassistant.call"` (DefaultLevel **Ask-first** — actuating the physical
  world warrants confirmation). Added to `DefaultLevels` + `AllCapabilities`.
- `toolmap.go`: `case "homeassistant"` parses `{operation}` → `call_service`
  maps to the call capability; `get_states` (and any unrecognised/garbled op)
  maps to the read capability — the low-risk default, so a malformed call can
  never silently gain actuation.

### Daemon wiring (`cmd/agezt/main.go`)
- `buildTools`: registers `homeassistant` only when `AGEZT_HOMEASSISTANT_URL` +
  `_TOKEN` are set AND at least one axis is enabled
  (`AGEZT_HOMEASSISTANT_TOOL_READ` and/or `AGEZT_HOMEASSISTANT_TOOL_SERVICES`;
  `AGEZT_HOMEASSISTANT_TOOL_ALLOW_ALL_SERVICES=1` appends `*` with a warning).
  So bare channel config (URL+token for outbound notify) does NOT auto-expose an
  actionable tool. Banner shows `homeassistant(read=N, services=M)`.
- `newDemoMock`: `AGEZT_DEMO_HOMEASSISTANT=1` scripts the agent to actuate then
  read via the tool, for offline end-to-end smoke (mirrors `AGEZT_DEMO_NOTIFY`).
- `kernel/controlplane/config.go`: the four new `AGEZT_HOMEASSISTANT_TOOL_*` /
  `AGEZT_DEMO_HOMEASSISTANT` env vars added to `configEnvVars` (alphabetical;
  enforced by `TestConfigEnvVars_CoversCmdAgeztReads`).

### Tests — `plugins/tools/homeassistant/homeassistant_test.go` (new; 11 funcs + 4 subtests)
A mock HA server (httptest) records calls and asserts the bearer token. Covers:
allowed call → POST to the right endpoint with `entity_id` merged + token; a
service outside the allowlist refused before any HTTP call; empty allowlist
fails closed; `domain.*` wildcard; single allowed/refused read; **bulk read
filtered to the allowlist (anti-enumeration)**; empty read allowlist fails
closed; input validation (missing url/token, empty/unknown op, missing
domain/service); `matchAllowed` wildcard semantics; Definition reflects enabled
axes. Plus 4 new `kernel/edict/toolmap_test.go` cases for the capability mapping.

## Verification

- **Unit:** `plugins/tools/homeassistant` 11/11 pass; `kernel/edict` pass.
- **Full gate:** `GOMAXPROCS=3 go test ./... -p 2 -count=1` exit 0 (80 packages).
  `go vet` + `staticcheck` clean on touched packages; gofmt clean on STAGED LF
  blobs. `go.mod`/`go.sum` unchanged.
- **Runtime smoke (executable proof, criterion-7):** booted the real daemon
  against a mock HA server with `AGEZT_DEMO_HOMEASSISTANT=1`,
  `AGEZT_HOMEASSISTANT_TOOL_READ=light.*`,
  `AGEZT_HOMEASSISTANT_TOOL_SERVICES=light.turn_off`:
  - banner: `tools … homeassistant(read=1, services=1)`.
  - `agt run "turn off the living room light and confirm it"` → run completed
    (3 iterations): `call_service` POSTed `/api/services/light/turn_off` with
    body `{"entity_id":"light.living_room"}` (Bearer `smoke-tok`), then
    `get_states` GET `/api/states/light.living_room` — BOTH authenticated at the
    mock; `task.completed` with the final answer. 0 panics, 0 error tool results.
  - journal `policy.decision` capabilities: exactly one `homeassistant.call` and
    one `homeassistant.read` — per-operation gating confirmed at runtime.
  - **Negative control:** booting with only `…_URL` + `…_TOKEN` (channel-style,
    no TOOL allowlist) → the tool is ABSENT from the banner (fail-closed: no
    auto-exposure). Graceful shutdown clean.

## Counts

- Packages: 79 → **80**. Tests (funcs + subtests): 2448 → **2463**.

## Out of scope (documented follow-ups)

- Reading the HA service registry / entity registry to advertise concrete
  service schemas to the model (currently the model names domain+service freely;
  the allowlist is the guard).
- `agt ha` CLI surface for operators to test reads/calls without a run.
