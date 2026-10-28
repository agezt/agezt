# W5.4p — the standing family's ports, and the nil-port discipline that moved with them

## What the slice is

The **third** family of the port programme, and the first one where the family
is not a set of *verbs* but a set of *stores*.

The roster's verbs were simple: the adapter asked the kernel a question about
an agent. The standing family's verbs are the same shape, but they sit on top
of a **library** — the daemon opens `standing/standing.json` and hands the
kernel a contract port (W3.50). A daemon with no standing library holds a
**nil** port, and the kernel's answers are then refusals, not panics.

The adapter carried that discipline itself, spelled out **twice**:

```go
// roster_teardown.go
if s.k.Standing() == nil {
    return nil
}
return s.k.Standing().List()
```

and once more in `roster_status_views.go`. Two surfaces, two copies of the same
nil-check, and a third surface that forgot it would have called into nothing.

So the standing family's ports moved into the application:

```go
// kernel/app/standing/ports.go
type Ports struct {
	Reader     Reader                                     // the contract's store port; NIL when there is no library
	Writer     Writer                                     // the kernel's four standing verbs
	Orders     func() []standingapi.Order                 // nil-safe listing
	Remove     func(id string) (bool, error)
	SetEnabled func(id string, enabled bool) error
}

func PortsFromKernel(k *runtime.Kernel) Ports
```

The application now spells the nil-check **once**, and the surfaces that read
the standing orders ask the application instead of the handle.

## What moved

| | Before | After |
|---|---|---|
| the standing service | `appstanding.New(s.k.Standing(), s.k, …)` — the adapter handed the **raw handle** to the application | `appstanding.New(ports.Reader, ports.Writer, …)` |
| the teardown's orders | a nil-check and a `List()` spelled in the adapter | `standing.Orders` |
| the teardown's removal | `s.k.RemoveStanding(id)` | `standing.Remove` |
| the teardown's pause | `s.k.SetStandingEnabled(id, enabled)` | `standing.SetEnabled` |
| the status view's orders | a nil-check and a `List()` spelled in the adapter | `standing.Orders` |

**Four** kernel verb sites left the adapter's production code, and **two**
copies of the nil-check became one.

One subtlety worth naming: the adapter hands the application a **narrow** view
of the handle — the application's `Writer` interface has four methods — and it
does so from the *application* side. The adapter no longer passes `s.k` into an
application constructor at all.

## The guard, and the rule it could not use

The guard now holds **21** application-owned verbs: the council's four
(hand-written), the roster's nine (derived from the kernel's roster accessor
file) and the standing family's **eight**.

The standing family needed its **own** rule, and the reason is worth recording:
the roster's rule ("the family's accessor file") does not transfer. The
standing accessor file also holds the data accessors — the artifact store, the
data lake, the voice adapter — so a file rule would sweep in verbs that are not
the standing family's.

The standing rule is therefore **the name**: every kernel method whose name
mentions the standing library.

- `Standing`, `StandingList`, `AddStanding`, `SetStandingEnabled`,
  `UpdateStanding`, `RemoveStanding` — the library's six.
- `WakeContextStandingID`, `WakeContextStandingName` — the wake-provenance
  context readers. The rule sweeps them in, and that is **right**: the adapter
  reads the wake provenance from the **contract** (`agentapi.WakeContext`), not
  through the kernel, and the standing order it names is the standing family's
  business.

A family whose rule is "the name" has no accessor file to point at, so the
test that holds the rule reads the kernel's **own method set** — and it does so
*independently* of the helper the guard uses, for the reason W5.4o's
calibration forced: the expectation must not be derived by the code path it
checks.

## The proof

### Permanent tests — 3 new

| Test | What it holds |
|---|---|
| `TestTheStandingPortsReachTheKernelOrders` (`kernel/app/standing`) | the orders the kernel holds are the orders the application adds, reads, pauses and removes — through the ports |
| `TestTheStandingPortsSurviveADaemonWithNoStandingLibrary` (`kernel/app/standing`) | the **nil-port discipline**: with no library the reader port is nil and the listing is empty, not a call into nothing |
| `TestTheAdapterBindsTheStandingPortsToItsOwnKernel` (`kernel/controlplane`) | the binding itself: the daemon's standing service serves the kernel's orders, and the roster teardown's standing port — bound from the application's port, not from the handle — sees the same library |

The guard's standing test (`TestTheGuardNamesEveryStandingVerbTheKernelHas`)
holds the derivation.

### The mutation run — 12 mutations, 12 caught

Two independent passes with **byte-identical** logs (SHA-256
`306EA35FB16D8F8235AC6002C3F4F75DAFFE51F737B854AD9880AB6514BB9E23`; the
elapsed times, ULIDs, epoch stamps, addresses and goroutine ids are
normalized, because they are not the mutation's fingerprint).

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01 | the reader port is nil — the service answers "not available" | the ports test |
| M02 | the writer port is nil — the service cannot add an order | the ports test |
| M03 | the listing port answers nothing | the ports test |
| M04 | the removal port removes nothing | the ports test |
| M05 | the pause port does not pause | the ports test |
| M06 | the pause port **inverts** the flag | the ports test |
| M07 | the adapter's service gets no reader — the daemon cannot list its standing orders | the binding test |
| M08 | the teardown's removal port removes nothing | the binding test |
| M09 | the teardown's pause port does not pause | the binding test |
| M10 | the standing rule loses its case — the family disappears from the guard | the guard test |
| M11 | the standing rule narrows to the setters — four verbs fall out of the guard | the guard test |
| M12 | the matcher sees calls only — a verb could leave the adapter as a **method value** | the matcher's own test |

M06 is the one worth reading twice: M05 and M06 break the port in *opposite*
directions (do nothing / do the opposite), and only the flag's direction in the
assertion separates them. A test that asserted "the order changed" would have
caught neither.

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
in **15** files, and it still holds the kernel handle for families this slice
did not touch.

The next family is the **schedule** library (W3.49), which is the same shape
as the standing one — a daemon-opened store, a nil port when the daemon has
none, and a write side that is a set of kernel verbs. The pattern is now
proven for both halves of that shape: a library the daemon owns (standing) and
a set of plain verbs (roster).

Two things this slice settled that the next one will reuse:

- **The nil-port discipline belongs to the application.** It was spelled twice
  in the adapter; now it is spelled once, where the port is filled.
- **The guard's rule is per family, and each rule is held by a test that
  derives the family independently.** The roster's rule is a file; the
  standing family's is a name. Neither is hand-written, and both are checked.
