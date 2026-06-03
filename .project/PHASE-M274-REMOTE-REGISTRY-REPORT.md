# M274 — `agt skill registry <url>`: remote registry browse + install

## Why
M273 made `agt skill export --all` write an `index.json` manifest so a registry
can be served over plain static HTTP. This milestone is the consumer side: point
`agt skill registry` at a URL to browse and install skills published by another
node — the capability that turns the local marketplace into a real, shareable
one. It is fully offline-testable (httptest) and proven against a real static
HTTP server.

## What
- **`cmd/agt/skill_registry_remote.go`** (new):
  - `isHTTPURL(s)` — routes an http(s) arg to the remote path.
  - `fetchRegistryFile(base, relPath)` — bounded GET (20s timeout, 8 MiB cap).
    A relative file name from the (untrusted) index is **validated to be a plain
    filename** — no `/`, `\`, or `..` — before being joined, so a malicious index
    cannot redirect the download elsewhere.
  - `remoteRegistry(url, install, …)` — fetches `<url>/index.json`, then either
    lists the entries (with per-skill `--install` hints) or installs one.
  - `remoteInstall(url, idx, name, …)` — resolves the name in the index to one
    entry, fetches its bundle, and installs through the shared
    `importSkillBundleBytes` (so content-address verification still applies; a
    tampered bundle is rejected even when fetched remotely).
- **`cmd/agt/skill_import.go`** — extracted `importSkillBundleBytes(data, …)` from
  `cmdSkillImport` so the file path and the remote path share one verify-then-
  install core.
- **`cmd/agt/skill_registry.go`** — `cmdSkillRegistry` routes a URL arg to
  `remoteRegistry`; help/usage updated to `<dir|url>`.

## Files
- `cmd/agt/skill_registry_remote.go` — remote fetch/list/install (new).
- `cmd/agt/skill_import.go` — `importSkillBundleBytes` extraction (edited).
- `cmd/agt/skill_registry.go` — URL routing + help (edited).
- `cmd/agt/skill_registry_remote_test.go` — 5 tests (new): URL detection; unsafe
  index file names refused before fetch; list from a served index (httptest);
  a 404 index is a clean error; install of an absent name errors before
  fetching/dialing.

## Verification
- `go test ./cmd/agt/ -run 'IsHTTPURL|FetchRegistryFile|RemoteRegistry|RemoteInstall'`
  — green; full suite **1875 → 1880** (+5), 68 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on all touched files; `go vet ./cmd/agt/`
  clean.
- `GOOS=linux go build -buildvcs=false ./...` clean; `go.mod` / `go.sum`
  unchanged (still stdlib `net/http` only).
- **Live-proven against a real static HTTP server**: published 2 skills with
  `export --all`, served the directory with `python3 -m http.server`, then from a
  fresh home `agt skill registry http://localhost:PORT` listed both skills (from
  the fetched index) and `--install deploy` fetched + verified + installed it as
  a fresh draft (confirmed in `skill list`).

## Scope notes
- The skill marketplace is now complete remote-capable: **export --all (publish)
  → host the directory on any static server → registry <url> (discover) →
  registry <url> --install <name> (install)**, every step content-address
  verified.
- No new dependency (stdlib `net/http`). Fetches are operator-initiated (not
  agent-driven) and bounded; the index file-name validation blocks download
  redirection, and the content address blocks tampering.
- A natural follow-on, if wanted: bundle signing for cross-org *authenticity*
  (the content address proves integrity, not who published it).
