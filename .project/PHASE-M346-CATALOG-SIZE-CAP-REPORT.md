# M346 — Catalog sync size-cap (OOM guard) coverage

## Why
Priority-A coverage on an untrusted-input boundary. `catalog.Syncer.Sync` fetches
the model/provider catalog over HTTP from an operator-configured URL (`AGEZT_CATALOG_URL`)
and caps the response body at `MaxSyncBytes` (8 MiB) via an
`io.LimitReader(resp.Body, MaxSyncBytes+1)` + length check, so a misbehaving or
hostile source can't OOM the daemon with a multi-gigabyte response. The fetch path
had tests for the happy path and the non-200 rejection, but the **size-cap
rejection itself** — the actual OOM guard — was untested. A regression that dropped
or loosened the cap would keep the suite green.

## What
Test-only. Added to `kernel/catalog/catalog_test.go`:
- **`TestSyncer_RejectsOversizedBody`** — an `httptest` server returns
  `MaxSyncBytes+1` bytes; `Sync` must return an error whose message is the
  size-cap rejection (`"…body exceeds …"`). Because the size gate runs *before*
  `ParseAPIFile`, the filler bytes are rejected on size — not JSON validity — so
  the test pins the cap specifically, not an incidental parse failure.

## Verification
- `go test ./kernel/catalog -run Syncer -v` — all three syncer tests pass
  (happy path, non-200, and the new over-cap rejection).
- `gofmt -l` clean; `go vet ./kernel/catalog/` clean; `GOOS=linux go build ./...`
  exit 0. Full suite **2071** passing (was 2070; +1), `go test ./...` exit 0.
  `go.mod` / `go.sum` unchanged.

## Scope notes
- No production change — the `LimitReader` + length check already worked; this
  pins the OOM guard. The rest of catalog (parse/merge/find-model/store load-save/
  Ollama discovery/modality/tool-capable-alternative selection) was already
  thoroughly covered.
- Mirrors the same untrusted-input size-bound discipline already tested elsewhere
  (plugin host frame cap, control-plane request cap, skill-registry fetch cap);
  this closes the catalog-sync member of that family.
