# W5.4l — the adapter stops naming the runtime's run decoration

## What the slice is

The control plane is an **adapter**: it speaks the protocol and hands the work
to the application layer. It must not reach into the runtime's module for the
run's own **decoration** — the agent's profile, the wake provenance, the cost
ceiling, the model chain, the tool allowlist.

Those are decisions the **application** makes. The host that starts a run
decides *why* the run exists and *which* agent runs it, and it says so
without naming the module that will execute the run.

## The measurement

The control plane's non-test files named **11 distinct runtime symbols** across
**20 files**:

| Symbol | What it is | Verdict |
|---|---|---|
| `runtime.Kernel` | the kernel handle the adapter holds | the edge itself — still open |
| `runtime.CouncilMember`, `runtime.CouncilResult` | the Council's rows (the module's api rows, re-exported) | the rows belong in the contract — later slice |
| `runtime.ReaperReport` | the reaper's report (the module's api row) | the rows belong in the contract — later slice |
| `runtime.WithAgentProfile` | binds the profile to a run | **decoration** |
| `runtime.WithWakeContext` | states why the run exists | **decoration** |
| `runtime.WithMaxCost` | the run's cost ceiling | **decoration** |
| `runtime.WithModel` | the run's model | **decoration** |
| `runtime.WithModelChain` | the run's fallback chain | **decoration** |
| `runtime.WithTools` | the run's tool allowlist | **decoration** |
| `runtime.WakeContext` | the provenance row itself | **decoration** |

**Seven** of the eleven were decoration. The adapter was building the runs
itself, in the module's language.

## What moved

### 1. The rows went to the contract layer

The wake provenance and the tool allowlist are **decisions about a run**, not
mechanisms of the runtime — so their rows and their context keys now live in
the contract layer, where the contract layer already keeps such things
(`opapi`, `memoryapi` and `toolapi` all carry context values today):

- `kernel/contract/agentapi/wake.go` — the `WakeContext` row, `WithWakeContext`
  and `WakeContextFrom`.
- `kernel/contract/toolapi/tools.go` — the per-run tool allowlist,
  `WithTools` and `ToolsFrom`.

The runtime keeps its own names as **aliases** (`type WakeContext =
agentapi.WakeContext`) and its setters delegate to the contract. One row, one
spelling; every existing caller compiles unchanged.

### 2. The application owns the decoration

`kernel/app/runs/run_context.go` is the new seam:

```go
func Decorate(ctx context.Context, p agentapi.Profile, wake agentapi.WakeContext, maxCostMc int64) context.Context
func DecorateModels(ctx context.Context, chain []string) context.Context
func DecorateTools(ctx context.Context, tools []string) context.Context
```

The application layer may name the module (L4 → L3 is legal); the adapter may
not (L5 → L3 is the edge this program is closing). The host now asks the
application for the decoration.

### 3. The adapter stopped naming the module's decoration

- `kernel/controlplane/workboard_dispatch.go` — the seat dispatch's context
  decoration now calls `appruns.Decorate` / `DecorateModels` /
  `DecorateTools`. **The file no longer imports the runtime at all.**
- `kernel/controlplane/workflow.go` — the workflow host's wake provenance now
  uses the contract's row. **The file no longer imports the runtime at all.**
- `kernel/controlplane/roster_escalation.go` — the operator wake's decoration
  now calls `appruns.Decorate`; the file keeps naming `runtime.Kernel` (it
  publishes on the kernel's bus) and nothing else.

The adapter now names **4** runtime symbols instead of **11**, and **two**
files left the edge (**20 → 18**).

One behaviour change came with it, and it is a fix: `DecorateModels` treats an
**empty** chain as a no-op. The old adapter code read `chain[0]` without
looking — an empty chain would have panicked.

## The proof

### Permanent tests — 13 new

| Test | What it holds |
|---|---|
| `TestTheAdapterNamesNoRunDecorationFromTheRuntime` (`kernel/controlplane`) | **the slice's product**: the adapter names no runtime decoration symbol. The GATE holds the edge at *package* granularity — as long as the adapter holds the kernel handle, the edge is allowed and the gate says nothing about *which* symbols the adapter names beside it. This test holds it at *symbol* granularity. |
| `TestDecorateCarriesTheProfileTheWakeAndTheCeiling` (`kernel/app/runs`) | the decoration reaches the run: the wake's source and reason, the cost ceiling, the profile's system flag, the acting agent |
| `TestDecorateWithoutACeilingLeavesTheRunUncapped` | a zero ceiling leaves the run uncapped |
| `TestDecorateModelsCarriesTheChainAndItsPrimary` | the chain and its primary reach the run |
| `TestDecorateModelsWithNoChainLeavesTheRunAlone` | an empty chain changes nothing |
| `TestDecorateToolsCarriesTheAllowlist` | the allowlist reaches the run |
| `TestTheWakeContextTravels`, `…IsTrimmed`, `TestAnEmptyWakeContextIsNotAttached`, `TestAPlainContextCarriesNoWake` (`kernel/contract/agentapi`) | the provenance row travels, is trimmed, and an empty one is not attached at all |
| `TestTheToolAllowlistTravels`, `TestAnEmptyAllowlistIsStillARestriction`, `TestAPlainContextCarriesNoAllowlist` (`kernel/contract/toolapi`) | the allowlist travels, and an **empty** allowlist is still a restriction ("no tools", not "every tool") |

### The mutation run — 6 mutations, 6 caught

Two independent passes with **byte-identical** logs (SHA-256
`2C2050A3…4798ADD`).

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01 | the adapter decorates the run itself again — it reaches into the module for the profile | the symbol-grained guard |
| M02 | the application drops the run's cost ceiling | the decoration test |
| M03 | the application drops the model chain | the decoration test |
| M04 | the application drops the tool allowlist | the decoration test |
| M05 | the contract stops trimming the wake provenance | the contract test |
| M06 | the contract treats an **empty** tool allowlist as no allowlist — "no tools" becomes "every tool" | the contract test |

M01 is the one that matters most: it is exactly the regression this slice
exists to prevent, and the guard test names it at the level it happens.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | OK — **2 allowlisted import violations and 0 allowlisted call sites remain** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `go run ./tools/docsindex` | OK |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## The ratchet, and what is honestly still open

The **forbidden-call** ratchet stands at **0** (W5.4k). The **import** ratchet
stands at **2**, and this slice did **not** move it:

```
adapter-bypass  kernel/controlplane -> kernel/runtime
cross-module    kernel/modules/agents/selfrepair -> kernel/runtime
```

The first edge is **narrower, not closed**: the control plane still holds the
kernel handle in **16** files. What this slice removed is the adapter's
knowledge of how a run is *built* — the profile binding, the wake, the
ceiling, the chain, the allowlist. What is left is the handle itself and the
four rows that the modules' api packages still own.

Closing the edge needs the handle itself to stop being a `*runtime.Kernel`:
the adapter has to hold a **port**. That port cannot live in the contract
layer — most of the kernel's verbs return **L3 types** (the roster, the
reaper, the council, the workboard), and a contract may not name a module.
So the port has to live in the **application** layer, which may name both,
and the adapter's native verbs have to move down into the application with
it. That is the redesign the measurement describes, and it is the next
programme of slices, not one of them.

The two rows still named by the adapter (`CouncilMember`, `CouncilResult`,
`ReaperReport`) are the modules' **api** rows. The adapter may not import a
module's api package either (P5 is a module-to-module rule; the adapter's
rule is P2, and it has no api exception), so those rows have to reach the
contract layer first — the same move this slice made for the wake.
