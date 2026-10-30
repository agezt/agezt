# W3.71 — the MCP registry's lifecycle and the agent bridge move into the extensions module

## What the slice is

The **fourth** slice of the runtime's dissolution, and the first one to move a
family whose **data** was already in a module while its **operations** were in
the daemon.

The MCP registry was split in half. Its **data** — the durable registrations,
the store that persists them, the client that dials a server — already lived in
the extensions module (`kernel/modules/extensions/mcp`, the store and the
stdio/HTTP clients). Its **operations** lived in the daemon
(`kernel/runtime/mcptool.go` and `kernel/runtime/mcptool_bridge.go`, 496
lines): the seven lifecycle verbs that register, enable, attach, detach and
remove a server, the live-connection map, the dial rule, the "already
attached" and "not attached" refusals, the five journaled transitions — and
the **bridge** that folds an attached server's tools into a run's tool map.

A module that keeps the registry and leaves the lifecycle in the daemon keeps
the data and gives away the meaning of the data. The target architecture says
so in one line: the extensions module owns the **MCP registry**.

## What the slice found

The measurement that named the candidate:

- the daemon's MCP files: **496 lines** across two files;
- the daemon's MCP verbs: **14 public names** (`MCPStore`, `MCPRegistrations`,
  `MCPRegistrationCount`, `MCPRegistration`, `MCPReader`, `MCPServers`,
  `MCPServer`, `AddMCPServer`, `SetMCPServerEnabled`, `AttachMCPServer`,
  `DetachMCPServer`, `RemoveMCPServer`, `AttachEnabledMCPServers`,
  `MCPAttached`);
- the daemon's calls into the registry's store: **9**;
- the module's calls into the registry's store: **0**.

The **shape** of the candidate decided the slice's design, and this time the
shape cut BOTH ways:

- the **lifecycle** and the **bridge** answer ONE question — which tools may
  this run call — and they share the live attachments that answer it. A second
  service would be a second owner of the same connections. So they moved
  **together**, as one service.
- the **reads** (`MCPStore`, `MCPRegistrations`, `MCPRegistrationCount`,
  `MCPRegistration`, `MCPReader`, `MCPServers`, `MCPServer`) are the daemon's
  nil-safe view over the **contract's port** — the shape W3.58 established
  ("the kernel holds the port and may not name the module"). They are thin
  enough to stay, and they stayed: the daemon still holds the port.

That is the slice's finding: **a family splits along the line between the
port and the operations.** The port stays with the host (the host is what
opens the file); the operations go with the module (the module is what owns
the registry).

## What changed

### The module owns the lifecycle and the bridge

`kernel/modules/extensions/mcp/registry.go` is the service: the registrations,
the live attachments, the dial, the refusals and the five journaled
transitions.

`kernel/modules/extensions/mcp/bridge.go` is the bridge: the merge, the
bridged tool, the lazy dispatcher and the callable-name rule.

### The module grows a face

```go
// kernel/modules/extensions/api/mcp.go
type MCPRegistryService interface {
	Add(corr string, srv mcpapi.Server) (mcpapi.Server, error)
	SetEnabled(corr, ref string, enabled bool) (mcpapi.Server, error)
	Attach(ctx context.Context, corr, ref string) (mcpapi.Server, []string, error)
	Detach(corr, ref string) error
	Remove(corr, ref string) (bool, error)
	AttachEnabled(ctx context.Context) (attached []string, failures map[string]error)
	Attached() map[string]int
	MergeTools(tools map[string]toolapi.Tool) map[string]toolapi.Tool
	Close()
}

type MCPRegistryDependencies struct {
	Registry   mcpapi.Registry
	Dialer     mcpapi.Dialer
	HTTPDialer mcpapi.HTTPDialer
	Publish    func(spec event.Spec)
}
```

The module's **api package** is new — the extensions module had no `api`
package until now. The ports are the host's: the registry's store (the
contract's port), the two dialers (the seams W3.58 established) and the bus to
publish on. Every port is a way to **ASK**; none is a way to reach.

### The daemon keeps its names and forwards

```go
// kernel/runtime/mcptool.go
func (k *Kernel) AddMCPServer(corr string, srv mcpapi.Server) (mcpapi.Server, error) {
	svc, err := k.mcpOrRefusal()
	if err != nil {
		return mcpapi.Server{}, err
	}
	return svc.Add(corr, srv)
}
```

**No caller outside the runtime changed**: **0 production files** in
`kernel/app` changed, and the plugin (`plugins/tools/mcptool`), the app's
lifecycle surface (`kernel/app/tools/mcp_lifecycle.go`), the marketplace
(`kernel/market`) and the control plane all keep speaking the daemon's
fourteen names.

### The wiring is a builder, not a setter

The daemon hands the kernel the module's **BUILDER**
(`cfg.MCPBuilder`), and the kernel calls it with the store, the two dialers
and its own publisher (`kernel/runtime/compose.go`). This is the W3.57
(memory) shape, and it is the right one here: the MCP store and the dialers
were already **cfg seams** (W3.58), so the service is built where the other
ports are collected — at `Open` — instead of in a second step after it.

A host that was given neither a store nor a builder holds **no service**: the
writes refuse and the reads answer "no servers".

### What the slice cost

**Six test setups** had to learn the builder: the runtime's own MCP test
helper, one runtime test that hands over a fake registry, the control plane's
two MCP test helpers, the built-in tools' MCP test helper and the application
layer's MCP ports test. The **daemon itself cost nothing** — `cmd/agezt` wires
its own builder by default, the way it wires the memory's.

That is the price of **inventing a new face**, and the slice states the rule
the prior three slices measured: a move into an **existing** face costs
**nothing** (W3.70: 0 sites), and a move that invents a **new** face costs a
wiring in every host that has one (W3.68: 11, W3.69: 6, W3.71: 6).

## The rule

The daemon holds the module's **face**; the module holds the module's
**body**.

The rule is held by the rule's own observer: a fake registry records what the
daemon asks. A daemon that grew the lifecycle back would answer on its own
and the fake would record nothing.

## What did NOT move, and why

- **The registry's READS stayed in the daemon.** They are the nil-safe view
  over the **contract's port** — the shape W3.58 established — and the host is
  what opens the registry's file. The daemon still holds `mcpapi.Registry` and
  still answers "no servers" when it holds none.
- **The dialers stayed the host's seams.** The implementations live in the
  module, but the host injects them (`mcpapi.Dialer`, `mcpapi.HTTPDialer`) —
  the module never names its own clients, and a host with no dialer refuses
  the attach with a readable error instead of calling a nil func value.
- **The event's shape did not change.** The five kinds (`mcp.added`,
  `mcp.updated`, `mcp.attached`, `mcp.detached`, `mcp.removed`), the subject
  (`mcp.<name>`), the actor (`mcp`) and the payload rows are the same ones the
  daemon published — only the place that speaks them moved.

## The contract row

The refusal became a **contract row** (`mcpapi.ErrNoRegistry`), because two
sides answer it and a caller compares it with `errors.Is`: the **module** raises
it when the daemon handed over no registry port, and the **daemon** raises it
when the module was never wired. Two look-alike errors of their own would
leave every comparison silently false — the same reason `okrapi.ErrNoLibrary`
exists (W3.69).

The daemon keeps the name (`runtime.ErrNoMCPRegistry`) as the row's alias, so
every existing comparison keeps working.

## The tests

**26 permanent tests**, in four places:

- `kernel/modules/extensions/mcp/registry_test.go` — **14 tests**: the service
  refuses to write without a store (the contract's row), it registers the
  server and journals `mcp.added` with the transport, a remote registration is
  journalled as http, it flips the flag and journals `mcp.updated`, it dials,
  discovers and journals the tools, it refuses an unknown server **without
  dialing**, it refuses a double attach **without dialing again**, a remote row
  dials over HTTP and a local one over stdio, the attach refuses without a
  dialer, the detach is the kill switch (the connection is closed and the
  transition is journalled), the remove detaches a live server first, the boot
  path skips the disabled and **reports** the failures instead of failing, the
  attached surface counts the tools, and the close detaches everything.
- `kernel/modules/extensions/mcp/bridge_test.go` — **8 tests**: the merge
  leaves the map untouched without attachments, it exposes the attached tools
  and forwards the call, it honours the allowlist, it collapses a lazy server
  into one dispatcher, the dispatcher refuses an unexposed tool, a registered
  tool wins a collision, the callable name is sanitized and capped at 64, and
  a failing call is an error **result** while a cancelled run stays cancelled.
- `kernel/runtime/mcp_module_test.go` — **3 tests**: the daemon refuses every
  registry verb when the module is not wired, the daemon forwards every
  registry verb to the module (a fake records the arguments and the answer
  comes from the module), and the daemon keeps the fourteen names.
- `kernel/runtime/mcp_merge_internal_test.go` — **1 test**: the daemon's tool
  merge is a forwarder — with no module the map comes back untouched, and
  with one the module's tool is in the merged map.

## The mutations

**13 mutations, 13 caught**, in two independent passes with **byte-identical**
logs.

SHA-256: `5BBC8B2B662AC26714D0289B241FBE7053BBB065539DA0906342AF70BA386B07`

The first calibration reported **one BUILDERR** and **one** mutation that
survived — and **both were the harness's and the test's** faults, not the
code's:

- the mutation that stopped the daemon asking the module left `svc` unused, so
  Go refused to compile — a compile error is not a caught mutation, it is a
  mutation that never ran. The mutation now keeps `svc` alive (`_ = svc`);
- the mutation that let a double attach through **survived**, and it was
  **correct**: the second guard (the one after the dial) still refuses the
  attach, so the behaviour was equivalent and the test could not see it. But
  the test was **wrong**, not the mutation: the dial **spawns a child
  process**, and a refused attach must not spawn one. The test now counts the
  dials and pins that a refused double attach does not dial again — and the
  mutation is caught.

- **M01**, **M02** the daemon stops asking the module, or swallows the
  refusal — the **fake registry** and the **refusal** catch them.
- **M03** the module accepts a write with no registry — the module's own
  refusal catches it.
- **M04**, **M05**, **M06** the module journals the wrong transport, journals
  the flip as an add, or does not journal the discovered tools — the
  **journaling** catches them.
- **M07**, **M08**, **M09** the module allows a double attach, does not close
  the detached connection, or does not detach before removing — the **double
  attach**, the **kill switch** and the **remove** catch them.
- **M10**, **M11** the module attaches the disabled servers too, or swallows
  the attach failures — the **boot path** catches them.
- **M12**, **M13** the bridge ignores the allowlist, or does not collapse a
  lazy server — the **allowlist** and the **lazy server** catch them.

## The measurement that moved

| | before | after |
|---|---|---|
| the daemon's calls into the registry's store | **9** | **3** (the reads) |
| the module's calls into the registry's store | **0** | **7** |
| the daemon's calls into the module | **0** | **9** |
| the daemon's public MCP names | **14** | **14** |
| the kernel's verbs | **240** | **241** |
| the application layer's production files that changed | — | **0** |
| the test setups that had to be wired | — | **6** |
| the contract's new rows | — | **1** |
| `kernel/runtime` (lines) | **9,395** | **9,170** |
| `kernel/modules/extensions` (lines) | **4,621** | **5,234** |
| `kernel/modules/extensions/mcp` (lines) | **821** | **1,323** |
| `kernel/contract/mcpapi` (lines) | **281** | **289** |
| the daemon's `mcptool.go` (lines) | **269** | **189** |
| the daemon's `mcptool_bridge.go` (lines) | **227** | **23** |
| the module's `registry.go` (lines) | — | **257** |
| the module's `bridge.go` (lines) | — | **245** |
| the module's `api/mcp.go` (lines) | — | **111** |

The kernel's verb count moved by exactly **one**: `SetMCPRegistry`, the seam
that lets a host swap the module's service. The **fourteen MCP names did not
move at all** — they are the daemon's names and always were; what moved is the
body behind them.

The daemon's calls into the registry's store fell from **9** to **3**: the
seven lifecycle calls became the module's, and the **three reads stayed** —
they are the daemon's nil-safe view over the contract's port, which is where
W3.58 put them.

The lines are counted the way the previous slices counted them: **non-blank**
lines of the package's production files (`git ls-files`, upper level for the
runtime, recursive for the module). The daemon shrank by **225** lines and the
extensions module grew by **613**.

### The daemon's own lock went with the attachments

The daemon's `mcpMu` guarded `mcpConns` — the live MCP attachments. With the
attachments the lock went too, and the daemon's **lock order** is now one
link shorter (`configMu < runsMu < fanoutMu < treeMu < steersMu < spawnsMu`).
The order was load-bearing in four places (the kernel struct, the run
executor's three files and the lifecycle's comment); all four were updated in
the same slice, because a lock order that names a lock that no longer exists
is a lie that ages badly.

## What is left

The extensions module now keeps the whole of the MCP registry's business: the
store, the clients, the lifecycle and the bridge.

What is left of the runtime's dissolution stands at **241 verbs** in
**9,170 lines**. The nearest families are the **MCP registry's fourteen** (now
forwarders), the **board's sixteen**, the **workflow host's twelve** and the
**reasoning module's six**.

The next candidates are the ones the target architecture names and the
dissolution map still holds: the **reaper** (→ `modules/health`, a new
module), the **epistemic gate** (→ `modules/knowledge`), and — last, because
it is the largest and the most entangled — the **run engine** (→
`modules/runs`).

The self-repair module's edge into the runtime is still in the archcheck
allowlist — it is the other program's work, and it waits there.
