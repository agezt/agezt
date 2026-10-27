# W5.4n — the application fills the council's own ports

## What the slice is

The measurement behind this slice (the family map of the control plane's
kernel use) found something worth building on:

> the control plane's kernel use is **almost entirely port filling** — it
> passes `*runtime.Kernel` methods into `kernel/app/<family>` port structs

The adapter was not *deciding* anything with the kernel; it was **wiring** the
council's panel, its seating, its convening and its conductor. And wiring is
the application's business: the application knows which capabilities the
council needs and how each of them answers.

So the council's ports moved into the application:

```go
// kernel/app/council/ports.go
func PortsFromKernel(k *runtime.Kernel, baseDir string) Ports
func EnginesFromKernel(k *runtime.Kernel) Engines
```

The shape already exists in the repo: `app/catalog`, `app/providers` and
`app/runs` take the kernel handle and build their own ports. The council
joins them.

## What moved

| | Before | After |
|---|---|---|
| the panel | `s.k.CouncilDefaultMembers()` in the host | the application's `Members` port |
| the seating | `s.k.SetCouncilMembers(...)` in the host | the application's `SetMembers` port |
| the store | `settings.NewStore(s.baseDir)` in the host | the application's `Store` port |
| the models | `s.k.Catalog().FindModel` in the host | the application's `Models` port |
| the convening | `k.Council(...)` in the host | the application's `Convene` engine |
| the conductor | `k.Conduct` in the host | the application's `RunConductor` engine |
| the correlation | `k.NewCorrelation` in the host | the application's `NewCorrelation` engine |

**Six** kernel verbs left the adapter. The host now says one sentence about
the council: *here is the handle*.

One subtlety worth naming: the council's members are **contract rows**
(`reasoningapi.CouncilMember` is the reasoning module's row, which is the
contract's), so the port hands them over without a conversion — the same
identity W5.4m found.

## The proof

### Permanent tests — 2 new, 1 extended

| Test | What it holds |
|---|---|
| `TestTheCouncilPanelReachesTheApplication` (`kernel/controlplane`) | the panel the council publishes is the panel the kernel holds, the panel the application seats is the panel that comes back, and the seating is applied |
| `TestTheAdapterCallsNoApplicationOwnedVerbOnTheKernel` (`kernel/controlplane`) | **the slice's product at verb granularity**: the host may not call the council's verbs on the handle. The GATE counts packages and cannot see this; the test counts verbs. |

The existing `TestCouncilAskBindsTheKernel` covers the moved engines (the
panel's answer, its members and its opinions).

### The mutation run — 4 mutations, 4 caught

Two independent passes with **byte-identical** logs (SHA-256
`4E4BB8C4…5FA34AE`).

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01 | the application's panel port answers empty — the council seats nobody | the council wiring test |
| M02 | the application's seating port drops the panel — the council keeps the old seats | the council wiring test |
| M03 | the application's council engine answers nothing — the panel never speaks | the council ask test |
| M04 | the host fills the council's ports itself again — the wiring drifts back to the adapter | the verb-grained guard |

M04 is the one that matters most: it is the regression this slice exists to
prevent, and **no behavioural test could catch it** — the wiring would work
exactly as well, it would simply be in the wrong layer. Only the verb-grained
guard names it.

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

## The programme this slice belongs to

The measurement gave the whole map, and it is worth recording here because it
is the shape of the rest of W5.4:

- The adapter calls **59** distinct kernel verbs.
- **All but four** return a type an application-layer port may name:
  contract, platform, module-api or a scalar.
- The four blockers are `Reflect()` (a module's implementation type),
  `LoopRunner()` and `RunPlan()` (the scheduler's), and the raw
  `*runtime.Kernel` handle itself.
- The kernel's accessors already return the modules' **api** types, so a port
  in the application layer can name them ✓.
- The per-request kernel arrives through two context keys: the routed
  (tenant) kernel and the server's primary kernel.
- The daemon builds the control plane at `cmd/agezt/main.go` with a
  `controlplane.Deps` that already carries thirteen injected capabilities.

So the remaining work is mechanical, one family at a time: move the family's
port-filling into the application (this slice's shape), then hand the
application's services to the host instead of the handle, until the last
family is gone and the handle goes with it.

This slice took the **first** family. The ratchet does not move yet — the
control plane still imports the runtime in **15** files — but the pattern is
now proven end to end, with a test that holds the wiring where it belongs.
