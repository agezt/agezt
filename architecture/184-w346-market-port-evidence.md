# W3.46 — the marketplace is the daemon's, the kernel holds the port

## What the slice is

The kernel still reached **17** modules after W3.45. This slice takes **one**
of those edges — and it is the last edge of the *market* family:

| Edge | Why it existed |
|---|---|
| `kernel/runtime -> kernel/market` | the kernel held the concrete `*market.Manager`, built the `market` agent tool itself, and named the module's rows |

The ratchet went **54 → 53**.

## What the kernel used to do

It held the module's concrete type:

```go
// kernel/runtime/runtime_kernel.go
marketMgr *market.Manager
// kernel/runtime/accessors.go
func (k *Kernel) Market() *market.Manager { return k.marketMgr }
func (k *Kernel) SetMarket(m *market.Manager) { k.marketMgr = m }
```

and it built the **tool** — the one thing an agent actually uses:

```go
// kernel/runtime/compose.go
var mktTool *market.Tool
if cfg.MarketTool {
    mktTool = market.NewTool()
    effTools["market"] = mktTool
}
// …and after the kernel was built:
mktTool.Manager = k.Market
```

The rows the port carries (`Listing`, `InstalledPack`, `Event`, `Source`,
`SyncResult`) were the module's own structs, and the app layer
(`kernel/app/market`) named them through the module's `core` import.

## What it does now

### The port is the contract's

```go
// kernel/contract/marketapi/manager.go
type Manager interface {
    List(query string) ([]Listing, error)
    Show(marketplace, name string) (Pack, InstalledPack, bool, error)
    Sources() ([]Source, error)
    InstallContext(ctx context.Context, corr, marketplace, name, version string, emit func(Event) error) (InstalledPack, error)
    UninstallContext(ctx context.Context, corr, name string, emit func(Event) error) error
    AddSource(name, rawURL, pubKey string) (Source, error)
    RemoveSource(name string) (bool, error)
    Sync(ctx context.Context, name string) ([]SyncResult, error)
}
```

**Eight** verbs, and the width is not a guess: the control plane reads the
catalogue and the sources (`List`, `Show`, `Sources`) and drives the lifecycle
(`InstallContext`, `UninstallContext`, `AddSource`, `RemoveSource`, `Sync`).
The module answers it with a compile-time assertion, so a port change breaks
in the module that implements it:

```go
// kernel/market/contract_type_test.go
var _ marketapi.Manager = (*Manager)(nil)
```

### The rows moved to the contract, the module kept its names

The five rows the port carries are contract rows now, and the module keeps
every name it had as an **alias** — one row, one spelling:

```go
// kernel/market/market.go
type InstalledPack = marketapi.InstalledPack
type Event = marketapi.Event
// kernel/market/manager_ops.go
type Listing = marketapi.Listing
// kernel/market/sources.go
type Source = marketapi.Source
// kernel/market/sync.go
type SyncResult = marketapi.SyncResult
```

### The tool is the daemon's

The tool drives the **contract's** port, so it no longer needs the module's
manager at all:

```go
// kernel/market/tool.go
type Tool struct {
    Manager func() marketapi.Manager
}
```

And the **composition root** builds it and hands it over like any other tool
— the same shape W3.44 gave the media tools:

```go
// cmd/agezt/main_toolinvoker.go
mktTool := market.NewTool()
cfg.Tools = withMarketTool(cfg.Tools, mktTool)
k, err := kernelruntime.Open(cfg)
if err != nil {
    return nil, err
}
mktTool.Manager = k.Market
```

The `MarketTool` knob is **gone** from `runtime.Config`: the kernel no longer
builds a tool, so a knob that says "build the tool" would be a lie. The
kernel's `compose.go` no longer imports the module at all.

### The tenant must not borrow the primary's marketplace

This is the part the slice had to be careful about. The primary kernel and
every tenant share **one** tool map (`tcfg := cfg` copies the struct, not the
map), so a tool written in place — or a single tool instance handed to both —
would give the tenant the **primary's** manager, and with it the primary's
Forge and MCP registry to install packs into.

The rule the code follows: **every kernel gets its own tool, bound to its own
kernel's port.** `withMarketTool` copies the map, and the tool resolves the
kernel's port lazily — which is also why it can be bound *after* `Open` (the
manager can only be built once the kernel exists, because the built-in
catalogue installs into the kernel's own Forge and MCP).

A tenant's kernel never gets a manager, so its tool reports the marketplace as
unavailable — exactly what it did before the slice.

### A missing marketplace is an answer, not a panic

The kernel used to reach into the module, so `k.marketMgr` was never nil and
the code never had to say what happens when it is. Now a host may open a
kernel without a marketplace: the port is **nil**, and the tool says so:

```
marketplace is not available on this daemon
```

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/marketapi`: `TestTheManagerPortHasExactlyEightVerbs` | the port is **exactly** eight verbs wide — a ninth would be a change nobody chose |
| `kernel/contract/marketapi`: `TestTheManagerRowsKeepTheirWireShape` | the five moved rows keep their wire key sets — a renamed key is a record nobody can read back |
| `kernel/market`: `TestTheManagerIsTheContractPort` | the module answers the contract's port — a port change is a compile error in the module |
| `kernel/market`: `TestTheRowsAreTheContractRows` | the five rows are the **contract's** rows, not copies of them |
| `kernel/market`: `TestTheToolDrivesThePortItWasGiven` | the tool passes the agent's **query**, **pack**, **marketplace** and **correlation** through to the port |
| `kernel/runtime`: `TestTheKernelHoldsTheMarketPortNotTheManager` | the kernel holds **nothing** until the daemon wires one — a nil port, not a panic |
| `cmd/agezt`: `TestOpenAppKernelWiresTheMarketTool` | the daemon end: the tool is live, it resolves the **kernel's** port, and the caller's tool map comes back untouched |
| `cmd/agezt`: `TestTheTenantDoesNotBorrowThePrimarysMarketplace` | every kernel gets its **own** tool, bound to its own port — the tenant finds no marketplace |
| `kernel/runtime`: `capability_guard_test` | the market tool is a **host** tool now, and it still resolves to the capability Edict governs |

### The mutation run

**8 mutations: 8 caught, 0 survived, 0 skipped, 0 build errors.**

| # | Mutation | Caught by |
|---|---|---|
| 1 | the kernel's field names the module's manager again | **archcheck** — `cross-module kernel/runtime -> kernel/market` |
| 2 | the kernel builds the market tool again | **archcheck** — the same edge |
| 3 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |
| 4 | the daemon does not hand the market tool to the kernel | `TestOpenAppKernelWiresTheMarketTool` |
| 5 | the daemon writes into the caller's tool map (both copies go) | `TestOpenAppKernelWiresTheMarketTool` |
| 6 | one tool instance for every kernel | `TestTheTenantDoesNotBorrowThePrimarysMarketplace` |
| 7 | the kernel drops the marketplace it was given | `TestOpenAppKernelWiresTheMarketTool` |
| 8 | the tool installs an empty pack name | `TestTheToolDrivesThePortItWasGiven` |

**Three of the eight are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that opens
the marketplace itself, as long as the marketplace works.

The run needed **two** passes, and the first one was not honest in either
direction:

- Mutation 5 (the caller's map) **survived** the first pass. The daemon
  copies the tool map **twice** — once in `mediaTools`, once in
  `withMarketTool` — so removing one copy is not observable: the other one
  still shields the caller. The mutation now removes both, which is what
  "the daemon writes into the caller's map" actually means.
- Mutation 6 (one tool instance for every kernel) was a **BUILDERR** in the
  first pass: the mutation left the `kernel/market` import unused in
  `main_toolinvoker.go`, and a compile error is not a catch. The mutation now
  removes the import it orphans.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 311 packages placed; 53 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 54** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rule the slice is really about

**A tool that outlives the module it was built from has to be re-homed.** The
`market` tool used to be the kernel's, because the kernel built it out of the
market module's own manager. The moment the manager became a **port**, the
tool had to become the **daemon's** too: a tool is a thing a host hands over,
and the daemon is the host. The knob that said "the kernel builds the market
tool" went away with the build.

**A port's holder is not its owner.** The kernel holds the marketplace port
because the control plane reaches the marketplace *through the kernel* — but
the manager behind that port is the daemon's, and so is the tool. Three
owners, one port, and the kernel only ever holds the port.

**Isolation is a property of the wiring, not of the code that reads it.** The
primary and the tenants share one tool map. Nothing in the tool's own code
says "this is the tenant's tool" — the isolation comes from each kernel
building its **own** tool over its **own** port. That is why the tenant test
exists at all: it is the only place where "one shared instance" is visible.

## What remains

The kernel still reaches **16** modules. The next candidates, in the order the
measurement suggests:

- **`workflow` and `workboard`** — both need wide new ports: the kernel names
  `workflow.Engine`, `workflow.Ports`, `workflow.Provenance` and
  `workflow.Trigger` (no contract home), and it calls **seventeen** workboard
  store methods.
- **`cadence`, `standing`, `mcp`** — many callers each; the shape is known (a
  contract port the host opens), the work is in the port's width.
- **`memory`, `worldmodel`, `roster`, `skill`, `okr`, `taste`, `catalog`** — the
  kernel holds each module's concrete store. Each one is the same slice: a
  contract port, the daemon opening the file, and the kernel's accessor
  carrying the port.
