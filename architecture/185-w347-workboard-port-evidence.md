# W3.47 — the task board is the daemon's file

## What the slice is

The kernel still reached **16** modules after W3.46. This slice takes **two**
of those edges — and they are the same family:

| Edge | Why it existed |
|---|---|
| `cross-module kernel/runtime -> kernel/modules/work/workboard` | the kernel held the concrete `*workboard.Store` and called **seventeen** of its methods |
| `adapter-bypass kernel/controlplane -> kernel/modules/work/workboard` | the control plane named the module's rows |

The ratchet went **53 → 51** — this is the first slice in a while that took
**two** edges at once (like W3.43, which took the artifact store's two).

## What the kernel used to do

It held the module's concrete type and drove the whole lifecycle through it:

```go
// kernel/runtime/runtime_kernel.go
workboard *workboard.Store
// kernel/runtime/workboard.go
func (k *Kernel) Workboard() *workboard.Store { return k.workboard }
```

**Seventeen** verbs went through that pointer — `Create`, `Claim`,
`Heartbeat`, `Comment`, `Block`, `Fail`, `Unblock`, `SetRetryPolicy`,
`Complete`, `Review`, `Archive`, `Link`, `AddDependency`, `ReclaimStale`,
`SweepStaleClaims`, plus `Get` and `Prove`. And the kernel **opened the
file** itself:

```go
// kernel/runtime/compose.go
wbstore, err := workboard.OpenStore(filepath.Join(cfg.BaseDir, "workboard"))
```

## What it does now

### The port is the contract's — and it is the HOST's port

```go
// kernel/contract/workboardapi/hoststore.go
type HostStore interface {
    Get(id string) (Task, bool)
    List(f Filter) []Task
    Create(spec CreateSpec, now time.Time) (Task, bool, error)
    Claim(id, agent, runID string, now time.Time) (Task, error)
    Heartbeat(id, agent, runID string, now time.Time) (Task, error)
    Comment(id, author, body string, now time.Time) (Task, error)
    Block(id, actor, reason string, now time.Time) (Task, error)
    Fail(id, actor, reason string, now time.Time) (Task, RetryDecision, error)
    Unblock(id, actor string, now time.Time) (Task, error)
    SetRetryPolicy(id, actor string, policy *RetryPolicy, now time.Time) (Task, error)
    Complete(id, actor string, now time.Time) (Task, error)
    Prove(id, actor string, p proofapi.Proof, now time.Time) (Task, error)
    Review(id, actor, summary string, now time.Time) (Task, error)
    SetSeat(id, seat string, now time.Time) (Task, error)
    Archive(id, actor string, now time.Time) (Task, error)
    Link(id, typ, target string, now time.Time) (Task, error)
    AddDependency(id, dependsOn string, now time.Time) (Task, error)
    BlockingDependencies(id string) ([]DependencyState, error)
    ReclaimStale(id, actor string, staleAfter time.Duration, now time.Time) (Task, error)
    SweepStaleClaims(actor string, staleAfter time.Duration, limit int, now time.Time) ([]Task, error)
}
```

**Twenty** verbs — and the width is not a copy of the store's method list. It
is the **host's** width, and the read port stays where it was:

- the **plugin** (the agent-facing `workboard` tool) keeps the **read side
  only** (`Store`, two verbs: `Get`, `List`) — a plugin that could claim,
  complete or archive a task would be driving another agent's work item;
- the **host** drives the whole lifecycle, so the host's port carries all
  twenty.

That is the same shape W3.45 gave the script-tool store (`Store` for the
plugin, `HostStore` for the host) — and the same rule: **a port's width is
decided by the weakest holder of it.**

The module answers the port with a compile-time assertion, so a port change
breaks in the module that implements it:

```go
// kernel/modules/work/workboard/aliases.go
var _ workboardapi.HostStore = (*Store)(nil)
```

### Two rules moved to the contract with the port

The port carries rows the kernel and the control plane named through the
module, so two of them had to move:

- **`ErrNotFound`** — the "no such task" answer. The host is the one who
  reports it, and the host may not name a module's error. The module keeps
  the name as an alias.
- **the retry rule** (`RetryDecisionFor`, `NormalizeRetryPolicy`) — it is
  pure, and it reads the task, so it travels with the task. The module
  delegates to it, so there is one rule: a second copy would drift, and the
  drift would show up as one task retrying forever next to another that
  gives up after a single failure.

### The store file is the daemon's

```go
// cmd/agezt/main_toolinvoker.go
if cfg.Workboard == nil {
    store, err := workboard.OpenStore(filepath.Join(cfg.BaseDir, "workboard"))
    if err != nil {
        return nil, err
    }
    cfg.Workboard = store
}
```

A store that cannot be opened is a **startup error** — the board is part of
the daemon, not a convenience.

### A missing board is an answer, not a panic — in TWO layers

The board used to be opened by the kernel, so the field was **never nil** and
the code never had to say what happens without one. Now a host may open a
kernel without a board, so two layers had to learn the rule:

- the **kernel**: every board operation goes through `workboardStore()`, which
  refuses with *"the task board is not available on this daemon"*;
- the **app layer** (`kernel/app/workboard`): the service, the watch, the
  dispatch and the execution all hold the store, and each of them refuses the
  same way instead of dereferencing nil.

The refusal is the same answer everywhere — one sentence, four surfaces.

One detail worth naming: `ProveTask` used to answer **"not found"** when the
board was missing, because the missing board and the missing task looked the
same from the inside. Now they are different: no board → *"not available"*;
a board without that task → *`workboard: not found`*.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/workboardapi`: `TestTheHostPortCarriesTheTwentyVerbs` | the host port is **exactly** twenty verbs wide |
| `kernel/contract/workboardapi`: `TestTheHostPortIsWiderThanTheReadPort` | the host port is a **superset** of the read port — the two shapes cannot drift apart |
| `kernel/contract/workboardapi`: `TestTheRetryRuleIsTheContracts` | the retry rule retries, escalates and blocks **as written** |
| `kernel/modules/work/workboard`: `TestTheStoreIsTheContractsHostPort` | the module answers the host port |
| `kernel/modules/work/workboard`: `TestTheNotFoundErrorIsTheContracts` | the module's not-found error **is** the contract's |
| `kernel/modules/work/workboard`: `TestTheRetryDecisionDelegatesToTheContract` | the delegation agrees with the contract on **every** task and reason |
| `kernel/runtime`: `TestTheBoardRefusesWithoutAStore` | **all sixteen** kernel board operations refuse instead of panicking |
| `kernel/runtime`: `TestTheKernelDrivesTheBoardThroughThePort` | the kernel drives the board through the port, and the port reaches the **live** store |
| `kernel/app/workboard`: `TestTheServiceRefusesWithoutABoard` | the service, the watch, the dispatch and the execution refuse instead of panicking |
| `cmd/agezt`: `TestOpenAppKernelWiresTheTaskBoard` | the daemon end: the port is live and the board's file is **on disk, under the daemon's base directory** |

### The mutation run

**8 mutations: 8 caught, 0 survived, 0 skipped, 0 build errors.**

| # | Mutation | Caught by |
|---|---|---|
| 1 | the kernel's field names the module's store again | **archcheck** — `cross-module kernel/runtime -> kernel/modules/work/workboard` |
| 2 | the kernel opens the board again | **archcheck** — the same edge |
| 3 | the control plane names the module's rows again | **archcheck** — `adapter-bypass kernel/controlplane -> kernel/modules/work/workboard` |
| 4 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |
| 5 | the daemon opens the board without injecting it | `TestOpenAppKernelWiresTheTaskBoard` |
| 6 | the kernel stops refusing without a board | `TestTheBoardRefusesWithoutAStore` |
| 7 | the app layer stops refusing without a board | `TestTheServiceRefusesWithoutABoard` |
| 8 | the host port loses a lifecycle verb | `TestTheHostPortCarriesTheTwentyVerbs` |

**Four of the eight are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that opens
the board itself, as long as the board works.

The run needed **two** passes, and the first pass's third mutation was
**caught for the wrong reason**: the harness replaces whole LINES, and the
mutation had replaced the function's signature line with a bare `workboard.Task`
— so archcheck failed with a **parse error**, not with the edge. A parse error
is not a catch of anything; the mutation now replaces the whole signature line
with the module's spelling, and the edge is what the gate reports.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 311 packages placed; 51 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 53** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rule the slice is really about

**A file that a host owns cannot be opened by the thing that uses it.** The
board was the kernel's file because the kernel used it; the moment the board
became a **port**, the file had to become the **host's** — and the kernel had
to learn to live without one. That is the same arc as the seats (W3.41), the
lake (W3.42), the artifacts (W3.43) and the forge (W3.45).

**The rule that moves with a port is the rule that reads the row.** The retry
rule was in the module because it read the module's row. The row moved to the
contract — and the rule had to move with it, or the module would have kept a
copy and the two would drift.

**A missing port is a refusal, and every holder of it must refuse.** The
kernel learned it in W3.45 (the forge), the app layer learned it here, and
the board needed **both** — because the board reaches the kernel *and* the
app. A refusal that only one layer knows is a panic waiting for the next
caller.

## What remains

The kernel still reaches **14** modules. The next candidates, in the order the
measurement suggests:

- **`workflow`** — the widest remaining edge: the kernel holds the store AND
  builds the engine (`workflow.NewEngine(k.workflows, k.workflowPorts())`),
  and it names `workflow.Engine`, `workflow.Ports`, `workflow.Provenance` and
  `workflow.Trigger` — none of which has a contract home. The engine is the
  hard part: its ports are built from the kernel's own state, so the
  composition root cannot build it before the kernel exists.
- **`cadence`, `standing`, `mcp`** — many callers each; the shape is known (a
  contract port the host opens), the work is in the port's width.
- **`memory`, `worldmodel`, `roster`, `skill`, `okr`, `taste`, `catalog`** — the
  kernel holds each module's concrete store. Each one is the same slice: a
  contract port, the daemon opening the file, and the kernel's accessor
  carrying the port.
