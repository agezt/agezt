# W5.4m — the adapter names the runtime only as a handle

## What the slice is

After W5.4l the control plane named **four** runtime symbols: the kernel
handle and **three module rows** (`CouncilMember`, `CouncilResult`,
`ReaperReport`).

This slice leaves the adapter with **one**: the handle.

## The measurement — and the surprise in it

The three rows looked like they needed the module. They did not.

The **reasoning** module's api rows are **already contract rows**:

```go
// kernel/modules/reasoning/api/api.go
type CouncilMember = reasoningapi.CouncilMember
type CouncilResult = reasoningapi.CouncilResult
```

`reasoningapi` lives in `kernel/contract/` — the contract layer, which the
adapter may name freely. So `runtime.CouncilMember` and
`reasoningapi.CouncilMember` were **the same type all along**, and the adapter
was reaching for the runtime's alias of a contract row. Naming the contract
row directly is a spelling change, not a conversion.

The **reaper's** rows were the exception: they were declared in the agents
module's api package. But they cross the boundary too — the module produces
them, the application layer (the operator's scan) reads them, and the adapter
hands them on — so their home is the contract layer, exactly like the wake
provenance and the tool allowlist were in W5.4l.

## What moved

**`kernel/contract/agentapi/reaper.go`** (new) holds the reaper's rows: the
dead-agent candidate, the degraded agent, the misconfigured agent, the three
pressure findings, the forced finding (one row, three names) and the report
itself.

**`kernel/modules/agents/api/api.go`** re-exports them as aliases — one row,
one spelling, every importer compiles unchanged.

**The adapter** now names the contract's rows:

| File | Before | After |
|---|---|---|
| `app_operations.go` | `runtime.CouncilMember`, `runtime.CouncilResult` | `reasoningapi.CouncilMember`, `reasoningapi.CouncilResult` |
| `roster_status_views.go` | `runtime.ReaperReport` | `agentapi.ReaperReport` |

Both files **stopped importing the runtime at all**: **18 → 15** files.

## The proof

### Permanent tests — 3 new, 2 moved

| Test | What it holds |
|---|---|
| `TestTheAdapterNamesTheRuntimeOnlyAsAHandle` (`kernel/controlplane`) | **the slice's product**: the adapter may name the runtime's `Kernel` type and **nothing else**. The test reads each file's own import to learn the runtime's name in it, so the check is precise — a file that imports the kernel runtime may only use its handle. |
| `TestTheReaperReportCarriesEveryKey` (`kernel/contract/agentapi`) | the report keeps every key — a missing key is a missing finding |
| `TestTheReaperRowsAreTheContractRows` (`kernel/modules/agents/api`) | the module's rows **are** the contract's rows (identity, checked at runtime) |
| `TestFindingRowsCarryEveryField`, `TestEmptyReportsNothingToReap` (**moved** to the contract) | the rows' shape and the report's emptiness — the tests moved with the rows |

The identity test is worth a word: the module's rows and the contract's rows
are the same type, and that is a **compile-time** fact everywhere they are
used. The test holds it at runtime anyway, because the moment the module
declared its own copy, the two would drift the moment one of them changed —
and the drift would be invisible to every test that only reads a report.

### The mutation run — 6 mutations, 6 caught

Two independent passes with **byte-identical** logs (SHA-256
`07ECFBA0…D19C834BE`).

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01 | the adapter names the module's council row again | the handle-only guard |
| M02 | the adapter names the module's reaper row again | the handle-only guard |
| M03 | the contract's degraded-agent row loses a key | the row-shape test |
| M04 | the report calls a stale artifact "nothing to reap" | the emptiness test |
| M05 | the forced finding loses its chain key | the row-shape test |
| M06 | the report loses a key | the report-key test |

The calibration run found **three faulty mutations** — an anchor that matched
three rows instead of one, an anchor that matched nothing, and a mutation
that broke the build. All three were the mutation's fault, not the test's,
and all three were rewritten.

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

## What is honestly still open

The **forbidden-call** ratchet stands at **0**. The **import** ratchet stands
at **2**:

```
adapter-bypass  kernel/controlplane -> kernel/runtime
cross-module    kernel/modules/agents/selfrepair -> kernel/runtime
```

The first edge is **narrower, not closed**. What is left is the handle itself:
the adapter holds `*runtime.Kernel` in **15** files, and every verb it calls
on that handle is a call into a module from an adapter.

Closing it needs the adapter to hold a **port** instead of the handle — and
the port cannot live in the contract layer, because most of the kernel's verbs
return **L3 types** (the roster, the reaper, the council, the workboard) and
a contract may not name a module. So the port has to live in the **application**
layer, and the adapter's native verbs have to move down into the application
with it. That is the next programme of slices, not one of them.

What this slice did do is make the remaining work **mechanical**: the adapter
no longer knows a single module row. When the handle becomes a port, the
adapter will already be speaking only contract rows and application
operations — there is nothing else left for it to unlearn.
