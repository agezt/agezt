# W3.23 — the catalog's rows move to the contract layer

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the **provider catalog's rows** — the description of one provider
and the models it serves. The plugin that names them is the **compatibility
adapter** (`plugins/providers/compat`), which turns a catalog row into a wire
provider, and it is a plugin: a plugin may reach the contract and the platform
layers and nothing else.

## What moved

| Before | After |
|---|---|
| `plugins/providers/compat` named `catalog.Provider`, `catalog.Family`, `catalog.Model` and the family constants | → `kernel/contract/catalogapi` — the rows' new home |
| the module's `FamilyFromNPM` (the NPM → family mapping) | → the contract's, **with the row**. The module no longer uses it, so it keeps no copy of the name — a name nobody calls is dead code, and the dead-code gate says so |
| the module's `FamilySupportsNativeJSONMode` and the credential helpers | → the contract's, with the row. The module keeps the names as one-line delegations, because the CLI, the governor and the app layer still call them |
| the row's methods (`Provider.Family`, `Provider.HasCredentials`, the model's helpers, the cost conversion) | → the contract's, **with the rows** (a method can only be defined in its type's package) |
| `plugin-reach plugins/providers/compat -> kernel/modules/providers/catalog` | **gone** |

The **catalog** itself — the merge of the sources, the sync from models.dev,
the store — stays in the module. It is the implementation, and the plugin has
no business naming it.

## The wire had never been measured, and this is the row where it matters most

The catalog is fetched from **models.dev** and unmarshalled **directly** — the
field names are models.dev's — and it is persisted as `api.json`. So the row's
JSON keys are not a naming choice: they are a contract with an upstream
service and with every catalog file already on disk. A renamed key would
silently drop every provider's models, and the catalog would come back empty.

The wire is now pinned, key for key, for the provider row (7 keys), the model
row (15 keys) and the cost row (the two prices that are always on the wire).

## The ratchet went DOWN

**285 packages, 100 import violations, 13 call sites** — from 101/13. The
**stale allowlist entry was deleted** — a leftover entry fails the archcheck
gate (the debt was paid, the list must shrink).

## The proof

### Permanent tests

- **The wire shape** (`kernel/contract/catalogapi/catalogapi_test.go`): the
  exact key set of the provider, the model and the cost rows, checked against
  the rows' own tags — so the test is checked against the *declaration* and
  not against a copy of it that could drift with it. The always-fields (the
  two prices without `omitempty`) are pinned too.
- **The rules, where the rows are** (same file): the NPM → family mapping
  (including the non-Vercel namespace and the case-insensitivity), the native
  JSON-mode families, the cost conversion (1 USD = 10^9 microcents), the
  credential key shape, and the model's own helpers — the modalities it
  accepts, whether it sees images, whether it caches prompts, whether its tool
  arguments are constrained, and the advisories an operator should see.
- **The identity assertions** (`kernel/modules/providers/catalog/contract_type_test.go`):
  the module's six rows ARE the contract's, as function types.
- **One rule, not two** (same file): the module's names and the contract's
  must agree on every verdict — a second copy of the mapping would drift
  silently, and the mapping decides which wire dialect a provider is spoken
  to.
- **The credential lookup** (`plugins/providers/compat/credential_lookup_test.go`):
  the provider-scoped vault key is found **first**, the bare env var second.
  A provider that only found the bare name would read another provider's
  credential.

### Mutation run

**20 mutations: 18 caught, 2 build errors (recorded separately).** Two
mutations **survived the first round** — the scoped-credential lookup and the
model's vision helper — and both gaps were closed before they were run again.

| # | Mutation | Result |
|---|---|---|
| M01 | the provider's `npm` key is renamed | caught — by the wire test |
| M02 | the model's `release_date` key is renamed | caught |
| M03 | the model's `tool_call` key is renamed | caught |
| M04 | the `groq` family mapping is dropped | caught |
| M05 | the `anthropic` family mapping is wrong | caught |
| M06 | the Ollama family loses its native JSON mode | caught |
| M07 | the cost conversion is off by a thousand | caught |
| M08 | the credential lookup drops the bare env var | caught |
| M09 | the module's family mapping is wrong | caught |
| M10 | the module's provider row drifts from the contract's | **build error** — the identity assertion |
| M11 | the module's credential name is wrong | caught |
| M12 | the compat plugin stops looking up the scoped key | **survived the first round** — the compat suite never covered the scoped lookup; the test was added and M12 was run again → caught |
| M13 | the OpenRouter namespace is unmapped | caught |
| M14 | the model's vision helper always says no | **survived the first round** — the model's helpers had no test of their own; they were measured with the row → caught |
| M15 | the model's prompt-cache helper always says no | caught |
| M16 | the strict-tool-args helper always says no | caught |
| M17 | the tool-use advisory is dropped | caught |
| M18 | the small-context advisory is dropped | caught |
| M19 | the modality helper is stubbed out | **build error** |
| M20 | the scoped key is the only one looked up | caught |

### Gates

The build, gofmt, vet, dead-code, archcheck (**285 packages, 100/13**),
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
