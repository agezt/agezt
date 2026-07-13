# WIP: `cmd/agezt/boot_tools.go` — Restore env-driven tool wiring

**Status:** Active WIP — not yet merged
**Date:** 2026-07-12
**Scope:** Expand `boot_tools.go` to match the original `cmd/agezt/main.go:buildTools` (pre-refactor) behavior, then test, then verify env-driven tool registrations re-fire.

---

## What this restore does

The earlier `gofmt -w .` + import cleanup swept four large helper bodies out of
`cmd/agezt/main.go` and parked them as stubs in `cmd/agezt/boot_tools.go`. The
stubs silently dropped a lot of env-driven behavior. This WIP restores the full
signatures and per-tool env wiring, with the goal of matching the original
implementation **byte-for-byte where it matters**.

### Behavior restored

| Tool | Env knobs (restored) | WIP status |
|---|---|---|
| `shell` | always on, routes through warden | ✅ kept (same as original) |
| `file` | always on, scoped to workspace | ✅ kept |
| `config` | always on, baseDir-bound | ✅ kept |
| `http` | `AGEZT_HTTP_ALLOWED_HOSTS`, `AGEZT_HTTP_ALLOW_ALL`, `AGEZT_HTTP_ALLOW_LOOPBACK`, `AGEZT_HTTP_ALLOW_PRIVATE` | ✅ restored |
| `browser.read` | `AGEZT_BROWSER_ALLOWED_HOSTS`, `AGEZT_BROWSER_ALLOW_{ALL,LOOPBACK,PRIVATE}` | ✅ restored |
| `browser.action` (verbs) | `AGEZT_BROWSER_ACTIONS=1` + 12 sub-flags incl. session dir, user-profile, remote-CDP | ✅ restored |
| `web_search`, `fetch`, `artifacts`, `db` | always on | ✅ kept |
| `council`, `conductor`, `research` | always on | ✅ kept |
| `coding` | `AGEZT_CODING_CMD` | ✅ restored |
| `code_exec` | `AGEZT_SANDBOX`, `AGEZT_SANDBOX_NO_NET` | ✅ restored |
| `acp_agent` | `AGEZT_ACP_AGENT_CMD` | ✅ restored |
| `homeassistant` | `AGEZT_HOMEASSISTANT_{URL,TOKEN,TOOL_READ,TOOL_SERVICES,TOOL_ALLOW_ALL_SERVICES}` | ✅ restored |
| `remote_run` (peer mesh) | `AGEZT_PEERS`, `AGEZT_TENANT_PEERS` | ✅ restored |
| External plugins | `AGEZT_PLUGINS`, `AGEZT_PLUGIN_PINS`, `AGEZT_PLUGIN_TOOLS` | ✅ restored |

### Out of scope / intentionally NOT preserved

- `br.CookiesEnabled = os.Getenv(...)` — the `*browser.Tool` type does NOT expose a `CookiesEnabled` field. The original file may have done this via a different setter or the field was removed during a later refactor. Investigation deferred. Coookies are still available via the standalone `browser.cookies` tool when `AGEZT_BROWSER_COOKIES=1`.
- Any per-daycap / quota wiring (it never lived in buildTools — it lives on the kernel side).

## Verification

| Command | Result |
|---|---|
| `go vet ./...` | **PASS** |
| `go build ./...` | **PASS** |
| `go test -run TestBuildToolsBrowserActionOptIn ./cmd/agezt` | **PASS** |
| `go test ./cmd/agezt/...` | **PASS** (full package) |
| `gofmt -l .` | empty (clean) |

## Files touched

- `cmd/agezt/boot_tools.go` — expanded from 80 → 536 lines, signature preserved, calls reset to original logic.
- `cmd/agezt/main.go` — imports trimmed earlier in this session (8 unused imports removed), four stub-references (`runUpdate`, `startUpdateChecker`, `buildTools`, `councilSeatName`) left pointing at `boot_*.go` files as designed.

## Open follow-ups (NOT in this WIP)

1. **Move `splitNonEmpty` to `internal/strutil` (already there as `strutil.SplitNonEmpty`).** Currently main.go holds its own copy AND this file uses it via the same-package symbol. Lifting it to one shared location would avoid the dual definition.
2. **Reconcile the `splitNonEmpty` duplicated body in main.go:4192.** When this WIP is finalized, main.go's copy can either be removed in favour of `strutil.SplitNonEmpty` (already imported via `internal/strutil`) or kept with a deprecation comment pointing at the strutil version.
3. **Verify `br.CookiesEnabled` history.** Run `git log -p -- cmd/agezt/main.go | grep -i 'CookiesEnabled' 'BROWSER_COOKIES'` to confirm whether the field was ever on `*browser.Tool`. If it was, the setter must be rediscovered (e.g., via a method) before cookies-via-browser.read can be wired.
4. **Pull the BR/wiki updates into `docs/OPERATIONS.md`.** Currently `OPERATIONS.md` may still document `AGEZT_BROWSER_COOKIES` as a `browser.read` flag; the correct path is now `browser.cookies`.
