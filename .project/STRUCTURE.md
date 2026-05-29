# Agezt — Repository Structure (STRUCTURE.md)

> The exact repository layout to produce. Reflects the foundational decisions (DECISIONS B0–B0d): JSON-RPC transport, in-process-default plugins, minimal-then-growing contract, mutable state store, DAG-as-second-layer. Build incrementally — not every directory exists at M0.5; this is the destination shape. Items marked **[M0.5]** are the minimal core; **[MVP]** land in Milestone 1; others grow later.

```
agezt/
├── LICENSE                      # MIT [M0]
├── README.md                    # [M0] (from the design README)
├── DEPENDENCIES.md              # [M0] every dep justified (POLICY §1)
├── Makefile                     # [M0] build/test/lint/gen targets
├── go.mod / go.sum              # [M0] module github.com/<org-TBD>/agezt
├── .github/workflows/           # [M0] CI: build(multi-arch)/test/lint/dep-check; release→GHCR+cosign+SBOM
├── docs/                        # [M0] the full spec suite moved here (SPEC-01..16, DECISIONS, POLICY, ...)
├── contract/
│   ├── agezt-contract.jsonc    # [M0] source of truth (JSON-RPC + JSON Schema)
│   └── gen/                     # [M0] build-time generated SDK types (Go first; ts/py/rust later)
│
├── cmd/
│   ├── agezt/                  # [M0.5] kernel/daemon binary (+ native plugins via build tags)
│   │   └── main.go
│   └── agt/                     # [M0.5] CLI binary (thin client over the control-plane socket)
│       └── main.go
│
├── internal/
│   └── brand/                   # [M0] name/paths/strings in ONE place (DECISIONS A1)
│       └── brand.go
│
├── kernel/
│   ├── event/                   # [M0.5] canonical Event type + Kind constants (grows append-only)
│   ├── journal/                 # [M0.5] append-only JSONL + BLAKE3 chain + ULID + recover + verify + snapshots
│   ├── state/                   # [M0.5] first-class mutable state store (embedded; CobaltDB driver later)
│   ├── bus/                     # [M0.5] in-process event bus (subject routing; durable-before-publish)
│   ├── agent/                   # [M0.5] canonical Provider/Tool interfaces + single-agent tool-loop (first-party)
│   ├── controlplane/            # [M0.5] unix-socket server: halt/resume/why/attach + journal verify
│   ├── conf/                    # [M0.5] config precedence (defaults<file<env<flags) + redaction
│   ├── conduit/                 # [MVP] provider/tool registry + Governor (routing, budget, fallback)
│   ├── catalog/                 # [MVP] provider/model catalog sync (models.dev-class) + local autodiscover
│   ├── pluginhost/              # [MVP] out-of-process plugin host (stdio/JSON-RPC, handshake, health, isolation)
│   ├── scheduler/               # [MVP] DAG compile + execute (SECOND layer over agent loop)
│   ├── planner/                 # [MVP] intent → DAG meta-agent
│   ├── edict/                   # [MVP] policy engine + trust ladder
│   ├── warden/                  # [MVP] isolation profiles (namespace; container; microvm optional later)
│   ├── chronos/                 # [MVP] cron/interval/event/condition/webhook scheduler
│   ├── pulse/                   # [MVP] heartbeat + observers + salience + initiative + briefing
│   ├── lifecycle/               # [MVP] agent supervisor (goroutine+mailbox; restart; replay-resume)
│   ├── memory/                  # [M2] tiers + world-model graph + retrieval pipeline + context mgmt
│   ├── forge/                   # [M2] skill lifecycle state machine + shadow-test
│   ├── reflect/                 # [M8] reflection loop
│   ├── migrate/                 # [MVP] migration runner (core + plugin schemas)
│   └── identity/                # [M2] export/import bundles + backup + point-in-time restore
│
├── plugins/
│   ├── providers/               # provider plugins (translate canonical↔dialect, SPEC-15)
│   │   ├── anthropic/           # [M0.5]
│   │   ├── ollama/              # [MVP] local fallback
│   │   ├── openai/              # [M1+]
│   │   ├── openai-compat/       # [M1+] generic for most 3rd-party/local
│   │   └── gemini/              # [M1+]
│   ├── tools/                   # tool plugins (in-process default; SPEC-04)
│   │   ├── shell/               # [M0.5]
│   │   ├── file/ http/          # [MVP]
│   │   ├── browser/             # [MVP] (container isolation)
│   │   └── image/ audio/ video/ docgen/ data/ search/   # [M2+]
│   ├── channels/                # channel plugins (normalize to UnifiedMessage)
│   │   ├── telegram/            # [MVP] duplex
│   │   └── discord/ slack/ whatsapp/ signal/ email/ sms/ matrix/ teams/ homeassistant/ webhook/  # [M3]
│   ├── coding/                  # coding-agent bridges (SPEC-04 §4) + ACP (SPEC-15 §3)
│   │   ├── claudecode/ codex/ aider/   # [M5]
│   │   └── acp-bridge/          # [M6]
│   ├── memory/                  # memory backends
│   │   ├── flintvector/ redis/  # [M2]
│   ├── storage/                 # storage drivers
│   │   ├── embedded/            # [M0.5] CobaltDB+JSONL+index (the default; may live in kernel for M0.5)
│   │   └── postgres/            # [M2]
│   ├── tunnels/                 # [M6] cloudflare/ tailscale/ wirerift/
│   └── mcp-bridge/              # [M6] any MCP server → Tool capabilities
│
├── sdk/
│   ├── go/                      # [MVP] base/serve helpers (~20-line plugin)
│   ├── ts/ py/ rust/            # [M6]
│   └── create-agezt-plugin/    # [M6] scaffolder
│
├── gateway/                     # [M4] remote transport (WS/SSE), auth, static UI host, OpenAI-compat + native API, ACP server
│
├── web/                         # [M4] React 19 + Vite + Tailwind + shadcn + React Flow
│   ├── src/
│   │   ├── flow-studio/         # DAG design/run/replay
│   │   ├── inbox/               # unified channels
│   │   ├── monitor/             # agents/pulse/cost/traces/health + HALT
│   │   ├── memory-explorer/     # knowledge/world-graph/skills+revert
│   │   ├── conversation/        # chat history + tool-call debug + context inspector (SPEC-07/10)
│   │   ├── widgets/             # widget system (sandboxed iframe+CSP, SPEC-12)
│   │   └── settings/            # trust ladder, edict tester, providers, channels, plugins, salience dial
│   └── ...
│
├── deploy/                      # [MVP+] Dockerfile (scratch/distroless), docker-compose.yml, k8s/, sandbox images
└── test/                        # [all] integration/golden-trace, injection corpus, conformance harness, chaos
```

## Module dependency direction
Everything in `kernel/` points inward toward `event` + `journal` + `state` + `bus`. Nothing mutates shared state except by appending events (journal) and updating the state store. Plugins depend only on `sdk/` + `contract/gen`, never on kernel internals. `web/` and external SDKs talk only to the gateway/control-plane over the contract — never import kernel packages.

## Config & runtime dirs (created at runtime, not in repo)
`~/.agezt/{config.yaml, journal/, snapshots/, state/, plugins/, secrets.enc, workspace/, runtime/sockets/}` — see `SPEC-16` §3 for the full `config.yaml` reference.

## Build-tag policy (POLICY §2.2)
Native plugins (anthropic/ollama/shell/file/http) may compile INTO `cmd/agezt` for the single-binary convenience build, or build as standalone binaries — controlled by build tags. Third-party plugins always run as separate processes over stdio/JSON-RPC.
