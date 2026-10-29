# W5.4ai — the marketplace family, and the port that asks when it is asked

## What the slice is

The **twenty-second** slice of the port programme, and the **thirteenth** one of its
last phase: the handle itself.

The marketplace is the daemon's own library of packs — the built-in Official
catalogue and the sources the operator adds — and the adapter reached for the
kernel's manager in **two** places in **one** file.

## What the slice found

| file | sites | whose marketplace |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 2 | the routed one |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **18** | **16** |
| distinct kernel verbs the adapter names | **14** | **13** |
| method values handed over | **9** | **9** |

The **application** layer held no marketplace handle either: `kernel/app/market`
already took a `Reader` and a `Writer` of its own. What the adapter did was reach
for the kernel's manager in both directions — once for the reads, once for the
writes — and spell the **nil case twice**, in the same file:

```go
manager := ctx.Value(appHostKey{}).(appHost).kernel.Market()
if manager == nil {
	return appmarket.NewReads(nil)
}
return appmarket.NewReads(manager)
```

Two spellings of the same question, in two shapes, for one family.

## What changed

The marketplace family's ports moved into the application
(`kernel/app/market/ports.go`):

```go
type Manager interface {
	Reader
	Writer
}

type Ports struct {
	manager func() Manager
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{manager: func() Manager { return k.Market() }}
}

func (p Ports) Manager() Manager {
	if p.manager == nil {
		return nil
	}
	return p.manager()
}
```

and the adapter keeps the binding (`kernel/controlplane/market_ports.go`) —
which does not name the runtime, like every binding since W5.4v:

```go
func requestMarketPorts(ctx context.Context) appmarket.Ports {
	return appmarket.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
```

There is **no daemon-level binding**, and the reason is the shape of the read
path: both surfaces — the reads and the writes — run on the **request's** path,
so the routed kernel is the one that answers. A binding nobody calls is dead
code.

### The port's names are not the kernel's

The port answers `Manager` — never `Market`. A port that carried the name of the
verb it stands for would let the adapter name the kernel's verb through the
port's door.
`TestTheMarketPortNamesAreNotTheKernelVerbs` holds that by reflection over the
port's own method set and the kernel's.

### The port asks WHEN IT IS ASKED

The port holds a **function**, not a value: `func() Manager { return k.Market() }`.
So the port asks the kernel at the moment it is asked, and a daemon that is given
its marketplace **after** the ports are filled still answers.

This is the same shape as the artifacts family's port (`func() BlobStore { return
k.Artifacts() }`) and the catalog family's (`k.Catalog` is a method value, and a
method value calls later). The family that answered at **fill** time would report
no marketplace for a daemon that has one, and the difference between the two is
invisible until the day the daemon is given its marketplace late.

### The nil case left the adapter

The adapter's two **nil checks** — one for the reads, one for the writes — left
the adapter with the port. The port hands over nothing when the daemon holds
nothing, and the surfaces that read it refuse instead of reading from nothing.

## The rule

The marketplace's verbs are the application's, and the rule is **by whole word**:

> a kernel verb whose name carries "Market" as a **complete word** is the
> marketplace family's business.

The rule catches **two** verbs, and the second one is the reason the rule is
worth stating: the camel-case splitter reads `SetMarket` as **"Set" + "Market"**,
so a family that looked for the exact name `Market` would guard the accessor and
leave the setter unguarded.

The rule is written **out**, not taken from the helper:

```go
for _, word := range camelWords(name) {
	if word == "Market" {
		verbs = append(verbs, name)
		break
	}
}
```

The sweep takes in **two** verbs today: `Market` and `SetMarket`.

The guard now holds **137** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheMarketPortsReachTheKernelManager` | the port hands over the daemon's own manager |
| `TestTheMarketPortsSurviveAHostWithNoKernel` | a host with no kernel hands over no manager |
| `TestTheMarketPortsSurviveAHostWithNoManager` | a daemon that holds no marketplace hands over no manager |
| `TestTheMarketPortsAskTheKernelWhenTheyAreAsked` | the port asks the kernel when it is asked, not when it was filled |
| `TestTheMarketPortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheAdapterBindsTheMarketPortsToTheRoutedKernel` | the request's ports follow the **routed** kernel, and the daemon's own request answers for the daemon's own kernel |
| `TestTheAdapterBindsTheMarketPortsToAHostWithNoManager` | a daemon given no marketplace hands over none, and the reads refuse |
| `TestTheMarketServiceReadsThroughThePort` | the application reads through the port |
| `TestTheGuardNamesEveryMarketVerbTheKernelHas` | the guard names every marketplace verb the kernel has |
| `TestTheMarketFamilyClaimsNoOtherFamilysVerbs` | the marketplace's list claims no other family's verb and does not claim the whole kernel |

**10** permanent tests.

## The mutations

**13** mutations, **13** caught, in two independent passes with **byte-identical**
logs (SHA-256 `95DF9C3CC9586475C99FBDF12E53A50C9992E02307509FDF7D6FD3205D80BF11`).

M01, M02, M03 and M13 are the ones that bite: the port holds **no** manager, a
host with no kernel asks the kernel **anyway**, the port hands over **nothing**,
and the port answers from what it was given **at fill time** — a daemon that is
given its marketplace late would report none.

M04 empties the binding, M05 makes it follow the **daemon's** kernel — a
tenant's marketplace would become the daemon's — M06 builds the reads from
**nothing**, M07 builds the writes from **nothing**, M08 gives the port a method
that carries a kernel verb's own name, M09 takes the family out of the guard,
M10 drops the setter half of the rule, M11 hands the family **another** family's
verbs, and M12 makes the binding name the runtime.

### One of the thirteen was caught only after the slice learned something

M13 first ran against the **service test** — and it **survived**: the service
test asks the port itself, so a mutation in the adapter's operation builder was
invisible to it. The test was the wrong test.

The mutation was then pointed at the thing it actually breaks — the port's
**timing** — and the slice wrote the test that holds it:
`TestTheMarketPortsAskTheKernelWhenTheyAreAsked`. The lesson is the one the
programme keeps teaching: a surviving mutation is a **missing test**, and the
missing test here was not about the port's answer but about the port's
**question**.

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

The handle stays, and the adapter still calls **16** kernel verbs on it and hands
over **9** method values: the run decorators, the approvals, the state, the
world, the taste, the data lake, the seats, the reflect, the reaper, the update
drain and the rest.

The **config reader** still holds the kernel handle — for the daemon's default
model and its default identity prompt. Those are the daemon's own identity, and
they are the next family's business, not the marketplace family's.

The daemon's **overview** (`kernel/app/system`) still holds the handle for
**thirteen** verbs across **eight** families. That surface is not one family but a
**roll-up** of many, and it will have to ask each of them rather than the kernel.
