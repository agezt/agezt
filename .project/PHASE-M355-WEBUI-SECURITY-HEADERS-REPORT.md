# M355 — Web UI defensive response headers

## Why
Priority-A security hardening on the daemon's main browser-facing attack surface.
The web monitor is a **control surface**: the dashboard renders state-mutating
controls (approve / deny / halt / resume / decide → POST `/api/...`), and its URL
carries the auth token in `?token=` (EventSource can't set an Authorization
header, so the query param is the browser's only option). Yet the responses set no
defensive headers, leaving two concrete, relevant exposures:

- **Clickjacking** — with no `X-Frame-Options`, an attacker page could iframe the
  dashboard and trick the operator into clicking the (invisible) "approve" button
  on a pending HITL approval.
- **Token leak via Referer** — with no `Referrer-Policy`, if the page ever loads or
  links an external resource, the browser sends the full URL (including
  `?token=<secret>`) in the `Referer` header.

These are standard OWASP secure-header recommendations, especially warranted for a
control UI with a token-in-URL.

## What
Production hardening + lock-in test.
- **`kernel/webui/webui.go`** — `setSecurityHeaders(w)` is called at the top of the
  `auth` middleware (before the auth check, so even 401s carry the headers), on
  every route. Sets:
  - `X-Frame-Options: DENY` (clickjacking),
  - `Referrer-Policy: no-referrer` (token-in-URL leak),
  - `X-Content-Type-Options: nosniff` (MIME sniffing).
- **`kernel/webui/webui_test.go`** — `TestSecurityHeadersOnEveryResponse`: asserts
  all three headers on both an authorized (200) and an unauthorized (401) response.

## Verification
- `go test ./kernel/webui -run 'SecurityHeaders|DashboardServedAtRoot|AuthRequired' -v`
  — passes; the existing dashboard (`Content-Type: text/html`) and auth tests are
  unaffected (the new headers are additive).
- `gofmt -l` clean; `go vet ./kernel/webui/` clean; `GOOS=linux go build ./...`
  exit 0. Full suite **2089** passing (was 2086; +3 incl. subtests), `go test
  ./...` exit 0. `go.mod`/`go.sum` unchanged. CHANGELOG updated (Security).

## Scope notes
- No `Content-Security-Policy` is set: the embedded dashboard uses inline scripts,
  so a strict CSP would break it without a refactor to externalised/nonce'd scripts
  — deliberately out of scope here (noted for a future hardening pass). Content-Type
  is already correctly set on every response (html / event-stream / json), so
  `nosniff` is belt-and-suspenders; the framing and referrer headers are the
  load-bearing additions for this control surface.
