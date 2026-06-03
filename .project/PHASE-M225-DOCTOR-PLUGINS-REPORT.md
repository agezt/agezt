# M225 — `agt doctor` plugin env-spec pre-flight

## Why
M223 made the daemon **hard-fail** on a malformed `AGEZT_PLUGINS` (and M216 did
the same for `AGEZT_PLUGIN_PINS` / `AGEZT_PLUGIN_TOOLS`). That's the right
startup posture, but it has a sharp edge: an operator who fat-fingers the spec
discovers it only when they restart and the daemon **refuses to come back up** —
the worst possible moment. `agt doctor` is the project's "catch config before it
bites you" surface (it already pre-flights `AGEZT_PEERS`, `AGEZT_MESH_MAX_HOPS`,
netguard, rate-limit, etc.), and now that the plugin specs are parsed by
extracted, importable functions (M216, M223), doctor can validate them too —
turning a failed restart into a caught typo.

## What
A new `checkPlugins()` doctor check, registered alongside `checkChannels` /
`checkMesh`. It reads the operator's environment (no spawn, no running daemon
needed for the logic) and runs the real parsers:

- **no `AGEZT_PLUGINS`** → informational OK ("no external plugins configured").
- **malformed `AGEZT_PLUGINS` / `AGEZT_PLUGIN_PINS` / `AGEZT_PLUGIN_TOOLS`** →
  **FAIL**, naming the offending env var and the parse error, with the hint that
  the daemon will refuse to start. (FAIL, not WARN: this is startup-blocking,
  unlike the mesh checks where the daemon merely degrades.)
- **stale pin/tool entry** (a prefix matching no configured plugin) → **WARN**,
  naming the entries (`pin:<x>`, `tools:<x>`) — the same staleness the daemon
  warns about at boot.
- **clean** → OK with the plugin count plus `N pinned` / `N allow-listed`
  annotations.

## Files
- `cmd/agt/doctor.go` — `checkPlugins()` + registration + the `kernel/plugin`
  import (edited).
- `cmd/agt/doctor_plugins_test.go` — 7 tests (new): not-configured, valid,
  quoted spaced path (ties in M224), the malformed set (missing `=`, dup prefix,
  bad pin, empty tool list) all FAIL with a hint, stale-pin WARN, stale-tool
  WARN, and valid-with-pins-and-tools OK with annotations.

## Verification
- `go test ./cmd/agt/` — green; full suite **1732 → 1739** (+7), 66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./cmd/agt/` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- **Live proof (end-to-end, against a running daemon):**
  - `AGEZT_PLUGINS="a=/x,a=/y" agt doctor` →
    `[FAIL] plugins : AGEZT_PLUGINS is malformed: plugin: prefix "a" is defined more than once`
    with the "daemon will refuse to start" hint.
  - valid + pin + tools →
    `[OK] plugins : 1 plugin(s) configured, 1 pinned, 1 allow-listed`.
  - stale pin →
    `[WARN] plugins : 1 plugin(s) configured, but these entries reference no plugin prefix: pin:ghost`.

## Scope notes
- doctor validates spec *syntax* and prefix cross-references; it does not stat
  the plugin binaries or hash them (the daemon does that at spawn, and the
  binary may legitimately not exist yet on the doctor host). A binary-existence
  check could be a later refinement.
