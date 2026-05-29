# Phase Report — Milestone 1.f (Catalog v1: live provider/model sync)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-15 §1 (Provider & model catalog sync)](SPEC-15-PROVIDER-ECOSYSTEM.md),
> [TASKS P1-CONDUIT-04](TASKS.md), and the user's "stop hardcoding
> providers/models — everything sync from models.dev/api.json"
> directive.
> Continues [PHASE-M1.e-REPORT.md](PHASE-M1.e-REPORT.md).

## Scope

This is the **first half** of the bigger pivot the user asked for:
*everything flexible, catalog-driven, no hardcoded providers/models*.
M1.f ships the **data layer**: a live catalog of every provider and
model (base URL, env-var names, compatibility family, prices,
modalities, context windows, tool/reasoning support), synced from
[models.dev/api.json](https://models.dev/api.json) and merged with
auto-discovered local services (Ollama). The Governor's pricing now
reads from the catalog with the old hardcoded table as fallback.

M1.g (next sub-phase) will do the **wire layer**: replace the
hardcoded `plugins/providers/{anthropic,ollama}` Go packages with a
generic compatibility-family adapter driven entirely by
`catalog.Provider` metadata (npm field → adapter selection, env →
credentials, api → base URL). After M1.g, adding a new provider is a
catalog refresh, not a code change.

| Concern | M1.f status |
|---|---|
| Provider list, models, prices, capabilities | ✅ catalog-driven (models.dev) |
| Local Ollama discovery merged into catalog | ✅ `agt catalog discover` |
| Custom operator overrides win over remote | ✅ `custom.json` |
| Governor pricing reads catalog | ✅ live; fallback table for first-boot |
| Provider selection / wire dialect | ⏸ M1.g — still hardcoded packages today |

## What shipped

### New package: `kernel/catalog` (1061 LoC, 16 tests)

| File | LoC | Role |
|---|---:|---|
| `types.go` | 310 | `Provider`, `Model`, `Cost`, `Modalities`, `Limit`, `Catalog` (merge, FindModel, ProviderList), `Family` (`FamilyFromNPM` derives wire-dialect from npm field) |
| `store.go` | 158 | `Store` over `<BaseDir>/catalog/` with api.json / local.json / custom.json + meta sidecar; atomic writes; merge order custom > local > api |
| `sync.go` | 107 | `Syncer` over models.dev/api.json (configurable URL); 30s timeout, 8 MiB body cap; produces raw bytes + parsed Catalog + SyncResult |
| `discovery.go` | 106 | `DiscoverOllama` against `/api/tags`; synthesises an `ollama-local` Provider with one Model per installed tag (Cost=nil → free) |
| `catalog_test.go` | 380 | 16 tests: parse, family-detect, microcent conversion, credential check, FindModel (qualified + bare), merge precedence, store roundtrip, custom-overrides-api on load, sync fixture, 502-rejection, Ollama discovery |

Public surface:

```go
type Provider struct {
    ID, Name, NPM, API, Doc string
    Env                     []string          // any-of credential env-vars
    Models                  map[string]*Model // keyed by model id
}
func (p *Provider) Family() catalog.Family
func (p *Provider) HasCredentials(lookup func(string) string) bool

type Model struct {
    ID, Name, Family            string
    ToolCall, Reasoning,
    Attachment, OpenWeight      bool
    Modalities                  Modalities
    Limit                       Limit       // context, output, input windows
    Cost                        *Cost       // nil → free/local
}
func (c *Cost) InputMicrocentsPerMTok() int64   // → governor's unit
func (c *Cost) OutputMicrocentsPerMTok() int64

type Catalog struct {
    Providers map[string]*Provider
    SyncedAt  time.Time
    Sources   []string
}
func (c *Catalog) FindModel(modelID string) (*Provider, *Model)  // "anthropic/claude-opus-4-7" or just "claude-opus-4-7"

type Store struct{ Dir string }
func (s *Store) Load() (*Catalog, error)
func (s *Store) SaveAPI(raw []byte, sourceURL string) error
func (s *Store) SaveLocal(c *Catalog, source string) error

type Syncer struct{ HTTP *http.Client; URL string; Timeout time.Duration }
func (s *Syncer) Sync(ctx) (raw []byte, c *Catalog, res SyncResult, err error)

func DiscoverOllama(ctx, endpoint string) (*Catalog, error)
```

### Family detection from the `npm` field

The models.dev catalog tags each provider with a `@ai-sdk/*` package
name. We repurpose that as the wire-dialect family hint:

| `npm` field | Family |
|---|---|
| `@ai-sdk/anthropic` | `FamilyAnthropic` |
| `@ai-sdk/openai` | `FamilyOpenAI` |
| `@ai-sdk/openai-compatible` | `FamilyOpenAICompatible` |
| `@ai-sdk/google` (or `-generative-ai` / `-vertex`) | `FamilyGoogle` |
| `@ai-sdk/ollama` | `FamilyOllama` |
| `@ai-sdk/mistral`, `cohere`, `amazon-bedrock`, `azure` | matching |
| anything else | `FamilyUnknown` |

M1.g will key the generic wire adapter on this family, so
"register provider X" stops requiring a per-provider Go package.

### On-disk layout under `<BaseDir>/catalog/`

```
api.json        most-recent remote sync (models.dev/api.json shape)
local.json      auto-discoveries (Ollama /api/tags etc.)
custom.json     operator-curated overrides; wins over local + api
meta.json       sync timestamps, source URL, sizes
```

Loader merges in api → local → custom order with later overriding
earlier. `agt catalog sync` only writes `api.json`, never clobbering
local discoveries or hand-edited overrides.

### Pricing override (Governor)

```go
// kernel/governor/pricing.go (M1.f)
var liveCatalog atomic.Pointer[catalog.Catalog]

func SetCatalog(c *catalog.Catalog) { liveCatalog.Store(c) }

func priceFor(model string) modelPrice {
    if c := liveCatalog.Load(); c != nil {
        if _, m := c.FindModel(model); m != nil && m.Cost != nil {
            return modelPrice{
                InputMicrocentsPerMTok:  m.Cost.InputMicrocentsPerMTok(),
                OutputMicrocentsPerMTok: m.Cost.OutputMicrocentsPerMTok(),
            }
        }
    }
    /* ... existing hardcoded fallback ... */
}
```

Atomic pointer = lock-free hot path. The Governor's existing
`costMicrocents(model, in, out)` is untouched; pricing data has just
moved from a Go literal to a swap-in catalog. Hardcoded table
survives only so first-boot (pre-sync) still produces sane numbers
for the known set.

### Runtime integration

`kernel/runtime`:
- `Config.CatalogDir` (defaults to `<BaseDir>/catalog`).
- `runtime.Open` loads the on-disk catalog and installs it into the
  Governor via `governor.SetCatalog(cat)` — every subsequent
  `Complete` reads live prices.
- `Kernel.Catalog()`, `Kernel.CatalogStore()`, `Kernel.ReloadCatalog()`
  accessors for the control plane and future planners.

`kernel/event/kinds.go` adds:
- `catalog.synced` / `catalog.sync_failed`
- `catalog.discovery_completed` / `catalog.discovery_failed`

Every sync + every discovery lands in the BLAKE3-chained journal so
"why did the price for model X change at 14:23?" has an exact
auditable answer.

### Control plane + `agt` CLI

```
CmdCatalogSync     "catalog_sync"      args: {url?, timeout_s?}
CmdCatalogList     "catalog_list"      args: -
CmdCatalogDiscover "catalog_discover"  args: {endpoint?}
```

New `agt` subcommands:

```
agt catalog sync [url]            # pull from models.dev (or url override)
agt catalog list                  # show providers + models + prices + creds
agt catalog discover [endpoint]   # auto-discover Ollama at the endpoint
```

`agt catalog list` shows each provider's family, base URL, env-var
list, credential-present flag, and per-model prices in `$X.XX / $Y.YY
per MTok` form so the operator can eyeball "what can I actually
call?" at a glance.

## Demo transcript

Live e2e against the real models.dev catalog (no fixture):

```
$ rm -rf /tmp/agezt-m1f && mkdir -p /tmp/agezt-m1f
$ AGEZT_HOME=/tmp/agezt-m1f AGEZT_PROVIDER=mock ./bin/agezt &

Agezt 0.0.0-m0 — daemon ready (protocol v1)
  base dir         : /tmp/agezt-m1f
  governor         : primary=mock(offline; scripted shell+final), daily_ceiling=$20.00
  tools            : shell(warden=requested-namespace), file(...), http(hosts=0)
  policy engine    : edict (defaults from DECISIONS F3; AskAllow)
  warden           : requested=namespace, effective=none (M1.c facade; downgrades journaled)
  control plane    : 127.0.0.1:58294

$ ./bin/agt catalog sync
{
  "bytes": 2134195,
  "duration_ms": 197,
  "model_count": 4965,
  "provider_count": 136,
  "url": "https://models.dev/api.json"
}

# Daemon log:
[evt seq=0 kind=catalog.synced  subject=catalog.sync]  ← NEW (M1.f)

$ ./bin/agt catalog list | head -40
136 providers (synced 2026-05-29T06:56:09.776645Z from https://models.dev/api.json)

  anthropic  (Anthropic, family=anthropic)  [no creds]
    env  : ANTHROPIC_API_KEY
    24 model(s):
      claude-3-5-haiku-20241022                 $0.80 / $4.00 per MTok
      claude-3-5-sonnet-20240620                $3.00 / $15.00 per MTok
      claude-3-7-sonnet-20250219                $3.00 / $15.00 per MTok
      claude-3-opus-20240229                    $15.00 / $75.00 per MTok
      claude-haiku-4-5                          $1.00 / $5.00 per MTok
      claude-opus-4-1                           $15.00 / $75.00 per MTok
      claude-opus-4-5                           $5.00 / $25.00 per MTok
      claude-opus-4-7                           $5.00 / $25.00 per MTok
      claude-sonnet-4-5                         $3.00 / $15.00 per MTok
      claude-sonnet-4-6                         $3.00 / $15.00 per MTok
      …

  google  (Google, family=google)  [no creds]
    env  : GOOGLE_API_KEY, GOOGLE_GENERATIVE_AI_API_KEY, GEMINI_API_KEY
    …
```

**The catalog already corrected drift in the hardcoded fallback table:**

| Model | Fallback table (M1.b) | Catalog (live, M1.f) |
|---|---|---|
| `claude-opus-4-7` | $15 / $75 per MTok | **$5 / $25 per MTok** |
| `claude-haiku-4-5` | $0.80 / $4.00 per MTok | **$1.00 / $5.00 per MTok** |

Every subsequent `agt run` against an Anthropic primary now records
`budget.consumed` events with the correct numbers — and the operator
can `agt catalog sync` again at any time to pull fresh prices.

`agt journal verify` returns `{"ok": true}` across all this — the
catalog.synced event is just another link on the same BLAKE3 chain.

## Verified invariants

| Invariant | Test |
|---|---|
| models.dev shape parses verbatim into Catalog | `TestParseAPIFile` |
| `@ai-sdk/*` → Family detection covers every shipping family | `TestFamilyFromNPM` |
| USD/MTok → microcents conversion: $5 → 5×10⁹ | `TestCostMicrocentsConversion` |
| nil Cost (local models) → 0, never panics | `TestCostNilIsZero` |
| Provider with no env list (local) is always credentialed; otherwise any-of | `TestHasCredentials` |
| `FindModel("anthropic/claude-opus-4-5")` qualified lookup works | `TestFindModel_QualifiedID` |
| `FindModel("solar-mini")` bare lookup finds it under upstage | `TestFindModel_BareID` |
| Missing or empty model id returns (nil, nil) | `TestFindModel_Missing` |
| custom.json merged over api.json wins for fields it sets; preserves the rest | `TestMerge_LocalOverridesAPI` |
| Empty store loads cleanly as empty Catalog (no error) | `TestStore_LoadEmpty` |
| SaveAPI → Load roundtrip preserves providers + writes meta | `TestStore_SaveAPIThenLoad` |
| custom.json on disk overrides api.json on load | `TestStore_CustomOverridesAPIOnLoad` |
| Syncer hits HTTP + parses + reports counts | `TestSyncer_FetchesAndParses` |
| Syncer surfaces non-200 as error (no partial write) | `TestSyncer_RejectsNon200` |
| Ollama `/api/tags` parses into synthesised provider + free models | `TestDiscoverOllama_ParsesTags` |
| Discovery against absent Ollama returns error (non-fatal upstream) | `TestDiscoverOllama_AbsentReturnsError` |
| Live catalog overrides hardcoded fallback in Governor pricing | `TestPricing_CatalogOverridesFallbackTable` |
| Empty catalog still falls back to hardcoded table for known models | `TestPricing_CatalogMissingFallsBackToTable` |

18 new tests (16 catalog + 2 governor pricing). Existing 180 tests
unaffected. Total module: **198 passing tests** across **26 packages**,
vet clean, depscheck clean.

## Cumulative status

```
26 packages | ~13,400 lines source+tests | 198 tests passing | 2 deps (allowlisted)
```

| Subsystem | LoC | Tests |
|---|---:|---:|
| `kernel/{ulid,event,journal,state,bus,agent,runtime,controlplane}` | ~4,800 | 65 |
| `kernel/edict` | ~600 | 16 |
| `kernel/governor` | ~950 | 14 |
| `kernel/warden` | 726 | 9 |
| `kernel/approval` | 578 | 8 |
| `kernel/scheduler` | 1,083 | 12 |
| `kernel/catalog` | **1,061** | **16** |
| `plugins/providers/{mock,anthropic,ollama}` | 1,034 | 13 |
| `plugins/tools/{shell,file,http}` | ~1,360 | 35 |
| `cmd/{agezt,agt}` | ~1,180 | 8 |
| `internal/{brand,paths}` | 102 | 1 |
| `tools/{jsonschemagen,depscheck}` | 633 | (jsonschemagen: 3 + e2e) |

## Deviations from spec (intentional)

1. **Hardcoded provider packages still drive the wire.** This phase
   ships the *data* layer only. `plugins/providers/anthropic`,
   `plugins/providers/ollama`, and the `AGEZT_PROVIDER` switch in
   `cmd/agezt/main.go` are unchanged. M1.g replaces them with a
   single `plugins/providers/compat` package that reads
   `catalog.Provider.Family()` and routes accordingly. After M1.g,
   `AGEZT_PROVIDER` becomes a catalog provider-id selector, not a
   compile-time switch.
2. **No HTTP catalog cache headers.** Every sync is a fresh GET; we
   don't honour `ETag` / `If-Modified-Since`. models.dev is small
   enough (~2 MiB) that the operator-driven sync cadence (manual or
   `agt catalog sync` on a cron) doesn't need it. Add if real-world
   usage shows we're hitting the bandwidth budget.
3. **No signature verification.** SPEC-15 §1.5 mentions "signed,
   content-addressed catalog snapshots" as a future supply-chain
   defense. M1.f trusts whatever models.dev returns over HTTPS.
   Local + custom files are operator-owned; the daemon only writes
   `api.json` itself.
4. **Discovery is Ollama-only.** lm-studio, vllm, jan.ai, llama.cpp
   server all expose introspection endpoints that could feed
   `local.json`; we ship the canonical one (`Ollama /api/tags`) and
   leave the rest for when there's a real ask.
5. **No `tiered` cost support.** Some providers (Anthropic
   prompt-caching, OpenAI batch) publish multi-tier pricing
   (`cost.tiers[]`). We extract `cost.input`, `cost.output`,
   `cost.cache_read`, `cost.cache_write`; tier-based pricing maps to
   the base rate. Honouring tiers needs request-time metadata the
   Governor doesn't track yet.
6. **No periodic background sync.** Sync is operator-triggered (`agt
   catalog sync` or a cron job around it). A daemon-side scheduler
   that auto-refreshes every N hours lands with Chronos (Pulse
   subsystem).

## Open items for M1.g

- **Generic compat provider** (`plugins/providers/compat`) — single
  Go package that reads `catalog.Provider` (family, api, env) and
  routes to anthropic-shape / openai-shape / openai-compatible /
  google-shape / ollama-shape adapters. Old per-provider packages
  shrink to thin re-exports during transition, then go away.
- **`AGEZT_PROVIDER` rework** — accepts any catalog provider id
  (`anthropic`, `openai`, `groq`, `cerebras`, `ollama-local`, …) and
  any catalog model id. Default behaviour: pick the cheapest
  credentialed provider whose family we can speak.
- **Subscription-first routing** (DECISIONS C2) — Governor reads
  catalog metadata + per-provider "subscription" credential to apply
  the subscription → quality → cost → latency selector.
- **`agt provider creds`** — surfaces which env vars are set for which
  providers so operators can see "Anthropic is missing because
  ANTHROPIC_API_KEY isn't set" at a glance.

## Pointers

- Tests: `go test ./...` (198 pass, vet clean, depscheck OK)
- Sync demo (real fetch, ~2 MiB, sub-second):
  ```
  AGEZT_HOME=/tmp/d AGEZT_PROVIDER=mock ./bin/agezt &
  ./bin/agt catalog sync          # → 136 providers, 4965 models
  ./bin/agt catalog list | head   # browse + see credential flags
  ./bin/agt catalog discover      # add local Ollama if running
  ```
- Override / customize:
  ```
  # Pin a custom price for one model (wins over live sync):
  cat > $AGEZT_HOME/catalog/custom.json <<'EOF'
  {"anthropic":{"id":"anthropic","models":{
    "claude-opus-4-7":{"id":"claude-opus-4-7","cost":{"input":3.5,"output":18}}
  }}}
  EOF
  # Restart daemon; the override applies on load.
  ```
- Self-hosted catalog source: `AGEZT_CATALOG_URL=https://your.host/catalog.json`
  during daemon startup or `agt catalog sync https://your.host/catalog.json`
  for a one-shot.
- Next milestone report: `PHASE-M1.g-REPORT.md` (kill the hardcoded
  provider packages)
