# W3.53 — the objective library is the daemon's

## What the slice is

After W3.52 the kernel still reached **7** modules and the ratchet stood at
**38**. This slice takes **one** of those edges and takes it **completely**:

| Edge | Why it existed |
|---|---|
| `cross-module kernel/runtime -> kernel/modules/work/okr` | the kernel held the module's concrete `*okr.Store` and opened the module's file itself |

The ratchet went **38 → 37**. The kernel still reaches **7** modules.

It is the same slice shape as W3.52 (`taste`), one step wider: the objective
library carries **nine** verbs instead of five, and — unlike the exemplar
library — it carries a **rule**.

## What the kernel used to do

```go
// kernel/runtime/runtime_kernel.go
okr *okr.Store

// kernel/runtime/compose.go
okrstore, err := okr.OpenStore(filepath.Join(cfg.BaseDir, "okr"))
if err != nil {
    return fail("runtime: okr", err)
}
```

The kernel opened the library's file, held the module's **concrete** store, and
reached the module's rows (`okr.Objective`, `okr.CreateSpec`, `okr.Filter`, the
`Status` constants) by name.

## What it does now

### The contract carries the rows, the rule and the port

```go
// kernel/contract/okrapi/okr.go
type Status string
const (StatusActive Status = "active"; StatusAchieved Status = "achieved"; StatusArchived Status = "archived")
type KeyResult struct { ID, Title string; Target int; TaskIDs []string; CreatedMS int64 }
type Objective struct { ID, Title, Description, Owner, Tenant string; Status Status; KeyResults []KeyResult; CreatedMS, UpdatedMS, AchievedMS int64 }
type ObjectiveProgress struct { ObjectiveID string; KeyResults []KeyResultProgress; Percent int; Achieved bool }
type CreateSpec struct { Title, Description, Owner, Tenant string }
type Filter struct { Status Status; Tenant string; IncludeArchived bool; Limit int }
```

```go
// kernel/contract/okrapi/store.go
type Store interface {
    Create(spec CreateSpec, nowMS int64) (Objective, error)
    Get(id string) (Objective, bool)
    List(f Filter) []Objective
    AddKeyResult(objID, title string, target int, nowMS int64) (Objective, error)
    LinkTask(objID, krID, taskID string, nowMS int64) (Objective, error)
    UnlinkTask(objID, krID, taskID string, nowMS int64) (Objective, error)
    SetStatus(id string, status Status, nowMS int64) (Objective, error)
    Archive(id string, nowMS int64) (Objective, error)
    ObjectivesForTask(taskID string) []string
}
```

The module keeps every name as an **alias**, so there is one row and one
spelling of it, and every existing caller is unchanged.

**The port carries no clock.** Six of the nine verbs stamp a row, and they take
`nowMS int64` — not a `time.Time`: the contract layer may not name a clock
(archcheck's principle P4), and a port that reads one cannot be tested
deterministically. The module's own verbs changed shape with it.

### The RULE travels with the ROW

This is the part that makes the OKR slice different from the others. The
objective's rollup — *does this goal count as met?* — was a **method** on the
objective:

```go
func (o Objective) Progress(doneOf func(taskID string) bool) ObjectiveProgress
```

A Go type alias **cannot carry methods**. If the rule had stayed in the
module, the alias would have lost it, and the layers above the module — the
runtime's `Rollup`, the app layer's read surface — would have had to reach
back into the module to compute a rollup. So the rule moved **with the row**
into the contract, and the module's `Objective` is an alias that keeps it.

The rule is not a detail: it is what decides whether a goal counts as met, and
a mutation that halves the percentage (`done * 50 / target`) still "works" —
the rollup is simply wrong, and a goal looks half met forever.

### The library is the daemon's

```go
// cmd/agezt/main_toolinvoker.go
if cfg.OKRStore == nil {
    store, err := okr.OpenStore(filepath.Join(cfg.BaseDir, "okr"))
    if err != nil {
        return nil, err
    }
    cfg.OKRStore = store
}
```

The kernel opens **no** library any more; it holds `cfg.OKRStore`, the
contract's port. Because the library lives under the **base dir**, a tenant's
kernel opens **its own** objectives.

### A missing library: the answers differ, on purpose

- The **read** surface answers the **empty** list — "no objectives" is an
  honest answer.
- The **named** read (`show`) **refuses**: "unknown objective" would blame the
  id for a library that is not there. A named thing cannot be answered from an
  empty library.
- The **write** surfaces (create, add key result, link, unlink, archive)
  **refuse** with *"the objective library is not available on this daemon"* —
  there is nowhere to write, so the refusal is the answer.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/okrapi`: `TestTheStorePortIsExactlyNineVerbsWide` | the store port is **exactly nine** verbs |
| `kernel/contract/okrapi`: `TestTheStorePortNamesOnlyContractRows` | the nine verbs keep their exact signatures and name only the contract's rows |
| `kernel/contract/okrapi`: `TestThePortCarriesNoClock` | no verb takes a `time.Time` — the clock is the caller's |
| `kernel/contract/okrapi`: `TestTheSentinelsAreTheContracts` | the three sentinels the layers compare against are the contract's |
| `kernel/contract/okrapi`: `TestTheRollupRuleTravelsWithTheRow` | the rollup rule is **on the contract's row** and computes the right answer (per-KR percentages, the effective target, the objective's percentage and achieved flag) |
| `kernel/modules/work/okr`: `TestTheRowsAreTheContractsRows` | the module's rows and status constants are the contract's — by **type identity** |
| `kernel/modules/work/okr`: `TestTheSentinelsAreTheContracts` | the module's sentinels **are** the contract's |
| `kernel/modules/work/okr`: `TestTheRollupRuleIsTheContracts` | the aliased objective **keeps** the contract's rule |
| `kernel/modules/work/okr`: the compile-time assertion | the module's store answers the contract's port |
| `kernel/runtime`: `TestTheOKRFaceIsTheContractsPort` | the kernel's face is **statically** the contract's port |
| `kernel/runtime`: `TestTheOKRPortStaysNilWithoutALibrary` | a kernel without a library hands over a **nil** port |
| `kernel/runtime`: `TestTheObjectiveSurfacesRefuseWithoutALibrary` | all four write surfaces refuse instead of panicking |
| `kernel/runtime`: `TestTheOKRPortAnswersTheLiveLibrary` | the port answers the **live** rows |
| `kernel/app/okr`: `TestTheReadSurfaceAnswersEmptyWithoutALibrary` | the read surface answers the empty list, present and not nil |
| `kernel/app/okr`: `TestShowRefusesWithoutALibrary` | the named read refuses rather than blaming the id |
| `kernel/app/okr`: `TestTheRefusalNamesItsReason` | the refusal says **why** |
| `cmd/agezt`: `TestOpenAppKernelWiresTheObjectiveLibrary` | the daemon end: the port is live and the library is a **directory on disk** |
| `cmd/agezt`: `TestTheObjectiveLibraryIsTheInjectedOne` | the daemon uses the **injected** library, not one of its own |

### The mutation run

**9 mutations: 9 caught, 0 survived, 0 skipped, 0 build errors** — in **two**
independent passes, both honest (every catch named the expected test or the
expected edge).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the kernel names the module again | **archcheck** — `cross-module kernel/runtime -> kernel/modules/work/okr` |
| 2 | the control plane names the module again | **archcheck** — `adapter-bypass kernel/controlplane -> kernel/modules/work/okr` |
| 3 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |
| 4 | the contract's store port grows a tenth verb | `TestTheStorePortIsExactlyNineVerbsWide` |
| 5 | the kernel's objective face is an anonymous shape, not the contract's port | `TestTheOKRFaceIsTheContractsPort` |
| 6 | the kernel ignores the library it was handed | `TestTheOKRPortAnswersTheLiveLibrary` |
| 7 | the objective surfaces stop refusing without a library | `TestTheObjectiveSurfacesRefuseWithoutALibrary` |
| 8 | the daemon does not hand the library to the kernel | `TestOpenAppKernelWiresTheObjectiveLibrary` |
| 9 | **the rollup rule is wrong** (the percentage is halved) | `TestTheRollupRuleTravelsWithTheRow` |

**Three of the nine are gate-measured.** "The kernel's reach is smaller" is not
something a test can see: a test would happily accept a kernel that opens the
library itself, as long as the objectives still show up.

Mutation **9** is the one this slice added. The rule that decides whether a
goal counts as met is a **behaviour**, not a shape: a static width test cannot
see it, and a mutation that halves the percentage still compiles, still passes
every structural check, and still answers "a percentage" — just a wrong one.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 313 packages placed; 37 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 38** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rules the slice is really about

**A rule must travel with its row.** A Go type alias cannot carry methods, so
a rule left in the module is a rule the layers above it cannot reach. The
rollup is not decoration — it decides whether a goal counts as met, and it now
lives where the goal lives.

**A named thing cannot be answered from an empty library.** "No objectives" is
an honest answer to "list the objectives"; it is a lie as the answer to "show
me objective o-1", where the truth is that there is no library to look in. The
two surfaces answer differently on purpose.

**A port that carries a clock cannot be tested deterministically.** Six of the
nine verbs stamp a row, and all six take `nowMS int64`.

## What remains

The kernel still reaches **7** modules, and the ratchet stands at **37**. The
next candidates:

- **`skill`** — the kernel holds the forge and its file store; five edges;
  `skillapi` exists (the `Forge` and `BundleStore` ports are already there).
  The **width** is the work: the forge's surface is 15+ verbs, and it is
  journaled through the bus, so the port and the publisher have to travel
  together.
- **`roster`** — the kernel holds the store; four edges and 73 importer files
  by far the largest fan-in; the rows are already in `agentapi`, only the
  store port and the sentinels are missing.
- **`memory`, `worldmodel`** — each one the same slice this one was (a
  contract port, the daemon opening the file, the kernel's accessor carrying
  the port), with the memory manager's bus-bound journal as the extra work.
