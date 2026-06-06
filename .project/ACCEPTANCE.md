# Agezt — Acceptance Scorecard ("EKSIKSIZ, sıfır-hata çalışan agentic mimari")

Live PASS/FAIL ledger for the ratified goal. Each criterion records its state, the
last evidence command, and the date. "PASS" = re-verified green by running the
command shown. Goal is MET when every row is PASS or a documented env-bound /
out-of-scope exception. Updated per milestone. Last refresh: 2026-06-07 (M550).

| # | Criterion | State | Last evidence (command) | Date |
|---|---|---|---|---|
| 1 | Build + cross-compile matrix | **PASS** | `go build ./...`=0; `GOOS/GOARCH go build ./...` OK for linux·darwin·windows {amd64,arm64} + freebsd/amd64 | 2026-06-06 |
| 2 | Static analysis | **PASS** | gofmt staged-blobs tree-wide=0 dirty; `go vet ./...`=0; `staticcheck ./...`=0; golangci correctness set triaged (M489) | 2026-06-06 |
| 3 | Tests | **PASS** | `GOMAXPROCS=3 go test ./... -p 2 -count=1`=0 (69 pkgs ok, 2195 tests) | 2026-06-06 |
| 4 | Fuzz (16 targets, 0 crasher) | **PARTIAL** | last active re-run M533; M535–M549 were test+docs only (parsers unchanged). Bounded re-run pending under this goal | — |
| 5 | Mutation floor (47 pkgs) | **PASS** | ratified HARDENING.md floor; no surviving non-equivalent mutant on the rubric package set (M490–M548) | 2026-06-06 |
| 6 | Secrets/security | **PASS** | `gitleaks detect`=0 (617 commits); gosec triaged (M487) | 2026-06-06 |
| 7 | Runtime / E2E (every surface) | **IN PROGRESS** | core proven: daemon boots, `agt run`/`status`/`doctor`/`journal` clean, graceful shutdown, 0 panics, 0 error-journal events (M550). HTTP surfaces / plugins / scheduler / mesh / tenant pending | 2026-06-06 |
| 8 | Plan-faithfulness (vision↔impl) | **TODO** | cross-check .project vision/IMPLEMENTATION/DECISIONS vs shipped surfaces; each capability exercised in §7 | — |
| 9 | CI wires every gate | **PASS** | `.github/workflows/ci.yml`: test+vet+build(3 OS), race, lint, secrets, multi-arch, codegen, deps (M489) | 2026-06-06 |

## Environment-bound / out-of-scope (do NOT block DONE)
- **govulncheck**: 0 requires go ≥ 1.26.4 (offline toolchain is 1.26.3 → 2 stdlib CVEs, toolchain-only fix). CI builds with patched `stable`.
- **race detector**: needs CGO + C compiler (absent offline). CI runs `go test -race`.
- **plan9 / js / wasm**: no process model for a subprocess-spawning daemon — architecturally out of scope.

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
| Out-of-process plugin + MCP bridge | TODO | testdata echoplugin / mcp | — |
| DAG scheduler + HITL gates | **PASS** | M551 schedule add/list+next-fire; M552 HITL: file-edit run blocked on real approval.requested (file.write L2), `agt approve` → granted → tool.invoked → tool.result → file written → run completed; 0 panics (deny is the symmetric path, unit-covered M504) | 2026-06-07 |
| Mesh peer delegation | TODO | two-node `remote_run` | — |
| Multi-tenant isolation | **PASS** | M551: `tenant create/list/token`, run routed to acme, primary journal stays seq=0 (isolated) | 2026-06-07 |
| Pulse engine | **PASS** | M551: `pulse status` running, dial=balanced; `budget` tracking | 2026-06-07 |
| Vault encryption + rotation | **PASS** | M551: `vault encrypt` → aes-256-gcm + pbkdf2 200k, 0 plaintext leak in creds.json; `vault rotate` re-encrypts, entries preserved | 2026-06-07 |
| Warden (OS-permitting) | N/A-Win | Linux prlimit; Windows facade=none (journaled downgrade) | — |
