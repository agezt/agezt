# W3.39b — the reasoning runners become the contract's ports

**Date:** 2026-10-24
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the tool registry's **reasoning row** — the one dependency the
council, conductor and research tools drive.

## What was in the way

The registry's `KernelDeps` carries the **live kernel** (`K *runtime.Kernel`)
because eight of the tools need something only the kernel has. Three of them
needed it for exactly one reason: their **runner**.

```go
// plugins/tools/council/council.go — before
type Runner interface {
	Council(ctx context.Context, corr, question string, members []reasoningapi.CouncilMember, rounds int) (reasoningapi.CouncilResult, error)
}
```

The interface was already the right shape — a port, one verb, and (since
W3.13) naming only the contract's rows. But it lived in the **tool's**
package, and the tool's package is a plugin (L6). So the registry, which is a
module (L3), could not hand the runner over: a module may not name a
plugin's type, and the registry's row had to be the **live kernel** instead —
the one type both sides could name.

That is the whole leak: a runner the tool already declared as a port, carried
by the kernel because the port was in the wrong layer.

## What moved

The three runners moved to the **contract layer**, where a port belongs, and
the tools keep their names as **aliases** — so every caller and every fake in
every tool's test is unchanged:

- **New** `kernel/contract/reasoningapi/runner.go`: `CouncilRunner`,
  `ConductorRunner`, `ResearchRunner` — one verb each — and `Runner`, the
  three together, which is the row the registry carries.
- `plugins/tools/council`: `type Runner = reasoningapi.CouncilRunner`
- `plugins/tools/conductor`: `type Runner = reasoningapi.ConductorRunner`
- `plugins/tools/research`: `type Runner = reasoningapi.ResearchRunner`
- `kernel/toolreg`: `KernelDeps` gains `Reasoning reasoningapi.Runner`
- `plugins/builtintools`: the three specs bind `d.Reasoning` instead of
  `d.K`
- `cmd/agezt`: the daemon supplies the port (`reasoningPort(k)`) with the
  same nil-port rule the other ports follow.

## The ratchet did NOT move — and that is the point of this slice

**307 packages, 78 import violations, 13 call sites** — unchanged.

This is the first slice in this run where the number does not go down, and
the reason is worth stating plainly: **the registry's rows are still in the
wrong layer.** The last `plugin-reach` edge (`plugins/builtintools ->
kernel/toolreg`) falls only when the registry's **rows** — the `Spec`, the
`Built`, the `BuildDeps`, the `KernelDeps`, the `LateDeps` and the registry
itself — live where a plugin may name them, and that needs **every** field
of every row to be a contract row or a contract port.

This slice removes the first of the five families that block it. The
remaining four are named in the evidence's last section, and each is the
same shape: a row that names a type a plugin may not name, replaced by a
port.

## The proof

### Permanent tests

- **The contract** (`kernel/contract/reasoningapi/runner_test.go`): each of
  the three ports has **exactly one** method (a fake in a tool's test must
  not have to implement a panel it never calls); the combined port carries
  exactly the three; the verbs' rows are the contract's own; every verb
  takes a **context** first.
- **The tools** — the ports are **aliases**, so the tool's runner and the
  contract's port are the *same type*. The identity is asserted in
  `plugins/builtintools/reasoning_ports_test.go` by comparing the **types
  themselves** (`reflect.TypeOf`), not by an assignment: an assignment would
  accept a copy with the same method set, while two interface types with
  identical method sets are still different types.
- **The plugin** (same file): all three specs bind the port, and the fake
  **records the call** — "the tool did not complain" is not enough, a tool
  could report unavailability for another reason and still look wired; and
  without the port the three tools say they are unavailable rather than
  calling into nothing.
- **The daemon** (`cmd/agezt/tooldeps_test.go`): the composition root
  supplies the port, it **is the live kernel's** reasoning surface, and it
  stays a **nil** port when there is no kernel.

### Mutation run

**7 mutations: 7 caught.** No mutation survived, none produced a compile
error, and none was skipped on a stale anchor (one was skipped in the first
round on a tab mismatch — the anchor, not the mutation, was wrong — and
re-written).

| # | Mutation | Result |
|---|---|---|
| M01 | the council spec does not bind the port | caught |
| M02 | the conductor spec does not bind the port | caught |
| M03 | the research spec does not bind the port | caught |
| M04 | the daemon does not supply the port | caught |
| M05 | the daemon's port stops being nil-safe | caught |
| M06 | the council tool declares its own runner copy | caught |
| M07 | the combined port loses a verb | caught |

There is **no gate-measured mutation in this slice**, and that is a fact
about the slice, not an omission: the registry's row for the live kernel is
a **same-module** edge (`kernel/toolreg` and `kernel/runtime` are both the
"runs" module), so archcheck cannot see it — there is no violation for it to
see. The slice's shape is pinned by the **compiler** instead (the kernel
must implement the port, or nothing builds) and by the shape test.

### Gates

The build, gofmt, vet, dead-code, archcheck (**307 packages, 78/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so is
the full test suite.

## What is deliberately left — the five families

The registry's rows still name types a plugin may not name. Each is the next
sub-slice, and the order below is by independence, not by size:

1. **the workflow and workboard rows** — their tools already declare narrow
   ports (`workflowtool.Kernel`, `workboardtool.Kernel`) over contract rows;
2. **the MCP row** — `mcptool.Kernel` already names only `mcpapi` rows;
3. **the forge row** — `forgetool.Kernel` names the toolforge rows, and
   `approval.Decision`, which is a platform type and would have to move;
4. **the schedule, standing, skill and journal rows** — the tools that read
   the daemon's schedules and standing orders;
5. **the platform rows** — the warden, the bus and the journal, which are
   platform types: the bus and the journal carry `event.Event`, which cannot
   move to the contract layer (it hashes with blake3), so those two need an
   adapter rather than an alias.

When the last of them is gone, the registry's rows are nameable by a plugin,
and the last `plugin-reach` edge falls with them.
