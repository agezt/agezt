# Architecture Map — security reconnaissance

**Scanned:** 2026-08-12 · `main` @ `905432d3`
**Supersedes:** the 2026-06-27 map taken at `99d2e426`. Between those commits the tree changed by
**1,396 files, +147,262 / −55,557 lines** — the previous map's measurements and several of its
security-relevant claims no longer describe this codebase.

> **Scope of this refresh.** Phase 1 only (recon + dependency audit). The Phase 2 vulnerability
> hunt across 40+ domains has **not** been re-run; `SECURITY-REPORT.md` and the `*-results.md`
> files still describe `ef7b412d`. Treat every finding in them as unverified against current
> source until that hunt runs.

---

## 1. Measurements

| Metric | 2026-06-27 (`99d2e426`) | 2026-08-12 (`905432d3`) | Δ |
|---|---|---|---|
| Go files | ~1,232 | **1,571** | +28% |
| Go LOC (incl. tests) | ~284,000 | **352,988** | +24% |
| Go LOC (non-test) | — | **193,889** | — |
| Go packages | — | **183** | — |
| TS/TSX files | ~340 | **436** | +28% |
| TS/TSX LOC | ~93,000 | **107,293** | +15% |
| TS/TSX LOC (non-test) | — | **77,699** | — |

The previous report's coverage claim ("~1,232 Go files … ~340 TS/TSX files") therefore describes
roughly three quarters of the current tree.

## 2. Technology stack

**Primary:** Go (pure — `CGO_ENABLED=0` is set explicitly in the Makefile, no C dependencies).
**Secondary:** TypeScript/React 19 web console, built with Vite 8 and `go:embed`-ed into the daemon.
**SDKs:** Go (in-tree), Python, TypeScript, Rust — each dependency-free beyond its standard library.

**Frameworks:** none on the Go side. HTTP is stdlib `net/http` throughout; there is no gin/echo/chi/
gorilla. Routing goes through the in-house `kernel/httpserver` router, which carries per-route auth
policy and body limits as data rather than middleware ordering.

**Package managers:** Go modules; npm for the frontend (`package-lock.json` is canonical —
`pnpm-lock.yaml` was removed 2026-07-26 and is now `.gitignore`d, so the previous map's
"npm/pnpm … both present" note is stale).

**Databases: none.** There is no SQL, NoSQL, ORM, or migration surface anywhere in the tree — the
entire injection-by-query class is structurally absent. Persistence is:

- `kernel/jsonstore` — atomic JSON document store behind ~13 typed stores
- `kernel/journal` — append-only, hash-chained event log (tamper-evident; `agt doctor` verifies it)
- `kernel/creds` — machine-bound vault, **AES-256-GCM** with a non-stdlib-dependency KDF

## 3. Trust boundaries and entry points

### Network listeners — corrected

The previous threat model asserted that *"every network listener (Web UI, REST, OpenAI-compatible
API, agent gateway) is off by default and loopback-bound."* **That is wrong for the Web UI**, and
the daemon's own source comment was wrong in the opposite direction. Measured at `httpsurfaces.go`:

| Surface | Default | Bind | Auth |
|---|---|---|---|
| Web UI console | **ON** | `127.0.0.1:8787` | bearer token, or console password; `STRICT` mode requires both |
| REST API (`/api/v1`) | off (blank = off) | operator-chosen | bearer token per route policy |
| OpenAI-compatible API | off (blank = off) | operator-chosen | bearer token |
| Agent gateway | unix socket / loopback | local | HMAC-signed tokens, per-install secret |
| Control plane | always on | `127.0.0.1` ephemeral | bearer token |

The Web UI is loopback-bound and credential-gated, so this is **not an exposure** — but "off by
default" was load-bearing in the old threat model's reasoning and is not true. It is
allow-by-default with an explicit opt-out (`AGEZT_WEB_ADDR=off`), consistent with the project's
stated posture. `cmd/agezt/httpsurfaces.go:61` claimed "unset = off" while the switch sixteen lines
below defaulted it on; that comment was corrected in this pass.

Also public by design on the Web UI surface: `/healthz`, `/readyz` (liveness, unauthenticated) and
`/metrics` (its own route policy).

### Other inbound surfaces

- **15 channel webhook listeners** (`chatwebhook`, `dingtalk`, `discord`, `feishu`, `imessage`,
  `line`, `nextcloudtalk`, `onebot`, `slack`, `sms`, `webhook`, `wecom`, `whatsapp`, `whatsappgw`,
  `zalo`) — each off unless its channel is configured, each binding an operator-chosen address.
  These are the widest inbound surface in the tree and the one most likely to be internet-facing,
  since webhooks must be reachable by the provider.
- **OAuth callback** (`controlplane/provider_oauth.go`) — a short-lived loopback listener during
  sign-in only.
- **`/hooks`** — the token-free path, and the only surface with a default rate limit
  (deliberate: authed run endpoints stay unthrottled per the project's default-allow posture).

### Untrusted input reaching the model

Channel messages, webhook bodies, fetched web content, MCP tool results, and file contents all
reach the agent loop. The prompt-injection guard (`AGEZT_PROMPT_INJECTION_GUARD`, default `warn`)
journals directive-shaped untrusted content and, at `on`/`block`, routes downstream effectful
actions to human approval.

### Egress

All outbound HTTP goes through `kernel/netguard`, an SSRF dialer guard that validates the
**resolved** IP on every redirect hop rather than the hostname. `Set.NetguardGaps` now runs at boot
(added 2026-08-12) and warns if any egress-guarded tool would fail to journal its refusals; it
currently reports none.

## 4. Code execution surface

- `plugins/tools/shell` and `plugins/tools/codeexec` are the command-execution choke point:
  env-scrubbed (`kernel/envscrub`), edict-gated, array-form `exec.Command`, warden-profiled.
- `kernel/executionprofile` adds remote execution backends (Kubernetes, Modal, Daytona, SSH) —
  13 files changed since the last scan and **not covered by any Phase 2 hunt**.
- `code_exec` is deliberately maximum-capability (network on, allow-by-default) per an explicit
  owner decision; secret-scrubbing, isolation and audit are the non-negotiable parts.

## 5. Packages added since the last scan

Not present at `99d2e426`, therefore never security-reviewed:

`kernel/auth` · `kernel/httpserver` · `kernel/toolreg` · `kernel/channelwire` · `kernel/selfrepair` ·
`kernel/proof` · `kernel/okr` · `kernel/assure` · `kernel/resume` · `kernel/chatgptauth` ·
`kernel/executionprofile` · `kernel/cadence/systemtasks` · `cmd/agezt/internal/daemonconfig` ·
`plugins/providerboot`

`kernel/auth` and `kernel/httpserver` matter most: they centralised route authorisation and body
limits, so every `/api/v1` access-control decision now flows through code the last assessment never
saw. `kernel/chatgptauth` and `kernel/executionprofile` add an OAuth token store and remote-exec
credential handling respectively.

## 6. What Phase 2 should prioritise

Ranked by (changed surface × blast radius), for whoever runs the hunt:

1. `kernel/auth` + `kernel/httpserver` — new, and the single choke point for API authz
2. `kernel/executionprofile` — remote code execution with credential forwarding
3. `kernel/agentgw` — token minting/validation (11 files changed)
4. `kernel/controlplane` — 129 files changed; the largest authenticated command surface
5. `kernel/webui` — 232 files changed; CSP, session, file-browser routes
6. `plugins/tools` — 93 files changed; the governed capability surface
7. `frontend/src` — 304 files changed; XSS/CSP/clickjacking re-check
