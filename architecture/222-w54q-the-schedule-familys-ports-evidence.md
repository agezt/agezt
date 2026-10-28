# W5.4q — the schedule family's ports, and the fifth nil-check that went with them

## What the slice is

The **fourth** family of the port programme, and the second one that is a
*library* rather than a set of plain verbs.

W5.4p moved the **standing** family's port-filling into the application and
brought the **nil-port discipline** with it: the daemon opens the library, hands
the kernel a contract port, and a daemon with no library holds a **nil** port
that the application answers with "nothing here" instead of a call into
nothing.

The **schedule** library (W3.49) is the same shape, and the adapter was
carrying the same discipline — **five** times:

```go
// roster_cascade.go, roster_status_views.go, roster_teardown.go
if s.k.Schedules() == nil {
    return nil
}
return s.k.Schedules().List()
```

Five copies of one rule, in three files, plus three more places that handed the
raw store to an application constructor. So the schedule family's ports moved
into the application as well:

```go
// kernel/app/schedule/ports.go
type Ports struct {
	Store      cadenceapi.HostStore                  // the contract's host store port; NIL when there is no library
	Entries    func() []cadenceapi.Entry             // nil-safe listing
	Remove     func(id string) (bool, error)         // nil-safe
	SetEnabled func(id string, enabled bool) (bool, error) // nil-safe
}

func PortsFromKernel(k *runtime.Kernel) Ports
```

The application now spells the nil-check **once**, and the surfaces that read
the schedules ask the application instead of the handle.

## What moved

| | Before | After |
|---|---|---|
| the schedule reads | `appschedule.New(s.k.Schedules(), …)` | `appschedule.New(ports.Store, …)` |
| the schedule editing | `appschedule.NewEditing(s.k.Schedules(), …)` | `appschedule.NewEditing(ports.Store, …)` |
| the schedule lifecycle | `appschedule.NewLifecycle(s.k.Schedules(), …)` | `appschedule.NewLifecycle(ports.Store, …)` |
| the schedule creation | `appschedule.NewCreation(host.kernel.Schedules(), nil)` — the **per-request** kernel | `appschedule.NewCreation(ports.Store, nil)` |
| the impact derivation's schedules | a nil-check and a `List()` spelled in the adapter | `schedule.Entries` |
| the status view's schedules | a nil-check and a `List()` spelled in the adapter | `schedule.Entries` |
| the teardown's schedules | a nil-check and a `List()` spelled in the adapter | `schedule.Entries` |
| the teardown's removal | a nil-check and a `Remove()` spelled in the adapter | `schedule.Remove` |
| the teardown's pause | a nil-check and a `SetEnabled()` spelled in the adapter | `schedule.SetEnabled` |

**Five** copies of the nil-check became one, and the schedule family's kernel
verb sites left the adapter's production code.

The schedule's **per-request** kernel is bound too: the creation service is
built from the routed (tenant) kernel, so the port is filled from the kernel the
request actually arrived on.

## The guard's third rule

The guard now holds **26** application-owned verbs: the council's four
(hand-written), the roster's nine (the family's accessor file), the standing
family's eight (the name) and the schedule family's **five** (the name).

The schedule rule is the standing rule generalised: the helper now takes the
family's name, so a family whose accessor file is shared with other families'
verbs is found by name instead.

The rule sweeps in `Scheduler`, `ScheduleEngine` and `SetScheduleEngine` — the
scheduler is the module's implementation, and the adapter must not name it
either. It also sweeps in `WakeContextScheduleID`, the wake-provenance context
reader, for the same reason the standing rule sweeps in its two.

## The proof

### Permanent tests — 4 new

| Test | What it holds |
|---|---|
| `TestTheSchedulePortsReachTheKernelSchedules` (`kernel/app/schedule`) | the schedules the kernel holds are the schedules the application reads, pauses and removes, through the ports |
| `TestTheSchedulePortsSurviveADaemonWithNoScheduleLibrary` (`kernel/app/schedule`) | the **nil-port discipline**: with no library the store port is nil, the listing is empty, and the removal and the pause say "nothing" rather than calling into nothing |
| `TestTheAdapterBindsTheSchedulePortsToItsOwnKernel` (`kernel/controlplane`) | the binding itself: the daemon's schedule service serves the kernel's schedules, and the roster teardown's schedule port — bound from the application's port, not from the handle — sees the same library |
| `TestTheGuardNamesEveryScheduleVerbTheKernelHas` (`kernel/controlplane`) | the derivation — read from the kernel's own method set, independently of the helper the guard uses |

### The mutation run — 12 mutations, 12 caught

Two independent passes with **byte-identical** logs (SHA-256
`B7E37BBAB7198D0671EF2A02F5E8C3AFB00F84110CD970E57ABC60187E5A9396`; the
elapsed times, ULIDs, epoch stamps, addresses and goroutine ids are
normalized, because they are not the mutation's fingerprint).

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01 | the store port is nil — the service answers "not available" | the ports test |
| M02 | the listing port answers nothing | the ports test |
| M03 | the removal port removes nothing | the ports test |
| M04 | the pause port does not pause | the ports test |
| M05 | the pause port **inverts** the flag | the ports test |
| M06 | the removal port **claims success** without removing | the ports test |
| M07 | the adapter's reads get no store | the binding test |
| M08 | the teardown's removal port removes nothing | the binding test |
| M09 | the teardown's pause port does not pause | the binding test |
| M10 | the creation service gets no store — the daemon cannot add a schedule | the schedule add/edit test |
| M11 | the schedule rule loses its case — the family disappears from the guard | the guard test |
| M12 | the shared rule narrows to the setters — most verbs fall out of the guard | the guard test |

M06 is the one worth reading: a removal port that says "removed" and removes
nothing is a **lie**, and only an assertion that looks at the library
afterwards catches it. A test that asserted the port's return value would have
passed.

M11 and M12 also show what the shared helper bought: a mutation in it takes
**both** families' guards with it, and the two family tests each catch it from
their own side — the expectation is derived independently in each.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | OK — **2 allowlisted import violations and 0 allowlisted call sites remain** |
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

The **ratchet does not move yet**: the control plane still imports the runtime
in **15** files.

The next family is the **workflow** library, and it is the first one that
hands the application the **handle itself** (`appworkflow.NewCopilot(s.k.Workflows(),
s.k, s.k.NewCorrelation)`). That is the shape the whole programme is heading
for: the port the application holds stops being "the kernel's verbs" and
becomes "the kernel, narrowed to what this application needs". The four
families so far — council, roster, standing, schedule — are the steps towards
it.
