# Refactor A3 — `kernel/httpserver` extraction (shared HTTP substrate)

> Companion to `docs/REFACTORING-SCAN.md` finding **A3**.
> **Generated:** 2026-07-03. Grounded in a measured signal-count across the surfaces.

## Evidence (measured)

Five listener surfaces (+ `acp`, a **client** — scoped OUT):

| Surface | Main file | Own auth? | Own listener? | Body caps (`MaxBytesReader`) |
|---|---|---|---|---|
| `webui` | webui.go (86.6 KB) | `auth()`/`authorized()` + `ConstantTimeCompare` ×4 | via controlplane server.go | ×6 |
| `restapi` | restapi.go (23.3 KB) | `auth()`/`authorized()` + **`adminAuth()`** + ×1 | own | ×4 |
| `openaiapi` | openaiapi.go (29.5 KB) | `auth()`/`authorized()` + ×1 | own | ×2 |
| `agentgw` | gateway.go + **token.go (5.7 KB)** + **secret.go (4.8 KB)** | full parallel token/secret stack | `net.Listen` ×6, `http.Server{}` | ×5 |
| `acp` | acp.go + client.go | — (client) | — | — |

**Confirmed duplication (verified in source):**
- Three independent copies of `func (s *Server) auth(next) / authorized(r) bool` + the gate
  `subtle.ConstantTimeCompare([]byte(presented), []byte(s.token)) == 1`:
  webui.go:1229/1406/1422, restapi.go:270/280, openaiapi.go.
- restapi adds a second tier `adminAuth()` (restapi.go:311).
- Bearer/Authorization parse hand-rolled in 4 surfaces (webui 5, restapi 3, openaiapi 3, agentgw 1).
- `MaxBytesReader` caps hand-applied in every surface (6/4/2/5).
- `http.NewServeMux` once per surface; agentgw owns the only raw `net.Listen` + `http.Server{}`.

**Design principle:** extract the cross-cutting *mechanism* (token gate, bearer parse, body cap,
listener/lifecycle, mux builder) into `kernel/httpserver`. Leave each surface's routes and handlers
in place — this is a shared substrate, NOT a merge of the surfaces. `acp` is out of scope (client).

## Target package

```
kernel/httpserver/
├── doc.go        boundary note
├── auth.go       Authenticator: token + bearer parse + constant-time compare + tiers
├── listener.go   Listen(cfg): tcp|unix|tls; graceful drain; 0600 token-file write
├── mux.go        Router.AddRoute(method, path, opts, handler): body-cap + auth wrappers
├── limits.go     BodyLimit middleware (named MaxBytesReader cap)
├── middleware.go compose: auth → limit → timeout → handler
└── *_test.go
```

```go
type Tier int // TierPublic, TierUser, TierAdmin
type Authenticator struct { /* primary token, sse token, admin token, tenant hook */ }
func (a *Authenticator) Middleware(t Tier) func(http.HandlerFunc) http.HandlerFunc
func (a *Authenticator) Authorized(r *http.Request, t Tier) bool // constant-time

type Config struct { Network, Addr, UnixPath string; TLS *tls.Config; TokenFile string }
func Listen(ctx context.Context, cfg Config) (*Server, error)
func (s *Server) Serve(h http.Handler) error

type Router struct { /* *http.ServeMux + Authenticator */ }
type RouteOpts struct { Tier Tier; BodyMax int64; Timeout time.Duration }
func (rt *Router) AddRoute(method, path string, opts RouteOpts, h http.HandlerFunc)
```

Absorbs: the 3 copied `auth/authorized/ConstantTimeCompare`, per-surface `MaxBytesReader`,
and agentgw's bespoke `net.Listen`/`http.Server{}`.

## Phases (gate: `go build ./... && go vet ./kernel/... && go test ./kernel/{httpserver,<surface>}/...`)

- **P0 scaffold + tests-first:** build the package; port webui's auth behavior as the reference
  (constant-time compare, sseToken second credential, bearer + `?token=` fallback); write
  `auth_test.go` covering all of it. No surface imports it yet — additive, zero risk.
- **P1 openaiapi (smallest, 2 files):** swap `auth/authorized` → `Authenticator`; `MaxBytesReader`
  → `RouteOpts.BodyMax`; mount via `Router.AddRoute`. Keep `responses.go` handlers. Gate: 9 tests.
- **P2 restapi (admin tier):** swap `auth`/`authorized`/**`adminAuth`** → `Middleware(TierUser|TierAdmin)`;
  wire `SetTenantAuthorizer` into the Authenticator. Validates the Tier design. Gate: 9 tests.
- **P3 webui:** point at `httpserver.Authenticator` (primary + sseToken); replace 6 `MaxBytesReader`.
  **Do before A1 Phase 6** — the route split must ride on the shared `Router`, not raw `ServeMux`.
  Gate: 13 tests (no frontend change → dist-in-sync unaffected).
- **P4 agentgw (parallel auth stack; highest value + risk):** replace token.go+secret.go verification
  with `Authenticator` (agentgw tokens = another credential source, `TierUser` + pluggable secret
  store); replace raw `net.Listen`+`http.Server{}`+unix lifecycle with `httpserver.Listen`. Own PR;
  gate: gitleaks + agentgw's 6 tests.
- **P5 token-file dedup:** move `writeAPIListenToken` (near cmd/agezt/main.go) into
  `httpserver.Config.TokenFile` — `Listen` writes the 0600 file, returns the prefix. Gate: cmd/agezt + surfaces.

## Sequencing

```
P0 scaffold  ← additive
P1 openaiapi ← proves auth+bodycap
P2 restapi   ← proves tier model
P3 webui     ← BEFORE A1 P6 (route split rides on Router)
P4 agentgw   ← highest value/risk (security); own PR + gitleaks
P5 token-file
```

## Non-goals / interactions

- Does NOT merge the surfaces, does NOT touch `acp` (client), does NOT change any route path or
  response shape. Replaces the copied mechanism with a shared one only.
- **Ordering with A1:** A3 P3 must precede A1 P6 (A1 P6's per-domain registrars call
  `httpserver.Router.AddRoute`, so the router must exist first).
- Per-PR: also run `go run ./tools/deadcodecheck` (each migration deletes a copied auth/authorized);
  P4 additionally gates on the gitleaks secrets check.
