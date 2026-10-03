# AGEZT Architecture — Codemap & System Overview

> **Scope:** the entire repository: Go daemon, CLI, kernel, plugins, SDKs, React console, build and CI tooling.
> **Snapshot:** analysed on 2026-10-02 against the working tree at `main` HEAD `f9e2c573`, including the
> ~500 uncommitted files. Every claim was read from source on disk, not from `docs/*.md`. Those docs are
> useful for vocabulary but several are stale; see [§9](#9-verified-findings-register).
> **Language:** English. File/line references use `path:line`.

This folder is a **codemap**. For every package it says what it is, which files it has, what each file
does, what it persists, which events it emits, how it is wired, and how to extend it. Start with this
page, then open the area document you need.

---

## 1. Document index

| # | Document | Covers | Size |
|---|---|---|---|
| 00 | **this file** | System overview, layering, package index, end-to-end flows, findings register, glossary | — |
| 01 | [01-daemon-boot-cmd-agezt.md](01-daemon-boot-cmd-agezt.md) | `cmd/agezt` composition root, the 50-step boot sequence, every `AGEZT_*` env var, `AGEZT_HOME` layout, `daemonconfig`, `internal/*`, Makefile/scripts/installers, `tools/*` gates, CI workflow | ~690 lines |
| 02 | [02-cli-cmd-agt.md](02-cli-cmd-agt.md) | `agt` CLI: dispatch/router/help, how it dials the daemon, offline vs daemon commands, complete command table, all 233 files | ~700 lines |
| 03 | [03-control-plane-and-http.md](03-control-plane-and-http.md) | `kernel/controlplane` protocol and all **321 ops**, `httpserver`, Web UI Go side (198 routes), REST, OpenAI-compatible API, `agentgw`, `auth`, `streamlimit`, `tunnel`, outbound `webhook`; HTTP-edge security; SSE path | ~1300 lines |
| 04 | [04-agent-runtime.md](04-agent-runtime.md) | `kernel/agent` loop, `kernel/runtime` (+`runexec`, `lifecycle`, `accessors`, `compose`, `types`), policy hook, delegation, steering/intervention, compaction, resume, planner, assure/proof, tool registry | ~730 lines |
| 05 | [05-governance-routing-security.md](05-governance-routing-security.md) | `governor` routing/budgets/breakers, `catalog`, `creds` vault + `sigv4`, `chatgptauth`, `settings`, `configcenter`, `executionprofile`, **Edict** policy, `warden`, `netguard`, `envscrub`, `redact`, `approval`, `seat`, `tenant`, `tenantctx`, `state` | ~720 lines |
| 06 | [06-data-memory-state.md](06-data-memory-state.md) | `event` (170 kinds, registry closed — W1.6), `journal` (BLAKE3 chain), `bus`, `ulid`, `filestore`, `memory`, `worldmodel`, `datalake`, `artifact`, `board`, `workboard`, `okr`, `taste`, `anomaly`, `alerter`; the store table | ~680 lines |
| 07 | [07-autonomy-and-extensibility.md](07-autonomy-and-extensibility.md) | `roster`, `cadence`(+`systemtasks`), `scheduler`, `standing`, `pulse`, `selfrepair`, `skill`, `market`, `plugin`, `mcp`, `acp`, `acpcatalog`, `workflow`, `workflowexec`, `update`, `toolbox`, `channel`, `channelwire`, `stt`, `voicetool`, `imagetool`, `reranktool`; how wakes become governed runs | ~1860 lines |
| 08 | [08-providers.md](08-providers.md) | Provider contract, 8 wire adapters + `compat` factory, sidecars (embed/voice/image/rerank), `providerboot` boot/reload, ChatGPT subscription, Bedrock/Vertex auth, `plugins/sdk`, `mcpbridge` | ~790 lines |
| 09 | [09-channels.md](09-channels.md) | `Channel` interface, 25 transport packages → 34 channel kinds, `builtinchannels` factories, multi-account `ENV#label`, inbound → run → reply | ~655 lines |
| 10 | [10-tools.md](10-tools.md) | Tool contract, catalog of 30 registered tools + 10 browser verbs, code_exec/shell/file/browser/research/council/conductor/overseer, guardians, built-in market and skills | ~550 lines |
| 11 | [11-frontend-console.md](11-frontend-console.md) | React 19 console: build→embed, hash routing, nav IA (36 views), data layer/SSE, design system, every file, guard tests, e2e | ~670 lines |
| 12 | [12-sdks-and-contract.md](12-sdks-and-contract.md) | Go/Python/Rust/TypeScript SDKs, parity matrix, `contract/` fixtures + codegen, `sdkparity`, API stability | ~450 lines |
| 20 | [20-target-architecture.md](20-target-architecture.md) | **The redesign:** structural faults F1–F4, principles P1–P8 (each enforced), layer stack L0–L7, the three pipelines (Operation / Run / Tool), module catalogue, platform layer, exit metrics | proposal |
| 21 | [21-migration-roadmap.md](21-migration-roadmap.md) | How to get there: waves W0–W5, PR slices, domain migration order, risk register, open owner decisions | proposal |

**How to use it**
- *"Where does X happen?"* Find the package in [§5](#5-package-index) and open its document.
- *"What happens when …?"* Read the flows in [§6](#6-end-to-end-flows). Each flow links to the document with the full detail.
- *"Is this safe to change?"* Every area document ends with **Extension points** and **Gotchas / invariants** sections.

---

## 2. What AGEZT is (one paragraph)

AGEZT is a self-hosted **agentic operating system** written stdlib-first in Go (5 direct third-party
dependencies). It ships two binaries:
- **`agezt`**, a long-running daemon that hosts the kernel.
- **`agt`**, the operator CLI.

A React console is compiled into the daemon. The daemon runs a fleet of durable, named agents. Every
model call goes through the **governor** (routing, fallback chains, budgets, circuit breakers). Every
tool call is gated by the **Edict** policy engine (allow / ask a human / deny). Every step is appended
to a **BLAKE3 hash-chained journal**, which is the single source of truth and the basis of `agt why`.
Agents wake from schedules, standing orders, Pulse observations, workflows, channel messages
(34 channel kinds) or API calls. All of them run under one correlation id per run.

---

## 3. The system at a glance

### 3.1 Size (non-generated, tracked files)

| Layer | Go packages | Non-test Go lines | Notes |
|---|---|---|---|
| `cmd/` (`agezt`, `agt` + 9 subpkgs) | 11 | ~44.6k | `cmd/agt` alone is 233 files |
| `kernel/` | 94 | ~113.4k | `controlplane` 204 files, `runtime` 81 + 5 subpkgs |
| `plugins/` | 80 | ~46.8k | providers, channels, tools, built-in seeds, plugin SDK, mcpbridge |
| `internal/` | 4 | ~0.3k | brand, paths, atomicfile, strutil |
| `sdk/` (Go) | 1 | ~0.7k | + Python, Rust, TypeScript SDKs |
| `tools/` | 8 | ~2.6k | repo gates and generators |
| **Go total** | **~199** | **~208k** (+ ~168k test lines in 992 `_test.go` files) | |
| `frontend/src` | — | ~66.6k TS/TSX | 36 views, embedded as `kernel/webui/dist` |

### 3.2 Runtime topology

```
                       ┌──────────────────────────── agezt daemon (one process) ─────────────────────────────┐
 agt CLI ──TCP 127.0.0.1:rand──▶ controlplane.Server ──┐                                                    │
 (runtime/control.addr/.token)   321 ops, NDJSON       │                                                    │
                                                       ▼                                                    │
 Browser ──HTTP :8787──▶ webui ──dials control port──▶ (same ops)          ┌──────────── runtime.Kernel ─────┐ │
          ◀──SSE /events── bus subscriber                                  │ runexec.Runner → agent.Run loop │ │
 OpenAI clients/IDEs ──▶ openaiapi  ──────(direct kernel iface)──────────▶ │   ├─ governor ─▶ providers      │ │
 REST SDKs (py/ts/rs) ──▶ restapi   ──────(direct kernel iface)──────────▶ │   ├─ policyHook ─▶ Edict/approval│ │
 Agent subprocesses ──▶ agentgw (unix sock, HMAC caps) ─────────────────▶ │   └─ tools (toolreg/plugins/MCP)│ │
 IDEs over ACP (stdio JSON-RPC) ──▶ kernel/acp ─────────────────────────▶ │ bus ─▶ journal (BLAKE3 chain)   │ │
 34 channel kinds (Telegram…) ──▶ channelwire ─▶ InboundHandler ─RunWith─▶ │ stores: memory/world/skills/... │ │
 Webhooks in (/hooks) ──▶ workflow triggers                                └───────────────┬─────────────────┘ │
                                                                                           │ events           │
   resident loops: Pulse · cadence · standing orders · workflow triggers · selfrepair ·    ▼                  │
   resumer · anomaly breaker · alerter · outbound webhooks · tickers   ◀── bus.Subscribe ──┘                  │
                       └──────────────────────────────────────────────────────────────────────────────────────┘
```

Key point: **there are two ways into the kernel.**
- The CLI and the Web UI go through **control-plane ops**.
- REST, the OpenAI-compatible API and agentgw call the kernel **directly** through narrow interfaces.

Image admission is shared by control-plane, REST/OpenAI and channels through `runtime.Kernel.AdmitImages` (W2.2a).
Tool allowlists, execution profiles and agent resolution remain control-plane features. See [03](03-control-plane-and-http.md).

---

## 4. Repository map (top level)

| Path | What it is |
|---|---|
| `cmd/agezt/` | Daemon binary and **composition root**: the only place that sees both the kernel and `plugins/*` and wires them together. → [01](01-daemon-boot-cmd-agezt.md) |
| `cmd/agt/` | Operator CLI (`package main` + `dial`, `format`, `haltresume`, `jsonout`, `keys`, `providerlookup`, `router`, `whoami`). → [02](02-cli-cmd-agt.md) |
| `kernel/` | All core subsystems (94 packages). Must not import `plugins/*`; one known exception, see [§4.1](#41-layering-rules-and-the-verified-import-graph). |
| `plugins/` | Pluggable implementations: `providers/*`, `channels/*`, `tools/*`, `builtin{channels,tools,guardians,market,skills}`, `providerboot`, `sdk` (plugin authoring kit), `external/mcpbridge`. |
| `internal/` | Tiny shared leaf helpers (`brand`, `paths`, `atomicfile`, `strutil`). |
| `sdk/` | Go SDK (control plane) + `python/`, `rust/`, `typescript/` REST SDKs. → [12](12-sdks-and-contract.md) |
| `contract/` | `fixtures/` (hand-written JSON event fixtures) and `gen/` (generated from `.project/agezt-contract.jsonc`, the plugin JSON-RPC contract). |
| `frontend/` | React console source (Vite/Vitest/Playwright). Build output goes to `kernel/webui/dist` and is committed and `go:embed`-ded. → [11](11-frontend-console.md) |
| `tools/` | Repo gates: `changelog-lint`, `changelog-split`, `deadcodecheck`, `depscheck`, `docclaimscheck`, `jsonschemagen`, `sdkparity`, `structure-md`. |
| `scripts/` | Build stamping, isolated dev loop (`.dev-home`), e2e/webui smoke harnesses, coverage ranking, nav audits. |
| `ops/wsl-runners/` | Self-hosted CI runner provisioning (historical; CI moved to GitHub-hosted `ubuntu-latest` on 2026-10-01). |
| `.github/workflows/` | `ci.yml` (17 gate jobs + 2 aggregator jobs required by branch-protection rulesets), `publish-sdks.yml`. |
| `examples/` | `agezt-run` (SDK usage) and `autonomous` demos. |
| `docs/` | Human docs, plans and audits (many are point-in-time; verify before trusting). |
| `.project/` | Planning/spec archive: vision docs, `agezt.proto` (deprecated), `agezt-contract.jsonc`, generated `STRUCTURE*`, phase archives. |
| `CHANGELOG/`, `CHANGELOG.md` | Split changelog, enforced by `changelog-lint`. |
| `security-report/` | Point-in-time security findings (go stale; re-verify). |
| `Makefile`, `dev.ps1`/`dev.sh`, `install.ps1`/`install.sh` | Build, dev loop, production installers (systemd / NSSM). |
| `*.exe` at repo root | Stray local build outputs (`agezt.exe`, `agt.exe`, `mcpbridge.exe`, `sdkparity.exe`, `docclaimscheck.exe`). Not source. |

### 4.1 Layering rules and the verified import graph

```
            cmd/agezt  (composition root: kernel + plugins)        cmd/agt (CLI; kernel types + controlplane client)
               │  ╲                                                   │
               ▼   ╲──────────────▶ plugins/*  ──────────────────────┐│
            kernel/*  ◀──────────────────(plugins import kernel)─────┘│
               │                                                      │
               ▼                                                      ▼
            internal/*  (leaf)                                    internal/*
```

Verified with `go list` (internal edges only):
- **Highest fan-in** (most imported): `kernel/agent` (67 importers), `kernel/event` (61), `kernel/bus` (55),
  `kernel/ulid` (46), `kernel/edict` (46), `kernel/channel` (31), `internal/brand` (21), `kernel/netguard` (17),
  `kernel/runtime` (14), `internal/atomicfile` (14), `kernel/platform/filestore` (13).
- **Highest fan-out:** `cmd/agezt` (59 internal imports), `kernel/controlplane` (49), `kernel/runtime` (48),
  `plugins/builtintools` (36), `cmd/agt` (33), `plugins/builtinchannels` (29).
- **The kernel never imports `plugins/*` — with one exception.** `kernel/controlplane` (`roster_repair.go`,
  `roster_wake.go`) and `kernel/selfrepair` import `plugins/tools/overseertool`. These are the only
  kernel→plugins edges in the graph.
- **Other upward or unusual edges:**
  - `kernel/toolreg` → `kernel/runtime`.
  - `kernel/governor`, `kernel/memory` and `kernel/worldmodel` → `kernel/agent` (the data layer depends on the loop types). Since W1.1 the types they use live in `kernel/contract/{llm,toolapi}` (aliased in `kernel/agent`); W1.2 repointed the importers: `governor` and `worldmodel` no longer import the loop; `memory` still does, for `GenerateObject` only (a model-gateway helper, moves with `platform/modelgw`).
  - `workboard → proof → assure` puts verifier types into the on-disk task format.
  - `cmd/agezt` imports `plugins/tools/codeexec` only for a banner line.
  - `cmd/agt` imports `plugins/tools/peer` for doctor checks.
- **Third-party dependencies are confined:**
  - `lukechampine.com/blake3`: `event`, `artifact`, `memory`, `plugin`, `skill`, `worldmodel`.
  - `emersion/go-imap/v2`: `channels/email`.
  - `btcsuite/btcec` and `coder/websocket`: `channels/nostr`.
  - `golang.org/x/net/publicsuffix`: `tools/browser`.
  - Everything else is the standard library.

---

## 5. Package index

Every Go package, one line each, with the document that covers it in depth.

### 5.1 `kernel/` — execution core → [04](04-agent-runtime.md)
| Package | Role |
|---|---|
| `agent` | **The** single model→tool loop (`agent.Run`). Defines `Provider`, `Tool`, `CompletionRequest/Response`, `ToolEffect`. Depends only on `bus` + `event`; pricing, policy and steering are injected. |
| `runtime` | `Kernel`: wires journal + state + bus + loop + providers + tools. Owns run registry, policy hook, delegation rails, MCP attach, lock order. |
| `runtime/runexec` | The run engine: `Runner.RunWith` (state setup → prompt build → loop config → resume ticket → `agent.Run` → post-run distill/forge/lifecycle). |
| `runtime/lifecycle`, `accessors`, `compose`, `types` | Extracted sub-surfaces of the runtime split. `compose` has no importers and `accessors` is constructed but unused; see findings. |
| `toolreg` | First-party tool registry: specs with Build/PreOpen/Configure/ConfigureLate hooks, conflict yielding, capability merging. |
| `toolexec` | Direct (operator/CLI) tool execution path outside a run. |
| `toolforge` | Agent-authored script tools promoted into durable callable tools (`forge_*`). |
| `contextselect` | Scores and selects context candidates (memory/world/skills/taste); journals `context.selection`. |
| `convo` | Collapses multi-turn chat into one governed intent (lossy by design). |
| `delegation` | Sub-agent spawn bookkeeping, depth/ancestry helpers (+ unused duplicate tool types). |
| `intent` | Turns an utterance into auditable intent metadata for the intent/regret guard. |
| `intervention` | Grammar for safe live changes to a running agent (pause/step/steer/BTW/lease). |
| `planner` | NL intent → `scheduler.Plan` DAG via the provider. |
| `reflect` | Reflection v1: meta-cognition loop over past runs. |
| `resume` | Durable per-run tickets, so in-flight runs survive restart/update/kill (quarantine on poison). |
| `meshctx` | Cross-node delegation hop count (mesh M8). |
| `assure` | Run → verify → retry-with-gap loop ("do it for sure"). |
| `proof` | Durable evidence that a workboard task met its acceptance criteria (Proof gate). |

### 5.2 `kernel/` — governance, routing, security → [05](05-governance-routing-security.md)
| Package | Role |
|---|---|
| `governor` | LLM routing brain: model→provider resolution, `@chain` expansion, per-task/agent chains, budgets (µ¢), rate limits, circuit breaker, retries, response cache, cost accounting. Itself an `agent.Provider`. |
| `catalog` | Live provider/model registry (models.dev sync; `api.json`/`custom.json`/`local.json`). |
| `creds` (+`sigv4`) | Machine-bound AES-256-GCM vault for provider env vars, multi-key keyring `NAME#label`, AWS credential chain; SigV4 signer. |
| `chatgptauth` | OAuth tokens for "Sign in with ChatGPT" (Codex subscription auth). |
| `settings` | `config.json` + schema registry behind the UI Config Center (non-secret operator settings). |
| `configcenter` | Separate agent-facing typed/audited config store with sensitivity + approval gating. Same name as `settings`' UI but a different store. |
| `executionprofile` | Named execution surfaces (host, docker, …) for high-risk work. |
| `edict` | Policy engine + trust ladder: 37 capabilities default L4 (allow), unknown capability ⇒ deny, shell hard-deny floor, trust ceilings, durable overlay. |
| `approval` | In-memory HITL pause point (5-minute timeout ⇒ deny; session auto-approve). |
| `warden` | Process isolation (process groups/rlimits on Linux; optional Docker backend; little isolation on Windows/macOS). |
| `netguard` | Egress SSRF guard checking the resolved IP at dial time (blocks link-local/metadata always). |
| `platform/netout` | The only HTTP client factory: `Egress` (agent tools: host allowlist per redirect + IP guard), `OperatorClient` (configured endpoints: proxy/pooling as DefaultTransport, metadata refused), `MetadataClient` (IMDS/GCE only). |
| `envscrub` | The child-env rules: launch allowlist + secret-name test. |
| `platform/sandbox` | The only way to build a child process outside warden: `Command`/`CommandContext` preset `IsolatedEnv` (allowlist + grants); `HelperEnv` (all but secrets) for operator helper CLIs. |
| `redact` | Secret scrubbing (vault values + patterns), installed on the bus before journaling. |
| `seat` | Named execution "seats" for workboard dispatch. |
| `tenant`, `tenantctx` | Multi-tenancy: one base dir + kernel per tenant; tenant id in `context`. |
| `state` | First-class mutable key/value state namespaces. |

### 5.3 `kernel/` — data, memory and observability → [06](06-data-memory-state.md)
| Package | Role |
|---|---|
| `event` | Canonical event: ULID, seq, BLAKE3 hash, subject/actor/correlation/cause, kind-tagged payload; 164 kind constants. |
| `journal` | Append-only BLAKE3-chained JSONL segments (64 MiB), full re-verify on open, no index (full scans). |
| `bus` | In-process pub/sub. `Publish` journals under lock (+fsync) then fans out non-blocking (slow subscriber drops). `PublishStreaming` skips the journal. |
| `ulid` | 26-char Crockford ULIDs. |
| `platform/filestore` | Tolerant load + atomic private (0600/0700) save for single-file JSON stores, plus a cross-process `Lock` for files the daemon and `agt` both write. |
| `memory` | Content-addressed memory: private by default, shared opt-in, distillation, dedupe, tidy, operator profile facets, keyword + hashed/provider embeddings. |
| `worldmodel` | Journaled graph of the operator's world (entities, relations). |
| `datalake` | File-based structured store ("Personal Data Lake") behind the `db` tool. |
| `artifact` | BLAKE3 content-addressed blob store + index + collection. |
| `board` | Shared topic-addressed message board/mailbox (**one** `board.Store` per daemon). |
| `workboard` | Durable typed task queue with acceptance criteria and the Proof gate. |
| `okr` | Objectives/key results computed live from workboard status. |
| `taste` | Operator-authored "what good looks like" exemplars injected before runs. |
| `anomaly` | Runaway circuit breaker (>120 tool calls / 10 s ⇒ halt). |
| `alerter` | Pushes warning/critical alerts to channels. |

### 5.4 `kernel/` — autonomy and extensibility → [07](07-autonomy-and-extensibility.md)
| Package | Role |
|---|---|
| `roster` | Durable agent profiles: soul, model + fallbacks, task type, trust ceiling, tools/noise policy, system/guardian flag. |
| `cadence` (+`systemtasks`) | Typed schedules waking agents/workflows/system tasks (catalog_sync, artifact_collect, memory_clean/tidy, log_clean, graveyard_scan, profile_distill). |
| `scheduler` | DAG layer above the single-agent loop (plans). |
| `standing` | Standing orders: event/cron triggers → wake bound agent with a task, under a trust ceiling. |
| `pulse` | Proactive heartbeat: observers → importance → tell or act (initiative `off\|ask\|act`). |
| `selfrepair` | Doctor/auto-repair coordinator driven by the reaper observer. |
| `skill` | Forge v1: content-addressed, lifecycle-governed learned skills, SKILL.md bundles, injection. |
| `market` | Capability marketplace: packs, Ed25519 signing, remote sync, vetting, install/uninstall. |
| `plugin` | Out-of-process plugin host (stdio line-JSON, BLAKE3 pins, capability manifest). |
| `mcp` | Durable MCP server registry + client (stdio, Streamable HTTP, lazy dispatcher). |
| `acp`, `acpcatalog` | ACP server (IDEs drive AGEZT over stdio JSON-RPC) and discovery of installed ACP coding agents. |
| `workflow`, `workflowexec` | n8n-style typed node graphs (trigger/tool/llm/condition/transform/delay), store and triggers. **Execution actually lives in `kernel/runtime/workflowrun*.go`**: a sequential breadth-first walk with per-node retry/timeout, every tool/http/code node using the shared governed invoker. `workflowexec` has no importers. |
| `update` | Self-update (endpoint or GitHub, staging, sentinel, watchdog). |
| `toolbox` | Host CLI-tool inventory + cross-platform installer. |
| `channel`, `channelwire` | Channel interface + `UnifiedMessage`, manifest registry, per-kind factories. |
| `stt`, `voicetool`, `imagetool`, `reranktool` | Speech-to-text client; agent-facing voice/image/rerank tools. |
| `internal/testfixtures` | Shared kernel test helpers. |

### 5.5 `kernel/` — surfaces → [03](03-control-plane-and-http.md)
| Package | Role |
|---|---|
| `controlplane` | Daemon↔CLI protocol (TCP loopback, NDJSON, one request per connection, 321 ops in 28 domains) + all domain handlers. |
| `httpserver` | Shared HTTP plumbing: auth levels per route, body limits, timeouts. |
| `webui` | Embedded SPA + 198 fixed route→op mappings, login/password, SSE `/events`, file manager. |
| `restapi` | `/api/v1` REST surface for SDKs (opt-in `AGEZT_REST_ADDR`). |
| `openaiapi` | `/v1/chat/completions`, `/v1/responses`, `/v1/models` (opt-in `AGEZT_API_ADDR`). |
| `agentgw` | Unix-socket gateway for agent subprocess code (HMAC capability tokens, journaled audit). |
| `auth` | Transport-independent credential verification and authority tiers. |
| `streamlimit` | 64 concurrent SSE streams per client IP, process-wide. |
| `tunnel` | Supervises cloudflared/ngrok/tailscale to expose the Web UI/REST. |
| `webhook` | Outbound signed webhooks for matching bus events. |

### 5.6 `plugins/`
| Package(s) | Role | Doc |
|---|---|---|
| `providers/{anthropic,bedrock,cohere,google,ollama,openai,openairesponses,vertex}` | Wire adapters implementing `agent.Provider` (+ streaming) | [08](08-providers.md) |
| `providers/compat` | The **only** factory: catalog entry → live provider (10 families via `npm` field) | [08](08-providers.md) |
| `providers/{embed,voice,image,rerank}` | Modality sidecars (embeddings, STT/TTS, image gen, rerank) | [08](08-providers.md) |
| `providers/mock` | Offline echo provider (tests, `AGEZT_DEMO_ECHO=1`) | [08](08-providers.md) |
| `providers/internal/{httpread,provopts,retry,toolname}` | Bounded bodies, Params/ProviderOptions, retry/Retry-After, injective tool-name sanitising | [08](08-providers.md) |
| `providerboot` | Boot + Reload share one registration path; primary via `AGEZT_PROVIDER` or `unconfigured` sentinel | [08](08-providers.md) |
| `sdk` (+`example/greet`) | Go kit for out-of-process tool plugins | [08](08-providers.md) |
| `external/mcpbridge` | Standalone MCP→plugin-protocol bridge binary (largely superseded by `kernel/mcp`) | [08](08-providers.md) |
| `channels/*` (25 pkgs) | Telegram, Slack, Discord, Matrix, Signal, WhatsApp (Cloud + gateway), email (SMTP/IMAP), IRC/Twitch, SMS, LINE, Feishu, DingTalk, WeCom, Zalo, OneBot (QQ/WeChat), Nostr, Mastodon, Nextcloud Talk, iMessage (BlueBubbles), Teams, Home Assistant, chat-webhook (Google Chat/Mattermost), generic webhook, push (7 providers) | [09](09-channels.md) |
| `builtinchannels` | Registers 34 manifests + factories, multi-account expansion, Connect wizard metadata | [09](09-channels.md) |
| `tools/*` (30 pkgs) | Agent tools (shell, file, http, fetch, browser, websearch, research, code_exec, council, conductor, overseer, board, workboard, workflow, schedule, standing, skill, config, db, artifacts, notify, send_media, mcp, acp_agent, coding, peer, homeassistant, introspect, runs, tool_forge) | [10](10-tools.md) |
| `builtintools` | Registers 32 tool specs in a ratcheted order; external plugin host last | [10](10-tools.md) |
| `builtinguardians` | 7 seeded system guardian agents (health, doctor, stuck, budget, routing, code, initiative) | [10](10-tools.md) |
| `builtinskills` | 16 embedded agentskills.io bundles, seeded + promoted at boot | [10](10-tools.md) |
| `builtinmarket` | Official offline marketplace: 16 single-bundle packs + 12 curated combos | [10](10-tools.md) |

---

## 6. End-to-end flows

### 6.1 Daemon boot (condensed; full 50-step table in [01 §2](01-daemon-boot-cmd-agezt.md))
1. Resolve `AGEZT_HOME`. Refuse to start if a live daemon is probed (unless `AGEZT_FORCE_START=1`).
2. Load and validate the catalog. Load the vault and upgrade it to machine-bound encryption in place.
3. **`injectConfig`**: settings pins + `config.json` + vault `AGEZT_*` values are written into the process env.
   Then `daemonconfig.Load` parses about 75 env vars into a typed `Config` (fatal vs warn per var).
4. `providerboot.Boot` builds the **governor**. Then warden, Edict (ask-policy, hard-deny floor, optional
   allow-all), channel manifests and tool specs are built.
5. **`runtime.Open(cfg)`** opens the journal (full chain re-verify), state, bus and every store.
6. The governor and warden get the bus. Tools are configured. **The redactor is installed on the bus before any run.**
7. Durable policy overlay replay. Orphan reconciliation (`task.abandoned`). Shared board opened.
8. **The control plane starts** (`runtime/control.addr` + `control.token` written). The banner is printed.
9. Channels are built and started. Then Pulse, tickers, Web UI, tunnel, OpenAI API, outbound webhooks,
   anomaly breaker, alerter and REST start.
10. Late boot steps: channel-bound tools, skill seeding + marketplace, guardians seeded, MCP servers attached.
    Then cadence, the update checker, standing orders, the **resumer** (interrupted runs), selfrepair and
    workflow triggers.
11. Block on a signal or `agt shutdown`. Graceful drain: in-flight runs are **suspended** before cancel,
    so they resume on the next boot.

### 6.2 One governed run (condensed; sequence diagram in [04](04-agent-runtime.md))
1. **Entry:** control-plane `run` op (CLI/Web UI), REST/OpenAI engine, channel `InboundHandler`, cadence,
   standing order, workboard dispatch, selfrepair, overseer, or the boot resumer. All of them reach
   `Kernel.RunWith` / `RunAssured` / `RunWithRetry`.
2. **`runexec.Runner.RunWith`:**
   1. Registers the run under a correlation id.
   2. Frames the intent.
   3. Builds the system prompt from the agent profile/soul, operator profile, taste, memory, world model and
      skills. Each injection is journaled as `context.selection`.
   4. Claims a resume ticket.
3. **`agent.Run` loop**, each iteration:
   1. Snapshot for resume.
   2. Apply steering/BTW/intervention at the boundary.
   3. **governor.Complete**: chain resolution → pre-flight (overrides, capability gate, rate limit,
      budgets, pricing) → provider → retry/fallback → cost.
   4. For each tool call, sequential gating: lookup → schema check → duplicate guard → **policyHook**
      (capability resolution → Edict decision with trust ceiling → per-agent hard denies → opt-in guards
      → `approval.Submit` if "ask") → read-only memo.
   5. Then **parallel execution** (default 4). Results go back to the model in the original call order.
   6. Old tool outputs are compacted (journal copy only).
4. **Events:** `policy.decision` always precedes `tool.invoked`; every call gets a `tool.result`; exactly
   one terminal event per run.
5. **Post-run:** skill outcome/forge/shadow-eval, memory distillation, lifecycle hooks, optional assure
   verdict / workboard Proof.

### 6.3 Event path
1. A subsystem publishes on the `bus`.
2. Unless the event is streaming, the bus redacts it, appends it to the `journal` (hash chain, fsync) and
   fans it out.
3. Subscribers include:
   - Web UI `/events` SSE: the whole main-kernel bus, not tenant-filtered.
   - Control-plane streams.
   - Outbound webhooks.
   - Pulse observers.
   - Anomaly breaker and alerter.
   - Workflow triggers and standing orders.
   - The daemon stdout log.
4. `agt why <id>` walks `cause`/`correlation` links backwards over the journal (full scans; there is no
   index). See [06](06-data-memory-state.md).

### 6.4 Inbound channel message
1. A channel `Start` loop (polling, webhook, websocket or IMAP) normalises the input to `UnifiedMessage`.
2. The shared `InboundHandler` (`cmd/agezt/main_channels_handler.go`) adds conversation history, describes
   images and transcribes voice.
3. It calls `k.RunWith` with correlation `chan-<ULID>`.
4. The reply goes back through `Channel.Send` (optionally as voice).
5. Allowlists gate who can trigger runs. See [09](09-channels.md) for per-channel transports and caveats.

### 6.5 Autonomy (who wakes agents without a human)
- **Cadence** schedules: cron, one-shot, daily, continuous.
- **Standing orders**: event or cron triggers.
- **Pulse**: observers produce observations. When initiative is `act`, a `pulse.initiative.act` event
  leads to a standing order and a governed run.
- **Self-repair / guardians**: system agents with clamped budgets and trust L2.
- **Workflow triggers**: events and `/hooks` webhooks.
- **Workboard dispatch**: seats.

All wakes go through the same `RunWith` path, so all are governed and journaled.
Agents do not run between wakes: a `roster.Profile` is just a record. The *when* lives in
`cadence`/`standing`/`pulse`; the *what* (the fire closures) lives in `cmd/agezt` (`main_cadence.go`,
`main_standing.go`, `makeChannelHandler`). Pulse never acts on its own: the bound `guardian-initiative`
standing order ships disabled. System-task cadences (catalog sync, memory tidy, …) bypass Edict;
their safety rests on a closed task list and operator-only creation.
See [07](07-autonomy-and-extensibility.md).

---

## 7. Persistence overview (`AGEZT_HOME`, default `~/.agezt`)

There are **two persistence tiers**:
1. The **journal**, the only immutable record (`journal/NNNNNNNN.jsonl`, 0600/0700).
2. **Mutable JSON projections**, each owned by one package and rewritten whole and atomically through
   `filestore`/`internal/atomicfile`.

Each store must be opened **once** per daemon. Two instances of the same store silently overwrite each
other; the board is the best-known case of this.

| Path | Owner | Doc |
|---|---|---|
| `journal/`, `state/` | journal, state | 06, 05 |
| `catalog/{api,custom,local,meta}.json` | catalog | 05, 08 |
| `creds.json` (encrypted vault) | creds | 05 |
| `config.json`, `schemas/` | settings | 05 |
| `configcenter/` | configcenter | 05 |
| `memory/`, `worldmodel/`, `datalake/`, `artifacts/` (+`index/`) | memory, worldmodel, datalake, artifact | 06 |
| `board/board.json`, `workboard/`, `okr/`, `taste/` | board, workboard, okr, taste | 06 |
| `skills/skills.json`, `skills/bundles/` | skill | 07 |
| `market/{installed,sources}.json`, `market/marketplaces/` | market | 07 |
| `roster/`, `cadence/`, `standing/`, `workflows/`, `mcp/`, `toolforge/`, `seats/` | respective packages | 07 |
| `resume/` (+`quarantine/`) | resume | 04 |
| `runtime/control.addr`, `runtime/control.token`, `runtime/edict_overlay_snapshot.json` | controlplane, edict | 03, 05 |
| `web-password`, `openai.token`, `rest.token` | webui, openaiapi, restapi | 03 |
| `tenants/<id>/…` | tenant (full nested home per tenant) | 05 |
| `workspace/`, `sandbox/`, `browser-sessions/` | file/shell, code_exec, browser | 10 |
| `bin/`, `update.lock`, `update.sentinel` | update, watchdog | 01, 07 |

---

## 8. Cross-cutting invariants (the "laws" encoded in code and guard tests)

| Invariant | Where enforced |
|---|---|
| Nothing is journaled unscrubbed: the redactor is installed on the bus before any run | `cmd/agezt` boot step 21; [01](01-daemon-boot-cmd-agezt.md) |
| `policy.decision` precedes every `tool.invoked`; exactly one terminal event per run; panic firewall in the loop | `kernel/agent`; [04](04-agent-runtime.md) |
| **Default-allow** posture: every known capability is L4 by default; restriction is opt-out. Hard denies, SSRF guards, budgets and explicit HITL stay | `kernel/edict`; [05](05-governance-routing-security.md) |
| **Unmapped tool ⇒ unknown capability ⇒ default-deny** (unless `AGEZT_ALLOW_ALL`); guard tests turn this into a test failure | `plugins/builtintools` guards; [10](10-tools.md) |
| No default provider/model: `unconfigured` sentinel until `AGEZT_PROVIDER`/catalog keys exist | `plugins/providerboot`; [08](08-providers.md) |
| Recoverable config mismatches warn and degrade; they never hard-fail boot | `daemonconfig`, boot table; [01](01-daemon-boot-cmd-agezt.md) |
| New `AGEZT_*` env vars read in `cmd/agezt` must be added to controlplane `configEnvVars` | controlplane guard test; [01](01-daemon-boot-cmd-agezt.md), [03](03-control-plane-and-http.md) |
| Memory and skills are private to the authoring agent by default; sharing is opt-in | `memory`, `skill`; [06](06-data-memory-state.md), [07](07-autonomy-and-extensibility.md) |
| One shared `board.Store` per daemon (generalises to every JSON store) | boot step 26; [06](06-data-memory-state.md) |
| Suspend-before-cancel on shutdown, so interrupted runs resume | `runtime`, `resume`; [04](04-agent-runtime.md) |
| Committed `kernel/webui/dist` must match the frontend sources | CI `frontend-dist-in-sync`; [11](11-frontend-console.md) |
| Nav IA: section = job, row = noun, tab = facet; the view id is a permanent address; ⌘K keywords mandatory | `nav.test`, `consoledoc.test`; [11](11-frontend-console.md) |
| Every channel manifest has a factory; every HTTP inbound listener rejects requests when its secret is empty | `builtinchannels` guard tests; [09](09-channels.md) |
| Third-party deps must be allowlisted and justified | `tools/depscheck`, `DEPENDENCIES.md`; [01](01-daemon-boot-cmd-agezt.md) |

---

## 9. Verified findings register

The analysis surfaced defects, drift and dead code. Each was **read from code but not runtime-verified**
unless the source document says otherwise. They are collected here so the codemap doubles as an audit
backlog. Details and file references are in the linked documents.

### 9.1 Security / correctness (highest signal)
| Finding | Area |
|---|---|
| Inbound **email trusts the `From:` header** (no DKIM/SPF, W4.4); ✅ case-sensitive allowlist fixed (W0.3). IRC/Twitch allowlist whole `#channel`s (any viewer can trigger billable runs). | [09](09-channels.md) |
| ✅ **Fixed (W0.3):** Channel `Start` errors were ignored (`go ch.Start(ctx)`): a dead channel was still reported live. IRC/email/Mastodon loops lacked `channel.Guard`, so a handler panic crashed the daemon. | [09](09-channels.md) |
| ✅ **Measured (W2.0):** the REST/OpenAI "bypass" is mostly missing features (they offer no `--tools`, execution profiles or `--agent`). The real outlier was the control plane's own direct agent run, which skipped the agent's tool deny-list and trust ceiling; fixed. ✅ **Fixed (W2.2a):** image admission is shared: REST/OpenAI now caption through the configured sidecar, and all three adapters journal correlated rejections. Channels already captioned; their empty-caption and missing-audit drift is fixed. | [03](03-control-plane-and-http.md) |
| ✅ **Fixed (W2.1a):** about 40 state-changing control-plane ops (provider keys, config-center entries and ACLs, schedules, routing, data-lake writes, deletions) left **no journal event**. Dispatch now journals every non-read-only op (`op.invoked` / `op.completed` / `op.failed`, secrets redacted). | [03](03-control-plane-and-http.md) |
| ✅ **Fixed (W0.3):** REST `/metrics`, `/api/v1/health` and `/api/v1/models` answered tenant tokens from the primary kernel. **Correction:** the Web UI `/events` stream being unfiltered is not a leak — the console admits only operator credentials. | [03](03-control-plane-and-http.md) |
| Web UI File Manager and rollback restore write the filesystem with **no op, no policy check, no journal event**. | [03](03-control-plane-and-http.md) |
| ✅ **Fixed (W0.3):** Login lockout counter was global: 8 bad passwords from anyone locked everyone out for 5 minutes. Now per client + global backstop. | [03](03-control-plane-and-http.md) |
| ✅ Fixed (W1.8): `configcenter` stored raw values, secrets included, in plaintext `entry_*.json` (0644, non-atomic); secrets now live in the vault and files are 0600/atomic. Its loader never matched a file name (`[:7]` vs six-byte `"entry_"`), so **no entry survived a restart**; fixed. `Get` held a read lock across a 5-minute approval wait, freezing the center; fixed. Config access is now journaled (`config.access`). | [05](05-governance-routing-security.md) |
| ~~Warden Docker backend passes env secrets as `-e NAME=VALUE` argv~~ ✅ Fixed (W1.5): secrets go by name. Warden isolation is nominal on Windows/macOS. | [05](05-governance-routing-security.md) |
| ✅ **Fixed (W1.3):** vault load-modify-save had no file lock, so daemon and `agt` lost each other's updates; now merge-on-save under a cross-process lock. | [05](05-governance-routing-security.md) |
| Seat `"container"` never maps to the container execution profile (dispatch only knows `"docker"`). | [05](05-governance-routing-security.md) |
| ~~`coding` tool runs `git` with the full daemon env~~ ✅ Fixed (W1.5): sandbox `IsolatedEnv`; toolbox installers/tunnels/probes also stopped inheriting. `coding`/`acp_agent`/browser bypass warden. Browser has a DNS-rebinding window. ~~`fetch` has no host allowlist~~ ✅ Fixed (W1.4): it takes the http tool's posture. `shell` `timeout_ms` is uncapped. | [10](10-tools.md) |
| ✅ **Fixed (W0.3):** Out-of-process plugins inherited the **full daemon environment**; now scrubbed base + `AGEZT_PLUGIN_ENV` grants. ✅ **Fixed (W1.5):** plugins are closed at shutdown (`toolreg.Set.Close`). | [07](07-autonomy-and-extensibility.md) |
| **Self-update cannot apply today**: `DefaultPublicKeyHex` is empty, the GitHub check never fills a SHA-256, apply paths drop the signature (W4.5). ✅ **Fixed (W0.4):** the checker halted the kernel *before* verifying and never resumed it on failure. | [07](07-autonomy-and-extensibility.md) |
| Market install verifies signatures with **no pinned key** (`VerifyPack(p, "")`); authenticity only comes from the source pin at sync time. Partial install failures are not rolled back. | [07](07-autonomy-and-extensibility.md) |
| **Partially fixed (W2.3a):** workflow registered-tool, HTTP, pipeline and canvas-node calls share `RunTool`/`toolexec`, with policy/tool audit, resolved capability metadata and correlated approvals. Direct denials emit terminal `tool.result`; direct lookup includes forge/MCP. ✅ **Fixed (W2.3b):** Council grounding uses the shared invoker and honors web-search level, agent deny-list, trust ceiling and correlated approvals; failed/refused searches preserve date-only deliberation. ✅ **Fixed (W2.3c):** Conductor and workflow code runners use invocation-local adapters through the same governed invoker; denial/audit failure prevents execution, correlated approval and terminal audit apply, and Conductor reports actual executor entry. **W2.3d foundation:** the direct invocation panic firewall lives in `platform/toolinvoke` with unchanged behavior. ✅ **Fixed (W2.3e):** agent-loop execution shares the platform primitive, releases call contexts and settles terminal batches before typed task failure. Sequential abort prevents later effects, skipped results have no invented latency, and terminal hooks/model work are suppressed. **W2.3f foundation:** agent offload representation is shared in `platform/tooloutput` with unchanged behavior. ✅ **Fixed (W2.3g):** runtime invocations share artifact-backed terminal audit via RunWithOptions while full caller/hook bytes, error causes and inline fallback remain intact. **W2.3h foundation:** pure verdict/callback contracts live in `contract/policyapi` and unchanged loop policy rendering lives in `platform/toolaudit`. ✅ **Fixed (W2.3i):** direct policy records share all 23 loop fields, retaining resource/epistemic/observation details and nil/zero representation through the common renderer. **W2.3j foundation:** unchanged schema/policy-context helpers have platform homes and agent forwarding compatibility. **W2.3k repointing:** direct preflight consumes those platform helpers; its production dependency closure no longer includes agent. Full app invoker convergence remains W2.3. | [04](04-agent-runtime.md) |
| ✅ **Fixed (W0.4):** `tool_search` and `browser.action` (+10 verbs) declared no capability; `tool_search` was default-denied. Guards now build opt-in tools. | [04](04-agent-runtime.md), [10](10-tools.md) |
| ✅ **Fixed (W0.4):** Provider retry never matched the adapters' `TransientError` wrapper, so connection-refused/reset/DNS errors were **not retried**. | [08](08-providers.md) |
| Anthropic thinking blocks are not replayed across tool turns. Bedrock-Anthropic discards thinking (billed, not shown). Vertex `claude-*` ignores JSON mode. Bedrock non-Anthropic streaming fails. ~~Inconsistent SSRF clients across~~ ✅ Fixed (W1.4, netout) — adapters. | [08](08-providers.md) |
| Agent SDK default socket `@agezt/agentgw.sock` ≠ the daemon's random `agentgw-<hex>.sock` (never published). Python `AgentClient` ignores HTTP status. Python `agent.py` star-import raises. The 30 s SDK timeout cuts long blocking runs. | [12](12-sdks-and-contract.md) |

### 9.2 Reliability / data
| Finding | Area |
|---|---|
| ✅ Fixed (W1.6c): one corrupt mid-journal line aborted `runtime.Open`; it is now quarantined and the gap journaled. Open: no journal index — `agt why` = 3 full scans (measured, deferred: W1.6b). | [06](06-data-memory-state.md) |
| Operator-profile facet text changes create a second active record (the old one is never superseded); both are injected into every run. | [06](06-data-memory-state.md) |
| ✅ **Fixed (W0.4):** Anomaly breaker disarmed after one trip and was not re-armed after resume. Anomaly/alerter watchers died silently on panic. | [06](06-data-memory-state.md) |
| Artifact GC can delete blobs still referenced by journal `raw_ref`. ~~JSON stores are written 0644~~ ✅ Fixed (W1.3): 0600/0700. Failed distillation is journaled as `memory.written`. | [06](06-data-memory-state.md) |
| ✅ **Fixed (W2.2b):** new root runs with profile-sourced soul/model values remain resumable. Provenance is tied to the agent slug; explicit per-run system/model/tools overrides stay non-resumable. The control plane no longer re-copies profile defaults as explicit overrides. Older non-resumable tickets are still quarantined. | [04](04-agent-runtime.md) |
| `epistemicGate` scans the whole journal on every gated tool call, even when escalation is off. | [04](04-agent-runtime.md) |
| Conversation history misses agent replies on most channels (only Telegram/Slack/Discord/Matrix/Signal fold them back). | [09](09-channels.md) |
| Built-in skills re-promote a quarantined (unchanged) built-in skill at next boot. Guardian reconcile resets operator-lowered spend caps to $5/$10 every boot. `SelfRepairPolicy.EscalateTo` is ignored. Pulse initiative level is neither live-changeable nor persisted. Cron/event workflow runs are labelled `source:"manual"`. `reranktool` can panic on a short `scores` slice. | [07](07-autonomy-and-extensibility.md) |
| Channel conversation continuity is a journal fold per sender/thread that **re-scans the whole journal for every inbound message** (there is no session object). | [07](07-autonomy-and-extensibility.md) |

### 9.3 Drift, dead code, docs
| Finding | Area |
|---|---|
| Kernel→plugins edge: `kernel/controlplane`, `kernel/selfrepair` → `plugins/tools/overseertool`. Module→adapter edge: `kernel/runtime` (+`accessors`, `runexec`) → `kernel/agentgw`. All 200 forbidden edges are now tracked by `tools/archcheck` (ratchet). | §4.1, [20](20-target-architecture.md) |
| ✅ **Removed (W0.5):** `kernel/workflowexec` and `kernel/runtime/compose` had no importers. Open: `acpcatalog.ResolveLaunch` and plugin host callbacks/`Reload` have no production caller. `runtime/accessors` is constructed but unused. `delegation` tool types duplicate runtime's. `ErrHalted`/`ErrNoVisionModel` are each defined twice. | [04](04-agent-runtime.md) |
| ✅ **Fixed (W0.5):** `contract/gen` CI drift check could never fail (gitignored output + `git diff`); it now builds the package. Open: `sdkparity` only checks report freshness. Fixtures are self-validating only. | [12](12-sdks-and-contract.md) |
| `designsystem.test.ts` scans only `components/`, not `features/`. 9 frontend modules are used only by their tests. Monaco loads from a CDN the CSP blocks. Fake file tree on 404. Observe badge never clears. | [11](11-frontend-console.md) |
| ✅ **Fixed (W1.6a):** never-emitted kinds deleted, ad-hoc `event.Kind("…")` strings (market included) made constants; `TestKindRegistryIsClosed` keeps the registry closed. | [06](06-data-memory-state.md), [07](07-autonomy-and-extensibility.md) |
| Stale package docs: `governor` (`PreferredProvider`), `creds` ("unencrypted"), Edict "ask-first" comments, several provider docs ("unsupported"), `journal` (sidecar index), email ("outbound-only"), `restapi` (doc lost). | [05](05-governance-routing-security.md), [06](06-data-memory-state.md), [08](08-providers.md), [09](09-channels.md) |
| ✅ **Fixed (W0.5):** `AGEZT_BROWSER_COOKIES` was never read — a lost feature, rewired. Open: ~15 channel `RequiredEnv` values disagree with factory gates. qq/wechat/zalo have no Config Center section. Boot banner says 6 guardians, 7 ship. | [09](09-channels.md), [10](10-tools.md) |
| CI aggregator `success*` match may not evaluate `needs.*.result`. `gofmt` roots differ between CI and `make fmt`. `scripts/webui-e2e.sh` still sets `AGEZT_MODEL=mock`. | [01](01-daemon-boot-cmd-agezt.md), [11](11-frontend-console.md) |

---

## 10. Glossary

| Term | Meaning |
|---|---|
| **Kernel** | `runtime.Kernel`, the in-process aggregate of journal, bus, stores, loop, governor and tools that a daemon (or tenant) hosts. |
| **Run / correlation id** | One governed execution of an intent. Every event of a run (and its sub-agents, policy decisions, tool calls) shares the correlation id. |
| **Governor** | Routing/budget layer in front of every model call; itself an `agent.Provider`. |
| **Chain / `@name`** | Ordered model fallback ladder (per task, per agent, named and reusable); expanded at one point in the governor. |
| **Edict** | Policy engine. Capability + trust level (L0–L4) → allow / ask / deny. |
| **Capability** | Policy axis a tool call maps to (e.g. `shell`, `http.post`, `code.exec`, `mcp.call`). |
| **Trust ceiling** | Upper bound on trust for a run (standing order, agent profile). It can only lower the level. |
| **HITL / approval** | Human-in-the-loop pause created when Edict says "ask" or a guard forces it. |
| **Warden** | Process isolation layer for tools that spawn processes. |
| **Netguard** | Egress SSRF guard. |
| **Journal** | Append-only BLAKE3 hash-chained event log; the system of record. |
| **Bus** | In-process pub/sub; durable publishes are journaled before fan-out. |
| **Roster / agent profile** | Durable named agent identity (soul, model, policies). |
| **Guardian** | Built-in system agent of the self-healing fleet. |
| **Soul** | An agent's persona/system prompt. |
| **Standing order** | Durable trigger rule: "when X (event/cron), wake agent with task Z". |
| **Cadence** | Typed schedule subsystem. |
| **Pulse** | Proactive heartbeat that observes, ranks importance, and tells or acts. |
| **Forge / skill** | Learned procedure with a governed lifecycle (draft→shadow→active→quarantined/archived). |
| **Pack / marketplace** | Installable bundle of skills + MCP servers + tool requirements; catalogue of packs. |
| **Workboard / seat / Proof** | Typed task queue; execution seat for a task; evidence gate before done. |
| **Assure** | Run → verify → retry-with-gap loop. |
| **Taste** | Operator exemplars of good output, injected before runs. |
| **World model** | Graph of the operator's projects, people, accounts and relations. |
| **Data lake** | File-based structured store exposed via the `db` tool. |
| **Board / mailbox** | Shared topic-addressed message board between agents and the operator. |
| **Control plane** | Loopback TCP NDJSON protocol between `agt`/Web UI and the daemon. |
| **agentgw** | Unix-socket gateway for agent-spawned subprocess code. |
| **ACP / MCP** | Agent Client Protocol (IDE ↔ agent); Model Context Protocol (tool servers). |
| **µ¢ (microcents)** | Money unit on the wire: $1 = 1e9 µ¢. |
