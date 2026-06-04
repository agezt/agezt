# M360 — /metrics: escape HELP text (completes exposition-format robustness)

## Why
Completes the `/metrics` exposition-format robustness started in M359 (which
sanitised metric *names*). The endpoint emits a `# HELP <name> <text>` record per
metric. The Prometheus exposition format requires HELP text to escape backslash
(`\` → `\\`) and newline (`\n` → `\n`); a raw newline in HELP ends the line
mid-description, and the leftover tail becomes a malformed line that — like a bad
name — breaks parsing of the whole exposition, silently dropping every metric.

Today's HELP strings are all single-line constants, so there is no live bug. As
with the name sanitisation, the value is making the handler robust to a future
metric definition (a multi-line or backslash-containing description) rather than
trusting every caller to pre-escape.

## What
Production hardening + lock-in test.
- **`kernel/restapi/restapi.go`** — new `promHelp(s)` escapes `\` then `\n` (in
  that order, so the backslash added for a newline isn't itself doubled).
  `handleMetrics` now emits `promHelp(m.Help)`.
- **`kernel/restapi/metrics_test.go`** — `TestPromHelp_EscapesBackslashAndNewline`:
  plain text unchanged; a backslash is doubled; a newline becomes `\n`; a string
  with both is handled; and the escaped output is asserted to contain no raw
  newline.

## Verification
- `go test ./kernel/restapi -run 'PromHelp|PromName|Metrics' -v` — all pass.
- `gofmt -l` clean; `go vet ./kernel/restapi/` clean; `GOOS=linux go build ./...`
  exit 0. Full suite **2096** passing (was 2095; +1), `go test ./...` exit 0.
  `go.mod`/`go.sum` unchanged. CHANGELOG updated (extends the M359 Reliability
  entry).

## Scope notes
- With M359 (names) + M360 (HELP), the `/metrics` text exposition is now robust to
  any metric definition the daemon could hand it: a bad name or description
  degrades to valid Prometheus output instead of breaking the entire scrape.
- Metric *values* were already safe — `strconv.FormatFloat` renders NaN/`+Inf`/
  `-Inf` as the tokens Prometheus accepts, and every rate computation in the
  control plane guards its divisor (`0.0` default + `if denom > 0`), so no NaN/Inf
  reaches the wire from a 0/0 in the first place (verified this session).
