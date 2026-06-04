# M359 — /metrics: sanitize metric names to valid Prometheus identifiers

## Why
Priority-A robustness on the observability surface. The REST `/metrics` endpoint
writes a Prometheus text exposition by emitting `agezt_<name> <value>` per metric.
Prometheus metric names must match `[a-zA-Z_:][a-zA-Z0-9_:]*`; a name containing a
`.`, `-`, or space produces a line the Prometheus parser rejects — and because one
malformed line aborts parsing of the whole exposition, a single bad metric would
**silently drop every other metric** from the scrape (the operator loses all
observability, with no error).

Today's metric names (cmd/agezt/main.go) are all valid identifiers, so there is no
live bug — but the handler is the integration point that should be robust to a
future metric definition (e.g. a per-model `model.latency` gauge) rather than
trusting every caller to hand-validate names. The fix makes a bad name degrade to
a still-scrapeable identifier instead of breaking the endpoint wholesale.

## What
Production hardening + lock-in test.
- **`kernel/restapi/restapi.go`** — new `promName(s)` coerces a name to the
  Prometheus grammar: any out-of-grammar byte becomes `_`, and a leading digit is
  prefixed with `_`. `handleMetrics` now emits `promName("agezt_" + m.Name)`.
- **`kernel/restapi/metrics_test.go`** — `TestPromName_SanitizesToValidIdentifier`:
  valid names pass through unchanged (`agezt_up`, `weird:name` — `:` is legal),
  `.`/`-`/space become `_`, empty → `_`, and a leading digit is prefixed; every
  output is asserted against the Prometheus identifier regex.

## Verification
- `go test ./kernel/restapi -run 'PromName|Metrics' -v` — all pass (the existing
  Prometheus-format and no-source tests still pass).
- `gofmt -l` clean; `go vet ./kernel/restapi/` clean; `GOOS=linux go build ./...`
  exit 0. Full suite **2095** passing (was 2094; +1), `go test ./...` exit 0.
  `go.mod`/`go.sum` unchanged. CHANGELOG updated (Reliability).

## Notes
- The test paid for itself immediately: the first implementation of `promName`
  recursed on the same string for a leading-digit input, which the test caught as a
  stack overflow before the change could ship. Fixed to prefix-without-recursion.
- HELP-text escaping (`\` / newline) is a separate, lower-risk concern (HELP lines
  are daemon-controlled single-line strings) — noted, not bundled here.
