# W5.4z — the skill family's ports, and the rule that had to learn a word

## What the slice is

The **thirteenth** slice of the port programme, and the **fourth** one of its
last phase: the handle itself.

The skill library is the daemon's own file (W3.55) — the forge that holds the
skills. The adapter reached for the kernel's forge in **seven** places across
**three** files.

## What the slice found

| file | sites | whose library |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 4 | the routed one — the tenant's library |
| `kernel/controlplane/roster_teardown_state.go` | 2 | the daemon's own |
| `kernel/controlplane/roster_cascade.go` | 1 | the daemon's own |

Four of the seven were **routed** — a tenant's skills are the tenant's skills —
and three read the daemon's own. The slice keeps that split exactly:
`requestSkillPorts(ctx)` answers for the routed kernel, `s.skillPorts()` for the
root.

The adapter's handle-verb debt, measured across the whole control plane:

| | before | after |
|---|---|---|
| call sites on the handle | **57** | **50** |
| distinct kernel verbs the adapter names | **28** | **27** |

## What changed

The skill family's ports moved into the application
(`kernel/app/skill/ports.go`):

```go
type Ports struct {
	curator skillapi.Curator
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{curator: k.Forge()}
}
```

and the adapter keeps the binding (`kernel/controlplane/skill_ports.go`) —
which does not name the runtime, like every binding since W5.4v:

```go
func (s *Server) skillPorts() appskill.Ports { return appskill.PortsFromKernel(s.k) }

func requestSkillPorts(ctx context.Context) appskill.Ports {
	return appskill.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
```

The curator is held **unexported**, like the bus's: the library may be absent,
and the port answers for that case. The adapter used to spell "there is no
skill library" **three** times across **two** files — twice as a nil check and
once as a refusal it wrote out in full:

```go
return errors.New("skill: the skill library is not available on this daemon")
```

The application already had that refusal, spelled the same way
(`errNoSkillLibrary`), for its own services. Now there is **one** spelling of
it, in the application, and the port hands it out.

The port offers the library's narrow views — `Reader`, `Curation`,
`Lifecycle`, `Observation` — because the skill application's own services
already declare them, and the narrow views are the consumers' business.

## The rule

The forge's verbs are the application's, and the rule is **by whole word**:

> a kernel verb whose name carries "Forge" as a **complete word** is the
> forge's business.

The word rule is new, and it was not a preference. The substring rule
over-captures here: **"Forget" begins with "Forge"**, so the memory's
`MemoryForget` would have been handed to the forge — and forgetting a memory
is not the forge's business. The rule is sharp, not only broad, and it is held
by its own test (`TestTheSkillFamilyDoesNotClaimTheMemorysVerbs`).

The family answers for four verbs — `Forge`, `MaybeForge`, `SkillForge` and
`SkillForgeMinTools` — and leaves the **tool** forge (`ToolForge`) to the
tools family, which named it first. The guard now holds **105**
application-owned verbs.

### Two more collisions the slice had to clear

The guard reads the adapter's **prose** as well as its code, and the slice
found two names that answered to a kernel verb:

1. **The tools port's field was called `Forge`.** It holds the *script-tool*
   forge — a different forge — but a port field that answers to a kernel verb's
   name is a verb the adapter may name, and the adapter holds that port. The
   field is now `ScriptForge`, which says what it is. This is the **fifth**
   time this programme learned that lesson (`MCPAttached` → `Attachment`,
   `Rollup` → `Progress`, the workboard's `Journal` → `Events`, the workflow's
   `Journal` → `Events`, and now the tools' `Forge` → `ScriptForge`).

2. **A comment named the verb.** The command catalogue's comment said
   "transitions go through the kernel's `skill.Forge`", and the guard — which
   reads the raw source — saw a selector. The prose says "the kernel's skill
   forge" now: the guard reads what the adapter says, and a comment that says
   a verb's selector says it.

## The tests

| test | holds |
|---|---|
| `TestTheSkillPortsReachTheKernelLibrary` | the ports reach the kernel's library: the port lists the library's skills and its archive really archives |
| `TestTheSkillPortsSurviveAHostWithNoLibrary` | a host with no library lists nothing, refuses an archive and hands out no reader |
| `TestTheAdapterBindsTheSkillPortsToItsOwnKernel` | the daemon's library answers for the daemon's kernel and the request's for the routed one |
| `TestTheGuardNamesEverySkillVerbTheKernelHas` | the guard names every forge verb the kernel has |
| `TestTheSkillFamilyDoesNotClaimTheMemorysVerbs` | the rule is sharp: the memory's verbs are not the forge's |

## The mutations

**13** mutations, **13** caught, in two independent passes with **byte-identical**
logs (SHA-256 `04B0CC0C2D03C7BB29AF874311CBB41149A160E3C56A827F88DE9A4C78550595`).

M11, M12 and M13 put the kernel back — the routed consumer reaches for the
kernel's forge again, the daemon's consumer does the same, and the tools port
answers to the kernel verb's name again — and the **three rules** catch every
one of them. None of them is a behavioural regression: the wiring would work
**exactly as well**, it would simply be in the wrong layer.

M05 is the sharp one: it puts the substring rule back, and the memory's
`MemoryForget` walks into the forge's list again. The rule's own test catches
it — the rule is asked whether it is **sharp**, not only whether it is
**broad**.

M07 survived the first calibration, and it is worth reading twice: it makes
the port's archive do nothing, and **nothing caught it** — because the test
asked the wrong question. The test asked "is the skill still active?", and a
newly created skill is a **draft**, so the answer was "no" before and after
the archive. The test now asks whether the skill the port archived really is
**archived** — and it is the second time this programme a mutation has found a
**missing assertion** rather than a missing rule.

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

The handle stays, and the adapter still calls **50** kernel verbs on it: the
providers (**7**), the base directory (**6**), the memory (**5**), the policy
engine (**4**), the correlation mint (**3**) and the rest.

The forge came over in the shape the rest will take: the application fills its
own ports, the adapter asks the family, and the **families that read the
library stop reaching for the kernel**.
