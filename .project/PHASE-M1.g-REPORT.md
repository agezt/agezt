# Phase Report — Milestone 1.g (Catalog-driven wire layer)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §2 (Adapter selection from catalog)](SPEC-15-PROVIDER-ECOSYSTEM.md),
> [TASKS P1-CONDUIT-05](TASKS.md), and the user's "stop hardcoding
> providers/models — everything sync from models.dev/api.json"
> directive.
> Continues [PHASE-M1.f-REPORT.md](PHASE-M1.f-REPORT.md).

## Scope

M1.g closes the loop on the catalog pivot. [PHASE-M1.f-REPORT.md](PHASE-M1.f-REPORT.md)
delivered the **data layer** — a live catalog with base URL, env-var
names, compatibility family, prices, and capabilities for every
provider/model in [models.dev/api.json](https://models.dev/api.json).
M1.g delivers the **wire layer**: the daemon no longer hardcodes
*which* provider Go package to use. Instead, the catalog entry's
compatibility family (derived from its `@ai-sdk/*` npm field) picks
the right adapter, the `api` field supplies the base URL, and the
`env` list resolves credentials.

After M1.g, **adding a new provider is a catalog refresh** — for any
provider whose family is already wired (anthropic, ollama in M1.g;
openai/google/etc. land in M1.h). Operators set
`AGEZT_PROVIDER=<catalog-id>` and optionally `AGEZT_MODEL=<id>`;
the daemon's primary provider is whatever the catalog says it is.

| Concern | M1.g status |
|---|---|
| Daemon selects provider by catalog id, not Go package | ✅ `AGEZT_PROVIDER` is a catalog id |
| Model id comes from catalog entry (override-able) | ✅ `AGEZT_MODEL` overrides `compat.FirstModelID` |
| Base URL comes from catalog `api` field | ✅ via `Provider.BaseURL` |
| Credentials resolved from catalog `env` list | ✅ first-non-empty-wins in `compat.Build` |
| Auto-pick primary when `AGEZT_PROVIDER` unset | ✅ first credentialed + supported entry |
| Local providers (no env list) work | ✅ ollama-local via `custom.json` |
| Governor registry keyed on catalog id, not wire family | ✅ `namedProvider` wrapper |
| Unsupported families fail loudly | ✅ `ErrFamilyUnsupported` with M1.h hint |
| openai / google / mistral / cohere / bedrock | ⏳ M1.h (clear deferral) |
| Browser tool, subscription routing, plugin host | ⏳ M1.h+ |

## Changes

### 1. Wire providers grow a `BaseURL` field

`plugins/providers/anthropic/anthropic.go` and
`plugins/providers/ollama/ollama.go` each gain a `BaseURL` field plus a
`resolveEndpoint()` precedence chain:

```
1. explicit Endpoint (existing behaviour, unchanged)
2. BaseURL + provider-specific path (/v1/messages, /api/chat)
3. package DefaultEndpoint
```

Existing callers that set `Endpoint` directly are untouched — this is
an extension, not a break.

### 2. New `plugins/providers/compat` package

The compatibility-family adapter. Tiny surface:

```go
func Build(p *catalog.Provider, modelID string, lookup CredLookup) (agent.Provider, string, error)
func IsSupportedFamily(f catalog.Family) bool
func FirstModelID(p *catalog.Provider) string
```

Internals:
- Resolves credentials from `p.Env` (first non-empty wins). Local
  families (no env list) skip this entirely — that's how ollama-local
  works.
- Switches on `p.Family()`:
  - `FamilyAnthropic` → builds `anthropic.Provider` with
    `BaseURL=p.API`, `Model=modelID`, `apiKey` from `Env`.
  - `FamilyOllama` → builds `ollama.Provider` with `BaseURL=p.API`,
    `Model=modelID`, no creds.
  - Anything else → `ErrFamilyUnsupported` with `"M1.g supports
    anthropic + ollama; openai/google/etc lands in M1.h"` message.
- Wraps the wire provider in `namedProvider{name: p.ID}` so the
  Governor's registry sees `"anthropic"` / `"ollama-local"` /
  `"groq"`, not `"anthropic"` for every anthropic-family entry.

Error sentinels: `ErrFamilyUnsupported`, `ErrMissingCredentials`,
`ErrModelUnknown`.

### 3. Daemon: `selectPrimary` is now catalog-driven

`cmd/agezt/main.go` no longer imports `plugins/providers/ollama`
directly. Selection precedence:

| `AGEZT_PROVIDER` | Behaviour |
|---|---|
| `mock` | Offline scripted demo (bypasses catalog) |
| `<catalog-id>` (e.g. `anthropic`, `ollama-local`, `groq`) | `cat.Providers[id]` → `compat.Build` |
| (unset) | Auto-pick: first catalog entry whose family is supported AND has creds |
| (no usable entry) | Fall through to mock with a stderr hint |

`AGEZT_MODEL` overrides the model id within the chosen provider;
otherwise `compat.FirstModelID` picks the alphabetically-first model
in the catalog entry.

The daemon pre-loads the catalog from
`<BaseDir>/catalog/{api,local,custom}.json` and passes it into both
`buildGovernor(cat)` and `kernelruntime.Config{Catalog: cat}` so the
selection layer and the pricing layer see the same snapshot.

### 4. Tests

`plugins/providers/compat/compat_test.go` — 11 tests:
- Anthropic family routes to `/v1/messages` with `x-api-key` + `anthropic-version` headers
- Ollama family routes to `/api/chat`
- Unsupported family → `ErrFamilyUnsupported`
- Missing creds → `ErrMissingCredentials` (also with nil lookup)
- Unknown model → `ErrModelUnknown`
- Nil provider → error
- `IsSupportedFamily` enumeration
- `FirstModelID` alphabetical / empty / nil
- Any-of credentials (second env var wins when first is empty)

Full suite: **217 tests pass, 0 fail** (`go test ./... -count=1`).

## Demo transcript

Clean home directory. Build the binaries.

```
$ rm -rf /tmp/agezt-m1g-demo
$ go build -o /tmp/agezt.exe ./cmd/agezt
$ go build -o /tmp/agt.exe ./cmd/agt
```

### Step 1 — start daemon with empty catalog

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo /tmp/agezt.exe
Agezt 0.0.0-m0 — daemon ready (protocol v1)
  governor : primary=mock(offline; auto-picked because catalog had no
             credentialed+supported provider — run `agt catalog sync`
             and set credentials), daily_ceiling=$20.00
```

Auto-pick fell through to the offline mock with a clear hint.

### Step 2 — sync the catalog

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo /tmp/agt.exe catalog sync
{
  "bytes": 2134195,
  "duration_ms": 185,
  "model_count": 4965,
  "provider_count": 136,
  "url": "https://models.dev/api.json"
}
```

136 providers, 4965 models. Confirm via list:

```
$ AGEZT_HOME=/tmp/agezt-m1g-demo /tmp/agt.exe catalog list | head -3
136 providers (synced 2026-05-29T... from https://models.dev/api.json)
  302ai  (302.AI, family=openai-compatible)  [no creds]
    api  : https://api.302.ai/v1
```

### Step 3 — explicit anthropic catalog id (auto-picked model)

```
$ AGEZT_PROVIDER=anthropic ANTHROPIC_API_KEY=fake-key /tmp/agezt.exe
  governor : primary=anthropic(catalog; family=anthropic,
             model=claude-3-5-haiku-20241022) → fallback=mock(offline),
             daily_ceiling=$20.00
```

Provider name = catalog id (`anthropic`, not wire family). Model =
`claude-3-5-haiku-20241022` (alphabetically first in the entry).
Fallback mock auto-registered.

### Step 4 — explicit model override

```
$ AGEZT_PROVIDER=anthropic AGEZT_MODEL=claude-opus-4-7 \
  ANTHROPIC_API_KEY=fake-key /tmp/agezt.exe
  governor : primary=anthropic(catalog; family=anthropic,
             model=claude-opus-4-7) → fallback=mock(offline)
```

`AGEZT_MODEL` wins.

### Step 5 — local-family provider via `custom.json`

models.dev only catalogues hosted providers; local Ollama is added via
the operator-override file:

```
$ cat /tmp/agezt-m1g-demo/catalog/custom.json
{ "ollama-local": {
    "id": "ollama-local", "name": "Ollama (local)",
    "npm": "@ai-sdk/ollama", "api": "http://localhost:11434",
    "models": { "llama3.2": { "id": "llama3.2", "name": "llama3.2" } }
}}
$ AGEZT_PROVIDER=ollama-local /tmp/agezt.exe
  governor : primary=ollama-local(catalog; family=ollama,
             model=llama3.2) → fallback=mock(offline)
```

No env vars needed (empty `env` list ⇒ local family).

### Step 6 — typo handling

```
$ AGEZT_PROVIDER=ollama /tmp/agezt.exe
agezt: AGEZT_PROVIDER="ollama" not in catalog;
run `agt catalog sync` then `agt catalog list`
```

Strict — no silent fallback when the operator is explicit. Hint
points at the discoverability path.

## Architectural consequences

1. **`plugins/providers/{anthropic,ollama}` are now wire-only.** They
   know how to *speak* their dialect; they don't know *when* to be
   used. That decision lives in `compat.Build` driven by catalog
   metadata.

2. **Governor registry is catalog-aligned.** `gov.Why()` reports
   `provider=anthropic` (the catalog id), so `agt catalog list` and
   `agt why <runID>` use the same identifier. No more impedance
   mismatch between "what the user typed" and "what the registry
   stored".

3. **Adding a same-family provider is zero code.** Every
   anthropic-family entry in models.dev (claude.ai-direct, vertex
   passthroughs, bedrock anthropic, etc.) is now selectable by id
   from a fresh `agt catalog sync` — provided the env-var list
   matches one of the user's shell vars.

4. **Adding a new family is one switch case.** M1.h ships the
   openai/google/mistral/cohere/bedrock adapters by adding case
   branches to `compat.Build` + new `IsSupportedFamily` entries.
   No daemon or catalog changes.

5. **Pricing stays honest.** The catalog already feeds the Governor's
   priceFor (M1.f); now the *wire path* and the *price path* both
   originate in the same `*catalog.Provider`. A model's price and its
   adapter can never drift.

## Deferrals → M1.h

Explicitly scoped out of M1.g, with clear error paths today:

- **openai / openai-compatible** — every catalog entry returns
  `ErrFamilyUnsupported`; the auto-picker skips them.
- **google / mistral / cohere / bedrock / azure** — same.
- **Subscription-first routing** (DECISIONS C2) — Governor still
  picks on cost; subscription-prefer logic waits for the auth-mode
  expansion.
- **`agt provider creds`** CLI for managing credentials in a vault.
- **Browser tool, plugin host, Pulse v1, planner** — unchanged
  deferral from M1.f.

## Files touched

```
plugins/providers/anthropic/anthropic.go   (+ BaseURL field, resolveEndpoint)
plugins/providers/ollama/ollama.go         (+ BaseURL field, resolveEndpoint)
plugins/providers/compat/compat.go         NEW
plugins/providers/compat/compat_test.go    NEW (11 tests)
cmd/agezt/main.go                         (selectPrimary catalog-driven;
                                            buildGovernor takes *catalog.Catalog;
                                            buildFromCatalog helper;
                                            ollama import removed)
```

No schema changes; no journal-replay implications; existing event
kinds untouched.

## Verification

```
$ go build ./...           # clean
$ go test ./... -count=1   # 217 pass, 0 fail
$ /tmp/agt.exe journal verify --home /tmp/agezt-m1g-demo
ok: 0 entries, head=GENESIS
```

(Demo daemons in this report didn't dispatch any runs — the catalog
selection paths are observable from the boot banner alone, and live
LLM dispatch is gated on real credentials.)
