# PHASE M585 — Plugin registry / marketplace (`agt plugin registry`)

**Status:** DONE — local, gated (unit + full end-to-end daemon smoke green),
ready for branch/PR. **Owner picked the plugin-catalog gap via AskUserQuestion**
after I found the remote *skill* registry already exists.

## Context — the marketplace was half-built

Investigating "Marketplace" surfaced that the remote **skill** registry is already
complete: `agt skill registry <url> --install <name>` (content-address-verified),
`agt skill export --all` (publish). The genuine gap was **plugins** (native
binaries). M585 fills it, mirroring the skill-registry shape, **without** weakening
the plugin security model.

## What shipped

`agt plugin registry <dir|url> [--install <name>] [--dir <installdir>] [--json]`
(`cmd/agt/plugin_registry.go`):
- **Browse:** reads the registry's `index.json` (a `pluginIndex` of plugins, each
  with per-platform `binaries` pinned by BLAKE3-256). Lists name/version/desc/
  platforms; flags when there's no build for the running host.
- **Install:** resolves the name → one plugin, selects the binary for
  `runtime.GOOS/GOARCH`, downloads it (bounded — 256 MiB cap — from a directory or
  an `http(s)` URL), **verifies BLAKE3 against the index pin in memory BEFORE
  writing** (mismatch → refuse, nothing lands on disk), stages it under
  `<paths.BaseDir>/plugins` (or `--dir`), and prints the exact `AGEZT_PLUGINS` +
  `AGEZT_PLUGIN_PINS` lines to enable it.
- **Operator authority preserved:** install = "fetch + verify + stage". It never
  edits the daemon env or loads anything — the daemon runs a plugin only when the
  operator wires it in. ("Autonomous, under your authority.")
- **Untrusted-index hygiene:** binary/index filenames validated (no `/`, `\`,
  `..`); a malformed pin in the index is rejected before any download.

Shared plumbing added to `kernel/plugin/pin.go`: `HashBytes([]byte) string` (the
in-memory sibling of `HashFile`, to verify a download before it touches disk) and
`LooksLikePin(string) bool` (exported pin-format check). `net/http` only — no new
dependency.

## Tests + smoke (all green)

- **7 unit tests** (`plugin_registry_test.go`): dir list; dir install verifies +
  stages + prints the exact env lines; **tampered pin refused AND not written**;
  missing name refused; no-build-for-host refused; unsafe filename (`../escape`)
  refused; **remote (httptest) list + install** verifies the pin.
- **Full end-to-end daemon smoke:** built the SDK `greet` example plugin, published
  it into a directory registry (real binary + its real BLAKE3), `agt plugin
  registry <dir> --install greet --dir …` → verified + staged + printed env; booted
  the daemon with those exact `AGEZT_PLUGINS` + `AGEZT_PLUGIN_PINS` → `agt plugin
  list` showed **`greet [pinned]`, 3 tools, no pin mismatch**. Proves the whole
  catalog → install → verify → enable → daemon-loads-pinned loop, and that the pin
  the registry emits is exactly what the daemon enforces.
- gofmt clean on staged LF blobs; full Go suite green; `go build ./...` clean;
  `go.mod` unchanged.

## Note (cross-platform, documented)

A Windows plugin binary must be named `*.exe` in the registry — the daemon execs by
path and Windows needs the extension. The installer stages the file verbatim (the
pin is content-addressed, name-independent), so this is a publisher-side naming
convention (`tool-windows-amd64.exe`), surfaced here so it's not a surprise. The
smoke initially reproduced the "executable file not found" failure with a no-`.exe`
name, then passed once named correctly.

## Backlog after M585

Remaining DEFERRED items genuinely need an owner steer (external services /
secrets): Tunnels (external relay → no offline smoke), SDK publish
(PyPI/npm/crates.io → registry secrets). `agt migrate` = no real migration → skip.
The marketplace (skills + plugins) is now complete.
