# M435 — Web dashboard: per-response CSP nonce (defense-in-depth)

## Context
XSS / web-vuln review of the embedded operator dashboard
(`kernel/webui/dashboard.html`, 1164 lines vanilla JS; `kernel/webui/webui.go`,
the server). The dashboard renders data from untrusted sources — LLM/agent
output, tool results, run transcripts, memory/world-model content, skill text,
channel messages, error strings, remote-catalog model/provider names, peer
responses — and is same-origin with the token-authed, state-mutating control
plane, so any XSS there is the equivalent of full daemon control.

## Review result: NO XSS found
The dashboard is built entirely with text-node DOM construction (the `textContent`
property). The reviewer enumerated and ruled out every dangerous sink: no
dynamic-HTML assignment sink, no markup-insertion call, no legacy document
stream-write, no jQuery-style html setter, no dynamic-code evaluation (eval, the
Function constructor, or string-form timers), no server-side templating of data
into the page (the HTML is a static embedded `[]byte`), and no navigation/src
assignment from data. Every untrusted field traced (all 18 panel renderers, the 4
log modals, the run-detail modal, the SVG world graph, the SSE feed) reaches the
DOM as text via `el()`/`chip()`/`muted()`/`kv()`/`pre()` (all text-node), never as
markup and never into an attribute/URL context. The text-node-only design
structurally eliminates the inconsistent-escaping bug class. `webui.go` already
sets `nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`,
constant-time token compare, and serves JSON with `application/json`. The token is
never embedded in the page body.

The one gap was a missing Content-Security-Policy (LOW, defense-in-depth).

## The change
Added a per-response CSP nonce on the dashboard route. The page's single inline
`<script>` and single inline `<style>` now carry `nonce="__CSP_NONCE__"`;
`handleDashboard` mints a fresh 16-byte `crypto/rand` base64 nonce per request,
substitutes both placeholders, and sets:

    default-src 'none';
    script-src 'nonce-<n>'; style-src 'nonce-<n>';
    connect-src 'self'; img-src 'self' data:;
    base-uri 'none'; form-action 'none'; frame-ancestors 'none'

A nonce (not `'unsafe-inline'`) is used deliberately: it is the difference
between real and cosmetic mitigation. With a nonce, an injected script tag from
any *future* dynamic-HTML regression is refused by the browser because it cannot
carry the unpredictable per-response value; `'unsafe-inline'` would have allowed
exactly such an injected inline script. `default-src 'none'` blocks any injected
external resource load, and `connect-src 'self'` / `base-uri` / `form-action` /
`frame-ancestors 'none'` close exfiltration and DOM-clobbering/pivot avenues.

The page needs nothing a strict CSP breaks: verified there are no external
resources (the only `http://` is the SVG namespace string; `url(#wg-arrow)` is an
SVG fragment ref), no img/data/link/external-script/iframe loads, no inline `on*=`
event handlers (all events bound via `addEventListener`), and the only runtime
style write is `element.style.flexGrow` (CSSOM property — not governed by
`style-src`). So script/style nonces with no `'unsafe-inline'` cover everything.

## Verification
- **`kernel/webui/webui_test.go`**:
  - `TestDashboard_SetsCSPNonce`: the response carries a CSP with
    `default-src 'none'` and a `script-src 'nonce-…'`; the placeholder is fully
    substituted (no leftover `__CSP_NONCE__`); both the `<script>` and `<style>`
    tags carry the SAME nonce as the header.
  - `TestDashboard_NoncePerRequest`: two requests yield different nonces.
  - **Negative controls:** (1) leave the nonce buffer unfilled (constant nonce)
    → NoncePerRequest FAILs ("nonce was reused… AAAA=="); (2) drop the CSP header
    Set → SetsCSPNonce FAILs ("no Content-Security-Policy header"). Both restored.
- **Gate:** staged (LF) blobs gofmt-clean (working-copy `gofmt -l` warning is the
  CRLF artifact), `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2298** passing (was 2296; +2),
  `go test ./...` exit 0. CHANGELOG Security entry.

## Review status
The operator web dashboard is reviewed and sound: no XSS, and now CSP-hardened.
