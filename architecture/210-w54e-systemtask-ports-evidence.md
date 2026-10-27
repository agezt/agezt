# W5.4e — the system tasks reach the daemon's capabilities through the contract's ports

## What the slice is

W5.4's second ratchet is the **import** one. W5.4b took the toolbox edge,
W5.4c took the two update edges, W5.4d took the channel edge; this slice
takes the **system-task** edge, and the ratchet goes **3 → 2**.

**The edge:** `cross-module kernel/cadence/systemtasks -> kernel/runtime`.

The cadence module (layer 3, module `triggers`) reached the runs module
(layer 3, module `runs`). A module may not name another module — it must go
through that module's public vocabulary — and the system tasks did not even
name a *type*: they took the whole kernel.

```go
func Run(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID, task string) error
```

Every one of the seven executors took the kernel as its argument and asked
it for what it needed. The kernel was the API.

## What the measurement said

The measurement counted **nine** accessors and **twenty-one** call sites,
and it flagged the two that a contract port may not name:

| Accessor | Return type | A contract may name it? |
|---|---|---|
| `Memory()` | `memoryapi.Manager` | ✅ contract |
| `DistillProfile(ctx, corr)` | `memoryapi.ProfileReport` | ✅ contract |
| `RosterList()` | `[]agentapi.Profile` | ✅ contract |
| `ArtifactIndex()` | `artifactapi.HostIndex` | ✅ contract |
| `CatalogSyncer()` | `catalogapi.Syncer` | ✅ contract |
| `CatalogStore()` | `catalogapi.Store` | ✅ contract |
| `Reload()` | `(*catalogapi.Catalog, bool, error)` | ✅ contract |
| `Bus()` | `*bus.Bus` | ❌ **platform** (layer 2) |
| `Journal()` | `*journal.Journal` | ❌ **platform** (layer 2) |

**Seven of the nine were already contract rows.** The measurement's first
question — "how wide is this port?" — turned out to be the easy one.

The hard question was the other two. The bus and the journal are **platform**
packages, and a contract package may import the standard library and other
contract packages **only** (P4). So the port could not simply carry the
concrete types.

The answer was already in the tree, twice:

- `eventapi.Publisher` — the contract's own one-verb publish port. The
  platform's event package keeps `Spec` and `Kind` as **aliases** of the
  contract's, so a spec built with `event.KindInfo` crosses the port
  unchanged: the same row, not a translation.
- `journalapi.Reader` — the journal's own read port, over the journal's own
  `Record` projection. The log task needed the **whole** history and the
  chain's head, so the journal's contract grew a second read port:
  `journalapi.Scanner` (`Scan` + `Head`) over the **same** record.

## What changed

### The contract owns the port set

`kernel/contract/cadenceapi/ports.go` (new) carries `SystemTaskPorts`: the
capability set the seven tasks run on. It names only contract rows and
contract ports — `eventapi.Publisher`, `memoryapi.Maintainer`,
`agentapi.Profile`, `journalapi.Scanner`, `catalogapi.Syncer`,
`catalogapi.Store` — plus four plain verbs for the capabilities that have no
port of their own (`DistillProfile`, `RosterList`, `CollectArtifacts`,
`Reload`).

### The ports are NARROW, and that is the point

Two of them were narrowed on purpose, and the narrowing is what the house
style asks for:

- **The artifact index** is a **ten-verb** host port (`artifactapi.HostIndex`),
  and two of its verbs **delete bytes** while a third (`SetRefGuard`) switches
  off the journal's protection of the bytes the journal still names. The
  collect task needs exactly one of them, so the port set carries
  `CollectArtifacts func(olderThanMs int64) (collected int, bytes int64)` and
  **not** the index. A maintenance job that could clear the ref guard could
  make the collector delete bytes a run's output still points at.
- **The memory** is the whole manager — recall, write, distil, the tool. The
  clean and tidy tasks need two hygiene verbs, so the port set carries the
  contract's **maintainer** half (`memoryapi.Maintainer`, five verbs) and not
  the manager.

### The kernel hands the ports over

`kernel/runtime/systemtask_ports.go` (new): `func (k *Kernel) SystemTaskPorts() cadenceapi.SystemTaskPorts`.

The conversion happens **where the values are still concrete** — the same
rule the board, the update engine and the channel guard each applied in their
own slice. A nil `*bus.Bus` or a nil `*journal.Journal` handed to an
interface parameter is a **non-nil interface**, and the holder would then
hold a capability that is not there. So:

- a capability the daemon does not have is a **nil port**, and the port set
  is checked on the concrete value;
- the artifact collect port is **nil** when the host opened no artifact index,
  and the closure that captures the index is only built when there is one.

The kernel is the right owner here for the same reason it owns the memory
publisher (`memory_publisher.go`) and the reaper's ports (`reaper.go`): the
capability is the kernel's own, and the module that consumes it must not
name the kernel.

### A missing capability is a REFUSAL, not an empty answer

"This daemon has no memory" and "the memory is empty" are different facts, so
every task that needs a capability checks for it and **refuses**:

| Missing | The task says |
|---|---|
| memory | `schedule <id>: memory unavailable` |
| journal | `schedule <id>: journal unavailable` |
| artifact index | `schedule <id>: artifact index unavailable` |
| roster | `schedule <id>: roster unavailable` |
| catalog library | `systemtasks: the catalog library is not available on this daemon` |
| reload path | `schedule <id>: catalog reload unavailable` |
| publisher | `schedule <id>: the event bus is unavailable` |

The publish error is still **discarded** — a summary event that could not be
written must not fail the task it summarises — but a daemon with **no
publisher at all** has nowhere to write to, and that is a refusal.

### The defect the slice found

The catalog sync's reload path built its error message from the port's
answer **without checking it**:

```go
freshCat, providersReloaded, provErr := ports.Reload()
if freshCat == nil {
    ... "error": provErr.Error()   // ← a nil error would panic here
```

A reload that answers *no catalog* and *no error* would have taken the
daemon down in the error path — the same class as the checksum slice's
`want[:8]` panic (W5.4c). It is not reachable with the catalog store as it
stands today (`Load` never returns a nil catalog), but the port makes the
contract visible: **the port answers three values, and one of them may be
nil**. The message is now built from what the port actually answered, and a
permanent test holds it.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `TestTheSystemTaskPortsNameOnlyContractRows` | every field of the port set is a contract's own row or port, and the set has exactly nine fields |
| `TestThePortsAreNilWhenTheDaemonHasNoCapability` | an empty port set holds nothing — a missing capability is a nil field |
| `TestTheArtifactPortIsOneVerbWide` | the collect port is one verb, not the index's ten |
| `TestTheMemoryPortIsTheMaintainerHalf` | the memory port is the hygiene half, not the whole manager |
| `TestTheDistillPortTakesTheCallersContext` | the distill verb carries the caller's context and correlation |
| `TestTheScanPortCarriesExactlyTwoVerbs` | the journal's scan port is `Scan` + `Head` and nothing else |
| `TestRunScheduledMemoryCleanPublishesSummary` | the clean runs for real (not a dry run), under the caller's correlation, and publishes the report's numbers |
| `TestRunScheduledMemoryTidyPublishesSummary` | the tidy publishes the collapsed count |
| `TestRunScheduledLogCleanPublishesSummary` | the log task counts the events, finds the oldest and the latest, and reads the head |
| `TestRunScheduledGraveyardScanReportsOnly` | the graveyard counts the **retired** agents, and reports only |
| `TestRunScheduledGraveyardScanFlagsTheEligible` | the eligible list is the retired ones past the window, **sorted** |
| `TestRunScheduledArtifactCollectPublishesSummary` | the collect passes a cutoff and publishes the counts |
| `TestRunScheduledCatalogSyncPublishesSummaryAndReloads` | the sync fetches, **saves the fetched bytes**, reloads the providers, and publishes the counts |
| `TestRunScheduledProfileDistillPublishesSummary` | the distill task publishes its report (it had **no** test before this slice) |
| `TestTheTasksRefuseWhenTheDaemonHasNoCapability` | nine refusals: each missing capability is refused by name |
| `TestTheCatalogSyncSurvivesAReloadThatAnswersNoError` | a reload that answers no catalog and no error does not take the daemon down |
| `TestTheCatalogSyncReportsTheReloadError` | a reload that fails publishes **that** error |
| `TestRunDispatchesEverySystemTask` | every task the catalogue names is reachable through the dispatch — the catalogue and the dispatch are two lists, and a task in one and not in the other never runs |
| `TestRunRejectsAnUnknownTask` | an unknown task name is an error |
| `TestTheSystemTaskPortsHoldTheKernelsCapabilities` | the kernel hands over its bus, journal, distill path, roster and reload |
| `TestTheSystemTaskPortsStayNilWithoutTheCapability` | a kernel without a memory, an index or a catalog library hands over **nil** ports |
| `TestTheSystemTaskPortsCarryTheArtifactIndex` | the collect port appears when the host opened an index |
| `TestTheSystemTaskScannerProjectsTheJournalsRecords` | the scan port hands back the contract's projection — kind, correlation, timestamp, payload |
| `TestTheSystemTaskPublisherWritesToTheBus` | the publish port writes the event the task handed over |
| `TestTheSystemTasksRunOnTheContractsPorts` | the daemon's dispatch runs the tasks on the **contract's** port set (checked statically) |
| `TestTheKernelHandsOverTheContractsPortSet` | the kernel's face is the contract's port set (checked statically) |

### The mutation run

**27 mutations, 27 CAUGHT**, in two independent passes with **byte-identical**
logs.

The calibration passes found **four** things, and three of them were **test
gaps**:

- **The memory tidy's refusal was never tested.** The refusal table covered
  the clean but not the tidy, so removing the tidy's nil check changed
  nothing observable. The table now covers both.
- **The graveyard test could not tell the retired agents from the living
  ones.** Its roster held one of each, so counting the living instead of the
  retired produced the same number. The roster now holds two living agents
  and one retired one.
- **The catalog sync's saved bytes were never asserted.** The test counted
  the saves but never looked at *what* was saved, so a save of empty bytes
  passed. It now asserts the bytes are the ones the syncer fetched.
- **One mutation was the mutation's fault** (`BUILDERR`): dropping the
  `SaveAPI` call orphaned the fetched bytes, and the compiler refused the
  build before the behaviour could be measured. The mutation was rewritten to
  save *empty* bytes — which compiles, and which the new assertion catches.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 317 packages placed; 2 allowlisted import violations and 0 allowlisted call sites remain (ratchet)** — **import edges down from 3** |
| `go run ./tools/deadcodecheck` | OK — 31 public SDK findings and 1 cross-package test seams allowlisted |
| `go run ./tools/depscheck` | OK — 24 core dependencies, all justified |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## What the ratchet says now

Two import edges remain, and neither of them is a "one function reached
another module" edge any more:

| Edge | What it needs |
|---|---|
| `adapter-bypass kernel/controlplane -> kernel/runtime` | the 37 verbs the control plane still takes from the kernel — a redesign, not a port |
| `cross-module kernel/modules/agents/selfrepair -> kernel/runtime` | a runs port that does not exist yet |

## The rules the slice is really about

**A module that takes the kernel takes the kernel's whole API.** The seven
executors took a `*kernelruntime.Kernel` and asked it nine questions. The
type was the coupling, and the type was the whole problem: nothing about
"the log task wants to walk the journal" says "the log task may name the
runs module".

**The port's width is what the consumer actually drives.** The artifact index
has ten verbs and two of them delete bytes; the collect task needs one. The
memory has five halves and the clean task needs one.

**A port that answers three values may answer one of them as nil.** The
error path that read the reload's error without checking it is the same
class as the checksum slice's sliced hash: the message is built from what
the port answered, and the port's answer may be empty.

**A missing capability is a refusal, not an empty answer.** Seven refusals,
each naming what is missing — because "no memory" and "an empty memory" are
different facts, and a maintenance job that reports the second one is
reporting a store it never looked at.
