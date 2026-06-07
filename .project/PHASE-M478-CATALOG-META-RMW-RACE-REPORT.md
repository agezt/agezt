# M478 — Catalog store: serialize meta updates + unique atomic-write temp

## Context
The catalog `Store` keeps a `meta.json` sidecar (sync timestamps, source URLs,
counts). `SaveAPI` and `SaveLocal` each write their data file, then
read-modify-write `meta.json` (mutating disjoint fields). The control plane handles
each command in its own goroutine, so `catalog sync` (`SaveAPI`) and `catalog
discover` (`SaveLocal`) can run concurrently.

## The bugs (MED)
1. **Lost update.** The RMW was unsynchronized:
   ```go
   meta, _ := s.LoadMeta(); meta.APISyncedAt = ...; return s.SaveMeta(meta)   // SaveAPI
   meta, _ := s.LoadMeta(); meta.LocalSyncedAt = ...; return s.SaveMeta(meta)  // SaveLocal
   ```
   Each loads the whole struct, mutates its fields, and writes the whole struct. Run
   concurrently, both can load before either saves → the second `SaveMeta` clobbers
   the first's fields. The meta sidecar silently loses one side's timestamps/source.
2. **Shared temp.** `atomicWrite` used a fixed `path + ".tmp"`, so two concurrent
   writes to the *same* target (e.g. two `SaveMeta`) race on `meta.json.tmp` — one
   renaming a half-written temp the other is still writing.

Sidecar-metadata only (informational), hence MED, not HIGH.

## The fix
- A `metaMu` mutex on `Store` + an `updateMeta(fn func(*Meta))` helper that
  load-mutate-saves under the lock; `SaveAPI`/`SaveLocal` use it; `SaveMeta` also
  locks (via `saveMetaLocked`). Serialized RMW with a fresh load inside the lock, so
  disjoint updates accumulate instead of clobbering.
- `atomicWrite` now writes a **unique** temp (`os.CreateTemp(dir, "."+base+".*.tmp")`)
  then renames — no shared-temp collision for any catalog file.

## Tests + negative controls
`kernel/catalog/catalog_test.go`:
- `TestStore_MetaSaveUsesUniqueTemp` — occupies the old fixed `meta.json.tmp` with a
  directory; `SaveMeta` must still succeed and round-trip. **Negative control:**
  reverting `atomicWrite` to the fixed name made it fail with `is a directory`.
- `TestStore_ConcurrentSaveNoLostMetaField` — concurrent `SaveAPI` + `SaveLocal`;
  both `APISourceURL` and `LocalSource` must survive. Passes `-count=5`. **Negative
  control:** removing the `metaMu` lock from `updateMeta` reproduced the lost update
  (`Local source lost by concurrent meta update: ""`) within `-count=30`.

Both restored; tests pass. (The metaMu serialization also removes a concurrent
same-target rename, which on Windows would otherwise fail with EACCES.)

## Provenance
From the scoped review of kernel/openaiapi + kernel/catalog (both openaiapi files,
catalog types/discovery/sync reviewed CLEAN; the catalog is swapped via
`atomic.Pointer`, so no concurrent-map data race on the live catalog).

## Verification / gate
- `kernel/catalog` tests pass (`-count=3`).
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
