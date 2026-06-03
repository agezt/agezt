# M224 — Quote-aware `AGEZT_PLUGINS` paths (spaces support)

## Why
M223's report flagged a pre-existing limitation: `ParsePluginSpec` split the
path on whitespace (`strings.Fields`), so a plugin path containing a space could
not be expressed. On Windows this blocks the single most common install
location — anything under `C:/Program Files/...` has a space, regardless of
slash direction — so an operator literally could not point `AGEZT_PLUGINS` at a
plugin installed there. Found while building M223; fixed in the same parser.

## What
- **`kernel/plugin/pluginspec.go`** — a new `splitFields` tokenizer replaces
  `strings.Fields` for the path/args portion. It splits on unquoted whitespace
  but honours single and double quotes: the quote characters are removed and the
  run between matching quotes is taken literally, so a quoted field keeps its
  spaces. Quoting may start/stop mid-field. An unterminated quote is an error.
  An empty quoted path (`prefix=""`) is still rejected as an empty path.

  ```
  AGEZT_PLUGINS='win="C:/Program Files/agezt-tool.exe" --verbose'
  → Path="C:/Program Files/agezt-tool.exe", Args=["--verbose"]
  ```

**Backward compatible.** Unquoted input tokenises exactly as `strings.Fields`
did (`a=/bin/x -v --depth 2` → path `/bin/x`, args `-v --depth 2`), so every
existing config behaves identically. Quoting is the only new capability.

## Files
- `kernel/plugin/pluginspec.go` — `splitFields` + wired into `ParsePluginSpec`;
  empty-path check tightened to also reject a single empty field; doc updated
  (edited).
- `kernel/plugin/pluginspec_test.go` — 5 new tests: quoted spaced path (the
  Windows case) + quoted arg, single-quoted path with spaces and a quoted arg,
  unquoted-still-splits (back-compat), unterminated quote, empty quoted path.

## Verification
- `go test ./kernel/plugin/` — green; full suite **1727 → 1732** (+5),
  66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./kernel/plugin/` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- **Live proof:** ran the daemon with a quoted spaced path
  `AGEZT_PLUGINS='demo="C:/No Such/my app/bin.exe"'` — the startup warning shows
  the path **with both spaces intact** (`plugin "demo" (C:/No Such/my app/bin.exe)
  failed to start`), i.e. it was passed to fork/exec whole. The unquoted control
  `demo=C:/No Such/my app/bin.exe` truncates at the first space (`C:/No`),
  confirming the quoting is what preserves the path.

## Scope notes
- Commas still can't appear in a path (the entry separator). A comma-bearing
  path would need a different entry-delimiter scheme — a separate milestone, and
  far rarer than spaces.
- No escape character (`\"` inside a quote) — quotes are the whole mechanism.
  Adequate for filesystem paths; revisit only if a real need appears.
