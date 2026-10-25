# W3.43 — the artifact store is the daemon's file

## What the slice is

The kernel still reached **19** modules after W3.42. This slice takes the
**artifact** family — the content-addressed blob store (SPEC-04 §3.6), the
metadata index over it (M822) and the ref guard that keeps the collector away
from the bytes the journal still names — and it is the one that unblocks the
three media tool seams.

The contract side did **not** exist, so this slice had to write it:

```go
// kernel/contract/artifactapi/artifactapi.go
type Blob interface { Put, Get, Has, Size }        // four verbs
type RefGuard interface { ReferencedRefs }          // one verb
type Index interface { PutEntry, List, Bytes, Delete }   // four verbs
type HostIndex interface { Index; Get, Count, IndexRef, StaleEntries, Collect, SetRefGuard }
```

The ratchet went **61 → 59**.

## What the kernel used to do

```go
// kernel/runtime/compose.go
artStore, err := artifact.Open(filepath.Join(cfg.BaseDir, "artifacts"))
if err != nil {
    return fail("runtime: artifacts", err)
}
artIndex, err := artifact.OpenIndex(artStore, filepath.Join(cfg.BaseDir, "artifacts"))
if err != nil {
    return fail("runtime: artifact index", err)
}
artIndex.SetRefGuard(&journalRefGuard{j: j})
```

and it held the concrete types:

```go
artifacts *artifact.Store          // runtime_kernel.go
artIndex  *artifact.Index          // runtime_kernel.go
func (k *Kernel) Artifacts() *artifact.Store
func (k *Kernel) ArtifactIndex() *artifact.Index
func (k *Kernel) ArtifactStore() *artifact.Store     // runtime_lifecycle.go
```

The kernel was opening a module's file and holding a module's type — twice,
once for the blobs and once for the index, and once more in the run engine's
`KernelAPI` declaration.

## What it does now

The kernel holds the **ports** and the composition root owns the **files** —
the shape W3.41 established for the seat catalog and W3.42 for the data lake:

| Where | Before | After |
|---|---|---|
| `runtime_kernel.go` | `artifacts *artifact.Store`, `artIndex *artifact.Index` | `artifacts artifactapi.Blob`, `artIndex artifactapi.HostIndex` |
| `accessors_standing.go` | `Artifacts() *artifact.Store` | `Artifacts() artifactapi.Blob` |
| `accessors_standing.go` | `ArtifactIndex() *artifact.Index` | `ArtifactIndex() artifactapi.HostIndex` |
| `runtime_lifecycle.go` | `ArtifactStore() *artifact.Store` | `ArtifactStore() artifactapi.Blob` |
| `runexec/api.go` | `ArtifactIndex() *artifact.Index` | `ArtifactIndex() artifactapi.HostIndex` |
| `compose.go` | opened the store **and** the index | opens nothing |
| `runtime.Config` | — | `Artifacts artifactapi.Blob`, `ArtifactIndex artifactapi.HostIndex` |
| `cmd/agezt` | — | opens both, injects both |

The daemon's side is the composition root's, and it opens the pair the way the
kernel used to — same path, same order:

```go
// cmd/agezt/main_toolinvoker.go
if cfg.Artifacts == nil {
    store, err := artifact.Open(filepath.Join(cfg.BaseDir, "artifacts"))
    if err != nil {
        return nil, err
    }
    index, err := artifact.OpenIndex(store, filepath.Join(cfg.BaseDir, "artifacts"))
    if err != nil {
        return nil, err
    }
    cfg.Artifacts = store
    cfg.ArtifactIndex = index
}
```

The posture is unchanged: a store that cannot be opened is a **startup error**,
and it is the daemon that refuses to boot now — not the kernel. The daemon's
behaviour is the same; only the owner of the file moved.

### The ref guard stayed with the journal

The guard is the one piece that did **not** move to the daemon, and that is
deliberate. The guard answers *"which blobs are still named from outside the
index?"* — and the answer comes from the **journal**, which is the kernel's.
So the kernel installs its own guard on the index it was given:

```go
// kernel/runtime/compose.go
if cfg.ArtifactIndex != nil {
    cfg.ArtifactIndex.SetRefGuard(&journalRefGuard{j: j})
}
```

Only the **port** moved to the contract layer (`artifactapi.RefGuard`, with the
module keeping `type RefGuard = artifactapi.RefGuard`), because the host has to
install a guard on an index it does not own.

## The port had to be split, and the first attempt was wrong

The first version of this slice put **ten** verbs on `artifactapi.Index`: the
four the tools use, plus the six the host uses. It broke a test that had been
in the tree long before this slice:

```go
// kernel/contract/artifactapi/artifactapi_test.go
func TestTheIndexPortHasExactlyFourMethods(t *testing.T) { ... }
// the artifact port is the whole of what a tool may do to the daemon's index
// — store an entry, list them, read one back, delete one. A fifth method
// would be a new reach into the daemon, and it would have to be earned here.
```

That guard is not a style rule. A plugin receives this port, so **every method
on it is a reach the plugin holds** — and two of the six host verbs are not
merely unnecessary for a tool but dangerous:

- `Collect` deletes entries.
- `SetRefGuard` **switches off the journal's protection** of the bytes the
  journal still names. A plugin that could clear the guard would make the
  collector delete bytes a run's output still points at.

So the port is **two** ports:

| Port | Verbs | Who holds it |
|---|---|---|
| `artifactapi.Index` | `PutEntry`, `List`, `Bytes`, `Delete` | the **tool registry** — every plugin that touches artifacts |
| `artifactapi.HostIndex` | those four **plus** `Get`, `Count`, `IndexRef`, `StaleEntries`, `Collect`, `SetRefGuard` | the **host** — the kernel, the daemon, the scheduler's collector |

The daemon holds one index; the kernel holds it as `HostIndex`; the tool
registry receives the narrower `Index`. One index, one wiring site — and the
plugin's reach is unchanged from before the slice.

The module answers both:

```go
// kernel/modules/artifacts/artifact/index.go
var _ artifactapi.Index = (*Index)(nil)
var _ artifactapi.HostIndex = (*Index)(nil)
```

## The wrapper is gone with the wrapping

The daemon used to turn the concrete index into the port, and the wrapper
carried a rule:

```go
// before — cmd/agezt/tooldeps.go
func artifactPort(idx *artifact.Index) artifactapi.Index {
    if idx == nil {
        return nil // a nil *Index in an interface is already a NON-nil interface
    }
    return idx
}
```

That wrapper existed only because the kernel held a concrete type. Now the
kernel holds the port, so there is nothing to wrap — and with it goes the only
place where a missing index could have become a non-nil port. The rule itself
did not disappear: it moved into the kernel, where the field is an interface
and a kernel given no index holds a **nil** interface, and into the tool
wiring, where the port the daemon injected is the port the tool gets.

## The tests became hosts

Moving the file out of the kernel has a consequence that is easy to miss: a
kernel opened **without** the store has no artifact store at all. Every test
that opened a kernel and expected artifacts had to become a host itself —
open the store, inject the ports:

```go
// kernel/runtime/artifact_refs_test.go
func openWithArtifacts(t *testing.T, cfg runtime.Config) *runtime.Kernel {
    store, err := artifact.Open(filepath.Join(cfg.BaseDir, "artifacts"))
    ...
    index, err := artifact.OpenIndex(store, filepath.Join(cfg.BaseDir, "artifacts"))
    ...
    cfg.Artifacts = store
    cfg.ArtifactIndex = index
    k, err := runtime.Open(cfg)
    ...
}
```

The same helper exists in `cmd/agezt` (`withArtifacts`). The suites that
needed it: `kernel/runtime`, `kernel/controlplane`, `cmd/agezt`, and — two
suites the first test run caught — `plugins/builtintools` and
`kernel/cadence/systemtasks`.

The two that failed first are worth naming, because they failed for the same
reason and it is the reason the slice is honest about:

- `plugins/builtintools`: `TestArtifactsSpecBindsTheLiveIndex` opened a kernel
  and asked it for an index. There is none: the index is the host's file now.
- `kernel/cadence/systemtasks`: `TestRunScheduledArtifactCollectPublishesSummary`
  opened a kernel and ran the collector, which needs an index. There is none.

Both were fixed the same way: **the test became the host.** Neither was
weakened, and neither was made to tolerate a missing index — the collector
still refuses when the index is unavailable, and that refusal is still the
scheduled task's error.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/artifactapi`: `TestTheIndexPortHasExactlyFourMethods` | the tool-facing port stays **four** verbs wide (the pre-existing guard, kept green) |
| `kernel/contract/artifactapi`: `TestTheToolPortCannotReachTheHostsVerbs` | the tool port must **not** carry `Get`, `Count`, `IndexRef`, `StaleEntries`, `Collect` or `SetRefGuard` |
| `kernel/contract/artifactapi`: `TestTheHostPortAddsTheHostsVerbs` | the host port is the tool port **plus exactly** the six host verbs |
| `kernel/runtime`: `TestArtifactCollect_KeepsTheBlobTheJournalNames` | the journal's `raw_ref` survives the collector — the guard the kernel installs is live |
| `kernel/runtime`: `TestArtifactCollect_StillReapsWhatTheJournalDoesNotName` | the protection is narrow: an unnamed artifact is still collected |
| `kernel/runtime`: `TestArtifactCollect_ProtectsEveryJournalNamedBlob` | the guard answers for a whole batch, not only the first candidate |
| `cmd/agezt`: `TestOpenAppKernelWiresTheArtifactStore` | the daemon end: the two ports are live **and they are a pair** — an entry the index holds points at bytes the blob store holds |
| `cmd/agezt`: `TestToolKernelDepsSuppliesTheArtifactPort` | the tool registry's port answers the **live** index |
| `cmd/agezt`: `TestArtifactPortStaysANilPortWithoutAnIndex` | a kernel with no injected index yields a **nil** port, and the daemon hands that nil to the tools |
| `plugins/builtintools`: `TestArtifactsSpecBindsTheLiveIndex` | the plugin's tool binds the live index — the test is the host now |
| `kernel/modules/artifacts/artifact`: `var _ artifactapi.Index`, `var _ artifactapi.HostIndex` | the module answers **both** ports — a port change is a compile error in the module, not a runtime surprise |

### The mutation run

**8 mutations: 8 caught, 0 survived, 0 skipped, 0 build errors.**

| # | Mutation | Caught by |
|---|---|---|
| 1 | the kernel opens the artifact store again (the module edge returns) | **archcheck** — `cross-module kernel/runtime -> kernel/modules/artifacts/artifact` |
| 2 | the run engine's API names the concrete index again | **archcheck** — `cross-module kernel/runtime/runexec -> kernel/modules/artifacts/artifact` |
| 3 | the daemon stops opening the store | `TestOpenAppKernelWiresTheArtifactStore` |
| 4 | the daemon opens the store but forgets the **blob** port | `TestOpenAppKernelWiresTheArtifactStore` |
| 5 | the kernel stops installing the journal's ref guard | `TestArtifactCollect_KeepsTheBlobTheJournalNames` |
| 6 | the tool-facing port grows a host verb (`Collect`) | `TestTheToolPortCannotReachTheHostsVerbs` |
| 7 | the tool registry is handed a nil artifact port | `TestToolKernelDepsSuppliesTheArtifactPort` |
| 8 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |

**Three of the eight are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that opens
the store itself, as long as the store works.

### The run had to be repeated — twice

The first run reported **8 caught**, and that report was **wrong**: the
harness passed `go test <pkg> -run <name>` as **one** argument, so `go test`
read the whole string as a directory name and failed with *"directory not
found"*. Five of the eight were "caught" by a malformed command whose output
happened to contain the expected test name. A mutation caught by the wrong
reason is not a catch, so the run was discarded and repeated with the package
and the filter as separate arguments.

The second run was honest: **seven caught for the right reason, one build
error**. The build error was mutation 3 — removing the daemon's block left the
`artifact` import unused, so the package no longer compiled. A compile error
is not a catch either, so the mutation was fixed (it removes the import as
well) and the run was repeated a third time: **8 caught, 0 survived, 0
skipped, 0 build errors**.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 310 packages placed; 59 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 61** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rule the slice is really about

**A port's width is a grant, and the audience decides it.** The first attempt
widened one port to serve two audiences and was wrong: the plugin holds every
method of the port it is given, so the host's verbs became the plugin's
verbs, and one of them (`SetRefGuard`) would have let a plugin switch off the
journal's protection. Two ports — one for the tools, one for the host — cost
six lines of contract and kept the plugin's reach exactly where it was.

**A guard belongs to the thing it protects, not to the thing it guards.** The
ref guard answers from the journal, and the journal is the kernel's, so the
kernel installs the guard. Only the *port* moved, because the host has to
install a guard on an index it does not own.

**Moving a file out of the kernel makes every test that used it a host.** That
is the real cost of the slice, and it is a good one: a test that opens a
kernel now has to say where the artifacts come from, instead of inheriting a
store nobody declared.

## What remains

The kernel still reaches **17** modules. The next candidates, in the order
the measurement suggests:

- **the three media tool seams** (`voicetool`, `imagetool`, `reranktool`) —
  the voice and image tools are bound to the artifact store *inside the
  kernel*; with the store now a port, the daemon can build the tools and
  inject the closure itself.
- **`toolforge`** — the rows are aliases, but the kernel calls **eight** store
  methods that the contract's two-verb port does not cover.
- **`workflow` and `workboard`** — both need wide new ports: the kernel names
  `workflow.Engine`, `workflow.Ports`, `workflow.Provenance` and
  `workflow.Trigger` (no contract home), and it calls **seventeen**
  workboard store methods.
