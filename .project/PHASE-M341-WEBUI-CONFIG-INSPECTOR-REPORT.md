# M341 — Web UI config inspector panel

## Why
Priority-B work: a genuinely missing, offline-verifiable first-party surface. The
daemon already exposes a `config` control-plane command (`agt config`) that answers
"what is this daemon actually running with?" — resolved paths, model, system-prompt
flag, tool/plugin counts, ask-policy, env-var presence, and routing tables. But the
web UI had **no** panel for it: every other read command (status, runs, budget,
providers, tools, policy, schedules, memory, world, skills, inbox, reflect,
approvals) was surfaced, except config. An operator monitoring via the browser
couldn't see the daemon's effective config without dropping to the CLI.

The backend is privacy-safe by construction (env reports PRESENCE only, never
values; `system_prompt_set` is a bool, never the prompt text), so surfacing it in
the browser leaks nothing.

## What
- **`kernel/webui/webui.go`**: added `/api/config → controlplane.CmdConfig` to the
  read-only `apiRoutes` proxy map (one line; read-only by construction).
- **`kernel/webui/dashboard.html`**:
  - a new "Config" panel section (after Status) with the standard ↻ refresh button;
  - a `config` renderer: a key/value block (model, system-prompt set/—, tool &
    plugin counts, ask-policy), an env-var-presence list rendered as name-only
    tags, the resolved base paths, and the routing tables (when present) in a
    scrollable `<pre>`;
  - CSS for `.envlist` / `.tag` (the presence chips) and `.cfgpre`.

## Verification
- **httptest** (`kernel/webui/webui_test.go`): `TestConfigRouteProxiesConfig`
  asserts `GET /api/config` proxies exactly the `config` command; the existing
  `TestAPIReadOnly` guard (extended with `config`) confirms it is a read-only
  command — a mutating command could never be added to this map without failing
  the test.
- **Live Playwright** against a real daemon (`AGEZT_PROVIDER=mock`,
  `AGEZT_SYSTEM_PROMPT="you are a test agent"`, `AGEZT_WEB_ADDR=127.0.0.1:8772`):
  the Config panel renders model=mock, system prompt=set, tools=7, plugins=0,
  ask policy=allow, "5 env var(s) set" as name tags (AGEZT_FORCE_START / _HOME /
  _PROVIDER / _SYSTEM_PROMPT / _WEB_ADDR), and the base paths. **Privacy proven
  visibly**: `AGEZT_SYSTEM_PROMPT` shows as a presence tag and the prompt shows
  "set" — the actual prompt text ("you are a test agent") appears NOWHERE in the
  panel, and no env var value is shown.
- `gofmt -l` clean (Go files); `go vet ./kernel/webui/` clean; `GOOS=linux go
  build ./...` exit 0. Full suite **2064** passing (was 2063; +1), `go test ./...`
  exit 0. `go.mod` / `go.sum` unchanged.

## Scope notes
- Read-only: the panel only fetches; no config mutation from the browser.
- The control-plane handler (`kernel/controlplane/config.go`) was unchanged — this
  milestone is purely the missing web surface over an existing, already-tested
  command.
