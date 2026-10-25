# W3.49 — the typed schedule library is the daemon's file

## What the slice is

After W3.48 the kernel still reached **12** modules. This slice takes
**three** of those edges — and they are the same family:

| Edge | Why it existed |
|---|---|
| `cross-module kernel/runtime -> kernel/cadence` | the kernel held the concrete `*cadence.Store`, **opened the file** itself and held the concrete `*cadence.Engine` |
| `cross-module kernel/runtime/runexec -> kernel/cadence` | the run-executor interface named the module's store |
| `adapter-bypass kernel/controlplane -> kernel/cadence` | the control plane named the module's rows |

The ratchet went **49 → 46**.

Cadence was the **cheapest** slice left by the measurement and the **widest**
payoff: only **six** call sites in the runtime, but **three** edges — the most
of any candidate. The rows were already in the contract (`cadenceapi.Entry`,
the `Target*`/`Source*`/`Mode*` vocabularies, the system-task catalog); what
was missing was a **port**.

## What the kernel used to do

```go
// kernel/runtime/runtime_kernel.go
schedules   *cadence.Store
schedEngine *cadence.Engine
// kernel/runtime/compose.go
schedStore, err := cadence.OpenStore(filepath.Join(cfg.BaseDir, "cadence"))
// kernel/runtime/runtime_accessors.go
func (k *Kernel) Schedules() *cadence.Store { return k.schedules }
```

The kernel opened the library's file, held the module's concrete store, and
handed the module's concrete resident to the app layers.

## What it does now

### The contract carries the port

```go
// kernel/contract/cadenceapi/hoststore.go
type HostStore interface {
    Store                       // the tool's eleven verbs
    AddWindow(...)
    Get(id string) (Entry, bool)
    Count() int
    SetEnabled(id string, enabled bool) (bool, error)
    SetIntent(id, intent string) (bool, error)
    SetModel(id, model string) (bool, error)
    SetIntentTarget(id string) (bool, error)
    Reschedule(...)
    RunNow(id string) (bool, error)
}
```

**Twenty** verbs: the tool's eleven plus the nine the host drives on its own
(the control plane retires a schedule, the guardians re-arm one, the app layer
edits its cadence, the status view counts what is armed).

The tool's port (`cadenceapi.Store`, eleven verbs) stays exactly as narrow as
it was — the agent-facing schedule tool adds, retargets and lists, and nothing
else. The host's port **embeds** it, so the two shapes cannot drift: if the
tool's port grows a verb, the host's port grows it too, and the module's
implementation is the one that has to answer.

### The resident is a port too

```go
// kernel/contract/cadenceapi/engine.go
type Engine interface {
    Start(ctx context.Context)
    Wait()
    WaitIdle(ctx context.Context) error
    RunningCount() int
}
```

**Four** verbs — the host only reads the resident's state and waits for it.
The resident's firing logic belongs to the module.

### The library's file is the daemon's; the resident is the daemon's too

```go
// cmd/agezt/main.go
schedStore, err := cadence.OpenStore(filepath.Join(baseDir, "cadence"))
if err != nil { ... }
cfg.Schedules = schedStore
// cmd/agezt/main_cadence.go
eng := cadence.NewEngine(store, run, 0, stdout)
k.SetScheduleEngine(eng)
```

A library that cannot be opened is a **startup error** — the schedule library
is part of the daemon, not a convenience. The resident is built by the
composition root over **the daemon's own store**, so the kernel's port and the
resident read the same rows.

### A missing library is an answer, not a panic — in TWO layers

Before the slice the kernel opened the library itself, so `k.Schedules()` was
**never nil** and nothing had to say what happens without one. Now a host may
open a kernel without a library, so two layers had to learn the rule:

- the **kernel**: the port is nil, and every holder that asks for it says so
  instead of calling into nothing;
- the **app layer** (`kernel/app/schedule`): the creation, the editing (begin
  and apply), the lifecycle (remove, run, enable) and the read (list) all
  refuse with *"the schedule library is not available on this daemon"*;
- the **guardians' host**: the schedule half of the seeder's port answers
  *"the schedule library is not available on this daemon"* too.

The control plane's three roster surfaces (impact cascade, status view,
teardown) read the port defensively: a host without a library sees **no**
schedules rather than calling into nothing.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/cadenceapi`: `TestTheHostStoreCarriesTheTwentyVerbs` | the host port is **exactly twenty** verbs wide |
| `kernel/contract/cadenceapi`: `TestTheHostPortIsWiderThanTheToolPort` | the host port is a **superset** of the tool's eleven — the width rule made mechanical |
| `kernel/contract/cadenceapi`: `TestTheEnginePortCarriesTheFourVerbs` | the resident's port is **exactly four** verbs wide |
| `kernel/contract/cadenceapi`: `TestTheEnginePortCarriesTheContractRows` | the four verbs keep their exact signatures |
| `kernel/cadence`: `TestTheStoreIsTheContractsHostPort` | the module's store answers **both** contract ports |
| `kernel/cadence`: `TestTheEngineIsTheContractsPort` | the module's resident answers the engine port |
| `kernel/cadence`: `TestTheRowsAreTheContractRows` | the module's rows and vocabularies **are** the contract's |
| `kernel/runtime`: `TestTheScheduleLibraryStaysANilPortWithoutOne` | a kernel without a library hands over a **nil** port |
| `kernel/runtime`: `TestTheScheduleFaceIsTheHostPort` | the kernel's face is **statically** the host port |
| `kernel/runtime`: `TestTheSchedulePortAnswersTheLiveLibrary` | the port answers the **live** rows, not a copy |
| `kernel/runtime`: `TestTheScheduleEnginePortIsTheContractPort` | the resident is held as the contract's port |
| `kernel/app/schedule`: `TestTheAppLayerRefusesWithoutALibrary` | all **six** app surfaces refuse instead of panicking |
| `cmd/agezt`: `TestOpenAppKernelWiresTheScheduleLibrary` | the daemon end: the port is live and the library's file is **on disk** |
| `cmd/agezt`: `TestTheScheduleEngineIsBuiltOverTheInjectedLibrary` | the daemon uses the **injected** library, not one of its own |
| `cmd/agezt`: `TestTheSchedulePortIsTheContractPort` | the tools get the **narrow** port, the kernel the wide one |

### The mutation run

**8 mutations: 8 caught, 0 survived, 0 skipped, 0 build errors** — in **two**
independent passes, both honest (every catch named the expected test or the
expected edge).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the runtime names the module again | **archcheck** — `cross-module kernel/runtime -> kernel/cadence` |
| 2 | the control plane names the module's rows again | **archcheck** — `adapter-bypass kernel/controlplane -> kernel/cadence` |
| 3 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |
| 4 | the kernel's face narrows to the tool's port | `TestTheScheduleFaceIsTheHostPort` |
| 5 | the kernel stops answering the live library | `TestTheSchedulePortAnswersTheLiveLibrary` |
| 6 | the resident setter drops the resident | `TestTheScheduleEnginePortIsTheContractPort` |
| 7 | the app layer stops refusing without a library | `TestTheAppLayerRefusesWithoutALibrary` |
| 8 | the daemon does not hand the library to the kernel | `TestOpenAppKernelWiresTheScheduleLibrary` |

**Three of the eight are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that opens
the library itself, as long as the schedules work.

The run earned one honest correction: the first attempt at mutation **8** was
a **build error** (the store became an unused variable), and a build error is
not a catch — the mutation was wrong, not the test. The mutation was rewritten
to keep the store bound and simply not hand it over.

Mutation **4** is the one worth naming: it narrows the kernel's face from the
host port to the tool's port. That **still compiles** (the tool's port is a
subset) and no runtime behaviour would notice — only a **static** check on the
declared return type catches it. A port's width is a contract, and a contract
that is only inferred from behaviour is a contract that can drift.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 311 packages placed; 46 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 49** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rule the slice is really about

**A port's width is decided by the weakest holder of it — and the two shapes
are pinned, not inferred.** The tool keeps eleven verbs; the host keeps
twenty; the host's port embeds the tool's, so a change to one is a change to
both. The kernel's face is checked **statically**, because narrowing it would
still compile and still pass every behaviour test.

**A library's file belongs to the daemon, and so does the resident.** The
daemon opens the file, hands the kernel the port and builds the resident over
the same store — so the kernel's port and the resident read the same rows. A
second instance over the same file would have been a silent split-brain, and
the slice's test (`TestTheScheduleEngineIsBuiltOverTheInjectedLibrary`) is
what makes that impossible to reintroduce quietly.

**Every test that expected a store became a host.** The kernel opened the
library before; now every test that needs one opens it, injects the port and
— where the resident is involved — builds the resident over that very store.
That is the same shift W3.43/W3.47/W3.48 made for the artifacts, the board and
the workflow library: a test cannot be a host and a user at the same time.

## What remains

The kernel still reaches **9** modules. The next candidates, in the order the
measurement suggests:

- **`standing`** — the kernel holds the module's concrete store; three edges
  (`runtime`, `runexec`, `controlplane`) and the rows are already in
  `standingapi`.
- **`skill`** — the kernel holds the forge and its file store; three edges plus
  the market's; `skillapi` exists.
- **`catalog`** — the kernel holds the store and the snapshot; two edges;
  `catalogapi` exists.
- **`memory`, `worldmodel`, `roster`, `okr`, `taste`** — the kernel holds each
  module's concrete store, and each one is the same slice (a contract port,
  the daemon opening the file, the kernel's accessor carrying the port).
