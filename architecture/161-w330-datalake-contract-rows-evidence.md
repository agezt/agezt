# W3.30 — the data-lake rows move to the contract layer

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the **Personal Data Lake's rows** — one collection's definition
(its schema and the columns it carries), one record (a row with a stable id
and a free-form field map), the query that narrows a listing, the summary a
listing shows, and the three refusals the lake answers with — and the
**port** through which the lake is driven. The tool that drives it
(`plugins/tools/db`) is a plugin, and a plugin may reach the contract and the
platform layers and nothing else.

## What moved

| Before | After |
|---|---|
| `kernel/modules/artifacts/datalake` named `Field`, `Schema`, `Record`, `Query`, `CollectionInfo` and the three errors | → `kernel/contract/datalakeapi` — the rows' new home |
| the tool's own `Store` interface — the lake's method set, spelled in the **module's** rows | → the contract's **port** (`datalakeapi.Lake`), which the module's lake satisfies and the tool drives |
| `kernel/toolreg`'s `KernelDeps.Lake` — a **concrete** `*datalake.Lake` | → the contract's port, so the registry names the contract and not the module |
| `plugin-reach plugins/tools/db -> kernel/modules/artifacts/datalake` | **gone** |
| `cross-module kernel/toolreg -> kernel/modules/artifacts/datalake` | **gone** |

This slice closes **two** edges at once — the plugin's and the registry's —
because the registry was holding the lake by its concrete type. The lake
itself, its persistence, the built-in collection gallery and the name rule
stay in the module: they are the implementation. The module keeps the names
its callers use as **aliases** and now carries a compile-time promise that it
satisfies the contract's port.

## The ratchet went DOWN

**292 packages, 91 import violations, 13 call sites** — from 93/13. The two
**stale allowlist entries were deleted**.

## The wire matters here too

A collection and its records are **persisted** (`$AGEZT_HOME/datalake/<coll>/
_schema.json` and `rec/<id>.json`) and read back by the console's tables, the
Web UI's bespoke views and the `db` tool. So the rows' JSON keys are a
contract with every collection file already on disk, and a renamed key would
silently drop that field.

The wire is now pinned key for key: the schema's ten fields, the column's
three, the record's six, and the listing summary's count — plus the
**optional** fields that must stay **off** the wire when they are empty (an
empty `"type": ""` in a stored schema would be a different file from the same
schema written by another version).

## The refusals are the tool's answers

The lake answers with three refusals — not found, already exists, system
collection — and the tool maps them to three **different** answers, each
naming the collection. So the messages are part of the contract, and they are
pinned: not only the refusal, but the tool's own phrasing, which names the
collection (the lake's raw message does not).

## The wiring, measured

The lake arrives at the registry as the contract's port, and the daemon hands
it over at the composition root — where the **nil-port rule** is stated once:
a nil lake must stay a nil *port*, not a port wrapping a nil implementation.
The check is on the **concrete** type on purpose — a nil `*Lake` handed to an
interface parameter is already a non-nil interface — so the rule is a named
function that can be tested on its own, and the composition root's own test
asserts that the port is present and answers the **live** lake (the daemon
seeds its seven built-in collections, and a collection created through the
port shows up in the same listing).

## The proof

### Permanent tests

- **The wire shape** (`kernel/contract/datalakeapi/datalakeapi_test.go`): the
  schema's, the column's and the record's exact key sets, the always-fields,
  the optional fields that must fall out of the wire (on the schema, on the
  column and on the record), the listing summary's embedded schema, the
  three refusals' messages, and the **port's** method set (exactly nine
  methods, with their signatures).
- **The identity assertions** (`kernel/modules/artifacts/datalake/contract_type_test.go`):
  the module's rows ARE the contract's rows, its errors ARE the contract's
  (by identity and by message), and its lake IS the contract's port.
- **The plugin names the contract** (`plugins/tools/db/contract_type_test.go`):
  the tool walks the port it drives and asserts that the module's package
  appears in **none** of it — a plugin may reach the contract and nothing
  else — and that the tool's store **is** the contract's port.
- **The tool's answers** (`plugins/tools/db/wire_test.go`): a create must say
  it created the collection, the stored collection must keep the **declared
  columns** (a create that dropped the declaration would leave the agent with
  a table it cannot write the right fields into), a second create must be
  refused with the tool's own answer that **names** the collection, and a
  tool that was never given the lake must say so rather than call into
  nothing.
- **The wiring** (`cmd/agezt/tooldeps_test.go`): the daemon must supply the
  port, and it must answer the **live** lake; a missing kernel and a missing
  lake must both yield a **nil** port.

### Mutation run

**36 mutations: 36 caught.** Two mutations **survived the first round** — the
column's optional `type` and `label` keys (the fixture had both filled, so
the `omitempty` was never exercised) and the create op's refusal mapping (the
test asked for "already exists", which the lake's raw message also contains).
So the tests were strengthened — a column with an empty type and label, and
the tool's own phrasing with the collection's name — and they were run again
→ caught. One mutation had a **stale anchor** and one was a **compile error**
(the mutation left a variable unused); both were re-authored so the code
still compiles → caught.

| # | Mutation | Result |
|---|---|---|
| M01 | the column's `name` key is renamed | caught |
| M02 | the column's `type` stays on the wire when it is empty | **survived the first round** — the fixture had a type → caught |
| M03 | the column's `label` stays on the wire when it is empty | **survived the first round** → caught |
| M04 | the schema's `name` key is renamed | caught |
| M05 | the schema's `title` stays on the wire when it is empty | caught |
| M06 | the schema's `view` stays on the wire when it is empty | caught |
| M07 | the schema's `fields` stays on the wire when it is empty | caught |
| M08 | the schema's `builtin` stays on the wire when it is false | caught |
| M09 | the schema's `system` stays on the wire when it is false | caught |
| M10 | the schema's `created_ms` key is renamed | caught |
| M11 | the schema's `created_by` stays on the wire when it is empty | caught |
| M12 | the schema's `icon` stays on the wire when it is empty | caught |
| M13 | the schema's `desc` stays on the wire when it is empty | caught |
| M14 | the record's `id` key is renamed | caught |
| M15 | the record's `fields` key is renamed | caught |
| M16 | the record's `updated_ms` key is renamed | caught |
| M17 | the record's `updated_by` stays on the wire when it is empty | caught |
| M18 | the record's `created_ms` key is renamed | caught |
| M19 | the listing summary's `count` key is renamed | caught |
| M20 | the "not found" refusal's message changes | caught |
| M21 | the "already exists" refusal's message changes | caught |
| M22 | the "system collection" refusal's message changes | caught |
| M23 | the module's `ErrNotFound` is not the contract's | compile error → re-authored → caught |
| M24 | the module's `ErrExists` is not the contract's | compile error → re-authored → caught |
| M25 | the module's `ErrSystem` is not the contract's | compile error → re-authored → caught |
| M26 | the create op maps the wrong refusal | **survived the first round** — the lake's raw message also says "already exists" → caught |
| M27 | the drop op maps the wrong refusal | caught |
| M28 | the drop op answers "not found" for a built-in | caught |
| M29 | the create op drops the declared columns | **survived the first round** → caught |
| M30 | the create op stops answering with the collection | compile error → re-authored → caught |
| M31 | the daemon stops supplying the port | caught |
| M32 | the daemon's nil-lake guard is removed (the typed-nil trap) | caught |
| M33 | the daemon's nil-kernel guard is removed | caught |
| M34 | the create op's answer loses "already" | stale anchor → re-authored → caught |
| M35 | the drop op's built-in answer loses half its sentence | caught |
| M36 | the drop op's missing-collection answer changes | caught |

### Gates

The build, gofmt, vet, dead-code, archcheck (**292 packages, 91/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **guardians** still reach into the roster module: they need
  `roster.Validate`, and the validation is not pure — it reads the trust
  ceiling through `kernel/edict`, and a contract package may not import a
  platform package. That is a slice of its own.
- The **runtime** still reaches into the artifacts module, because it owns
  the lake's lifecycle (opening it, seeding the built-ins). Moving the lake
  to the app layer is a slice of its own.
- The remaining plugin-reach edges: the workboard tool, the config tool, the
  ACP agent, the built-in tools, the built-in channels, the guardians.
