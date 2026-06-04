# M356 — Lock in the dashboard's XSS-safe-by-construction invariant

## Why
Priority-A (+C) — a lock-in test for a security property that was previously
maintained only by developer discipline and a comment. The web dashboard renders
server-supplied text into the DOM (tool output, run intents, provider-fallback
reasons, schedule descriptions). It is XSS-safe **by construction**: the renderers
use `textContent` and an `el()` createElement helper exclusively — there is no
HTML-injection sink (raw-HTML assignment, adjacent-HTML insertion, or document
stream write) anywhere. Verified: the only two textual mentions of the raw-HTML
property name are comments documenting the policy.

That safety, however, lived only in a convention. A future edit that assigned a
server string as raw HTML to "render some markup" would silently introduce a
stored-XSS vector (attacker-controlled tool output / channel message rendered as
live markup in the operator's browser) and ship green. This milestone converts the
convention into an enforced invariant.

Context: a strict Content-Security-Policy was the originally-noted future-hardening
item, but the dashboard uses inline scripts (CSP would require a nonce/externalise
refactor) **and** is already XSS-safe by construction with X-Frame-Options +
Referrer-Policy already in place (M355) — so CSP is low-value/high-risk here. The
higher-value, lower-risk move is to lock the no-injection-sink invariant directly.

## What
Test-only. **`kernel/webui/webui_test.go`** — `TestDashboard_NoUnsafeDOMSinks`
scans the embedded `dashboardHTML` and fails if it contains any of the three known
HTML-injection sinks, or any raw-HTML-property assignment to anything other than a
node-clearing empty string (`""` / `''`). It passes today and trips the moment a
sink is added.

## Verification
- `go test ./kernel/webui -run NoUnsafeDOMSinks -v` — passes (no sinks present).
- `gofmt -l` clean; `go vet ./kernel/webui/` clean; `GOOS=linux go build ./...`
  exit 0. Full suite **2090** passing (was 2089; +1), `go test ./...` exit 0.
  `go.mod`/`go.sum` unchanged. No CHANGELOG (test-only; no behaviour change).

## Scope notes
- Resolves the "CSP refactor" future-hardening note: verified the dashboard has no
  dynamic-markup sink, so CSP would be defense-in-depth at real risk of breaking the
  inline-script dashboard. The XSS-safe property is now test-enforced instead, which
  is the load-bearing guarantee CSP would have backstopped.
- The regex is intentionally permissive about clearing a node (assigning an empty
  string) and strict about everything else.
