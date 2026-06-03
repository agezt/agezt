# M251 — Enforce the http tool's host allowlist across redirects

## Why
Pivoting off the vision arc, an audit of the built-in `http` tool found a real
defense-in-depth gap. The tool restricts the agent to an operator-configured
host allowlist, and its client is netguard-protected (default-deny to
internal/metadata IPs on every dial). But the **host allowlist was checked only
on the initial URL** (`Invoke` → `hostAllowed(u.Hostname())`), while the Go
client follows up to 10 redirects automatically. So an allowlisted host that
returns a `302 Location: https://attacker.example/...` would have the follow-up
request — carrying whatever headers the agent set, including any `Authorization`
— sent to a host the operator never allowed. netguard would still block
internal IPs, but an arbitrary *external* host sailed through, defeating the
allowlist and potentially leaking credentials via an open redirect.

## What
- **`plugins/tools/http/http.go`** — `client()` now sets `CheckRedirect` on the
  netguard client: each redirect hop's target host is re-checked against
  `hostAllowed`, and a hop outside the allowlist is refused (surfaced as the
  existing `ErrHostDenied`, tagged "redirect target"). The chain is capped at
  `maxRedirects` (10, made explicit since setting `CheckRedirect` replaces Go's
  default cap). netguard's dial-level IP guard is unchanged and still applies
  per hop; this adds the orthogonal allowlist check the redirect path was
  missing.

## Files
- `plugins/tools/http/http.go` — `CheckRedirect` + `maxRedirects` (edited).
- `plugins/tools/http/redirect_test.go` — 2 tests: a redirect to a
  non-allowlisted host is blocked (and the initial host was still reached), a
  same-host redirect is still followed (new).

## Verification
- `go test ./plugins/tools/http/` — green; full suite **1820 → 1822** (+2), 66
  packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./plugins/tools/http/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- The fix is on the netguard-built client (the production default). A caller
  that injects its own `Tool.HTTP` client owns its own redirect policy.
- `AllowAll` still bypasses the host check uniformly (initial and redirects),
  since `hostAllowed` returns true for any host — behaviour is consistent across
  hops.
- This is a tool-security hardening, independent of the vision arc; chosen as a
  fresh, evidence-based, user-felt target after the vision work wrapped.
