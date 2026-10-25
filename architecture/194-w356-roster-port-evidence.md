# W3.56 — the roster is the daemon's

## What the slice is

After W3.55 the ratchet stood at **28/13** and the allowlist still named the
roster module from four places. This slice takes **four** of those edges — and
it takes them **completely**:

| Edge | Why it existed |
|---|---|
| `cross-module kernel/runtime -> kernel/modules/agents/roster` | the kernel held the module's concrete `*roster.Store` and **opened the file** itself |
| `cross-module kernel/runtime/runexec -> kernel/modules/agents/roster` | the run-executor declared the module's concrete store in its `KernelAPI` |
| `adapter-bypass kernel/controlplane -> kernel/modules/agents/roster` | the control plane named the module's rows |
| `adapter-bypass kernel/agentgw -> kernel/modules/agents/roster` | the agent gateway named the module's rows |

The ratchet went **28 → 24**. The **13** allowlisted call sites are unchanged.

This slice is the **cheapest** of the W3.5x series, and it is cheap for a
reason worth naming: the roster's **rows were already in the contract**
(`agentapi.Profile` moved there in W3.14). What was missing was only the
**port**, the **sentinels** and the **rule** — the three things the layers
above the module could not reach.

It is also the first slice in the series that is **not bus-bound**: the roster
does not journal itself (the **kernel** presses the journal), so there is no
publisher port and no builder to inject. The daemon opens the file and hands
over the store, and that is the whole handover.

## What the kernel used to do

```go
// kernel/runtime/compose.go
rstore, err := roster.Open(filepath.Join(cfg.BaseDir, "roster"))
if err != nil {
    return fail("runtime: roster", err)
}
closers = append(closers, rstore)
```

The kernel opened the roster's file, held the module's **concrete** type, and
the run executor, the control plane and the agent gateway then had to name that
type as well.

## What it does now

### The contract carries the port, the sentinels and the rule

`kernel/contract/agentapi` already existed (the agent-facing tools are
PLUGINS, and a plugin may reach the contract and nothing else). This slice
**adds** to it:

```go
// kernel/contract/agentapi/store.go
var ErrNotFound = errors.New("roster: profile not found")
var ErrRetired  = errors.New("roster: profile is retired")

type Store interface {                        // eight verbs
    Add(p Profile) (Profile, error)
    SetEnabled(ref string, enabled bool) (Profile, error)
    SetRetired(ref string, retired bool, reason ...string) (Profile, error)
    Update(ref string, mutate func(*Profile)) (Profile, error)
    Remove(ref string) (Profile, bool, error)
    Get(ref string) (Profile, bool)
    List() []Profile
    Count() int
}

type Reader interface {                       // two verbs
    Get(ref string) (Profile, bool)
    List() []Profile
}

type RepairSource interface {                 // one verb
    RepairAgent(ref, reason string) (RepairResult, error)
}
```

The module keeps every name as an **alias** (`roster.ErrNotFound =
agentapi.ErrNotFound`, `roster.Profile = agentapi.Profile`,
`roster.RepairSource = agentapi.RepairSource`), so there is one row and one
spelling of it — and `errors.Is` matches on **both** sides of the module
boundary.

**The port is SPLIT, not widened.** The gateway only looks agents up, so it
takes the **two-verb** `Reader`; the control plane drives the lifecycle, so it
takes the **eight-verb** `Store`; a repair pass takes the **one-verb**
`RepairSource`. A single wide port would have handed the gateway the whole
agent lifecycle — a gateway whose only jobs are to list agents and read one
profile.

**The rule travels with the row.** `AutonomyRunbook` — the machine-readable
wake contract of a profile (trigger / route / recovery / sleep) — moved from the
module **with** the profile row it judges, into the contract. The control
plane's wake evidence, the run executor's sub-agent prep and the daemon's
cadence all build the **same** payload, and they may not name the module to
reach the rule.

**The test seam stays out of the port.** `SetNowForTest` is deliberately
**not** on `agentapi.Store`: a port is what the layers above the module may
call, and a clock hook is not theirs. The tests open the store and inject it.

### The daemon opens the file; the kernel holds the port

```go
// cmd/agezt/main_toolinvoker.go
if cfg.Roster == nil {
    store, err := roster.Open(filepath.Join(cfg.BaseDir, "roster"))
    if err != nil {
        return nil, err
    }
    cfg.Roster = store
}
```

```go
// kernel/runtime/compose.go
roster: cfg.Roster,
```

Because the roster lives under the **base dir**, a **tenant** keeps **its
own** agents — the tenant's kernel is opened with the tenant's base dir, so it
opens the tenant's roster.

### A missing roster answers, on purpose

The roster is the **fleet**, so a kernel without one is not an edge case — it
is a daemon that has no agents yet. The answers are therefore **different per
direction**, and each one is stated once:

- the **reads** answer **"no agents"** — `RosterGet` finds nothing and
  `RosterList` returns an empty list. They go through the kernel's **nil-safe**
  helpers (`RosterGet` / `RosterList`), so every reader above the module is
  covered by one decision, and the **control plane, the app layer and the
  gateway** call those helpers instead of the raw port;
- the **writes refuse** — `AddProfile`, `SetProfileEnabled`, `SetProfileRetired`,
  `UpdateProfile` and `RemoveProfile` answer *"the roster is not available on
  this daemon"* rather than calling into nothing;
- the **port itself** is still returned — a kernel with no roster holds a
  **nil** port, which is a different thing from a typed nil.

### The nil port is the reason the reads had to move

Three production surfaces took the module's **concrete** store and would have
panicked the moment the roster was absent — the app layer's overseer source,
the guardians' host and the control plane's operation builders:

```go
// before — a method value on a nil interface panics at the call site
cur, ok := s.k.Roster().Get(ref)

// after — the kernel answers "no agents"
cur, ok := s.k.RosterGet(ref)
```

This is the **typed-nil trap** the roster slice inherited from the standing
and schedule slices: a nil port handed to an interface parameter is a
**non-nil** interface, and a method value taken from it panics before the
first call. Every one of those surfaces now goes through the kernel's
nil-safe readers.

### Every test became a host

Twelve test packages opened a kernel and expected agents to be there:
`cmd/agezt`, `cmd/agt`, `kernel/app/configtool`, `kernel/app/guardians`,
`kernel/app/overseer`, `kernel/app/runs`, `kernel/cadence/systemtasks`,
`kernel/controlplane`, `kernel/modules/agents/reaper`,
`kernel/modules/agents/selfrepair`, `kernel/runtime` and
`plugins/builtintools`. Each became a **host**: it opens the roster itself and
injects it, the same way the daemon does.

```go
// kernel/runtime/roster_host_test.go
func withRoster(t *testing.T, cfg runtime.Config) runtime.Config {
    t.Helper()
    if cfg.Roster != nil {
        return cfg
    }
    base := cfg.BaseDir
    if base == "" {
        base = t.TempDir()
    }
    store, err := roster.Open(filepath.Join(base, "roster"))
    if err != nil {
        t.Fatalf("roster.Open: %v", err)
    }
    cfg.Roster = store
    return cfg
}
```

The store is opened **once per config**: a test that reopens the same config —
a **restart** — keeps the same store, so a profile written before the restart
is still there after it. That is the whole point of the file.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/agentapi`: `TestTheStorePortIsExactlyEightVerbsWide` | the store port is **exactly eight** verbs |
| `kernel/contract/agentapi`: `TestTheStoreNamesOnlyContractRows` | the eight verbs keep their exact signatures, and **no verb takes a `time.Time`** (P4) |
| `kernel/contract/agentapi`: `TestTheReaderPortIsExactlyTwoVerbsWide` | the read-only half keeps its **two** verbs |
| `kernel/contract/agentapi`: `TestTheRepairSourcePortIsOneVerb` | the repair surface is **one** verb |
| `kernel/contract/agentapi`: `TestTheSentinelsAreTheContracts` | the two sentinels exist and are **distinct** |
| `kernel/contract/agentapi`: `TestTheAutonomyRunbookTravelsWithTheRow` | the runbook rule decides right — the trigger, the route, the recovery and the sleep contract, each in its own case |
| `kernel/modules/agents/roster`: `TestTheModuleAnswersTheContractsPorts` | the module's store **is** the contract's store port |
| `kernel/modules/agents/roster`: `TestTheSentinelsAreTheContractsValues` | the module's aliases **are** the contract's sentinel values |
| `kernel/modules/agents/roster`: `TestTheRowsAreTheContractsRows` | the module's rows are the contract's — by **type identity** |
| `kernel/modules/agents/roster`: `TestTheStoreRaisesTheContractsSentinels` | the store's refusals are the contract's sentinels, so `errors.Is` holds |
| `kernel/modules/agents/roster`: `TestTheRunbookRuleTravelsWithTheRow` | the module's runbook is the contract's runbook, not a second derivation |
| `kernel/modules/agents/roster`: the compile-time assertion | the module's store answers the contract's port |
| `kernel/runtime`: `TestTheKernelHoldsTheContractsPort` | the kernel's face is **statically** the contract's port (the declared return type is compared) |
| `kernel/runtime`: `TestTheKernelNeverOpensARosterOfItsOwn` | the kernel opens **no** roster directory of its own |
| `kernel/runtime`: `TestAKernelWithoutARosterAnswersNoAgents` | the reads say "no agents" and the writes **refuse** — no panic, no swallowed write |
| `kernel/runtime`: `TestTheInjectedRosterIsTheLiveOne` | the injected roster is the kernel's roster |
| `cmd/agezt`: `TestOpenAppKernelWiresTheRoster` | the daemon end: the roster is a **directory on disk** with its file, and the port is live |
| `cmd/agezt`: `TestTheRosterIsTheInjectedOne` | the daemon uses the **injected** roster, and opens no second one |

### The mutation run

**11 mutations: 11 caught, 0 survived, 0 skipped, 0 build errors** — in **two**
independent passes, both honest (every catch named the expected test or the
expected edge).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the agent gateway names the roster module again | **archcheck** — `adapter-bypass kernel/agentgw -> kernel/modules/agents/roster` |
| 2 | the control plane names the roster module again | **archcheck** — `adapter-bypass kernel/controlplane -> kernel/modules/agents/roster` |
| 3 | the kernel names the roster module again | **archcheck** — `cross-module kernel/runtime -> kernel/modules/agents/roster` |
| 4 | the run engine names the roster module again | **archcheck** — `cross-module kernel/runtime/runexec -> kernel/modules/agents/roster` |
| 5 | the contract's store port grows a ninth verb | `TestTheStorePortIsExactlyEightVerbsWide` |
| 6 | the kernel's roster face is an anonymous shape, not the contract's port | `TestTheKernelHoldsTheContractsPort` |
| 7 | the kernel ignores the roster it was handed | `TestTheInjectedRosterIsTheLiveOne` |
| 8 | the daemon does not hand the roster to the kernel | `TestOpenAppKernelWiresTheRoster` |
| 9 | **the runbook rule is wrong** (a delegation-only agent is operator-wakeable) | `TestTheAutonomyRunbookTravelsWithTheRow` |
| 10 | **the module raises its own error instead of the contract's sentinel** | `TestTheStoreRaisesTheContractsSentinels` |
| 11 | **the module keeps a sentinel of its own instead of the contract's** | `TestTheSentinelsAreTheContractsValues` |

**Four of the ten are gate-measured.** "The kernel's reach is smaller" is not
something a test can see: a test would happily accept a kernel that opens the
roster itself, as long as the agents still show up.

**Mutation 6 needed the run executor's declaration to travel with it.** The
kernel's face is also declared in the run executor's `KernelAPI`; drifting only
the kernel's face is a **compile error**, not a catch. The mutation therefore
drifts both declarations — the code compiles, the interface still matches, and
only a **static** check on the declared return type catches it. (The same
lesson as the catalog, world-model and skill slices, met for the **fourth**
time.)

**Mutations 9, 10 and 11 are the three this slice added.** All three measure
a **behaviour**, not a shape:

- the runbook rule decides whether a **managed** agent may be woken from
  outside — a rule that quietly admitted one would let a delegation-only agent
  be woken by an operator, and no structural check can see it;
- the sentinels are what the layers above the module **compare against** — a
  module that raised a look-alike error of its own, or that kept a sentinel of
  its own instead of the contract's, would leave every `errors.Is` comparison
  silently false, and the layer would report "no such agent" as some other
  failure. Mutation 10 breaks the **raise**; mutation 11 breaks the **alias**;
  they are two different halves of the same promise.

### Three honest corrections

The run earned three, and all three are worth naming:

1. **The first pass was invalid.** The second mutation's anchor pointed at
   `kernel/controlplane/app_operations.go`, but the function it names lives in
   `kernel/controlplane/standing.go` — so the mutation never landed and the
   case reported **SKIP**. A SKIP is not a catch and not a pass; the anchor
   was corrected and the whole run repeated.
2. **The run engine's mutation was a parse error.** The reference was inserted
   **inside** the `KernelAPI` interface body, where a declaration cannot sit.
   The mutation was moved **after** the interface, so the file still parses —
   a mutation that broke the parse would have been a malformed case, not a
   catch.
3. **The sentinel mutation was aimed at the wrong test.** The first attempt
   mutated the store's raised error but expected the **alias** test to catch
   it; the alias test only compares the two **values**, so it stayed green and
   the mutation **survived**. The expectation was moved to
   `TestTheStoreRaisesTheContractsSentinels` — the test that actually reads
   the error the store raises — and the mutation is caught there. A **second**
   sentinel mutation (the module keeping a sentinel of its own) was added
   afterwards, so the alias half is measured too.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 314 packages placed; 24 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 28** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rules the slice is really about

**A port is a promise about what a reader may do.** The roster has three
readers with three different needs — the fleet's manager, the operator's
control plane and the gateway's lookup — and three ports. The mistake to avoid
is not a missing verb; it is a **reader handed the whole lifecycle**.

**A rule lives with the row it judges.** The runbook is the profile's wake
contract; a rule that stayed in the implementation while its row moved to the
contract could not be reached by the layers that need it.

**A missing library answers, and the answers differ by direction.** "No
agents" is an honest answer to a read; "the roster is not available on this
daemon" is an honest answer to a write. Neither is a panic, and neither is a
swallowed write.

**A nil port is not a typed nil.** The moment the roster became the daemon's
file, every surface that took a method value from the port became a potential
panic. The nil-safe readers put that decision in **one** place.

## What remains

The ratchet stands at **24/13**.

**A correction to a carried number.** The previous slices' documents say "the
kernel still reaches **7** modules". Recomputing the metric — the distinct
`kernel/modules/...` packages the allowlist still names — gives **six** after
W3.55, not seven: the number had been carried forward without recomputation
since W3.51. This slice takes it to **five**:

> `kernel/modules/agents/roster` · `kernel/modules/board/board` ·
> `kernel/modules/extensions/mcp` · `kernel/modules/extensions/toolbox` ·
> `kernel/modules/knowledge/memory` · `kernel/modules/work/assure`

The next candidates:

- **`memory`** — six edges; the manager is bus-bound and LLM-bound (distill,
  embeddings), so its port would be the widest of all — and it is the **third**
  bus-bound module, so the builder-plus-publisher pattern applies.
- **`mcp`** — two edges, the smallest remaining family.
