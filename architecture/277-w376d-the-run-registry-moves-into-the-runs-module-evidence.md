# W3.76d — the run registry moves into the runs module

## What the slice is

The **twelfth** slice of the runtime's dissolution and the **fourth** slice of
the **run engine** programme (W3.76). W3.76a took the durable **tickets**,
W3.76b the **steering**, W3.76c the **run context**; this one takes the **run
registry** — the first of the three nouns the target architecture names for the
runs module ("**run registry**, resume tickets, steering/interventions"), and
the last one left in the daemon.

The family was split across two places:

- `kernel/runtime/runtime_kernel.go` and `kernel/runtime/accessors_misc.go`
  held the daemon's **state**: the live-run map (`runs map[corr]CancelFunc`),
  the halt latch (`halted`), the drain waitgroup (`runWG`), the suspend latch
  (`suspending`) and the mutex that guarded the first two (`runsMu`) — and the
  **six public verbs** over them (`Halted`, `SetHalted`, `Runs`, `RunsMu`,
  `RunWG`, `Suspending`);
- `kernel/runtime/lifecycle/` (**3** files, **267** non-blank lines) held the
  **operations**: `Halt`/`HaltWith`, `Resume`/`ResumeWith`, `CancelRun`,
  `DrainAndHalt`, `ActiveRuns`, `ActiveRunIDs`, `NewCorrelation`,
  `SubjectForRun` — and a `KernelAPI` interface whose seven verbs were exactly
  the daemon's state, handed to the manager as a port.

## What the slice found

The daemon's half of the family was **state without operations**, and the
operations were **state without ownership**: the manager reached the daemon
through a seven-verb `KernelAPI` to touch six fields that the daemon itself
only ever read. The manager did not need the daemon to *do* anything — it
needed it to **read and write four things** (the live runs, the halt latch, the
drain counter, the suspend latch) and to perform **two side effects** it
cannot perform itself (`Suspend` — the daemon's own restart protocol — and
`PublishBus` — the daemon's bus).

That is the same shape W3.76b found in the steering family: a family is
**port-shaped** when its operations touch the host in a small, enumerable way.
Here the touch was small and the ownership was wrong, so the split was
unambiguous: **the data is the module's own** (the runs module owns the run
registry), therefore the **operations over it are the module's** too.

The target architecture's own sentence is the decision, not a coincidence:
the runs module owns "run registry, resume tickets, steering/interventions".
All three are now in `kernel/modules/runs/`.

## What changed

### The module owns the store and the operations

```
kernel/modules/runs/registry/registry.go     (118)  the live-run store
kernel/modules/runs/lifecycle/api.go          (18)  the Host interface
kernel/modules/runs/lifecycle/doc.go          (17)  the package's contract
kernel/modules/runs/lifecycle/manager.go     (170)  the halt/resume/cancel surface
```

The **registry** (`registry.Registry`) is a plain store with an **explicit
lock discipline**: `Lock`/`Unlock` are part of its surface, because the daemon's
compound cleanups must hold the registry's lock across the fan-out, tree, steer
and spawn locks — that lock order **starts** at the registry's, so the lock is
the store's contract and not a hidden field. Every verb below it expects the
lock to be held; the waitgroup and the suspend latch have their own
synchronisation and are safe on their own.

The verbs: `New`, `Lock`/`Unlock`, `Halted`/`SetHalted`, `Running`,
`Register`/`Unregister`, `Cancel`, `Count`, `IDs`, `Cancels`, `HaltAll`,
`Add`/`Done`/`Wait`, `Suspending`.

The **lifecycle manager** holds the registry **directly** — the registry is the
module's own data, so the manager does not reach back to the host for it. The
host interface is now **two verbs**:

```go
type Host interface {
	Suspend(reason string) int
	PublishBus(spec event.Spec) (*event.Event, error)
}
```

### The daemon keeps the store handle — and nothing else

The daemon's five fields (`runs`, `runsMu`, `halted`, `runWG`, `suspending`)
collapsed into **one** field, `registry *registry.Registry`, and the six public
verbs over them are **gone**: `Halted`, `SetHalted`, `Runs`, `RunsMu`, `RunWG`
and `Suspending` were public API of the daemon and are now either the module's
own verbs or gone.

What the daemon still has, and why:

- the **store handle** — the daemon registers the runs, drains them and reads
  the halt latch; that is its own business (the run engine's), and the store
  is the module's implementation of it;
- the **two side effects** — `Suspend` and `PublishBus` are the daemon's
  (the restart protocol and the daemon's bus), so the module asks for them
  through the `Host` port;
- the **doors it needs for its own callers** — `IsHalted()` and the halt
  refusals in the run executor and the brain-distill path stay, because they
  are the daemon's answers to its own callers, not the module's API.

## The measurements

Base `9863c497` (the W3.76c merge), working tree after this slice:

| Measurement | Base | Now | Delta |
|---|---|---|---|
| `kernel/runtime` (recursive, production) | 73 files / 8,968 | 70 files / 8,686 | −3 files / −282 lines |
| `kernel/runtime` (upper level, production) | 64 files / 7,809 | 64 files / 7,794 | −15 lines |
| `kernel/modules/runs` | 16 files / 1,763 | 20 files / 2,086 | +4 files / +323 lines |
| `kernel/runtime/lifecycle` → `kernel/modules/runs/lifecycle` | 3 files / 267 | 3 files / 205 | −62 lines |
| `kernel/modules/runs/registry` | — | 1 file / 118 | new |
| the daemon's public verbs (`func (k *Kernel) [A-Z]`) | **244** | **238** | **−6** |
| `kernel/runtime`'s test functions | 396 | 392 | −4 (−9 moved out, +5 new) |
| the contract's new rows | 0 | 0 | 0 |
| the application layer's changed production files | 0 | 0 | 0 |

The verb count is verified with `git grep -h -E '^func \(k \*Kernel\) [A-Z]'`
against the base commit and the working tree (**244 → 238**); the six that
disappeared are exactly the six this slice moved.

The **205 vs 267** in the lifecycle package is the slice's real cost: the
`KernelAPI` interface's seven verbs and the manager's access to them are gone,
replaced by two host verbs and a direct store handle.

## The tests

- `kernel/modules/runs/registry/registry_test.go` — **8** test functions: the
  register/unregister round trip, the ownership rule (unregister does not
  cancel), the cancel path (removes **and** hands back), the halt sweep (takes
  every run, empties the registry, latches the flag), the read side (ids and
  cancels describe the same live set), the drain accounting, the suspend latch
  and the halt setter's round trip;
- `kernel/modules/runs/lifecycle/manager_test.go` — **9** test functions,
  **moved** from `kernel/runtime/lifecycle/`: the fake host became a
  `fakeHost` with the two verbs the port has, and the fixture builds the
  manager over a **real** registry, because the registry is the module's own
  store and a stand-in for it would test the stand-in;
- `kernel/runtime/registry_ratchet_test.go` — **5** test functions: the
  registry lives in the runs module, the daemon holds no run-registry field,
  the daemon holds no registry accessor, the daemon keeps the lifecycle doors,
  and the daemon talks to the module through the registry.

## The mutation run

**30 mutations, 30 caught, 0 survived, 0 broken.** Two byte-identical runs,
SHA-256 `6BD9ACF83BBFB504F9AC3367DC7EE7281C8C9E421DCA02EBDE1F48FDE489BCCF`.

The first run found **five** things worth recording, and three of them are
about the mutation, not about the code:

1. **A mutation that changes a capacity is not a mutation that empties a
   list.** `ids := make([]string, 0, len(r.entries))` → `ids := []string{}`
   compiles, looks like "the ids are never listed", and changes nothing:
   `append` fills the slice either way, and the test passed. The same held
   for the cancels. The mutation was rewritten to drop the append itself —
   but then the loop variable became unused (`declared and not used`), so the
   working mutation is `_ = corr`: the id is read, and nothing is listed.
2. **`ok = ok` is not a mutation.** The first version of "the cancel does not
   forget the run" assigned `ok` to itself; it compiled, it changed the text,
   and it changed nothing. The mutation now removes the `delete` from
   `Cancel` — the run really does stay in the registry.
3. **`SetHalted` had no test at all.** The mutation "the halt flag is never
   set" survived because the setter was never exercised: the latch was only
   ever set by `HaltAll`, which sets it itself. This was a **test gap in the
   new code**, not a bad mutation, so the gap was closed
   (`TestRegistry_SetHaltedLatchesAndClears`) and the mutation's `-run`
   pattern was widened to cover both tests.
4. **A mutation that removes the only use of an import is a build error, not a
   kill.** Three mutations were BUILDERR: replacing the cancels with a
   `context.CancelFunc` literal, dropping the `sort` call, and dropping the
   `ulid` call. Each one left an import unused. The mutations were rewritten
   so they compile and still break the behaviour: `cancels = nil` (the halt
   takes no cancel), `sort.Reverse` (the ids are sorted backwards) and a lost
   `run-` prefix (the correlation loses its prefix).
5. **The harness's own discipline held.** The mutation loop runs inside
   `try { } finally { Restore-All }`, so an interrupted run cannot leave a
   mutated tree behind — the failure mode that bit W3.76c.

## What is left

The runs module now holds **three of the three** nouns the target
architecture names for it — the run registry, the resume tickets and the
steering/interventions — plus the run context (W3.76c) and the durable resume
handshake. The remaining run-engine families in the daemon are the **prompt**
assembly (520 lines, one public verb, but roughly eight ports: memory, taste,
world, forge, bus, config and the context selection), the **delegation**
surface (939 lines, 13 verbs, which touches the bus, the journal, the roster
and the spend limits and therefore needs a port design before it moves), the
**loop config** (99 lines) and the **loop** itself (`kernel/agent`, 2,629 lines
across 20 files, the most contract-bound family of all).

Evidence for the neighbouring slices:
`architecture/274-…`, `architecture/275-w376b-the-steering-moves-into-the-runs-module-evidence.md`,
`architecture/276-w376c-the-run-context-moves-into-the-runs-module-evidence.md`.
