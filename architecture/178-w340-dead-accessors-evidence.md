# W3.40 — the kernel's dead accessors sub-manager is deleted

## What was actually in the tree

`kernel/runtime/accessors` was a **sub-package of the kernel** that the kernel
built during startup and then never called:

```go
// kernel/runtime/compose.go
k.accessors = accessors.New(k)
```

That assignment was the package's **only** production use. Nothing in the
tree ever read `k.accessors`. The package was:

| File | Content |
|---|---|
| `accessors.go` | 38 functions — the read surface, one-liner getters and live reads |
| `accessors_mutate.go` | 10 functions — the standing / roster CRUD, with their bus events |
| `api.go` | the `KernelAPI` interface the Accessor spoke to (≈45 methods) |
| `doc.go` | the package's story |
| `accessors_test.go` | 51 tests, 413 lines — testing a copy nobody used |

**48 production functions, 898 lines** — a second copy of the kernel's own
read surface, with a full test suite of its own.

## Why it was never called

The package was announced as the "Day 13 split" of the kernel's read surface:
the `*Kernel` bodies were supposed to become *thin delegations* to the
`Accessor`. They never did, and the reason is a genuine cycle:

```go
// kernel/runtime/accessors.go (before W3.40)
// ... their legacy *Kernel bodies have become thin delegations to the
// sub-package's Accessor (see runtime.go).
```

An `Accessor` dispatches back into `*Kernel` through the implicit `KernelAPI`
interface; if `*Kernel`'s own getter delegated to the `Accessor`, the call
would recurse. So `*Kernel` kept the bodies and read the fields **directly**,
and the `Accessor` — which wraps the same `*Kernel` — sat beside it with a
copy of the same logic.

The comment claimed a delegation that did not exist, and the `*Kernel` bodies
in `accessors_standing.go` / `accessors_roster.go` said so themselves:

> They read the underlying fields directly (rather than going through
> `*accessors.Accessor`) to avoid the recursion that would result from the
> Accessor dispatching back into `*Kernel` via the implicit `KernelAPI`
> interface.

**So the package was unreachable by construction.** The split was announced,
the code was written, the tests were written — and the call was never made.

## Nothing observable was lost

Both copies published the **same bus events**, so deleting the unreachable one
cannot change what the daemon writes:

| Copy | Events |
|---|---|
| deleted `accessors_mutate.go` | `standing.created`, `standing.updated` ×2, `standing.removed`, `roster.created`, `roster.updated` ×3, `roster.removed` |
| live `accessors_standing.go` / `accessors_roster.go` | the same nine, one for one |

The two implementations had **not drifted**: the live ones were the bodies the
deleted ones forwarded to, so the deletion removes a forward, not a behaviour.

## The slice

Five files deleted (`git rm`, so the history follows the code):

```
kernel/runtime/accessors/accessors.go
kernel/runtime/accessors/accessors_mutate.go
kernel/runtime/accessors/accessors_test.go
kernel/runtime/accessors/api.go
kernel/runtime/accessors/doc.go
```

The wiring went with them: the `accessors` field on `*Kernel`, the import in
`runtime_kernel.go`, and the construction in `compose.go`.

Two comments that described the delegation were corrected to describe what was
true, and `runtime_lifecycle.go`'s half-finished sentence was finished.

### The dead getter went with the package

`ConfigMu()` had the deleted package as its **only** caller — the `Accessor`
took the host's mutex to honour the lock order. No live `KernelAPI` required
it, so it was deleted too. A getter nobody calls is the same defect as a copy
nobody calls.

`Scheduler()` and `AgentGateway()` were **kept**: they are required members
of the live `runexec.KernelAPI`, which the run engine really uses.

## The ratchet went DOWN by eleven

```
OK: 309 packages placed; 66 allowlisted import violations and 13 allowlisted
call sites remain (ratchet).
```

**77 → 66.** Eleven edges were deleted from `tools/archcheck/allowlist.txt`:
ten `cross-module` and one `upward`:

```
cross-module kernel/runtime/accessors -> kernel/modules/artifacts/artifact
cross-module kernel/runtime/accessors -> kernel/cadence
cross-module kernel/runtime/accessors -> kernel/modules/providers/catalog
cross-module kernel/runtime/accessors -> kernel/modules/artifacts/datalake
cross-module kernel/runtime/accessors -> kernel/market
cross-module kernel/runtime/accessors -> kernel/modules/knowledge/memory
cross-module kernel/runtime/accessors -> kernel/modules/knowledge/worldmodel
cross-module kernel/runtime/accessors -> kernel/modules/skills/skill
cross-module kernel/runtime/accessors -> kernel/modules/agents/roster
cross-module kernel/runtime/accessors -> kernel/standing
upward    kernel/runtime/accessors -> kernel/agentgw
```

Every one of them was an edge of the **deleted** package. The kernel's module
reach was being counted **twice**: once for `kernel/runtime` and once for the
copy that never ran. The allowlist had been carrying the price of a dead
package since the "Day 13 split", and the ratchet never showed it, because the
ratchet counts edges — not whether the edges mean anything.

This is the largest single drop of the whole W3 series, and it is the
cheapest one: no behaviour changed, and the reach got smaller.

## What is left of the kernel's reach

| Package | Module edges |
|---|---|
| `kernel/runtime` (the root) | **22** |
| `kernel/runtime/accessors` | 0 — **deleted** |
| `kernel/runtime/runexec` | 11 |
| **total** | **33** (of 46 cross-module edges in the whole allowlist) |

The root is the next slice: it still reaches 22 modules, and its accessor
methods return **concrete** module types (`Memory() *memory.Manager`,
`World() *worldmodel.Graph`, `Forge() *skill.Forge`, …). Those rows are the
remaining shape of the kernel's reach, and they are the rows that have to
become contract ports.

## The proof

### Permanent tests

The deletion removed a **compile-time check**: the deleted package declared a
`KernelAPI` interface of ≈45 methods, and handing `*Kernel` to it proved the
kernel answered every one of them. That check protected an unreachable copy —
so it protected nothing — but the surface it guarded **is** the surface
callers use. The loss is therefore compensated by an explicit, **named** list
in `kernel/runtime/submanager_test.go`:

| Test | What it holds |
|---|---|
| `TestTheKernelHoldsOnlyTheSubManagersItCalls` | the kernel holds **exactly** two sub-managers — `lifecycle` and `runexec`. The set is read off the **type** (`reflect.TypeOf(runtime.Kernel{})`), so a third sub-manager that nobody calls is caught the moment it is *wired*, not the moment it is noticed. The message says why: a sub-manager nobody calls is a second copy of the surface, and it is counted twice in the ratchet |
| `TestTheKernelStillAnswersItsAccessorSurface` | the **42 accessor names** the kernel answers are pinned by name, and **`ConfigMu`'s absence is pinned too** — if it comes back, it comes back dead |

The second test's comment states the trade honestly: the deleted package's
compile-time check is gone, so the surface is pinned *by name* instead. A
getter that disappears now fails **here**, instead of failing nowhere.

### The mutation run

**6 mutations: 6 caught, 0 survived, 0 skipped, 0 build errors.**

| # | Mutation | Caught by |
|---|---|---|
| 1 | the kernel gains a **third** sub-manager handle | `TestTheKernelHoldsOnlyTheSubManagersItCalls` — the set is read off the type, so a wired-but-uncalled sub-manager is caught at wiring time |
| 2 | the deleted package comes back (and is wired) | `TestTheKernelHoldsOnlyTheSubManagersItCalls` |
| 3 | the accessor surface loses a member (the kernel's and the run engine's `Scheduler()`) | `TestTheKernelStillAnswersItsAccessorSurface` |
| 4 | the dead getter comes back (`ConfigMu()`) | `TestTheKernelStillAnswersItsAccessorSurface` — the **absence** is pinned |
| 5 | the package comes back **with a module edge** (the ratchet counts it again) | **archcheck** — `cross-module kernel/runtime/accessors -> kernel/modules/knowledge/memory` |
| 6 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |

**Two of the six are gate-measured**, and that is the point of the slice: the
claim "the kernel's reach is smaller" is not something a test can see — it is
what archcheck counts. A test would happily accept a package that reaches a
module, as long as it is called.

### The honest record of the run

The **first** run of the harness was invalid and was thrown away. Five of the
six mutations were measured correctly, but mutation 5 was not: the harness
saved the *original* of the file the mutation **creates** — a file that does
not exist — so `ReadAllText` failed, the created package was then written
**empty** by the restore step, and archcheck failed for a reason that had
nothing to do with the module edge. A mutation caught for the wrong reason is
not a caught mutation; it is a measurement that happened to pass.

The harness was fixed to mark a created file as **create-only** (no original to
save, no text to write over it) and to **refuse** an empty or wrong package,
and the run was repeated. The second run caught mutation 5 for the reason it
was written for:

```
M05 CAUGHT ::   - cross-module kernel/runtime/accessors -> kernel/modules/knowledge/memory
```

The restore check passed on both runs: every mutated file is byte-identical to
its original, and the deleted package is still gone.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 309 packages placed; 66 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 77** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rule the slice is really about

**Dead code that is a copy of live code is deleted, not converted.**

The tempting move — "the Accessor's methods are a nice narrow surface, let me
port them to the contract layer" — would have been the wrong one: it would
have kept a copy of the kernel's read surface alive under a new name, in a
new layer, with a new set of edges. The kernel already answers every one of
those questions; the copy answered them for nobody.

And the second rule, the one the slice had to learn the hard way: **a
ratchet that counts edges also counts the edges of a package that never
runs.** The eleven edges did not disappear because the architecture improved
— they disappeared because the thing they belonged to was deleted.

## What is deliberately left

`PublishBusEvent` has no caller in the tree any more (the `Accessor` was its
consumer), but it is **kept**: it is the one place the kernel publishes a
standing / roster event, and the app layer passes the standing / roster CRUD
in as closures. Removing it would move that event into the callers. It now
says so in its own comment instead of claiming an `Accessor` that is gone.

The rest of the kernel's reach is untouched: the root still reaches **22**
modules and still returns concrete module types. That is the next slice.
