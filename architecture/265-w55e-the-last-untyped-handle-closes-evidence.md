# W5.5e — the last untyped handle closes

## What the slice is

The **forty-seventh** slice of the port programme, and the **fifth** slice of
its **next phase**: the **adapter's** handle.

The phase so far: W5.5a made the adapter's handle **unnamed**, W5.5b moved the
**holding** to the **composition root**, W5.5c closed the **door** (the
constructors take a scope, by type), W5.5d closed the **wiring around** it
(the constructor and its `Deps` are the only way in). What was left was the
**last untyped handle**: the application layer's bridge still handed the
daemon over through an `any`.

## What the slice found

The slice started with a measurement of **one** untyped handle in the whole
kernel:

```go
// kernel/app/binding/ports.go (before)
func Fill[T any](handle any, fill func(*runtime.Kernel) T) T {
	return fill(handle.(*runtime.Kernel))
}
```

The measurement was **wrong**, and the way it was wrong is the finding.

The ratchet that held the count matched the handle's **long names**:

```go
// kernel/app/untyped_handle_test.go (before)
var untypedHandleParameter = regexp.MustCompile(`\b(?:k|kh|kernel|handle)\s+any\b`)
```

**Six** family fills named their handle **`h`**:

```go
// kernel/app/files/ports.go (before)
func RunnerFromHandle(h any) Runner { … }
// kernel/app/steer/ports.go (before)
func RunsFromHandle(h any) Runs { … }
// kernel/app/council/ports.go (before)
func PortsFromHandle(h any, baseDir string) Ports { … }
// kernel/app/research/ports.go (before)
func EnginesFromHandle(h any, newCorrelation func() string) Engines { … }
func ResearchFromHandle(h any) func(…) { … }
// kernel/app/runs/ports.go (before)
func PlanRunnerPortsFromHandle(h any) PlanRunnerPorts { … }
```

A rule that names the shapes it knows is a rule with a hole in it: `h any`
walked straight through `k|kh|kernel|handle` and the count said **one** while
the tree held **seven**. The measurement of this slice is therefore **7 → 0**,
not **1 → 0** — and the rule now knows **two** shapes instead of one.

Every one of the six was a **door**: a value that stands for the daemon's
kernel and whose type the compiler cannot check. The `ok` check inside each
one (`h.(*runtime.Kernel)`) was the family's own refusal — and a refusal for a
handle that, after the typing, **cannot arrive at all**.

## What changed

### The bridge fills the families itself

`binding.Fill` is deleted. The bridge no longer hands a value over and asks a
generic function to do the typing; it calls the **forty** families directly,
each by its own name:

```go
// kernel/app/binding/ports.go (after)
func ScopeFromKernel(k *runtime.Kernel, baseDir string) Scope {
	if k == nil {
		return RefusingScope(baseDir)
	}
	return Scope{
		ApprovalPorts:     appapprovals.PortsFromKernel(k),
		…
		WorldPorts:        appworld.PortsFromKernel(k),
		Directory:         baseDir,
		filled:            true,
	}
}
```

### The families take the daemon, by type

The six `FromHandle(h any)` fills take the daemon's own kernel now, and they
wear the **same name** as the other thirty-four:

| before | after |
|---|---|
| `appfiles.RunnerFromHandle(h any)` | `appfiles.RunnerFromKernel(k *runtime.Kernel)` |
| `appsteer.RunsFromHandle(h any)` | `appsteer.RunsFromKernel(k *runtime.Kernel)` |
| `appcouncil.PortsFromHandle(h any, baseDir string)` | `appcouncil.PortsFromKernel(k *runtime.Kernel, baseDir string)` |
| `appresearch.EnginesFromHandle(h any, newCorrelation func() string)` | `appresearch.EnginesFromKernel(k *runtime.Kernel, newCorrelation func() string)` |
| `appresearch.ResearchFromHandle(h any)` | `appresearch.ResearchFromKernel(k *runtime.Kernel)` |
| `appruns.PlanRunnerPortsFromHandle(h any)` | `appruns.PlanRunnerPortsFromKernel(k *runtime.Kernel)` |

The council's two fills are **one** fill now: `PortsFromKernel` refuses when
there is no daemon, which is exactly what the second one used to do — and the
refusal is the **same** refusal, unchanged in what it answers.

The families' own refusals did not move: a host with no daemon still gets a
runner that refuses, controls that refuse, engines that refuse, a plan runner
that refuses, and a council whose store is nowhere. What moved is **who can
ask for them**: only the daemon's own kernel, by type.

### The tests had to say what they mean

Five family tests used to pass the **string** `"not a daemon"` to prove the
refusal:

```go
// before
runner := RunnerFromHandle("not a daemon")
```

A foreign handle cannot be passed any more — the compiler says so, and the
test says what it actually means: **there is no daemon**:

```go
// after
runner := RunnerFromKernel(nil)
```

## The rule

The ratchet knows **both** shapes now:

```go
var untypedHandleParameter = regexp.MustCompile(
	`\b(?:k|kh|kernel|handle|h)\s+any\b` + // a handle's name, typed as any
		`|\bfunc\s+\w*FromHandle\([^)]*\bany\b`) // a family fill that takes an any handle
```

The second alternative is the **shape** rule: whatever the handle is called,
a family fill that takes an `any` handle is a door. The first is the **name**
rule, and it now knows the short name too.

The rule is held by the rule: the fixture carries all three shapes the door
has worn — the long name, the short name the rule used to miss, and the family
fill under a name the rule does not recognise at all — and the clean fixtures
carry the shapes that are **not** handles (a JSON payload, a `map[string]any`).

## The tests

**5 permanent tests**, three in `kernel/app/binding` and two in `kernel/app`:

- `TestTheBridgeFillsTheFamilysPortsFromTheHandle` — the bridge fills the
  journal, the bus, the tools and the council, and keeps the scope's own
  directory. Before this slice it looked at **one** family and the other
  thirty-nine could have been empty; a bridge that filled the journal and
  left the council empty would have passed it.
- `TestTheBridgesFillIsTyped` — the bridge's fill takes the daemon's own
  kernel, by reflection on the signature itself.
- `TestTheBridgeHandsTheFamilysRefusalThrough` — no daemon, no fill.
- `TestTheKernelNamesNoUntypedHandle` — the count, held at **0**.
- `TestTheUntypedHandleRuleSeesTheViolationItExistsFor` — the rule sees the
  violations it exists for.

The five family refusal tests changed their **subject** — from "a handle that
is not the daemon's own" to "there is no daemon" — and kept their claim.

## The mutations

**13 mutations, 13 caught**, in two independent passes with **byte-identical**
logs.

SHA-256: `64165A501A98F89A2419FCCDB192CC23DCBC0BC5476E9F55226A0CD1A521E382`

No mutation needed a rewrite this time.

- **M01** the untyped fill comes back — the **count rule** catches it.
- **M02** the bridge's fill takes an untyped handle — the **signature rule**
  catches it.
- **M03** the scope forgets its own directory.
- **M04** … **M07** a family is not filled: the journal, the bus, the tools,
  the council. Each one is caught by the test that lives with it.
- **M08** the scope says it is not filled.
- **M09** the refusal becomes a filled scope.
- **M10** … **M12** a family fill takes an untyped handle again: the council,
  the runner, the plan runner — the **count rule** catches all three, which is
  the shape the old rule could not see.
- **M13** the rule forgets the short handle name — the **rule's own fixture**
  catches it.

## The measurement that moved

| | before | after |
|---|---|---|
| the kernel's untyped handles | **7** | **0** |
| the bridge's generic fills | **1** | **0** |
| the family fills that take an `any` handle | **6** | **0** |
| the shapes the ratchet knows | **1** | **2** |
| the families the bridge fills directly | **0** | **40** |
| control-plane production files that name the runtime | **0** | **0** |
| application-owned verbs the guard holds | **170** | **170** |

## What is left

The kernel names no untyped handle any more: the daemon crosses into the
application layer **by type**, and the application hands the adapter a
**scope**, never a daemon.

What is left is the **root's** own shape. The daemon still opens the kernel
and fills the scope in one place (`binding.ScopeFromKernel`), and the
families' ports are still filled one family at a time inside that fill — the
scope could be built by the families rather than by the bridge. And the
`kernel/modules/agents/selfrepair` → `kernel/runtime` edge is still in the
allowlist: the self-repair module's own runtime accessors are the parallel
session's work, and the edge stays until it lands.
