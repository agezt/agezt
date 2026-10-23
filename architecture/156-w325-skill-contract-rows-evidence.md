# W3.25 — the skill's rows move to the contract layer

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the **skill rows** — the lifecycle state of a reusable procedure,
the record itself, the input that authors one — and the **ports** through
which the agent's skill tool drives them. The tool is the **skill tool**
(`plugins/tools/skilltool`), and it is a plugin: a plugin may reach the
contract and the platform layers and nothing else.

## What moved

| Before | After |
|---|---|
| `kernel/modules/skills/skill` named `Status` (and its five constants), `Metrics`, `Skill` and `CreateSpec` | → `kernel/contract/skillapi` — the rows' new home |
| the module's lifecycle rules (`CanTransition`, `ValidStatus`, `PromoteTarget`) | → the contract's, **with the row**. They are pure — a state machine over five values — so they belong where the row is, and they are the rule the whole daemon enforces |
| the module's `Skill.Active()` (the retrieval-pool predicate) | → the contract's, **with the row** (a method can only be defined in its type's package) |
| the tool's own `forge` interface, naming `skill.CreateSpec`, `skill.Skill`, `skill.Status` and `skill.BundleStore` | → the contract's **ports**: `skillapi.Forge` (author, promote, quarantine, read) and `skillapi.BundleStore` (list, read, dir) |
| the tool reached for the bundle store through the Forge (`f.Bundles()`), i.e. it named the module's concrete store | → the **daemon hands the bundle port over separately**, so the tool never names an implementation |
| `plugin-reach plugins/tools/skilltool -> kernel/modules/skills/skill` | **gone** |

The **Forge**, the store, the content-addressing (`ContentID` — it hashes
with BLAKE3, and a contract package may not import a third-party package)
and the on-disk bundle store stay in the module: they are the implementation,
and the plugin has no business naming them.

The module keeps the names its callers still use as **aliases**
(`skill.Status`, `skill.Skill`, `skill.Metrics`, `skill.CreateSpec`, the five
constants), and its rules as one-line delegations — so there is one row and
one spelling of it.

## The ratchet went DOWN

**288 packages, 98 import violations, 13 call sites** — from 99/13. The
**stale allowlist entry was deleted** — a leftover entry fails the archcheck
gate (the debt was paid, the list must shrink).

## This is the row where the wire matters: the record is persisted

A skill is written to `<home>/skills/skills.json` and read back **by key** —
the CLI, the Web UI and the retrieval layer all read that file. So the row's
JSON keys are not a naming choice: they are a contract with every skill file
already on disk, and a renamed key would silently drop that field. The wire
is now pinned key for key: the record's nine always-fields (the optional ones
must stay **off** the wire when they are empty) and the metrics' three keys.

## The rules are the daemon's rules, so they are measured edge by edge

The lifecycle state machine decides what may be deployed: `draft → shadow →
active`, `active → quarantined`, `quarantined → active` (the un-quarantine),
and `archived` from any non-archived state. It is now measured in the
contract — every legal edge, every illegal edge — and the module's delegations
are measured against the contract **on every pair of states**, because a
second copy of the state machine would drift silently.

## The proof

### Permanent tests

- **The wire shape** (`kernel/contract/skillapi/skillapi_test.go`): the
  record's exact key set, the always-fields that must stay on the wire (a
  missing `status` would read as the empty string), the optional fields that
  must stay off it, and the metrics' keys.
- **The rules, where the row is** (same file): every legal and illegal
  transition, the validity of every persisted state, the promote ladder, the
  retrieval-pool predicate, and the persisted **values** of the five states —
  a renamed state would orphan every stored skill.
- **The identity assertions** (`kernel/modules/skills/skill/contract_type_test.go`):
  the module's four rows ARE the contract's, and the module's Forge and
  bundle store ARE the contract's two ports (compile-time promises).
- **One rule, not two** (same file): the module's three rules and the
  contract's agree on every verdict, over every pair of states.
- **The plugin speaks the contract's rows**
  (`plugins/tools/skilltool/contract_type_test.go`): the tool's own functions
  take the contract's row, its `Bind` takes the contract's two ports, and the
  view the agent reads is pinned key for key.
- **The wiring, twice** (`plugins/builtintools/skill_ports_test.go`): a skill
  the agent learns through the tool lands in the **kernel's own Forge** (so
  the port really carried the live implementation), and the **second** wire —
  the bundle port — really reaches the tool (a resource the daemon attached
  is listed by `op=files`).

### Mutation run

**24 mutations: 24 caught.** One mutation **survived the first round** — the
bundle port was never handed over, and the wiring test only proved the Forge
wire, so the tool would have answered "skill bundles are not available on
this daemon" on every live box while the test stayed green. The wiring test
now proves the second wire as well, and the mutation was run again → caught.
One mutation was an **equivalent mutation** (a rule rewrite that changed
nothing for any input) — it was re-authored into a rule that really disagrees
with the contract, and it was run again → caught.

| # | Mutation | Result |
|---|---|---|
| M01 | the `active` status value is renamed | caught — by the value test |
| M02 | the `quarantined` status value is renamed | caught |
| M03 | the default version is changed | caught |
| M04 | shadow → active is not a legal transition | caught |
| M05 | un-quarantine is not a legal transition | caught |
| M06 | draft → shadow is not a legal transition | caught |
| M07 | a persisted state is no longer valid | caught |
| M08 | promote stops at shadow | caught |
| M09 | the retrieval-pool predicate is wrong | caught |
| M10 | the record's `status` key is renamed | caught — by the wire test |
| M11 | the record's `metrics` key is renamed | caught |
| M12 | the record's `body` key is renamed | caught |
| M13 | the metrics' `uses` key is renamed | caught |
| M14 | the module's lifecycle rule disagrees with the contract | caught |
| M15 | the module's promote rule disagrees with the contract | caught |
| M16 | the module's validity rule disagrees with the contract | **survived the first round** — the first version of this mutation was an *equivalent* mutation (the draft state is valid in the contract too, so the rule was unchanged for every input); it was re-authored into a real disagreement → caught |
| M17 | a learned skill is announced as active | caught |
| M18 | a retired skill is announced as archived | caught |
| M19 | the bundle port is dropped on bind | caught |
| M20 | a promoted skill is announced as active whatever the Forge said | caught |
| M21 | the bundle directory is not reported | caught |
| M22 | the resource content is not published | caught |
| M23 | the bundle port is never handed over | **survived the first round** — the wiring test only proved the Forge wire; the second wire is proved now → caught |
| M24 | the forge port is never handed over | caught |

### Gates

The build, gofmt, vet, dead-code, archcheck (**288 packages, 98/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **marketplace** still reaches into the skills module: it reads the
  **embedded** built-in bundles (the bytes baked into the binary) and parses
  their SKILL.md. The parser is pure and belongs in the contract, but the
  embedded bytes are an implementation — the marketplace needs a port for
  them, and that is a slice of its own.
- The **guardians** still reach into the roster module: they need
  `roster.Validate`, and the validation is not pure — it reads the trust
  ceiling through `kernel/edict`, and a contract package may not import a
  platform package.
- The remaining plugin-reach edges: the database tool, the workboard tool,
  the forgetool, the market plugin, the built-in tools and the built-in
  channels.
