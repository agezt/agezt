# W5.4ad — the daemon's data root, and the family that owns nothing but a place

## What the slice is

The **seventeenth** slice of the port programme, and the **eighth** one of its
last phase: the handle itself.

The data root is where the daemon keeps everything — the journal's segments,
the sandbox's projects, the storage's artifacts, the policy overlay's
snapshot, the roster's workspaces. The adapter reached for the kernel's base
directory in **six** places across **five** files, and every one of them
belonged to a **different** family.

## What the slice found

| file | site | whose root |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 3 | two routed, one the daemon's own |
| `kernel/controlplane/config_handler.go` | 1 | routed |
| `kernel/controlplane/journal_stats.go` | 1 | routed |
| `kernel/controlplane/roster_workspace.go` | 1 | the daemon's own |

| | before | after |
|---|---|---|
| call sites on the handle | **34** | **28** |
| distinct kernel verbs the adapter names | **22** | **21** |
| method values handed over | **15** | **15** |
| control-plane files naming the runtime | **15** | **14** |

## What changed

The data root's ports moved into the application
(`kernel/app/basedir/ports.go`):

```go
type Ports struct {
	dir string
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{dir: k.BaseDir()}
}
```

and the adapter keeps the binding (`kernel/controlplane/basedir_ports.go`) —
which does not name the runtime, like every binding since W5.4v:

```go
func (s *Server) baseDirPorts() appbasedir.Ports { return appbasedir.PortsFromKernel(s.k) }

func requestBaseDirPorts(ctx context.Context) appbasedir.Ports {
	return appbasedir.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
```

### The family that owns nothing but a place

The data root is not the journal's business, nor the sandbox's, nor the
storage's — it is the place **all** of them keep their files, and it is a
family of its own for exactly that reason. The port is one string wide, and
that is the whole truth about it: the adapter asks the data root where the
daemon keeps its files, and the families that keep files are handed the answer
as a plain path.

### The routed root is not the daemon's root

This is the one binding where "the daemon's base dir" would be **wrong**. The
tenant registry opens **one base directory per tenant** — the tenant registry's
own test says so: *tenants must have distinct base dirs*. Four of the six
sites are routed, so four of them would have put a tenant's journal and a
tenant's artifacts under the **daemon's** directory if the server's own field
had been used.

The daemon and the routed kernel are therefore bound apart, like every other
family since W5.4v.

### The journal reads stopped naming the runtime

The journal statistics file asked the kernel for its base directory and
nothing else, and that one call was the file's **only** reason to name the
runtime at all. With the root coming from the application's own port, the file
does not name the runtime any more — and the **ratchet** moved:

| | before | after |
|---|---|---|
| control-plane production files naming the runtime | **15** | **14** |

The ratchet is a machine-checked number, and it is compared in **both**
directions: a number that is a measurement has to be updated in the slice that
changes it. The constant moved with the tree, and so did the documents that
state it.

### The test's name carried the number

The ratchet test was named `TestTheControlPlaneNamesTheRuntimeInFifteenFiles`.
Its name carried the count, so every move of the count wanted a rename — and a
rename is a chance to forget the documents that name the test. The test is now
`TestTheControlPlaneNamesTheRuntimeInTheRecordedNumberOfFiles`: the name says
the **rule**, and the rule does not move.

## The rule

The data root's verbs are the application's, and the rule is **by word** — but
the family name is **two** words:

> a kernel verb whose name carries **both** the `Base` and the `Dir` word is
> the data root's business.

Camel-case splits `BaseDir` into `Base` and `Dir`, so a rule that looked for
the whole name would never match — the whole-word rule found nothing, and the
guard's own test said so: *no BaseDir verbs found*. Neither half is the
family on its own: `Base` alone is too loose, and the compound `BaseDir` as a
**substring** over-captures — `RebaseDir` begins with `Base` and is not the
data root's business. `RebaseDir` splits into `Rebase` and `Dir` and carries
no `Base` word at all, so the two-word rule is sharp where the substring rule
would not have been.

The guard now holds **123** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheBaseDirPortsReachTheKernelRoot` | the port reports the root the daemon keeps its files under |
| `TestTheBaseDirPortsSurviveAHostWithNoKernel` | a host with no kernel reports no root and invents nothing |
| `TestTheAdapterBindsTheBaseDirPortsToItsOwnKernel` | the daemon's root answers for the daemon's kernel, the request's for the routed one, and they are **not** the same directory |
| `TestTheGuardNamesEveryBaseDirVerbTheKernelHas` | the guard names every data-root verb the kernel has |
| `TestTheBaseDirFamilyClaimsNoOtherFamilysVerbs` | the data root's list claims no other family's verb and does not claim the whole kernel |

## The mutations

**13** mutations, **13** caught, in two independent passes with **byte-identical**
logs (SHA-256 `861050558E718450FDF51254F6645D7139144B286DBE36F7A99FFBABDCC2E794`).

M07 through M12 put the kernel back — the sandbox, the storage, the policy
overlay, the config reader, the journal reads and the workspace root each reach
for the kernel's base directory again — and the **verb guard** catches every
one of them. None of them is a behavioural regression: the wiring would work
**exactly as well**, it would simply be in the wrong layer.

M01, M02, M03 and M06 are the ones that bite: the port binds **no** root, the
daemon's binding answers for **nobody**, the request's binding answers for
**nobody**, and the port reports **another** root.

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

The handle stays, and the adapter still calls **28** kernel verbs on it and
hands over **15** method values: the correlation mint (**3**), the run
decorators (**4**), the approvals, the market, the plugins and the rest.

Two doors are still open in the adapter's own layer: the **catalog** and the
**reload** — and the OAuth/catalog family (`kernel/app/providers`) still holds
the handle for both, along with the catalog store. That family will have to
answer the handle rule before it can answer the verb rule, exactly as the
memory family's distillation service had to.
