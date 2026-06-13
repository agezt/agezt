# Architecture Map — AGEZT (security-check Phase 1 / Recon)

Scan date: 2026-06-13. Scanner: security-check pipeline.

## 1. Technology Stack
- **Primary language: Go** (module `github.com/agezt/agezt`, go 1.26.3) — ~997 `.go` files. Lean-deps policy (only stdlib + `lukechampine.com/blake3`).
- **Frontend: React + TypeScript** (`frontend/`, ~147 `.tsx` + 77 `.ts`) — Tailwind/shadcn/React Flow SPA, `go:embed`-ded into the binary (`kernel/webui`).
- **SDKs: Python** (`sdk/python`, ~22 `.py`) and **TypeScript** (`sdk/typescript`), plus **Rust** (6 `.rs`).
- Binaries: `agezt` (daemon, `cmd/agezt`) and `agt` (CLI, `cmd/agt`).

## 2. Application Type
Self-hosted **multi-agent orchestration daemon** ("Jarvis"-style). Exposes:
- A **web console** (HTTP, default `localhost:8787`, `kernel/webui`) with token + optional password/session auth.
- A **control-plane** RPC/HTTP server (`kernel/controlplane`) for the `agt` CLI and peers.
- An **agent gateway** (`kernel/agentgw`) — HTTP over unix socket (or TCP) for agent subprocess SDK calls (eventbus/memory/log/config), HMAC-token auth.
- A **REST API** (`kernel/restapi`) and an **OpenAI-compatible API** (`kernel/openaiapi`).
- **Channel plugins** (Slack/Discord/Matrix/Signal/Telegram/WhatsApp/SMS/webhook/homeassistant) — inbound webhooks.
- **MCP** client/host (stdio + Streamable-HTTP), **tunnel**, **plugin host** (subprocess plugins).

## 3. Entry Points (external input)
- `kernel/agentgw/gateway.go`: `/v1/eventbus/*`, `/v1/memory/*`, `/v1/log/*`, `/v1/agent/*`, `/v1/config/*` (Bearer-auth), `/v1/token/create` (**no auth**), `/health`.
- `kernel/webui`, `kernel/restapi`, `kernel/openaiapi`: console + REST + OpenAI-compat HTTP.
- `kernel/controlplane`: CLI/peer RPC.
- `kernel/webhook` + `plugins/channels/*`: inbound 3rd-party webhooks (Slack/Discord/etc.).
- `kernel/mcp`: outbound MCP (stdio subprocess + HTTP).
- CLI args (`cmd/agt`, `cmd/agezt`).

## 4. Security-sensitive sinks
- **Process exec:** `kernel/warden` (sandboxed tool exec), `kernel/tunnel`, `kernel/mcp/client.go` (stdio MCP spawn), `kernel/plugin/host.go`+`pin.go` (plugin spawn), `kernel/creds/aws.go` (`credential_process` shell-parse exec), `cmd/agt/listen.go`.
- **Outbound HTTP (SSRF surface):** `kernel/creds` (AWS IMDS/STS/SSO), `kernel/catalog` (discovery/sync), `kernel/mcp/http.go`, channel plugins. SSRF guard lives in `kernel/netguard`.
- **Crypto:** `kernel/creds/encrypt.go` (vault AES-256-GCM + PBKDF2), `kernel/agentgw/token.go` (HMAC tokens), `kernel/webui/session.go` (session + constant-time password), `kernel/configcenter`, `kernel/governor`, `kernel/webhook` (HMAC sig verify).
- **Sandboxed code-exec tool** (`code_exec`) — deliberately max-capability per owner policy.

## 5. Trust boundaries / auth models
- **Agent gateway:** HMAC-SHA256 "JWT-like" bearer tokens (`kernel/agentgw/token.go`), capability claims (`kernel/agentgw/capabilities.go`). **Signing secret is hardcoded** (`DefaultTokenSecret = "change-me-in-production"`, no env/vault override) — see findings.
- **Web console:** URL token + optional `AGEZT_WEB_PASSWORD` session (constant-time compare, 8-attempt lockout, sliding 12h session). Looks solid.
- **Control plane:** constant-time token (per M187 report).
- **Capability/policy engine:** `kernel/edict` + `kernel/warden` (default-allow posture per owner law; restriction = opt-out).
- **Vault:** `kernel/creds` at-rest AES-256-GCM, machine-bound auto-encrypt (M934).

## 6. Working-tree hygiene risks
- `token.txt`, `temp_token.txt`, `verify_sig.py`, `decode_jwt.py`, `test_token.py`, `test_hash.go/py`, `debug_parse.py` — owner's gateway-auth debugging artifacts, **untracked and NOT covered by `.gitignore`** (`.gitignore` only lists `.env*`, `creds.json`). Secret-leak-into-git risk.

## 7. Detected languages → Phase 2 scanners
- Go → sc-lang-go (primary)
- TypeScript/JS → sc-lang-typescript (frontend + TS SDK)
- Python → sc-lang-python (Python SDK + debug scripts)
- Rust → sc-lang-rust (minor)
Infra: GitHub Actions workflows present (`.github/workflows`) → sc-ci-cd. No Dockerfile/Terraform/K8s detected at root (verify in hunt).
