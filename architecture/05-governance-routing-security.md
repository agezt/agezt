# 05 — Governance, Routing & Security

**Scope:** `kernel/governor`, `kernel/catalog`, `kernel/creds` (+ `kernel/creds/sigv4`), `kernel/chatgptauth`, `kernel/settings`, `kernel/configcenter`, `kernel/executionprofile`, `kernel/edict`, `kernel/warden`, `kernel/netguard`, `kernel/envscrub`, `kernel/redact`, `kernel/approval`, `kernel/seat`, `kernel/tenant`, `kernel/tenantctx`, `kernel/state`.

Sibling docs: [00-README.md](00-README.md) · [01-daemon-boot-cmd-agezt.md](01-daemon-boot-cmd-agezt.md) (boot wiring that builds these objects) · [03-control-plane-and-http.md](03-control-plane-and-http.md) (HTTP/control-plane surfaces over them) · [04-agent-runtime.md](04-agent-runtime.md) (`policyHook`, run context setters) · [06-data-memory-state.md](06-data-memory-state.md) (bus/journal that receives the events) · [07-autonomy-and-extensibility.md](07-autonomy-and-extensibility.md) (standing orders that apply trust ceilings) · [08-providers.md](08-providers.md) (providerboot builds the governor; provider retry/Retry-After) · [10-tools.md](10-tools.md) (tools that call warden/netguard/envscrub).

All source read from the working tree (disk), 2026-10-02.

---

## Responsibilities at a glance

| Concern | Package(s) | One-line role |
|---|---|---|
| Model routing + spend | `governor` | Implements `agent.Provider`; every LLM call: response cache → model-chain walk → pre-flight cascade (overrides, capability, rate, budgets, pricing) → provider chain (cost-ordered, breaker-filtered) → in-place retry → fallback → usage/cost accounting in integer microcents. |
| Model/provider data | `catalog` | models.dev-shaped registry merged from `api.json` < `local.json` < `custom.json`; prices, capabilities (tool_call, vision, strict args, JSON mode family), Ollama discovery, SSRF-guarded sync. |
| Credentials | `creds`, `creds/sigv4` | `creds.json` vault (AES-256-GCM, PBKDF2, machine-bound by default), multi-key keyring (`NAME#label`), lookup chaining, full AWS credential chain (env/shared file/credential_process/IMDSv2/STS AssumeRole/SSO/Web identity), SigV4 signer. |
| ChatGPT subscription OAuth | `chatgptauth` | PKCE + token refresh for the unofficial Codex/ChatGPT OAuth client; token blob stored as one vault secret `AGEZT_CHATGPT_OAUTH`. |
| Operator config | `settings` | `config.json` (non-secret AGEZT_* values, account-keyed) + schema registry (built-in sections + `schemas/*.json` plugin-registered sections) driving the console's "Config Center" UI. |
| Agent-facing config KV | `configcenter` | Separate rated key/value store (public/internal/restricted/secret) with per-agent ACLs, rate limits, HITL for restricted keys, JSONL audit. Not the same thing as the UI "Config Center". |
| Execution surfaces | `executionprofile`, `seat` | Inventory of isolation profiles (local, warden, docker, ssh, remote-agezt, modal, daytona, k8s, …), env/secret passthrough policy, health diagnosis; seats = task-facing presets (model chain / tool tier / isolation). |
| Policy | `edict`, `approval` | Capability-based trust ladder L0..L4 + immutable hard-deny floor + AskPolicy; HITL approval queue with timeout-deny. |
| Sandboxing / egress / env / secrets-in-logs | `warden`, `netguard`, `envscrub`, `redact` | Process exec with timeouts/output caps/empty-env default (+Linux rlimits/pgid, optional docker backend); dial-time IP egress guard; secret-free child env; journal secret scrubber. |
| Multi-tenancy | `tenant`, `tenantctx` | Per-tenant base dir + lazily opened kernel + persistent tenant token; tenant id carried on run ctx. |
| Mutable kernel state | `state` | Namespaced JSON KV (`<base>/state/<ns>.json`), atomic rewrite per mutation. |

---

## Import graph (in-scope packages)

| Package | Depends on (internal) | Imported by |
|---|---|---|
| `kernel/governor` | kernel/agent, kernel/bus, kernel/catalog, kernel/event | cmd/agezt, cmd/agt, kernel/controlplane, kernel/runtime, plugins/providerboot |
| `kernel/catalog` | internal/atomicfile, kernel/netguard | cmd/agezt, cmd/agt, cmd/agt/providerlookup, kernel/cadence/systemtasks, kernel/controlplane, kernel/governor, kernel/runtime(+accessors, runexec), plugins/providerboot, plugins/providers/compat |
| `kernel/creds` | internal/atomicfile, internal/strutil, kernel/creds/sigv4, kernel/envscrub | cmd/agezt, cmd/agt, cmd/agt/providerlookup, kernel/chatgptauth, kernel/controlplane, kernel/executionprofile, plugins/tools/config |
| `kernel/creds/sigv4` | — | cmd/agezt, kernel/creds, plugins/providers/bedrock |
| `kernel/chatgptauth` | internal/brand, kernel/creds, kernel/netguard | kernel/controlplane, plugins/providerboot |
| `kernel/settings` | internal/atomicfile | cmd/agezt, kernel/channelwire, kernel/controlplane, plugins/tools/config, plugins/tools/overseertool |
| `kernel/configcenter` | kernel/approval | cmd/agt, kernel/agentgw, kernel/controlplane, kernel/runtime(+accessors, runexec) |
| `kernel/executionprofile` | kernel/creds, kernel/warden | kernel/controlplane, plugins/tools/codeexec, plugins/tools/shell |
| `kernel/edict` | internal/atomicfile | ~45 importers: cmd/agezt, daemonconfig, agentgw, controlplane, delegation, imagetool, market, memory, reranktool, roster, runtime(+accessors, compose, runexec), voicetool, worldmodel, and essentially every `plugins/tools/*` package |
| `kernel/warden` | kernel/bus, kernel/event | cmd/agezt, kernel/controlplane, kernel/executionprofile, kernel/pulse, kernel/runtime(+accessors, compose, runexec), kernel/toolreg, plugins/tools/codeexec, plugins/tools/shell |
| `kernel/netguard` | — | cmd/agezt, cmd/agt, catalog, chatgptauth, controlplane, market, mcp, update, plugins/channels/onebot, plugins/external/mcpbridge, plugins/providers/{embed,openairesponses,voice}, plugins/tools/{browser,fetch,http,websearch} |
| `kernel/envscrub` | — | kernel/creds, plugins/tools/{acpagent,browser,coding} |
| `kernel/redact` | — | cmd/agezt, kernel/controlplane, kernel/openaiapi, plugins/builtintools |
| `kernel/approval` | kernel/bus, kernel/event, kernel/ulid | kernel/configcenter, kernel/controlplane, kernel/runtime(+accessors, runexec), kernel/scheduler, plugins/tools/forgetool |
| `kernel/seat` | internal/atomicfile | kernel/controlplane, kernel/runtime |
| `kernel/tenant` | — | cmd/agezt, kernel/controlplane |
| `kernel/tenantctx` | — | kernel/runtime, plugins/tools/peer |
| `kernel/state` | internal/atomicfile | cmd/agezt, kernel/pulse, kernel/runtime(+accessors, runexec) |

Layering notes: `governor` imports `kernel/agent` (it *is* an `agent.Provider`) and `kernel/catalog` (pricing only; capability lookups are injected as funcs to stay decoupled). `edict` and `netguard`, `envscrub`, `redact`, `tenant`, `tenantctx`, `sigv4` are leaf packages. `edict` is the most widely imported kernel package in this area because tools declare their capability via `edict.Capability`.

---

## AGEZT_HOME persistence map (this area)

| Path under `<baseDir>` | Owner | Format / perms |
|---|---|---|
| `creds.json` | creds | Plaintext `{NAME: value}` map **or** encrypted envelope `{"schema":"agezt-creds-v2", "encryption":"aes-256-gcm", "kdf":"pbkdf2-hmac-sha256", "kdf_iter":200000, "kdf_salt", "nonce", "ciphertext"}` (base64 fields). 0600, atomic via `internal/atomicfile`. **Merge-on-save** (W1.3): `Save`/`Rotate` take `filestore.Lock`, re-read the file with the passphrase that opened it, apply only this Store's pending `Set`/`Remove`s, write, and adopt the merged map — so the daemon and `agt` no longer delete each other's keys, and a Store that cannot decrypt the file refuses to overwrite it. |
| `config.json` | settings | `{account: {AGEZT_X: value}}` (`_default` account); legacy flat map accepted on load; UTF-8 BOM stripped. 0600. **Merge-on-save** (W1.3): `Save` takes `filestore.Lock`, re-reads the file and applies only this Store's pending changes, so the per-request `NewStore→Load→Set→Save` handlers and `agt config` no longer revert each other. |
| `schemas/<id>.json` | settings.Registry | One registered `Section` per file. 0600. |
| `catalog/api.json`, `local.json`, `custom.json`, `meta.json` | catalog | models.dev-shaped JSON; meta sidecar. 0644. |
| `configcenter/entry_<sha256(key)[:16]>.json` | configcenter | Full `ConfigEntry` **including raw `value`**, 0644, plain `os.WriteFile` (not atomic). |
| `configcenter/audit_YYYY-MM-DD.jsonl` | configcenter | One `AuditEntry` per line, 0644, O_APPEND. |
| `runtime/edict_overlay_snapshot.json` | edict (written by controlplane compact handler) | `OverlaySnapshot{through_seq, changes[]}`, 0644, atomic. Trusted at boot only if its `ContentHash()` matches the latest journaled `policy.compacted`. |
| `seats/seats.json` | seat | `{version:1, seats:[custom...]}` (built-ins live in code). |
| `state/<ns>.json` | state | `{key: rawJSON}` per namespace, 0644, atomic; file removed when namespace empties. |
| `tenants/<id>/` | tenant | A full per-tenant base dir (journal, state, vault, …) + `.tenant-token` (32-byte hex, 0600, O_EXCL mint). |
| `sandbox/…`, secret-file mount dirs | executionprofile (`PrepareSecretFileMounts`) | Temp files of vault secrets written 0600 for a profile run, removed by the returned cleanup. |

---

# Part A — Model routing (`kernel/governor`, `kernel/catalog`)

## A.1 `kernel/governor`

**Purpose.** The LLM routing brain. `*Governor` implements `agent.Provider` (`Name()=="governor"`, `Complete`) and `agent.StreamingProvider` (`CompleteStream`), so the agent loop calls it like any provider while it transparently applies model-chain fallback, provider ordering, gates, retries, breaker and accounting.

**Key exported API**

- Types: `Governor`, `Config`, `Registry`, `ProviderInfo{Name, Provider, AuthMode, IsFallback, Models}`, `AuthMode` (`AuthSubscription`, `AuthLocal`, `AuthAPIKey`), `TaskRoutes`, `TaskRouteRequires`, `TaskModelOverrides`, `TaskModelChains`, `BudgetSnapshot`, `TaskBudgetSnapshot`, `ErrNoProviders{Tried, Last}`, `ErrNoModelConfigured{TaskType}`.
- Constructors / lifecycle: `New(Config)`, `NewRegistry()`, `(*Governor).Replace(*ProviderInfo)` (hot reload; rebuilds cached chains), `SetBus`, `WithDailyCeiling`, `WithLimits(ceiling, ratePerMin)` (per-tenant sibling).
- Live knobs: `SetTaskModelChains`, `SetFallbackChains(chains, default)`, `SetDailyCeiling`.
- Views: `Providers`, `ProviderHealth` (breaker state per provider), `TaskRoutesView`, `TaskRouteRequiresView`, `TaskModelOverridesView`, `TaskModelChainsView`, `FallbackChainsView`, `Snapshot`, `SpentMicrocents`, `SpentByTaskMicrocents`, `SpentByAgentMicrocents`, `DailyCeilingMicrocents`, `StrictPricingEnabled`, `UsageFor(corr)`.
- Package funcs: `SetCatalog(*catalog.Catalog)` (global `atomic.Pointer` pricing source), `CostMicrocents`, `ModelIsPriced`, parsers `ParseTaskRoutesEnv`, `ParseTaskModelOverridesEnv`, `ParseTaskModelChainsEnv`, `ParseFallbackChainsEnv`, `ParseTaskBudgetsEnv`.
- Constants: `DefaultDailyCeilingMicrocents` (= $20/day = 20·10⁹ microcents), `DefaultProviderRetries=2`, `DefaultRetryBaseDelay=500ms`, `DefaultBreakerThreshold=5`, `DefaultBreakerCooldown=30s`, `DefaultResponseCacheSize=256`, `ChainPrefix="@"`.
- Sentinel errors: `ErrBudgetExceeded` (and wrapped `ErrTaskBudgetExceeded`, `ErrAgentBudgetExceeded`), `ErrUnpricedModel`, `ErrRateLimited`, `ErrModelLacksToolUse`, `ErrModelUnservable`, `ErrStreamInterrupted`, `ErrAlreadyRegistered`.

**Units.** All money is `int64` microcents (1 USD = 10⁹ microcents; prices are microcents per million tokens). Cost math uses saturating multiply/add (`bits.Mul64`) and clamps negative token counts → a hostile usage report can only over-charge, never credit the ledger.

### A.1.1 Request lifecycle (Complete / CompleteStream)

```mermaid
flowchart TD
  A[agent loop: Complete(req)] --> C{respCache enabled\nand exact hit?}
  C -- hit --> CH[journal routing.decision cache=hit\nreturn cached resp; no rate slot, no spend]
  C -- miss/disabled --> M[completeChained: resolve model list]
  M --> M1{req.ModelChain set?\n(per-agent / seat / run)}
  M1 -- yes --> L[models = req.ModelChain\nscope=agent-chain]
  M1 -- no --> M2{taskModelChains[req.TaskType]?}
  M2 -- yes --> L2[models = task chain\nscope=model-chain]
  M2 -- no --> M3{defaultChain set?}
  M3 -- yes --> L3[models = fallbackChains[default]\nscope=default-chain]
  M3 -- no --> E0[models = empty]
  L --> X[expandChains: '@name' -> named chain models,\nflatten, dedupe, unknown names dropped]
  L2 --> X
  L3 --> X
  E0 --> X
  X --> Z{len(models)==0?}
  Z -- yes, req.Model empty --> ERR1[ErrNoModelConfigured]
  Z -- yes, req.Model set --> ONE[runOne(req)]
  Z -- no --> W[for each model m]
  W --> U{modelKnownUnservable(m)?\nevery provider declares Models\nand none lists m}
  U -- yes --> SKIP[provider.fallback skipped=true; next model]
  U -- no --> ONE2[runOne(req with Model=m)]
  ONE --> PF
  ONE2 --> PF[preflightAndRoute]
  PF --> PRE[runPreflight cascade - see A.1.2]
  PRE -- refuse --> RET[error; budget/rate errors are terminal]
  PRE -- ok --> RC[routeChain - see A.1.3]
  RC --> RD[journal routing.decision primary/chain/task_model/task_type]
  RD --> RUN[runChain: openChain breaker filter;\nper provider callWithRetry]
  RUN -- success --> REC[recordUsage: cost, ledgers,\nbudget.unpriced?, budget.consumed;\nbreaker success -> provider.breaker_closed]
  RUN -- fallback-able error --> BR[breaker.failure -> provider.breaker_open?\nprovider.fallback; next provider]
  BR --> RUN
  RUN -- all failed --> NP[ErrNoProviders{Tried, Last}]
  NP --> W2{shouldFallback(err)?}
  W2 -- yes --> PFB[provider.fallback failed_model/next_model] --> W
  W2 -- no --> RET
```

Notes:
- **Cache** (`cache.go`): opt-in (`AGEZT_LLM_CACHE_TTL`), TTL'd LRU keyed by SHA-256 of JSON `{Model, System, Messages, Tools, MaxTokens, JSONMode, TaskType, Params, ProviderOptions}`. Only `Complete` reads/writes it; `CompleteStream` never uses it. The cache key uses the *pre-chain* `req.Model`.
- **Streaming parity**: `CompleteStream` runs the identical path; a `wrapped` onChunk sets `emitted=true` on first non-empty chunk; any later failure becomes `ErrStreamInterrupted` which `shouldFallback` treats as terminal (avoids duplicating output). Non-streaming chain entries are called via `Complete`.
- **Nil-response guard**: a provider returning `(nil, nil)` is converted to an error at the governor boundary so it falls back instead of panicking.

### A.1.2 Pre-flight cascade (`preflight.go`)

Ordered table `preflightSteps` (order is load-bearing):

| # | Step | Effect | Event |
|---|---|---|---|
| 1 | `task-model-override` | `TaskModelOverrides[TaskType]` replaces `req.Model` — skipped when a task *chain* exists for that task. | — |
| 2 | `capability-down-route` | If `DownRouteToolModels` and request carries tools and catalog **knows** the model lacks tool-use, remap to `ToolCapableAlternative(model)` (same provider; cross-provider when `AGEZT_MODEL_DOWNROUTE_CROSS`). Runs **before** the gate so remap beats reject. | `capability.rerouted` |
| 3 | `capability-gate` | `StrictModelCapabilities`: tools + known-incapable → `ErrModelLacksToolUse`. Unknown models never blocked. | `capability.rejected` |
| 4 | `strict-tool-args-degradation` | Known model without sampler-level schema enforcement → note only. | `capability.degraded` |
| 5 | `json-mode-degradation` | JSON-mode request on a family without native JSON mode → note only. | `capability.degraded` |
| 6 | `rate-limit` | Fixed UTC-minute window (`RateLimitPerMin`); runs **before** budgets so a throttled call never touches the ledger. | `rate.limited` |
| 7 | `budgets` | `budgetScopes` table, widening→narrowing: `global` (effective daily ceiling), `task` (`TaskBudgets[TaskType]`), `agent` (`req.AgentDailyCeilingMc` keyed by `req.Agent`). First exceeded → refuse. | `budget.exceeded` with `scope` |
| 8 | `strict-pricing` | `StrictPricing` and model has no price in catalog or fallback table → `ErrUnpricedModel` (empty model exempt; known-free models pass). | `budget.unpriced` |

**Budgets are soft caps** (documented, reaffirmed 2026-06): check and `recordUsage` are separate critical sections with the provider call between them; N concurrent calls can overshoot by up to N−1 calls. Day rollover is lazy (`rolloverIfNeededLocked`, UTC date string) and clears global/task/agent ledgers.

**Unpriced models (BIZ-001)**: lax mode bills at `unpricedFallbackPrice` (Sonnet-class $3/$15 per MTok) and journals `budget.unpriced` on every call — previously $0, which bypassed every ceiling.

### A.1.3 Provider chain construction (`governor_providers.go`, `routes_apply.go`)

```mermaid
flowchart LR
  R[Registry insertion order] --> S[split: primary vs IsFallback]
  S --> SP[sortedPrimary: stable sort by authModePriority\nsubscription=0, local=1, api-key=2, unknown=2]
  SP --> CH[chain = sortedPrimary + fallback]
  CH --> Q{TaskRouteRequires[taskType]?}
  Q -- matched, none registered --> NIL[empty chain -> 'no eligible providers']
  Q -- matched --> HARD[chain = ONLY listed providers, listed order]
  Q -- no --> TR{TaskRoutes[taskType]?}
  TR -- yes --> SOFT[hoist listed registered providers to front]
  TR -- no --> MR
  SOFT --> MR{req.Model != ''}
  MR -- yes --> HM[applyModelRoute: hoist providers whose Models contain req.Model]
  MR -- no --> OUT[chain]
  HM --> OUT
  OUT --> OB[openChain: drop breaker-open providers\nunless that empties the chain]
```

- Model → provider resolution is **by hoisting**, not by restriction: if no provider lists the model, the chain is unchanged and the model id is sent to the cost-preferred provider (benefit of the doubt). The `modelKnownUnservable` skip (M955) is the only place a model is rejected before dispatch, and only when *every* registered provider declares a non-empty `Models` list.
- `ProviderInfo.Models` comes from the catalog (`catalogModelIDs` in providerboot); empty = unknown coverage.
- Quality/latency are **not** ordering dimensions (doc.go says so explicitly).

### A.1.4 Retry, breaker, Retry-After

- `callWithRetry`: up to `ProviderRetries` (default 2 → 3 attempts) in-place retries with exponential backoff `base<<attempt` + 0–25% jitter, only when `shouldFallback(err) && isTransient(err)`. `isTransient` is a lowercase substring match over the error text (`429`, `rate limit`, `overloaded`, `5xx`, `timeout`, `connection reset`, `eof`, …) because adapters surface upstream failures as wrapped strings. Each retry journals `provider.retry`.
- `shouldFallback`: false for `context.Canceled`, `DeadlineExceeded`, `ErrBudgetExceeded` (and wrappers), `ErrStreamInterrupted`; true otherwise.
- **Circuit breaker** (`breaker.go`, M997): per-provider consecutive-failure counter; at threshold (5) opens for cooldown (30s); after cooldown it is half-open (allowed); success closes. Only fall-back-worthy failures count (cancels/budget don't). `openChain` never empties the chain. Breaker is per-`Governor` instance (tenant siblings get their own).
- **Retry-After is NOT handled in the governor.** It lives in the provider adapters: `plugins/providers/internal/retry` (`NewHTTPError`, `ParseRetryAfter`, `RetryAfterOf`, used as a capped delay floor in its own retry loop). See [08-providers.md](08-providers.md). The governor's own backoff is unaware of server-directed delays.

### A.1.5 Named chains, per-task / per-agent chains

| Source | Config | Precedence |
|---|---|---|
| Per-request chain | `req.ModelChain` (runtime `WithModelChain` from agent profile `Fallbacks`, workboard seat `ModelChain`) | highest ("agent-chain") |
| Per-task chain | `AGEZT_TASK_MODEL_CHAINS="chat=a,b;code=c,d"`; live via `SetTaskModelChains` (Routing UI) | second |
| Default chain | `AGEZT_DEFAULT_CHAIN=<name>` → `fallbackChains[name]` | third |
| Single model | `req.Model` (AGEZT_MODEL / per-run) + `AGEZT_TASK_MODEL_OVERRIDES` | when no chain |
| Named chains | `AGEZT_FALLBACK_CHAINS="fast=haiku,gpt-4o-mini;thorough=opus,gpt-5"`; any slot may be `@name` | expanded once in `expandChains` (one level, no recursion, unknown → dropped) |

There is **no default model** baked in: an empty model list + empty `req.Model` → `ErrNoModelConfigured` naming the task type.

### A.1.6 Accounting (`governor_usage.go`)

`recordUsage` → `costMicrocentsCached(model, in, cachedRead, cacheWrite, out)` (cache read/write subsets clamped to `[0,in]`; missing cache prices bill at full input rate) → adds to `spentToday` (atomic) + `spentByTaskToday` + `spentByAgentToday` under `mu` → `indexUsageTokens` (two-generation bounded map, 8192 per gen, migrate-on-write so partial sums are never served) → `budget.unpriced` (if needed) → `budget.consumed` with provider/model/tokens/cost/spent/ceiling/correlation.

Price lookup `priceForOk`: live catalog `FindModel` (exact; `provider/model` syntax supported) → `modelPriceTable` exact → longest case-insensitive prefix (binary search over `sortedPrefixes`, then slow scan for runtime-added entries) → not found.

### A.1.7 Concurrency

- `mu sync.Mutex`: ledgers, rate window, task/fallback chain maps, ceiling override.
- `spentToday atomic.Int64` for lock-free reads.
- `chainMu sync.RWMutex`: `primary`/`sortedPrimary`/`fallback`; `Replace` builds fresh slices (readers keep consistent old backing arrays).
- `usageMu`: usage index only. `breaker.mu`, `respCache.mu` internal.
- `bus atomic.Pointer[bus.Bus]`; `liveCatalog atomic.Pointer[catalog.Catalog]` is **package-global** (shared by all governors incl. tenant siblings).

### A.1.8 Config / env (parsed in `plugins/providerboot`, `governorConfigFromEnv`)

`AGEZT_PROVIDER`, `AGEZT_MODEL`, `AGEZT_TASK_ROUTES`, `AGEZT_TASK_ROUTE_REQUIRES`, `AGEZT_TASK_MODEL_OVERRIDES`, `AGEZT_TASK_MODEL_CHAINS`, `AGEZT_FALLBACK_CHAINS`, `AGEZT_DEFAULT_CHAIN`, `AGEZT_TASK_BUDGETS` (microcents, >0), `AGEZT_RATE_PER_MIN`, `AGEZT_MODEL_STRICT`, `AGEZT_PRICING_STRICT`, `AGEZT_MODEL_DOWNROUTE`, `AGEZT_MODEL_DOWNROUTE_CROSS`, `AGEZT_LLM_CACHE_TTL`. Daily ceiling boots at `DefaultDailyCeilingMicrocents` and is adjusted at runtime via `SetDailyCeiling` (control plane / UI). Tenant quotas: `AGEZT_TENANT_DAILY_CEILING` (USD), `AGEZT_TENANT_RATE_PER_MIN` (daemonconfig).

### A.1.9 Events emitted

`routing.decision` (subjects `governor.route`, `governor.cache`), `provider.fallback`, `provider.retry`, `provider.breaker_open`, `provider.breaker_closed`, `budget.consumed`, `budget.exceeded`, `budget.unpriced`, `budget.ceiling_set` (actor `operator`), `rate.limited`, `capability.rerouted`, `capability.rejected`, `capability.degraded`. All carry the request `CorrelationID`.

### A.1.10 File-by-file

| File | What it does |
|---|---|
| `doc.go` | Package doc: three pieces (registry, chain construction, microcent budget engine); documents the shipped cost-only chain order (subscription → local → api-key → fallback). |
| `governor.go` | `Governor` struct (ledgers, chains, breaker, bus, usage index, cache), `New`, `Name`, `Registry`, `Replace` (hot reload + chain rebuild), sentinel budget/pricing/rate/capability errors, `DefaultDailyCeilingMicrocents`. |
| `governor_config.go` | `Config` struct (registry, bus, ceilings, task routes/overrides/chains/requires/budgets, named chains, rate cap, injected catalog capability funcs, strict flags, down-route, cache TTL/size, retry and breaker knobs) + retry defaults. |
| `governor_errors.go` | `ErrNoModelConfigured`, `ErrNoProviders` (+Unwrap). |
| `governor_complete.go` | `Complete` (cache check → completeChained → preflightAndRoute → runChain; nil-response guard; cache put) and `CompleteStream` (emitted tracking → `ErrStreamInterrupted`). |
| `governor_complete_chained.go` | `completeChained`: chain source precedence (agent > task > default), `@` expansion, unservable-model skip, model→model fallback walk with `provider.fallback` events. |
| `governor_complete_helpers.go` | `preflightAndRoute`, `runChain` (breaker filter, usage on success, fallback events), `openChain`, `ProviderHealth`, `callWithRetry` (backoff+jitter, `provider.retry`), `isTransient`. |
| `governor_chains.go` | `modelChainFor`, `modelKnownUnservable`, `SetTaskModelChains`, `TaskModelChainsView`, `ChainPrefix`, `SetFallbackChains`, `FallbackChainsView`, `defaultChainModels`, `expandChains`. |
| `governor_providers.go` | `Providers`, `sortPrimary`, `routeChain` (requires → routes → model hoist), `applyModelRoute`, `authModePriority`. |
| `routes.go` | `TaskModelOverrides`, `TaskRouteRequires`, `TaskRoutes` types with semantics docs; `parseTaskRoutesEnv` (`k=a,b;k2=c`), `ParseTaskModelOverridesEnv`. |
| `routes_apply.go` | `applyTaskRouteRequire` (hard restriction; nil sentinel), `applyTaskRoute` (soft hoist). |
| `routes_parsers.go` | Exported parsers `ParseTaskRoutesEnv`, `TaskModelChains` + `ParseTaskModelChainsEnv`, `ParseFallbackChainsEnv`, `ParseTaskBudgetsEnv`. |
| `preflight.go` | `preflightStep` table + the eight steps, `runPreflight`, `publishCapability`. |
| `budgetgate.go` | `budgetScope` abstraction, `budgetScopes` (global/task/agent), `evalBudgetScope`, `gateBudgets`. |
| `governor_ceiling.go` | `SpentMicrocents`, `SetBus`, `DailyCeilingMicrocents`, `effectiveCeilingLocked`, `SetDailyCeiling` (runtime override; 0 = unlimited), `StrictPricingEnabled`, `WithDailyCeiling`, `WithLimits`. |
| `governor_usage.go` | `recordUsage` (token sanitizing, cost, ledgers, events), `indexUsageTokens` (2-gen bounded index), `UsageFor`, `Snapshot`. |
| `governor_usage_helpers.go` | `admitRate`, `SpentByTaskMicrocents`, `SpentByAgentMicrocents`, snapshot types, `rolloverIfNeededLocked`, `publish`, `shouldFallback`, `providerNames`. |
| `pricing.go` | `modelPrice`, `liveCatalog` + `SetCatalog`, fallback `modelPriceTable` (Claude list prices + free local/mock), `sortedPrefixes`, `unpricedFallbackPrice`, `priceFor`, `priceForOk`, `modelIsPriced`, `ModelIsPriced`. |
| `pricing_cost.go` | `CostMicrocents`, `costMicrocents`, `costMicrocentsCached`, `saturatingMul`, `saturatingAdd`. |
| `cache.go` | Opt-in TTL LRU response cache (`respCache`, `cacheKey`, `get`, `put`). |
| `introspect.go` | Read-only copies of task routes/requires/overrides; `copyStringSliceMap`. |
| `registry.go` | `AuthMode`, `ProviderInfo` (+`Serves`), `Registry` (`Register` with name-match check, `Replace`, `Remove`, `Get`, `All`, `Names`). |

Tests (30 files): budget scopes (`*budget*_test.go`, `agent_budget_test.go`, `ceiling_set_internal_test.go`), chains (`modelchain*_test.go`, `agentchain_test.go`, `fallbackchains_internal_test.go`, `modeloverride_test.go`, `modelroute_test.go`, `require_test.go`, `routes_test.go`), pricing (`pricing_internal_test.go`, `pricing_fuzz_internal_test.go`, `overflow_test.go`, `strict_pricing_test.go`, `unpriced_budget_test.go`), resilience (`retry_test.go`, `breaker_test.go`, `nilresp_test.go`), concurrency (`snapshot_race_test.go`, `usage_index_internal_test.go`), `cache_test.go`, `capability_degraded_test.go`, `governor_correlation_test.go`.

## A.2 `kernel/catalog`

**Purpose.** Data-driven provider/model registry mirroring models.dev `api.json`. Feeds: providerboot (which providers to build + their `Models`), governor pricing (`SetCatalog`), capability injections (`ToolCapableAlternative`, `StrictToolArgsNative`, `FamilySupportsNativeJSONMode`), the vault credential naming scheme, and the Models UI.

**Key types/functions.** `Catalog{Providers map[string]*Provider, SyncedAt, Sources}`, `Provider{ID, Name, Env, NPM, API, Doc, Models}`, `Model{ID, Name, Family, Attachment, Reasoning, ToolCall, StrictToolArgs, SchemaConstrainedDecoding, GrammarConstrainedDecoding, Knowledge, Release, Modalities, OpenWeight, Limit, Cost}`, `Cost` (USD/MTok floats → `*MicrocentsPerMTok()` int64), `Family` + `FamilyFromNPM` (maps `@ai-sdk/*` package → wire dialect; groq/xai/deepseek/… → openai-compatible; unknown → `FamilyUnknown`), `Store` (`Load`, `SaveAPI`, `SaveLocal`, `SaveCustom`, `UpsertCustomProvider`, `LoadMeta`), `Syncer` (`Sync`), `DiscoverOllama`, `FindModel`, `ToolCapableAlternative[Among]`, `VisionCapableAmong`, `BestModelsAcross`, `ProviderCredentialName` / `ProviderCredentialLookupNames` / `DuplicateCredentialEnvs`, `(*Provider).HasCredentials(lookup)`.

**Merge precedence.** `Load` merges `api.json` → `local.json` → `custom.json`; later wins on non-empty scalar fields, model maps union with later winning per model id.

**Credential naming.** Provider-scoped vault key `provider:<providerID>:<ENV>` is looked up first, then the bare env name — so two providers sharing `OPENAI_API_KEY` can carry different keys; `DuplicateCredentialEnvs` tells callers when a bare legacy vault entry is ambiguous.

**Sync.** `Syncer.Sync` GETs `DefaultSyncURL=https://models.dev/api.json` (overridable `AGEZT_CATALOG_URL` daemon-side) with a netguard client (`AllowLoopback`+`AllowPrivate`; link-local always blocked → no metadata pivot), 30s timeout, 8 MiB cap, and refuses a payload with zero providers (keeps the prior `api.json`). `DiscoverOllama` probes `/api/tags` (3s) and synthesises provider `ollama-local`.

**Concurrency.** `Catalog` is read-only after construction. `Store.metaMu` serializes meta.json read-modify-write.

| File | What it does |
|---|---|
| `doc.go` | Package doc: models.dev schema, three on-disk layers and precedence. |
| `types.go` | `Provider`, credential naming helpers, `DuplicateCredentialEnvs`, `HasCredentials`, `Family` constants. |
| `family.go` | `FamilyFromNPM`, `FamilySupportsNativeJSONMode`, `Model`, `Modalities`, capability predicates (`SupportsVision`, `SupportsPromptCache`, `SupportsStrictToolArgs`, `AgentWarnings`), `Limit`, `Cost` + microcent converters, `Catalog`. |
| `catalog_merge.go` | `Merge`, `ParseAPIFile`, `MarshalAPI`. |
| `catalog_ops.go` | `NewEmpty`, `ProviderList`, `FindModel`, `StrictToolArgsNative`, tool-capable/vision/best-model pickers. |
| `store.go` | On-disk `Store`, file name constants, `Meta`, atomic writes, `UpsertCustomProvider`. |
| `sync.go` | `guardedClient`, `Syncer`, `SyncResult`, `Sync` (size cap, empty-catalog refusal). |
| `discovery.go` | `DiscoverOllama`, `ollamaModelHasVision`. |

Tests: `catalog_test.go`, `downroute_test.go`, `sync_test.go`, `fuzz_test.go` (ParseAPIFile fuzzing).

## A.3 Keyring / multi-key selection (`creds/keyring.go`)

"Store many, pick active" (M700): every key for env var `NAME` is stored under `NAME#<label>` (label slug `^[a-z0-9][a-z0-9_-]{0,31}$`); the **active** key is mirrored into the bare `NAME`, which is what provider `CredLookup` reads. `KeyringAdd(name,label,value,makeActive)` (first key auto-activates), `KeyringActivate`, `KeyringRemove` (removing the active key clears the bare name), `KeyringList` returns `KeyInfo{Label, Active, Last4}` only — never values. A bare key without a matching slot shows as synthetic label `default`. **No automatic rotation or failover between keys**: switching is a manual copy (owner decision); provider-level fallback is the governor's job. The same `#label` convention is reused by `settings` for multi-account channels.

## A.4 Routing summary diagram (model → provider → endpoint → credential)

```mermaid
flowchart TD
  RUN[run ctx: agent profile model + fallbacks / seat chain / task type] --> REQ[agent.CompletionRequest\nModel, ModelChain, TaskType, Agent, AgentDailyCeilingMc]
  REQ --> GOV[Governor.completeChained -> preflight -> routeChain]
  GOV --> PI[ProviderInfo chosen\n(Name, AuthMode, Models from catalog)]
  PI --> AD[provider adapter (plugins/providers/*)\nbuilt by providerboot from catalog Provider:\nFamily from NPM, base URL from API]
  AD --> CR[CredLookup chain:\nscoped vault key provider:ID:ENV -> bare vault ENV -> os env\n(AWS: + shared file / credential_process / IMDS / STS / SSO / web identity)]
  AD --> HTTP[HTTP endpoint; adapter retry honours Retry-After]
  CAT[(catalog api/local/custom.json)] --> PI
  CAT --> PRICE[governor pricing via SetCatalog]
  VAULT[(creds.json)] --> CR
```

---

# Part B — Policy (`kernel/edict`, `kernel/approval`)

## B.1 `kernel/edict`

**Purpose.** Pure, journal-free policy decision engine: capability-scoped trust ladder + immutable hard-deny floor + ask-resolution policy. The runtime journals each `Outcome` as `policy.decision`; runtime changes are journaled as `policy.changed` and can be replayed (`AGEZT_EDICT_DURABLE=on`) or compacted (`policy.compacted` + snapshot file).

### B.1.1 Model

- **Capabilities** (`edict.go`, `AllCapabilities()` — 37): `shell`, `file.read`, `file.write`, `file.delete`, `file.list`, `http.get`, `http.post`, `provider.call`, `delegate`, `coding`, `acp_agent`, `remote_run`, `notify`, `homeassistant.read`, `homeassistant.call`, `browser.read`, `browser.action`, `memory`, `world`, `web.search`, `research`, `schedule`, `runs.read`, `standing`, `board`, `workboard`, `skill`, `introspect`, `oversee`, `code.exec`, `tool.forge`, `mcp.install`, `mcp.call`, `config.read`, `config.write`, `workflow.manage`, `market.install`.
- **Trust levels**: `LevelDeny`(L0) · `LevelAsk`(L1) · `LevelAskFirst`(L2) · `LevelAskScoped`(L3) · `LevelAllow`(L4). `ParseTrustLevel` accepts `L0..L4` and `deny/ask/askfirst/askscoped/allow`; unknown = error. Note: L1/L2/L3 are **behaviourally identical** in `DecideWithCeiling` (all "Ask-class"); the per-session/per-scope semantics in their names are not implemented in the engine.
- **AskPolicy** (`AGEZT_APPROVAL_MODE`): `AskAllow` (default; Ask → Allow + `WouldAsk`), `AskDeny` (Ask → Deny), `AskPrompt` (Ask → `Decision=Deny` + `RequiresApproval=true`; runtime routes to HITL). Unknown env value falls back to AskAllow with a banner note (`selectAskPolicy` in cmd/agezt).
- **Default posture = default-allow**: `DefaultLevels()` sets **every** known capability to `LevelAllow` (guard test `TestDefaultLevels_MaxAutonomy`). Many per-capability doc comments still say "Ask-first by default" — those describe intent, not the shipped levels.
- **Unknown capability** (not in the level map): default-**deny** ("no trust level configured … (default-deny)") unless `Options.UnknownAllow` (set only by `AGEZT_ALLOW_ALL=1`).
- **Hard-deny floor** (`DefaultHardDeny`, DECISIONS F4) — all scoped to `shell`: fork-bomb `:(){:|:&};:`, `rm -rf /`, `rm -rf --no-preserve-root`, `mkfs`, `wipefs`, `dd if=`, `of=/dev/{sd,nvme,vd,xvd,mmcblk}`, `shutdown -`, `poweroff`, `reboot`, `format-volume`. Plus operator rules from `AGEZT_EDICT_DENY` (`substr` or `cap:substr`; named `operator[N]`) and runtime rules (`AddHardDeny`, named `runtime[N]`, removable only if `IsRuntimeRule` strict-matches `runtime[<digits>]`). Matching is case-insensitive substring over `denyCandidates(input)`: raw input + every JSON string value whitespace-collapsed + punctuation-adjacent-whitespace-stripped (defeats JSON-escape and padding evasion without merging words — M173/M426).
- **Trust ceiling** (`DecideWithCeiling`): clamps the looked-up level down to `ceiling`; can only tighten. Supplied by the runtime from ctx (`runtime.WithTrustCeiling`, monotonic-tightening down delegation trees): standing orders (`standingTrustCeiling` = min(order `max_trust`, initiative-mode implied level: inform_only→L0, ask→L1)), agent profile `TrustCeiling`, resumed tickets.

### B.1.2 Tool name → capability

Runtime `capabilityFor` (kernel/runtime/policy.go) resolves in order: (1) `ToolDef.Capability.For(input)` if it names a known capability; (2) plugin manifest declaration `Config.ToolCapabilities` (validated by `edict.KnownCapability`; a plugin may join an axis, never invent one); (3) `edict.CapabilityForToolCall(name, input)`:

- `forge_*` → `code.exec`; `mcp_*` → `mcp.call`.
- op-sensitive tools: `file` (read/stat/search→file.read; list/glob→file.list; write/append/replace→file.write; delete→file.delete; other op → `file.<op>` = unknown = deny), `artifacts` (delete→file.delete else file.read), `tool_forge` (test→code.exec else tool.forge), `mcp` (list→introspect else mcp.install), `workflow` (list/show→introspect else workflow.manage), `config` (set/register/unregister→config.write else config.read), `homeassistant` (call_service→homeassistant.call else read), `http` (POST→http.post else http.get).
- aliases: `fetch`→http.get; `browser.*` actions→browser.action; `voice`/`image_generate`/`rerank`→provider.call; `db`→memory; `delegate_await`/`council`→delegate; `conductor`→code.exec; `send_media`→notify.
- **Unmapped name → `Capability(toolName)`** → not in level map → **default-deny** (unless ALLOW_ALL). Garbled ops deliberately land on the *lower-risk* read axis for read/write pairs and on the *gated* axis for install/automation tools.

Guard tests keep this governed: `kernel/runtime/capability_guard_test.go` (`TestRuntimeTools_ResolveToGovernedCapabilities`, `…DeclarationMatchesTheNameSwitch`, `TestPreviouslyUngovernedTools_*`) and `plugins/builtintools/capability_guard_test.go` (`TestBootTools_ResolveToGovernedCapabilities`, `…DeclareTheirCapability`, `TestGovernedCapabilities_AllowedByDefault`).

### B.1.3 Decision flowchart (engine + runtime policyHook)

```mermaid
flowchart TD
  TC[ToolCall name+input] --> CAP[capabilityFor: ToolDef decl -> plugin manifest -> CapabilityForToolCall]
  CAP --> HD{any hard-deny rule matches\nany denyCandidate for this cap?}
  HD -- yes --> DENYH[Deny, HardDenied=true, rule name]
  HD -- no --> LV{level configured for cap?}
  LV -- no --> UA{UnknownAllow (ALLOW_ALL)?}
  UA -- no --> DENYU[Deny: default-deny unknown capability]
  UA -- yes --> L4[lvl = L4]
  LV -- yes --> LVL[lvl]
  L4 --> CEIL
  LVL --> CEIL[lvl = min(lvl, ceiling from ctx)]
  CEIL --> SW{lvl}
  SW -- L0 --> D0[Deny]
  SW -- L4 --> A4[Allow]
  SW -- L1..L3 --> AP{AskPolicy}
  AP -- AskDeny --> DA[Deny]
  AP -- AskAllow --> AA[Allow, WouldAsk]
  AP -- AskPrompt --> RA[Deny + RequiresApproval]
  A4 --> RT
  AA --> RT
  RA --> RT
  D0 --> RT
  DA --> RT
  RT[runtime policyHook post-processing] --> TP{agent ToolAllow/ToolDeny\nor noise policy denies?}
  TP -- yes --> HDENY[Allow=false, HardDenied]
  TP -- no --> G{Allow and a guard fires?\nepistemic escalation / intent-regret gating /\nprompt-injection guard=on (effectful, directive-like taint, untrusted run)}
  G -- yes --> REQ[requiresApproval=true, guardRaised]
  G -- no --> RQ{requiresApproval?}
  REQ --> RQ
  RQ -- no --> V[verdict]
  RQ -- yes --> AUTO{no guard raised AND session\nauto-approve covers cap?}
  AUTO -- yes --> AG[Allow + auto-approve event]
  AUTO -- no --> HITL[approval.Registry.Submit -> block\ngrant -> Allow; deny/timeout/cancel -> Deny]
```

Hard-denies never reach approval; session auto-approve answers only the Edict Ask axis, never an opt-in guard.

### B.1.4 Durability

- `PolicyChange{Action: mode.set|level.set|deny.add|deny.rm, Capability, To, Name, Substring, AppliesTo}` journaled as `policy.changed` by the control plane.
- `ProjectPolicyChanges` folds them (last-wins levels/mode, add/rm by name, malformed skipped) → `PolicyOverlay{Levels, DenyRules, Mode}` → `ApplyOverlay` (rules re-added with fresh `runtime[N]` names). Gated on `AGEZT_EDICT_DURABLE=on` at boot and at lazy tenant open (live env read).
- Compaction: `OverlaySnapshot{ThroughSeq, Changes}` at `<base>/runtime/edict_overlay_snapshot.json`; `ContentHash` (SHA-256 of marshaled snapshot) must equal the latest journaled `policy.compacted` hash or the snapshot is ignored (tamper → full replay).

### B.1.5 Concurrency

`Engine.mu sync.RWMutex`; `Decide*` holds the read lock; `SetLevel`, `SetAskPolicy`, `AddHardDeny`, `RemoveHardDeny` take the write lock. `Levels`/`HardDenyRules` return copies.

| File | What it does |
|---|---|
| `edict.go` | Package doc (two layers); all `Capability` constants with rationale comments; `TrustLevel` constants. |
| `edict_types.go` | `TrustLevel.String`, `ParseTrustLevel`, `Decision`, `Outcome`, `AskPolicy` (+`String`, `ParseAskPolicy`), `HardDenyRule`. |
| `edict_match.go` | `HardDenyRule.matches`, `denyCandidates`, `collectJSONStrings`, `collapseWhitespace`, `stripPunctAdjacentWhitespace`, `Options`, `Engine` struct, `RuntimeRulePrefix`, `IsRuntimeRule`. |
| `edict_engine.go` | `Levels`, `HardDenyRules`, `AskPolicy`, `SetAskPolicy`, `New`, `SetLevel`, `AddHardDeny`, `RemoveHardDeny`. |
| `edict_decide.go` | `Decide` and `DecideWithCeiling` (the algorithm in B.1.3). |
| `edict_defaults.go` | `DefaultLevels` (all L4), `DefaultHardDeny`, `AllCapabilities`, `KnownCapability`, `ParseDenyRules`. |
| `edict_overlay.go` | `PolicyChange`, `PolicyOverlay`, `ProjectPolicyChanges`, `ApplyOverlay`, `Level`. |
| `snapshot.go` | `OverlaySnapshotFile`, `OverlaySnapshot`, `ToChanges`, `ContentHash`, `LoadOverlaySnapshot`, `SaveOverlaySnapshot`. |
| `toolmap.go` | `CapabilityForToolCall` (B.1.2). |

Tests: `edict_test.go` (levels, ask modes, hard-deny, overlays, `TestDefaultLevels_MaxAutonomy`), `edict_forkbomb_test.go`, `strip_whitespace_test.go` (M426 false-deny regression), `fuzz_test.go`, `ceiling_test.go`, `snapshot_test.go`, `toolmap_test.go`, `browser_caps_test.go`, `coverage_boost_test.go`.

## B.2 `kernel/approval`

**Purpose.** In-process HITL queue. `Registry.Submit(ctx, SubmitSpec)` mints `appr-<ULID>`, stores a pending entry with a 1-buffered `done` channel, publishes `approval.requested` (subject `approval.request`, payload includes the **verbatim tool input**, effect class, predicted effects, affected resources, rollback notes, confidence, intent/regret fields), then blocks until `Resolve` (grant/deny only), timeout (`DefaultTimeout=5m`, override `AGEZT_APPROVAL_TIMEOUT` → `Config.Timeout`) → `DecisionTimeout`, or ctx cancel → `DecisionCancel` (journaled as `approval.denied`). Resolution event subject `approval.resolve` with kind `approval.granted|denied|timeout`. `Pending()` snapshot sorted by CreatedAt; `PendingCount()`.

Concurrency: `mu` around `entries`; `Resolve` deletes then non-blocking sends; `Submit` always `detach`es on exit. **State is in-memory only** — pending approvals do not survive a restart (the waiting run is resumed/cancelled by the runtime's resume machinery, see [04-agent-runtime.md](04-agent-runtime.md)).

Consumers: runtime `policyHook`, `configcenter` (restricted key HITL), `scheduler`, `plugins/tools/forgetool`, controlplane (`agt approve|deny`, web UI).

| File | What it does |
|---|---|
| `doc.go` | Package doc (pause point, four outcomes). |
| `approval.go` | `Decision` + `IsTerminal`, `DefaultTimeout`, `Request`, `Outcome`, `pending`, `Registry`, `Config`, `New`, `ErrUnknownApproval`, `SubmitSpec`. |
| `approval_methods.go` | `Submit`, `Resolve`, `Pending`. |
| `approval_ops.go` | `cloneRequest`, `PendingCount`, `detach`, `publishRequested`, `publishResolved`, helpers. |

Tests: `approval_test.go`, `coverage_edge_test.go`, `timeout_default_internal_test.go`.

---

# Part C — Sandboxing, egress, env, redaction

## C.1 `kernel/warden`

**Purpose.** Single `Engine` interface for running external processes with timeout, output caps, empty-env default and audit. Profiles `none`, `namespace`, `container`, `microvm`.

**What is actually enforced (package doc is explicit, RCE-002):**

| Host | `namespace` request | `container` request | `microvm` request |
|---|---|---|---|
| Linux | `namespace` = `Setpgid` (kill sweeps process group) + best-effort post-Start `prlimit` (CPU, AS, NOFILE, FSIZE). **No namespaces, no seccomp, no cgroups.** | `container` if the docker/podman backend is enabled (`AGEZT_WARDEN_DOCKER=on`), else → namespace | → namespace |
| Windows / macOS / other | → `none` (nothing isolated) | `container` if backend enabled, else `none` | `none` |

Cross-platform: `context.WithTimeout` (default 30s), `cmd.WaitDelay` (500ms), stdout/stderr each tail-capped (`capBuffer`, default 256 KiB), `Spec.Env == nil` → **empty** env (M186; never inherits daemon env), Windows `cmd /S /C` verbatim fixup (M958).

**Container backend** (`container.go`): `docker|podman run --rm --network none(default) -v <abs workdir>:/workspace -w /workspace -e K=V… <image python:3.12-slim> <inner argv>`; the runtime CLI itself is launched with an empty env. Env vars are passed as `-e NAME=VALUE` **on the CLI argv** (visible in host process listings).

**Events:** `warden.executed` (subject `warden.exec`: requested/effective profile, exit, durations, bytes, truncated, timed_out), `warden.profile_downgraded` (once per requested profile per process), `warden.limit_exceeded` (timeout/output_bytes/rlimit failures).

**Ctx helpers:** `WithCorrelation`/`CorrelationFrom` (stamped by runtime on every run), `WithProfileOverride`/`ProfileOverrideFrom` (per-run requested profile from execution profiles/seats).

**Concurrency:** engine is stateless per Run except `downgradeWarned` (mutex). `SetBus` must be called before first Run (unsynchronised read).

| File | What it does |
|---|---|
| `doc.go` | Honest capability statement: SPEC roadmap vs what ships (RCE-001/002 history). |
| `warden.go` | `Profile` + `IsKnown`, defaults, `Limits` (incl. Linux rlimit fields), `Spec`, `Result`, `Engine` interface, `engine` struct. |
| `warden_run.go` | `New`, `NewWithOptions`, `SetBus`, `EffectiveProfile`, `Run`, `classifyWaitErr` (M475), publishers. |
| `warden_ctx.go` | Correlation and profile-override context helpers. |
| `warden_helpers.go` | `actorOrDefault`, `downgradeReason`, ctx key constants. |
| `warden_linux.go` | `resolveEffectiveProfile`, `configurePlatformAttrs` (Setpgid + group SIGKILL cancel), `applyPlatformLimits`, raw `SYS_PRLIMIT64` (no x/sys per lean-deps). |
| `warden_other.go` | Non-Linux stubs: everything → `ProfileNone`. |
| `container.go` | `Options`, `ContainerOptions`, argv builder, env/path rewriting into `/workspace`. |
| `capbuf.go` | `capBuffer`: tail-keeping capped writer. |
| `cmdline_windows.go` / `cmdline_other.go` | `fixupWindowsCmd` (verbatim `cmd /S /C "<command>"`) / no-op. |

Tests: `warden_test.go`, `container_test.go`, `capbuf_test.go`, `classify_test.go`, `env_test.go` (empty-env contract), `cmdline_windows_test.go`, `coverage_supp_test.go`.

## C.2 `kernel/executionprofile`

**Purpose.** Inventory + policy contract for "where does high-risk work run". Does not launch work itself; tools (`shell`, `code_exec`) and the control plane consult it.

- `Build(Options) Inventory` → 10 profiles: `local`, `warden`, `worktree-coding`, `browser-session`, `docker`, `ssh`, `remote-agezt`, `modal`, `daytona`, `k8s`, each with requested vs effective isolation, routed/degraded flags, filesystem/network/env/secrets descriptions, status (`supported|degraded|partial|planned`).
- `WardenProfileForRun(id)`: `local`→`none`, `warden`→`namespace`, `docker`→`container`; everything else not routable via warden. `RoutableRunProfileIDsFor(inv)` adds docker/ssh/remote-agezt/modal/daytona/k8s when routed and not degraded.
- `ProfilePolicy` from `AGEZT_EXEC_PROFILE_ALLOW` / `AGEZT_EXEC_PROFILE_DENY`.
- Remote backends: `SSHConfigFromEnv` (`AGEZT_EXEC_SSH*`), `K8sConfigFromEnv` (`AGEZT_EXEC_K8S*`), `ModalConfigFromEnv` (`AGEZT_EXEC_MODAL*`), `DaytonaConfigFromEnv` (`AGEZT_EXEC_DAYTONA*`), each with argv builders and `With*Override`/`*OverrideFrom` ctx helpers.
- Env passthrough: `AGEZT_EXEC_ENV_{LOCAL,WARDEN,DOCKER}` (non-secret names; secret-shaped and `AGEZT_*` rejected) and `AGEZT_EXEC_SECRET_ENV_{…}` (explicit secret names allowed). `IsSecretEnvName` duplicates envscrub's fragment list.
- Secret files: `AGEZT_EXEC_SECRET_FILES_{LOCAL,WARDEN,DOCKER}` → `PrepareSecretFileMounts` loads the vault, writes each secret to a 0600 temp file, returns `ENVNAME=<path>` (docker: `/workspace/...` path) + cleanup.
- Remote secret policy `AGEZT_EXEC_REMOTE_SECRET_POLICY`: default/invalid = `deny`; `metadata` = names only; values are never exported.
- `Diagnose(inv, HealthOptions)` → `HealthReport` (looks up docker/podman/ssh/modal/daytona/kubectl via `exec.LookPath`; checks `AGEZT_REMOTE_ARTIFACT_BYTES`).

| File | What it does |
|---|---|
| `doc.go` | Package doc. |
| `profile.go` | `Status`, `Profile`, `Inventory`, `Options`, `Build`, `Find`, `WardenProfileForRun`, `RoutableRunProfileIDs[For]`. |
| `profile_local.go` | Builders for local, warden, worktree-coding, browser-session, docker profiles. |
| `profile_remote.go` | Builders for ssh, remote-agezt, modal, daytona, k8s profiles. |
| `profile_helpers.go` | Tool-set helpers. |
| `policy.go` | `ProfilePolicy`, `PolicyFromEnv`, `ParseProfilePolicy`, `Allows`, `RoutableRunProfileIDsForPolicy`. |
| `check.go` | Health checks, `Diagnose`, `RemoteArtifactBytesEnv`. |
| `env.go` | Env passthrough policy, secret-name heuristic, summaries. |
| `secretfiles.go` | Secret-file mount parsing + materialisation from the vault. |
| `secretpolicy.go` | Remote secret policy parse/summary. |
| `ssh.go`, `k8s.go`, `modal.go`, `daytona.go` | Per-backend env config, argv builders, ctx overrides. |

Tests: `profile_test.go`, `env_test.go`, `secretfiles_test.go`.

## C.3 `kernel/netguard`

**Purpose.** SSRF/egress guard at the **dialer** (`net.Dialer.Control`) so it sees the resolved IP on every connect and redirect hop (defeats DNS rebinding and redirect pivots).

- `New(opts...)` → `*Guard`; options `AllowLoopback()`, `AllowPrivate()` (RFC1918 + ULA + CGNAT 100.64/10), `OnBlock(fn)`.
- `Allowed(ip)`: always blocks unspecified, `0.0.0.0/8`, **link-local (169.254/16 incl. cloud metadata, fe80::/10) — no option relaxes it**, multicast, broadcast; blocks loopback/private unless allowed. IPv6 forms embedding IPv4 (NAT64 `64:ff9b::/96`, IPv4-compatible `::/96`) are collapsed first (M171). Non-literal dial address → fail closed.
- `Dialer(timeout)`, `HTTPClient(timeout)` (fresh transport, **no Proxy func** — proxy env vars are ignored, which also prevents proxy-based bypass).
- `OnBlock` is wired by `kernel/toolreg` (`SetOnBlock(d.NetguardPublish(tool))`) and mcpbridge to journal `netguard.blocked`.

Single file `netguard.go`. Tests: `netguard_test.go`, `coverage_test.go`.

## C.4 `kernel/envscrub`

`Scrubbed()` returns a child env containing only an allowlist (PATH, PATHEXT, COMSPEC, SYSTEMROOT, HOME/USERPROFILE/APPDATA…, TEMP/TMP, LANG, `LC_*`, …) minus any name for which `IsSecretName` is true (contains `KEY`, `TOKEN`, `SECRET`, `PASSWORD`, `PASSWD`, `CRED`, `AWS_`, `AGEZT_`). `With(base, kvs...)` appends explicit values. Used by AWS `credential_process` (SEC-003), `acpagent`, `browser`, `coding` tools. Single file `envscrub.go`; tests `envscrub_test.go`, `coverage_test.go`.

## C.5 `kernel/redact`

**Purpose.** Chokepoint that scrubs secrets before they enter the permanent hash-chained journal.

- `Redactor{literals}`; `New()`, `SetSecrets(values)` (drops < `minLiteralLen`=8, dedupes, longest-first), `Redact(s)`, `RedactBytes(b)`, placeholder `"[REDACTED]"`.
- Patterns (`redact_patterns.go`): `sk-…`, `AKIA…`, `gh[pousr]_…`, `github_pat_…`, `xox[baprs]-…`, `xapp-…`, Telegram bot token, `gsk_`, `xai-`, `pplx-`, `fw_`, `AIza…`, JWT, `Bearer …`, PEM private key blocks; templated (context-preserving) patterns: URL userinfo password, `aws_secret_access_key=…`, Slack/Discord/Teams webhook URLs. Plus `MatchedCategories` helper.
- Wiring (cmd/agezt): when `AGEZT_REDACT` is not `off`, one redactor seeded with **every vault value** (`credSecrets`) + `AGEZT_REDACT_EXTRA` (`;`-list) is installed on the primary and every tenant bus via `bus.SetRedactor`; refreshed on `OnReload`. Bus applies it to every durably-published payload. `openaiapi` (error messages), controlplane remote mirror, and plugin log lines use their own pattern-only redactors.

| File | What it does |
|---|---|
| `doc.go` | Rationale: literals + patterns, determinism (stable hashes). |
| `redact.go` | `Redactor`, `SetSecrets`, `Redact`, `RedactBytes`. |
| `redact_patterns.go` | `Placeholder`, `minLiteralLen`, pattern catalogues, templated patterns, `MatchedCategories`. |

Tests: `redact_test.go`, milestone regressions `redact_m228/m231/m366/m490_test.go`, `redact_webhookurl_test.go`, `fuzz_test.go`, `coverage_test.go`.

---

# Part D — Credentials (`kernel/creds`, `kernel/creds/sigv4`, `kernel/chatgptauth`)

## D.1 Vault storage format and encryption

- File `<base>/creds.json`, flat `map[string]string` keyed by env-var-style names (incl. `provider:<id>:<ENV>`, `<ENV>#<label>`, `AGEZT_*` channel secrets, `AGEZT_CHATGPT_OAUTH`).
- **Encrypted envelope** (`SchemaEncrypted="agezt-creds-v2"`): AES-256-GCM, 12-byte nonce, 32-byte salt per save, PBKDF2-HMAC-SHA256 (stdlib-only implementation, single block) at `KDFIterations=200000`; legacy `hmac-sha256-iter` chain still decrypts. Decrypt bounds: `100000 ≤ kdf_iter ≤ 10000000` (SEC-002: an edited huge iteration count would hang boot); nonce length validated before `gcm.Open` (avoids panic). Derived keys memoised in `kdfCache` keyed by `(kdf, iter, salt, sha256(passphrase))` because many paths open a fresh `Store` per request.
- **Passphrase chain** (`defaultPassphraseChain`, M934): `AGEZT_VAULT_PASSPHRASE` → machine-bound key (unless `AGEZT_VAULT_AUTOENCRYPT=off`) → `""` (plaintext).
- **Machine-bound key**: `"machine-v1:" + hex(sha256("agezt-vault-machine-v1|" + machineID + "|" + uid|username))`, machineID = Windows `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid` (64-bit view), Linux `/etc/machine-id` or `/var/lib/dbus/machine-id`, macOS `ioreg IOPlatformUUID`; other OS → "" (plaintext). Threat model (documented): protects the file leaving the machine, not same-user local malware.
- Boot (`cmd/agezt/main.go`): `Load` (fatal on error) → `EncryptInPlace` (upgrade plaintext → envelope) → `injectConfig` (vault `AGEZT_*` entries exported to process env where unset).
- `Rotate(newPass)` (`agt vault rotate`, `AGEZT_VAULT_PASSPHRASE_NEW`), `InspectVault(path)` (no passphrase needed), `MigrateEncryption` (legacy KDF / low-iter → current).
- Writes: `atomicfile.WriteFile(…, 0600)` (unique temp + fsync + rename). `Store.mu` RWMutex in-process; **no cross-process lock** — `agt`, the daemon, and per-request `creds.NewStore` instances each do load-modify-save.

## D.2 Lookup chaining

`ChainLookup(sources...)` returns the first non-empty value. Daemon (`cmd/agezt/awschain.go`): `ChainLookup(vaultLookup, os.Getenv, AWSDefaultChain())` (vault wins over env for provider keys — opposite of `injectConfig`, where the real env wins for `AGEZT_*` settings). CLI (`cmd/agt/providerlookup`): `ChainLookup(ScopedVaultLookup(cat, vault), os.Getenv)`.

## D.3 AWS credential chain

| Source | Function | Notes |
|---|---|---|
| Env | inside `AWSDefaultChain` | `AWS_ACCESS_KEY_ID` etc. |
| Shared files | `AWSSharedCredentialsLookup(profile)` | `~/.aws/credentials` + region from `~/.aws/config`; profile from arg/`AWS_PROFILE`/default; INI parser; answers only `awsRecognisedNames`. |
| credential_process | `runCredentialProcess` | Opt-in `AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED`; 10s timeout; child env = `envscrub.Scrubbed()` + AWS selector vars + `AGEZT_AWS_CREDENTIAL_PROCESS_ENV` allowlist; cached until `Expiration − 60s`. |
| IMDSv2 | `AWSIMDSLookup(client)` | Token PUT (TTL 21600) then role creds; base `http://169.254.169.254` or `AWS_EC2_METADATA_BASE`; 1s timeout; 30s negative cache; refresh 60s before expiry. |
| STS AssumeRole | `AssumeRole`, `AWSAssumeRoleLookup` | SigV4-signed (`sts`) using base creds; XML parse; cache with refresh lead + failure suppression (`errCredFetchSuppressed`). |
| SSO | `GetSSORoleCredentials`, `AWSSSOLookup`, `LoadSSOParamsFromProfile` | Reads cached SSO token from `~/.aws/sso/cache/<sha1(startURL)>.json`, calls portal `GetRoleCredentials`. |
| Web identity | `AssumeRoleWithWebIdentity`, `AWSWebIdentityLookup` | Unsigned STS call with OIDC token file. |

`kernel/creds/sigv4`: `SignRequest(req, service, region, body, Creds, now)` adds `Authorization`, `X-Amz-Date`, `X-Amz-Content-Sha256`, `X-Amz-Security-Token`; `CanonicalQuery`, `AWSURIEncode`. Used by Bedrock, STS/SSO, and cmd/agezt.

## D.4 `kernel/chatgptauth`

"Sign in with ChatGPT" (unofficial Codex CLI OAuth client `app_EMoamEEZ73f0CkXaXp7hrann`, `auth.openai.com`, redirect `http://localhost:1455/auth/callback`, callback listener `127.0.0.1:1455`). `Manager` (per baseDir, `mu`): `Token(ctx)` refreshes proactively 2 min before the access-token JWT `exp`, tolerating refresh failure while a non-empty access token exists; `ForceRefresh` (on 401); `StoreTokens`; `ExchangeCode(code, verifier)`; `ImportFromCodexCLI(path)` (`DefaultCodexAuthPath` = `~/.codex/auth.json`); `Account()` (email from id_token); `Logout`. Helpers `GeneratePKCE`, `RandomState`, `AuthorizeURL`. Token endpoint calls use `netguard.New().HTTPClient` (public-only). Persistence: JSON `Tokens{access_token, refresh_token, id_token, account_id, last_refresh}` as vault key `AGEZT_CHATGPT_OAUTH` (shown read-only in the settings schema). Account id extracted from id_token claims. Consumers: providerboot (`openairesponses` subscription provider, `AuthSubscription` tier), controlplane login routes.

| File | What it does |
|---|---|
| `doc.go` | Package doc (unofficial path, UI acknowledgement gate). |
| `chatgptauth.go` | Constants, `Tokens`, `Manager` (load/persist via vault, `Token`, `ForceRefresh`, refresh, `StoreTokens`, `Account`, `Logout`). |
| `chatgptauth_helpers.go` | `ExchangeCode`, `ImportFromCodexCLI`, `DefaultCodexAuthPath`, PKCE/state/authorize URL. |
| `chatgptauth_http.go` | `postToken`, JWT payload/exp/email decoding, `accountIDFromIDToken`. |

Tests: `chatgptauth_test.go`, `exchange_ctx_internal_test.go`, `coverage_boost_test.go`.

## D.5 `kernel/creds` file-by-file

| File | What it does |
|---|---|
| `doc.go` | Package doc (stale: still says "No encryption"; see Gotchas). |
| `creds.go` | `FileName`, `PassphraseEnvVar`, `NewPassphraseEnvVar`, `Store`, `NewStore`, `SetPassphraseFn`, `IsEncrypted`. |
| `creds_io.go` | `Load` (envelope detection, machine-key error hint), `Save`, `atomicWriteVault`, `Rotate`. |
| `creds_ops.go` | `Set`/`Get`/`Has`/`Remove`/`Names`/`Lookup`, `ChainLookup`, `MaskValue`, `validateName`. |
| `encrypt.go` | Envelope constants and struct, `ErrPassphraseRequired`, `ErrWrongPassphrase`, `isEncryptedVault`, `encryptVault`, `decryptVault`. |
| `encrypt_kdf.go` | `kdfCache`, `cachedDeriveKey`, `deriveKeyPBKDF2`, `deriveKeyLegacyHMAC`. |
| `machine.go` | `AutoEncryptEnvVar`, `MachinePassphrase`, `defaultPassphraseChain`, `EncryptInPlace`. |
| `machineid_windows.go` / `_linux.go` / `_darwin.go` / `_other.go` | Per-OS `machineID()`. |
| `migrate.go` | `VaultStatus`, `InspectVault`, `MigrateEncryption`. |
| `keyring.go` | Multi-key keyring (A.3). |
| `aws.go` | credential_process opt-in/env constants, `credentialProcessEnv`, timeout, recognised names. |
| `aws_shared.go` | `AWSSharedCredentialsLookup`. |
| `aws_shared_files.go` | `loadAWSSharedFiles`, `awsConfigFilePath`, `readINISection`, IMDS base/timeout constants. |
| `aws_shared_process.go` | `runCredentialProcess`, `splitCommandLine`. |
| `aws_imds.go` | `AWSIMDSLookup`, `imdsCache`, `fetchIMDSCreds`, `imdsGet`, `readBody`, `AWSDefaultChain`. |
| `sts.go` / `sts_helpers.go` | `AssumeRoleParams`, `AssumedCreds`, `AssumeRole`, `AWSAssumeRoleLookup`; endpoint, XML parse, session name, `assumeRoleCache`. |
| `sso.go` / `sso_runtime.go` | `SSOParams`, SSO cache-file helpers, `GetSSORoleCredentials`; `ssoCache`, `AWSSSOLookup`, `LoadSSOParamsFromProfile`. |
| `web_identity.go` / `_cache.go` / `_parse.go` | `WebIdentityParams`, `AssumeRoleWithWebIdentity`, `AWSWebIdentityLookup`, cache, XML parse. |
| `sigv4/doc.go`, `sigv4.go`, `sigv4_crypto.go`, `sigv4_helpers.go` | SigV4 signer, key derivation, canonicalisation. |

Tests (19): `creds_test.go`, `encrypt_test.go`, `pbkdf2_test.go`, `kdf_known_answer_internal_test.go` (RFC vectors), `rotate_test.go`, `migrate_test.go`, `machine_test.go`, `keyring_test.go`, AWS (`aws_test.go`, `aws_process*_test.go`, `aws_http_timeout_internal_test.go`, `credfetch_negcache_test.go`, `sts_test.go`, `sso*_test.go`, `web_identity_test.go`), `sigv4/sigv4_test.go` (service scoping is load-bearing).

---

# Part E — Configuration (`kernel/settings`, `kernel/configcenter`)

Two distinct things share the name "Config Center":

1. **`kernel/settings`** = what the web console's Config Center page, `agt config`, and the `config` tool edit (non-secret settings + schema registry). Secret fields of that schema go to the vault.
2. **`kernel/configcenter`** = an agent-facing rated KV store with ACL/HITL/audit, opened by `runtime/compose.go` at `<base>/configcenter/` and exposed via controlplane `configcenter_handler.go` and `agentgw/config_handler.go`.

## E.1 `kernel/settings`

- **Schema**: `Field{Env, Label, Type(text|password|number|bool|csv|select), Secret, Required, Help, Apply(live|restart), Options, ReadOnly, Locked}`, `Section{ID, Name, Help, Source, Locked, Fields}`. `builtinSections()` (`schema_builtin.go`, ~200 fields, 45 sections: provider, embeddings, voice, ~31 channel sections, interfaces, browser-actions, execution-profiles, tunnel, limits, workboard, alerts, security, files, run-health). Only provider/model are `ApplyLive`.
- **Registry** (`registry.go`): `NewRegistry(base)` over `<base>/schemas/`; `Sections()` = built-ins + registered (registered fields must match `^AGEZT_[A-Z0-9_]+$` and must not shadow a built-in env; all forced to `ApplyRestart`); `Register` validates strictly (slug id, name, typed fields, select options); `Unregister(id, force)` refuses `Locked` sections without force; stateless (re-reads the dir every call).
- **Store** (`store.go`): `config.json` account-keyed; account-less accessors operate on `_default`. `Validate(field, value)`.
- **Multi-account** (`accounts.go`): `AccountSep="#"`, `SuffixEnv(base,label)`, `AccountLabels(keys, bases)`, `FieldGetter(label)` (reads process env), `SectionEnvs(sectionID)`.
- **Reload boundary** (`boundary.go`): `ReloadBoundaries(sections)` groups env names by Apply.
- **Persistence → runtime**: `cmd/agezt injectConfig` (before providerboot): computes `pinned` = schema envs already set in the real env (shown read-only in UI), then (unless `AGEZT_CONFIG=off`) `os.Setenv` for every non-empty store value and every vault `AGEZT_*` secret **not already set** in the real env. So precedence for AGEZT_* settings is: real env > config.json/vault. Live edits for `ApplyLive` fields are applied by the control plane via `os.Setenv` + provider reload.

| File | What it does |
|---|---|
| `doc.go` | Package doc (non-secret store keyed by exact AGEZT_* names). |
| `schema.go` | `FieldType`, `Apply`, `Field`, `Section`, `SourceBuiltin`, `Schema()`. |
| `schema_builtin.go` | Compiled-in sections and fields. |
| `schema_validate.go` | `Validate`. |
| `registry.go` | Schema registry (disk-merge pattern), `validateSection`. |
| `store.go` | `Store` (`Load` with BOM strip + legacy flat fallback, `Get`/`Set`/`Remove`/`All`/`Names`/`Save`). |
| `accounts.go` | Multi-account instance helpers. |
| `boundary.go` | `ReloadBoundary`, `ReloadBoundaries`. |

Tests: `settings_test.go`, `registry_test.go`, `accounts_test.go`.

## E.2 `kernel/configcenter`

- `Center` (`New`/`Open(DefaultConfig(base))`): `Store` (in-memory map of `ConfigEntry`, loaded from `entry_*.json`), `SecretClassifier`, `AccessPolicy`, `AuditLogger`. `SetApprovalRegistry` enables HITL.
- `ConfigEntry{Key, Value, Rating, Tags, Description, AccessPolicy, AllowedAgents, ExcludedAgents, VaultBacked, VaultPath, ValueHash, Version, Metadata, CreatedBy/At, UpdatedAt}`.
- Ratings `public|internal|restricted|secret` → default policies `auto|auto|hitl|deny` (`PolicySecretDeny` alias). `Set` auto-classifies when Rating empty and computes SHA-256 `ValueHash`.
- `AccessPolicy.Evaluate`: per-agent (120/min) and per-key (600/min) sliding windows → entry lookup → AllowedAgents/ExcludedAgents → rating (stored or classified) → stale-cache hash check (`CachedValueHash` mismatch → deny with old/new hash) → policy: auto (allow+audit), deny, hitl (`approval.Registry.Submit` with capability `config.access`, tool `config.get`; no registry → deny).
- `SecretClassifier.Classify(key,value)`: manual override → contextual rules (DB URL with creds, PEM private key, sk- key in name field, HMAC key length) → weighted key regexes → value regexes.
- Audit: `audit_YYYY-MM-DD.jsonl`; value logging re-classifies (`REDACTED` for secret, preview `first8...hash8` for restricted/internal, hash or full for public); `Query`/`QueryAccessLog`.
- `VaultConfig` (hashicorp/aws-secrets/azure) and `HitlConfig.NotifyChannels`, `AutoDenyOnTimeout`, `Audit.RetentionDays` are declared but not consumed by any code path in this package.

| File | What it does |
|---|---|
| `doc.go` | Package doc. |
| `types.go` | `ConfigEntry`, `ConfigAccessRequest/Response`, `AccessDecision`, `AuditEntry`, `Store` struct. |
| `config.go` | `Config`, `DefaultConfig`, `Rating`, `Policy`, `RefreshStrategy`, Hitl/RateLimit/Audit/Vault/Classifier config, `EnsureDefaults` (creates dir). |
| `center.go` | `Center`, `New`, `Open`, `SetApprovalRegistry`, `Close`, `GetEntry`, accessors, `SearchOptions`. |
| `center_ops.go` | `Get` (policy-gated), `Set`, `Delete`, `List*`, `Search*`. |
| `center_ops_filters.go` | Agent visibility filters, `UpdateRating`, `SetOverride`, audit/stat accessors, `ParseRating`. |
| `center_ops_persist.go` | `entryFile`, `persistEntry`, `loadStoreFromDisk`. |
| `access.go` | `AccessPolicy`, sliding-window rate limiter, `Evaluate`. |
| `access_helpers.go` | `resolvePolicy`, `checkRateLimit`, `requestHITLApproval`, `maskValue`, `HashValue`. |
| `audit.go` | `AuditLogger` (JSONL append, query). |
| `classifier.go` | `SecretClassifier` and pattern tables. |
| `entry_builder.go` | `NewConfigEntry` fluent builder. |
| `errors.go` | `ConfigError` codes. |
| `store.go` | In-memory `Store` methods. |

Tests: `configcenter_test.go`, `coverage_boost/extra/supp_test.go`.

---

# Part F — Execution seats, tenancy, state

## F.1 `kernel/seat`

Task-facing execution presets for workboard dispatch. `Seat{ID, Name, Description, ExecutionProfile, ModelChain, Tools, RestrictTools, Icon, Builtin}`. Built-ins: `default` (inherit), `reader` (tools restricted to `web_search, fetch, browser.read, artifacts, db`), `builder` (`local`), `isolated` (`warden`). `Store` (`OpenStore(<base>/seats)`, `seats.json`, ≤500 custom, id `^[a-z][a-z0-9-]{1,31}$`, temp-file crash recovery) overlays custom seats; `Create`, `Delete` (built-ins immutable), `Valid`, `List`, `Get`. `ValidExecutionProfile` accepts `"", local, warden, container`.

Applied in `controlplane/workboard_dispatch.go`: seat `ModelChain` → `WithModel`+`WithModelChain` (governor `req.ModelChain`), `RestrictTools` → `WithTools`, `ExecutionProfile` → `applyWardenExecutionProfile` (exec-profile policy check → `WardenProfileForRun` → `warden.WithProfileOverride`), degrading with a task comment on failure.

Files: `seat.go` (types, built-ins, `Get`, `IsBuiltin`), `store.go` (custom store). Tests: `seat_test.go`, `store_test.go`, `coverage_supp_test.go`.

## F.2 Multi-tenancy (`kernel/tenant`, `kernel/tenantctx`)

```mermaid
flowchart LR
  REQ[HTTP request with tenant id + token] --> AUTH{Registry.Authorize(id, token)\nconstant-time vs .tenant-token\nOR daemon admin token}
  AUTH --> ACQ[Registry.Acquire(id): mkdir tenants/id,\nload-or-mint token, OpenFunc]
  ACQ --> OPEN[cmd/agezt OpenFunc:\ntgov = gov.WithLimits(ceiling, rate)\ncfg.BaseDir=tenants/id, TenantID=id,\nfresh Warden + Edict, no OnReload]
  OPEN --> K[tenant Kernel: own journal/state/memory/vault dir,\nshared provider pool, own spend ledger + rate window,\nsame redactor, optional durable policy replay]
  K --> CTX[runtime stamps tenantctx.WithTenant on every run]
  CTX --> TOOL[tools e.g. peer/remote_run read tenantctx.Tenant(ctx)]
```

- `tenant.Registry` (`New(root, OpenFunc)`; root `<base>/tenants`): `ValidID` (`^[a-z0-9][a-z0-9_-]{0,63}$`, plus `filepath.Dir(dir)==root` containment), `Acquire` (lazy, idempotent, under `mu` — the kernel open happens while holding the registry lock), `Token` (never creates a tenant), `Authorize`, `Exists`, `List`, `Release` (close, keep disk), `Remove` (close + `RemoveAll`), `CloseAll`, `Count`. Token minting is race-safe via `O_CREATE|O_EXCL` with blank-file reclaim (M474).
- Opt-in `AGEZT_MULTITENANT=on`; quotas `AGEZT_TENANT_DAILY_CEILING` (USD; default = primary's effective ceiling at boot) and `AGEZT_TENANT_RATE_PER_MIN`.
- Isolation is **by base dir + separate kernel**; the provider registry, provider credentials, catalog pricing pointer, and the redactor are shared with the primary. Tenant spend does not count against the primary's ledger.
- `tenantctx`: `WithTenant(ctx, id)` (no-op for ""), `Tenant(ctx)`. Stamped by the kernel (`runtime.Config.TenantID`) so schedule/channel/pulse triggers are covered too.

Files: `tenant/tenant.go`, `tenantctx/tenantctx.go`. Tests: `tenant_test.go`, `tenant_list_test.go`, `tenantctx_test.go`.

## F.3 `kernel/state`

`Store` interface (`Get`, `Set`, `Delete`, `Keys`, `Close`) exposed to plugins as `kernel.stateGet/stateSet` (contract §3). `FileStore` (`Open(<base>/state)`): loads every valid `<ns>.json` at open (foreign/invalid names skipped), `RWMutex`, every mutation rewrites the whole namespace atomically (0644); empty namespace deletes its file; `Namespaces()` for `agt state list`. Namespace chars `[A-Za-z0-9_.-]`, not `.`/`..`. `toRawMessage` validates pre-serialized `json.RawMessage` (M426: an invalid value used to wedge the namespace). Opened by `runtime/compose.go`; used by pulse, runtime accessors/runexec. Single file `state.go`; test `state_test.go`.

---

## Extension points

| To add… | Do this |
|---|---|
| A new governed tool capability | Add a `Cap*` constant in `edict.go`, append to `AllCapabilities()` (it then defaults to L4 via `DefaultLevels`), declare it on the tool's `ToolDef.Capability` (and/or a `CapabilityForToolCall` case for name-only/dynamic tools). The capability guard tests in `kernel/runtime` and `plugins/builtintools` fail if a registered tool resolves to an unknown capability. |
| A new hard-deny floor rule | Append to `DefaultHardDeny` (boot floor, unremovable) or operators use `AGEZT_EDICT_DENY` / runtime `AddHardDeny`. |
| A new pre-flight gate | Add a `preflightStep` to the `preflightSteps` table at the right position (order is a decision; see the comment). |
| A new budget scope | Add a `budgetScope` entry to `budgetScopes` and record the ledger in `recordUsage` + `rolloverIfNeededLocked`. |
| A new provider family | Map its npm id in `catalog.FamilyFromNPM`; implement the adapter in `plugins/providers` (see 08). |
| A model price | Prefer `agt catalog sync` / `custom.json`; the `modelPriceTable` fallback is bootstrap-only. |
| A config setting | Built-in: add a `Field` in `schema_builtin.go` (and to controlplane `configEnvVars` if read in cmd/agezt — guard test). Plugin/skill: `settings.Registry.Register` (`agt config schema register`, `config` tool). |
| A new isolation backend | Profile builder in `executionprofile/profile_*.go`, `RoutableRunProfileIDsFor`, a `*Config` + ctx override; warden backends via `warden.Options`. |
| A seat | `seat.Store.Create` (control plane) or add to `builtins`. |
| Redaction pattern | Append to `patterns`/`templatedPatterns` in `redact_patterns.go` (keep patterns specific; fuzz + regression tests). |
| An outbound HTTP client | `kernel/platform/netout` — never `&http.Client{}` (archcheck fails it): `netout.Egress{...}.Client(timeout)` for agent-driven calls (host allowlist re-checked per redirect + IP guard; set `OnBlock` for journaling), `netout.OperatorClient(timeout)` for operator-configured endpoints, `netout.MetadataClient` only for IMDS/GCE metadata. |

---

## Gotchas / invariants

1. **Default-allow posture**: every known capability is L4; restriction is opt-out. Only unknown capabilities (unmapped tool names, undeclared plugin axes, unenumerated file ops) are default-denied — a tool that forgets its capability is silently dead (the reason the guard tests exist).
2. **Hard-deny floor can only be tightened at runtime**; `IsRuntimeRule` strict shape is the invariant. Built-in and `operator[N]` rules survive every overlay; ceilings and AskPolicy never relax it. Hard-deny substrings are shell-scoped only; on Windows most are no-ops in practice.
3. **L1/L2/L3 are indistinguishable in the engine** (no per-session/per-scope memory). "Ask first per session" behaviour, where it exists, comes from session auto-approve grants in the runtime, not edict.
4. **Trust ceilings only tighten**, and `WithTrustCeiling` keeps the lower of inherited vs new (VULN-001 fix).
5. **Budgets are soft caps** (concurrent overshoot by design). Unpriced models are billed at the Sonnet-class fallback rate, never $0 (BIZ-001). Money math is saturating integer microcents; negative token counts clamp to 0.
6. **Retry-After lives in provider adapters, not the governor.** The governor's `isTransient` is a substring heuristic over error text; an unrecognised transient error falls back immediately.
7. **Stream failures after first chunk are terminal** (`ErrStreamInterrupted`) — no retry, no fallback.
8. **Model routing hoists, doesn't restrict**: a model id no provider lists is still sent to the cost-preferred provider unless *every* provider declares a model list (`modelKnownUnservable`).
9. **Doc drift**: `governor/doc.go` mentions `RouteOptions.PreferredProvider` — no such type exists in the codebase. `routes_parsers.go`'s trailing comment says `applyTaskRouteRequire` returns "a SINGLE-element chain containing nil"; the code returns `nil`. `creds/doc.go` (and the `creds.go` provenance block) still say "Plain-JSON storage. No encryption" — the vault has encrypted by default since M934. Many `edict.go` capability comments say "Ask-first by default" while `DefaultLevels` is all-L4.
10. **Warden isolation is mostly nominal**: no namespaces/seccomp/cgroups anywhere; Windows/macOS run everything as `none`. Always key decisions (e.g. secret buckets) off `EffectiveProfile`, never the requested profile (RCE-001).
11. **Warden container backend leaks env values into argv** (`-e NAME=VALUE`), visible to other host users via process listings; secret-file mounts (`AGEZT_EXEC_SECRET_FILES_DOCKER`) avoid that.
12. **`Spec.Env == nil` means empty env**, not inheritance (M186). Callers wanting inheritance must pass `os.Environ()` explicitly.
13. **netguard always blocks link-local** (cloud metadata) regardless of options, and its client ignores proxy env. Validation is at dial time so it covers DNS rebinding and redirects.
14. **Redaction literals come only from the vault** (+ `AGEZT_REDACT_EXTRA`). A provider key supplied solely via the real process env is protected only if it matches a built-in pattern. Literals < 8 chars are never scrubbed.
15. **Approval payloads journal the raw tool input** (`approval.requested.input`) — redaction on the bus is the only scrub.
16. **Vault concurrency**: many code paths (`chatgptauth.persist`, controlplane handlers, `executionprofile.PrepareSecretFileMounts`) open a fresh `creds.Store`, Load, mutate, Save; there is no file lock, so concurrent writers (daemon + `agt`) can lose updates. The KDF cache exists because of this per-request pattern.
17. **Env precedence differs by kind**: for AGEZT_* settings the real process env wins over config.json/vault (`injectConfig`, and those fields are "pinned" read-only in the UI); for provider credentials the **vault wins** over env (`ChainLookup(vault, os.Getenv, …)`).
18. **`configcenter` persists raw values in plaintext** (`entry_*.json`, 0644, non-atomic `os.WriteFile`) — including keys it rates `secret` — and its audit logger re-classifies by key/value instead of using the entry's stored rating (an operator-overridden "secret" whose value looks public is logged in full). `Center.Get` holds `c.mu.RLock` across a potentially 5-minute HITL wait, blocking `Set`/`Delete`. `AccessPolicy` builds its own `SecretClassifier`, so `Center.SetOverride` does not reach it (masked in practice because `Set` always stores a rating). Value regex `^true|false$` is missing a group; the AKIA pattern is duplicated.
19. **`approval.SubmitSpec.AutoRec` and `ValuePreview` are dead fields**: set by configcenter, never copied into `Request` or any event.
20. **Seat/execution-profile id mismatch**: `seat.ValidExecutionProfile` (and `ErrInvalidIso`) accept `"container"`, but `executionprofile.WardenProfileForRun` only maps `"docker"` → `ProfileContainer`; a seat pinned to `container` always degrades at dispatch with "not a routable execution profile".
21. **Tenant `Acquire` opens the kernel under the registry mutex**, so a slow tenant open blocks all other tenant lookups. Tenants share the primary's provider credentials and `liveCatalog`; per-tenant breaker/ledger/rate are independent.
22. **`governor.liveCatalog` is package-global**; `SetCatalog` affects every governor in the process.
23. **Catalog sync refuses an empty provider list** (M425) — never overwrite a good `api.json` with `{}`.
24. **Vault KDF iteration bounds are a DoS guard** (SEC-002): never accept the envelope's `kdf_iter` unbounded.
25. **AWS `credential_process` is opt-in** and runs with a scrubbed env (SEC-003); it executes an arbitrary binary named in `~/.aws/config`.
