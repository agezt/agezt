# Refactor A3 + B5 (reconciled) — `kernel/auth` (domain) + `kernel/httpserver` (transport)

> Reconciles `docs/REFACTORING-SCAN.md` findings **A3** (six overlapping HTTP surfaces) and
> **B5** (no dedicated auth package). **Supersedes** the standalone
> `docs/REFACTOR-A3-HTTPSERVER-PLAN.md` for the auth-related phases.
> **Generated:** 2026-07-03. Grounded in a measured auth-surface scan.

## The overlap, and why it's a layer split (not a merge)

- **A3** wanted `kernel/httpserver` with an `Authenticator` — but scoped to HTTP transport
  (token gate, bearer parse, body caps, listener, router).
- **B5** wanted `kernel/auth/{token,middleware,tenant,oauth}.go` — the auth domain
  (token verify, **tenant resolution**, **OAuth flows**).

They overlap only on the middleware + token-compare. Measured surface shows auth is broader than HTTP:

| Concern | Lives today | Transport? |
|---|---|---|
| token mint / constant-time verify | copied ×3 (webui/restapi/openaiapi) | shared |
| bearer / `?token=` parse | 4 surfaces | transport |
| tenant resolution | controlplane/tenant.go (9.6 KB) + kernel/tenant + kernel/tenantctx | **domain** |
| channel OAuth | controlplane/channel_oauth.go (12.1 KB) | **domain** |
| provider OAuth | controlplane/provider_oauth.go (7.6 KB) | **domain** |
| ChatGPT auth | kernel/chatgptauth | **domain** |
| credentials | kernel/creds (13 files) | **domain** |

**Verdict:** split by layer. `kernel/auth` owns the domain; `kernel/httpserver` owns transport and
**imports** `kernel/auth`. A3's `Authenticator` becomes a thin transport adapter over `auth.Verifier`.

## Unified layout

```
kernel/auth/                    (B5 — domain)
├── token.go   Verifier: mint, constant-time verify, WriteTokenFile(0600)
├── tier.go    Tier{Public,User,Admin}; credential→tier
├── tenant.go  tenant resolution (absorbs controlplane/tenant.go, wraps kernel/tenant)
└── oauth/{channel.go, provider.go}  (absorb controlplane/{channel,provider}_oauth.go)

kernel/httpserver/              (A3 — transport; imports kernel/auth)
├── authmw.go   Middleware(tier) — thin adapter calling auth.Verifier
├── listener.go Listen(cfg): tcp|unix|tls; drain; uses auth.WriteTokenFile
├── mux.go      Router.AddRoute(method, path, RouteOpts{Tier,BodyMax,Timeout}, h)
└── limits.go   BodyLimit
```

```go
// kernel/auth
type Verifier interface {
    Authorize(presented string, tier Tier) bool          // constant-time
    ResolveTenant(r *http.Request) (TenantID, error)
}
// kernel/httpserver
func (rt *Router) authMiddleware(tier auth.Tier) mw       // delegates to injected auth.Verifier
```

## Phases (gate: `go build ./... && go vet ./kernel/... && go test ./kernel/{auth,httpserver,<surface>}/...`)

- **P0 kernel/auth (domain) first:** `Verifier` (constant-time, sseToken second credential, bearer +
  `?token=` fallback, ported from webui reference), `Tier`, `WriteTokenFile` (absorbs
  `writeAPIListenToken`). No surface touched. Gate: `kernel/auth`.
- **P1 kernel/httpserver (transport):** `Listener`, `Router.AddRoute`, `BodyLimit`, `authMiddleware`
  delegating to `auth.Verifier`. Gate: `kernel/httpserver`.
- **P2 openaiapi:** smallest surface; auth via `auth.Verifier`, routes via `Router`, caps via `RouteOpts`. Gate: 9 tests.
- **P3 restapi:** tier model (`TierUser`/`TierAdmin` replaces `auth`/`adminAuth`); wire
  `SetTenantAuthorizer` → `auth.Verifier.ResolveTenant`. Gate: 9 tests.
- **P4 tenant + OAuth → kernel/auth (SECURITY):** relocate controlplane/tenant.go + channel_oauth.go +
  provider_oauth.go into kernel/auth; controlplane keeps HTTP entry points, calls
  `auth.OAuth.ExchangeChannel(...)` / `auth.ResolveTenant(...)`. Own PR; gate: OAuth ctx-cancel tests + gitleaks.
- **P5 webui:** auth via `auth.Verifier` (primary + sseToken); caps via `RouteOpts`.
  **Must precede A1 Phase 6** (route split rides on `Router`). Gate: 13 tests.
- **P6 agentgw (SECURITY):** replace token.go+secret.go with `auth.Verifier` (agentgw tokens = another
  credential source + pluggable secret store); replace raw net.Listen/http.Server{} with
  `httpserver.Listen`. Own PR; gate: gitleaks + 6 tests.
- **P7 token-file dedup:** `WriteTokenFile` removes the last copy; verify cmd/agezt uses it.

## Sequencing

```
P0 auth(domain) → P1 httpserver(transport) → P2 openaiapi → P3 restapi(tiers)
→ P4 tenant+oauth→auth (security, own PR) → P5 webui (BEFORE A1 P6) → P6 agentgw (security) → P7 token-file
```

## Why two packages, not one

A single `auth+httpserver` blob re-creates the layering problem — HTTP listener lifecycle has no
business next to OAuth token exchange. The split keeps `kernel/auth` importable by non-HTTP consumers
(CLI, agentgw secret store, tunnel auth) without dragging in `net/http` server machinery.

## Cross-plan constraints

- **P5 (webui) before A1 Phase 6** — A1's per-domain route registrars call `httpserver.Router.AddRoute`.
- **Disjoint from A1 in controlplane:** A1 moves domain folds (runs/roster/board); this moves
  auth/oauth/tenant (tenant.go, *_oauth.go). Different file sets → can run in parallel if coordinated.
- `acp` remains out of scope (client, not a listener).
- Supersedes REFACTOR-A3-HTTPSERVER-PLAN.md for auth phases; that doc's non-auth transport detail still applies.
