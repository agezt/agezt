# W3.27 — the market's rows move to the contract layer

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the **marketplace's rows** — one installable pack, the skill and
MCP servers it carries, the catalogue entry a marketplace lists it under, the
catalogue itself — and the **rules** that govern them. The built-in
"Official" catalogue is a plugin (`plugins/builtinmarket`), and a plugin may
reach the contract and the platform layers and nothing else.

## What moved

| Before | After |
|---|---|
| `kernel/market` named `Pack`, `PackSkill`, `Signature`, `Marketplace` and `MarketplaceEntry` | → `kernel/contract/marketapi` — the rows' new home |
| the pack's methods (`Counts`, `Validate`, `CanonicalBytes`, `ContentHash`, `Entry`) | → the contract's, **with the row** (a method can only be defined in its type's package) |
| the market's `Library` interface (the catalogue port) | → the contract's **port** (`marketapi.Library`), implemented by the built-in catalogue and by the market's own composite library |
| the version comparator (`CompareVersions`) and the skill summary (`SkillSummary`) | → the contract's. Both are pure, and both are **the one rule** their callers must share: a second comparator would silently fork the ordering that decides which version an install resolves to |
| the name and version patterns | → the contract's, as the **pattern strings** (`NamePattern`, `SemverPattern`). Each side compiles its own regex from them, so the error text an operator reads is the same on both |
| the resource-path rule (`safeRelPath`) | → the contract's (`SafeRelPath`) |
| `plugin-reach plugins/builtinmarket -> kernel/market` | **gone** |

The **manager**, the **store**, the **syncer** and the composite library stay
in the module: they are the implementation. The module keeps the names its
callers use as **aliases** and its rules as one-line **delegations** — so
there is one row and one spelling of it.

`InstalledPack` stays in the module on purpose: it is the manager's own
bookkeeping, persisted in `installed.json`, and no plugin names it.

## The ratchet went DOWN

**289 packages, 96 import violations, 13 call sites** — from 97/13. The
**stale allowlist entry was deleted**.

## This is the row where the wire matters

A pack is **signed**, **hashed** and **published**: the signature is over the
pack's canonical bytes, the content hash is the SHA-256 of those bytes, and a
registry is written against the JSON. So the row's JSON keys are a contract
with every published pack and every signature ever made — a renamed key would
silently change what the signature covers.

The wire is now pinned key for key (the pack, the skill it carries, the
catalogue entry, the catalogue), and the **always-fields** are pinned too:
the two fields every consumer reads (`name`, `version`) must stay on the wire
even when they are empty.

## The rules are the daemon's rules, so they are measured rule by rule

The pack's `Validate` decides what may be installed — the name's shape, the
version's shape, that the pack carries something, that each skill's SKILL.md
parses, that each resource path stays inside its bundle, that each MCP server
passes the **contract's own** check. Every one of those refusals is now
measured individually, because a pack that is installed when it should have
been refused is the failure that matters.

## The proof

### Permanent tests

- **The wire shape** (`kernel/contract/marketapi/marketapi_test.go`): the
  exact key set of the pack, the skill it carries, the catalogue entry and
  the catalogue, plus the always-fields.
- **The rules** (same file): `Validate` refuses — rule by rule — a name that
  is not kebab-case, a version that is not semver, an empty pack, a skill
  with an empty or unparseable SKILL.md, a resource that escapes its bundle,
  and an MCP server the contract refuses; the content hash ignores the
  signature (a signature signs the content, so it cannot be part of it); the
  catalogue entry carries the pack's counts and hash; the version comparator
  follows semver — including the malformed-version fallback, where two
  unparseable versions compare as "different strings", never "the same"; the
  resource-path rule; and the skill summary — including the branch where the
  skill has no description and the summary is its name alone.
- **The identity assertions** (`kernel/market/contract_type_test.go`): the
  module's five rows ARE the contract's, and the module's `Library` IS the
  contract's port.
- **One rule, not two** (same file): the module's comparator and the
  contract's agree on every pair of versions, the module's summary and the
  contract's agree on every skill, and the module's patterns ARE the
  contract's.
- **The plugin** (`plugins/builtinmarket/ports_test.go`): the catalogue
  implements the contract's port (a compile-time promise), and its packs are
  built from what the built-ins port hands over.
- **The wiring** (`cmd/agezt/marketlib_test.go`): the catalogue the daemon
  serves is not empty, and it is served through the contract's port.

### Mutation run

**26 mutations: 26 caught.** Two mutations **survived the first round** — the
malformed-version fallback (the test measured only well-formed versions) and
the summary's no-description branch (the test measured only a skill with a
description) — so both tests were strengthened and the mutations were run
again → caught. Three mutations were **first authored as build errors** (a
compile error is not a catch) and one had a stale anchor; all four were
re-authored so the code still compiles, and were run again → caught.

| # | Mutation | Result |
|---|---|---|
| M01 | the pack row's `name` key is renamed | caught — by the wire test |
| M02 | the pack skill row's key is renamed | caught |
| M03 | the catalogue row's `packs` key is renamed | caught |
| M04 | the entry row's skill-count key is renamed | caught |
| M05 | a pack name that is not kebab-case is accepted | caught |
| M06 | a version that is not semver is accepted | caught |
| M07 | an empty pack is accepted | caught |
| M08 | a skill whose SKILL.md does not parse is accepted | caught |
| M09 | a resource that escapes the bundle is accepted | caught |
| M10 | an MCP server the contract refuses is accepted | caught |
| M11 | the signature becomes part of the content it signs | caught |
| M12 | a signed pack reports itself as unsigned | caught |
| M13 | the entry row loses the counts | **build error** at first — re-authored → caught |
| M14 | two unparseable versions compare equal | **survived the first round** — the test measured only well-formed versions; the malformed fallback is measured now → caught |
| M15 | a release no longer outranks its prereleases | caught |
| M16 | an absolute resource path is accepted | caught |
| M17 | an empty segment in a resource path is accepted | **build error** at first — re-authored → caught |
| M18 | the skill summary loses the name | **survived the first round** — the test measured only a skill with a description; the bare branch is measured now → caught |
| M19 | the module's version rule disagrees with the contract | caught |
| M20 | the module's summary disagrees with the contract | caught |
| M21 | the module's name pattern disagrees with the contract | caught |
| M22 | the module's resource-path rule disagrees with the contract | caught |
| M23 | the pack loses the parsed triggers | caught |
| M24 | an explicit version is silently substituted | caught |
| M25 | the catalogue reports no format version | caught |
| M26 | the catalogue entry loses the content hash | **build error** at first — re-authored → caught |

### Gates

The build, gofmt, vet, dead-code, archcheck (**289 packages, 96/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **guardians** still reach into the roster module: they need
  `roster.Validate`, and the validation is not pure — it reads the trust
  ceiling through `kernel/edict`, and a contract package may not import a
  platform package. That is a slice of its own.
- The remaining plugin-reach edges: the database tool, the workboard tool,
  the forgetool, the config tool, the workflow tool, the ACP agent, the
  built-in tools and the built-in channels.
