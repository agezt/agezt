# M340 — State FileStore: accessor traversal guards + concurrency coverage

## Why
Priority-A coverage on a core correctness/security path. `kernel/state.FileStore`
is the projection key-value store the daemon rebuilds from events (agents,
config, planner, scheduler all read/write it). Two genuine gaps, found by reading
the code against its tests:

1. **Traversal guard only tested on `Set`.** The namespace (`ns`) is the sole
   caller-controlled value that becomes a filename (`pathFor(ns) = dir/ns.json`);
   `validateNamespace` is an allowlist (a-z A-Z 0-9 `_` `-` `.`, with `.`/`..`
   rejected). `Get`, `Delete`, and `Keys` all call it too — but no test asserted
   that. A future accessor added without the guard (or a guard accidentally
   removed from a read path) would be a path-traversal hole and the suite would
   stay green.
2. **No concurrency test.** The store is shared across the daemon's agent loop,
   scheduler, and planner; its `RWMutex` is load-bearing, yet nothing exercised
   concurrent Set/Get/Delete/Keys/Namespaces.

## What
Test-only, white-box (`package state`). Added to `kernel/state/state_test.go`:
- **`TestValidateNamespace_EnforcedOnAllAccessors`** — for a set of forbidden
  namespaces (`""`, `.`, `..`, `../escape`, `a/b`, `` a\b ``) asserts **every**
  accessor (`Get`, `Delete`, `Keys`, `Set`) returns `ErrInvalidNamespace`. Locks
  the guard onto the whole surface so a new/edited accessor can't silently skip
  it.
- **`TestConcurrentAccess_RaceSafe`** — 16 goroutines × 200 iterations hammering
  Set/Get/Delete/Keys/Namespaces across 4 namespaces, then a post-storm
  consistency sweep (every reported key must still be Gettable). Designed to run
  under `go test -race` in CI; without the detector it still proves no
  panic/deadlock and a self-consistent store.

## Verification
- `go test ./kernel/state -run 'ValidateNamespace_EnforcedOnAllAccessors|ConcurrentAccess' -v`
  — both pass (concurrency test ~3.6s).
- `gofmt -l` clean; `go vet ./kernel/state/` clean; `GOOS=linux go build ./...`
  exit 0. Full suite **2063** passing (was 2061; +2), `go test ./...` exit 0.
  `go.mod` / `go.sum` unchanged.

## Honest scope note
- The race **detector** could not run on this host: `go test -race` requires cgo
  and there is no C compiler here (`CGO_ENABLED=0`, no gcc). The concurrency test
  therefore ran WITHOUT `-race`, verifying functional consistency only; the data-
  race guarantee is delegated to a CI run with cgo enabled. This is recorded
  rather than glossed — per the project rule not to claim a verification that
  wasn't performed.
