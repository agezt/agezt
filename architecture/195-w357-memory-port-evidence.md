# W3.57 — the memory is the daemon's

## What the slice is

After W3.56 the ratchet stood at **24/13** and the allowlist still named the
memory module from six places. This slice takes **six** of those edges — the
widest slice of the W3.5x series — and it takes them **completely**:

| Edge | Why it existed |
|---|---|
| `cross-module kernel/runtime -> kernel/modules/knowledge/memory` | the kernel held the module's concrete `*memory.Manager` and **opened the file** itself |
| `cross-module kernel/runtime/runexec -> kernel/modules/knowledge/memory` | the run executor declared the module's concrete manager in its `KernelAPI` |
| `adapter-bypass kernel/controlplane -> kernel/modules/knowledge/memory` | the control plane named the module's rows |
| `adapter-bypass kernel/agentgw -> kernel/modules/knowledge/memory` | the agent gateway named the module's rows |
| `cross-module kernel/contextselect -> kernel/modules/knowledge/memory` | the context selector named the module's rows |
| `cross-module kernel/modules/knowledge/memory -> kernel/agent` | the module reached into the **runs** module for the structured-output path |

The ratchet went **24 → 18**. The **13** allowlisted call sites are unchanged.

This is the **third bus-bound module** (after the world model and the skill
forge) and the **first one that is also LLM-bound**: the manager distils facts,
brains and profiles through a model, and it embeds text through an embedder.
Both of those seams had to be **ports**, and both are now.

## What the kernel used to do

```go
// kernel/runtime/compose.go
memDir := filepath.Join(cfg.BaseDir, "memory")
mgr := memory.NewManager(memory.Open(memDir), cfg.Provider, ...)
```

The kernel opened the memory's file, held the module's **concrete** type, and
the run executor, the control plane, the agent gateway and the context selector
had to name that type or its rows as well. The module itself reached into the
**runs** module (`kernel/agent`) for the structured-output path — a module
naming another module.

## What it does now

### The contract carries the rows, the rules, the sentinels and the ports

`kernel/contract/memoryapi` is **new** — the memory had no contract before, and
that was the whole reason the layers above it had to name the module.

```go
// kernel/contract/memoryapi/ports.go
type Store interface {          // six verbs
    Put(r Record) error
    Get(id string) (Record, bool, error)
    Delete(id string) (bool, error)
    All() ([]Record, error)
    Count() int
    Close() error
}

type Publisher interface {       // one verb
    Publish(spec eventapi.Spec) (id string, err error)
}

type Reader interface {          // nine verbs
    Recall(corr, query string, limit int) ([]Scored, error)
    Search(query string, limit int) ([]Scored, error)
    RecallScoped(corr, query string, limit int, scope string) ([]Scored, error)
    SearchScoped(query string, limit int, scope string) ([]Scored, error)
    Active() ([]Record, error)
    Get(id string) (Record, bool, error)
    All() ([]Record, error)
    Count() int
    ProfileText() string
}

type Writer interface {          // five verbs
    Remember(corr string, spec RememberSpec) (Record, bool, error)
    Forget(corr, id string) (bool, error)
    Promote(corr, id string) (bool, error)
    Supersede(corr, oldID, newID string) (bool, error)
    Suspend(corr, id string, reason string) (bool, error)
}

type Maintainer interface {      // five verbs
    Prune(corr string, maxAgeMS int64) (int, error)
    Audit(corr string) (AuditReport, error)
    Hygiene(corr string) (HygieneStats, error)
    CleanLowValue(corr string, minScore float64) (CleanReport, error)
    DedupeDistilled(corr string) (int, error)
}

type Distiller interface {       // three verbs
    Distill(ctx context.Context, corr string, provider llm.Provider, req llm.CompletionRequest, transcript string) (BrainDistillReport, error)
    DistillBrain(ctx context.Context, corr string) (BrainDistillReport, error)
    DistillProfile(ctx context.Context, corr string) (ProfileReport, error)
}

type Tooler interface {          // one verb
    Tool() toolapi.Tool
}

type Manager interface {         // the composition of the halves
    Reader; Writer; Maintainer; Distiller; Tooler
}

type ReaderWriter interface { Reader; Writer }   // the gateway's surface
```

**The port is SPLIT, not widened — six times over.** A single wide port would
have handed the agent gateway the whole memory lifecycle (audit, prune,
distil). The gateway only looks notes up and writes them, so it takes the
**nine+five** `ReaderWriter`; the control plane drives the operations, so it
takes the `Manager`; the context selector only reads, so it takes the
`Reader`.

**The rules travel with the row.** `Record.Active()`, `Record.Suspended()`,
`Record.Expired(nowMS)` and `Record.Usable(nowMS)` decide whether a note still
counts, and a Go type alias **cannot carry methods** — had the rules stayed in
the module, the alias would have lost them and the layers above would have had
to reach back into the implementation to judge a record. So the rules moved
**with the row** into the contract, and the module's `Record` is an alias that
keeps them.

`ValidType` is the same story: the membership set is part of the type's
meaning, so it lives with the type.

**The sentinels moved to the contract.** `ErrRecordNotFound` and
`ErrEmptyContent` are what the layers above the module compare against; the
module keeps both as **aliases**, so `errors.Is` matches on **both** sides of
the module boundary.

**The structured-output sentinel moved too.** `llm.ErrNoObjectGenerated` is
now the contract's: the module asks "did the model produce an object?" and the
answer must be a value the layers above can compare against without naming the
runs module. `kernel/agent` keeps the name as an **alias**.

**The context seam lives in the contract.** `WithCorrelation` /
`CorrelationFrom` / `WithScope` / `ScopeFrom` moved to `memoryapi`: a context
key is an **identity**, and two packages that each define one would not see
each other's values. The module keeps the two **readers** (it uses them); the
**writers** are the contract's alone, because the layers above the module write
the seam and may not name the module to do it.

### The daemon opens the file and injects the seams

```go
// cmd/agezt/main_toolinvoker.go
if cfg.MemoryStore == nil {
    store, err := memory.Open(filepath.Join(cfg.BaseDir, "memory"))
    if err != nil {
        return nil, err
    }
    cfg.MemoryStore = store
}
if cfg.MemoryBuilder == nil {
    cfg.MemoryBuilder = func(deps memoryapi.Deps) memoryapi.Manager {
        return memory.NewManager(deps)
    }
}
if cfg.MemoryGenerator == nil {
    cfg.MemoryGenerator = agent.GenerateObject
}
```

```go
// kernel/runtime/compose.go
mgr := cfg.MemoryBuilder(memoryapi.Deps{
    Store:     cfg.MemoryStore,
    Publisher: memoryPublisher{b: k.bus},
    Embedder:  cfg.MemoryEmbedder,
    Generator: cfg.MemoryGenerator,
})
```

This is the **bus-bound pattern** the world-model slice (W3.54) established,
and it is the reason the module's `kernel/agent` edge could go: the
structured-output path is a **port** (`memoryapi.Generator`), injected by the
daemon — the module never names the runs module.

**The publisher answers with the event's id, not the envelope.** The durable
envelope carries the hash chain, and the contract layer may not name it. What
the manager needs is the **id** — that is the record's `SourceEvent`
provenance.

**`SetEmbedder` is deliberately OFF the port.** The embedder is a
live-configuration seam: it is given at construction time (`Deps.Embedder`),
not called on the port afterwards.

### A missing memory answers, and the answers differ by direction

The memory is the agent's own knowledge, so a kernel without one is not an
edge case — it is a daemon that has no memory yet. The answers are therefore
**different per direction**, and each one is stated once:

- the **reads** answer **"no memory"** — `MemoryCount` is 0, `MemoryActive` is
  empty, `MemoryProfileText` is empty, `MemorySearchScoped` and
  `MemoryRecallScoped` find nothing;
- the **writes refuse** — `MemoryForget`, `MemoryDistillFacts`,
  `MemoryDistillBrain` and `MemoryDistillProfile` answer `ErrNoMemory` rather
  than calling into nothing;
- the **port itself** is still returned — a kernel with no memory holds a
  **nil** port, which is a different thing from a typed nil.

The refusal is deliberately **not** a silent no-op. "Nothing was forgotten" and
"there is nothing to forget" are different answers, and only the first is a
successful write.

### Every test became a host

Nine test packages opened a kernel and expected memory to be there:
`cmd/agezt`, `kernel/app/introspect`, `kernel/app/system`,
`kernel/cadence/systemtasks`, `kernel/controlplane`, `kernel/runtime`,
`plugins/builtintools` and the shared `kernel/internal/testfixtures`. Each
became a **host**: it opens the memory itself and injects it, the same way the
daemon does.

```go
// kernel/runtime/memory_host_test.go
func withMemory(t *testing.T, cfg runtime.Config) runtime.Config {
    t.Helper()
    if cfg.MemoryStore != nil {
        return cfg
    }
    base := cfg.BaseDir
    if base == "" {
        base = t.TempDir()
    }
    store, builder, err := testfixtures.MemoryPorts(base)
    if err != nil {
        t.Fatalf("open the memory store: %v", err)
    }
    cfg.MemoryStore = store
    cfg.MemoryBuilder = builder
    return cfg
}
```

The store is opened **once per config**: a test that reopens the same config —
a **restart** — keeps the same store, so a note written before the restart is
still there after it. That is the whole point of the file.

`cmd/agezt` and `plugins/builtintools` are **outside** `kernel/`, so the
`kernel/internal` rule keeps them from the shared fixtures; they carry their
own local helper that names the module — the daemon is the composition root,
and the composition root names the module.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/memoryapi`: `TestTheStorePortIsExactlySixVerbsWide` | the store port is **exactly six** verbs |
| `kernel/contract/memoryapi`: `TestTheStoreNamesOnlyContractRows` | the six verbs keep their exact signatures |
| `kernel/contract/memoryapi`: `TestThePublisherPortIsOneVerb` | the publisher is **one** verb, and it answers the event's **id** and the error |
| `kernel/contract/memoryapi`: `TestTheReaderPortIsExactlyNineVerbsWide` | the read-only half keeps its **nine** verbs |
| `kernel/contract/memoryapi`: `TestTheWriterPortIsExactlyFiveVerbsWide` | the mutating half keeps its **five** verbs |
| `kernel/contract/memoryapi`: `TestTheMaintainerPortIsExactlyFiveVerbsWide` | the hygiene half keeps its **five** verbs |
| `kernel/contract/memoryapi`: `TestTheDistillerPortIsExactlyThreeVerbsWide` | the LLM half keeps its **three** verbs, and **no distiller holds a provider as a field** |
| `kernel/contract/memoryapi`: `TestTheToolerPortIsOneVerb` | the agent-facing surface is **one** verb |
| `kernel/contract/memoryapi`: `TestTheManagerPortIsTheCompositionOfTheHalves` | the manager is **exactly** the union of its halves — no verb of its own |
| `kernel/contract/memoryapi`: `TestTheReaderWriterPortIsTheReaderPlusTheWriter` | the gateway's surface carries neither maintenance nor distillation |
| `kernel/contract/memoryapi`: `TestTheSentinelsAreTheContracts` | the two sentinels exist, are **distinct**, and the structured-output sentinel lives in the LLM contract |
| `kernel/contract/memoryapi`: `TestTheRecordRulesTravelWithTheRow` | the four record rules decide right — plain, forgotten, superseded, suspended and expired records, each in its own case |
| `kernel/contract/memoryapi`: `TestValidTypeIsTheMembershipSet` | the type's membership set is the type's meaning |
| `kernel/contract/memoryapi`: `TestTheContextSeamIsShared` | the correlation and the scope are **orthogonal**, and an empty scope stays unset |
| `kernel/contract/memoryapi`: `TestTheDepsGatherThePortsInOneStruct` | the seams travel in **one** struct, and the builder takes it by value and answers the manager port |
| `kernel/contract/memoryapi`: `TestTheEmbedderPortIsOneVerb` | the embedder is **one** verb |
| `kernel/modules/knowledge/memory`: `TestTheRowsAreTheContractsRows` | the module's rows are the contract's — by **type identity** |
| `kernel/modules/knowledge/memory`: `TestTheSentinelsAreTheContractsValues` | the module's aliases **are** the contract's sentinel values |
| `kernel/modules/knowledge/memory`: `TestTheModuleAnswersTheContractsPorts` | the manager's method set **covers** the contract's manager port |
| `kernel/modules/knowledge/memory`: `TestTheRulesTravelWithTheRow` | the module's validity rule is the **contract's** rule over the whole domain, and the row's rules travel with it |
| `kernel/modules/knowledge/memory`: `TestTheModuleNamesNoOtherModule` | the module holds the contract's **generator type**, not a named path |
| `kernel/modules/knowledge/memory`: the compile-time assertion | the manager answers the contract's manager, reader and writer |
| `kernel/runtime`: `TestTheKernelHoldsTheMemoryContractsPort` | the kernel's face is **statically** the contract's port (the declared return type is compared) |
| `kernel/runtime`: `TestTheKernelNeverOpensAMemoryOfItsOwn` | the kernel opens **no** memory directory of its own |
| `kernel/runtime`: `TestAKernelWithoutAMemoryAnswersNoMemory` | the reads say "no memory" and the writes **refuse** — no panic, no swallowed write |
| `kernel/runtime`: `TestTheInjectedMemoryIsTheLiveOne` | the injected memory is the kernel's memory |
| `cmd/agezt`: `TestOpenAppKernelWiresTheMemory` | the daemon end: the memory is a **directory on disk** with its file, and the port is live |
| `cmd/agezt`: `TestTheMemoryIsTheInjectedOne` | the daemon uses the **injected** memory, and opens no second one |
| `cmd/agezt`: `TestTheDaemonWiresTheStructuredOutputPath` | the daemon's generator really drives a distillation — the agent would otherwise **silently stop learning** |

### The mutation run

**13 mutations: 13 caught, 0 survived, 0 skipped, 0 build errors** — in **two**
independent passes, both honest (every catch named the expected test or the
expected edge).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the agent gateway names the memory module again | **archcheck** — `adapter-bypass kernel/agentgw -> kernel/modules/knowledge/memory` |
| 2 | the control plane names the memory module again | **archcheck** — `adapter-bypass kernel/controlplane -> kernel/modules/knowledge/memory` |
| 3 | the kernel names the memory module again | **archcheck** — `cross-module kernel/runtime -> kernel/modules/knowledge/memory` |
| 4 | the run engine names the memory module again | **archcheck** — `cross-module kernel/runtime/runexec -> kernel/modules/knowledge/memory` |
| 5 | **the memory module names the runs module again** | **archcheck** — `cross-module kernel/modules/knowledge/memory -> kernel/agent` |
| 6 | the contract's reader port grows a tenth verb | `TestTheReaderPortIsExactlyNineVerbsWide` |
| 7 | the kernel's memory face is an anonymous shape, not the contract's port | `TestTheKernelHoldsTheMemoryContractsPort` |
| 8 | the kernel ignores the memory it was handed | `TestTheInjectedMemoryIsTheLiveOne` |
| 9 | the daemon does not hand the memory to the kernel | `TestOpenAppKernelWiresTheMemory` |
| 10 | **the record's usable rule is wrong** | `TestTheRecordRulesTravelWithTheRow` |
| 11 | **the module keeps a sentinel of its own instead of the contract's** | `TestTheSentinelsAreTheContractsValues` |
| 12 | **the module keeps a validity set of its own instead of the contract's** | `TestTheRulesTravelWithTheRow` |
| 13 | **the kernel drops the injected structured-output path** | `TestTheDaemonWiresTheStructuredOutputPath` |

**Five of the thirteen are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that opens
the memory itself, as long as the notes still show up.

**Mutation 5 is the one this slice is really about.** The module reached into
the **runs** module for the structured-output path. A test cannot see that
reach — the distillation still works — but the archcheck gate can, and the
slice's answer is that the path became a **port** the daemon injects.

**Mutations 10, 11, 12 and 13 measure behaviours, not shapes:**

- the **usable rule** decides whether a note may enter a run's context. A rule
  that quietly admitted a forgotten or an expired record would put deleted
  knowledge back in front of the agent, and no structural check can see it;
- the **sentinels** are what the layers above the module **compare against**. A
  module that kept a look-alike error of its own would leave every `errors.Is`
  comparison silently false, and the layer would report "no such note" as
  some other failure;
- the **validity rule** is part of the type's meaning. A module that kept a
  set of its own could drift from the contract's: the layers above would
  accept a type the contract rejects, and nothing structural would see it;
- the **structured-output path** is the only way a distillation pass can turn
  a model's answer into a typed value. A kernel that dropped it would leave
  every distillation **refusing** — the agent would silently stop learning,
  with no error anywhere.

**Mutation 7 needed the run executor's declaration to travel with it.** The
kernel's face is also declared in the run executor's `KernelAPI`; drifting only
the kernel's face is a **compile error**, not a catch. The mutation therefore
drifts both declarations — the code compiles, the interface still matches, and
only a **static** check on the declared return type catches it. (The same
lesson as the catalog, world-model, skill and roster slices, met for the
**fifth** time.)

### Three honest corrections

The run earned three, and all three are worth naming:

1. **The first pass had a broken mutation.** The contract-port mutation
   inserted the reference **inside** a `var` block, where a declaration cannot
   sit — the file stopped parsing and the case reported **BUILDERR**. A compile
   error is not a catch. The reference was moved **after** the block, so the
   file still parses.
2. **The row mutation could not compile at all.** The first attempt replaced
   the module's `Record` **alias** with a look-alike struct — but the module's
   own code uses the row's fields everywhere, so the whole package stopped
   compiling. A mutation that breaks the build measures nothing. It was
   replaced with a mutation that **compiles**: the module keeps a validity set
   of its own instead of the contract's, which is the same promise (the rule
   travels with the row) in a form that can be observed.
3. **The generator mutation was aimed at the wrong side.** The first attempt
   removed the daemon's `cfg.MemoryGenerator = agent.GenerateObject` — which
   also **unused** the `agent` import, so the package stopped compiling. The
   mutation was moved to the **kernel's** side (`Generator: nil` in the
   composition root), where it compiles and where the daemon's end-to-end
   distillation test sees it.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 315 packages placed; 18 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 24** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rules the slice is really about

**A port is a promise about what a reader may do — and it is often several
promises.** The memory has six readers with six different needs — the agent
gateway, the context selector, the control plane, the app layer, the run
executor and the module's own tool — and the port is **split six ways**. The
mistake to avoid is not a missing verb; it is a **reader handed the whole
memory lifecycle**.

**A rule lives with the row it judges.** The record's usability, the type's
membership and the distiller's provider-as-parameter are all part of what the
rows mean. A Go type alias cannot carry methods, so a rule that stayed in the
implementation could not be reached by the layers that need it.

**A seam that crosses a module boundary becomes a port.** The structured-output
path is the clearest case: the memory module needs it, it lives in the runs
module, and neither may name the other. The path became a **port** the daemon
injects — and the cost of that is visible in the ratchet.

**A missing library answers, and the answers differ by direction.** "No
memory" is an honest answer to a read; "the memory is not available on this
daemon" is an honest answer to a write. Neither is a panic, and neither is a
swallowed write.

**A context key is an identity.** The seam's keys and helpers moved to the
contract together — two packages that each define one would not see each
other's values.

## What remains

The ratchet stands at **18/13**.

**The module count, recomputed.** The distinct `kernel/modules/...` packages
the allowlist still names went from **five** to **four**:

> `kernel/modules/board/board` · `kernel/modules/extensions/mcp` ·
> `kernel/modules/extensions/toolbox` · `kernel/modules/work/assure`

(The allowlist also names `kernel/agent` and `kernel/runtime` — the runs
module and the kernel itself — but those are not `kernel/modules/...` packages,
so they do not count toward the metric the previous slices carried.)

The next candidates:

- **`mcp`** — two edges, the smallest remaining family.
- **`kernel/agentgw`'s own edge** — the gateway's port to the memory is now
  the contract's; the same split would apply to the board and the toolbox.
- **`kernel/modules/board/board`** — named by three places (the control plane,
  the REST API and the self-repair module).
