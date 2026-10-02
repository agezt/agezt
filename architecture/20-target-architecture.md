# 20 — Target Architecture (the redesign)

> **Status:** proposal, 2026-10-02. **Inputs:** the codemap (00–12), its findings register
> ([00 §9](00-README.md#9-verified-findings-register)), and `docs/REFACTORING-SCAN-2026-08.md` (Phases 0–3
> shipped). **Companion:** [21-migration-roadmap.md](21-migration-roadmap.md) says *how and in what order* we
> get here. This document says *what "clean" means*, precisely enough to be checked by a tool.

---

## 0. Why a redesign, and why not a big-bang rewrite

The 2026-08 refactor fixed **local** structure. Registries replaced switches, `main.go` lost 47%,
`agent.Run` was decomposed and 14 live defects were fixed. It did not change the **global** shape. Four
structural faults remain, and every finding in the register traces back to one of them.

| # | Structural fault | Measured | Findings it produces |
|---|---|---|---|
| **F1** | **Two god objects own the use cases.** `runtime.Kernel` has 284 methods and 60 fields. Feature logic (council, conductor, research, reaper, workboard dispatch, workflow execution, OKR, taste, intent, epistemic) lives *inside* the runtime package. `controlplane.Server` has 437 methods. | `kernel/runtime` 81 files; `kernel/controlplane` 204 files | Hard to test in isolation; every feature edit touches the hub; tenant routing is per-handler discipline. |
| **F2** | **No single application boundary.** CLI and Web UI go through control-plane ops. REST, OpenAI, agentgw, channels, cadence, standing, selfrepair and overseer each call `RunWith` *directly*, each decorating the context in its own way. | 10 run entry points outside the runtime | Run checks bypassed on REST/OpenAI; tenant leaks on `/events` and `/metrics`; unjournaled File Manager writes; SDK ↔ server drift; 198 hand-written web routes. |
| **F3** | **Cross-cutting concerns are opt-in per call site, not structural.** Policy, sandboxing, env scrubbing, SSRF-safe HTTP, journaling and atomic persistence each exist as excellent packages, but a call site must *remember* to use them. | — | Workflow nodes, council grounding, conductor verifier and `toolexec` skip policy/journal; plugins and `coding` get the full secret env; 3 different HTTP client postures in providers; configcenter writes plaintext 0644; vault races. |
| **F4** | **Contracts and implementations share packages.** The provider/tool contract lives in `kernel/agent` *with the loop*. Domain stores import it, so the data layer depends upward on execution. `kernel/controlplane`/`selfrepair` → `plugins/tools/overseertool`; `toolreg` → `runtime`. | 3 upward edges + 1 kernel→plugins | Import cycles get "solved" with func-pointer injection and setter wiring (`SetBus`, `SetMarket`, `Bind(LateDeps)`). |

**Decision: redesign every module's boundary and contract, and rewrite a module's internals only where
its internals are the problem.** Do this as a module-by-module *strangler*, not a big-bang branch.

Why not big-bang:
- 208k lines and 168k test lines encode hundreds of fixed bugs. The tests are the real specification.
- A rewrite from zero discards them and freezes the product for months.
- This repo already proved that a big extraction ships red without source-package tests (memory: *refactor
  extraction semantic drift*).

The strangler keeps `main` green after every PR. It still lets us replace whole modules wholesale. The
rewrite is real; it just lands one module at a time behind a stable contract.

**What gets rewritten from scratch** (internals are the problem):
- the application/op layer (new),
- the control plane (it becomes a transport),
- the run ingress pipeline (new),
- the tool invocation pipeline (new),
- triggers/autonomy (cadence + standing + pulse-initiative + workflow triggers + system tasks unify),
- storage (one store framework),
- the event log index + kind registry,
- config/secrets (settings + configcenter + vault unify),
- the channel supervisor + conversation store,
- the HTTP client factory and process-exec factory.

**What is kept and re-homed** (internals are good; the codemap and the 2026-08 scan agree):
- the `agent.Run` loop,
- the governor routing core,
- Edict's decision engine,
- the journal chain format,
- provider wire adapters (behind a fixed wire layer),
- channel transports,
- tool implementations,
- the skill/market/plugin logic,
- frontend views.

---

## 1. Architectural principles (each one is enforced, not aspirational)

| # | Principle | Enforced by |
|---|---|---|
| P1 | **Dependencies point down the layer stack only** ([§2](#2-the-layer-stack)). No upward, no sideways-between-modules except through the other module's `api` package. | `tools/archcheck` (new) in CI, with a ratchet allowlist that may only shrink |
| P2 | **Every use case is exactly one registered Operation.** All ingress adapters (control plane, Web UI, REST, OpenAI, agentgw, ACP, MCP-server, channels, triggers) call operations; none call domain services or the run engine directly. | archcheck: adapters may import only `app/...` + their transport libs; op-registry coverage test |
| P3 | **Cross-cutting concerns are pipelines, not helpers.** Authn/authz, tenant routing, validation, audit, policy, sandbox, redaction and journaling happen in the pipeline that *every* call traverses. A handler cannot opt out because it never sees an un-piped call. | Pipeline is the only constructor path; `go vet`-style analyzer forbids `exec.Command`, `http.Client{}`, `os.WriteFile` outside the owning platform packages |
| P4 | **Contracts live in leaf packages with zero logic.** Types and interfaces only; a contract package imports only other contract packages and the standard library. | archcheck layer L1 rule |
| P5 | **Each module owns its data exclusively.** Only module `X` reads or writes `X`'s store; others go through `X/api`. A store path can be opened once per process. | Store registry panics on a second open of the same path; archcheck forbids importing `X/internal` |
| P6 | **Fail closed at registration, not at run time.** A tool without a governed capability, an op without an authz level, an event kind without a schema, or a channel without a config schema is a **startup/test error**, never a silent runtime default-deny or no-op. | Registry `Validate()` called in boot + a test per registry |
| P7 | **One way to do each infrastructural thing.** One atomic writer, one store framework, one HTTP client factory, one process launcher, one SSE writer, one retry policy, one projection engine. | Analyzers from P3 + dead-duplicate tests |
| P8 | **Wiring is declarative.** Modules declare `Requires`/`Provides`; the composition root resolves them. No `SetX()` back-wiring, no `Bind(LateDeps)` phases, no func-pointer cycle breakers. | `module` registry topological start/stop; archcheck forbids exported `Set*` on module services |

---

## 2. The layer stack

```
 L7  cmd/agezt (composition root)        cmd/agt (CLI = thin client of L5 control-plane transport)
       │ imports everything below, wires modules, nothing else
 ────────────────────────────────────────────────────────────────────────────────────────────────
 L6  plugins/*  — implementations of L1 contracts: providers, channel transports, tools, seeds
       │ may import L1 contracts + L2 platform (http/exec/...) ONLY. Never app, never modules' internals.
 ────────────────────────────────────────────────────────────────────────────────────────────────
 L5  adapters/* (inbound transports)  controlplane · webui · restapi · openaiapi · agentgw · acp ·
       │                              mcpserver · channel ingress · trigger ingress
       │ may import app/ (operations + schemas) and L2 transport helpers (httpserver, sse, auth)
 ────────────────────────────────────────────────────────────────────────────────────────────────
 L4  app/  — Operation registry + pipelines (authn/z, tenant, validate, audit), RunService (run ingress
       │      pipeline), ToolInvoker (tool pipeline). The ONLY entry to behaviour.
 ────────────────────────────────────────────────────────────────────────────────────────────────
 L3  modules/<domain>/  — one tree per bounded context, each with:
       │   api/       exported service interface + DTOs (what other modules & app may import)
       │   internal/  model, store, service impl, projections (nobody else may import)
       │   ops.go     its Operations, registered into app (handlers call its own service)
       │   tools.go   its agent tools (via ToolInvoker registration)
       │   events.go  its event kinds + payload schemas (registered into eventlog)
       │   module.go  Module{Name, Requires, Provides, Start, Stop}
       │ may import: other modules' api/, L2, L1
 ────────────────────────────────────────────────────────────────────────────────────────────────
 L2  platform/  — infrastructure with no domain knowledge:
       │   eventlog (journal+bus+index+kind registry+projections) · store (framework+registry) ·
       │   modelgw (governor) · policy (edict+approval+guards) · sandbox (warden+envscrub+exec factory) ·
       │   netout (netguard HTTP client factory+retry) · secrets+config (vault+settings unified) ·
       │   tenancy · httpserver/sse/auth helpers · clock/ids
       │ may import L1, L0
 ────────────────────────────────────────────────────────────────────────────────────────────────
 L1  contract/  — pure types+interfaces: llm (Message, CompletionRequest/Response, Provider, Chunk, Usage),
       │          tool (ToolDef, Capability, Effect, Tool, Result), event (Event, Kind), channel
       │          (Channel, UnifiedMessage), op (Operation descriptor), ids
       │ may import L0 + stdlib only
 ────────────────────────────────────────────────────────────────────────────────────────────────
 L0  internal/ — brand, paths, atomicfile, strutil (leaf, stdlib only)
```

Today → target mapping of the key moves:

| Today | Target | Why |
|---|---|---|
| `kernel/agent` (contract **and** loop) | `contract/llm` + `contract/toolapi` (L1, **done W1.1**) and `modules/runs/internal/loop` (the kept `agent.Run`) | F4: data modules stop importing the loop. Measured: `memory` and `worldmodel` use only contract types (`Tool`, `Result`, `Provider`, `Message`) |
| `kernel/runtime` (284 methods) | **dissolved**: run engine → `modules/runs`; council/conductor/research → `modules/reasoning`; reaper → `modules/health`; workboard dispatch + proof + OKR → `modules/work`; workflow execution → `modules/workflows`; taste/intent/epistemic → `platform/policy` guards or `modules/knowledge` | F1 |
| `kernel/controlplane` (437 methods) | `adapters/controlplane`: wire framing + auth only (~10 files). Handlers move to each module's `ops.go` | F1, F2 |
| `kernel/webui` 198 fixed routes | routes **derived** from op registry metadata (`HTTP: GET /api/x`) | F2; deletes a hand-maintained table |
| `kernel/restapi`, `openaiapi`, `agentgw` call kernel interfaces directly | call `app` operations; OpenAI/REST runs go through `RunService` | F2: closes the run-check bypass and tenant leaks |
| `cadence` + `standing` + pulse initiative + `workflow` triggers + `cadence/systemtasks` + `scheduler` | `modules/triggers` (one model: `Trigger{Source, Target, Policy}`) | Five overlapping wake mechanisms, one of which bypasses Edict |
| `settings` + `configcenter` + `creds` | `platform/config` (non-secret settings, schema registry) + `platform/secrets` (vault, keyring, file lock) — one precedence rule | Two "Config Centers", plaintext secrets, opposite precedence, races |
| `warden` + `envscrub` + ad-hoc `exec.Command` in plugins/coding/browser/git | `platform/sandbox.Launcher` (the only way to start a child) | F3: secret leakage into plugin/git children |
| `netguard` + 3 provider HTTP postures + bare `http.Client` | `platform/netout` (**done W1.4**): `Egress.Client` / `OperatorClient` / `MetadataClient` — the only way to dial out (archcheck allowlist empty); retry stays provider-side | F3: SSRF inconsistency, broken TransientError retry |
| `journal` + `bus` + `event` + 15 projections | `platform/eventlog` with a **kind registry** and a **sidecar index** | No index (full scans on `why`, channel history, epistemic gate); 8 dead kinds; ad-hoc kinds |
| 13 `jsonstore` users + `board` single-instance rule | `platform/filestore` (**done W1.3**: 0600/0700 + cross-process `Lock`; path registry not needed, see roadmap) | P5; perms; vault races |
| `kernel/channel` + `channelwire` + `builtinchannels` | `contract/channelapi` (**done W1.1**) + `modules/channels` (supervisor, conversation store, accounts from config schema) + `plugins/channels/*` transports | Silent dead channels, panics crash daemon, history misses replies |
| `kernel/plugin` + `plugins/sdk` + dead `contract/gen` | `platform/extension` (one out-of-process protocol, kinds: tool now; channel/provider later) + delete the dead schema halves | Dead architecture |
| `kernel/controlplane` → `plugins/tools/overseertool` | overseer logic → `modules/fleet` ops; the tool becomes a thin L6 caller of `app` | Kernel→plugins edge |

---

## 3. The three pipelines (the heart of the redesign)

Every behavioural path in the system goes through one of these three. That is what makes the
cross-cutting guarantees structural (P3).

### 3.1 Operation pipeline (`app.Dispatch`)

```
 transport (cp/webui/rest/openai/agentgw/acp/mcp/channel/trigger)
   └─▶ app.Dispatch(ctx, Caller, OpName, rawArgs)
         1. authenticate   Caller → Principal{kind: operator|tenant|agent|system, tenant, scopes}
         2. resolve op     registry[OpName] → OpSpec{Input schema, Output schema, Authz, Tenancy, Stream, Audit}
         3. authorize      Principal × OpSpec.Authz   (replaces 47-op tenant allowlist & per-route levels)
         4. route tenant   ctx ← tenancy.For(Principal)  (MANDATORY; handlers never pick a kernel)
         5. decode+validate rawArgs → typed Input (JSON Schema generated from Go type)
         6. audit-begin    eventlog: op.invoked (kind registered, redacted args)       [if OpSpec.Audit]
         7. handler(ctx, Input) → Output | Stream[Output]
         8. audit-end      op.completed / op.failed
```

**OpSpec** is the single source of truth. Each of the following is *generated* from it:

| Generated from OpSpec | Replaces today |
|---|---|
| control-plane op table | 321 ops in 28 hand-written register functions |
| Web UI HTTP routes (`HTTP` hint) | 198 fixed route→op table entries |
| REST `/api/v1` routes | hand-written restapi |
| OpenAPI / JSON Schema → TypeScript client for the console | hand-written fetch wrappers |
| Python/TS/Rust SDK clients + contract fixtures | hand-written SDKs + `sdkparity` freshness check |
| `agt` command help and `--json` shapes | parts of the help data table |

### 3.2 Run pipeline (`app/runs.Start`)

All ten of today's `RunWith` entry points collapse into **one** request type and one pipeline.

```
 RunRequest{ Intent, Agent?, Source{kind: chat|api|openai|channel|trigger|delegation|repair|resume|workboard},
             Correlation?, ParentRun?, Overrides{model, tools, profile, budget}, Attachments, Stream }
   └─▶ RunService.Start
         1. resolve agent       roster → AgentProfile (soul, model chain, policies, trust ceiling)
         2. effective config    daemon cfg ⊕ live edits ⊕ agent overrides (today's effectiveConfig)
         3. admission gates     vision gate · tool allowlist · execution profile · budget pre-check ·
                                halt state · anomaly breaker   (TODAY: only on the control-plane path)
         4. context assembly    profile/taste/memory/world/skills → context.selection events
         5. resume ticket       stores {agent slug + RunRequest}, NOT resolved overrides
                                (fixes: named agents with soul/model are non-resumable)
         6. loop                modules/runs/internal/loop (today's agent.Run, unchanged contract)
                                  model calls → platform/modelgw
                                  tool calls  → ToolInvoker (§3.3)
         7. post-run            skills outcome/forge · memory distill · proof/assure · lifecycle
```

Sources differ only in `Source.kind` (for labels, quotas and audit) and in which admission gates they
are *allowed* to relax. Relaxation is declared in a table, not in each caller. Delegation (sub-agent) is
just `Start` with `ParentRun` set. The depth, tree and fan-out rails become admission gates, so sub-agents
are governed identically by construction. That generalises the LD-1 fix.

### 3.3 Tool invocation pipeline (`app/tools.Invoke`)

One invoker is used by the agent loop, workflow nodes, `toolexec` (operator direct calls), council
grounding, the conductor verifier, MCP-server exposure and out-of-process plugins.

```
 Invoke(ctx, Caller{run|workflow|operator|system}, ToolName, Input)
   1. lookup         tool registry (static + forge_* + mcp_* + plugin.*)  — unknown ⇒ error
   2. capability     ToolDef.Capability REQUIRED and governed (registration-time error otherwise, P6);
                     dynamic surfaces register their capability when attached
   3. schema         validate Input
   4. policy         platform/policy.Decide(capability, trust ceiling, agent hard-denies, guards)
                     → allow | ask (approval.Submit) | deny          → policy.decision event ALWAYS
   5. execute        inside sandbox profile if the tool spawns processes (Launcher) and
                     netout profile if it dials (Client) — tools receive these, never build their own
   6. journal        tool.invoked / tool.result (redacted, artifact offload) ALWAYS, incl. deny
```

This removes every side-path finding:
- workflow nodes journaling no policy/tool events,
- council `web_search` without a policy check,
- conductor verifier without a `code.exec` decision,
- `toolexec` emitting no `tool.result` on deny,
- `tool_search` and `browser.*` with no capability.

---

## 4. Module catalogue (L3)

Each module = one bounded context with exclusive data ownership. Columns: what it owns, its public `api`,
and the platform services it needs.

| Module | Owns (data) | Public api (examples) | Needs (L2) | Absorbs today's |
|---|---|---|---|---|
| `runs` | run registry, resume tickets, steering/interventions | `Start`, `Get`, `List`, `Cancel`, `Steer`, `Halt`/`Resume`, `Why` | eventlog, modelgw, policy, tools invoker | `runtime` run engine, `runexec`, `agent` loop, `resume`, `delegation`, `intervention`, `convo`, `contextselect`, `planner`, `scheduler` (plans), `meshctx` |
| `agents` (fleet) | roster profiles, guardians | `Create/Edit/Remove/Wake`, `Repair`, `Overseer.*` | store, runs/api, triggers/api | `roster`, `selfrepair`, overseer logic, `builtinguardians` seeding, reaper |
| `knowledge` | memory, world model, taste, operator profile | `Remember/Recall/Forget`, `World.*`, `Taste.*`, `Profile.Distill` | store, eventlog, modelgw (aux) | `memory`, `worldmodel`, `taste`, brain/profile distill |
| `skills` | skills + bundles | `Create/Promote/Quarantine/Activate`, `Bundle.*` | store, eventlog, modelgw | `skill`, `builtinskills` seeding |
| `market` | packs, sources, installs | `List/Install/Uninstall/Sync/Publish` | store, netout, skills/api, mcp/api | `market`, `builtinmarket` |
| `work` | workboard tasks, OKRs, proofs, seats | `Task.*`, `OKR.*`, `Prove`, `Dispatch` | store, runs/api, eventlog | `workboard`, `okr`, `proof`, `assure`, `seat` |
| `workflows` | workflow graphs, run history projection | `Save/Run/Test/Draft` | store, tools invoker, modelgw | `workflow`, `workflowexec`, `runtime/workflowrun*`, `workflowdraft` |
| `triggers` | schedules, standing orders, webhooks-in, observation rules, system tasks | `Create/Fire/Pause`, `SystemTask.*` | store, eventlog, clock, runs/api, workflows/api | `cadence`(+`systemtasks`), `standing`, pulse initiative, workflow triggers |
| `pulse` | observers, briefs | `Beat`, `Observers.*`, `Briefs` | eventlog, channels/api (sinks) | `pulse`, `anomaly`, `alerter` |
| `channels` | accounts, conversations (sessions), live state | `Send`, `SendMedia`, `Accounts.*`, `Conversation.*` | config/secrets, netout, runs/api | `channel`, `channelwire`, `builtinchannels`, channel handler in `cmd/agezt` |
| `reasoning` | — (stateless harnesses) | `Council`, `Conductor`, `Research` | runs/api, tools invoker, modelgw | `runtime/{council,conductor,research}*` |
| `board` | board/mailbox | `Post/Read/Watch` | store, eventlog | `board` |
| `artifacts` | blobs, index, data lake | `Put/Get/List/Collect`, `DB.*` | store | `artifact`, `datalake` |
| `extensions` | MCP registry, plugin specs, ACP catalog, toolforge, toolbox | `MCP.*`, `Plugins.*`, `Forge.*`, `Toolbox.*` | sandbox, netout, store | `mcp`, `plugin`, `toolforge`, `toolbox`, `acpcatalog` |
| `providers` | catalog, keyring selection, ChatGPT auth | `Catalog.Sync`, `Keys.*`, `SignIn` | secrets, netout, modelgw registry | `catalog`, `chatgptauth`, `providerboot` |
| `system` | update, backups, status, doctor | `Update.*`, `Status`, `Doctor` | sandbox, netout | `update`, status/doctor ops |

Rules that keep modules clean:
- A module's **tools** are registered by the module (`tools.go`) and call the module's own service. So
  `plugins/tools/boardtool`, `workboardtool`, `skilltool`, `standingtool`, `workflowtool`, `runstool`,
  `introspecttool`, `overseertool`, `forgetool`, `mcptool`, `config`, `db` and `artifacts` move *into*
  their modules.
- `plugins/tools/*` keeps only tools that are **host capabilities**: shell, file, http, fetch, browser,
  websearch, code_exec, coding, acp_agent, homeassistant, notify/send_media (via `channels/api`), peer.
- **Cross-module calls go through `api` interfaces only.** Cross-module *reactions* go through events
  (`eventlog.Subscribe(kind)`), never through back-pointers.

---

## 5. Platform layer (L2) — one implementation each

| Package | Contract | Fixes |
|---|---|---|
| `platform/eventlog` | `Publish(ctx, Event)`, `PublishStreaming`, `Subscribe(filter)`, `Query(index)`; **KindRegistry** (`Register(kind, payloadSchema, journaled bool)`); sidecar **index** by correlation/cause/kind/subject (rebuildable from segments); `Project(kinds, fold)` engine; **tenant-scoped** subscriptions | Full-scan `why`/history/epistemic; dead and ad-hoc kinds; unfiltered `/events`; mid-file corruption aborting boot (quarantine segment + continue read-only) |
| `platform/filestore` (W1.3) | `Load`/`LoadFrom`/`Save` (atomic, 0600/0700), `Lock(path)` (cross-process file lock; vault + settings merge-on-save). `Collection[T]` and a path `Registry` were dropped after measurement: one store has multiple holders and it is already single-instance | Single-instance hazard, 0644 stores, vault races, plaintext configcenter |
| `platform/modelgw` | today's governor routing/budget core + aux-call helper; **pricing becomes per-instance** (no package global `liveCatalog`) | Global catalog shared by tenants |
| `platform/policy` | Edict decision engine + approval + guards (epistemic, intent/regret, prompt-injection) behind `Decide(ctx, Request) Decision`; approvals **persisted** (survive restart) | Guards scattered in runtime; in-memory approvals lost on restart |
| `platform/sandbox` | `Launcher.Start(ProcSpec{profile, env policy, cwd, limits})` — env scrubbed by default, secrets by explicit grant; backends: host-pg, docker, (future) job objects/namespaces | Plugin/git/coding/browser children inheriting secrets; Docker `-e` argv secrets; Windows grandchildren |
| `platform/netout` | `Client(Profile{public|private-ok|loopback-ok, allowlist})` with uniform retry (transient classification by **type**, not text) + Retry-After + body cap | 3 SSRF postures; TransientError never retried; no `fetch` allowlist |
| `platform/config` + `platform/secrets` | One precedence: **explicit runtime edit > config store > env > default** for settings; secrets only from vault (env import is a one-shot migration); schema registry is the source for UI, env docs, channel RequiredEnv | Two Config Centers; opposite precedence; RequiredEnv drift; dead env vars |
| `platform/tenancy` | `For(Principal) Scope{base dir, stores, eventlog view, budgets}` | Tenant leak via unscoped surfaces |
| `platform/httpx` | httpserver + **one SSE writer** (gate + keepalive) + auth extractors | 9 SSE implementations (partly done) |

---

## 6. Extension model (L6) — one protocol, three kinds

- **In-process plugins** (compiled in) implement L1 contracts and register through their module:
  - providers → `providers` module registry
  - channels → `channels` registry
  - tools → ToolInvoker registry

  Each registration carries its **schema** (config fields, capability, effect). Registries validate at boot (P6).
- **Out-of-process extensions** (`platform/extension`) use one versioned protocol (today's stdio line-JSON
  v1, kept). Kind `tool` ships today; `channel` and `provider` are reserved but **not** half-specified:
  delete the dead `contract/gen` capability halves until implemented. Children are always started through
  `sandbox.Launcher`.
- **Data extensions** (skills, market packs, MCP servers, workflows) stay data and go through their modules'
  ops; this part was already excellent per the 2026-08 scan.

---

## 7. Frontend and SDK alignment

- `frontend/src/app/api` becomes a **generated** typed client from the op registry's OpenAPI. Hand-written
  fetch wrappers disappear, so a renamed field is a TypeScript error instead of an empty table.
- Frontend layering mirrors the backend: `features/*` may import `components/`, `lib/`, `app/`; never the
  reverse; features never import each other. Enforce it with an eslint boundaries rule. Today `components/`
  and `lib/` import `features/`, and features have cycles.
- SDKs (Go/Python/TS/Rust) are generated from the same OpenAPI. Contract fixtures are produced *by the
  server* in tests (golden), consumed by every SDK's test suite. This replaces the self-validating fixtures
  and the freshness-only `sdkparity`.

---

## 8. What "done" looks like (measurable exit criteria)

| Metric | Today | Target |
|---|---|---|
| archcheck violations (`tools/archcheck/allowlist.txt`) | **200** (88 plugin-reach · 70 cross-module · 34 adapter-bypass · 8 upward), 2026-10-02 | **0** |
| Methods on the biggest type | `controlplane.Server` 437, `runtime.Kernel` 284 | no type > 60 methods |
| Largest Go package (files) | controlplane 204, runtime 81 | ≤ 40 |
| Run entry points outside `modules/runs` | 10 | **1** (`RunService.Start`, called only from `app` ops and triggers) |
| Direct `exec.Command` / `http.Client{}` / `os.WriteFile` outside their L2 home (`tools/archcheck/calls-allowlist.txt`) | **85 sites** (13 exec · 58 http-client · 14 raw-write; union over linux/windows/darwin), 2026-10-02 → **27** after W1.4 (http-client 0) | 0 |
| Hand-maintained route/op/SDK tables | control plane table, 198 webui routes, 4 SDKs | 0 (generated) |
| Event kinds without schema / never emitted | ~7 ad-hoc / 8 dead | 0 / 0 |
| Journal full scans on hot paths (`why`, channel history, epistemic gate) | 3+ per call | 0 (index) |
| Registry-time validation (tools, ops, kinds, channels, settings) | partial (tests only) | all, at boot + test |
| Findings register items open | ~60 | 0 |

## 9. Non-goals and constraints carried over

- **Stdlib-first** stays; new dependencies follow `DEPENDENCIES.md` + `depscheck`. Whether to adopt an
  embedded SQL store is an owner decision, see [21 §5](21-migration-roadmap.md#5-open-decisions-owner).
- **Default-allow** posture stays. Hard denies, SSRF, budgets and explicit HITL stay (owner law).
- **No default provider/model** stays.
- **Boot resilience** stays: recoverable mismatches warn and degrade.
- **`agt` stays a thin client:** offline commands operate on `AGEZT_HOME` only through the same module
  stores (with the cross-process lock), never by re-implementing file formats.
- **The journal chain format is frozen** (v1 segments stay readable forever). The index is additive and
  rebuildable.
