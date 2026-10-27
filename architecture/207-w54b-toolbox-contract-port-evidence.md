# W5.4b — the toolbox reaches the control plane through the contract's port

## What the slice is

W5.4 asks for two ratchets at zero: the **call-site** one (done in W5.4a) and
the **import** one. This slice takes the first of the seven import edges — and
it takes the smallest of them, but the smallest one turned out to be the
clearest statement of the rule the whole ratchet is about.

**The edge:** `adapter-bypass kernel/controlplane -> kernel/modules/extensions/toolbox`.

The control plane is an **adapter** (layer 5). The toolbox is a **module**
(layer 3). An adapter reaches the app layer's typed operations, never a module
(archcheck's principle P2) — so the control plane's import of the toolbox was
debt.

It was small in code: `kernel/controlplane/toolbox.go` was twenty-four lines
and held two one-line adapters that forwarded to the module's package
functions. But the code was not the debt — the **shape** was. The control
plane's operations were built from the module's rows, and the module's
functions were called by the control plane's own file. The schema of a typed
operation was a module's implementation detail, and the adapter was the thing
that reached the module.

**The ratchet goes 7 → 6.**

## What the measurement said, and where it was half right

The measured map of the seven remaining edges (from the W5.4a evidence) put
this one first: *the smallest — three functions (`toolbox.Detect/Outdated/
Install`) plus the `toolbox.Inventory/InstallResult` rows; the control plane
wraps the module's native adapters, so it can move to a contract/api package.*

Two things in that map were half right, and the slice had to correct both.

**Half right #1 — "the control plane wraps the module's native adapters".**
It did not wrap them: it *implemented* them. `nativeToolboxReader` and
`nativeToolboxInstaller` were the control plane's own types whose methods
called the module's package functions. The wrapper was not a seam; it was a
second name for the module, written in the adapter.

**Half right #2 — "so it can move to a contract/api package".** Moving the
*adapters* would not have closed the edge: the adapter still had to call
`toolbox.Detect`, and the adapter still imported the module. The edge only
closes when the **implementation** comes from somewhere the adapter cannot
name — that is, from the composition root, through a port the contract owns.

That is the same shape as the board (W3.59): the daemon opens the module's
store and hands the control plane `boardapi.Store`. The daemon is the
composition root; the control plane is an adapter; the contract is what they
both speak.

## What changed

### One contract: `kernel/contract/toolboxapi`

The rows (`Inventory`, `ToolStatus`, `InstallResult`), the port (`Host`) and
the one answer a daemon without a toolbox owes its callers
(`ErrUnavailable`) live in the contract. The contract may import the standard
library and other contracts only (P4) — it does.

The module **re-exports the rows under its own name** as **aliases**:

```go
type Inventory = toolboxapi.Inventory
```

so there is one row and one spelling of it. An alias is not a copy: a copy
would fork the wire schema, and the operation's schema would stop matching the
row the daemon answers with. A permanent test holds that invariant by
comparing the *types*, not the values.

### The module implements the port

`kernel/modules/extensions/toolbox/host.go` adds `Host` — the module's
implementation of `toolboxapi.Host` — and `NewHost()`, the one call the
composition root makes. The work stays in the module (the catalog, the
probes, the package-manager call); only the *port* is shared.

### The daemon hands the port over

`cmd/agezt` — the composition root — calls `toolbox.NewHost()` and puts it in
`controlplane.Deps.Toolbox`. The control plane stores it in
`Server.toolboxHost` and the typed operations read it from there.

### The app layer speaks the contract, not the module

`kernel/app/tools` no longer imports the toolbox module **at all** — not even
in its tests. Its ports (`ToolboxReader`, `ToolboxInstaller`) and its
operations' schemas are the contract's rows.

### A daemon without a toolbox says so

This is the part that is easy to get wrong, and it is the part the permanent
tests exist for.

A missing port is **not** an empty answer. "The toolbox is not available on
this daemon" and "this machine has no tools" are different facts; if the first
is reported as the second, an operator reads a *daemon that cannot inspect the
machine* as a *machine with nothing on it*.

So the three operations answer `toolboxapi.ErrUnavailable` — and the install
refuses **before** it publishes `toolbox.install.requested`, because nothing
will be installed and nothing may be journaled as requested.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `TestTheModuleRowsAreTheContractRows` | the module's rows are the contract's rows — aliases, not copies |
| `TestTheModuleHostIsTheContractPort` | the module's host answers what the module's functions answer, and never runs a package manager for a tool that is not in the catalog |
| `TestToolboxWithoutAPortAnswersUnavailableAndPublishesNothing` | the app's reads and the install answer "unavailable", and the install publishes **nothing** |
| `TestToolboxInstallStillValidatesNamesBeforeThePort` | a malformed request is malformed on every daemon — the port's absence does not change the validation answer |
| `TestTheControlPlaneHoldsTheToolboxContractPort` | the control plane's setter and deps take the **contract's** port — asserted statically, because an anonymous interface with the same verbs would compile and would be a second shape of the same port |
| `TestToolboxOperationsWithoutAPortSaySoAndJournalNothing` | a daemon with no toolbox says so on both reads **and** the install, and journals no toolbox lifecycle event |
| `TestToolboxOperationsReachTheContractPort` | with the port wired, the operations answer with the **host's** rows — the machine is never consulted |
| `TestToolboxDepsCarryTheContractPort` | the daemon's deps reach the server: the wiring is proven at the daemon's door, not only through a setter |

### The mutation run

13 mutations, 13 CAUGHT, two independent runs with byte-identical logs.

| # | Mutation | Caught by |
|---|---|---|
| 1 | the daemon without a toolbox says something else | `TestToolboxOperationsWithoutAPortSaySoAndJournalNothing` |
| 2 | the detection answers an empty inventory instead of saying the toolbox is unavailable | `TestToolboxWithoutAPortAnswersUnavailableAndPublishesNothing` |
| 3 | the outdated read has no guard at all | `TestToolboxWithoutAPortAnswersUnavailableAndPublishesNothing` |
| 4 | the outdated read loses the count of the outdated tools | `TestToolboxOperationsReachTheContractPort` |
| 5 | the install has no guard at all | `TestToolboxWithoutAPortAnswersUnavailableAndPublishesNothing` |
| 6 | the install publishes "install requested" even when there is no port to install through | `TestToolboxWithoutAPortAnswersUnavailableAndPublishesNothing` |
| 7 | the detection answers an empty inventory **without** an error | `TestToolboxWithoutAPortAnswersUnavailableAndPublishesNothing` |
| 8 | the reads do not talk to the daemon's toolbox port | `TestToolboxOperationsReachTheContractPort` |
| 9 | the install does not talk to the daemon's toolbox port | `TestToolboxOperationsReachTheContractPort` |
| 10 | the setter throws the port away | `TestToolboxOperationsReachTheContractPort` |
| 11 | the daemon's deps never reach the server | `TestToolboxDepsCarryTheContractPort` |
| 12 | the module hands out no host at all | `TestTheModuleHostIsTheContractPort` |
| 13 | the module's host answers an empty inventory | `TestTheModuleHostIsTheContractPort` |

The first calibration run reported one **NO-TESTS** — mutation 4 was pointed
at the app package while the test that observes the count lives in the control
plane's. That is a defect of the mutation, not a gap in the tests: the
mutation was re-pointed, and the two reported runs are the clean ones.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 316 packages placed; 6 allowlisted import violations and 0 allowlisted call sites remain (ratchet)** — **import edges down from 7** |
| `go run ./tools/deadcodecheck` | OK — 31 public SDK findings and 1 cross-package test seams allowlisted |
| `go run ./tools/depscheck` | OK — 24 core dependencies, all justified |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## What the ratchet says now

Six import edges remain, and none of them is a "the adapter wrapped a module"
edge any more:

| Edge | What it needs |
|---|---|
| `adapter-bypass kernel/controlplane -> kernel/runtime` | the 37 verbs the control plane still takes from the kernel — a redesign, not a port |
| `adapter-bypass kernel/controlplane -> kernel/update` | the update service's port (`kernel/app/update` already exists) |
| `adapter-bypass kernel/restapi -> kernel/update` | the same port, on the REST door |
| `cross-module kernel/cadence/systemtasks -> kernel/runtime` | nine kernel accessors, two of which are platform types the contract may not name |
| `cross-module kernel/modules/agents/selfrepair -> kernel/runtime` | a runs port that does not exist yet |
| `upward kernel/channel -> kernel/bus` | 26 plugin configs holding `*bus.Bus` |

## The rules the slice is really about

**An adapter's wrapper is not a seam.** `nativeToolboxReader` looked like a
seam — a small type in the adapter, injected into the app layer. It was a
second name for the module: its methods called the module's functions, so the
adapter's import was the module's import wearing a hat. A seam is a seam when
the *implementation* can come from somewhere else.

**The rows belong to the contract when the schema is built from them.** The
operation's schema is a contract-level fact: it is what a client sees. A
schema built from a module's row makes the module part of the wire format, and
the only way out is a copy — which forks the row. An **alias** keeps one row
and one spelling of it.

**A missing port must not look like an empty answer.** The board's rule
(W3.59) again: a daemon with no board must not answer like a board with no
messages. The toolbox's rule is the same shape: a daemon with no toolbox must
not answer like a machine with no tools — and must not journal an install it
never ran.

**The composition root is where the implementation is named.** The daemon
opens the board's store, and now the toolbox's host, and hands over the
contract's port. The adapter is the layer that must not know.

**A port's shape is asserted, not assumed.** `TestTheControlPlaneHoldsTheToolboxContractPort`
compares the declared parameter type with the contract's interface: an
anonymous interface with the same verbs would compile, would work, and would
be a second shape of the same port, drifting from the contract on its own.
