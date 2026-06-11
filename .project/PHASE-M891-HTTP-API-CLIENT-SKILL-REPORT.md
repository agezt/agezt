# PHASE M891 — Built-in http-api-client skill bundle

**Status:** shipped
**Milestone:** M891 (this session's range is M889–M899; concurrent session holds
M880–M888 + M900–M909).
**Theme:** Backlog **#34** — an eleventh built-in skill bundle: the write-capable
complement to `fetch`/web-research — call REST/JSON APIs with any method, auth,
and bodies.

## What shipped

A built-in `http-api-client` bundle, seeded active at startup by the existing
`plugins/builtinskills` seeder (`builtinBundles` + `go:embed` only):

- `SKILL.md` — the fetch-vs-this distinction (read a page → fetch/web-research;
  call a JSON API → this), the spec, secrets guidance, and the "non-2xx returns
  the error body rather than throwing" contract.
- `scripts/setup.sh` — `pip install requests`.
- `scripts/api.py` — one JSON-spec helper over `requests`: `method`, `url`,
  `headers`, `params`, `json` **or** `data`, `auth` (bearer/basic), `timeout`,
  `max_chars`. Returns `{ok,status,elapsed_ms,headers,json|text}`; parses JSON by
  Content-Type; **never echoes the request's Authorization header back**.
- `reference/recipes.md` — token GET, JSON POST, cursor pagination, retry/backoff,
  multipart upload, the integration pipeline (API → data-analysis → sql-db), and
  secrets handling.

## Why this milestone is conflict-free

A new bundle touches **only** `plugins/builtinskills/` — never `cmd/agezt/main.go`
or `kernel/runtime`/`agent`/`governor`. The seeder auto-loads it. It tests in
isolation: `go test ./plugins/builtinskills/`.

## Verification

- **Isolated gate:** `go vet` clean; linux cross-build of `./plugins/builtinskills/`
  clean; `gofmt -l` empty; `python -m py_compile api.py` passes; `sh -n setup.sh`
  clean. Package suite green — `TestSeedAll_InstallsHTTPAPI` asserts the bundle
  seeds **active** and materializes `api.py` / `setup.sh` / `recipes.md`;
  bundle-count assertions now cover eleven bundles.
- No new Go dep; no new env. `.gitattributes` already forces LF on
  `plugins/builtinskills/**`. Full `go build ./...` deliberately skipped.

## Notes
- Eleven seeded bundles now ship. http-api-client + the data bundles complete an
  integration loop: call an API → DataFrame the rows → persist with sql-db →
  bundle outputs with archive-tools. Distinct from the built-in `fetch` tool,
  which stays the right choice for reading rendered web pages.
