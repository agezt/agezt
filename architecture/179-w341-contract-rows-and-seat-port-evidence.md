# W3.41 — the kernel names the contract's rows, and holds the contract's port

## What the slice is

The kernel (`kernel/runtime`, layer 3, module `runs`) still reached **22 other
modules**. Three of those edges are gone in this slice, and they are three
different shapes of the same mistake:

| Edge | What the kernel actually needed | What it named instead |
|---|---|---|
| `kernel/runtime -> kernel/modules/work/proof` | the proof row | the module that publishes it |
| `kernel/runtime -> kernel/modules/work/assure` | the verdict row | the module that publishes it |
| `kernel/runtime -> kernel/modules/work/seat` | the seat catalog | the module's concrete store |

The ratchet went **66 → 63**.

The proof and the assure edges were **free**: the rows the kernel names were
*already* the contract's rows, and the kernel used nothing else from those
modules. The seat edge was the real work, and it is the shape the remaining
nineteen edges need: **the kernel holds a port, the composition root owns the
file.**

## Part one — the rows come from the contract

`kernel/runtime/workboard_proof.go` named `proof.Proof`, `proof.Criterion` and
`proof.Evidence`; `kernel/runtime/{api,runexec,runtime_lifecycle,
workboard_proof}.go` named `assure.Verdict` and `assure.Result`.

Every one of those is already a contract row:

```go
// kernel/modules/work/proof/aliases.go
type Criterion = proofapi.Criterion
type Evidence = proofapi.Evidence
type Proof     = proofapi.Proof

// kernel/modules/work/assure/assure.go
type Verdict = assureapi.Verdict
type Result  = assureapi.Result
```

They are **aliases**, so the types were already identical — naming the
contract instead of the module is a spelling change, not a type change, and no
caller moved. The kernel now imports `kernel/contract/proofapi` and
`kernel/contract/assureapi`, and the two module edges are gone.

The **run engine** (`kernel/runtime/runexec`) still names the assure module,
and that edge stays: it drives `assure.Until`, the attempt loop, which is
behaviour and belongs to the module. Only the kernel root's rows moved.

## Part two — the seat catalog is the daemon's file

The seat catalog is a store of operator-defined execution presets, and the
kernel opened it itself:

```go
// before
seatstore, err := seat.OpenStore(filepath.Join(cfg.BaseDir, "seats"))
if err != nil {
    return fail("runtime: seats", err)
}
```

That is the kernel being the composition root — and it is why the kernel
reached the seat module at all. The kernel is a **module**: a module may
import another module's `api` package and nothing else (archcheck's principle
P5). The repo already had the answer for exactly this shape, in
`cmd/agezt/main_toolinvoker.go`:

> It also binds two modules — the reasoning module … and the agents module's
> reaper … A module cannot be imported by the kernel itself … so the composition
> root builds them here, over the kernel's own ports, and injects them.

### The row moved down

`kernel/contract/seatapi` is new: the `Seat` row (pure data — id, name,
description, execution profile, model chain, tool allowlist, icon, builtin)
and the `Store` port the host holds, with the five verbs the host actually
uses:

```go
type Store interface {
	List() []Seat
	Get(id string) (Seat, bool)
	Valid(id string) bool
	Create(spec Seat) (Seat, error)
	Delete(id string) error
}
```

The module keeps its name as an **alias** of the row and asserts the port:

```go
// kernel/modules/work/seat/seat.go
type Seat = seatapi.Seat

// kernel/modules/work/seat/store.go
var _ seatapi.Store = (*Store)(nil)
```

One seat, one spelling of it: the module's implementation is the port's
implementation, and the assertion is in the module so a port change is a
compile error **there** rather than a runtime surprise at the composition
root.

### The composition root owns the file

```go
// cmd/agezt/main_toolinvoker.go
func openAppKernel(cfg kernelruntime.Config) (*kernelruntime.Kernel, error) {
	cfg.NewToolInvoker = apptools.NewInvoker
	if cfg.Seats == nil {
		seats, err := seat.OpenStore(filepath.Join(cfg.BaseDir, "seats"))
		if err != nil {
			return nil, err
		}
		cfg.Seats = seats
	}
	k, err := kernelruntime.Open(cfg)
	...
}
```

The kernel's field is now the **port** (`seat seatapi.Store`), its accessor
answers the port (`Seats() seatapi.Store`), and `compose.go` no longer opens
anything. A store that cannot be opened is still a **startup error** — it is
the daemon that refuses to boot now, not the kernel, so the behaviour at the
daemon is unchanged.

### The missing port is refused, not called

A host that was given no catalog holds a **nil port**. Two things must hold,
and both are measured:

**The daemon end — the port is live.** `TestOpenAppKernelWiresTheSeatCatalog`
opens the kernel through the composition root and asks the catalog: the
built-in seats are there, and a seat that does not exist is refused.

**The tool/app end — the port missing says "not available".**

- `kernel/app/seats`: every operation (`List`, `Create`, `Delete`) returns
  *"the seat catalog is not available"* instead of calling into nothing.
- `kernel/app/workboard`: a task that asks for a seat and finds no catalog
  **degrades** — the same posture an unknown seat takes ("running with agent
  defaults"), recorded as a workboard comment.
- `kernel/controlplane`: the seat validity check mirrors what the port
  itself answers for the case that needs no catalog — an **empty** id means
  "no seat" and stays valid (a task that asks for no seat is not asking for an
  unknown one), while a **named** seat cannot be confirmed without a catalog
  and is refused with *"unknown execution seat"*.

That last one is the subtlety the slice taught. The first version of the
nil-validator refused **every** id, including the empty one — and it broke two
existing workboard tests, because a task with no seat is not asking for an
unknown seat. The port's own semantics (`Valid("")` is true) are now mirrored
rather than invented.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/seatapi`: `TestTheSeatRowIsTheContractRow` | the row's **nine fields** and their **JSON tags** — a row may not grow a field, and the wire shape is the contract's |
| `kernel/contract/seatapi`: `TestTheStorePortHasTheVerbsTheHostUses` | the port's **five verbs** and their argument types — a verb nobody calls is a promise nobody can check |
| `kernel/runtime`: `TestTheKernelHoldsTheSeatPortItWasGiven` | the kernel holds **exactly** the port it was given — not a wrapper of its own, and not a store of its own |
| `kernel/runtime`: `TestTheKernelDoesNotOpenASeatCatalogOfItsOwn` | a kernel given no catalog holds **nil** — and nil, not a non-nil interface holding a nil pointer (the typed-nil trap) |
| `kernel/controlplane`: `TestTheSeatValidatorRefusesEverySeatWhenTheCatalogIsMissing` | the missing-port end: no panic, the empty id stays valid, a named seat is refused |
| `kernel/controlplane`: `TestTheSeatValidatorAsksTheCatalog` | the other end: a catalog that **is** there is asked, not assumed |
| `kernel/app/seats`: `TestTheServiceRefusesWhenThereIsNoCatalog` | the service says "not available" on all three operations instead of panicking |
| `cmd/agezt`: `TestOpenAppKernelWiresTheSeatCatalog` | the daemon end: the composition root's catalog is live and knows the built-ins |

### The mutation run

**7 mutations: 7 caught, 0 survived, 0 skipped, 0 build errors.**

| # | Mutation | Caught by |
|---|---|---|
| 1 | the kernel opens the seat store again (the module edge returns) | **archcheck** — `cross-module kernel/runtime -> kernel/modules/work/seat` |
| 2 | the kernel **wraps** the port it was given | `TestTheKernelHoldsTheSeatPortItWasGiven` — the wrapper is not the port |
| 3 | the daemon stops injecting the seat catalog | `TestOpenAppKernelWiresTheSeatCatalog` |
| 4 | the app service stops refusing when there is no catalog | `TestTheServiceRefusesWhenThereIsNoCatalog` (and the service tests) |
| 5 | the missing catalog refuses the **empty** seat id | `TestTheSeatValidatorRefusesEverySeatWhenTheCatalogIsMissing` (and two workboard tests) |
| 6 | the seat row drifts (the wire tag changes) | `TestTheSeatRowIsTheContractRow` |
| 7 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |

**Two of the seven are gate-measured.** The claim of this slice is
architectural — *"the kernel's reach is smaller"* — and that is not something a
test can see: a test would happily accept a kernel that opens the seat store
itself, as long as the store works. Only the ratchet counts the edge.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 310 packages placed; 63 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 66** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rules the slice is really about

**A row that the contract already publishes is named from the contract.**
The proof and the assure edges were not "refactors" — they were the kernel
naming a module for a type that the module itself named the contract's. Two
spellings of one row is how a reach comes back.

**A module holds ports; the composition root holds the files.**
The kernel used to open nineteen stores. Each one it opens is an edge it pays
for. Moving a store out of the kernel's `Open()` is not a refactor either —
it moves the *ownership* of a file to the layer that owns files, and the
kernel keeps the port.

**A missing port is a refusal, not a nil dereference.**
A port may be absent (the daemon did not hand one over). The nil port has to
be visible as nil, and every holder of it has to say "not available" — and to
say it the way the port itself would have said it.

## What remains

The kernel still reaches **19** modules. The next candidates are the ones
whose rows are already in the contract (`workflow`, `workboard`, `toolforge`,
`artifact`, `datalake` all have contract ports today) and whose kernel-side
use is a handful of verbs — and the three media tool seams
(`imagetool`, `reranktool`, `voicetool`), which are blocked behind the
artifact store: the voice and image tools need the artifact store, so the
daemon has to own that store before the daemon can own those tools.
