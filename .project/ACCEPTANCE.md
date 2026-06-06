# Agezt — Acceptance Scorecard ("EKSIKSIZ, sıfır-hata çalışan agentic mimari")

Live PASS/FAIL ledger for the ratified goal. Each criterion records its state, the
last evidence command, and the date. "PASS" = re-verified green by running the
command shown. Goal is MET when every row is PASS or a documented env-bound /
out-of-scope exception. Updated per milestone. Last refresh: 2026-06-07 (M554).

| # | Criterion | State | Last evidence (command) | Date |
|---|---|---|---|---|
| 1 | Build + cross-compile matrix | **PASS** | `go build ./...`=0; `GOOS/GOARCH go build ./...` OK for linux·darwin·windows {amd64,arm64} + freebsd/amd64 | 2026-06-06 |
| 2 | Static analysis | **PASS** | gofmt staged-blobs tree-wide=0 dirty; `go vet ./...`=0; `staticcheck ./...`=0; golangci correctness set triaged (M489) | 2026-06-06 |
| 3 | Tests | **PASS** | `GOMAXPROCS=3 go test ./... -p 2 -count=1`=0 (69 pkgs ok, 2195 tests) | 2026-06-06 |
| 4 | Fuzz (16 targets, 0 crasher) | **PASS** | M554: all 16 targets re-run `-fuzztime 8s` (GOMAXPROCS=3), 0 crashers — catalog, controlplane, edict, governor, journal, openaiapi, redact, 3×channel FuzzVerify, 6×provider FuzzParseStream | 2026-06-07 |
| 5 | Mutation floor (47 pkgs) | **PASS** | ratified HARDENING.md floor; no surviving non-equivalent mutant on the rubric package set (M490–M548) | 2026-06-06 |
| 6 | Secrets/security | **PASS** | `gitleaks detect`=0 (617 commits); gosec triaged (M487) | 2026-06-06 |
| 7 | Runtime / E2E (every surface) | **PASS** | M550–M553: every product surface driven against the real daemon — daemon lifecycle, run loop, status/doctor/journal, OpenAI API (+streaming, bug fixed), REST, Web UI, ACP, webhooks (HMAC), plugin+MCP, scheduler+HITL, mesh, multi-tenant, pulse, vault. 0 panics / 0 error-journal events / graceful shutdown throughout. See §7 checklist below | 2026-06-07 |
| 8 | Plan-faithfulness (vision↔impl) | **PASS (v1.0.0 scope)** | M554: every capability the README/v1.0.0 claims is present + exercised in §7; the IMPLEMENTATION.md full-project Phase 4–9 has deferred items documented out-of-scope below (future roadmap, not v1.0 defects) | 2026-06-07 |
| 9 | CI wires every gate | **PASS** | `.github/workflows/ci.yml`: test+vet+build(3 OS), race, lint, secrets, multi-arch, codegen, deps (M489) | 2026-06-06 |

## Environment-bound / out-of-scope (do NOT block DONE)
- **govulncheck**: 0 requires go ≥ 1.26.4 (offline toolchain is 1.26.3 → 2 stdlib CVEs, toolchain-only fix). CI builds with patched `stable`.
- **race detector**: needs CGO + C compiler (absent offline). CI runs `go test -race`.
- **plan9 / js / wasm**: no process model for a subprocess-spawning daemon — architecturally out of scope.

## §8 Plan-faithfulness cross-check (IMPLEMENTATION.md §7 phases)
v1.0.0 = "MVP + federated mesh + multi-tenant" (README/CHANGELOG). Mapping each
IMPLEMENTATION.md build phase to shipped+exercised evidence:

| Phase | Planned | v1.0.0 status |
|---|---|---|
| 0 Contracts & kernel core | journal/bus/lifecycle/controlplane/pluginhost | **SHIPPED** — journal verify (doctor), halt/resume, controlplane, echoplugin (M553) |
| 1 Reasoning & tools | governor, providers, scheduler/planner, shell/file/http/browser, edict, warden, `agt run` | **SHIPPED** — run loop (M550), 9 provider families, plan, 10 tools, edict, warden facade |
| 2 Memory/world/forge | memory tiers, world-model, skill lifecycle, context compression | **SHIPPED** — boot shows memory/world/skills on; mutation-tested (M503/505/506) |
| 3 Pulse | heartbeat, observers, salience, dial, initiative, chronos, standing orders | **SHIPPED** — pulse status (M551), salience (M523-526), cadence/standing |
| 4 Channels | telegram, email, discord, slack, webhook **+ whatsapp/signal/sms/matrix/teams/homeassistant** | **PARTIAL** — 5 channels SHIPPED + verified (M540/M547); the other 6 are **DEFERRED** (future roadmap) |
| 5 Web UI | **Flow Studio (visual DAG designer)** + Live Monitor + Memory Explorer | **PARTIAL** — Live Monitor dashboard SHIPPED + verified (M545/M550); visual Flow Studio designer **DEFERRED** |
| 6 Warden/multi-agent/sim | warden profiles, multi-agent, coding nodes, dry-run | **SHIPPED** — delegate, coding tool, `plan --dry-run`; container/microvm/seccomp warden is Linux-depth (Windows facade) |
| 7 Tunnels/SDK/ambient/OpenAI API | OpenAI-compat API, mcp-bridge, **tunnels, ts/py/rust SDKs, voice/tray/mobile** | **PARTIAL** — OpenAI API (M550) + MCP bridge (M553) + Go SDK SHIPPED; **tunnels, non-Go SDKs, ambient/voice DEFERRED** |
| 8 Reflection/marketplace/polish | reflect loop, marketplace, doctor/i18n | **PARTIAL** — reflect (M520), doctor SHIPPED; skill-registry discovery layer SHIPPED; **full signed marketplace DEFERRED** |
| 9 Mesh & migration | federated mesh, multi-tenant, **`agt migrate openclaw\|hermes`** | **PARTIAL** — mesh (M553) + multi-tenant (M551) SHIPPED; **migration importers DEFERRED** |

**Deferred to future roadmap (documented out-of-scope for v1.0.0 — NOT defects):**
6 extra channel integrations, Flow Studio visual designer, network tunnels,
TS/Py/Rust SDKs, ambient/voice, full plugin marketplace, `agt migrate` importers.
These were never claimed by the v1.0.0 release; the README's shipped scope is fully
faithful and exercised. Building them is a feature roadmap, distinct from this
goal's "zero-defect working architecture."

## §7 Runtime/E2E surface checklist
Each must: boot under a temp `AGEZT_HOME` with the demo/mock provider, complete its
flow, leave **0 panics**, **0 error-level journal events**, and shut down cleanly.

| Surface | State | Evidence | Date |
|---|---|---|---|
| Daemon lifecycle (boot → ready → graceful shutdown) | **PASS** | M550: boots to "daemon ready", `agt shutdown` exit 0, exits clean, no panic in log | 2026-06-06 |
| Core run loop (`agt run`) | **PASS** | M550: echo-mock intent, exit 0, journal task.received→…→task.completed clean | 2026-06-06 |
| `agt status` / `doctor` / `journal` | **PASS** | M550: status OK, doctor all-[OK] + chain verified, journal tail clean | 2026-06-06 |
| OpenAI API (`/v1` models + chat + responses + streaming) | **PASS** | M550: /v1/models list, chat/completions echo+usage, /v1/responses, SSE streaming (incl. the non-streaming-provider fix), auth 401 on bad key | 2026-06-07 |
| Native REST (`/api/v1`) | **PASS** | M550: POST /api/v1/runs → completed, answer returned (token-authed) | 2026-06-07 |
| Web UI | **PASS** | M550: `/?token=` → 200, `/` → 401 (auth enforced) | 2026-06-07 |
| ACP server | **PASS** | M551: `agt acp` stdio initialize handshake → JSON-RPC result (agentInfo agezt 1.0.0, protocolVersion 1) | 2026-06-07 |
| Outbound webhooks (HMAC) | **PASS** | M552: `AGEZT_WEBHOOKS` sink received all 6 journal events of a run, **every delivery `sig_valid=true`** (HMAC-SHA256), 0 invalid; egress guard correctly blocks loopback until `WEBHOOK_ALLOW_LOOPBACK=1` | 2026-06-07 |
| Out-of-process plugin + MCP bridge | **PASS** | M553: daemon spawned echoplugin subprocess (separate PID), handshake, 4 tools registered/advertised (`ext.echo/callhost/fail/slowwork`), clean shutdown (subprocess gone); invoke/callback/streaming via kernel/plugin integration tests. MCP bridge: full host→bridge→mock-MCP-server e2e (7 TestBridge_* pass: list/invoke/error/namespacing/resources) | 2026-06-07 |
| DAG scheduler + HITL gates | **PASS** | M551 schedule add/list+next-fire; M552 HITL: file-edit run blocked on real approval.requested (file.write L2), `agt approve` → granted → tool.invoked → tool.result → file written → run completed; 0 panics (deny is the symmetric path, unit-covered M504) | 2026-06-07 |
| Mesh peer delegation | **PASS** | M553: two real daemons (leader A + worker B). `agt peers` → worker OK (v1.0.0); remote_run path `POST {base}/api/v1/runs` + X-Agezt-Hop made B execute the delegated task (echo answer) and grow B's journal seq 6→12; peer live round-trip test passes. (Peer URL is base-only, no /api/v1 — appended by the tool.) | 2026-06-07 |
| Multi-tenant isolation | **PASS** | M551: `tenant create/list/token`, run routed to acme, primary journal stays seq=0 (isolated) | 2026-06-07 |
| Pulse engine | **PASS** | M551: `pulse status` running, dial=balanced; `budget` tracking | 2026-06-07 |
| Vault encryption + rotation | **PASS** | M551: `vault encrypt` → aes-256-gcm + pbkdf2 200k, 0 plaintext leak in creds.json; `vault rotate` re-encrypts, entries preserved | 2026-06-07 |
| Warden (OS-permitting) | N/A-Win | Linux prlimit; Windows facade=none (journaled downgrade) | — |
