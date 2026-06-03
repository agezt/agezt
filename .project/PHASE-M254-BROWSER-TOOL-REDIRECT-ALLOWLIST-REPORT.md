# M254 — Enforce the browser tool's host allowlist across redirects

## Why
The `browser` tool deliberately mirrors the `http` tool's allowlist + netguard
design, and it mirrored the M251 bug too: `Invoke` checked the host allowlist
only on the **initial** URL (`hostAllowed(u.Host, …)`), while the fetch client
followed redirects automatically with no allowlist re-check. So an allowlisted
page returning `302 Location: https://attacker.example/…` would have the
follow-up fetch sent to a host the operator never allowed — netguard still
blocked internal IPs, but an arbitrary external host went through.

## What
- **`plugins/tools/browser/browser.go`** — `client()` now sets `CheckRedirect`
  on the netguard fetch client: each redirect hop's host is re-checked against
  `hostAllowed` (honouring `AllowAll`), and an off-allowlist hop is refused via
  the existing `ErrHostDenied` (tagged "redirect target"). The chain is capped
  at `maxRedirects` (10). This is the exact M251 fix applied to the browser
  tool's parallel code.

## Files
- `plugins/tools/browser/browser.go` — `CheckRedirect` + `maxRedirects` (edited).
- `plugins/tools/browser/redirect_test.go` — 2 tests: a redirect to a
  non-allowlisted host is blocked (initial host still reached), a same-host
  redirect is still followed and its content extracted (new).

## Verification
- `go test ./plugins/tools/browser/` — green; full suite **1825 → 1827** (+2),
  66 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./plugins/tools/browser/` clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.

## Scope notes
- Applies to the netguard-built fetch client (the production default); an
  injected `Tool.HTTP` owns its own redirect policy.
- Completes the redirect-allowlist hardening across both URL-fetching tools
  (`http` M251, `browser` M254). The shared `hostAllowed` is duplicated per tool
  by design (the source comment notes it's cheaper than shared infra for an
  eight-line check); both copies are now enforced per hop.
- Fourth milestone in the post-vision tool-security sweep (M251 http redirect,
  M252/M253 file symlinks, M254 browser redirect).
