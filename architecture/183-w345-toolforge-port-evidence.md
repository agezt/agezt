# W3.45 — the script-tool store is the daemon's file

## What the slice is

The kernel still reached **17** modules after W3.44. This slice takes **one**
of those edges — and it is the last edge of the *toolforge* family:

| Edge | Why it existed |
|---|---|
| `kernel/runtime -> kernel/modules/extensions/toolforge` | the kernel opened the script-tool store, held the concrete type, and called eight methods on it |

The ratchet went **55 → 54**.

## What the kernel used to do

```go
// kernel/runtime/compose.go
tfstore, err := toolforge.Open(filepath.Join(cfg.BaseDir, "toolforge"))
if err != nil {
    return fail("runtime: toolforge", err)
}
```

and it held the module's type:

```go
// kernel/runtime/runtime_kernel.go
toolForge *toolforge.Store
// kernel/runtime/scripttool.go
func (k *Kernel) ToolForge() *toolforge.Store { return k.toolForge }
```

The kernel called **eight** store methods (`Add`, `Update`, `Get`, `RecordTest`,
`Promote`, `Quarantine`, `Remove`, `Active`) and named the module's rows
(`toolforge.ScriptTool`, `toolforge.StatusActive`) and its sentinels
(`toolforge.ErrNotFound`, `toolforge.ErrUntested`).

## What it does now

### The store is the daemon's file

The kernel holds the **port** and the composition root owns the **file** — the
shape W3.41 (seat), W3.42 (lake) and W3.43 (artifacts) established:

```go
// cmd/agezt/main_toolinvoker.go
if cfg.ToolForge == nil {
    store, err := toolforge.Open(filepath.Join(cfg.BaseDir, "toolforge"))
    if err != nil {
        return nil, err
    }
    cfg.ToolForge = store
}
```

A store that cannot be opened is a **startup error** — now the daemon's to
raise, exactly like the lake's and the artifact store's.

### The port had to be split, and the contract already said so

The contract's `toolforgeapi.Store` had **two** verbs (`Get`, `List`) and a
guard test that said why:

> the port must have exactly Get and List

A plugin holds every method of the port it is given — and one of the host's
verbs is `Promote`: a plugin that could promote would be able to put its own
code in front of the operator's agent. So the port is **two** ports, the same
split W3.43 made for the artifact index:

| Port | Verbs | Who holds it |
|---|---|---|
| `toolforgeapi.Store` | `Get`, `List` | the tool registry — the plugin that lists the catalogue |
| `toolforgeapi.HostStore` | those two **plus** `Add`, `Update`, `RecordTest`, `Promote`, `Quarantine`, `Remove`, `Active`, `Count` | the host — the kernel and the control plane |

The module answers both:

```go
// kernel/modules/extensions/toolforge/toolforge_store.go
var _ toolforgeapi.Store = (*Store)(nil)
var _ toolforgeapi.HostStore = (*Store)(nil)
```

### The sentinels became rows

`ErrNotFound` and `ErrUntested` are the **answers** the host gives: "no such
tool" and "the current code has no passing test". A host may not name a
module's sentinel, so they moved to the contract — and the module keeps its
names as aliases:

```go
// kernel/modules/extensions/toolforge/toolforge.go
var ErrNotFound = toolforgeapi.ErrNotFound
var ErrUntested = toolforgeapi.ErrUntested
```

### The runner seam found its home

The kernel's config carried `ScriptRunner toolforge.Runner` — the sandbox
executor. That method set is the registry's own contract row
(`toolregapi.ScriptRunner`, from W3.38), so the module's name became an alias
of it and the kernel names the contract:

```go
// kernel/modules/extensions/toolforge/toolforge.go
type Runner = toolregapi.ScriptRunner
```

### The app layer speaks the contract too

`kernel/app/tools`'s forge catalogue named `toolforge.ScriptTool` — the
module's alias of the contract's row. It now names the contract's row
directly, and the app layer no longer imports the module at all.

### A missing store is a refusal, not a panic

This is the part the slice had to teach the kernel. The store used to be
guaranteed: the kernel opened it. Now a host may open a kernel without one —
and the first version of the kernel **panicked** on `k.toolForge.Add(st)`. The
tests caught it, and every forge operation now refuses:

```go
// kernel/runtime/scripttool.go
var errNoForge = errors.New("runtime: the script-tool forge is not available on this daemon (no forge store)")

func (k *Kernel) scriptForge() (toolforgeapi.HostStore, error) {
    if k.toolForge == nil {
        return nil, errNoForge
    }
    return k.toolForge, nil
}
```

The app layer learned the same rule: the forge catalogue answers "the forge
is not available on this daemon" instead of dereferencing a nil reader.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/toolforgeapi`: `TestTheStorePortShape` | the tool-facing port stays **exactly** two verbs wide (the pre-existing guard, kept green) |
| `kernel/contract/toolforgeapi`: `TestTheHostPortCarriesTheLifecycleVerbs` | the host port is the tool port plus **exactly** the ten verbs it uses |
| `cmd/agezt`: `TestOpenAppKernelWiresTheForge` | the daemon end: the port is live, and it is the **live store** — a tool drafted through the kernel shows up in the catalogue |
| `cmd/agezt`: `TestForgeStoreStaysANilPortWithoutAStore` | a kernel with no injected store holds a **nil** port, and the daemon hands that nil to the tool registry |
| `cmd/agezt`: `TestToolKernelDepsSuppliesTheForgePort` | the tool registry's port answers the live store |
| `kernel/runtime`: `TestTheForgeRefusesWithoutAStore` | every forge operation **refuses** without a store — none of them panics |
| `kernel/modules/extensions/toolforge`: `var _ toolforgeapi.HostStore` | the module answers **both** ports — a port change is a compile error in the module |

### The mutation run

**8 mutations: 8 caught, 0 survived, 0 skipped, 0 build errors.**

| # | Mutation | Caught by |
|---|---|---|
| 1 | the kernel opens the store again | **archcheck** — `cross-module kernel/runtime -> kernel/modules/extensions/toolforge` |
| 2 | the kernel's field names the module's concrete store | **archcheck** — the same edge |
| 3 | the kernel's config names the module's runner seam | **archcheck** — the same edge |
| 4 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |
| 5 | the daemon opens the store but does not inject it | `TestOpenAppKernelWiresTheForge` |
| 6 | the kernel's "no forge" guard is inverted (the panic returns) | `TestTheForgeRefusesWithoutAStore` |
| 7 | the tool-facing store port grows `Promote` | `TestTheStorePortShape` |
| 8 | the tool registry is handed a nil forge port | `TestToolKernelDepsSuppliesTheForgePort` |

**Four of the eight are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that opens
the store itself, as long as the store works.

The run was honest on the first pass — the harness was already fixed from
W3.43's two false starts (the package and the `-run` filter are separate
arguments, and a build error is not a catch).

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 311 packages placed; 54 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 55** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rule the slice is really about

**A store the host opens is a store the host owns — and the kernel must be
able to live without one.** The kernel used to open the store itself, so
`k.toolForge` was never nil and the code never had to say what happens when
it is. Moving the file out of the kernel created that question, and the
answer is the same everywhere in this codebase: a missing port is an
**answer**, not a panic. The first version of the kernel panicked, the tests
caught it, and every forge operation now refuses with a plain message.

**A port's width is decided by the weakest holder.** The contract already
said the store's port is two verbs wide, and said why: a plugin holds every
method of the port it is given, and `Promote` is not a verb a plugin may
have. The host's eight verbs went on a **second** port rather than widening
the first — the same split W3.43 made for the artifact index, and the reason
the guard test in the contract was already there to be broken.

**The daemon is where the file lives.** Four stores have moved now — seats,
the lake, the artifacts, the script tools — and the shape never changed: the
daemon opens the file, the kernel holds the port, and a file that cannot be
opened is the daemon's startup error.

## What remains

The kernel still reaches **17** modules. The next candidates, in the order the
measurement suggests:

- **`market`** — the manager is already the daemon's (the daemon creates it and
  injects it with `SetMarket`), but the kernel builds the **market tool** and
  holds the concrete `*market.Manager`. The app layer already speaks two narrow
  interfaces (a three-verb reader and a six-verb writer), so a host-facing
  contract port is a small piece of work.
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
