# W3.42 — the data lake is the daemon's file

## What the slice is

The kernel still reached **19** modules after W3.41. This slice takes **two**
of those edges at once — the kernel's own and the run engine's — and it is
the cheapest of the remaining ones, because the contract side was already
done:

```go
// kernel/contract/datalakeapi/lake.go
type Lake interface { ... }   // nine verbs

// kernel/modules/artifacts/datalake/aliases.go
var _ datalakeapi.Lake = (*Lake)(nil)
```

The port existed, the module already asserted it, and every row the lake
carries (`Schema`, `Record`, `Query`, `Field`, `CollectionInfo`,
`ErrNotFound`, `ErrExists`, `ErrSystem`) was already an alias of the
contract's. **Nothing had to move.** What was missing was the kernel's own
use: it still opened the lake and held the concrete type.

The ratchet went **63 → 61**.

## What the kernel used to do

```go
// kernel/runtime/compose.go
lake, err := datalake.Open(cfg.BaseDir, func() int64 { return time.Now().UnixMilli() })
if err != nil {
    return fail("runtime: data lake", err)
}
// Seed the built-in Personal Data Lake collections (M835) — expenses, calendar,
// tasks, notes, habits, bookmarks, contacts. Idempotent … and best-effort.
_, _ = lake.SeedBuiltins("system")
```

and it held the concrete lake:

```go
lake *datalake.Lake                       // runtime_kernel.go
func (k *Kernel) DataLake() *datalake.Lake // accessors_standing.go
DataLake() *datalake.Lake                 // runexec/api.go — the run engine's API
```

The kernel was opening a module's file and holding a module's type. That is
the edge.

## What it does now

The kernel holds the **port** and the composition root owns the **file** —
the same shape W3.41 established for the seat catalog:

| Where | Before | After |
|---|---|---|
| `runtime_kernel.go` | `lake *datalake.Lake` | `lake datalakeapi.Lake` |
| `accessors_standing.go` | `DataLake() *datalake.Lake` | `DataLake() datalakeapi.Lake` |
| `runexec/api.go` | `DataLake() *datalake.Lake` | `DataLake() datalakeapi.Lake` |
| `compose.go` | opened the lake, seeded it | opens nothing |
| `runtime.Config` | — | `Lake datalakeapi.Lake` |
| `cmd/agezt` | — | opens the lake, **seeds it**, injects the port |

The seeding moved with the file, and it is worth saying why: **seeding is
not part of the port.** The port is how a host drives the lake; seeding is
what the owner of the lake's files does once, at startup. The daemon does it
while it still holds the concrete lake:

```go
// cmd/agezt/main_toolinvoker.go
if cfg.Lake == nil {
    lake, err := datalake.Open(cfg.BaseDir, func() int64 { return time.Now().UnixMilli() })
    if err != nil {
        return nil, err
    }
    // The seeding happens HERE, while the daemon still holds the concrete
    // lake: seeding is not part of the port the host is given.
    _, _ = lake.SeedBuiltins("system")
    cfg.Lake = lake
}
```

The posture is unchanged: a lake that cannot be opened is a **startup
error**, and a seeding hiccup is still best-effort (the next start retries).

## The wrapper is gone with the wrapping

The daemon used to turn the concrete lake into the port, and the wrapper
carried a rule:

```go
// before — cmd/agezt/tooldeps.go
func lakePort(l *datalake.Lake) datalakeapi.Lake {
    if l == nil {
        return nil // a nil *Lake in an interface is already a NON-nil interface
    }
    return l
}
```

That wrapper existed only because the kernel held a concrete type. Now the
kernel holds the port, so there is nothing to wrap — and with it goes the
only place where a missing lake could have become a non-nil port. The rule
itself did not disappear: it moved into the kernel, where the field is an
interface and a kernel given no lake holds a **nil** interface (not a nil
pointer in an interface), and into the tool wiring, where the port the daemon
injected is the port the tool gets.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/runtime`: `TestTheKernelHoldsTheLakePortItWasGiven` | the kernel holds **exactly** the port it was given — not a wrapper of its own, and not a lake of its own |
| `kernel/runtime`: `TestTheKernelDoesNotOpenADataLakeOfItsOwn` | a kernel given no lake holds **nil** — and not a non-nil interface holding a nil pointer (the typed-nil trap) |
| `cmd/agezt`: `TestOpenAppKernelWiresTheDataLake` | the daemon end: the port is live **and the built-in collections are seeded** — the seeding assertion moved here with the seeding |
| `cmd/agezt`: `TestToolKernelDepsSuppliesTheLakePort` | the db tool's port answers the **live** lake (it now opens the kernel through the composition root, because a kernel opened without the root has no lake) |
| `cmd/agezt`: `TestLakeStoreStaysANilPortWithoutALake` | a missing kernel — and a kernel with no lake — still yield a **nil** port |
| `kernel/app/data`: `TestUnavailable` | the app's end: every operation refuses with *"data lake unavailable"* when there is no lake (this rule already existed; the slice keeps it) |

### The mutation run

**6 mutations: 6 caught, 0 survived, 0 skipped, 0 build errors.**

| # | Mutation | Caught by |
|---|---|---|
| 1 | the kernel opens the data lake again (the module edge returns) | **archcheck** — `cross-module kernel/runtime -> kernel/modules/artifacts/datalake` |
| 2 | the kernel **wraps** the lake port it was given | `TestTheKernelHoldsTheLakePortItWasGiven` |
| 3 | the daemon stops injecting the data lake | `TestOpenAppKernelWiresTheDataLake` and `TestToolKernelDepsSuppliesTheLakePort` |
| 4 | the daemon stops **seeding** the built-in collections | the same two — the built-ins are missing |
| 5 | the app service stops refusing when there is no lake | `TestUnavailable` |
| 6 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |

**Two of the six are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that
opens the lake itself, as long as the lake works.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 310 packages placed; 61 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 63** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rule the slice is really about

**When the port already exists, the slice is the kernel's use — not the
contract.** The datalake needed no new contract surface: the port was there,
the rows were aliases, the module already asserted the port. Everything that
was left was the kernel opening a module's file and holding a module's type.
Measuring first is what made this a two-edge slice instead of a redesign.

**Seeding is the owner's job, and it is not part of the port.** A port says
how to drive a thing; it does not say how the thing is prepared. The daemon
owns the lake's files, so the daemon seeds them — and the test that used to
assert "the live lake carries its built-in collections" moved with the
seeding, to the composition root.

## What remains

The kernel still reaches **17** modules. The next candidates, in the order
the measurement suggests:

- **`artifact`** — the only one that unblocks the three media tool seams
  (`imagetool`, `reranktool`, `voicetool`): the voice and image tools are
  bound to the artifact store *inside the kernel*. It needs new contract
  surface (a blob port, a wider index port, and `artifact.RefGuard`).
- **`toolforge`** — the rows are aliases, but the kernel calls **eight**
  store methods that the contract's two-verb port does not cover.
- **`workflow` and `workboard`** — both need wide new ports: the kernel
  names `workflow.Engine`, `workflow.Ports`, `workflow.Provenance` and
  `workflow.Trigger` (no contract home), and it calls **seventeen**
  workboard store methods.
