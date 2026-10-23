# W3.22 — the MCP server's row moves to the contract layer

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the **MCP server row** — the description of one registered MCP
server, a stdio command or a remote HTTP endpoint the daemon can spawn and
speak MCP to. Two plugins name it: the **MCP tool** (the agent-facing one) and
the **marketplace** (which ships the builtin server packs).

## What moved

| Before | After |
|---|---|
| `plugins/tools/mcptool` named `mcp.Server` | → `kernel/contract/mcpapi` — the row's new home |
| `plugins/tools/mcptool` reached for the **registry** (`MCPStore() *mcp.Store`) | → the kernel's new **row accessors** `MCPServers()` / `MCPServer(ref)`. The registry — the store that persists the rows — stays in the module |
| `plugins/builtinmarket` named `mcp.Server` | → `mcpapi.Server` |
| the module's `Validate` (the rule that checks a server's fields) | → the contract's, with the **caps** (`MaxArgs`, `MaxEnv`, `MaxHeaders`, `MaxToolAllow`) beside it. The module keeps the name as a one-line delegation, and the rule is measured to be *the same rule* |
| `plugin-reach plugins/tools/mcptool -> kernel/modules/extensions/mcp` | **gone** |
| `plugin-reach plugins/builtinmarket -> kernel/modules/extensions/mcp` | **gone** |

**The rule travels with the row.** `Validate` checks the name format, the
one-transport rule (a server is either a local process or a remote endpoint),
the URL's shape and the caps — and it is pure, so it belongs where the row is.
The caps are the policy, so they are exported from the contract and the module
aliases them.

**The row's wire was never measured.** The registry PERSISTS these rows and
the control plane and the Web UI read them back by key, so a renamed key is a
silent break: the old registry file would lose the field and the UI would read
nothing. The wire is now pinned — the exact key set of a full row, and the
always-fields that must stay on the wire even when they are empty.

## The ratchet went DOWN

**284 packages, 101 import violations, 13 call sites** — from 103/13. The
**two stale allowlist entries were deleted** — a leftover entry fails the
archcheck gate (the debt was paid, the list must shrink).

## The proof

### Permanent tests

- **The wire shape** (`kernel/contract/mcpapi/mcpapi_test.go`): the exact key
  set of the row (13 keys) and the always-fields of a minimal row, checked
  against the row's own tags — so the test is checked against the
  *declaration*, not against a copy of it that could drift with it.
- **The rule** (same file): the name format, the one-transport rule, the URL's
  shape, the env and header names, the allowlist — and the **caps measured at
  the boundary** (32 accepted / 33 rejected; 128 accepted / 129 rejected). A
  test that builds `Cap+1` items would pass whatever the cap is, so the
  policy is spelled out instead of read back from the constant.
- **The identity assertion** (`kernel/modules/extensions/mcp/contract_type_test.go`):
  the module's `Server` IS the contract's, as a function type — an assignment
  would also accept a copy with the same fields, while a function type requires
  identity.
- **The rule is one rule** (same file): the module's `Validate` and the
  contract's must agree on every verdict *and* on the message — a second copy
  of the rule would drift silently.
- **The wiring** (`plugins/builtintools/mcp_ports_test.go`): against a **real**
  kernel, a server registered in the kernel's own registry is listed by the
  tool, and a server the tool adds lands in that same registry.

### Mutation run

**13 mutations: 11 caught, 2 build errors (recorded separately).** Two
mutations **survived the first round** — the caps — because the test built
`Cap+1` items and therefore passed whatever the cap was; the caps are now
measured at the boundary, and both mutations were run again.

| # | Mutation | Result |
|---|---|---|
| M01 | the `tool_allow` key is renamed | caught — by the wire test |
| M02 | `enabled` loses its always-field | caught |
| M03 | the name-format rule is dropped | caught |
| M04 | the one-transport rule is dropped | caught |
| M05 | the env-name rule is dropped | caught |
| M06 | the arg cap is raised | **survived the first round** — the test built `MaxArgs+1` args, so it passed whatever the cap was; the cap is now measured at the boundary → caught |
| M07 | the module's `Validate` stops validating | caught |
| M08 | the kernel's row accessor returns nothing | caught — **by the wiring test** |
| M09 | the kernel's row lookup always misses | caught — by the wiring test |
| M10 | the module's row drifts from the contract's | **build error** — the identity assertion |
| M11 | the tool's port method is renamed | **build error** |
| M12 | the marketplace's builtin server is renamed | caught |
| M13 | the tool-allowlist cap is raised | **survived the first round** — the same self-referential shape; all four caps are now measured at their boundary → caught |

M08 and M09 are caught only by the **wiring** test: the tool's own suite proves
the tool, and nothing but a test against a live kernel proves that the daemon
still hands it what it needs.

### Gates

The build, gofmt, vet, dead-code, archcheck (**284 packages, 101/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **guardians** still reach into the roster module: they need
  `roster.Validate`, and the validation is not pure — it reads the trust
  ceiling through `kernel/edict`, and a contract package may not import a
  platform package. That is a slice of its own.
- The remaining plugin-reach edges: the database tool, the workboard tool,
  the skill tool, the forgetool, the introspector's kernel edge, the market
  plugin, the built-in tools and the built-in channels.
