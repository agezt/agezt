# M561 — REAL race: plugin Reload didn't join the old read loop before respawn

## How it surfaced
The CI `race (linux, cgo)` job (which I can't run offline — no CGO/C compiler, the
documented exception) failed intermittently on
`kernel/plugin TestReload_CorrelationIDsStayMonotonic`:

```
reloadid_test.go:109: Reload: plugin reload: respawn: initialize:
  plugin: connection lost: read stdout: read |0: file already closed
```

Not a data-race report — a phantom "connection lost" on the *new* child's
`initialize`, intermittent (it passed the prior CI run; passes 30× locally without
`-race`). `-race`'s slower scheduling widens the window.

## Root cause
`Reload` = `Close()` (tear down old child) → `respawn()` (reuse `p` in place).
`Close` reaps the old process, which shuts the old stdout pipe; the **old
`readLoop` goroutine** then gets `read |0: file already closed` from `readFrame`
and calls `markDead(...)`. But `Close` never *joined* that goroutine — it could
still be in flight when `respawn` ran `p.dead.Store(false); p.setDeathErr(nil)`.

`markDead` does `dead.CompareAndSwap(false, true)`; once respawn reset `dead` to
false, the old loop's late `markDead` **succeeded against the new child**, setting
`dead=true` + the stale "file already closed" death error. The new child's
`initialize`/`invoke` then saw dead → returned `connection lost: …file already
closed`. (Secondary: the old loop read `p.stdout` lock-free while respawn
reassigned it — a data race on the field.)

## Fix (`kernel/plugin/host.go`)
Join the old read loop before respawn reuses the struct:
- Added a per-loop `readDone chan struct{}`; `readLoop(done)` does `defer
  close(done)` (closes the channel it was *started with*, so a respawn that
  replaces the field never closes the wrong one).
- `Spawn` and `respawn` each create a fresh `readDone`, store it under `mu`, and
  pass it to `go p.readLoop(done)`.
- `Reload` snapshots the old `readDone`, calls `Close()`, then **`<-oldReadDone`**
  before `respawn()`. So the old loop has fully returned (and done its harmless
  markDead) before respawn resets liveness state and installs the new pipe — no
  late clobber, no `p.stdout` data race.

Bounded: `Close` reaps the process → old pipe closed → `readFrame` errors → loop
returns → `defer close(done)` fires (even on panic). `<-oldReadDone` can't hang.
`readDone` is nil-guarded (a never-started loop).

## Verification
- `go build ./...` 0; `go vet` 0; gofmt-clean staged blob.
- `TestReload*` 30× and the full `kernel/plugin` suite pass; full `go test ./...`
  exit 0 (`GOMAXPROCS=3 -p 2`); `go.mod`/`go.sum` unchanged.
- **`-race` cannot run offline** (CGO exception) — relying on the CI `race` job to
  confirm; the fix is a standard goroutine-join and removes the only window where a
  dying loop touches the respawned child's state.

## Note
This is the second real bug the runtime/CI dimension surfaced (after M550). It is
exactly the class unit tests miss — an intermittent reload-respawn lifecycle race
only the race detector's scheduling exposes.
