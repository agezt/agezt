# Phase M574 — Microsoft Teams (outbound) channel

**Type:** Feature (channel sweep; tenth channel)
**Date:** 2026-06-07
**Branch:** `feat-teams-channel`

## Goal

Add Microsoft Teams as an outbound channel so Pulse briefs and `agt send` post to
Teams channels via Incoming Webhooks. Stdlib-only, no dependency, fail-closed.

## What shipped

### `plugins/channels/teams/teams.go` (new)
- Outbound over Teams Incoming Webhooks on `net/http` only.
- Holds a NAMED map of webhooks (name → URL), because Teams webhooks are
  per-channel. `Send` looks up `out.ChannelID` in the map and POSTs a
  `MessageCard` (`{"@type":"MessageCard","@context":…,"text":text}`). Empty text =
  no-op; an unknown name is refused before any HTTP call (fail-closed); a non-2xx
  response errors. `Names()` lists configured names (Pulse fan-out + status).
  `Start` blocks on ctx (outbound-only). Bus event `channel.outbound.teams`.

### `plugins/channels/teams/teams_test.go` (new — 4 tests, all pass)
A mock webhook server records POSTed cards. Covers: configured name → MessageCard
with the text + outbound event; unknown name → refused, no POST, no event;
empty-noop + non-2xx → error; `Names()` enumerates the map.

### Daemon wiring
- `cmd/agezt/main.go`: `buildTeams(ctx, k)` parsing
  `AGEZT_TEAMS_WEBHOOKS=name=url,name2=url2` via a shared `parseNamedWebhooks`
  (splits each entry on the first `=`, since URLs contain `=`); banner,
  `combineSinks`, `liveChannels["teams"]`, `collectChannels()`, pulse brief sink
  fanning out to every configured name.
- `kernel/controlplane/config.go`: `AGEZT_TEAMS_WEBHOOKS` added to `configEnvVars`.

## Verification

- **Unit:** `plugins/channels/teams` — 4/4 pass.
- **Full gate:** `GOMAXPROCS=3 go test ./... -p 2 -count=1` exit 0 (79 packages).
  gofmt / vet / staticcheck clean. `go.mod`/`go.sum` unchanged.
- **Runtime smoke (executable proof, criterion-7):** booted the real daemon with
  `AGEZT_TEAMS_WEBHOOKS=general=<mock>`:
  - banner `teams channel : outbound → 1 Teams webhook(s)`; `agt status` →
    `teams (outbound-only, allow 1)`.
  - `agt send --channel teams --to general "deploy finished"` → `sent to
    teams/general` (POSTed the MessageCard to the mock, 2xx).
  - `agt send --channel teams --to nope …` → `teams: no webhook configured named
    "nope"` (fail-closed). 0 panics.

## Counts

- Packages: 78 → **79**. Tests (funcs + subtests): 2444 → **2448**.

## Out of scope (documented follow-ups)

- Adaptive Card payload for the newer Workflows-based webhooks (currently the
  long-standing MessageCard form).
- Inbound (Teams → Agezt) via the Bot Framework — a separate auth model.
