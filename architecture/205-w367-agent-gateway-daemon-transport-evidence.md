# W3.67 — the agent gateway is the daemon's transport

## What the slice is

W3.66 left the import ratchet at **9/3**. This slice takes **two** import edges
at once and the ratchet goes **9 → 7**:

```
upward kernel/runtime       -> kernel/agentgw
upward kernel/runtime/runexec -> kernel/agentgw
```

The gateway is the **agent SDK's transport**: a capability-scoped HTTP surface
(`/v1/token/create`, `/v1/eventbus/*`, `/v1/memory/*`, `/v1/log/*`,
`/v1/agent/*`, `/v1/config/*`) that an agent subprocess talks to with a
bearer token. It is an **ADAPTER** (layer 5). The kernel is a **MODULE**
(layer 3), and a module may not hold an adapter — that is the whole point of
the layering: the upward edge was not an accident of imports, it was the
kernel serving an inbound transport it does not own.

So the fix could not be a port. It had to be a **move**, and the measurement
said so before a line was written: `k.AgentGateway()` had **no caller**
anywhere in the tree outside its own declaration — the accessor and the
`runexec.KernelAPI` verb that repeated it were **free riders**.

## What the edges were

`kernel/runtime` held the gateway in four places:

| Where | What it held |
|---|---|
| `runtime_kernel.go` | the `agentGW *agentgw.Gateway` field |
| `runtime_accessors.go` | the `AgentGateway()` accessor (**no caller**) |
| `compose.go` | the setup: config, token secret, `Attach`, `SetConfigCenter`, `SetAuditJournal`, `go Listen` |
| `compose_close.go` | the close — **after** the journal |
| `runexec/api.go` | `AgentGateway() *agentgw.Gateway` on the `KernelAPI` interface |

## What changed

### The daemon builds the gateway over the kernel's own ports

`cmd/agezt/main_agentgw.go` (new) is the composition root's side of the
gateway:

```go
gwCfg := agentgw.DefaultGatewayConfig(baseDir)
secret, err := agentgw.ResolveTokenSecret(baseDir)   // startup error
gwCfg.TokenSecret = secret
if sockPath := strings.TrimSpace(os.Getenv(agentgwSocketEnv)); sockPath != "" {
    gwCfg.SocketPath = sockPath
}
gw := agentgw.NewGateway(gwCfg)
gw.Attach(k.Bus(), k.Memory(), k.Roster())
gw.SetConfigCenter(k.ConfigCenter())
gw.SetAuditJournal(k.Journal())
if err := gw.Listen(context.Background()); err != nil { slog.Error(...) }
```

The daemon is the right owner: it already opens the bus, the memory store, the
roster, the config center and the journal, and it already owns the process
lifecycle. The kernel's ports are read through the accessors that already
exist (`k.Bus()`, `k.Memory()`, `k.Roster()`, `k.ConfigCenter()`,
`k.Journal()`) — **no new port, no new field, no new seam**.

The token secret is a **startup error** if it cannot be resolved: a daemon
that signs with a key the `agt` CLI does not have would fail every SDK call
with a 401 that reads like a bug. The **listener** stays best-effort and is
logged, not fatal — that is the behaviour the kernel had, and a convenience
transport must not become a boot dependency.

### One gateway per daemon, not per kernel

The kernel built a gateway **per kernel**: the primary and every tenant that
booted. Each one bound a socket path from `uniqueSocketPath()` — a random
name — and **nothing published that path**: it was not written to a file, not
exported to a child process, and the SDK's default constructor does not match
it (see 12's reachability gap). With `AGEZT_AGENTGW_SOCKET` set, every kernel
in the process tried to bind the **same** path, so at most one could win and
the losers were only logged. Now there is **one** gateway per daemon, built
over the primary kernel's ports.

### The close order became a guarantee

The gateway's audit trail is **buffered** and flushed on `Close`, and the
flush appends to the kernel's journal. Inside the kernel the journal closed
**first** (`closeAll(state, worldDir, skillStore, journal, agentGW)`), so the
flush landed on a closed journal and was lost. The daemon now closes the
gateway **first**:

```go
func shutdownAgentGateway(gw *agentgw.Gateway, k *kernelruntime.Kernel) {
    if gw != nil { _ = gw.Close() }   // the audit flush needs an open journal
    if k != nil  { _ = k.Close() }
}
```

The order is written in **one place** instead of being left to the order two
`defer`s happen to be registered in — and `main.go` registers a single
deferred call, not two.

### The kernel's accessor is gone, and its absence is pinned

`agentGW`, `AgentGateway()` and the `KernelAPI` verb are deleted. The
accessor is pinned by **name** in `kernel/runtime/submanager_test.go`, like
`ConfigMu`'s: an accessor that comes back comes back with the upward edge
this slice closed.

## The proof found two real defects

The daemon-level proof was written **first**, before the slice was called
done, and it failed on the first run. Both failures were real.

### Defect 1 — the audit record was dropped on the floor

The audit entry's actor was the token's **parent** id:

```go
g.auditLog.Log(AuditEntry{ TokenID: claims.ParentTokenID, ... })
```

A **subprocess** token inherits its parent's id, but a **top-level** token —
the only kind `agt token create` mints, and the only kind the SDK path uses
— has **no parent**. The journal rejects an event without an actor
(`event: actor required`), so **every access made with a CLI-minted token was
recorded nowhere**. The trail that answers "who asked for what" was
empty, and the kernel's own comment (`SetAuditJournal(j) // wire the audit
trail`) was a claim with nothing behind it.

A top-level token is now charged to itself (`auditActor`), and the empty case
is a **contract**, not an accident: an empty actor does not mean "an
anonymous access", it means the access is **not recorded at all**.

**What this slice does NOT fix**, named rather than claimed away: the audit
entry's `cap` **tag is still empty**. The access is recorded before the
handler runs (`withAuth` audits, then calls the handler), and the handler is
what checks the capability — so at audit time the required capability is not
known yet. Closing that means the **route table** has to carry the required
capability (`withAuth(gw, CapMemoryWrite, handler)`), which is a change to
the gateway's auth architecture, not to a move. The trail records **who**
(the actor), **what** (operation and path), **when**, and the run — not which
capability was spent.

### Defect 2 — the memory's type was refused for existing

The memory's type vocabulary is upper-case (`FACT`, `SUMMARY`) and the
contract validates it strictly, so a client that wrote `"fact"` — the
spelling the type's **name** uses — was refused with a `500 WRITE_ERROR`
(`memory: invalid type "fact"`) for a type that **exists**. The gateway is
the transport: it canonicalises the spelling it accepts
(`memoryRecordType`).

## The gate's finding: the gateway's own dead code

Moving the gateway to the daemon made the **dead-code gate** report five
symbols in `kernel/agentgw` that it had never reported before:

```
AuditLogger.LogSync, CapabilityChecker.CheckAny, CapabilityChecker.CheckAll,
CapsIntersect, TokenManager.CreateSubprocessToken
```

The measurement is unambiguous: **none of them has a production caller** — not
the daemon, not the `agt` CLI, not the SDK. Their only consumers are the
package's own tests. The analyzer runs **without `-test`** (that is the
gate's strictness), so they were unreachable in the binaries.

The gate's own rule says what to do with such a symbol — not an allowlist
entry, which would weaken the gate, but a **move into a `_test.go` in its own
package**: "a same-package test-only helper does NOT belong here: move it
into a _test.go in its own package instead, which states what it is and keeps
the gate strict." So they moved to `kernel/agentgw/testonly_test.go`, with
their tests, and the package's file table in 03 says exactly what they are.

The move is **behaviour-neutral** and it keeps the **comparison**: the HTTP
mint path implements the subprocess-token rule itself, and
`parent_token_id_test.go` still compares it against the library's copy
(which is stricter in one place — it halves the burst, where the HTTP path
clamps it — and laxer in another: it drops an escalated capability, where the
HTTP path refuses it with 403). Both facts are now written down where the
copy lives, instead of one of them being a comment that claimed the two paths
were identical when they were not.

## The proof

### Permanent tests

Nine permanent tests: five **daemon-level** end-to-end ones over the wiring,
four **unit** ones over the two defects.

| Test | What it holds |
|---|---|
| `TestTheDaemonWiresTheAgentGatewayAndItListens` | the daemon's gateway really **listens** and answers `/health` — a listener that was never started answers nothing at all |
| `TestTheDaemonGatewayAcceptsTheInstallSecretTheCLIMints` | a token minted from the **install secret** (the same file the `agt` CLI mints from) is accepted, and a **memory round-trip** works through the gateway — the secret is shared, the memory is attached, the listener is live |
| `TestTheDaemonGatewayPublishesOnTheDaemonsBus` | an event published through the gateway lands in the daemon's **journal** — the bus is attached |
| `TestTheDaemonGatewayServesTheConfigCenterAndTheRoster` | the config-center and roster routes **answer** instead of 404/503 — the routes exist only when the gateway was given them |
| `TestTheDaemonClosesTheGatewayBeforeTheKernel` | after the daemon's shutdown the audit trail is **on disk** — the flush happened while the journal was still open, so the order is right |
| `TestTheAuditRecordIsChargedToTheTokenThatMadeTheRequest` | a subprocess token is charged to its **parent**, a top-level token to **itself**, and the actor is **never empty** |
| `TestTheAuditTrailReachesTheJournalForATopLevelToken` | the trail **lands** for a top-level token — the defect that made it vanish |
| `TestTheMemoryTypeIsCanonicalised` | `"fact"`, `"FACT"`, `"Summary"`, `"  observation  "`, `""` all land on the contract's vocabulary |
| `TestTheGatewayRecordsTheAccessItAuthorised` | the record carries its **run** and its **payload** — the operation and the path an operator reads |

### The mutation run

| # | Mutation | Caught by |
|---|---|---|
| 1 | the daemon reads a different env var, so the **socket override** is ignored | `TestTheDaemonWiresTheAgentGatewayAndItListens` |
| 2 | the daemon signs with a key that is not the **install secret** | `TestTheDaemonGatewayAcceptsTheInstallSecretTheCLIMints` |
| 3 | the daemon attaches **no memory** to the gateway | `TestTheDaemonGatewayAcceptsTheInstallSecretTheCLIMints` |
| 4 | the daemon attaches **no bus** to the gateway | `TestTheDaemonGatewayPublishesOnTheDaemonsBus` |
| 5 | the daemon attaches **no roster** to the gateway | `TestTheDaemonGatewayServesTheConfigCenterAndTheRoster` |
| 6 | the daemon gives the gateway **no config center** | `TestTheDaemonGatewayServesTheConfigCenterAndTheRoster` |
| 7 | the daemon gives the gateway **no journal** | `TestTheDaemonClosesTheGatewayBeforeTheKernel` |
| 8 | the daemon **never starts the listener** | `TestTheDaemonWiresTheAgentGatewayAndItListens` |
| 9 | the daemon closes the **kernel before the gateway** | `TestTheDaemonClosesTheGatewayBeforeTheKernel` |
| 10 | the audit actor falls back to the **empty parent** again | `TestTheAuditRecordIsChargedToTheTokenThatMadeTheRequest` |
| 11 | the gateway stops **canonicalising the memory type** | `TestTheMemoryTypeIsCanonicalised` |
| 12 | the kernel grows an `AgentGateway` **accessor** again | `TestTheKernelStillAnswersItsAccessorSurface` |

**12 mutations, 12 caught**, in two independent passes with **byte-identical**
logs. Nine of the twelve aim at the **wiring** — the socket override, the
install secret, the memory, the bus, the roster, the config center, the audit
journal, the listener and the close order — and only the daemon's end-to-end
test can see them. That is the point of the recipe from W3.57 and W3.66:
**a module test cannot see the wiring**.

Every mutation keeps its file's imports used, so a mutation that broke the
build would have been a BUILDERR and never counted as a catch. The harness
compares the mutated file with the original with **`-ceq`** — `-eq` is
case-insensitive, and a mutation that only changes the case of a letter would
look like no change at all (W3.65's lesson).

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 314 packages placed; 7 allowlisted import violations and 3 allowlisted call sites remain (ratchet)** — **import edges down from 9** |
| `go run ./tools/deadcodecheck` | OK — 31 public SDK findings and 1 cross-package test seams allowlisted (the same numbers as before the slice) |
| `go run ./tools/depscheck` | OK — 24 core dependencies, all justified |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## What the ratchet says now

**7 import edges / 3 call sites**, down from 9/3:

| Rule | Edges left |
|---|---|
| `adapter-bypass` | `controlplane → modules/extensions/toolbox`, `controlplane → runtime`, `controlplane → update`, `restapi → update` |
| `cross-module` | `cadence/systemtasks → runtime`, `modules/agents/selfrepair → runtime` |
| `upward` | `channel → bus` |

## What remains

The traps are unchanged, and they are the map for the next slices:

- **`controlplane → runtime`** — the control plane calls **37** verbs on the
  kernel's god-object. A redesign, not a slice.
- **`selfrepair → runtime`** — the six agent rows could move to the reaper's
  `api` package, but `RunWith`/`RunWithRetry`/`ReaperScan`/`NewCorrelation`
  need a **runs port that does not exist** yet, so that port is a slice of its
  own.
- **`channel → bus`** — 26 plugin `Config`s hold `*bus.Bus` and nil-check it.
  An interface parameter turns a nil `*bus.Bus` into a **non-nil** interface,
  the guard stops firing, and the publish panics inside the deferred recover.

The **forbidden-call** ratchet is still **3**, with the same two holes the
measurement found: the staged writes are not **fsynced** (so a swapped-in
tree can hold a half-written file), and the swap **removes the old copy
before** the new one is in place (so a failure there loses the content, and a
partial removal leaves a half-deleted tree).

## The rules the slice is really about

**A "free rider" edge is not a port's problem, it is a move's.** Two edges
that hang off an accessor with no caller cannot be closed by a port: there is
nothing to port. The measurement said "move it", and the slice moved it.

**A module that holds an inbound transport serves something it does not own.**
The kernel held a gateway — a listener, a token manager, a rate limiter, an
audit trail — and none of it is the kernel's business. The composition root
is where a transport belongs, and the daemon is the composition root.

**A wiring that was never proven is a wiring that is off.** The gateway's
setup lived inside `runtime.Open`, where no test could reach it: nobody had
shown that it listened, that a token the `agt` CLI mints is accepted, or that
its audit trail ever reached the journal. The first daemon-level test proved
that **two** of those three were broken.

**An empty actor does not record an anonymous access — it records nothing.**
The journal rejects an event without an actor, so an audit trail whose actor
is empty for the common case is not a sparse trail. It is an **empty** one.

**A gate that was blind to a package is not a gate that was passed.** The
dead-code gate reported the gateway's five dead symbols only once the daemon
imported the package. They were dead before — the analyzer simply had not
been asked about them. The gate's own rule (a same-package test-only symbol
belongs in a `_test.go`, not in an allowlist) turned that finding into the
truth of the tree instead of a suppressed line.
