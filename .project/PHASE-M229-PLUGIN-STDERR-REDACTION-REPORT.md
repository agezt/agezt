# M229 — Redact plugin stderr before logging it

## Why
Found while auditing redaction call sites for M228. Secret redaction is applied
centrally on the **bus** — every journaled event has its payload and tags
scrubbed (`kernel/bus/bus.go`). But that covers the *journal*, not every egress.
A third-party plugin's **stderr** is captured by the plugin host and written
straight to the daemon's log via the per-plugin `Logger`
(`[plugin:<name>] <line>`) — a path that never went through any redactor. A
plugin that printed a secret to stderr (its own API key in an error message, a
token in a debug line) leaked it in the clear into the operator's logs.

Plugin stderr is *untrusted* output (the plugin is third-party), which makes it
exactly the kind of thing that should be scrubbed before it lands in a log.

## What
- **`cmd/agezt/main.go`** — `buildTools` now creates one pattern-based redactor
  (`redact.New()`) for the plugin loop and routes every plugin stderr line
  through a new `pluginLogLine(r, prefix, line)` helper before writing it. The
  `[plugin:<name>]` prefix is preserved so the operator still sees which plugin
  spoke.

Pattern-only redaction (no configured literals) is the correct fit: a plugin
leaks **its own** secrets, which the daemon doesn't hold as literals, but whose
formats (`sk-…`, Telegram, Groq, the M228 additions, …) the built-in detectors
catch. Using `redact.New()` locally also avoids reordering daemon init (the
literal-bearing bus redactor is built later, after `buildTools` runs).

## Files
- `cmd/agezt/main.go` — `pluginLogLine` helper + a per-loop `redact.New()` +
  the `Logger` now redacts (edited).
- `cmd/agezt/plugin_log_test.go` — 2 tests (new): a line carrying an `sk-…` /
  `gsk_…` / Telegram secret is scrubbed to the placeholder with the prefix
  intact; an ordinary line is passed through unchanged.

## Verification
- `go test ./cmd/agezt/` — green; full suite **1751 → 1753** (+2), 66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./cmd/agezt/` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- **Live proof (gold standard, end-to-end):** built a throwaway plugin that
  prints `using api key sk-LIVEPROOF…` to stderr at startup, configured it via
  `AGEZT_PLUGINS`, and started the daemon. The daemon log shows
  `[plugin:leak] startup: using api key [REDACTED] now`, and the raw secret
  appears **0 times** in the entire log. Before this change that line would have
  carried the key verbatim.

## Scope notes
- This covers the plugin `Logger` path specifically. Other daemon stderr (its
  own banners/warnings) is first-party and built from known-safe values; a
  blanket redacting stderr writer would be a larger, separate change with its own
  line-buffering trade-offs.
