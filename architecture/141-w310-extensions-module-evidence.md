# W3.10 — the extensions module has a home

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 step 9 —
`modules/extensions` (mcp, plugin host, toolforge, toolbox, acpcatalog).

## What moved

| Before | After | What it is |
|---|---|---|
| `kernel/mcp` (11 files) | `kernel/modules/extensions/mcp` | the MCP client and registry: the store, the validation, the stdio and HTTP transports |
| `kernel/plugin` (38 files) | `kernel/modules/extensions/plugin` | the out-of-process plugin host: the protocol, the spawn and the read loop, the pin, the spec parsers |
| `kernel/toolforge` (6 files) | `kernel/modules/extensions/toolforge` | the script-tool forge: the store, the draft/test/promote/quarantine lifecycle |
| `kernel/toolbox` (8 files) | `kernel/modules/extensions/toolbox` | the host CLI tool inventory, the recipes and the installer |
| `kernel/acpcatalog` (10 files) | `kernel/modules/extensions/acpcatalog` | the ACP agent discovery, the official clients list and the launch resolution |

All five were already one module — the layer map named `kernel/mcp`,
`kernel/plugin`, `kernel/toolforge`, `kernel/toolbox` and `kernel/acpcatalog`
as layer 3 in the module `extensions` — but they sat at the top of `kernel/`,
outside the `kernel/modules/` tree W3 establishes as the target home of the
bounded contexts. The package names do not change, so the move is an
import-path change and nothing else: **83 importer files** repointed.

The placement is now *derived* (`kernel/modules/*/...` → module `extensions`)
and the **five hand-written layer rules are gone**.

## The ratchet did not move

**275 packages, 126 import violations, 13 call sites** — the same 126/13 as
after W3.9, with **no new edge**. The module's name does not change
(`extensions`), so every edge keeps its shape and only its path changes; the
nine allowlist entries moved with their edges:

- `cross-module kernel/runtime → kernel/modules/extensions/mcp`
- `cross-module kernel/runtime → kernel/modules/extensions/toolforge`
- `cross-module kernel/market → kernel/modules/extensions/mcp`
- `adapter-bypass kernel/controlplane → kernel/modules/extensions/toolbox`
- `plugin-reach plugins/builtinmarket → kernel/modules/extensions/mcp`
- `plugin-reach plugins/builtintools → kernel/modules/extensions/plugin`
- `plugin-reach plugins/tools/mcptool → kernel/modules/extensions/mcp`
- `plugin-reach plugins/tools/forgetool → kernel/modules/extensions/toolforge`
- `plugin-reach plugins/tools/acpagent → kernel/modules/extensions/acpcatalog`

A module that is **five packages** is not a new problem here: the five
packages already were one module, so the host already reached them through
five edges. Splitting a module into more packages is what would cost edges —
not this move.

## The proof

The moved packages keep their own suites, and they are the proof that the move
did not quietly stop testing anything: the plugin host's frame, pin, reload
and flood tests, the MCP store's validation tests, the forge's lifecycle tests,
the toolbox's catalog-integrity tests and the ACP catalog's detection tests all
run in their new home.

One test was **strengthened**: `kernel/modules/extensions/plugin/pin_test.go`'s
`TestVerifyPin_RejectsMalformedPin` asserted only that *an* error came back.
A malformed pin (`""`, `"abc"`, a wrong charset, a wrong length) is rejected
as a **malformed digest**, and that is a different sentence to an operator than
"the file changed" — so the test now says which rejection happened.

## Mutations — 12 mutations, 12 caught

| # | Mutation | Caught by |
|---|---|---|
| M01 | an MCP server may carry both a command and a url | the MCP store's validation suite |
| M02 | an MCP url without a host is accepted | the MCP store's validation suite |
| M03 | the plugin host accepts an oversized frame | the plugin host's frame suite |
| M04 | the plugin host accepts a malformed pin | `TestVerifyPin_RejectsMalformedPin` (strengthened) |
| M05 | the plugin host accepts a mismatched pin | the pin suite |
| M06 | the forge promotes an untested tool | the forge's lifecycle suite |
| M07 | the forge promotes an already active tool | the forge's lifecycle suite |
| M08 | the toolbox clips a short value anyway | the toolbox's helper suite |
| M09 | the toolbox does not pick the first available manager | `TestResolveInstall_PicksFirstAvailableManager` |
| M10 | an unrelated command matches an ACP agent | the ACP catalog's matching suite |
| M11 | a path command does not match an ACP agent | `TestCommandMatchesAgent_Path` |
| M12 | a windows `.exe` command does not match an ACP agent | `TestCommandMatchesAgent_WindowsExe` |

Two notes on the run, recorded honestly:

- **M04 escaped the first pass.** The malformed-pin test asserted only that
  an error came back, and a malformed pin that fell through to the hash
  comparison still came back as an error — a *different* one. The test now
  says which rejection happened, and the mutation is caught on the repeat.
- **One mutation was an equivalent mutant.** Removing the empty-command guard
  in `commandMatchesAgent` changes nothing: the next guard (an empty field
  list) refuses the same case. It was replaced by two mutations that are not
  equivalent (the path normalization and the `.exe` suffix).

The mutations on the moved packages matter as much as the ones on a fix: they
prove the suites still test the code after the move.

## The gates

All green: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (**275 packages, 126 import violations, 13 call sites** — the
ratchet unchanged, five hand-written layer rules gone), `tools/structure-md`
(+ its `-check`), `tools/changelog-lint`, `tools/docclaimscheck`,
`staticcheck`, and the full `go test ./...`.

## What the move broke, and what it cost

A move that renames a path breaks the references that name the path. This one
had four kinds, and all are fixed:

- **The parity ledger.** `cmd/agt/compare_data.go` lists the files that back
  each OpenClaw/Hermes parity claim and **asserts that they exist**; two rows
  pointed at `kernel/mcp/store.go` and `kernel/toolforge/toolforge.go`. Fixed.
- **The dead-code checker's own allowlist.** `tools/deadcodecheck` allows the
  `acpcatalog` package's SDK-facing exports by path prefix
  (`kernel/acpcatalog/`); the prefix moved with the package. Fixed — and the
  gate still passes, which is the proof that the allowlist entry was the only
  thing standing between the moved package and a false "dead code" report.
- **The layer map.** The five hand-written rules are gone; the placement is
  derived now.
- **The documentation.** Eleven live documents named the old paths
  (`architecture/00`, `04`, `07`, `08`, `10`, `12`, `20`, `docs/API-STABILITY`,
  `docs/CONSOLE-IA`, `docs/PLUGIN-SECURITY`, `docs/SPEC-IMPLEMENTATION-STATUS`)
  plus `docs/THREAT-MODEL`, `docs/REFACTORING-SCAN`, the plugin-governance
  example and the SDK's own doc comment. All updated. The **dated** reports
  (`.project/PHASE-*`, `CHANGELOG/unreleased/m*.md`, the dated scans and
  audits) describe the tree as it was and are left alone.

## What is deliberately left

- **`modules/providers`** (catalog, chatgptauth, providerboot) is the next W3
  step, then `modules/runs` last (what remains of `kernel/runtime` +
  `runexec` + the agent loop + resume + delegation + intervention;
  `kernel/runtime` is deleted when empty).
- **The agents module's overseer ops.** Step 8's third item — the overseer
  tool as an L6 caller of `app` ops — is still open; the api door it needed
  now exists (W3.9).
- **The plugin host's protocol consolidation.** The target architecture's
  `platform/extension` row (one out-of-process protocol, the dead
  `contract/gen` halves) is a later step, not this move.
