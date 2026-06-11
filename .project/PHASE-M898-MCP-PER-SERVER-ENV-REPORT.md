# PHASE M898 — Per-server env for MCP servers (credentialed servers work)

**Status:** shipped
**Milestone:** M898 (session range M889–M899; branched from `origin/main`).
**Theme:** Backlog **#39** — makes the M897 catalog's credentialed presets
(github, brave, slack, …) actually usable by letting the operator give one MCP
server exactly the API key it needs — **without un-scrubbing the daemon's own
environment**.

## The design (security-preserving, opt-in)

The MCP attach path spawns the server with a **scrubbed environment**
(`scrubbedEnv()` in `kernel/mcp/client.go` — strips `AGEZT_*` and all
secret-shaped vars). That stays the default. M898 adds an **opt-in per-server
`Env`** map the operator explicitly supplies; those entries are layered on top of
the scrubbed base at attach time. So:

- the daemon's ambient secrets/keys still never leak to a server (scrub intact);
- only the exact values the operator typed for *that* server are injected.

Consistent with `Args`, which already carry secrets in plaintext (e.g. a postgres
URL) — so this isn't a new at-rest exposure class; the SKILL/UI tell the operator
to use a dedicated low-scope token.

## What shipped

- **`kernel/mcp/store.go`** — `Server.Env map[string]string` (+ `Validate`: ≤32
  entries, POSIX env-key names, via `envKeyRe`/`maxEnv`).
- **`kernel/mcp/client.go`** — `Dialer`/`Dial` gain an `env` param; new
  `appendEnv(scrubbedEnv(), env)` overlays the opt-in vars (allowed through even
  when secret-shaped — that's the point).
- **`kernel/runtime/mcptool.go`** — `AttachMCPServer` passes `srv.Env` to `dial`.
  (Only `mcptool.go` changed — `runtime.go` references `mcp.Dialer` by name, so it
  needs no edit and stays merge-compatible with the concurrent arc.)
- **`kernel/controlplane/mcp.go`** — `mcpServerView` **redacts env values**:
  deletes `env` from the wire and exposes only sorted `env_keys`, so `/api/mcp`
  never echoes a secret back.
- **`frontend/src/views/Mcp.tsx`** — a `KEY=value`-per-line **Environment** field
  in the register form (`parseEnv` helper); catalog presets prefill blank `KEY=`
  lines for the secret they need (github/brave/slack); registered cards show
  `env: <KEYS>` (names only). A note explains values are stored and never shown
  again.

## Verification

- **Build/test (clean origin/main base — no concurrent edits present):**
  `go build ./...` clean; `go build` linux/amd64 of the three kernel packages
  clean; `gofmt -l` clean; `go vet` clean. New Go tests: `TestValidateServer`
  (env key rules), `TestAppendEnv` (injection + nil no-op); existing MCP
  attach/detach/controlplane suites green.
- **Frontend:** `tsc --noEmit` clean; `vitest run src/views/Mcp` green **11/11**
  (new `parseEnv` test); `vite build` emits the committed-LF dist.
- No new `AGEZT_*` daemon env var (the per-server env is operator data, not daemon
  config) → the configEnvVars guard is not implicated.

## Notes
- This + M897 deliver the catalog **and** the credential path for #39. Remaining
  #39 depth (SSE transport for remote MCP, lazy context-efficient tool loading)
  stays open. Env values live in `servers.json` like args — a dedicated low-scope
  token is the right call; a future vault-backed reference would remove plaintext
  at rest.
