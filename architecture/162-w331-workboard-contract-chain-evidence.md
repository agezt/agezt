# W3.31 — the workboard chain moves to the contract layer

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the **workboard chain** — the task row and everything that hangs
off it. A task carries its claims, its dependencies, its attempts, its
comments, its links, its retry policy and its **acceptance criteria**; the
criteria carry a **proof**, and the proof carries the **verdict** the
completion judge reached. The `workboard` tool that drives all of it is a
plugin, and a plugin may reach the contract and the platform layers and
nothing else.

## What moved

| Before | After |
|---|---|
| `kernel/modules/work/workboard` named `Status`, `Task`, `Claim`, `Dependency`, `DependencyState`, `Attempt`, `Comment`, `Link`, `RetryPolicy`, `RetryDecision`, `CreateSpec`, `Filter` and the eight lifecycle states | → `kernel/contract/workboardapi` — the rows' new home |
| the module's own `ParseStatus` rule and its status order | → the contract's rule (pure), the module keeps a one-line **delegation** |
| `kernel/modules/work/proof` named `Criterion`, `Evidence`, `Proof` (and the rules `Satisfied` / `UnmetCount` / `Clone`) | → `kernel/contract/proofapi` — moved with `git mv`, so the history follows the rows |
| `kernel/modules/work/assure` named `Verdict`, `Attempt`, `Result` and the verdict parser | → `kernel/contract/assureapi` — the rows and the parser; the **loop** stays in the module |
| the tool's read path reached the store **through the kernel** (`k.Workboard()` — a concrete implementation) | → the contract's **port** (`workboardapi.Store`: `Get`, `List`), handed over as `Bind(k, store)` |
| `plugin-reach plugins/tools/workboardtool -> kernel/modules/work/workboard` | **gone** |

The chain moves **as a whole**, because the task row carries a proof and the
proof carries a verdict: a verdict that stayed behind would not fit the proof
that carries it. So the three contracts are `workboardapi` → `proofapi` →
`assureapi`, and each one is a row the plugin can name.

The store, the lifecycle transitions, the retry rule and the on-disk
persistence stay in the module: they are the implementation. The module keeps
its names as **aliases** and its rules as **delegations**, and now carries a
compile-time promise that it satisfies the contract's port.

## The ratchet went DOWN

**295 packages, 90 import violations, 13 call sites** — from 91/13. The
**stale allowlist entry was deleted** (`plugin-reach plugins/tools/workboardtool
-> kernel/modules/work/workboard`).

## The wire matters here more than anywhere else

A task is **persisted** (`$AGEZT_HOME/workboard.json`) and read back by the
`agt workboard` CLI, the console's board view, the Web UI's task detail and
the tool. So the row's JSON keys are a contract with every task file already
on disk, and a renamed key would silently drop that field — a task that lost
its `idempotency_key` would be created twice, and one that lost its `claim`
would be re-claimable while it is already held.

The wire is now pinned key for key: the task's five always-fields, the
twenty optional ones that must stay **off** the wire when they are empty, the
proof's three always-fields and three optional ones, the criterion's two
always-fields and one optional one, and the evidence's four — plus the eight
**lifecycle values**, which are wire too: a renamed state would orphan every
stored task.

The **verdict** is wire as well — it is the model's answer, parsed into the
row and carried by the proof — so `complete` is always present and `gap`
falls away when there is none.

## The refusals say which refusal they are

The status rule refuses in two different ways, and the difference is visible
to an operator: the **empty** status is refused as the bare contract error,
while an **unknown** status is refused naming itself (`workboard: invalid
status: bogus`). A rule that folded the two into one message would lose the
difference between "you sent nothing" and "you sent something I do not
know". Both are measured, and so is the fact that an unknown status has **no
position** in the lifecycle (`-1`) — it is not "the first one", which is the
triage lane's place.

That last one was a real defect found by the new tests: `StatusOrder`
documented `-1` for an unknown status and returned `0`, which is **triage's**
position — an unknown status would have sorted as the first lane. The
implementation now honours the documented contract.

## The wiring, measured

The board arrives at the tool as the contract's port, and the daemon hands it
over at the composition root — where the **nil-port rule** is stated once: a
missing kernel must yield a nil *port*, not a port wrapping a nil
implementation. The check is on the **concrete** type on purpose — a nil
`*Store` handed to an interface parameter is already a non-nil interface.

Two wirings are measured, because a daemon that dropped either would leave
the tool answering "not available" on every live box while every test that
builds the tool by hand stayed green: the **composition root's** own test
asserts that the port is present and answers the **live** board (a task
created through the kernel shows up in the port's listing), and the
**built-in tools'** test asserts that the registered tool is bound to that
same live port (and that a daemon which hands over only the kernel leaves the
tool reporting that the board is unavailable — not dereferencing nil).

## The proof

### Permanent tests

- **The contract's wire and rules** (`kernel/contract/workboardapi/workboardapi_test.go`):
  the task row's exact key set, the always-fields, the optional fields that
  must fall out of the wire, the eight lifecycle values, the known-status
  set, the lifecycle order (strictly increasing, and `-1` for an unknown
  status), the status rule (normalization, the two refusals and their
  messages), the failed-attempt count (only "failed" and "stale" count — case
  and space folded), and the **port's** method set.
- **The assurance contract** (`kernel/contract/assureapi/assureapi_test.go`):
  the verdict's and the result's wire, the attempt budget, and the verdict
  parser (strict JSON, a fenced reply, a prose-wrapped reply, an empty
  object, and the replies it must **refuse**).
- **The proof's wire** (`kernel/contract/proofapi/proof_wire_test.go`): the
  proof's, the criterion's and the evidence's key sets. The proof's own rule
  tests **moved with it** (`proof_test.go`), because a proof that stays behind
  while the rule travels would be a proof of nothing.
- **The identity assertions** (`kernel/modules/work/workboard/contract_type_test.go`,
  `kernel/modules/work/assure/contract_type_test.go`,
  `kernel/modules/work/proof/contract_type_test.go`): the modules' rows ARE
  the contract's rows, the modules' rules AGREE with the contract's on
  **every** verdict (every status, every reply — the same answer, the same
  error, the same message), and the store IS the contract's port.
- **The plugin names the contract** (`plugins/tools/workboardtool/contract_type_test.go`):
  the tool walks the kernel interface it drives and asserts that the module's
  package appears in **none** of it, that its store field IS the contract's
  port, and that an unwired store stays a nil port.
- **The tool's read path** (`plugins/tools/workboardtool/store_port_test.go`):
  the filter the tool asks the port for (status, tenant, assignee, the
  archived switch and the limit), the **bounds** of the limit (the default of
  twenty, the ceiling of a hundred, a negative limit), the id the show op
  asks for, the failed-attempt count on the wire (and its absence when there
  is none), and the unwired board being reported rather than called into.
- **The wiring** (`cmd/agezt/tooldeps_test.go`, `plugins/builtintools/workboard_ports_test.go`):
  the daemon must supply the port and it must answer the **live** board; a
  missing kernel must yield a **nil** port; the registered tool must be bound
  to that live port; and a daemon that hands over only the kernel must leave
  the tool saying the board is unavailable.

### Mutation run

**37 mutations: 33 caught, 4 equivalent.** Nine of them were re-authored —
seven because they produced a **compile error** rather than a kill (an unused
variable, an unused import, a syntax error, and a row whose alias turns out to
be load-bearing) and two because their **anchor was stale** — and all nine
were rewritten so the code still compiles → caught.

Four mutations were **equivalent** — the code behaves the same with and
without them — and they are recorded as such rather than dressed up:

| # | Mutation | Result |
|---|---|---|
| M01 | the empty status is accepted | caught |
| M02 | the status is not lowercased | caught |
| M03 | the status is not trimmed | caught |
| M04 | the lifecycle order moves "ready" | caught |
| M05 | an unknown status has a position (0 instead of -1) | caught |
| M06 | every status is known | compile error → re-authored → caught |
| M07 | every attempt counts as a failure | compile error → re-authored → caught |
| M08 | a stale attempt is not a failure | caught |
| M09 | the failure status is not case-folded | caught |
| M10 | the failure status is not trimmed | caught |
| M11 | an unreadable verdict is accepted | stale anchor → re-authored → caught |
| M12 | the JSON span is not isolated | caught |
| M13 | the empty span is not guarded | **equivalent** — the span still fails to unmarshal, so the refusal still happens |
| M14 | the ``` fence is not dropped | **equivalent** — the span isolation already excludes the fence |
| M15 | the closing fence is not dropped | **equivalent** — likewise |
| M16 | the module's status rule does not delegate | caught |
| M17 | the module's rows are copies, not aliases | compile error (the alias is load-bearing: the store stops answering the port) → re-authored on another row → caught |
| M18 | the module's failed-attempt count does not delegate | compile error (unused import) → re-authored → caught |
| M19 | the assurance module's parser does not delegate | caught |
| M20 | the proof module's rows drifted | caught |
| M21 | the plugin's Bind drops the store | caught |
| M22 | the bound store is not read | caught |
| M23 | an unwired board is called into | caught |
| M24 | the wire drops the failed-attempt count | caught |
| M25 | the default limit is not twenty | caught |
| M26 | the limit ceiling is not a hundred | caught |
| M27 | the status filter is dropped | compile error (unused variable) → re-authored → caught |
| M28 | an empty retry policy is built | caught |
| M29 | the daemon does not supply the board port | caught |
| M30 | a missing kernel is not a nil port | compile error (syntax) → re-authored → caught |
| M31 | a missing board is not a nil port | **equivalent** — a live kernel always opens its board, so the branch is unreachable through the public API; re-authored on the reachable sibling rule (M32) → caught |
| M32 | the registered tool is bound without the store port | caught |
| M33 | the module's retry decision is a copy, not an alias | caught |
| M34 | the task wire renames the idempotency key | stale anchor → re-authored → caught |
| M35 | the triage state is renamed | caught |
| M36 | the verdict wire renames "complete" | caught |
| M37 | the criterion wire renames the text | caught |

The three **equivalent** mutations (M13–M15) are worth stating plainly: the
verdict parser's fence trimming is **redundant** with its span isolation —
the parser finds the `{...}` span whatever surrounds it, so dropping the
fence changes nothing. That is a fact about the code, not a gap in the
tests; the span isolation is the rule that is measured (M12, caught).

### Gates

The build, gofmt, vet, dead-code, archcheck (**295 packages, 90/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **runtime** still names the workboard module, because it owns the
  board's lifecycle (opening the store, publishing the events). The runtime
  is the host — the host may reach a module; it is the *plugin* that may
  not.
- The **app layer** (`kernel/app/workboard`) still names the module for the
  same reason: it is the layer that drives the board's operations, and it is
  above the module.
- The remaining plugin-reach edges: the config tool, the ACP agent, the
  built-in tools, the built-in channels, the guardians.
