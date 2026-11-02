# W3.76k — the daemon's dead surface dies

**Slice:** W3.76k (the nineteenth of the runtime's dissolution)
**Base:** `f9212b7c` (the W3.76j merge)
**Date:** 2026-11-02

---

## 1. The family

The **daemon's dead surface** is not a package — it is a set of *verbs*: the
public methods on `*Kernel` that **no caller reaches**.

The target architecture dissolves the kernel ("`kernel/runtime` (284
methods) → **dissolved**", `architecture/20-target-architecture.md`), and the
W3 exit is the walk that does it. The walk so far moved whole families out
(W3.76h the loop, W3.76j the run engine). This slice shaves the surface that
the moves left behind: the verbs the daemon kept for a family it no longer
owns, for a module it no longer drives, or for a caller that has been gone
for a long time.

The measurement: the daemon held **238** public verbs. **Nineteen** of them
had no caller outside the daemon — the number the scan found, and **sixteen**
the slice retired (three of the nineteen turned out to be *port entries*, not
verbs: see §4). Two more went with them: the daemon kept **two names for one
operation** (`DistillBrain`/`DistillProfile` and `MemoryDistillBrain`/
`MemoryDistillProfile` beside them), and the second spelling is gone.

The retired eighteen, with the reason each one died:

| verb | file | why it died |
|---|---|---|
| `ArtifactStore` | `runtime_lifecycle.go` | the run engine's port listed it; the engine never asked |
| `PublishBusEvent` | `runtime_lifecycle.go` | the deleted `Accessor` sub-package's chokepoint (dead since W3.40) |
| `Scheduler` | `runtime_accessors.go` | the DAG executor is the daemon's own field; nobody asked for the getter |
| `RunTimeoutFromCtx` | `accessors_ctx.go` | the daemon's forwarding copy of the runs module's vocabulary |
| `MergeAutoApproveCapabilities` | `accessors_ctx.go` | the daemon's forwarding copy of the runs module's vocabulary |
| `MemoryActive` | `accessors_memory.go` | the memory family is the knowledge module's; nobody asked |
| `MemoryProfileText` | `accessors_memory.go` | same |
| `MemorySearchScoped` | `accessors_memory.go` | same |
| `MemoryRecallScoped` | `accessors_memory.go` | same |
| `MemoryForget` | `accessors_memory.go` | same |
| `MemoryDistillFacts` | `accessors_memory.go` | same |
| `MemoryDistillBrain` | `accessors_memory.go` | a second name for `DistillBrain` — one operation, one spelling |
| `MemoryDistillProfile` | `accessors_memory.go` | a second name for `DistillProfile` |
| `ObjectiveRollup` | `okr.go` | the OKR family is the work module's; nobody asked |
| `SetForge` | `scripttool.go` | the composition root wires the forge through `Config.ForgeBuilder` |
| `SetMCPRegistry` | `mcptool.go` | the composition root wires the registry through `Config.MCPBuilder` |
| `MCPRegistration` | `mcptool.go` | the daemon's own reader; the row surface above it answers the same rows |
| `MCPRegistrations` | `mcptool.go` | same |

## 2. The measurement that found them

The scan walks the daemon's production files, takes the exported methods on
`*Kernel` (**case-sensitively** — the `-match` trap costs 46 phantom
methods), and asks the tree whether anything outside `kernel/runtime` calls
each one.

Two traps had to be dodged, and both of them are the reason the slice is
small but exact:

1. **Interface satisfaction is a caller.** The scan said `ArtifactStore`,
   `Scheduler`, `RunTimeoutFromCtx` and `MergeAutoApproveCapabilities` had
   no caller — but the **compiler** disagreed: `*Kernel` did not satisfy
   `runner.KernelAPI` without them. They were callers after all, *through
   the port*. The port listed four verbs the run engine never opened, so
   the slice removed them from **both** sides: the port lost four methods,
   the daemon lost four verbs. The compiler is the arbiter here, and it is
   what caught the scan's blind spot.
2. **A name can be a field, not a call.** The scan matches `\.Name\b`, so a
   struct field (`cfg.ToolForge`) looks like a call. The final list was
   verified one by one — production callers, test callers and *string*
   occurrences (reflection, route tables) — before anything was deleted.

## 3. What the daemon looks like now

| measure | before | after |
|---|---:|---:|
| the daemon's public verbs | 238 | **220** |
| the daemon's production files | 54 | 54 |
| the daemon's production lines (non-blank) | 6,281 | **6,200** |
| the run engine's port (`runner.KernelAPI`) | ~85 methods | **~81** |

The daemon keeps the **port** over the memory (`k.Memory()`), the **count**
the app layer reads (`kernel/app/introspect/ports.go:113`,
`kernel/app/system/ports.go:144`) and the two **distill doors** the app
layer and the pulse timer call (`kernel/app/memory/ports.go:123,127`,
`cmd/agezt/main_pulse.go:109,138`). The six operations nobody read are gone,
and the two distill operations keep **one** spelling: the daemon used to
answer both `DistillBrain` and `MemoryDistillBrain` for the same work.

So the daemon keeps the *port* of a family it does not own and does not keep
a verb per operation of that family.

**After the slice, the daemon's public surface has no dead verb left**: the
scan finds **one** verb with no caller outside the daemon — `DescribeImages`,
which the daemon's own production path opens (the image admission). Every
other public verb is a door somebody above the daemon opens. A public verb on
the host is now either a door somebody opens or a door the host opens itself.

The two wiring setters died because the production path never used them:
`compose.go:389-392` builds the forge and the registry services from the
Config builders and stores them in the kernel's fields. The setters were
left over from the pre-W3.72 wiring, and their only caller was the daemon's
own test.

## 4. The runner's port

`runner.KernelAPI` is the run engine's only channel to its host — about
eighty-five methods, of which four were never opened:

```go
RunTimeoutFromCtx(ctx context.Context) time.Duration
MergeAutoApproveCapabilities(ctx context.Context, from map[string]bool) map[string]bool
Scheduler() *scheduler.Executor
ArtifactStore() artifactapi.Blob
```

The engine reads the run's dispatch through the **module** (`runctx.`), not
through the daemon's forwarding copies, and it never asks for the scheduler
or the artifact store. The port is the seam between the module and the host,
so a verb in it is a *promise* the module may keep relying on — and the
promise was never kept.

## 5. The tests that moved with it

The daemon's own tests were the only callers of six of the retired verbs,
so they are where the work shows:

- **the memory port test** (`memory_port_test.go`) now asks the **port**
  (`k.Memory().Active()`) instead of the daemon's per-operation verbs, and
  keeps the refusals that are still the daemon's (the two distill doors);
- **the OKR tests** (`okr_heavycov_test.go`, `okr_module_test.go`) ask for
  the rollup of the objective they already hold (`k.Rollup(obj)`) instead of
  the retired "fetch by id and roll up" verb;
- **the forge and MCP module tests** now hand their fake over the way the
  composition root hands it over — `runtime.Config.ForgeBuilder` /
  `runtime.Config.MCPBuilder` — so they exercise the **production wiring**
  instead of a test-only setter. That is a strictly better test than the one
  it replaces;
- **the MCP port test** reads the row surface (`MCPServers` / `MCPServer`),
  which is what the agent-facing tool reads anyway.

## 6. The ratchet — and its probe

`kernel/runtime/dead_surface_ratchet_test.go` (new) pins the slice:

1. **the eighteen retired names are not methods of `*Kernel` any more** —
   read from the package's own sources with `go/ast`, so a name that comes
   back fails the test;
2. **the daemon's public surface does not grow past 220** — the ceiling this
   slice reached. A new verb needs a new reason, and the ratchet says so;
3. **the daemon does not speak for the families it does not own** — the
   memory's port and its count stay, the six operations do not, and the
   distill operations keep one name.

**The probe.** A ratchet that is never probed never fires (the lesson
W3.76i paid for). Putting `PublishBusEvent` back into
`runtime_lifecycle.go` made **both** ratchets fail in one run:

```
--- FAIL: TestTheRetiredVerbsDoNotComeBack
    dead_surface_ratchet_test.go:91: *Kernel has the PublishBusEvent method again …
--- FAIL: TestTheDaemonKeepsItsSurfaceFromGrowing
    dead_surface_ratchet_test.go:105: the daemon holds 223 public verbs, want at most 222 …
```

The probe was reverted and the tree scanned for leftovers (none).

## 7. The ratchets that changed honestly

Two of the daemon's own ratchets named the verbs this slice retired, and
they say so:

- `submanager_test.go` — the accessor surface is pinned **by name**, so
  `Scheduler` and `PublishBusEvent` left the list and their **absence** is
  pinned now, next to `ConfigMu` and `AgentGateway`, which went the same way
  in W3.40 and W3.67.
- `runctx_ratchet_test.go` — the check that the daemon reads the run's
  dispatch through the module was **file-scoped** (`accessors_ctx.go`), and
  the two forwarding copies this slice retired lived in that file. The check
  is now **package-scoped**, which is both stronger (the readers that drive a
  run are the ones that use the module's names) and honest (it no longer
  pins a door that does not exist).
- `mcp_module_test.go` — the daemon's MCP verb list lost the two internal
  readers; the list now names the row surface and pins the two readers'
  absence.

## 8. Verification

- **the compiler**: `go build ./...` and `go vet ./...` clean — every caller
  and every port is type-checked;
- **the ratchet probe** (§6) — both new ratchets fire;
- **the full test suite**: `go test ./...`;
- **the thirteen gates**: gofmt, build, vet, archcheck, deadcode, depscheck,
  sdkparity, structure-md, changelog-lint, docclaims, docsindex, staticcheck,
  test — **0 failures**.

**No mutation run.** This slice deletes code and moves callers; there is no
rule whose reversal a mutation could expose. The evidence of a deletion is
the compiler (every caller type-checked), the call-site scan (§2), the
ratchet and its probe (§6), and the full suite — the same four legs W3.76i
and W3.76j stood on.

## 9. What is left

The daemon holds **220** public verbs and **6,200** production lines, and
**none of its public verbs is dead any more** — the scan's last one
(`DescribeImages`) is a door the daemon opens itself.

The rest of the W3 exit is unchanged: the daemon's composition root
(`compose.go`, 497 lines), its `Kernel` (`runtime.go`, 665), the accessors
family (7 files, 965) and the control plane (`kernel/controlplane`, 7,570).

Evidence: `architecture/284-w376k-the-daemons-dead-surface-dies-evidence.md`
