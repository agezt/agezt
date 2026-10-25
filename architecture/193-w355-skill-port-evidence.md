# W3.55 — the skill library is the daemon's

## What the slice is

After W3.54 the kernel still reached **7** modules and the ratchet stood at
**33**. This slice takes **five** of those edges — the **widest** slice since
the standing-order library — and it takes them **completely**:

| Edge | Why it existed |
|---|---|
| `cross-module kernel/runtime -> kernel/modules/skills/skill` | the kernel held the module's concrete `*skill.Forge` and **opened the file** itself |
| `cross-module kernel/runtime/runexec -> kernel/modules/skills/skill` | the run-executor declared the module's concrete forge in its `KernelAPI` |
| `cross-module kernel/contextselect -> kernel/modules/skills/skill` | the context selector named the module's rows |
| `cross-module kernel/market -> kernel/modules/skills/skill` | the marketplace named the module's rows |
| `adapter-bypass kernel/controlplane -> kernel/modules/skills/skill` | the control plane named the module's rows |

The ratchet went **33 → 28**. The kernel still reaches **7** modules.

This slice is the **second bus-bound module** — the first was the world model
(W3.54) — and it reuses the pattern that slice established: a **builder
injected by the daemon**, a **publisher port** the kernel hands over.

It is also the **widest port** in the codebase so far: **twenty** verbs.

## What the kernel used to do

```go
// kernel/runtime/compose.go
skstore, err := skill.Open(filepath.Join(cfg.BaseDir, "skills"))
if err != nil {
    return fail("runtime: skills", err)
}
closers = append(closers, skstore)
forge := skill.NewForge(skstore, kbus)
if bundles, berr := skill.OpenBundles(filepath.Join(cfg.BaseDir, "skills")); berr == nil {
    forge.SetBundles(bundles)
}
```

The kernel opened the library's file, built the forge over its own bus, and
held the module's **concrete** type — which the run executor, the context
selector, the marketplace and the control plane then had to name as well.

## What it does now

### The contract carries the rows, the rules and the ports

`kernel/contract/skillapi` already existed (the agent-facing skill tool is a
PLUGIN, and a plugin may reach the contract and nothing else). This slice
**widens** it:

- the **rows** that the layers above the module share — `Scored` (a skill with
  its retrieval score), `ActivationDirective` (an explicit skill request parsed
  out of an intent) and `HygieneReport` (the operator's cleanup view);
- the **sentinels** the layers compare against — `ErrIllegalTransition`,
  `ErrNotFound`, `ErrEmptyBody`;
- the **pure rules** — the retrieval ranking (`Retrieve`, `RetrieveShadow`) and
  the directive parser (`ParseActivationDirective`);
- the **ports**:

```go
// kernel/contract/skillapi/ports.go
type Store     interface { Put, Get, All, Count, Close }              // five verbs
type Publisher interface { Publish(spec eventapi.Spec) (id string, err error) } // one verb
type Forge     interface { Create, Promote, Quarantine, Get, List }   // five verbs (unchanged)
type Lifecycle interface { Archive, Revert, RestoreStatus, Reassign }  // four verbs
type Activator interface { ActivateFor, ActivateExplicitFor }         // two verbs
type Outcome   interface { RecordOutcome }                            // one verb
type Proposer  interface { Propose, ShadowEvaluate }                  // two verbs
type Observer  interface { Count, Hygiene }                           // two verbs
type Bundler   interface { Bundles() BundleStore }                    // one verb
type Tuner     interface { SetAutoQuarantine, SetAutoShadow, SetAutoPromote } // three verbs

type Curator interface { Forge; Lifecycle; Activator; Outcome; Proposer; Observer; Bundler; Tuner }
```

The module keeps every name as an **alias**, so there is one row and one
spelling of it.

**The RULES travel with the ROWS.** The retrieval ranking decides which skill
is offered to a run, and the directive parser decides what the run's intent
really says once the control line is stripped. Both are pure functions over the
contract's rows, so they moved **with** the rows into the contract — the
layers above the module rank skills and parse directives too, and they may not
name the module to reach either.

**The port is SPLIT, not widened.** `skillapi.Forge` stays at its **five**
verbs: it is the seam the agent-facing tool (a plugin) drives, and every
plugin's fake implements it. The wider surface the kernel, the control plane
and the app layer need is the **`Curator`** — the module's whole public
surface as those layers see it, composed of the narrow ports above so each one
keeps a single meaning.

**The publisher is a port, not the bus.** The bus answers with the durable
**envelope**, and the envelope carries the hash chain — the contract layer may
not name it. The port answers with the event's **id**, which is exactly what a
skill records as its provenance (`SourceEvent`) and what `agt why` reads.

### The library is built by a builder the daemon injects

```go
// cmd/agezt/main_toolinvoker.go
cfg.SkillStore = store
cfg.SkillCurator = func(store skillapi.Store, pub skillapi.Publisher) skillapi.Curator {
    f := skill.NewForge(store, pub)
    if bundles, berr := skill.OpenBundles(filepath.Join(cfg.BaseDir, "skills")); berr == nil {
        f.SetBundles(bundles)
    }
    return f
}
```

```go
// kernel/runtime/compose.go
var forge skillapi.Curator
if cfg.SkillStore != nil && cfg.SkillCurator != nil {
    forge = cfg.SkillCurator(cfg.SkillStore, skillPublisher{b: kbus})
}
```

**The daemon names the module; the kernel does not.** The daemon opens the
library's file and hands over the *builder*; the kernel calls that builder with
the store it was given and its own publisher, so every transition journals
through the kernel's bus without either side naming the other's implementation.

Because the library lives under the **base dir**, a **tenant** curates **its
own** skills — the tenant's kernel is opened with the tenant's base dir, so it
opens the tenant's library.

### A missing library answers, on purpose

- the **kernel** holds a **nil** port: the run carries no skill, the run
  executor proposes nothing, the shadow evaluation judges nothing, and the
  status and the metrics report **zero** — never a panic;
- the **app layer REFUSES** every surface that needs the library with *"the
  skill library is not available on this daemon"* — including the roster
  teardown's archive;
- the **observations' history** still answers: it folds the **journal**, and
  the journal holds the rows whether or not this daemon has a library.

### Two things the slice had to fix on the way

**The dead plumbing.** The module's `WithCorrelation` wrote a context key that
**nothing ever read** (the forge's verbs all take the correlation explicitly),
and the kernel's `SkillStore()` accessor was **called from nowhere** while
being the only reason the kernel needed the file store's *type*. Both are
gone.

**The latent nil.** `k.World().Count()` and `k.Forge().Count()` were called
without a guard in the status, the introspect overview and the metrics — the
world one was a latent hole from W3.54 (a kernel with no world model panicked
in the status), and the skill one would have been the same. Both now report
zero when the port is absent.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/skillapi`: `TestTheCuratorPortIsExactlyTwentyVerbsWide` | the curator port is **exactly twenty** verbs |
| `kernel/contract/skillapi`: `TestTheNarrowPortsKeepTheirWidths` | each narrow port keeps its own width (5/5/1/4/2/1/2/2/1/3) |
| `kernel/contract/skillapi`: `TestTheCuratorNamesOnlyContractRows` | the twenty verbs keep their exact signatures |
| `kernel/contract/skillapi`: `TestThePublisherPortIsOneVerb` | the publisher answers the event's **id** and the error, nothing else |
| `kernel/contract/skillapi`: `TestTheRetrieveRuleTravelsWithTheRows` | the ranking rule decides right — only the **active** pool is offered, a non-matching intent retrieves nothing, the shadow pool is ranked but never injected |
| `kernel/contract/skillapi`: `TestTheDirectiveRuleTravelsWithTheRow` | the parser strips the control line, keeps a bare directive's intent and leaves a plain intent plain |
| `kernel/contract/skillapi`: `TestTheSentinelsAreTheContracts` | the three sentinels exist and are **distinct** |
| `kernel/modules/skills/skill`: `TestTheRowsAreTheContractsRows` | the module's rows are the contract's — by **type identity** |
| `kernel/modules/skills/skill`: `TestTheSentinelsAreTheContracts` | the module's sentinels are the contract's, so `errors.Is` holds |
| `kernel/modules/skills/skill`: `TestTheRulesAreTheContractsRules` | the module's names rank and parse exactly as the contract does |
| `kernel/modules/skills/skill`: `TestTheBundlePortAnswersARealNil` | a forge with no bundle store answers a **real** nil, not a typed nil in an interface |
| `kernel/modules/skills/skill`: the compile-time assertion | the module's forge answers the contract's curator |
| `kernel/runtime`: `TestTheSkillFaceIsTheContractsPort` | the kernel's face is **statically** the contract's port |
| `kernel/runtime`: `TestTheSkillPortStaysNilWithoutALibrary` | a kernel without a library hands over a **nil** port |
| `kernel/runtime`: `TestTheSkillPortAnswersTheLiveLibrary` | the port answers the **live** rows |
| `kernel/app/skill`: `TestTheAppLayerRefusesWithoutACurator` | every surface refuses instead of panicking |
| `kernel/app/skill`: `TestTheRefusalNamesItsReason` | the refusal says **why** |
| `cmd/agezt`: `TestOpenAppKernelWiresTheSkillLibrary` | the daemon end: the library is a **directory on disk** and the port is live |
| `cmd/agezt`: `TestTheSkillLibraryIsTheInjectedOne` | the daemon uses the **injected** library, not one of its own |

### The mutation run

**9 mutations: 9 caught, 0 survived, 0 skipped, 0 build errors** — in **two**
independent passes, both honest (every catch named the expected test or the
expected edge).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the kernel names the module again | **archcheck** — `cross-module kernel/runtime -> kernel/modules/skills/skill` |
| 2 | the control plane names the module again | **archcheck** — `adapter-bypass kernel/controlplane -> kernel/modules/skills/skill` |
| 3 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |
| 4 | the contract's curator port grows a twenty-first verb | `TestTheCuratorPortIsExactlyTwentyVerbsWide` |
| 5 | the kernel's skill face is an anonymous shape, not the contract's port | `TestTheSkillFaceIsTheContractsPort` |
| 6 | the kernel ignores the library it was handed | `TestTheSkillPortAnswersTheLiveLibrary` |
| 7 | the app layer stops refusing without a curator | `TestTheAppLayerRefusesWithoutACurator` |
| 8 | the daemon does not hand the library to the kernel | `TestOpenAppKernelWiresTheSkillLibrary` |
| 9 | **the ranking rule is wrong** (a draft is offered to a run) | `TestTheRetrieveRuleTravelsWithTheRows` |

**Three of the nine are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that opens
the library itself, as long as the skills still show up in the prompt.

**Mutation 5 needed the run executor's declaration to travel with it.** The
kernel's face is also declared in the run executor's `KernelAPI`; drifting only
the kernel's face is a **compile error**, not a catch. The mutation therefore
drifts both declarations — the code compiles, the interface still matches, and
only a **static** check on the declared return type catches it. (The same
lesson as the catalog and world-model slices, met for the third time.)

**Mutation 9 is the one this slice added.** The ranking rule is a
**behaviour**, not a shape: a rule that quietly admits a draft would put a
skill that was never promoted in front of the agent, and no structural check
can see it.

### Two honest corrections

The run earned two, and both are worth naming:

1. **The first attempt at mutation 4 survived.** The harness replaces the
   **first** matching line, and `Count() int` sits in **two** ports — the
   mutation landed on the *store* port, which the curator does not embed, so
   the curator's width did not change. The anchor was narrowed to the
   *hygiene* verb, which is unique, and the mutation then landed where it was
   meant to.
2. **The third run hit a file lock.** A `go run` from the previous pass still
   had the file mapped, and the mutation's write failed. The harness's write
   now retries like its restore already did — a locked file is a harness
   failure, never a silent pass.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 314 packages placed; 28 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 33** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rules the slice is really about

**A bus-bound module cannot be built by the layer that holds the bus.** The
forge journals every transition; the bus lives inside the kernel; the kernel
may not name the module. The way out is the same way it was for the world
model: a **builder the daemon injects** and a **publisher port the kernel
hands over**.

**A wide surface is split, not widened.** The plugin seam (`Forge`, five
verbs) and the kernel's face (`Curator`, twenty verbs) are different ports for
different readers. Widening the five-verb seam would have broken every
plugin's fake for no benefit.

**A missing library answers, it does not panic.** The kernel reports zero, the
app layer refuses and says why, and the journal-based history still answers.

## What remains

The kernel still reaches **7** modules, and the ratchet stands at **28**. The
next candidates:

- **`roster`** — four edges and the **largest fan-in** by far (73 importer
  files); the rows are already in `agentapi`, only the store port and the
  sentinels are missing. The app layer (28 production files) is the bulk of
  the work.
- **`memory`** — six edges; the manager is bus-bound and LLM-bound (distill,
  embeddings), so its port would be the widest of all — and it is the **third**
  bus-bound module, so the builder-plus-publisher pattern applies.
- **`mcp`** — two edges, the smallest remaining family.
