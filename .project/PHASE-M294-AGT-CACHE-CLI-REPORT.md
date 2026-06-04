# M294 — `agt cache` CLI

## Why
M293 added the `cache_stats` aggregate and surfaced it in the Web UI Cache panel,
but the CLI had no equivalent — unlike `agt provider stats` / `agt tool stats`,
which exist on both surfaces. This closes the CLI↔Web-UI parity gap for the
prompt-cache savings view.

## What
- **`cmd/agt/cache.go`** (new): `cmdCache` implements
  `agt cache [--since <dur>] [--tenant <id>] [--json]`. It proxies
  `CmdCacheStats` and renders the savings:

  ```
  prompt cache (over N priced call(s)[ in the last <dur>]):

    saved        : $X.XXXX
    cache reads  : N tok
    cache writes : N tok
  ```

  Zero priced calls → a clear one-liner; `--json` emits the raw payload.
  Mirrors the `cmdProviderStats` arg-parsing/rendering conventions
  (`--since`/`--since=`, `--tenant`, `fmtUSD`, `intOfStatus`).
- **`cmd/agt/main.go`**: `case "cache"` dispatch.

## Files
- `cmd/agt/cache.go` (new), `cmd/agt/main.go` (dispatch) (edited).
- `cmd/agt/cache_test.go` (new) `TestCmdCache_ArgParsing` — the pre-dial branches
  (`--help` → 0 + usage; bad/missing `--since` and unexpected args → 2).

## Verification
- Full suite **1905**, 68 packages, `go test ./...` exit 0; `go vet ./cmd/agt/`
  clean; `gofmt -l` clean on the new files + `main.go`; `GOOS=linux` build clean;
  `go.mod` / `go.sum` unchanged.
- **Live-proven end-to-end**: `AGEZT_DEMO_CACHED=1` + one run →
  `agt cache` printed `prompt cache (over 1 priced call(s)): saved $0.0024 /
  cache reads 9000 tok / cache writes 500 tok`; `agt cache --json` emitted
  `{cached_input_tokens:9000, cache_write_input_tokens:500, saved_microcents:
  2392500, calls:1, window_ms:0}`.

## Scope notes
- Pure CLI over the existing `CmdCacheStats`; no kernel/control-plane change, no
  new dependency. Tenant-scoped through the same `withTenant`/`extractTenantFlag`
  helpers as the other observability commands (and `cache_stats` is already in the
  tenant read-only allowlist from M293).
- Completes the prompt-cache surface: aggregate (M293) on both CLI (`agt cache`)
  and Web UI (Cache panel). The cost arc (M289–M294) is now complete, visible, and
  reachable from both surfaces.
