# Agezt

> An open-source (MIT) **agentic operating system**: a stdlib-first Go core
> that turns intent into auditable, reversible action; runs autonomous
> agents under a policy/trust system; proactively informs you (Pulse); and
> extends via in-process or out-of-process plugins.
> **Autonomous, under your authority.**

**Status:** v1 substrate — shipped (post-M1.zz, May 2026).
**Tests:** 688 passing across 39 packages.
**Dependencies:** one (`lukechampine.com/blake3`) + one transitive.

## What you get

A single Go daemon (`agezt`) and a CLI (`agt`) that, together, let you:

```
agt run "summarise the latest commits and email the team"      — one-shot intent
agt plan generate "audit my repo for secrets, propose fixes"   — LLM-generated DAG
agt plan refine plan.json --feedback "skip the email step"     — operator-driven re-plan
agt plan validate plan.json                                    — pure client-side check
agt pulse --correlation run-01H...                             — live tail of one chain
agt pulse --since 0 --replay-rate 50                           — historical replay
agt status                                                     — daemon health overview
agt budget                                                     — spend vs daily / per-task caps
agt tool list                                                  — in-process tools the model sees
agt plugin list                                                — external plugins loaded
agt provider check --all                                       — verify all credentials
agt provider creds set OPENAI_API_KEY sk-...                   — managed vault
agt vault encrypt / vault rotate                               — at-rest encryption + key rotation
agt why <event_id> --payload                                   — walk the audit chain w/ payloads
agt approvals --json                                           — HITL queue (machine-readable)
```

with **9 provider families** (Anthropic, OpenAI + ~11 compatibles, Google
direct + Vertex, Cohere, Mistral, Ollama, AWS Bedrock with bearer +
SigV4 + STS-AssumeRole + SSO, Azure OpenAI), **all streaming**, **4 in-process
tools** (`shell`, `file`, `http`, `browser.read` with cookies),
**out-of-process plugins** in any language over a tiny JSON protocol
(with **hot-reload**, **BLAKE3 pin gating**, **tool allowlists**,
**streaming progress**, and **kernel-callbacks**), an **MCP bridge plugin**
(stdio + SSE transports), a **DAG scheduler** with HITL gates and
**operator-driven re-planning**, a **BLAKE3 hash-chain journal** with
`agt why` audit, **subscription-first provider routing** with
**per-task-type budget caps**, **hot reload** of catalog + vault without
restart, a **vault encrypted at rest** with AES-256-GCM and **passphrase
rotation**, and a **Linux warden** with `prlimit64`-enforced CPU/mem/FD
limits and process-group SIGKILL.

## Quick start

```bash
# Build:
go build ./...
# (or with make: `make build`)

# Tell the CLI where to send commands and what model to use:
mkdir -p ~/.agezt
agt provider creds set ANTHROPIC_API_KEY sk-ant-…
agt catalog sync                       # pull the models.dev catalog
agt provider check                     # verify credentials work

# Start the daemon (terminal 1):
./bin/agezt
# tools registered: shell(...), file(...), http(...), browser.read(...)
# provider primary=anthropic ...

# Run an intent (terminal 2):
agt run "list the files here and tell me what this project is"
```

For the full operator cheat sheet: `agt help`.

## Where the design lives

The full spec suite is under [`.project/`](.project/) and remains the
binding source of authority:

- [`.project/BUILD-GUIDE.md`](.project/BUILD-GUIDE.md) — start here
- [`.project/DECISIONS.md`](.project/DECISIONS.md) — supreme authority
- [`.project/STRUCTURE.md`](.project/STRUCTURE.md) — repo layout
- [`.project/SPEC-*.md`](.project/) — 16 component specs
- [`.project/PHASE-*-REPORT.md`](.project/) — every shipped phase, its
  scope and trade-offs (47+ reports from M1.a through M1.zz and beyond)

## What's built

The v1 substrate. Highlights:

**Kernel** (`kernel/`)
- `agent` — single-agent tool-loop with streaming
- `bus` — pattern-subscribed event bus (NATS-style wildcards)
- `journal` — append-only JSONL + BLAKE3 hash chain
- `state` — file-backed mutable store
- `event` — typed events with `IsEphemeral()` discriminator for
  streaming tokens
- `governor` — per-task routing + fallback chain + USD-microcents
  budget cap with **per-task-type daily ceilings**, subscription-first
- `scheduler` — DAG executor with LoopNode + GateNode
- `planner` — LLM → validated `scheduler.Plan` JSON, with
  **operator-driven refinement** (`agt plan refine`)
- `controlplane` — line-delimited JSON over TCP between
  `agt` ↔ `agezt`; includes operator visibility commands
  (`status`, `tool list`, `plugin list`, `budget`, `why --json/--payload`)
- `creds` — credential vault, AES-256-GCM at rest, passphrase rotation,
  pure-stdlib AWS chain (vault → env → SSO → STS-AssumeRole → default)
- `catalog` — models.dev integration; hot reload
- `approval` — HITL queue (with `--json` for automation)
- `edict` — declarative policy engine
- `warden` — process isolation: Linux `prlimit64` + process-group SIGKILL;
  no-op stubs on macOS/Windows
- `runtime` — wires it all into `Kernel.Open(Config) → *Kernel`
- `plugin` — out-of-process plugin host (stdio JSON protocol) with
  **hot-reload**, **BLAKE3 pin gating**, **tool allowlists**,
  **streaming progress**, and **plugin→host callbacks**

**Providers** (`plugins/providers/`)
- `anthropic`, `openai`, `google`, `vertex` (Gemini + Anthropic on Vertex),
  `bedrock` (bearer + SigV4 + AI21 Jamba + Cohere + Llama + Mistral),
  `cohere`, `ollama`, `compat` (OpenAI-compatible vendors: Groq, DeepSeek,
  xAI, OpenRouter, Together, ...), Azure OpenAI, Mistral. Every family
  has working streaming.

**Tools** (`plugins/tools/`)
- `shell` — warden-isolated subprocess
- `file` — scoped to `AGEZT_WORKSPACE`
- `http` — GET/POST with host allowlist
- `browser.read` — fetch + HTML→text extraction, opt-in cookie jar

**External plugins** (`plugins/external/`)
- `mcpbridge` — Model Context Protocol bridge, both stdio and HTTP+SSE
  transports

**Binaries**
- `cmd/agezt` — the daemon
- `cmd/agt` — the operator CLI

## Verify

```bash
make test     # 688 tests, all green
make build    # produces bin/agezt + bin/agt
make gen      # regenerate SDK types from the contract
```

Or without `make`:

```bash
go test ./...
go build ./...
go run ./tools/jsonschemagen -in .project/agezt-contract.jsonc -out contract/gen/types.gen.go -pkg gen
```

## What's deferred (post-v1)

Genuine remaining deferrals — every one is blocked on a non-stdlib
dependency, a CGO requirement, or a substantial design phase:

- **Plugin sandboxing** — out-of-process plugins are isolated only at the
  process boundary today; per-plugin warden profiles (cgroup v2 + seccomp
  BPF + user-namespace) need either non-stdlib bindings or per-OS CGO.
- **Browser tool — JS rendering, screenshots, search** — would require
  `chromedp` (a CGO/non-stdlib dependency).
- **Vault — OS-keychain auto-integration, argon2 KDF** — both need per-OS
  CGO or non-stdlib bindings; PBKDF2-SHA-256 is the stdlib fallback today.
- **Planner v2 — sub-planners, planner-side tool calls** — operator-driven
  refinement shipped (`agt plan refine`); recursive sub-planning is a
  separate design phase.
- **Pulse v2 — TUI** — non-stdlib (Bubble Tea / tview). Programmatic
  observability is complete (`agt pulse`, `agt status`, `agt tool list`,
  `agt plugin list`, `agt budget`, `agt why --json/--payload`).
- **Windows job objects / macOS sandbox-exec** — both need per-OS CGO
  bindings; in M1.d Linux got `prlimit64` (raw syscall, stdlib).

## License

MIT. See [`LICENSE`](LICENSE). Dependency policy: every external
dep requires a written justification in
[`DEPENDENCIES.md`](DEPENDENCIES.md). Current count: 1 + 1 transitive.
