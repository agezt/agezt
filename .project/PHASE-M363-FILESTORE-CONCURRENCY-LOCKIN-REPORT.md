# M363 — Concurrency lock-in tests for memory & worldmodel FileStore

## Why
A concurrent-map-write idiom audit (Go fatals — not recovers — on a concurrent
map write, even without `-race`: a real daemon crash) swept every long-lived
shared map in the daemon:

- **Channels** (webhook/slack mutex-guarded dedup; discord/telegram hold no
  shared map — discord acks each interaction with `responseDeferred` so it never
  re-delivers, telegram's poll loop is serial) — **clean**.
- **`memory.FileStore`** — `data map[string]Record` guarded by `sync.RWMutex`.
- **`worldmodel.FileStore`** — `entities`/`relations` maps guarded by
  `sync.RWMutex`.

Both stores are correct, but the load-bearing mutex on the two memory/worldmodel
stores had **no concurrency lock-in test** — unlike `state.FileStore`, which got
one in M340. These stores are long-lived singletons: the agent loop upserts
records/entities/relations while control-plane handlers (`agt memory`,
`agt world`, recall/resolve) read them (Get/All/Count) on other goroutines. A
refactor that dropped a lock would silently reintroduce a
`fatal error: concurrent map writes` daemon crash (and, since every `Put`
snapshots to disk under the lock, a torn on-disk snapshot).

## What
Test-only, no production change.
- **`kernel/memory/concurrency_test.go`** — `TestFileStore_ConcurrentAccessGuarded`:
  16 workers × 120 iters hammering Put/Get/All/Count over a bounded id space
  (insert + overwrite), then a post-storm consistency sweep (every record All()
  reports is individually Gettable with non-empty content; Count()==len(All())).
- **`kernel/worldmodel/concurrency_test.go`** — same shape across *both* maps:
  PutEntity/PutRelation/GetEntity/AllEntities/AllRelations/Count, post-storm
  entity-consistency + relation-presence sweep.

## Verification
- **Negative control (proves the test bites):** temporarily neutered the
  `mu.Lock/RLock` pairs in `memory.FileStore` → the test immediately produced
  `fatal error: concurrent map writes` and FAILed. Restored via
  `git checkout` (production file byte-identical, 4 lock sites back).
- `go test ./kernel/memory ./kernel/worldmodel -run ConcurrentAccessGuarded` —
  pass (memory ~2.0s, worldmodel ~4.4s; the disk snapshot under lock makes them
  deliberately heavy).
- `gofmt -l` clean (mine); `go vet` clean; `GOOS=linux go build ./...` exit 0.
  Full suite **2100** passing (was 2098; +2), `go test ./...` 0 failures.
  `go.mod`/`go.sum` unchanged. No CHANGELOG (test-only, like M340).

## Scope notes
- **Host limitation:** `-race` is unavailable here (no cgo/C compiler), so this
  is a functional-consistency stress, not a race-detector run. A concurrent map
  write still fatals deterministically enough under 16×120 contention to catch a
  dropped lock; true race detection is delegated to a cgo-enabled CI.
- The `Record`/`Entity`/`Relation` values returned by All*() carry inner map
  fields (Tags/Attrs), but `Put*` replaces each stored value wholesale and never
  mutates an already-stored value's inner map → no escape-after-unlock hazard.
  Only the outer maps are concurrency-critical, and those are fully guarded.
- **Concurrent-map-write sweep is now CLEAN across the whole daemon** (channels,
  state[M340], memory, worldmodel). Recorded so it is not re-run.
