# W5.4c — the update engine reaches the adapters through the contract's port

## What the slice is

W5.4's second ratchet is the **import** one (seven edges). W5.4b took the
first of them (the toolbox); this slice takes **two** at once — the two
`adapter-bypass` edges into the update module — and the ratchet goes
**6 → 4**.

**The edges:** `adapter-bypass kernel/controlplane -> kernel/update` and
`adapter-bypass kernel/restapi -> kernel/update`.

The control plane and the REST surface are **adapters** (layer 5). The update
engine is a **module** (layer 3). An adapter reaches the app layer's typed
operations, never a module (archcheck's principle P2) — so both adapters
importing the update module was debt.

The REST surface's own comment claimed the opposite: *"The update service is
injected via SetUpdateService so this package stays decoupled from
kernel/update."* The claim was false — the package imported the module, named
its concrete `*update.Service` in a setter, and built its answers from the
module's rows. After this slice the claim is true of the code.

## What the measurement said, and where it was half right

The measured map called this pair *"the update service's port (`kernel/app/update`
already exists)"* — and that was right about the shape but wrong about the
work.

Right: the app layer already owned the update use cases
(`kernel/app/update`, with `Backend`, `Service` and the typed operations), so
the adapters were not supposed to drive the engine at all.

Wrong: **the app's `Backend` was built from the module's rows**
(`core.CheckResult`, `core.UpdateInfo`, `core.DrainResult`), so moving the
*adapters* to the app layer would not have closed the edge. The port's
*shape* was the module's, and an adapter holding it had to name the module's
types to implement it.

So the port's rows had to move down to the contract, and the app's port had
to become an **alias** of the contract's — not a second interface with the
same verbs, which would drift from the contract on its own.

## What changed

### One contract: `kernel/contract/updateapi`

The rows (`UpdateInfo`, `CheckResult`, `DrainResult`, `Manifest`, `Source`,
`Provenance`), the errors that cross the module's boundary
(`ErrDrainTimeout`, `ErrChecksumMismatch`, `ErrSignatureInvalid`) and the
port (`Backend`) live in the contract. The contract imports the standard
library only (P4).

The module re-exports the rows under its own name as **aliases**, so there is
one row and one spelling of it.

The errors that do **not** cross the boundary stay in the module
(`ErrUpdateInProgress`, `ErrSignatureMissing`, `ErrSignatureKeyNotConfigured`):
they are the engine's own refusals, and a contract that carries them would be
a contract with unused rows.

### The module implements the port

`update.Service` satisfies `updateapi.Backend` — asserted at compile time in
the module's own test.

### The app's port is an alias

`kernel/app/update.Backend` is now `= updateapi.Backend`. The app layer no
longer imports the update module **at all**, not even in its tests.

### The adapters hold the contract's port

- The control plane's `Deps.UpdateSvc` and `SetUpdateService` take
  `updateapi.Backend`.
- The REST surface's field and `SetUpdateService` take `updateapi.Backend`.
- The daemon (the composition root) converts the concrete engine to the port.

### One current version, not two

The update module kept `var CurrentVersion = brand.Version` — a second copy of
the running version, read by the module and by both adapters. There is now
**one** current version in the tree: the brand's, which the build stamps. The
adapters ask `brand.Version`; the module compares against `brand.Version`. A
test pins `brand.Version` (and restores it), exactly as it pinned the copy.

### The daemon converts where the value is still concrete

A nil `*update.Service` handed to an interface parameter is a **NON-nil**
interface: the holder would hold an engine that is not there, every nil check
downstream would be false, and the "update is disabled" answer could never
fire. The daemon converts at the wiring point (`updatePort`), which is the
same rule the board's port follows (W3.59).

## The bug this slice found

`ErrChecksumMismatch.Error()` did `e.Want[:8]` — and the **wanted** hash comes
from a manifest, and a manifest may carry a hash shorter than eight
characters. Reporting a failed checksum therefore **panicked**: a slice out of
range, in the error path, on a daemon that is already unhappy. The REST
surface reached it directly (`POST /api/v1/update/apply` with
`"sha256":"abc"`), and the CLI path would have taken the daemon with it.

The message now **truncates** instead of slicing. A permanent test proves it
end to end: the REST handler answers `400 checksum_mismatch` with
`want=abc` instead of crashing.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `TestTheChecksumMismatchMessageSurvivesAShortHash` | **the bug**: a short wanted hash produces a message, not a panic |
| `TestTheChecksumMismatchMessageTruncatesLongHashes` | a full digest is shown by its first eight characters |
| `TestTheSignatureInvalidMessageCarriesItsReason` | the reason is the whole message of that error |
| `TestTheModuleRowsAreTheContractRows` | the module's rows are the contract's rows — aliases, not copies |
| `TestTheModuleSentinelsAreTheContractSentinels` | the drain timeout is **one** sentinel, not two |
| `TestTheModuleServiceAnswersThroughTheContractPort` | the module's service is the contract's port and answers through it |
| `TestTheModuleCarriesTheRunningVersion` | the engine's interval and the brand's version are the module's business |
| `TestTheControlPlaneHoldsTheUpdateContractPort` | the control plane's setter and deps take the **contract's** port — asserted statically, because an anonymous interface with the same verbs would compile and would be a second shape of the same port |
| `TestTheRestAPIHoldsTheUpdateContractPort` | the REST surface's setter takes the contract's port |
| `TestTheUpdateOperationsReachTheContractPort` | the operations answer with the **port's** rows |
| `TestTheUpdateOperationsWithoutAPortReportDisabled` | a daemon with no engine answers "update is disabled" and still names the running version |
| `TestTheUpdatePortIsTheContractPort` | the port the server holds is the contract's |
| `TestTheApplyAnswersAChecksumMismatchWithAShortHash` | **the bug, end to end**: the REST surface answers `400 checksum_mismatch` instead of crashing |
| `TestTheUpdateCheckWithoutAnEngineReportsTheRunningVersion` | the disabled answer names the running version |
| `TestTheNilEngineBecomesANilPort` | **the trap**: a nil engine becomes a nil port, so the disabled answer stays reachable |
| `TestTheEngineBecomesThePort` | a wired engine is handed over as the contract's port |

### The mutation run

13 mutations, 13 CAUGHT, two independent runs with byte-identical logs.

| # | Mutation | Caught by |
|---|---|---|
| 1 | the checksum message slices the wanted hash again — a short hash panics | `TestTheChecksumMismatchMessageSurvivesAShortHash` |
| 2 | the checksum message shows the whole hash | `TestTheChecksumMismatchMessageTruncatesLongHashes` |
| 3 | the module's update row is a copy of the contract's row, not an alias | `TestTheModuleRowsAreTheContractRows` |
| 4 | the module's drain row is a copy of the contract's row | `TestTheModuleRowsAreTheContractRows` |
| 5 | the module keeps its own drain-timeout sentinel, so the adapters would report a different error | `TestTheModuleSentinelsAreTheContractSentinels` |
| 6 | the app keeps its own port instead of the contract's | `TestTheControlPlaneHoldsTheUpdateContractPort` |
| 7 | the control plane throws the update port away | `TestTheUpdateOperationsReachTheContractPort` |
| 8 | the daemon's deps never reach the server | `TestTheUpdateOperationsReachTheContractPort` |
| 9 | the control plane answers with an empty current version | `TestTheUpdateOperationsWithoutAPortReportDisabled` |
| 10 | the REST surface throws the update port away | `TestTheApplyAnswersAChecksumMismatchWithAShortHash` |
| 11 | the disabled answer does not name the running version | `TestTheUpdateCheckWithoutAnEngineReportsTheRunningVersion` |
| 12 | the checksum mismatch is no longer reported as a mismatch | `TestTheApplyAnswersAChecksumMismatchWithAShortHash` |
| 13 | a nil engine becomes a non-nil port — the disabled answer can never fire | `TestTheNilEngineBecomesANilPort` |

The first calibration pass reported one **SURVIVED** and two **BUILDERR**s,
and all three were the **mutation's** defects, not gaps in the tests:

- the truncation assertion was a **prefix** of the untruncated hash
  (`have=01234567` is inside `have=0123456789abcdef`), so the mutation that
  showed the whole hash still satisfied it — the assertion now also demands
  that the whole hash is **absent**;
- the two mutations that removed the only use of an import broke the build
  before the behaviour could be measured (one of them also removed the port
  the adapter holds), so they were re-pointed at behaviour that is observable
  without breaking the build.

The two reported runs are the clean ones.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 317 packages placed; 4 allowlisted import violations and 0 allowlisted call sites remain (ratchet)** — **import edges down from 6** |
| `go run ./tools/deadcodecheck` | OK — 31 public SDK findings and 1 cross-package test seams allowlisted |
| `go run ./tools/depscheck` | OK — 24 core dependencies, all justified |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## What the ratchet says now

Four import edges remain, and none of them is a "the adapter reached a module"
edge any more:

| Edge | What it needs |
|---|---|
| `adapter-bypass kernel/controlplane -> kernel/runtime` | the 37 verbs the control plane still takes from the kernel — a redesign, not a port |
| `cross-module kernel/cadence/systemtasks -> kernel/runtime` | nine kernel accessors, two of which are platform types the contract may not name |
| `cross-module kernel/modules/agents/selfrepair -> kernel/runtime` | a runs port that does not exist yet |
| `upward kernel/channel -> kernel/bus` | 26 plugin configs holding `*bus.Bus` |

## The rules the slice is really about

**A comment that claims a decoupling is a claim about the code.** The REST
surface said it was "decoupled from kernel/update" while importing it. The
claim became true because the code changed — not because the comment did.

**A port's rows belong to the contract when the adapters build their answers
from them.** The app's `Backend` was already a port; it was built from the
module's rows, so every holder of the port had to name the module's types.
The rows went down to the contract, and the app's port became an **alias** of
the contract's.

**An error message must not panic.** `e.Want[:8]` on a value that comes from
outside the program is a crash waiting for a short input. The message
truncates now. The general rule: **a value from a manifest, a request body or
a user is never safe to slice**.

**A copy of the truth is a second truth.** The update module kept its own
`CurrentVersion` while the brand kept the real one. Two copies of the running
version can drift, and only the tests ever changed one of them.

**A nil concrete value handed to an interface parameter is not nil.** The
daemon converts where the value is still concrete, so the "update is
disabled" answer stays reachable. This is the board's rule, applied a second
time.
