# W3.58 — the registry is the daemon's

## What the slice is

After W3.57 the ratchet stood at **18/13** and the allowlist still named the MCP
module from two places. This slice takes **both** of those edges, and it takes
them **completely**:

| Edge | Why it existed |
|---|---|
| `cross-module kernel/runtime -> kernel/modules/extensions/mcp` | the kernel held the module's concrete `*mcp.Store` and **opened the file** itself |
| `cross-module kernel/market -> kernel/modules/extensions/mcp` | the marketplace named the module's rows |

The ratchet went **18 → 16**. The **13** allowlisted call sites are unchanged.
The kernel still reaches **3** `kernel/modules/...` packages.

This is the **fourth** module of the "the file is the daemon's" family, and the
**first one whose implementation also holds a live client**. The registry is a
file the daemon opens; the **client** is a child process the kernel spawns and a
remote endpoint it speaks to. Both had to become something the kernel could
hold without naming the module.

## What the kernel used to do

```go
// kernel/runtime/compose.go
mcpstore, err := mcp.OpenStore(filepath.Join(cfg.BaseDir, "mcp"))
if err != nil {
    return fail("runtime: mcp", err)
}
```

The kernel opened the registry's file, held the module's **concrete** store, and
the marketplace had to name the module's row to register a pack's server. The
kernel's **bridge** held live attachments as the module's `mcp.Conn`, and its
**dial** was a direct call into the module's `mcp.Dial` / `mcp.DialHTTP` — a
fallback that silently coupled the kernel to the module's clients.

## What it does now

### The contract carries the rows, the ports and the sentinel

The MCP contract `kernel/contract/mcpapi` already existed — the agent-facing MCP
tool is a **plugin**, and a plugin may reach the contract and the platform
layers and nothing else. It carried the `Server` row, its `Validate` rule and
the `Runner` port (the surface the plugin drives). What was missing was
everything the **registry** and the **client** needed.

```go
// kernel/contract/mcpapi/ports.go
type ToolDef struct {           // the row one server advertises
    Name        string
    Description string
    InputSchema json.RawMessage
}

type Conn interface {           // three verbs — a live attachment
    Tools() []ToolDef
    Call(ctx context.Context, tool string, args json.RawMessage) (string, bool, error)
    Close() error
}

type Dialer func(ctx context.Context, command string, args []string, env map[string]string) (Conn, error)
type HTTPDialer func(ctx context.Context, url string, headers map[string]string) (Conn, error)

var ErrNotFound = errors.New("mcp: server not found")

type Registry interface {       // six verbs — the store's whole surface
    Add(srv Server) (Server, error)
    SetEnabled(ref string, enabled bool) (Server, error)
    Remove(ref string) (Server, bool, error)
    Get(ref string) (Server, bool)
    List() []Server
    Count() int
}

type RegistrationReader interface {   // one verb — the catalog's half
    List() []Server
}
```

**The port is SPLIT, not widened.** The registry's six verbs are the store's
whole surface and no more: the store validates, assigns ids and timestamps and
keeps its own on-disk shape, and a caller that could add a server without
those rules would bypass them. But the **catalog** layer only lists — it
projects the rows for the operator — so it takes the **one-verb**
`RegistrationReader` instead of the whole registry.

**The dialers are SEAMS, not ports.** `Dialer` and `HTTPDialer` are function
types: the implementations live in the module (`mcp.Dial`, `mcp.DialHTTP`) and
the **kernel may not name them**. The daemon injects them, the way it injects
every other host-side seam. This is the one place where the old code had a
*fallback* — the kernel called `mcp.Dial` directly when no dialer was
configured — and the fallback is exactly the edge the slice removes.

**The sentinel moved to the contract.** `ErrNotFound` is what the app layer
compares against (it turns "not found" into "unknown mcp server"), and a module
that raised a look-alike of its own would leave every `errors.Is` comparison
silently false. The module keeps the name as an **alias**.

**The shapes are aliases, not copies.** `ToolDef`, `Conn`, `Dialer` and
`HTTPDialer` live in the contract and the module keeps each name as an
**alias**. A copy with the same fields would satisfy every assignment and
still be a second shape: a `Conn` that is not the contract's `Conn` cannot be
handed to a caller that holds the contract's port, and a `ToolDef` that is not
the contract's `ToolDef` cannot sit in the slice the contract's `Conn` returns.

### The daemon opens the file and injects the seams

```go
// cmd/agezt/main_toolinvoker.go
if cfg.MCPStore == nil {
    store, err := mcp.OpenStore(filepath.Join(cfg.BaseDir, "mcp"))
    if err != nil {
        return nil, err
    }
    cfg.MCPStore = store
}
if cfg.MCPDialer == nil {
    cfg.MCPDialer = mcp.Dial
}
if cfg.MCPHTTPDialer == nil {
    cfg.MCPHTTPDialer = mcp.DialHTTP
}
```

A registry that cannot be opened is a **startup error**: the registry is part of
the daemon, not a convenience. The file lives under the **base dir**, so a
tenant's kernel keeps its own registry.

The module is **not bus-bound** — the same shape as the roster (W3.56). The
lifecycle journaling lives one layer up (the kernel presses the journal), so
there is **no publisher port and no builder to inject**. The daemon opens the
file and hands over the port, and that is the whole handover.

### The nil-port surface lives in the kernel, and only there

A kernel that was handed no registry is a daemon with no MCP servers. The
answers are stated once, in the kernel:

- the **reads** answer "no servers" — `MCPRegistrations` is empty,
  `MCPRegistrationCount` is 0, `MCPRegistration` finds nothing, and
  `MCPReader` hands the catalog an empty list;
- the **writes refuse** — `AddMCPServer`, `SetMCPServerEnabled`,
  `AttachMCPServer` and `RemoveMCPServer` answer `ErrNoMCPRegistry` rather
  than calling into nothing;
- the **dialer seam refuses** — a kernel with no dialer answers "no stdio MCP
  dialer is configured" (or the http twin) rather than calling a nil func
  value, which would panic at the call.

**The raw port is a nil-able thing, and the catalog does not touch it.** The
control plane used to hand the catalog `kernel.MCPStore()` — the raw port —
and call `List()` on it. With no registry that is a nil interface and a panic
at the first read. The catalog now reads through `MCPReader()`, the kernel's
nil-safe reader, so the one nil-port surface in the system is the kernel's.

### Every test became a host

Four test packages opened a kernel and expected a registry to be there:
`cmd/agezt`, `kernel/controlplane`, `kernel/runtime` and
`plugins/builtintools`. Each became a **host**: it opens the registry itself
and injects it, the same way the daemon does.

```go
// kernel/runtime/mcp_host_test.go
func withMCP(t *testing.T, cfg runtime.Config) runtime.Config {
    t.Helper()
    if cfg.MCPStore != nil {
        return cfg
    }
    base := cfg.BaseDir
    if base == "" {
        base = t.TempDir()
    }
    registry, err := testfixtures.MCPRegistry(base)
    if err != nil {
        t.Fatalf("open the MCP registry: %v", err)
    }
    cfg.MCPStore = registry
    return cfg
}
```

The store is opened **once per config**: a test that reopens the same config — a
**restart** — keeps the same registry, so a server registered before the
restart is still there after it. That is the whole point of the file.

`cmd/agezt` and `plugins/builtintools` are **outside** `kernel/`, so the
`kernel/internal` rule keeps them from the shared fixtures; they carry their
own local helper that names the module — the daemon is the composition root,
and the composition root names the module.

## The proof

### Permanent tests

**18 new test functions** across five packages, plus the module's **four
compile-time identity assertions** (a permanent check that fails the build
rather than a test that fails the run). The table lists **19 rows**: the
eighteen functions and the one row that holds the four assertions.

| Test | What it holds |
|---|---|
| `kernel/contract/mcpapi`: `TestRegistryPortCarriesTheWholeRegistryAndNothingMore` | the registry port is **exactly six** verbs |
| `kernel/contract/mcpapi`: `TestRegistrationReaderPortListsAndNothingElse` | the catalog's half is **one** verb |
| `kernel/contract/mcpapi`: `TestConnPortIsAToolsCallCloseTriple` | a live attachment is **exactly three** verbs |
| `kernel/contract/mcpapi`: `TestDialerSeamsCarryTheTransportSignatures` | the two seams carry the **transport** signatures (stdio takes the process, http takes the URL) |
| `kernel/contract/mcpapi`: `TestErrNotFoundIsTheContractSentinel` | the sentinel lives in the contract, and a **look-alike with the same message does not match it** |
| `kernel/contract/mcpapi`: `TestToolDefIsTheAdvertisedToolRow` | the advertised tool's shape and its wire form |
| `kernel/modules/extensions/mcp`: `TestTheShapesAreTheContractsShapes` | the module's `ToolDef`, `Conn`, `Dialer` and `HTTPDialer` **are** the contract's — by **type identity** |
| `kernel/modules/extensions/mcp`: `TestErrNotFoundIsTheContractsSentinel` | the module's alias **is** the contract's sentinel value |
| `kernel/modules/extensions/mcp`: `TestStoreIsTheContractsRegistry` | the store answers the contract's port, sentinel included |
| `kernel/modules/extensions/mcp`: the compile-time assertions | the shapes are the contract's, at compile time |
| `kernel/runtime`: `TestTheKernelHoldsTheMCPRegistryContractsPort` | the kernel's face is **statically** the contract's port (the declared return type is compared) |
| `kernel/runtime`: `TestKernelNeverOpensARegistryOfItsOwn` | the kernel opens **no** registry of its own — no directory, no file |
| `kernel/runtime`: `TestKernelWithoutARegistryAnswersNoServersAndRefusesWrites` | the reads say "no servers" and the writes **refuse** — no panic, no swallowed write |
| `kernel/runtime`: `TestKernelHoldsTheRegistryItWasGiven` | the injected registry is the kernel's registry |
| `kernel/runtime`: `TestAttachWithoutADialerRefusesInsteadOfPanicking` | a kernel with no dialer **refuses** the attach with a readable error |
| `kernel/app/tools`: `TestMCPCatalogWithNoReaderAnswersNoServers` | a catalog with no reader answers "no servers", not a crash |
| `cmd/agezt`: `TestOpenAppKernelWiresTheMCPRegistry` | the daemon end: the registry is a **file on disk** and the port is live |
| `cmd/agezt`: `TestTheMCPRegistryIsTheInjectedOne` | the daemon uses the **injected** registry, and opens no second one |
| `cmd/agezt`: `TestTheDaemonWiresTheDialers` | the daemon's attach really reaches the **spawn** — without a dialer, no server could ever be attached |

### The mutation run

**13 mutations: 13 caught, 0 survived, 0 skipped, 0 build errors** — in **two**
independent passes, both honest (every catch named the expected test or the
expected edge).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the kernel names the mcp module again | **archcheck** — `cross-module kernel/runtime -> kernel/modules/extensions/mcp` |
| 2 | the marketplace names the mcp module again | **archcheck** — `cross-module kernel/market -> kernel/modules/extensions/mcp` |
| 3 | the kernel opens the registry's file behind the host's back | `TestKernelNeverOpensARegistryOfItsOwn` |
| 4 | the kernel ignores the registry it was handed | `TestKernelHoldsTheRegistryItWasGiven` |
| 5 | the kernel's registry face is an anonymous shape, not the contract's port | `TestTheKernelHoldsTheMCPRegistryContractsPort` |
| 6 | the contract's registry port grows a seventh verb | `TestRegistryPortCarriesTheWholeRegistryAndNothingMore` |
| 7 | **the module keeps a sentinel of its own instead of the contract's** | `TestErrNotFoundIsTheContractsSentinel` |
| 8 | **the module keeps a shape of its own instead of the contract's** | `TestTheShapesAreTheContractsShapes` |
| 9 | **the kernel calls into nothing when no dialer is configured** | `TestAttachWithoutADialerRefusesInsteadOfPanicking` |
| 10 | **the kernel's writes call into nothing without a registry** | `TestKernelWithoutARegistryAnswersNoServersAndRefusesWrites` |
| 11 | the daemon does not hand the registry to the kernel | `TestOpenAppKernelWiresTheMCPRegistry` |
| 12 | the daemon does not wire the dialers | `TestTheDaemonWiresTheDialers` |
| 13 | the catalog stops reading the registry | `TestMCPCatalogListKeepsStoreOrderFreshRowStatusAndInitialAttachedCount` |

**Two of the thirteen are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that opens
the registry itself, as long as the servers still show up.

**Mutations 7, 8, 9 and 10 measure behaviours, not shapes:**

- the **sentinel** is what the layers above the module **compare against**. A
  module that kept a look-alike error of its own would leave every `errors.Is`
  comparison silently false, and the app would report "unknown mcp server" as
  some other failure;
- the **shapes** are what the layers above the module **hold**. A copy with the
  same fields satisfies every assignment and still is a second shape — and a
  second shape drifts on its own;
- the **dialer refusal** is the difference between a readable error and a
  panic. A nil func value does not fail when it is called; it crashes the
  daemon;
- the **write refusal** is the difference between an honest answer and a
  panic. "There is no registry" and "the write did nothing" are different
  answers, and only the first is true.

**Mutation 8 is the one that needed a compile-time-safe form.** The first
attempt replaced the module's `ToolDef` **alias** with a look-alike struct — but
the module's own client returns that row, so the whole package stopped
compiling. A mutation that breaks the build measures nothing. The mutation was
replaced with one on the `Dialer` **seam**, which has the same promise (the
module's shape is the contract's) in a form that compiles and can be observed:
a named func type with the same underlying shape is still a **different type**.

**Mutation 9 needed the guard to be found, not just changed.** The kernel has
**two** dialer guards with the same text — the http one and the stdio one — so
the harness had to learn to replace the first line matching a pattern *after* an
anchor. Replacing the first occurrence alone would have measured the http seam
and said nothing about the stdio one.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 315 packages placed; 16 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 18** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rules the slice is really about

**A client that lives behind a module boundary becomes a seam, not a fallback.**
The old kernel called `mcp.Dial` when no dialer was configured — a *fallback*,
which is a dependency with an escape hatch. The fallback is what kept the edge
alive. The seam (`mcpapi.Dialer`) has no fallback: the daemon injects it, and a
kernel without one **says so**.

**A nil-able port is not a surface — it is a trap with a name.** The raw
`MCPStore()` port is `nil` when the daemon handed over no registry, and the
control plane used to call `List()` on it. The fix was not a `nil` check at the
call site; it was a **nil-safe reader** (`MCPReader`) that the kernel owns, so
that the one place that knows about the absence is the kernel.

**The registry's file is the daemon's; the registry's PORT is the contract's.**
The two are different claims and the slice had to make both. The file belongs to
the composition root (the daemon opens `mcp/servers.json`); the port belongs to
the contract (the layers above the module speak it). The module keeps the
implementation and the names.

**A port is a promise about what a reader may do — and here it was two
promises.** The lifecycle drives the six verbs; the catalog only lists. Handing
the catalog the whole registry would have given it the lifecycle it has no
business driving — and the catalog is the layer that renders the operator's
list, so its width is the one an operator can see.
