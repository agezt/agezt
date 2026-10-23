# W3.28 — the script-tool rows move to the contract layer

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the **script-tool rows** — one named, durable script the agent can
call as a tool, the lifecycle state it is in, and the rule that decides whether
it may be drafted — and the **port** through which the agent-facing tool reads
the stored ones. The `tool_forge` tool is a plugin
(`plugins/tools/forgetool`), and a plugin may reach the contract and the
platform layers and nothing else.

## What moved

| Before | After |
|---|---|
| `kernel/modules/extensions/toolforge` named `ScriptTool` and `Status` (and its three constants) | → `kernel/contract/toolforgeapi` — the rows' new home |
| the draft gate (`Validate`) and the three size caps it enforces | → the contract's. The gate is **pure** — a name pattern, a language pattern, three size limits — so it belongs where the row is, and it is the rule the whole daemon enforces |
| the tool's own `Kernel` interface, naming `toolforge.ScriptTool` | → the contract's row. The plugin may not name the module |
| the tool's read path (`k.ToolForge().List()` / `.Get()`) — a **concrete** store type reached through the kernel | → the contract's **port** (`toolforgeapi.Store`: `Get`, `List`), which the daemon hands over as a second argument to `Bind(k, store)` |
| `plugin-reach plugins/tools/forgetool -> kernel/modules/extensions/toolforge` | **gone** |

The **store**, the lifecycle transitions and the on-disk persistence stay in
the module: they are the implementation. The module keeps the names its
callers use as **aliases** and its rule as a one-line **delegation** — so
there is one row and one spelling of it. The module's store now carries a
compile-time promise that it satisfies the contract's port.

## The ratchet went DOWN

**290 packages, 95 import violations, 13 call sites** — from 96/13. The
**stale allowlist entry was deleted**.

The `cross-module kernel/runtime -> kernel/modules/extensions/toolforge` edge
stays: the runtime drives the forge's **lifecycle** (add, update, record a
test, promote, quarantine, remove), and those are implementation calls, not
rows. The plugin's edge was the one that had to go.

## The wire matters here too

A script tool is **persisted** (`<home>/scripttools.json`) and its record is
read by key — the CLI (`agt toolforge list/show`), the console's forge view
and the Web UI all read it. So the row's JSON keys are a contract with every
tool file already on disk, and a renamed key would silently drop that field.

The wire is now pinned key for key (the ten always-fields, the two optional
ones that must stay **off** the wire when they are empty), and the three
**lifecycle values** (`draft`, `active`, `quarantined`) are pinned too — a
renamed state would orphan every stored tool.

## The rules are the daemon's rules, so they are measured rule by rule

The draft gate decides what may become a tool. Every refusal is now measured
individually — a name that is not a token, a missing description (the **model**
reads it to decide when to call the tool), a missing language, missing code, a
schema that is not a JSON object — and every **limit** is measured **at its
boundary** (a name of exactly 40 characters passes, 41 does not; a description
of exactly 2 KiB passes, one byte more does not), because a cap that is only
measured at cap+1 would also pass an implementation that had no cap at all.
The cap **numbers** are pinned separately: a test that measures a cap against
the constant itself passes any value at all.

## The interesting gap was the wiring

The read ops (`op=list`, `op=show`) need the store. The tool used to reach it
through the kernel (`k.ToolForge()`), which is how a plugin got hold of a
concrete implementation. Now the store arrives as the contract's port — and a
port that nobody hands over is a port that is **silently nil**: the tool would
answer "not available on this daemon" on every live box, while every test that
builds the tool by hand stayed green.

So the daemon's wiring was pulled out into a named function
(`cmd/agezt/tooldeps.go` → `forgeStore` / `forgePort`) and **measured there**:
the daemon must supply the port, and the port must answer the **live** store.
The **nil-port rule** is stated once — a nil store must stay a nil *port*, not
a port wrapping a nil implementation — and the check is on the **concrete**
type on purpose: a nil `*Store` handed to an interface parameter is already a
non-nil interface.

## The proof

### Permanent tests

- **The wire shape** (`kernel/contract/toolforgeapi/toolforgeapi_test.go`): the
  row's exact key set, the always-fields, the optional fields that must fall
  out of the wire, the three lifecycle values, and the **port's** method set
  (exactly `Get` and `List`, with their signatures — an extra method would be
  a capability the plugin could reach without the daemon's consent).
- **The rules** (same file): `Validate` refuses — rule by rule, each with its
  own message — a bad name, an empty or blank description, a missing or
  malformed language, missing or blank code, a schema that is not JSON and a
  schema that is not an object; the name length and the three size caps are
  measured **at their boundaries**; and the three cap numbers are pinned.
- **The identity assertions** (`kernel/modules/extensions/toolforge/contract_type_test.go`):
  the module's row IS the contract's row, its lifecycle values ARE the
  contract's, its caps ARE the contract's, and its store IS the contract's
  port.
- **One rule, not two** (same file): the module's `Validate` and the
  contract's agree on every verdict — **and on the message**, because a rule
  that refuses for the wrong reason is a rule nobody can debug.
- **The plugin names the contract** (`plugins/tools/forgetool/contract_type_test.go`):
  the tool walks its own kernel interface and its `Bind` signature and asserts
  that the module's package appears in **neither** — a plugin may reach the
  contract and nothing else.
- **The tool's wire** (`plugins/tools/forgetool/wire_test.go`): the view the
  agent reads is pinned key for key (the five always-fields, the two
  conditional ones), the **test record** is measured as a *value* (a tool that
  was tested must say so), and the list carries its count.
- **The wiring, twice** (`cmd/agezt/tooldeps_test.go`,
  `plugins/builtintools/forge_ports_test.go`): the daemon must supply the
  port and it must answer the **live** store; a tool the agent drafted
  through the tool must show up in that same store; and a daemon that handed
  over no store must leave the read ops reporting that — not calling into
  nil.

### Mutation run

**41 mutations: 41 caught.** Four mutations **survived the first round** — the
three cap values (the test measured the cap against the constant itself, so
*any* value passed) and the test record (the view was only ever read on an
untested tool) — so the tests were strengthened and the mutations were run
again → caught. One mutation had a **stale anchor** (an em dash the harness
could not match through its encoding) and was re-authored so the code still
compiles → caught.

| # | Mutation | Result |
|---|---|---|
| M01 | the name pattern's length limit is shortened | caught — by the boundary test |
| M02 | the name pattern loses its length limit | caught |
| M03 | the name pattern accepts an upper-case start | caught |
| M04 | a blank description is accepted | caught |
| M05 | the description's size cap is not enforced | caught |
| M06 | the description's cap number is doubled | **survived the first round** — the test measured the cap against the constant; the number is pinned now → caught |
| M07 | a missing language is accepted | caught |
| M08 | blank code is accepted | caught |
| M09 | the code's size cap is not enforced | caught |
| M10 | the code's cap number is doubled | **survived the first round** → caught |
| M11 | the schema's size cap is not enforced | caught |
| M12 | the schema's cap number is doubled | **survived the first round** → caught |
| M13 | a schema that is not JSON is accepted | caught |
| M14 | a schema that is not an object is accepted | caught |
| M15 | the `draft` state is renamed | caught |
| M16 | the `active` state is renamed | caught |
| M17 | the `quarantined` state is renamed | caught |
| M18 | the `tested_ok` key is renamed | caught |
| M19 | the optional `input_schema` stays on the wire when it is empty | caught |
| M20 | the optional `tested_ms` stays on the wire when it is zero | caught |
| M21 | the `language` key is renamed | caught |
| M22 | the module's rule stops delegating to the contract | caught |
| M23 | the module's `draft` value disagrees with the contract | caught |
| M24 | the module's `active` value disagrees with the contract | caught |
| M25 | the module's code cap disagrees with the contract | caught |
| M26 | the module's description cap disagrees with the contract | caught |
| M27 | the view always reports "not tested" | **survived the first round** — the view was only read on an untested tool; the test record is measured as a value now → caught |
| M28 | the view drops the language | caught |
| M29 | the callable name loses its `forge_` prefix | caught |
| M30 | the description falls out of the wire | caught |
| M31 | the list's count is always zero | caught |
| M32 | the show op drops the code | caught |
| M33 | the read ops stop reporting a missing store | caught |
| M34 | `Bind` drops the store | caught |
| M35 | the demotion warning loses its "re-test" | **stale anchor** at first — re-authored → caught |
| M36 | the daemon stops supplying the port | caught |
| M37 | the daemon's nil-kernel guard is removed | caught |
| M38 | the daemon's nil-store guard is removed (the typed-nil trap) | caught |
| M39 | the registry hands the tool a nil store | caught |
| M40 | the description's cap number is one byte off | caught |
| M41 | the view drops the status | caught |

### Gates

The build, gofmt, vet, dead-code, archcheck (**290 packages, 95/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## One operator-visible change, recorded

The native `toolforge_draft` / `toolforge_edit` operations decode the tool
argument into the row, and the JSON decoder names the type it failed on. That
type is now the contract's row, so the message reads
`toolforgeapi.ScriptTool` where it used to read `toolforge.ScriptTool`. The
refusal is the same; only the name of the type in the message changed, and it
now names the row's real home.

## What is deliberately left

- The **guardians** still reach into the roster module: they need
  `roster.Validate`, and the validation is not pure — it reads the trust
  ceiling through `kernel/edict`, and a contract package may not import a
  platform package. That is a slice of its own.
- The **runtime** still reaches into the toolforge module, because it drives
  the forge's lifecycle transitions (add, update, record a test, promote,
  quarantine, remove) — implementation calls, not rows. Moving that store to
  the app layer is a slice of its own.
- The remaining plugin-reach edges: the database tool, the workboard tool,
  the config tool, the workflow tool, the ACP agent, the built-in tools and
  the built-in channels.
