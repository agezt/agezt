# W3.66 — the pulse's structured-output path is a port

## What the slice is

W3.65 left the ratchet at **10/3**. This slice takes one import edge and the
ratchet goes **10 → 9**:

```
cross-module kernel/pulse -> kernel/agent
```

The forbidden-call ratchet is unchanged at **3**.

The edge was **measured first**, against all twelve remaining import edges
(W3.65's measurement), and it came second in that ranking: the smallest ready
edge in the tree — **one import line, one call site, one verb**.

## What the edge was

`kernel/pulse` is the module that turns a stream of system deltas into the
few that matter. Its optional LLM refine asks a model to score a delta, and
the model's answer has to become a **typed value** — that is the structured-
output path, `agent.GenerateObject`.

The pulse module reached **into the runs module** to call it:

```go
// kernel/pulse/salience.go — before
if _, err := agent.GenerateObject(ctx, s.provider, llm.CompletionRequest{...}, nil, &out); err != nil {
    return 0, "", false
}
```

One import, one call site — and a module reaching into another module for a
**function**. The rule the architecture already has for this is the memory
module's (W3.57): *"a module may not name another module; the host injects
the path."* The memory module got that treatment; the pulse module had not.

## What changed

### The path is the provider contract's

The path is not the memory module's and not the pulse module's — it is
**everybody's**: it is the way a model answer becomes a typed value. So it
lives in the provider's own contract, `kernel/contract/llm`:

```go
// kernel/contract/llm/generator.go
type Generator func(ctx context.Context, p Provider, req CompletionRequest, schema json.RawMessage, out any) (Usage, error)
```

`memoryapi.Generator` is now an **alias** of it:

```go
// kernel/contract/memoryapi/ports.go
type Generator = llm.Generator
```

So there is **one spelling** of the structured-output path in the tree. The
memory module and its daemon wiring keep compiling unchanged — an alias is
the same type — and the pulse module gets the same port without inventing a
second identical type (two named func types with the same shape are **not**
assignable to each other in Go, so a second type would have meant a second
name for one thing, and a conversion at every call site).

### The pulse module asks for a path instead of naming one

`kernel/pulse/salience.go` no longer imports `kernel/agent`. The salience
carries the port:

```go
type Salience struct {
    ...
    generator  llm.Generator // optional; the structured-output path (W3.66)
    ...
}
```

and the refine asks it:

```go
if _, err := s.generator(ctx, s.provider, llm.CompletionRequest{...}, nil, &out); err != nil {
    return 0, "", false
}
```

`pulse.Config` gained the matching field, and the engine carries it into the
salience.

### The daemon injects it

```go
// cmd/agezt/main_pulse_engine.go
eng := pulse.New(pulse.Config{
    ...
    Provider:   k.Provider(),
    Generator:  agent.GenerateObject, // the structured-output path (W3.66)
    ...
})
```

The daemon is the composition root: it names the module that owns the path
and hands the function over. The pulse module never names it.

### The nil semantics are written down

A host that wires **no** path gets **no** refine: the severity score stands
and nothing pretends a model was asked. The gate is explicit:

```go
if s.useLLM && s.provider != nil && s.generator != nil {
```

That is the same posture the memory module's distillers take ("a nil
generator means no structured output available — report that, rather than
pretending you distilled something"), and it is why the refine **cannot**
panic on a nil path.

## The proof

### Permanent tests

`kernel/pulse/salience_llm_test.go` (five tests) — the refine path was
**entirely untested** before this slice; nothing in the twelve pulse test
files covered it.

| Test | What it holds |
|---|---|
| `TestTheLLMRefineUsesTheInjectedPath` | the refine asks the **injected** path, the score and the reason are the model's, and the model is asked **about this change** (the delta's summary is in the request, and the system prompt asks for a score) |
| `TestTheLLMRefineIsOffWithoutTheSwitch` | the refine is **opt-in** (`PULSE_LLM=on`): with the switch off the path is never asked — a pulse that refined on every beat would spend a model call per delta, unasked |
| `TestTheLLMRefineSkipsWhenNoPathIsInjected` | no path means no refine, and **no panic** |
| `TestTheLLMRefineSkipsWhenThePathFails` | an unusable answer is a **skip**, not a zero score: the severity score stands |
| `TestTheLLMRefineClampsTheScore` | the model's score is bounded to **[0,1]** — a model that answers 5.0 must not turn a medium delta into an alert |

`cmd/agezt/pulse_port_test.go` (one test) — the **daemon's** wiring, proven
end to end: the daemon opens a kernel with a mock provider, builds the pulse
engine with `PULSE_LLM=on`, adds an observer, beats once, and waits for the
journal to carry a `pulse.salience` event whose reason is the **model's**
reason (`"llm: the disk is about to fill"`) and whose score is the model's
(0.87).

That test is the reason the slice is not just a rename. Without it, a daemon
that forgot to inject the path would look **exactly** like a daemon with the
LLM refine switched off: the pulse keeps working, it just gets quieter, and
nothing anywhere reports that the refine is dead. The daemon's own end-to-end
path sees it — the same shape as the memory slice's distillation test.

### The mutation run

**10 mutations: 10 caught, 0 survived, 0 build errors, 0 skipped** — in **two**
independent passes, with **byte-identical** logs (compared case-sensitively).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the refine runs even with **no** path injected | `TestTheLLMRefineSkipsWhenNoPathIsInjected` |
| 2 | the refined reason loses its `"llm: "` marker | `TestTheLLMRefineUsesTheInjectedPath` |
| 3 | a negative model score is not clamped | `TestTheLLMRefineClampsTheScore` |
| 4 | a score above one is not clamped | `TestTheLLMRefineClampsTheScore` |
| 5 | the refined score is discarded | `TestTheLLMRefineUsesTheInjectedPath` |
| 6 | a failed path counts as a **successful** refine | `TestTheLLMRefineSkipsWhenThePathFails` |
| 7 | the refine ignores the opt-in switch | `TestTheLLMRefineIsOffWithoutTheSwitch` |
| 8 | the model is not told **what changed** | `TestTheLLMRefineUsesTheInjectedPath` |
| 9 | **the daemon stops injecting the path** | `TestTheDaemonWiresTheStructuredOutputPathIntoPulse` |
| 10 | the engine does not carry the path into the salience | `TestTheDaemonWiresTheStructuredOutputPathIntoPulse` |

Mutations 9 and 10 are the ones that make this slice more than a move: they
are the two halves of the wiring (the daemon's hand-over and the engine's
carry), and only the **daemon's** end-to-end test sees them.

The **first** pass reported **three build errors**, and all three were the
**mutation's** defect rather than the code's — the same class this series has
met five times now:

- the two clamp mutations removed a brace, so the file did not compile (a
  build error says nothing about the behaviour — the mutations now move the
  *threshold* instead, so the file still compiles and only the guarantee is
  broken);
- the mutation that removes the daemon's injection also has to remove the
  `kernel/agent` **import**, and the harness's `extra` edit was written in a
  single-quoted PowerShell string, where a backtick-t is a literal
  backtick-t and not a tab. The import survived, `agent` became unused, and
  the package stopped compiling. The harness now writes that edit in a
  double-quoted string, where `` `t `` is a tab.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 314 packages placed; 9 allowlisted import violations and 3 allowlisted call sites remain (ratchet)** — **import edges down from 10** |
| `go run ./tools/deadcodecheck` | OK — 31 public SDK findings and 1 cross-package test seam allowlisted |
| `go run ./tools/depscheck` | OK — 24 core dependencies, all justified |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean (207 packages in the codemap) |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `staticcheck ./...` | clean |
| `go test ./... -count=1` | **all green** |

## What remains

The ratchet stands at **9/3**. The nine import edges are:

| Kind | Edges |
|---|---|
| `adapter-bypass` | `controlplane → modules/extensions/toolbox`, `controlplane → runtime`, `controlplane → update`, `restapi → update` |
| `cross-module` | `cadence/systemtasks → runtime`, `modules/agents/selfrepair → runtime` |
| `upward` | `channel → bus`, `runtime → agentgw`, `runtime/runexec → agentgw` |

The measurement's traps still stand, and they are the map for the next
slices:

- **`controlplane → runtime`** — the control plane holds the kernel's
  concrete `*runtime.Kernel` and calls **37** verbs on it. Closing it is a
  redesign: the control plane must reach the kernel through app operations,
  not through the kernel's god-object.
- **`selfrepair → runtime`** — the six agent rows could move to the reaper's
  `api` package, but `RunWith`, `RunWithRetry`, `ReaperScan` and
  `NewCorrelation` need a **runs port that does not exist** yet. That port is
  a slice of its own, and this edge cannot close before it.
- **`channel → bus`** — 26 plugin `Config`s hold `*bus.Bus` and nil-check it.
  An interface parameter turns a nil `*bus.Bus` into a **non-nil** interface,
  the guard stops firing, and the publish panics inside the deferred recover.
- **`runtime → agentgw`** and **`runtime/runexec → agentgw`** — **free
  riders**: `k.AgentGateway()` has **no caller** outside its own declaration.
  The fix is not a port but a **move** (the daemon builds the gateway), and
  the price is that the gateway's *wiring* is untested today — the same
  "wiring nobody proves" shape this slice just fixed for the pulse.

And the **forbidden-call** ratchet is still at **3**, with two holes the
measurement found and this slice did not touch: the staged writes are not
**fsynced** (so a swapped-in tree can hold a half-written file), and the
swap **removes the old copy before** the new one is in place (so a failure
there loses the content, and a partial removal leaves a half-deleted tree).

## The rules the slice is really about

**A port is not a rename.** The pulse module and the memory module needed the
same function, and the naive port — one type per module — would have produced
two named types with the same shape. Go does not assign those to each other,
so every call site would have needed a conversion: two names for one thing.
The port belongs to the contract that owns the **provider**, and the memory
module's name is now an alias. One spelling, and the modules stay honest
about what they do not own.

**A module that reaches into another module for a function is paying for a
layer it does not need.** The pulse module reached into the runs module to
call one function. The host was already holding the very same function — the
daemon injected it for the memory module three slices earlier. The rule was
written; the pulse module had simply not been held to it.

**A feature that is wired but untested is a feature that is off.** The refine
had no test at all, and the daemon's wiring had no test either. A daemon that
forgot to inject the path would not fail — it would get quieter, and nothing
would say why. The daemon's end-to-end test now sees the difference between
"the refine answered" and "the refine is dead", because the difference is
**visible in the journal** and nowhere else.

**A mutation that leaves an import unused does not compile, and a harness
that does not apply all of its edits measures its own omission.** The fifth
time this series has met that shape. The rule is mechanical every time: the
mutation removes the import it made unused, and the harness's `extra` edit is
part of the mutation, not an afterthought — and it must be written in a
PowerShell string that actually contains a **tab**.
