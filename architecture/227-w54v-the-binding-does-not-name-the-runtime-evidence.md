# W5.4v — the binding does not name the runtime

## What the slice is

The **ninth** slice of the port programme, and the first one that is about a
**word** rather than a **call**: the adapter's three newest bindings named the
kernel's type in order to hand the handle over.

W5.4s (the tools family), W5.4t (the objectives) and W5.4u (the workboard)
each ended with a binding file that looked like this:

```go
// kernel/controlplane/tools_ports.go
import (
	"context"

	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/runtime"
)

func toolsPortsFor(k *runtime.Kernel) apptools.Ports { return apptools.PortsFromKernel(k) }

func (s *Server) toolsPorts() apptools.Ports { return toolsPortsFor(s.k) }
```

The helper exists for one reason: to spell the kernel's type. It is the only
thing in the file that needs the runtime's import, and the only reason the
file has one.

Go does not need the spelling. A value of the kernel's type can be passed to
a function that expects it without the caller ever naming the package:

```go
// kernel/controlplane/tools_ports.go
import (
	"context"

	apptools "github.com/agezt/agezt/kernel/app/tools"
)

func (s *Server) toolsPorts() apptools.Ports { return apptools.PortsFromKernel(s.k) }

func requestToolsPorts(ctx context.Context) apptools.Ports {
	return apptools.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
```

That is the **roster** family's shape (`kernel/controlplane/roster_ports.go`)
and the **workflow** family's shape
(`kernel/controlplane/workflow_ports.go`): the adapter may hold the handle,
but it does not have to name the runtime to hand it over.

## What the slice found

The three files above were the **only** difference between the control plane's
runtime-naming count and the number every document has been stating since
W5.4n:

| | before | after |
|---|---|---|
| control-plane production files naming the runtime | **18** | **15** |

Evidence 224, 225 and 226 all say — correctly, as of the moment they were
written — that *the control plane still imports the runtime in **15** files*.
Three slices later the number was **18**, and **nothing failed**: the count
was never measured by anything. The archcheck ratchet holds the
`kernel/controlplane -> kernel/runtime` **edge** at package granularity, so a
new file inside the same package is invisible to it.

The claim was false for three merged slices. This slice makes it **true** and
makes it **checkable**.

## What changed

Three files, one shape each:

| file | before | after |
|---|---|---|
| `kernel/controlplane/tools_ports.go` | `toolsPortsFor(k *runtime.Kernel)` + a runtime import | `apptools.PortsFromKernel(s.k)` / `apptools.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)` |
| `kernel/controlplane/okr_ports.go` | `okrPortsFor(k *runtime.Kernel)` + a runtime import | `appokr.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)` |
| `kernel/controlplane/workboard_ports.go` | `workboardPortsFor(k *runtime.Kernel)` + a runtime import | `appworkboard.PortsFromKernel(s.k)` / `appworkboard.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)` |

No behaviour changed: the daemon's bindings still answer for the **root**
kernel and the request's bindings still answer for the **routed** kernel — a
tenant's tools, objectives and board are the tenant's own. The existing tests
(`TestTheAdapterBindsTheToolsPortsToItsOwnKernel`,
`TestTheRequestToolsPortsFollowTheRoutedKernel`,
`TestTheAdapterBindsTheObjectivePortsToItsOwnKernel`,
`TestTheAdapterBindsTheWorkboardPortsToItsOwnKernel`) hold that, and they
still pass.

## The two rules

### The binding rule

A binding file — a `kernel/controlplane/*_ports.go` file — does not import the
runtime at all. The rule is read from the tree: the binding files are the
files whose names end in `_ports.go`, so a family's binding is covered the
day it is written, and no list can rot.

### The ratchet

`controlPlaneRuntimeNamingFiles = 15` is the **recorded** measurement: how
many production files of the control plane name the kernel runtime. The
comparison runs in **both** directions, because a number that is a
measurement has to be updated in the slice that changes it:

- if the count **grows**, the handle walked back in — the slice that did it
  has to be undone;
- if the count **shrinks**, the constant and the documents that state it
  change **in the same slice**.

This is the first machine-checked statement of that number. Before this
slice, the number was prose: it was true in eight documents and false in the
tree, and nothing noticed.

## The tests

| test | holds |
|---|---|
| `TestTheAdapterBindingsDoNotNameTheRuntime` | no binding file names the runtime |
| `TestTheBindingRuleSeesTheViolationItExistsFor` | the rule can see the violation it exists for |
| `TestTheControlPlaneNamesTheRuntimeInFifteenFiles` | the recorded number is the measured number |
| `TestTheRuntimeNamingCheckFailsInBothDirections` | the comparison fails in both directions |
| `TestTheRuntimeNamingMeasurementCountsWhatItClaims` | the measurement counts production files |

## The mutations

**13** mutations, **13** caught, in two independent passes with **byte-identical**
logs (SHA-256 `59FBC2685262ED55C6E67904F69CD7BBA581535FF17E4D80CA94042E3B37EB90`).

The three that matter most are M01, M02 and M03: they put the runtime import
back into each of the three bindings — the exact regression the slice
repairs — and the two rules catch every one of them. None of them is a
behavioural regression: the wiring would work **exactly as well**, it would
simply be in the wrong layer, and no behaviour test can see that.

M07 is the one worth reading twice: the binding rule's file filter widens
from `_ports.go` to `_go`, the rule starts flagging the whole adapter, and
the rule's own test catches it — because the rule is not asked whether it is
*too broad*, only whether it is *sharp*.

M08 and M09 hold the ratchet. M08 raises the recorded number to eighteen and
the comparison's own test says the check has stopped rejecting the number it
records. M09 turns the comparison into a one-sided one — a check that only
fails when the debt **grows** — and the same test catches it, because a debt
that grows is exactly what a one-sided check cannot see.

M10 and M11 hold the **measurement**: M10 lets the test files into the count
(the count jumps to 166), M11 narrows the count to the binding files (the
count drops to zero). Both are the ways a count like this silently stops
counting what it says it counts.

M12 and M13 are the behavioural pair: the daemon's tools binding and the
request's tools binding answer for nobody, and the bindings' own tests catch
it — so the slice's shape change is held at the behaviour as well as at the
boundary.

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

The **ratchet does not move yet**: the control plane still names the runtime
in **15** files — the files that hold the handle on purpose.

Those fifteen are the ones the programme's last step is about: the families
that own a **store** (the seat catalog, the journal, the sandbox, the
providers) are the ones the roadmap calls *libraries*, and the store is the
host's file. The binding rule does not touch them: a file that holds the
handle may name it. What the rule says is narrower and sharper — a file that
merely **hands the handle over** must not name it.

That is the difference between this slice and the last one. W5.4u finished
the family map's *verbs without libraries*. This slice finishes the **binding
shape**: from now on, a family that declares its ports binds them by handing
the handle over, and the adapter never spells the runtime to do it.
