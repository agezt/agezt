# W3.21 — two more tools name their rows where they live

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the **board tool** (how agents talk to each other) and the
**artifacts tool** (the files an agent saved). Both are plugins, and both
reached into a module: the board tool for the message row, the wildcard
recipient and the **store it drove**, the artifacts tool for the artifact row
and the **filter** it queries with.

## What moved

| Before | After |
|---|---|
| `plugins/tools/boardtool` named `board.Message` | → `kernel/contract/boardapi` — the row already lived there (W3.14) |
| `plugins/tools/boardtool` named `board.Everyone` | → `boardapi.Everyone` — the wildcard recipient moves to the contract with the row |
| `plugins/tools/boardtool` declared its own `boardStore` interface and took `*board.Store` in `BindStore` | → `boardapi.Store`, a **port** in the contract layer: the eleven methods the tool drives. The module's `*board.Store` implements it |
| `plugins/tools/boardtool.Bind(dir)` **opened the module's store** | → **gone**. The daemon opens the one shared store and hands it over; a plugin may not open a store of its own (two writer instances silently drop each other's messages) |
| `plugins/tools/artifacts` named `artifact.Entry` and `artifact.Filter` | → `kernel/contract/remoteapi` — the artifact row already lived there (W3.16); the filter is the query over that row, so it lives beside it |
| `plugin-reach plugins/tools/boardtool -> kernel/modules/board/board` | **gone** |
| `plugin-reach plugins/tools/artifacts -> kernel/modules/artifacts/artifact` | **gone** |

The wildcard recipient was the interesting one. `board.Everyone` was a value
the module used consistently — so **no test could see it change**. But the
value is read **by value** across the boundary: the persisted board file
stores it, the daemon's boot replay special-cases it, and the **SDK's mailbox
filter compares the literal `"*"`**. A change to the constant would have
silently broken every reader that knows the value and not the constant. The
SDK now names the contract's constant instead of repeating the literal, and a
test pins the value itself.

## The ratchet went DOWN

**283 packages, 103 import violations, 13 call sites** — from 105/13. The
**two stale allowlist entries were deleted** — a leftover entry fails the
archcheck gate (the debt was paid, the list must shrink).

## The proof

### Permanent tests

- **The store port** (`kernel/modules/board/board/store_port_test.go`): a
  compile-time assertion that the board's store IS `boardapi.Store`, and a
  test that the store still works when it is *held as the port* — the tool
  never sees the concrete type, so a method that only worked on `*Store` would
  slip through every other test.
- **The identity assertions**: the artifacts module's `Filter` IS the
  contract's (`kernel/modules/artifacts/artifact/contract_type_test.go`), as a
  function type — an assignment would also accept a copy with the same fields,
  while a function type requires identity.
- **The wildcard's wire value** (`kernel/contract/boardapi/boardapi_test.go`):
  the value is pinned, because it is read by value.
- **The wiring** (`plugins/builtintools/board_artifact_ports_test.go`):
  against a **real** board store and a **real** kernel, a message the board
  tool posts lands in the daemon's own store (so the port really carries the
  live store), and an artifact the index holds is listed by the tool (so the
  contract-typed interface really carries the live index).
- **The wire shapes**, which had never been measured: the board tool's
  message view and the artifacts tool's list row are now pinned — every key
  they emit is a key of the contract row, the values are the row's own, and
  the empty fields stay out of the wire.

### Mutation run

**17 mutations: 14 caught, 3 build errors (recorded separately).** One
mutation **survived the first round** — the wildcard's value — and the gap was
closed before it was run again.

| # | Mutation | Result |
|---|---|---|
| M01 | the wildcard recipient's value changes | **survived the first round** — no test could see it, because the module used the constant consistently; the value is read by value across the boundary, so the SDK now names the constant and a test pins the value → caught |
| M02 | the contract's store port loses a method | **build error** — the port is a promise, not a comment |
| M03 | the board tool's `BindStore` drops the store | caught — **by the wiring test** |
| M04 | the board view drops the reply link | caught |
| M05 | the board view renames `reply_to` | caught |
| M06 | the board module's wildcard alias drifts from the contract | caught |
| M07 | the contract's filter loses its `Source` field | **build error** |
| M08 | the artifacts row drops the caption | caught |
| M09 | the artifacts tool's filter drops the correlation | caught |
| M10 | the artifacts row renames `mime` | caught |
| M11 | the board view renders the wrong instant | caught |
| M12 | the board view drops the help flag | caught |
| M13 | the SDK's broadcast filter stops matching the wildcard | caught |
| M14 | the allowlist gets the paid debt back | caught — **by the archcheck gate** |
| M15 | the board tool binds a nil store | caught — by the wiring test |
| M16 | the artifacts tool stops deleting | caught |
| M17 | the contract's store port loses the topic read | **build error** |

M03 and M15 are caught only by the **wiring** test: the tool's own suite
proves the tool, and nothing but a test against the live store proves that
the daemon still hands it what it needs.

### Gates

The build, gofmt, vet, dead-code, archcheck (**283 packages, 103/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **guardians** still reach into the roster module: they need
  `roster.Validate`, and the validation is not pure — it reads the trust
  ceiling through `kernel/edict`, and a contract package may not import a
  platform package. That is a slice of its own.
- The remaining plugin-reach edges: the database tool, the workboard tool,
  the skill tool, the forgetool, the MCP tool, the introspector's kernel
  edge, the market plugin, the built-in tools and the built-in channels.
