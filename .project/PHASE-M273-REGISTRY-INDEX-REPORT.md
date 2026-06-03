# M273 — `index.json` registry manifest for `agt skill export --all`

## Why
A directory of bundles is browsable locally (`agt skill registry <dir>` scans the
files), but a registry served over plain static HTTP cannot be listed — a remote
consumer has no way to enumerate the bundles. The standard fix is a manifest the
publisher writes and the host serves at a known path. This milestone makes
`agt skill export --all` emit that manifest (`index.json`), the foundation for a
remote registry adapter (the next slice).

## What
- **`cmd/agt/skill_registry.go`** — new types + const:
  - `registryIndex{Tool, FormatVersion, GeneratedUnixMS, Skills}` and
    `indexSkill{Name, Version, ID, Description, File}` — the manifest shape.
  - `registryIndexName = "index.json"`.
- **`cmd/agt/skill_export.go`** — `exportAllSkills` accumulates an `indexSkill`
  per written bundle and writes `index.json` into the registry directory after
  the bundles. The output notice now mentions the index.

## Files
- `cmd/agt/skill_registry.go` — `registryIndex` / `indexSkill` / `registryIndexName`
  (new).
- `cmd/agt/skill_export.go` — index accumulation + write (edited).
- `cmd/agt/skill_registry_test.go` — 2 tests (new): the directory scan ignores
  `index.json` (it is not a `*.skill.json`, so it never appears as a malformed
  bundle); the `registryIndex` JSON round-trips so a consumer can rely on its
  shape.

## Verification
- `go test ./cmd/agt/ -run 'TestScanSkillRegistry_IgnoresIndex|TestRegistryIndex_RoundTrips'`
  — green; full suite **1873 → 1875** (+2), 68 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on all touched files; `go vet ./cmd/agt/`
  clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged.
- **Live-proven**: `agt skill export --all --dir <dir>` for 2 skills wrote the
  two bundle files plus an `index.json` listing both with the correct
  name/version/id/description and the matching `file` for each.

## Scope notes
- Pure additive: the local `registry` view is unchanged (the scan ignores
  `index.json`); the manifest exists for remote consumption.
- Sets up the next marketplace slice — **`agt skill registry <url>`**: fetch
  `<url>/index.json` (httptest-provable, no real network), list the entries, and
  `--install <name>` fetches + verifies + installs the named bundle — reusing the
  content-addressed verification the local path already has.
