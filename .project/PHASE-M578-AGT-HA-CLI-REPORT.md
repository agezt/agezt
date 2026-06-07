# Phase M578 — `agt ha` operator CLI for Home Assistant

**Type:** Feature (CLI; M575 follow-up — operator complement to the HA tool)
**Date:** 2026-06-08
**Branch:** `feat-agt-ha-cli`

## Goal

Give the operator a direct Home Assistant client from the terminal: read state,
discover the service registry, and call services — without a running daemon. This
complements the agent-facing `homeassistant` TOOL (M575): the tool is fail-closed
behind read/service allowlists so a prompt-injected agent is constrained, whereas
this CLI is the OPERATOR acting with their own authority, so it has full access.
`agt ha services` is also the introspection an operator uses to discover which
`domain.service` names to put in `AGEZT_HOMEASSISTANT_TOOL_SERVICES`.

## What shipped

### `cmd/agt/ha.go` (new) — `agt ha <command>`
Self-contained; reads `AGEZT_HOMEASSISTANT_URL`/`_TOKEN` and talks to HA's REST
API directly (`net/http` only, no daemon, no new dependency):
- **`states [entity_id] [--json]`** — GET `/api/states[/{id}]`. All entities print
  as sorted `entity_id = state` lines + a count; one entity prints as pretty JSON.
- **`services [--json]`** — GET `/api/services`, printed as sorted `domain.service`
  lines + a count (the service-registry introspection).
- **`call <domain.service> [--entity id] [--data '<json>'] [--json]`** — POST
  `/api/services/{domain}/{service}` with the data object (+ merged `entity_id`);
  prints `called X.Y ok` and the changed-states result.
- Missing URL/token → exit 2 with a hint; transport error / non-2xx → exit 1 with
  the status; bad `--data` JSON or a non-dotted target → exit 2. `--json` prints
  raw responses; bodies are size-capped (1 MiB) for display.

### Wiring
- `cmd/agt/main.go`: `case "ha"` dispatch + a line in `agt help`.

### Tests — `cmd/agt/ha_test.go` (new; 10 tests)
Against an `httptest` HA mock (env pointed at it, asserting the bearer token):
no-config fail-closed (exit 2); help; states-all listing + count; single-entity
pretty JSON; services `domain.service` listing + count; call POSTs with merged
`entity_id` + forwarded data; call requires a dotted target; bad `--data` JSON;
unknown subcommand; non-2xx → exit 1 with `HTTP 500`.

## Verification

- **Unit:** `cmd/agt -run TestHA` — 10/10 pass.
- **Full gate:** `GOMAXPROCS=3 go test ./... -p 2 -count=1` exit 0 (80 packages).
  `go vet` + `staticcheck` clean on `cmd/agt`; gofmt clean. `go.mod`/`go.sum`
  unchanged.
- **Runtime smoke (executable proof, criterion-7):** built the real `agt` binary,
  pointed it at a mock HA, and ran each subcommand:
  - `agt ha services` → `climate.set_temperature` / `light.turn_off` /
    `light.turn_on` + `(3 services)`.
  - `agt ha states` → `light.living_room = on` / `lock.front_door = locked` +
    `(2 entities)`.
  - `agt ha states light.living_room` → pretty JSON with attributes.
  - `agt ha call light.turn_off --entity light.living_room` → `called
    light.turn_off ok` + result; the mock confirmed `POST
    /api/services/light/turn_off {"entity_id":"light.living_room"}`. All
    bearer-authenticated.

## Counts

- Packages unchanged (80); `cmd/agt` gains 10 tests → tree 2463 → **2473**.

## Out of scope (documented follow-ups)

- Routing `agt ha` through the daemon's control plane (current direct-to-HA design
  is simpler and works without a daemon; a control-plane variant could reuse the
  in-process tool + its allowlists for a "what would the agent be allowed to do?"
  view).
- Entity-registry introspection beyond states (areas, devices).
