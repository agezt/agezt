# M351 — Shared rune-safe truncation helper + kernel-side conversions

## Why
M349/M350 fixed three rune-unsafe truncations one at a time; a codebase grep then
showed the same `s[:n] + marker` byte-slice idiom duplicated across ~10 sites, some
already rune-safe (channel history) and some not. The root cause was the absence of
a shared helper — so the bug kept being re-introduced per package. M351 centralises
the idiom and converts the kernel-side offenders; M352 (next) does the `cmd/agt`
CLI sites.

## What
- **New `internal/strutil` package** with `Ellipsis(s, maxBytes, marker)`: returns
  `s` if it fits in `maxBytes`, else a prefix cut on a UTF-8 rune boundary
  (`utf8.RuneStart`, dropping a straddling rune whole) plus `marker`. The marker is
  a parameter so each call site keeps its existing style (`…` vs `...`). Six unit
  tests: under/exact/over cap, ASCII, rune-boundary split, all-multi-byte odd cap,
  non-positive max.
- **Kernel conversions** (byte-slice → `strutil.Ellipsis`, behaviour preserved):
  - `kernel/controlplane/status.go` — provider-fallback **reason** in `agt status`
    (`r[:160]` → rune-safe).
  - `kernel/planner/planner.go` — generated-plan node **snippet** (`s[:200]+"..."`).
  - `kernel/creds/{sso,sts,web_identity}.go` — AWS auth **error excerpts**
    (`excerpt[:512]+"..."`); these are JSON/XML error bodies that can be non-ASCII.
  - `kernel/channel/history.go` — `clip` (already rune-safe in-line) now delegates
    to the helper, so there is a single implementation and its local `unicode/utf8`
    import is dropped.

## Verification
- `go test ./internal/strutil/ ./kernel/{channel,planner,controlplane,creds}/` —
  all pass (incl. the existing channel-history clip tests, unchanged behaviour).
- `gofmt -l` clean on all edited files (`creds/creds.go` + `encrypt.go` carry
  pre-existing CRLF artifacts, untouched here); `go vet` clean; `GOOS=linux go
  build ./...` exit 0. Full suite **2083** passing (was 2078; +5), `go test ./...`
  exit 0. `go.mod` / `go.sum` unchanged (new package is in-module, no dependency).
  CHANGELOG updated.

## Scope notes
- Behaviour preserved exactly: same byte bound, same marker per site, only the cut
  is now rune-aware (identical for ASCII / under-cap). The redundant outer
  `if len > 512` guards in the creds files are left in place (harmless; `Ellipsis`
  re-checks).
- Remaining byte-slice truncations live in `cmd/agt` (run/pulse/check/plan-visualize
  intent + task displays) — converted next in M352. Hex/ULID prefix slices
  (`hash[:12]`, `id[:12]`) are intentionally left: hex/ULID are ASCII, never
  multi-byte.
