# W3.39d — the MCP and standing hosts become the contract's ports

**Date:** 2026-10-24
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the tool registry's **MCP row** and **standing row** — the two
dependencies the MCP tool and the standing-orders tool drive.

## The same leak, two more times

```go
// plugins/tools/mcptool/tool.go — before
type Kernel interface {
	AddMCPServer(corr string, srv mcpapi.Server) (mcpapi.Server, error)
	AttachMCPServer(ctx context.Context, corr, ref string) (mcpapi.Server, []string, error)
	DetachMCPServer(corr, ref string) error
	RemoveMCPServer(corr, ref string) (bool, error)
	MCPServers() []mcpapi.Server
	MCPServer(ref string) (mcpapi.Server, bool)
	MCPAttached() map[string]int
}

// plugins/tools/standingtool/standing.go — before
type Host interface {
	AddStanding(o standingapi.Order) (standingapi.Order, error)
	RemoveStanding(id string) (bool, error)
	StandingList() []standingapi.Order
}
```

Both were already ports naming only the contract's rows (`mcpapi.Server`,
`standingapi.Order`), and both lived in the **tool's** package — a plugin
(L6) — while the registry is a module (L3). A module may not name a plugin's
type, so the registry's row had to be the **live kernel**.

## What moved

- **New** `kernel/contract/mcpapi/runner.go`: `Runner` — seven verbs.
- **New** `kernel/contract/standingapi/host.go`: `Host` — three verbs.
- The two tools keep their names as **aliases**:
  - `mcptool.Kernel = mcpapi.Runner`
  - `standingtool.Host = standingapi.Host`
- `kernel/toolreg`: `KernelDeps` gains `MCP` and `Standing`.
- `plugins/builtintools`: the two specs bind the new rows instead of `d.K`.
- `cmd/agezt`: the daemon supplies both ports (`mcpPort(k)`,
  `standingPort(k)`) under the same nil-port rule.

### The roster lookup: a closure, on purpose

The standing-orders tool also checks an **agent profile** — it refuses a
managed sub-agent's direct self-wake. That lookup came from the roster
module:

```go
st.BindAgentLookup(d.K.Roster().Get)
```

The roster is a module, and a row a plugin names cannot live in a module's
implementation package. But the lookup is not a **type** — it is a function
value, and a function value names no type at all. So the registry carries it
as a plain closure:

```go
AgentLookup func(ref string) (agentapi.Profile, bool)
```

and the daemon builds it from the live roster. A daemon with no roster hands
over **nothing** — a closure over a nil roster would panic on the first
lookup, where a nil closure leaves the tool without the lookup it can do
without.

## The ratchet did NOT move — for the fourth time, and for the same reason

**307 packages, 78 import violations, 13 call sites** — unchanged.

The registry's **rows** are still in the wrong layer. The last `plugin-reach`
edge (`plugins/builtintools -> kernel/toolreg`) falls only when the rows
live where a plugin may name them, and that needs **every** field of every
row to be a contract row or a contract port.

This slice removes the **third** of the five families that block it.

## The proof

### Permanent tests

- **The contracts** (`kernel/contract/mcpapi/runner_test.go`,
  `kernel/contract/standingapi/host_test.go`): the MCP port has **exactly
  seven** methods and the standing port **exactly three**; every type each
  port names is the contract's row or a builtin, and each signature is
  pinned argument by argument and answer by answer.
- **The tools** — the ports are **aliases**, so the tool's type and the
  contract's port are the *same type*. The identity is asserted in
  `plugins/builtintools/mcp_standing_ports_test.go` by comparing the **types
  themselves** (`reflect.TypeOf`), not by an assignment: an assignment would
  accept a copy with the same method set, while two interface types with
  identical method sets are still different types.
- **The plugin** (same file): both specs bind the port, the fakes **record
  the call**, and without the port the two tools say they are unavailable
  rather than calling into nothing.
- **The daemon** (`cmd/agezt/tooldeps_test.go`): the composition root
  supplies both ports and the lookup, they **are the live kernel's**, the
  lookup **answers the live roster**, and they stay **nil** when there is no
  kernel.

### The tests that had to change, and why

Two existing tests (`mcp_ports_test.go`, `trigger_ports_test.go`) had to be
re-pointed at the new rows: they had been reaching the live kernel through
`d.K` to get the runner as well. The runner now arrives through its own port,
so the tests supply it there — what they measure and what they assert are
unchanged.

### Mutation run

**10 mutations: 10 caught.** No mutation survived, none produced a compile
error after the re-authoring below, and none was skipped on a stale anchor.

| # | Mutation | Result |
|---|---|---|
| M01 | the MCP spec does not bind the port | caught |
| M02 | the standing spec does not bind the port | caught |
| M03 | the standing spec drops the agent lookup | caught |
| M04 | the daemon does not supply the MCP port | caught |
| M05 | the daemon does not supply the standing port | caught |
| M06 | the daemon does not supply the agent lookup | caught |
| M07 | the MCP port stops being nil-safe | caught |
| M08 | the MCP tool declares its own runner copy | caught |
| M09 | the standing tool declares its own host copy | caught |
| M10 | the MCP port loses a verb | caught |

M08 and M09 were **re-authored** after the first round: each declared only
part of the port, which produced a *compile error* rather than a measured
failure — the tools call the other verbs. The re-written mutations declare
the whole port, and the identity assertions caught them.

There is **no gate-measured mutation in this slice**, for the same reason as
W3.39b and W3.39c: the registry's row for the live kernel is a **same-module**
edge, so archcheck cannot see it — there is no violation for it to see. The
shape is pinned by the **compiler** (the kernel must implement the ports, or
nothing builds) and by the shape tests.

### Gates

The build, gofmt, vet, dead-code, archcheck (**307 packages, 78/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so is
the full test suite.

## What is deliberately left — two families and the final move

1. **the schedule and skill rows** — the schedule tool already takes the
   cadence store through the contract's port (`cadenceapi.Store`), but the
   spec reaches the store and the roster through the live kernel, so the
   registry needs a store row and a lookup row;
2. **the forge row and the platform rows** — `forgetool.Kernel` names the
   toolforge rows plus `approval.Decision` (a platform type), and the warden,
   the bus and the journal are platform types themselves; the bus and the
   journal carry `event.Event`, which cannot move to the contract layer (it
   hashes with blake3), so those two need an **adapter** rather than an
   alias.

When the last of them is gone, every field of every row is a contract row or
a contract port — and the registry's rows can move to the contract layer,
which is where the last `plugin-reach` edge falls with them.
