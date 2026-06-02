# M182 — Cap on advertised tool count

## Why
The plugin-host review (finding M2) noted the initialize result's `Tools` list is
consumed verbatim:

```go
p.tools = initResult.Tools   // no count/size limit
```

`Tools(prefix)` later materializes one registry map entry + a `remoteTool` wrapper per
def. The M177 stdout frame bound caps the *raw bytes* of the initialize reply at
16 MiB, but a minimal tool def (`{"name":"t"}`) is ~12 bytes, so ~1M defs still fit —
a hostile or buggy plugin can drive a large allocation at registration. With no
allowlist set (the opt-in default), nothing bounded the count.

## What
- **`Config.MaxAdvertisedTools`** (default `DefaultMaxAdvertisedTools = 256`) — the cap,
  defaulted in `Spawn`. Real plugins advertise a handful to a few dozen tools; 256 is
  generous while bounding the blast radius.
- **`capAdvertisedTools(advertised, max)`** — returns a wrapped `ErrTooManyTools`
  (naming the count) when exceeded.
- Called in **`Spawn`** and **`respawn`** right after the initialize result is
  unmarshalled and before the allowlist check / registration, so an over-cap plugin
  fails to spawn (and a reload to an over-cap binary rolls back, like the pin/allowlist
  checks) before any map is built.

## Tests
- `kernel/plugin/toolcap_test.go` (white-box) — `capAdvertisedTools` under, at, and
  over the cap (over → `errors.Is(ErrTooManyTools)`).
- `kernel/plugin/toolcap_integration_test.go` (live) — the echo fixture advertises 4
  tools; `Spawn` with `MaxAdvertisedTools: 2` fails with `ErrTooManyTools`. Reuses the
  shared echo binary unchanged (no count/allowlist assertions disturbed).

## Verification
- `go test ./...` — 1574 passing, 0 failing.
- `go vet ./kernel/plugin/` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/plugin/host.go` — `DefaultMaxAdvertisedTools`, `ErrTooManyTools`,
  `Config.MaxAdvertisedTools` (+ default in `Spawn`), `capAdvertisedTools`, calls in
  `Spawn` and `respawn`.
- `kernel/plugin/toolcap_test.go`, `kernel/plugin/toolcap_integration_test.go` — new.

## Follow-ups (same review, remaining)
M3 (`Kill` nil-`Process` guard), M4 (process-group kill for orphaned grandchildren).
The CRITICAL (C1) and all three HIGH (H2/H3/H4) plus two of the MEDIUMs (M1/M2) are now
fixed (M177–M182); M3/M4 are the last queued items.
