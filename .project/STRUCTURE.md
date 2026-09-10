# Agezt — Repository Structure (STRUCTURE.md)

> **This document is generated.** Run `make structure-md` (or `go run ./tools/structure-md`)
> to regenerate from the actual filesystem. Do not edit by hand — the source of truth
> is each package's `doc.go` (Go) or top-of-file comment. CI runs the generator and
> fails on drift via `make check`.
>
> The map below reflects the **actual** repository. Historical SPEC-01 (gRPC
> over UDS) and the older STRUCTURE.md have been superseded by DECISIONS B0
> (stdio + JSON-RPC + JSON Schema) and by 900+ M-phase work; the
> `legacy-STRUCTURE.md` and `legacy-SPEC-01.md` siblings are kept only for
> archaeology. **Do not use the legacy versions as architecture references.**

## Top-level layout

```
agezt/
├── LICENSE                      # MIT
├── README.md                    # user-facing entry
├── DEPENDENCIES.md              # every dep justified (POLICY §1)
├── Makefile                     # build/test/lint/gen/structure-md/check
├── go.mod / go.sum              # module github.com/agezt/agezt
├── .github/workflows/           # CI: build/test/lint/check/release
│
├── .project/                    # design + planning
│   ├── DECISIONS.md             # ⛔ FROZEN — supersedes every SPEC where they conflict
│   ├── ROADMAP.md               # milestone plan (M0.5 → M8)
│   ├── STRUCTURE.md             # this file (generated)
│   ├── agezt-contract.jsonc     # wire contract source of truth (JSON Schema)
│   ├── SPEC-01..16              # the spec suite (append-only; superseded parts marked)
│   ├── PHASE-M*.md              # per-milestone reports (M918+ are current; earlier is archive)
│   └── legacy-*.md              # superseded by DECISIONS — read-only
│
├── contract/                    # generated Go types from agezt-contract.jsonc
│   ├── fixtures/                # test fixtures of the wire contract
│   └── gen/types.gen.go         # ⛔ DO NOT EDIT — produced by `make gen`
│
├── cmd/
│   ├── agezt/                   # kernel/daemon binary (main.go: boot, signal, drain)
│   └── agt/                     # CLI client (TCP loopback → control plane)
│
├── internal/                    # stdlib-style utilities; importable from anywhere
│   ├── atomicfile/              # atomic file write (tmp+fsync+rename, Windows retry)
│   ├── brand/                   # frozen identity: Name, Binary, CLI, EnvPrefix, ProtocolVersion
│   ├── paths/                   # BaseDir resolver ($AGEZT_HOME || ~/.agezt)
│   └── strutil/                 # rune-safe truncation, config fallback chains
│
├── kernel/                      # the runtime — see "Kernel package map" below
│
├── plugins/                     # built-in plugin set — see "Plugin layout" below
│
├── sdk/                         # client SDKs (Go first; ts/py/rust)
│
├── frontend/                    # React 19 + Vite SPA — see "Frontend layout" below
│
├── tools/                       # build-time generators + linters
│   ├── changelog-lint/          # changelog gate
│   ├── changelog-split/         # changelog splitter
│   ├── deadcodecheck/           # dead-code detector
│   ├── depscheck/               # dependency allowlist enforcer
│   ├── jsonschemagen/           # contract → Go types (make gen)
│   ├── sdkparity/               # SDK cross-language parity report
│   └── structure-md/            # this file's generator (make structure-md)
│
├── ops/                         # operational scripts (deploy, backup, …)
├── scripts/                     # dev + e2e scripts
├── examples/                    # example plugin / schedule / run specs
├── plans/, CHANGELOG/           # planning + release history
├── docs/                        # user-facing docs (CONSOLE.md, CONNECT.md, …)
│
└── tmp/                         # build/test scratch (gitignored)
```

## Kernel package map (81 packages)

`kernel/` is the runtime. It is organized as a layered composition root that
points everything inward toward `event` + `bus` + `journal` + `state` (the
non-negotiable four; see DECISIONS B0c). The `go list ./...` of `kernel/` and
its auto-extracted doc-comment summaries are reproduced in
`STRUCTURE.kernel.md` (regenerated alongside this file).

Top-level grouping, with the in-tree layout that this commit matches:

```
kernel/
├── Foundation (4)              # every other kernel package depends on these
│   ├── event/                  # canonical event.Spec, kinds, redactor seam
│   ├── bus/                    # pub/sub; durable-before-publish invariant
│   ├── journal/                # append-only JSONL + BLAKE3 chain + ULID
│   └── state/                  # namespaced KV (FileStore today, CobaltDB-ready)
│
├── Composition root (1)
│   └── runtime/                # Kernel{} + Open/Close/Suspend/Reload; wires all
│
├── Brain (2)
│   ├── agent/                  # canonical Provider/Tool + first-party single-agent tool-loop
│   └── governor/               # routing, budget, fallback chain, breaker, cache, pricing
│
├── Wire surfaces (5)
│   ├── controlplane/           # TCP/loopback + JSON for agt CLI
│   ├── webui/                  # HTTP front-end on top of controlplane
│   ├── restapi/                # generic REST surface
│   ├── openaiapi/              # OpenAI-compat relay
│   └── httpserver/             # transport-level HTTP primitives (auth, body cap, …)
│
├── Plugin host + SDK (2)
│   ├── plugin/                 # stdio + JSON-RPC 2.0 host, frame/callback/tool caps
│   └── (sdk/                   # lives at repo root, not under kernel/)
│
├── Safety + sandbox (5)
│   ├── edict/                  # policy engine + trust ladder
│   ├── warden/                 # process sandbox profiles (none/namespace/container/microvm)
│   ├── approval/               # HITL approval queue
│   ├── redact/                 # secret scrubber (deterministic; journal-stable)
│   └── envscrub/               # child-process env builder (allowlist minus secrets)
│
├── Identity + context (5)
│   ├── tenant/                 # multi-tenant storage + lifecycle
│   ├── tenantctx/              # context-stamped tenant id
│   ├── meshctx/                # cross-node hop counter (X-Agezt-Mesh-Hop)
│   ├── auth/                   # tier-based verifier (Public/User/Admin)
│   └── netguard/               # egress guard (loopback/RFC1918/ULA/metadata block)
│
├── Self-governance (4)
│   ├── anomaly/                # rate circuit breaker (default >300 tool-call/5min)
│   ├── assure/                 # do-it-for-sure run/verify/retry
│   ├── proof/                  # durable workboard evidence
│   └── selfrepair/             # auto-repair coordinator (allowed to import runtime)
│
├── Reliability + ops (3)
│   ├── intervention/           # halt/abort/redirect/... wire contract
│   ├── update/                 # self-update with Provenance enum (UPD-001 fix)
│   └── streamlimit/            # backpressure / per-stream rate limits
│
├── Persistence stores (12)     # each opens its own <base>/<name>/ subdir
│   ├── catalog/                # models.dev sync + 3-layer merge (api/local/custom)
│   ├── creds/                  # vault (AES-256-GCM) + keyring + chain lookup
│   ├── jsonstore/              # shared Load/Save primitive for the stores below
│   ├── memory/                 # tiered memory (Store pure + Manager/Graph bus-wrapped)
│   ├── worldmodel/             # knowledge graph
│   ├── skill/                  # skill bundles (BLAKE3 content-addressed)
│   ├── taste/                  # operator preferences
│   ├── standing/               # standing orders (event triggers)
│   ├── okr/                    # objectives & key results
│   ├── workboard/              # typed task queue (8-state lifecycle + proof gate)
│   ├── board/                  # agent-to-agent message board
│   └── market/                 # plugin marketplace registry
│
├── Orchestration + automation (5)
│   ├── scheduler/              # DAG executor (layer 2 over agent loop — DECISIONS B0d)
│   ├── cadence/                # cron/interval/event/condition/webhook triggers
│   ├── workflow/               # workflow engine (nodes, drafts, runs, history)
│   ├── workflowexec/           # workflow node executors
│   └── planner/                # intent → DAG meta-agent
│
├── Capability surface (8)
│   ├── toolreg/                # tool registry
│   ├── toolforge/              # "forge" new tool scripts
│   ├── toolexec/               # direct/CLI tool-call path (no agent loop)
│   ├── toolbox/                # install/upgrade binaries
│   ├── market/                 # (see persistence) — also install surface
│   ├── mcp/                    # MCP server registry
│   ├── channel/                # channel registry (inbound/outbound)
│   └── channelwire/            # unified message wire shape
│
├── Domain modules (~20)        # leaf features over the foundation
│   ├── roster/                 # agent identities (system/user/subagent)
│   ├── seat/                   # org positions
│   ├── pulse/                  # heartbeat + observers + salience + initiative
│   ├── reflect/                # reflection loop (M8)
│   ├── council/                # multi-model deliberation
│   ├── conductor/              # role-based orchestration
│   ├── research/               # research flow
│   ├── intent/                 # intent capture
│   ├── contextselect/          # context budget selector
│   ├── delegation/             # sub-agent delegation
│   ├── convo/                  # conversation surface
│   ├── episodic (reflect-related)/
│   ├── acp/                    # Agent Client Protocol
│   ├── acpcatalog/             # ACP catalog bridge
│   ├── agentgw/                # agent-side IPC gateway (used by SDK agent subs)
│   ├── voice/                  # voice companion
│   ├── stt/                    # speech-to-text
│   ├── voicetool/              # TTS / voice I/O
│   ├── imagetool/              # image generation/editing
│   ├── reranktool/             # reranking
│   ├── alerter/                # alert surface
│   ├── webhook/                # webhook dispatcher
│   ├── tunnel/                 # cloudflared / tailscale / mesh
│   ├── openaiapi/              # (see wire surfaces)
│   ├── datalake/               # typed collections (file-backed Mnesia-like)
│   ├── artifact/               # content-addressed blob store
│   ├── configcenter/           # config UI surface (read-side, not policy)
│   ├── settings/               # settings store
│   ├── executionprofile/       # shell/python/runtime profiles
│   ├── chatgptauth/            # ChatGPT subscription OAuth (M5)
│   ├── proof/                  # (see self-governance)
│   ├── journal/                # (see foundation)
│   ├── ulid/                   # ULID generation
│   └── internal/               # kernel-private helpers (NOT for outside import)
│
└── .well-known-extension-pt/   # reserved
```

**Note on the mapping above.** The 81 kernel packages observed by
`go list ./kernel/...` are split into the buckets here for human orientation.
The generator does not enforce the bucket labels — it only extracts the
package name + its `doc.go` first line. If you add a new package, write a
`doc.go` (one paragraph) and the generator picks it up.

**Renamed from earlier SPEC/STRUCTURE drafts.** These three renames happened
during the M0.5 → M1 evolution and the old names must not be reintroduced:

- `kernel/conduit/` → `kernel/governor/` (M0.5; "Conduit" became "Governor"
  to avoid confusion with the bus's "conduit" / pubsub role)
- `kernel/chronos/` → `kernel/cadence/` (M1; the "cron resident" landed
  under the scheduler-adjacent name)
- `kernel/pluginhost/` → `kernel/plugin/` (M1; merged with the SDK
  base/serve helpers; the directory is the host itself, not a host-builder)

## Plugin layout

Plugins run as separate processes over stdio + JSON-RPC 2.0 (DECISIONS B0).
The kernel's `plugin/` package is the host; the in-tree built-ins are in
`plugins/`. Third-party plugins land under `plugins/external/` or are
declared at runtime via the `AGEZT_PLUGINS` env.

```
plugins/
├── builtinchannels/    # inbound/outbound channel adapters
├── builtinguardians/   # self-healing system agents (System=true)
├── builtinmarket/      # built-in marketplace entries
├── builtinskills/      # 16 markdown SKILL.md files (dataanalysis, webresearch, …)
├── builtintools/       # 10 first-party tool implementations
├── channels/           # community / external channel plugins
├── external/           # cross-vendor adapters (mcpbridge, …)
├── providerboot/       # one-time provider bootstrap (ChatGPT subscription, …)
├── providers/          # 148 LLM provider adapters
├── tools/              # 141 tool plugins (research, fetch, browser, …)
└── sdk/                # plugin-author SDK
```

The directory is **flat by family**, not nested by interface, because the
seven plugin interfaces (Channel / Provider / Tool / CodingAgent / Memory
/ Storage / Tunnel — DECISIONS B0b) are not equally populated: Tools and
Providers dominate, Channels are a separate concern, Memory/Storage/Tunnel
live behind storage abstractions in `kernel/` proper.

## Frontend layout

```
frontend/
├── src/
│   ├── App.tsx                # root shell (hash router, theme, providers)
│   ├── main.tsx               # createRoot + global provider stack
│   ├── nav.tsx                # NAV_GROUPS registry: 8 sections → 30+ rows → 67 views
│   ├── index.css              # design tokens (Tailwind 4 @theme inline)
│   │
│   ├── components/            # 45 files, ~7,874 LOC
│   │   ├── ui/                # primitives: button, card, page, modal, …
│   │   ├── AppNav.tsx         # two-level rail (M974)
│   │   ├── Header.tsx         # status bar
│   │   ├── Vitals.tsx         # daemon vitals strip
│   │   ├── CommandPalette.tsx # ⌘K
│   │   ├── HelpDrawer.tsx     # page-aware help (?)
│   │   ├── Inspector.tsx      # LLM/tool traces (Ctrl+Shift+I)
│   │   ├── MiniChat.tsx       # overlay chat
│   │   ├── Widgets.tsx        # data viz primitives (Ring/Sparkline/…)
│   │   ├── AgentDetail/       # 15-tab agent page
│   │   ├── RunDetail/         # run page + steering
│   │   └── …                  # domain widgets
│   │
│   ├── views/                 # 20 page-level entry points (route targets)
│   │   ├── Chat/              # the canonical conversation surface
│   │   ├── Connections.tsx    # cockpit + Provider Keys tab
│   │   ├── Models.tsx, Routing.tsx, Channels.tsx, …  # Connect surface
│   │   ├── Roster.tsx, Agents.tsx, Skills.tsx, …     # Fleet surface
│   │   ├── Memory.tsx, Taste.tsx, World.tsx, …       # Knowledge surface
│   │   ├── Backup.tsx, Setup.tsx, ConfigCenter.tsx, … # Admin surface
│   │   └── …                  # all lazy-imported via nav.tsx
│   │
│   └── lib/                   # ~100 support files
│       ├── api.ts             # getJSON/postAction + bearer-token scrubbing
│       ├── events.tsx         # single EventSource + subscribe() context
│       ├── usePanel.ts        # mount-fetch + manual reload
│       ├── cursorPager.ts     # generic ?cursor=&limit= pager
│       ├── chatStore.tsx      # cross-navigation chat state
│       ├── councilStore.ts    # module-level council event accumulator
│       ├── conductorStore.ts  # ditto
│       ├── commands.ts        # ⌘K item builder
│       ├── nav.ts             # hash→view resolver
│       ├── agentnav.ts        # #agent/<slug> deep links
│       ├── incidentnav.ts     # #incident/<id> deep links
│       ├── appearance.ts      # theme/console-name bundle I/O
│       ├── configbackup.ts    # config bundle I/O
│       ├── theme.ts           # theme tokens
│       ├── accent.ts          # accent-hue picker
│       ├── brand.ts           # console name
│       ├── advanced.ts        # Calm/Advanced mode
│       ├── setup.ts           # anyCredentialed() helper
│       ├── alerts.ts          # attention alert count
│       ├── globalActivity.ts  # global activity provider
│       ├── help.ts            # page-aware help content
│       └── …                  # 80+ more
│
├── package.json               # React 19 + Vite 8 + TypeScript 7
├── vite.config.ts             # tailwind plugin + path alias
├── vitest.config.ts           # jsdom env, @ alias
└── public/                    # self-hosted fonts (Inter/JetBrains Mono/Space Grotesk var)
```

**Two-level nav (M974).** The 67 views are organized into 8 sections
(`talk / observe / automate / govern / fleet / knowledge / connect / admin`).
View IDs are stable across renames — the URL hash, the help topic, and the
⌘K target all key on the view id.

## SDK layout

```
sdk/
├── *.go (root)                # Go SDK: client, mailbox, approvals, runs, sdk base
│                              # uses controlplane Client (loopback TCP), no HTTP
├── python/
│   └── agezt/                 # Python SDK: client.py, agent.py, aio.py, errors.py
│                              # stdlib-only (urllib); SSE; same-origin redirect defense
├── rust/                      # Rust SDK: client.rs + http.rs (stdlib TcpStream)
└── typescript/                # TypeScript SDK: client.ts + agent.ts (global fetch)
```

The Go SDK lives at `sdk/` root (not under `sdk/go/`) because the very first
plugin and the very first `agt`-like helper are written in Go; the polyglot
SDKs are siblings.

## Module dependency direction (unchanged from the original)

- `kernel/` points inward toward `event` + `bus` + `journal` + `state`.
- `plugins/` depend only on `sdk/` + `contract/gen`, never on kernel
  internals. (In-process built-ins are the documented exception — see
  DECISIONS B0a.)
- `frontend/` and external SDKs talk only to the HTTP surface
  (`kernel/webui` + `kernel/restapi` + `kernel/openaiapi`) over the
  contract. They never import kernel packages.
- `cmd/agt/` is a thin client over `kernel/controlplane` Client (TCP
  loopback). It does not import `kernel/runtime` directly.
- `sdk/` is a client of the HTTP surface; the Go SDK additionally
  shortcuts via the control-plane socket.

## Config & runtime dirs (created at runtime, not in repo)

```
~/.agezt/
├── config.yaml                # config (precedence: defaults < file < env < flags)
├── journal/                   # append-only JSONL, 64 MiB segments, BLAKE3 chain
│   └── 00000001.jsonl, …
├── snapshots/                 # journal snapshots every 10k events or 1h
├── state/                     # namespaced KV (one file per namespace)
├── catalog/                   # api.json / local.json / custom.json / meta.json
├── vault/                     # AES-256-GCM encrypted creds (M172+)
├── memory/, worldmodel/,      # content-addressed BLAKE3 stores
│   skill/, taste/, standing/,
│   okr/, workboard/, board/,
│   market/, datalake/, mcp/,
│   channel/, seat/, schedule/,
│   toolforge/, workflow/,
│   configcenter/, settings/,
│   resume/, cadence/, agents,
│   artifacts/, datalake/,
│   tenant/                    # per-tenant sub-tree (when multi-tenant is on)
├── secrets.enc                # standalone secret bundle (M172+)
├── runtime/
│   ├── control.addr           # "127.0.0.1:NNNNN\n"
│   ├── control.token          # hex token, mode 0600
│   └── plugins/               # plugin sockets
└── workspace/                 # default per-agent workdir
```

## Build-tag policy (POLICY §2.2)

Native plugins (anthropic, ollama, shell, file, http) MAY compile INTO
`cmd/agezt` for the single-binary convenience build, or build as
standalone binaries — controlled by build tags. Third-party plugins
always run as separate processes over stdio + JSON-RPC.

## How to regenerate this file

```sh
make structure-md
# or:
go run ./tools/structure-md
```

The generator walks `kernel/`, `internal/`, `cmd/`, `plugins/`, `sdk/`,
`tools/`, `frontend/src/` and reads each package's first doc comment
(`doc.go` for Go, top-of-file `/** … */` or `// …` block for TS/TSX).
Output is written to `STRUCTURE.md` (this file) and a sidecar
`STRUCTURE.<dir>.md` per major root, all grouped into
`.project/STRUCTURE.generated/`. CI fails on uncommitted changes via
`make check`.
