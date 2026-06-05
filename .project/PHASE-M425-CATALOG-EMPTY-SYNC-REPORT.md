# M425 — Catalog empty-sync no longer wipes the working catalog (MEDIUM)

## Context
The catalog finding from the Forge/catalog review (the Forge fixes shipped in M424).
The catalog's fetch/parse-error path is already fail-safe (a transient error never
reaches `SaveAPI`, so the prior catalog survives), but a *valid-but-empty* result was
not guarded.

## The bug
`kernel/catalog/sync.go`: `ParseAPIFile` treats `null` or `{}` as success —
`json.Unmarshal("null", &byID)` leaves the map nil, yielding a Catalog with zero
providers and no error. The control-plane handler then calls
`CatalogStore().SaveAPI(raw, …)`, overwriting `api.json` with the empty payload. A
sync source (or an intercepting proxy/CDN) returning HTTP 200 with body `null`/`{}`
therefore silently wiped the catalog; the next `Load` yields zero providers and the
Governor has no models to route to — a self-inflicted outage from a "successful" sync.

## The fix
`Sync` now returns an error when the parsed catalog has zero providers, before
returning the raw bytes. Since the control-plane handler journals
`catalog.sync_failed` and returns **without** calling `SaveAPI` on a `Sync` error, the
empty payload is never written and the prior `api.json` is preserved — making the
write path fail-safe against an empty result, matching its existing fail-safety
against fetch/parse errors.

## Verification
- **`kernel/catalog/sync_test.go`** (new):
  - `TestSync_RejectsEmptyPayload`: `null`, `{}`, and whitespace all fail the sync.
  - `TestSync_AcceptsNonEmptyPayload`: a single-provider payload syncs (provider
    count 1).
  - **Negative control:** removing the zero-provider guard → `null`/`{}` sync
    succeeds → `TestSync_RejectsEmptyPayload` FAILs. Restored byte-identical.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2282** passing (was 2280; +2). CHANGELOG
  Reliability entry.

## Review status
This closes the catalog review finding. `catalog/store.go` (sync fail-path preserves
the prior catalog), `discovery.go` (bounded read, no panic on malformed `/api/tags`),
and `types.go` (nil-safe cost accessors, deterministic model selection) were found
clean.
