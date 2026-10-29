# W5.4al — the taste family, and the library the daemon may not have

## What the slice is

The **twenty-fifth** slice of the port programme, and the **sixteenth** one of
its last phase: the handle itself.

The taste is the daemon's own library of curated exemplars — the examples a run
may imitate. The adapter reached for the kernel's library in **one** place.

## What the slice found

| file | sites | whose library |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 1 | the routed one |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **13** | **12** |
| distinct kernel verbs the adapter names | **11** | **10** |
| method values handed over | **8** | **8** |

The **application** layer held no taste handle either: `kernel/app/taste`
already took the contract's `tasteapi.Store` as an argument.

## What changed

The taste family's ports moved into the application
(`kernel/app/taste/ports.go`):

```go
type Ports struct {
	store func() tasteapi.Store
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{store: func() tasteapi.Store { return k.Taste() }}
}

func (p Ports) Store() tasteapi.Store {
	if p.store == nil {
		return nil
	}
	return p.store()
}
```

and the adapter keeps the binding (`kernel/controlplane/taste_ports.go`) — which
does not name the runtime, like every binding since W5.4v:

```go
func requestTastePorts(ctx context.Context) apptaste.Ports {
	return apptaste.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
```

There is **no daemon-level binding**, and the reason is the shape of the read
path: every surface that reads the taste reads it on the **request's** path, so
the routed kernel is the one that answers. A binding nobody calls is dead code.

### The port's names are not the kernel's

The port answers `Store` — never `Taste`. A port that carried the name of the
verb it stands for would let the adapter name the kernel's verb through the
port's door.
`TestTheTastePortNamesAreNotTheKernelVerbs` holds that by reflection over the
port's own method set and the kernel's.

### The library is the daemon's file

The curated exemplar library is **not** opened by the kernel: it is the
composition root's file, and the kernel holds the contract's store port. A
daemon that was given none holds a **nil** port and injects no exemplars into a
run rather than calling into nothing.

So the port's empty case is a **real** case here — not a theoretical one — and
the port answers it: it hands over nothing, and the surfaces that read it report
the empty library, which is the honest answer.

## The rule

The taste's verbs are the application's, and the rule is **by whole word**:

> a kernel verb whose name carries "Taste" as a **complete word** is the taste
> family's business.

The sweep takes in **one** verb today: `Taste`.

The guard now holds **140** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheTastePortsReachTheKernelStore` | the port hands over the daemon's own library |
| `TestTheTastePortsSurviveAHostWithNoLibrary` | a daemon that was given no library hands over none |
| `TestTheTastePortsSurviveAHostWithNoKernel` | a host with no kernel hands over no store |
| `TestTheTastePortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheAdapterBindsTheTastePortsToTheRoutedKernel` | the request's ports follow the **routed** kernel, and the daemon's own request answers for the daemon's own |
| `TestTheAdapterBindsTheTastePortsToAHostWithNoLibrary` | a daemon given no library hands over none |
| `TestTheGuardNamesEveryTasteVerbTheKernelHas` | the guard names every taste verb the kernel has |
| `TestTheTasteFamilyClaimsNoOtherFamilysVerbs` | the taste's list claims no other family's verb and does not claim the whole kernel |

**8** permanent tests.

## The mutations

**11** mutations, **11** caught, in two independent passes with **byte-identical**
logs (SHA-256 `BEBEF94C74FE41D0F1E1D8FF1DFA7981043D95F84DA7829042B3E744ED27D19A`).

M01, M02 and M03 are the ones that bite: the port holds **no** store, a host
with no kernel asks the kernel **anyway**, and the port hands over **nothing**.

M04 gives the port a method that carries a kernel verb's own name, M05 empties
the request's binding, M06 makes it follow the **daemon's** kernel — a tenant's
library would become the daemon's — M07 puts the kernel back and the **verb
guard** catches it, M08 takes the family out of the guard, M09 hands the family
**another** family's verbs, M10 drops the family's word from the rule, and M11
makes the binding name the runtime.

### Three of the eleven were caught only after the slice learned something

M01, M03 and M05 first ran against tests that opened a daemon **without** a
taste library — and a daemon without one holds a **nil** store, so the port's
empty answer and the kernel's empty answer were the same answer. The three
mutations **survived**: the tests could not see them.

The slice then wrote the daemon the **library** as well — `withTaste` opens the
store the daemon opens and hands it to the kernel — and the three mutations are
caught. The lesson is the one the world slice taught a day earlier, and it is
worth repeating because it is easy to forget: **a port's test must see both of
the port's states**. The empty state is where the refusal lives, and the full
state is where the wiring lives; a test that sees only one of them cannot hold
the other.


## What the slice does not do

The handle stays, and the adapter still calls **12** kernel verbs on it and hands
over **8** method values: the run decorators, the state, the data lake, the
seats, the reflect, the reaper, the update drain and the rest.

The **runs family** still holds the kernel handle — for the run decorators, the
roster, the warden, the model and the catalog. It is the next family, and it is
the largest one left.

The daemon's **overview** (`kernel/app/system`) still holds the handle for
**thirteen** verbs across **eight** families. That surface is not one family but a
**roll-up** of many, and it will have to ask each of them rather than the kernel.
