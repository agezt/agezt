# M223 — Extract `plugin.ParsePluginSpec`, reject duplicate prefix

## Why
A known defect carried in `next.md`: the `AGEZT_PLUGINS` boot loop parsed its
spec inline in `cmd/agezt/main.go`, and a **duplicate prefix** slipped through.
`AGEZT_PLUGINS="search=/a,search=/b"` spawned *both* processes; the first
registered its tools under `search.`, and the second's identical-named tools
then lost the `out[name]` conflict check — producing a misleading
`"conflicts with existing tool — keeping in-process version"` warning (it was
another *plugin*, not an in-process tool) while a second subprocess ran for
nothing. Because the parse was inline, it was also untestable.

This is the same silent-misconfiguration class closed for the sibling specs in
M215 (`AGEZT_PEERS` dup name), M216 (`AGEZT_PLUGIN_PINS` / `AGEZT_PLUGIN_TOOLS`
dup prefix), M217–M218 (`AGEZT_WEBHOOKS`). `AGEZT_PLUGINS` was the last spec in
the plugin family still parsed inline and still tolerant of a dup.

## What
- **`kernel/plugin/pluginspec.go`** (new) — `ParsePluginSpec(spec string)
  ([]PluginSpecEntry, error)` and the `PluginSpecEntry{Prefix, Path, Args}`
  type. Semantics mirror `ParsePinSpec` / `ParseToolAllowlistSpec` exactly:
  - entry missing `=` → hard error;
  - empty prefix → hard error;
  - empty path → hard error;
  - **duplicate prefix → hard error** (the fix);
  - empty / whitespace-only spec → nil slice, nil error;
  - whitespace around tokens trimmed; the path is split on spaces into
    executable + args.
- **`cmd/agezt/main.go`** — the boot loop now calls `ParsePluginSpec` up front
  (hard-failing startup on a bad spec, consistent with the pin/allowlist specs
  parsed immediately above) and ranges over the parsed entries. The
  spawn/conflict/manifest logic is unchanged.

**Behaviour change (intentional):** malformed entries that were previously a
`WARNING` + skip are now hard startup errors. This aligns `AGEZT_PLUGINS` with
its three sibling parsers (all of which hard-fail malformed input) and with the
project's "fast feedback on config" stance — a typo silently dropping a
configured plugin is worse than a clear startup failure.

## Files
- `kernel/plugin/pluginspec.go` — the parser (new).
- `kernel/plugin/pluginspec_test.go` — 6 tests (new): valid multi-entry with
  args, whitespace tolerance, empty/trailing-comma cases, the error set
  (missing `=`, empty prefix, empty/whitespace path, dup-in-many), the
  duplicate-prefix rejection (incl. whitespace-disguised dup), and the no-arg
  empty-Args contract.
- `cmd/agezt/main.go` — boot loop refactored to use the parser (edited).

## Verification
- `go test ./kernel/plugin/` — green; full suite **1721 → 1727** (+6),
  66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./kernel/plugin/ ./cmd/agezt/`
  clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- **Live proof:** ran the daemon with `AGEZT_PLUGINS="search=/a,search=/b"` — it
  hard-fails at startup, before binding, with
  `agezt: AGEZT_PLUGINS: plugin: prefix "search" is defined more than once`. A
  control run with a single prefix does not hit that path (it proceeds to spawn
  and warns softly on a missing binary, as designed).

## Scope notes
- The path is still split on spaces (`strings.Fields`), so a plugin path
  containing a space can't be expressed in `AGEZT_PLUGINS` — pre-existing
  behaviour, unchanged here; a quoting scheme would be a separate milestone.
