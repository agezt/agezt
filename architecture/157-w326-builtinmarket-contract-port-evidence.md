# W3.26 — the marketplace's built-in catalogue reads the bundles through the contract's port

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the **built-in "Official" catalogue** — the marketplace's built-in
packs, built from the skill bundles baked into the binary. The catalogue is a
plugin (`plugins/builtinmarket`), and a plugin may reach the contract and the
platform layers and nothing else.

## What moved

| Before | After |
|---|---|
| `plugins/builtinmarket` named `skill.BuiltinNames()` and `skill.BuiltinBundle(name)` — the module's **public seam** for the embedded bundles | → the contract's **port** (`skillapi.Builtins`: the bundle names, and one bundle's SKILL.md plus its resource files). The module's implementation is now `skill.NewBuiltins()` |
| `plugins/builtinmarket` named `skill.ParseSkillMD` | → the contract's parser (`skillapi.ParseSkillMD`). The parser is **pure** — a stdlib-only frontmatter reader — so it belongs in the contract, and the marketplace (a plugin) is exactly the kind of caller that may only name the contract |
| `plugin-reach plugins/builtinmarket -> kernel/modules/skills/skill` | **gone** |

The **embedded bytes** stay in the module (`kernel/modules/skills/builtins`):
they are the implementation, and a plugin has no business naming them. The
module's `ParseSkillMD` name stays as a **delegation** — so there is one
parser, and the module's own CLI (`agt skill import`) and the marketplace
cannot drift apart.

## The ratchet went DOWN

**288 packages, 97 import violations, 13 call sites** — from 98/13. The
**stale allowlist entry was deleted**.

## The wiring is a contract too — and the daemon hands the bundles over

The catalogue is built over whatever the port hands over. If the daemon handed
over nothing, the market would offer **no packs at all** — and every test
that builds the plugin by hand would still be green. So the daemon's wiring
was pulled out of `runDaemon` into a named function
(`cmd/agezt/marketlib.go`, `builtinMarketLibrary`) and **measured there**: the
catalogue the daemon actually serves must carry the built-in bundles.

## The proof

### Permanent tests

- **The parser, where the row is** (`kernel/contract/skillapi/skillmd_test.go`,
  moved from the module with the parser): the inline lists, the block lists,
  the quoted scalars, the `tools` alias, the CRLF endings — and the errors,
  measured **by which error they are**: a parser that rejects a document for
  the wrong reason still rejects it, and the reason is what an importer acts
  on.
- **One parser, not two** (`kernel/modules/skills/skill/contract_type_test.go`):
  the module's `ParseSkillMD` and the contract's agree on every document —
  the same verdict, the same row and the same error message.
- **The port** (same file): the module's built-ins implementation IS the
  contract's port (a compile-time promise), and the module's `SkillMD` IS the
  contract's row.
- **The plugin reads through the port** (`plugins/builtinmarket/ports_test.go`):
  the catalogue is built from what the port hands over, and a bundle that
  cannot be read — or whose SKILL.md does not parse — is **skipped**, not
  fatal, while the rest of the catalogue still stands.
- **The wiring** (`cmd/agezt/marketlib_test.go`): the catalogue the daemon
  serves is not empty.

### Mutation run

**18 mutations: 18 caught.** One mutation **survived the first round** — the
parser's "no frontmatter" check removed: the test only asserted that *an*
error came back, and the parser failed for another reason instead. The test
now measures **which** error it is, and the mutation was run again → caught.
Two mutations were **first authored as build errors** (a compile error is not
a catch) and were re-authored so the code still compiles.

| # | Mutation | Result |
|---|---|---|
| M01 | the parser accepts a document with no name | caught |
| M02 | the parser accepts a document with no body | caught |
| M03 | the `tools` alias is dropped | caught |
| M04 | the trigger list is dropped | caught |
| M05 | the tools list is dropped | caught |
| M06 | the quoted scalars are not unquoted | caught |
| M07 | a document without frontmatter is accepted | **survived the first round** — the error test asserted only that *an* error came back; the error is now measured → caught |
| M08 | the CRLF line endings are not normalized | caught |
| M09 | the description is dropped | caught |
| M10 | the module's parser delegation disagrees with the contract | caught |
| M11 | the built-ins port lists nothing | caught |
| M12 | the built-ins port hands over nothing | caught |
| M13 | the catalogue reads no bundles at all | caught |
| M14 | the pack loses its description | caught |
| M15 | the pack loses the parsed triggers | caught |
| M16 | the pack loses the bundle resources | **build error** at first (the variable became unused) — re-authored → caught |
| M17 | a bundle that does not parse still becomes a pack | caught |
| M18 | the daemon hands over no built-in bundles | **build error** at first (the import became unused) — re-authored → caught |

### Gates

The build, gofmt, vet, dead-code, archcheck (**288 packages, 97/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **guardians** still reach into the roster module: they need
  `roster.Validate`, and the validation is not pure — it reads the trust
  ceiling through `kernel/edict`, and a contract package may not import a
  platform package. That is a slice of its own.
- The remaining plugin-reach edges: the database tool, the workboard tool,
  the forgetool, the market plugin's own edge to `kernel/market`, the
  built-in tools and the built-in channels.
