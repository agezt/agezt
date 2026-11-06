# W3.76ae — the run's live state moves into the runs module

**Slice:** W3.76ae (the thirty-ninth of the runtime's dissolution)
**Base:** `379938a7` (the W3.76ad merge)
**Date:** 2026-11-06

---

## 1. The family

A run in flight has a **live state**: it is registered in the live-run
registry, it holds a cancel func, a drain ticket and a **steering control** that
an operator can pause, step or steer from another goroutine.

That state was the **daemon's**:

- the `k.steers` map (correlation id → the live `*steer.Control`) behind the
  daemon's own `steersMu`;
- the three verbs that created and released it — `SetupRunState`,
  `CleanupRunState`, `DeregisterRunSteer` — with their bodies in the daemon's
  `runner_lifecycle.go`.

And the run engine — which lives **inside** the runs module — asked the
**host** for all three through its `KernelAPI` port:

```go
r.k.SetupRunState(corr, ctx)      // the engine asks the host
    → k.setupRunState(corr, ctx)  // the host answers
```

**A port entry that is a round trip inside one module is a promise the host
never had to keep.** The module has the body; the module calls it.

Worse: the objects themselves were the module's. The live steering controls are
`steer.Control` — the runs module's type — and the module's **own steering
service** asked the **host** for them back:

```go
Lookup: func(corr string) (*steer.Control, bool) { rc := k.controlFor(corr); ... }
```

## 2. What moved

A new package: **`kernel/modules/runs/state`** — the runs module's live-run
state. It owns:

- the **steering registry** (the live control of every run in flight);
- the **per-run registration**: the halted check, the duplicate-correlation
  guard, the tenant and auto-approve decoration, the run's deadline, the
  registry entry and the drain accounting;
- the **per-run cleanup**: the registry entry, the delegation tallies, the
  steering slot and the orphaned async spawns;
- the **child runs' steering registration** (`RegisterSteer` /
  `UnregisterSteer`), which the delegation engine reaches through the host's
  door;
- the **shutdown broadcast**, which the daemon's suspend path asks.

The daemon keeps the **store handle** (`k.registry`, the module's own
`registry.Registry`) and hands the module its own configuration: the tenant, the
operator's auto-approve grant and the run ceiling.

The daemon's three verbs and its steering map are gone, and the run engine
asks the module:

```go
runCtx, cancel, steer, err := r.runState.SetupRunState(corr, ctx)
```

## 3. The port

The run engine's host port lost **three** entries — `SetupRunState`,
`CleanupRunState` and `DeregisterRunSteer` — and carries **34** now (it carried
37). What stayed is what the **host** owns: the run's setup helpers that are
genuinely the host's (`BuildRunPrompt`, `InjectHostEnvironment`,
`ResolveRunModel`), the resume handshake, the roster's read and the host's own
state.

## 4. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 121 | **118** |
| the daemon's production lines | 5,571 | **5,418** |
| the daemon's production files | 55 | **55** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the run engine's host port (`KernelAPI`) | 37 entries | **34 entries** |
| the run engine's package | 908 | **935** |
| the runs module's **live-run state** | — | **new: 1 file / 279 lines** |

**Three verbs off the daemon's surface and 153 lines with them** — the
daemon's biggest line drop yet, because the bodies and the account of them
left together. The run engine's package grew by 27 lines (the state's handle,
the comments that say where the calls go), and the module's new state package
holds the 279 lines that moved.

## 5. The sentinel

`ErrHalted` — the sentinel a run gets on a halted daemon — now lives with the
state that refuses the run (`state.ErrHalted`), and the run engine re-exports
it (`runner.ErrHalted`) and the daemon re-exports it (`runtime.ErrHalted`): one
value, three names, and `errors.Is` matches every one.

## 6. The guard lists

The control plane's **state** family excludes `CleanupRunState` and
`SetupRunState` from its word rule ("State" is the state family's business, and
the run's state is not). Both verbs are **gone** now, and the exclusion stays
written down: a verb that comes back must not fall into the state family's lap
either.

## 7. The ratchets — and the probe

**The daemon's side** (`kernel/runtime/run_state_ratchet_test.go`, new): the
daemon holds **no** verb per run-state operation, its production files declare
none, and it holds **no** steering registry — no map, no mutex.

**The module's side** (`kernel/modules/runs/state/state_ratchet_test.go`, new):
the state holds the steering registry, it registers and releases the run in the
module's own store, and the **daemon** holds none of it.

**The engine's side** (`kernel/modules/runs/runner/runner_state_ratchet_test.go`,
new): the engine's host port carries **no** run-state entry, the engine holds
the module's state, and a host that wired none leaves the engine quiet.

The daemon's own surface ceiling followed the floor again: **121 → 118** (220 →
201 → 194 → 179 → 173 → 166 → 164 → 156 → 152 → 148 → 143 → 135 → 129 → 125 →
121 → 118), and the three retired names joined the retired-verb list.

**The probe** put the daemon's `SetupRunState` and `DeregisterRunSteer` back,
gave the steering registry back to the daemon and put the run-state entry
back into the engine's port: the retired-verb ratchet, the surface ceiling
(120 > 118), the daemon's three run-state ratchets, the module's daemon-half
ratchet and the engine's port ratchet all went red at once — **seven**
ratchets in one shot. The probe was reverted and the suites went green again.

## 8. The coverage moved with the subject

Two ratchets that watched the daemon's own files learned the move:

- the **registry ratchet** now watches the module's state file for the
  registration (`reg.Register(`, `reg.Unregister(`, `reg.Add(`, `reg.Done()`)
  and pins that the daemon's runner lifecycle file registers **nothing** any
  more;
- the **run-context ratchet** now watches the module's state file for the
  auto-approve merge — the daemon no longer decorates a run's context, the
  module does — and keeps the daemon's own remaining reads.

## 9. Verification

- the compiler (`go build ./...`, `go vet ./...`) and `gofmt`;
- the daemon's tests, the run engine's tests, the module's state tests, the
  applications' tests, the control plane's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 10. What is left

The run's live state — the steering registry and the per-run registration —
is the runs module's, and the daemon holds no verb over it.

The daemon holds **118** public verbs and **5,418** production lines; **forty**
application packages still name it; the run engine's host port carries **34**
entries.

What is left in the daemon is the run's **remaining machinery**: the publish
helpers (`PublishIntentInterpreted`, `PublishContextFailureAnalysis`), the
plan runner's registration, the resume handshake and the prompt doors — the
surface a run touches on its way through the host.

The **standing package** itself still lives at `kernel/standing`, and its
eventual home is the **triggers module** (`kernel/modules/triggers`) — the
architecture gate already places both `kernel/cadence` and `kernel/standing` in
that module, so the family's boundary is drawn; the module's directory does not
exist yet, and the move is a path move (four production importers), not a
lifecycle one.
