# W5.4ac — the provider's door, and the two sites the measurement could not see

## What the slice is

The **sixteenth** slice of the port programme, and the **seventh** one of its
last phase: the handle itself.

The provider is the daemon's live door to the model: the routing tables it
routes by, the routing view it reports, the spend it keeps and the completion
the chat and plan families drive. The adapter reached for the kernel's provider
in **nine** places across **two** files.

## What the slice found

| file | sites | whose provider |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 8 | the daemon's own |
| `kernel/controlplane/config_handler.go` | 1 | routed |

Seven of the nine were calls and two were **method values** — the provider
handed over as a value, not called:

| site | what it reached for | who asked |
|---|---|---|
| `app_operations.go` (budget) | `k.Provider().(*governor.Governor)` | the budget family |
| `app_operations.go` (routing, ×4) | `k.Provider().(taskChainsReader)` and friends | the routing family |
| `app_operations.go` (mission control) | `k.Provider().(*governor.Governor)` | the mission control family |
| `app_operations.go` (chat summary) | `k.Provider` — a **method value** | the chat family |
| `app_operations.go` (plan authoring) | `k.Provider` — a **method value** | the runs family |
| `config_handler.go` | `k.Provider().(appconfig.RoutingReader)` | the config family |

| | before | after |
|---|---|---|
| call sites on the handle | **41** | **34** |
| distinct kernel verbs the adapter names | **23** | **22** |
| method values handed over | **17** | **15** |

### The measurement and the guard saw different things

The established measure counts **calls** on the handle — `k.Provider()` — and
the provider family had **seven** of them. The two method values (`k.Provider`
without parentheses) were invisible to it: a measurement that counts calls
cannot see a value.

The **guard** reads the adapter's raw source, so it saw all nine, and it
caught the two that were left behind the moment the family was declared. The
measurement is the one that reports progress; the guard is the one that
reports truth. Both are needed, and the slice made that difference visible.

## What changed

The provider family's ports moved into the application
(`kernel/app/provider/ports.go`):

```go
type Ports struct {
	provider llm.Provider
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{provider: k.Provider()}
}
```

and the adapter keeps the binding (`kernel/controlplane/provider_ports.go`) —
which does not name the runtime, like every binding since W5.4v:

```go
func (s *Server) providerPorts() appprovider.Ports { return appprovider.PortsFromKernel(s.k) }

func requestProviderPorts(ctx context.Context) appprovider.Ports {
	return appprovider.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
```

### One door, four faces

The daemon's live provider is the **governor**, and the governor carries more
than one face: the routing tables it routes by, the routing view it reports
and the spend it keeps. The adapter used to reach through the door once per
face, asserting the provider to a different interface each time.

The port names the faces as **capabilities**, and each consumer asks for the
one it needs:

```go
type Chains interface { ... }   // the routing tables
type Routing interface { ... }   // the routing view
type Budget interface { ... }    // the spend keeper
```

A family that asks for the chains never learns what the governor is.

### The budget's crossing is the budget's business

The adapter used to hold the conversion — the governor's snapshot crossed into
the budget's own row shape by a wrapper that lived in the adapter. The wrapper
moved into the budget family (`kernel/app/budget/ports.go`), where the
conversion belongs:

```go
func GovernorFrom(s Spender) Governor {
	if s == nil {
		return nil
	}
	return spenderGovernor{s: s}
}
```

The keeper's ceiling answers "how much is left", and the budget does not
report that — so the crossing drops it, in the budget's own code.

### The port may not carry the name of the verb it stands for

The port's first name for the provider itself was `Provider()`. The guard
refused it:

```
app_operations.go reaches the kernel's Provider — that verb belongs to the application layer
```

The adapter that asks `providerPorts().Provider` was seen reaching for the
kernel's own verb. This is the **fifth** time a port's own name has collided
with a kernel verb (the tools' `Forge` became `ScriptForge`), and the rule is
the same: a port answers under a name the adapter can say out loud.

The name is now `Live()` — the live provider, which is what it is.

## The rule

The provider's verbs are the application's, and the rule is **by whole word**:

> a kernel verb whose name carries "Provider" as a **complete word** is the
> provider family's business.

Only one kernel verb carries the word, so the family holds **one** verb and
the guard now holds **122** application-owned verbs.

The rule is held in both directions, as the policy slice's rule was: **broad**
(every provider verb the kernel has is named by the guard) and **sharp** (the
provider's list claims no other family's verb and does not claim the whole
kernel).

## The tests

| test | holds |
|---|---|
| `TestTheProviderPortsReachTheKernelProvider` | the ports reach the kernel's provider: the chains read and written are the daemon's, the routing view is the daemon's, the spend is the daemon's |
| `TestTheProviderPortsSurviveAHostWithNoProvider` | a host with no provider hands out no provider, reads no chains, holds no routing view and no budget |
| `TestTheProviderPortsRefuseAProviderThatCarriesNoFaces` | a provider that carries none of the faces answers "absent", not "empty" |
| `TestTheBudgetsGovernorComesFromTheSpendKeeper` | the keeper's snapshot crosses as the budget's own rows, and the ceiling crosses as a question |
| `TestTheBudgetHasNoGovernorWithoutASpendKeeper` | a host with no keeper has no governor — not a governor that reports zero |
| `TestTheAdapterBindsTheProviderPortsToItsOwnKernel` | the daemon's provider answers for the daemon's kernel and the request's for the routed one |
| `TestTheGuardNamesEveryProviderVerbTheKernelHas` | the guard names every provider verb the kernel has |
| `TestTheProviderFamilyClaimsNoOtherFamilysVerbs` | the provider's list claims no other family's verb and does not claim the whole kernel |

## The mutations

**13** mutations, **13** caught, in two independent passes with **byte-identical**
logs (SHA-256 `B6747B25B4118C91FC7D98DF5FD64A31F93F76417D0911659AC8BAB88030A89B`).

M11, M12 and M13 put the kernel back — the config reader reaches for the
kernel's provider again, the chat summarizer is handed the kernel's provider
again, and the plan authoring is handed the kernel's provider again — and the
**verb guard** catches every one of them. None of them is a behavioural
regression: the wiring would work **exactly as well**, it would simply be in
the wrong layer.

M06, M07 and M09 are the ones that bite: the port claims it holds **no**
chains, the port writes **no** chains, and the budget's ceiling is **not
forwarded**.

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

The handle stays, and the adapter still calls **34** kernel verbs on it and
hands over **15** method values: the base directory (**6**), the correlation
mint (**3**), the run decorators (**4**) and the rest.

Two doors are still open in the adapter's own layer: the **catalog** and the
**reload** — and the OAuth/catalog family (`kernel/app/providers`) still holds
the handle for both. That family will have to answer the handle rule before it
can answer the verb rule, exactly as the memory family's distillation service
had to.

The **base directory** is the other open question: six sites reach for it and
no family owns it yet.
