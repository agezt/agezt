# W5.4x — the journal family's ports, and the seventeen families that read it

## What the slice is

The **eleventh** slice of the port programme, and the **second** one of its
last phase: the handle itself.

The journal is the one capability **every** family reads. The adapter reached
for the kernel's journal in **twenty-eight** places across **eight** files —
more sites than any other verb in the control plane, and more families than
any other family.

## What the slice found

| file | sites | whose journal |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 20 | twelve routed, eight the daemon's own |
| `kernel/controlplane/roster_status_views.go` | 2 | the daemon's own |
| `kernel/controlplane/schedule_helpers.go` | 2 | the caller's kernel |
| `kernel/controlplane/runs.go` | 1 | the caller's kernel |
| `kernel/controlplane/tenant.go` | 1 | the tenant's own kernel |
| `kernel/controlplane/journal_stats.go` | 1 | the caller's kernel |
| `kernel/controlplane/pulse.go` | 1 | the daemon's own |
| `kernel/controlplane/channels_inbox_ports.go` | 1 | the daemon's own |

Twelve of the twenty-eight were **routed** — a tenant's journal is the
tenant's journal — and the rest read the daemon's own. The slice keeps that
split exactly: `requestJournalPorts(ctx)` answers for the routed kernel,
`s.journalPorts()` for the root.

The adapter's handle-verb debt, measured across the whole control plane:

| | before | after |
|---|---|---|
| call sites on the handle | **104** | **76** |
| distinct kernel verbs the adapter names | **30** | **29** |

## What changed

The journal family's ports moved into the application
(`kernel/app/journal/ports.go`):

```go
type Ports struct {
	// Events is the journal — the event log every family reads.
	Events *journalstore.Journal
}

func PortsFromKernel(k *runtime.Kernel) Ports { return Ports{Events: k.Journal()} }
```

and the adapter keeps the binding (`kernel/controlplane/journal_ports.go`) —
which does not name the runtime, like every binding since W5.4v:

```go
func (s *Server) journalPorts() appjournal.Ports { return appjournal.PortsFromKernel(s.k) }

func requestJournalPorts(ctx context.Context) appjournal.Ports {
	return appjournal.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
```

The port carries the **journal itself**, and that is the point of this family:
the families already narrow it. Every consumer declares its **own** interface
over the part of the journal it uses — the roster's range, the channel inbox's
append, the schedule's firing, the pulse stream's tail — and those narrow
views are the consumers' business, not the family's. A port shaped like one
consumer's view would be a second place to change when a consumer grows.

The field is named `Events`, not `Journal`: **the fourth time** this
programme learned that a port field must not answer to a kernel verb's name
(`MCPAttached` → `Attachment`, `Rollup` → `Progress`, the workboard's
`Journal` → `Events`). The collision this time was the **workflow**
application's own port field, which was named `Journal` and which the adapter
read as `ports.Journal` — so the field was renamed to `Events` and the
adapter reads `ports.Events`.

### The trace service was handed the whole handle

The journal family's trace service took the **handle**:

```go
return appjournal.NewTrace(ctx.Value(appHostKey{}).(appHost).kernel)
```

The **handle rule** caught it the moment the journal family declared its own
ports: a family that declares its ports takes ports, not the handle they were
filled from. And the fix was free, because the kernel's three provenance verbs
are **pure delegations to the very same journal**:

```go
func (k *Kernel) Why(eventID string) ([]*event.Event, error) { return k.runexec.Why(eventID) }
// and runexec's:
// return r.k.Journal().Why(eventID)
```

So the trace service reads the routed kernel's journal, which is what the
handle answered with — the same answer, one layer down.

## The rule

The journal's verbs are the application's, and the rule is **by name** — the
kernel's verb says what it is. The guard now holds **94** application-owned
verbs.

## The tests

| test | holds |
|---|---|
| `TestTheJournalPortsReachTheKernelJournal` | the ports reach the kernel's journal |
| `TestTheAdapterBindsTheJournalPortsToItsOwnKernel` | the daemon's journal answers for the daemon's kernel and the request's for the routed one |
| `TestTheGuardNamesEveryJournalVerbTheKernelHas` | the guard names every journal verb the kernel has |

## The mutations

**13** mutations, **13** caught, in two independent passes with **byte-identical**
logs (SHA-256 `F58AEDF458319C2068593B0A82441C0ECD0B950F3D7897CB61BE633B59D420D3`).

M11, M12 and M13 are the three that matter most: they put the kernel back —
the routed consumer reaches for the kernel's journal again, the daemon's
consumer does the same, and the binding file names the runtime again — and the
**three rules** catch every one of them. None of them is a behavioural
regression: the wiring would work **exactly as well**, it would simply be in
the wrong layer, and no behaviour test can see that.

M08 and M09 are the ones that bite: the runs fold and the tenant's activity
read the **daemon's** journal instead of the kernel they were handed — the
routed kernel's rule, broken in the one place where a tenant's runs silently
become somebody else's.

The calibration caught two of the mutations' own faults, and both are worth
keeping: M09 left the tenant's kernel "declared and not used" (the mutation
did not compile — the mutation's fault), and M10 pointed at a test that never
reads the journal (the registry test, while the **stats** assertion lives in
the routing test). A mutation pointed at the wrong test is not a mutation that
survived; it is a mutation that was never aimed.

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

The handle stays, and the adapter still calls **76** kernel verbs on it: the
event bus (**19**), the skill forge (**7**), the providers (**7**), the base
directory (**6**), the memory (**5**), the policy engine (**4**), the
correlation mint (**3**) and the rest.

The journal was the largest of them and the one with the most consumers, and
it came over in the shape the rest will take: the application fills its own
ports, the adapter asks the family, and the **seventeen families that read it
stop reaching for the kernel**.
