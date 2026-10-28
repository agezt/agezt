# W5.4ab — the policy family's ports, and the decision that must fail closed

## What the slice is

The **fifteenth** slice of the port programme, and the **sixth** one of its
last phase: the handle itself.

The policy engine is the daemon's own authority — the thing every capability is
decided against. The adapter reached for the kernel's engine in **four** places
across **two** files, and handed the engine's two **views** (the read view and
the write view) to two different services.

## What the slice found

| file | sites | whose policy |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 3 | one the daemon's own, two routed |
| `kernel/controlplane/config_handler.go` | 1 | routed |

All four were the same verb: the kernel's **engine accessor** (`Edict`).

| | before | after |
|---|---|---|
| call sites on the handle | **45** | **41** |
| distinct kernel verbs the adapter names | **24** | **23** |

## What changed

The policy family's ports moved into the application
(`kernel/app/edict/ports.go`):

```go
type Ports struct {
	engine *core.Engine
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{engine: k.Edict()}
}
```

and the adapter keeps the binding (`kernel/controlplane/edict_ports.go`) —
which does not name the runtime, like every binding since W5.4v:

```go
func (s *Server) edictPorts() appedict.Ports { return appedict.PortsFromKernel(s.k) }

func requestEdictPorts(ctx context.Context) appedict.Ports {
	return appedict.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
```

The engine is PLATFORM: the kernel always opens one, so a host that was given
none holds a nil engine — and the family's answer for that case is a
**refusal**, not a panic.

### Two views, one engine

The engine is not one capability but **two**: the family reads the policy
(`Levels`, `HardDenyRules`, `AskPolicy`, `Decide`) and the family **writes** it
(`SetLevel`, `SetAskPolicy`, `AddHardDeny`, `RemoveHardDeny`). The port keeps
that split:

```go
func (p Ports) Engine() Engine      { ... }
func (p Ports) WriteEngine() WriteEngine { ... }
```

The adapter no longer hands the engine to the two services itself. The read
service takes `requestEdictPorts(ctx).Engine()`, the writes service takes
`requestEdictPorts(ctx).WriteEngine()` — and the service that only reads
cannot write, because the view it holds does not have the door.

### The refusal fails closed

A host with no engine decides nothing, and the refusal is a **DENY**:

```go
func (p Ports) DecideWithCeiling(capability core.Capability, input string, ceiling core.TrustLevel) core.Outcome {
	if p.engine == nil {
		return core.Outcome{
			Decision:   core.DecisionDeny,
			Capability: capability,
			Reason:     "no policy engine on this daemon",
		}
	}
	return p.engine.DecideWithCeiling(capability, input, ceiling)
}
```

This is the one place in the port programme where the nil case had to be
**decided**, not merely reported. Every other family answers "there is nothing
here" — no records, no engine, no bus. The policy family's answer has a
**direction**: a policy that cannot be asked must not let a capability through.
An empty answer would have been an allow, and the port would have opened a door
it could not close.

### The config reader stopped reaching for the kernel

The fourth site was the **config reader** — the thing that reports the daemon's
live fields to the operator:

```go
func (r nativeConfigReader) AskPolicy() string { return r.kernel.Edict().AskPolicy().String() }
```

The reader now carries the family's ports beside the tools family's ports:

```go
type nativeConfigReader struct {
	kernel *runtime.Kernel
	tools  apptools.Ports
	edict  appedict.Ports
}
```

and asks the family for the answer. The reader still holds the kernel — for the
**base directory**, the **model**, the **plugins** and the **routing** — but the
**policy** is no longer one of its reasons.

## The rule

The policy's verbs are the application's, and the rule is **by whole word**:

> a kernel verb whose name carries "Edict" as a **complete word** is the
> policy family's business.

The whole-word rule is the sharp one here: "Edict" hides inside no other word
of the kernel's, so the rule could have been a substring rule and would have
given the same list. It is written as a whole word anyway, because the rule
must not become looser by accident — the forge's rule had to be sharp because
"Forget" begins with "Forge", and a rule that is looser than it needs to be
will eventually be looser than it means.

The rule is held by the rule in both directions. **Broad**: every policy verb
the kernel has is named by the guard. **Sharp**: the policy's list carries
nothing that belongs to another family, and it is a strict subset of the
kernel's verbs — a rule that only ever grows is a rule that eventually guards
the whole kernel, and a guard that guards the whole kernel guards nothing.

The guard now holds **121** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheEdictPortsReachTheKernelEngine` | the ports reach the kernel's engine: the level the port writes is the level the engine reads, and the ask-policy answer is the engine's own |
| `TestTheEdictPortsRefuseOnAHostWithNoEngine` | a host with no engine hands out no engine, decides **deny**, names the capability, and says why |
| `TestTheAdapterBindsTheEdictPortsToItsOwnKernel` | the daemon's policy answers for the daemon's kernel and the request's for the routed one |
| `TestTheGuardNamesEveryEdictVerbTheKernelHas` | the guard names every policy verb the kernel has |
| `TestTheEdictFamilyClaimsNoOtherFamilysVerbs` | the policy's list claims no other family's verb, and does not claim the whole kernel |

## The mutations

**13** mutations, **13** caught, in two independent passes with **byte-identical**
logs (SHA-256 `31FE3623D6AE4B005F2FE30C0ECDD1B1B1B607299FFB7C46EDFAD167E6C3512D`).

M11, M12 and M13 put the kernel back — the routed consumer reaches for the
kernel's engine again, the daemon's consumer does the same, and the config
reader reaches for it too — and the **verb guard** catches every one of them.
None of them is a behavioural regression: the wiring would work **exactly as
well**, it would simply be in the wrong layer.

M06, M07 and M08 are the ones that bite: the port lets **everything** through
where it should refuse, the refusal loses its **reason**, and the port decides
with no engine at all.

M02 and M03 are the bindings: the daemon's and the request's policy answer for
**nobody**, and the binding test catches each.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | OK — **2 allowlisted import violations and 0 allowlisted call sites remain** (ratchet) |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `go run ./tools/docsindex` | OK |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## What the slice does not do

The handle stays, and the adapter still calls **41** kernel verbs on it: the
providers (**7**), the base directory (**6**), the correlation mint (**3**) and
the rest.

The policy came over in the shape the rest will take: the application fills its
own ports, the adapter asks the family, and the **families that decide stop
reaching for the kernel**.

The **providers** are next, and they are not a copy of this slice: the
providers' application holds the handle itself (`appproviders.New(host.kernel,
server.baseDir)`), so the providers family will have to answer the handle
rule before it can answer the verb rule.
