# Phase Report — Milestone 1.a (MVP Foundation: Tools + Provider + Policy)

> Status: **shipped** · Date: 2026-05-29
> Per [ROADMAP §2 (MVP)](ROADMAP.md) and [TASKS Phase 1](TASKS.md).
> Continues [PHASE-M0.5-REPORT.md](PHASE-M0.5-REPORT.md).

## Scope

ROADMAP §2.1 lists seven MVP essentials. M1.a delivers four foundational
pieces; the remaining three (DAG scheduler, Pulse v1, Telegram channel)
land in subsequent M1 sub-phases.

| MVP essential | Status |
|---|---|
| #1 Kernel core (journal/bus/supervisor/control plane) | ✅ M0.5 |
| #2 DAG scheduler + Planner | ⏸ next |
| #3 Governor + 2 providers (Anthropic + Ollama) | ✅ **Ollama added** (Governor lands with #2) |
| #4 4 tools: shell, file, http, browser | ✅ **file + http added** (browser → M1.b with Warden container) |
| #5 1 channel: Telegram | ⏸ next |
| #6 Pulse v1 | ⏸ next |
| #7 Safety: Edict v1, Warden, redaction, halt | ✅ **Edict v1 added** (Warden → M1.b) |

## What shipped

### New packages

| Package | LoC | Tests | Notes |
|---|---:|---:|---|
| `plugins/tools/file` | 632 | 17 | Read/write/list/search/stat/delete; workspace-scoped path containment (no `..`, no symlink escape, no absolute-outside-root); MaxReadBytes=256K, MaxListEntries=1000 |
| `plugins/tools/http` | 470 | 12 | GET/POST; default-deny host allowlist with `*.example.com` wildcard; http/https only; MaxResponseBytes=256K; MaxRequestBodyBytes=256K |
| `plugins/providers/ollama` | 442 | 6 | Non-streaming /api/chat; canonical↔Ollama dialect (synthesises per-call IDs since Ollama omits them); default model `llama3.2` |
| `kernel/edict` | 538 | 14 | Trust ladder L0–L4; hard-deny rules (fork-bomb, rm-rf-/, mkfs, dd, shutdown, reboot, format-volume); AskAllow vs AskDeny; per-call `Decide` returns `Outcome{Decision,Capability,Level,Reason,HardDenied,WouldAsk}` |

### Wiring changes

- `kernel/agent.LoopConfig` adds a `Policy` hook. **Every ToolCall is now
  gated through the hook**, and a `policy.decision` event is published per
  call (even when no policy is configured — Allow + "no policy
  configured" so the journal is honest about the gating posture).
- `kernel/agent.PolicyVerdict` carries `Allow`, `Capability`, `Reason`,
  `WouldAsk`, `HardDenied`. Deny verdicts skip `tool.invoked` entirely and
  synthesise a tool-result with the reason so the model can react.
- New event kind `policy.decision` (`event.KindPolicyDecision`) added to
  the base set; subject pattern `agent.<actor>.policy`.
- `kernel/runtime.Config` adds `Edict *edict.Engine` (auto-defaults to
  `edict.New(edict.Options{})` when nil); `Kernel.Edict()` accessor
  exposes it for future `agt trust` commands.
- `kernel/edict.CapabilityForToolCall(name, input)` is the single source
  of truth for "which capability does THIS tool call exercise?" The
  runtime uses it to feed Edict.
- `cmd/agezt`:
  - Provider selection extended: `AGEZT_PROVIDER=mock|ollama|anthropic`
    (Anthropic remains the default when `ANTHROPIC_API_KEY` is set).
  - Tool registration via new `buildTools` helper: shell + file (rooted
    at `AGEZT_WORKSPACE` or `<BaseDir>/workspace`) + http (default-deny;
    allowlist via `AGEZT_HTTP_ALLOWED_HOSTS`, or `AGEZT_HTTP_ALLOW_ALL=1`
    with a stderr warning).
  - Startup banner now reports tools and policy engine.

### Default trust ladder (DECISIONS F3, adapted to M1 caps)

| Capability | Default level | Effect under AskAllow |
|---|---|---|
| `shell` | L2 (ask on first use) | allow + WouldAsk=true in journal |
| `file.read`, `file.list` | L4 (allow) | silent allow |
| `file.write`, `file.append` | L2 | allow + WouldAsk |
| `file.delete` | L1 (always ask) | allow + WouldAsk |
| `http.get` | L2 | allow + WouldAsk |
| `http.post` | L1 | allow + WouldAsk |
| `provider.call` | L4 | silent allow (budgeted, not Edict-gated) |
| (anything else) | L0 default-deny | denied with reason |

### Hard-deny rules (DECISIONS F4)

Always block (regardless of trust level), case-insensitive substring match
against tool input, scoped to capability:

- `:(){:|:&};:` (fork bomb, shell only)
- `rm -rf /` (shell only)
- `rm -rf --no-preserve-root` (shell only)
- `mkfs` (shell only)
- `dd if=` (shell only — guards `dd of=/dev/sdX`)
- `shutdown -` (shell only)
- `reboot` (shell only)
- `format-volume` (PowerShell, shell only)

Unit-tested (`TestDecide_HardDenyAlwaysWins`,
`TestHardDeny_CaseInsensitive`, `TestDecide_HardDenyScopedToCapability`).

## Demo transcript

```
$ AGEZT_HOME=/tmp/agezt-m1a-demo AGEZT_PROVIDER=mock ./bin/agezt &
Agezt 0.0.0-m0 — daemon ready (protocol v1)
  base dir         : /tmp/agezt-m1a-demo
  provider         : mock (offline; scripted 2-turn shell+final)
  tools            : shell, file(root=…/workspace), http(hosts=0)
  policy engine    : edict (defaults from DECISIONS F3; AskAllow)
  control plane    : 127.0.0.1:59924

$ ./bin/agt run "list the files here and tell me what this project is"
  [evt seq=0 kind=task.received]
  [evt seq=1 kind=llm.request]
  [evt seq=2 kind=llm.response]
  [evt seq=3 kind=policy.decision]   ← NEW in M1.a
  [evt seq=4 kind=tool.invoked]
  [evt seq=5 kind=tool.result]
  [evt seq=6 kind=llm.request]
  [evt seq=7 kind=llm.response]
  [evt seq=8 kind=task.completed]

--- final answer ---
[offline-mock] I ran a directory listing via the shell tool. This project
is Agezt — an open-source, MIT-licensed agentic operating system written
in Go. The M0.5 foundation under kernel/ (event, journal, state, bus,
agent, runtime, controlplane) plus the in-process plugins under plugins/
are what just executed this run; every step you saw was journaled and
BLAKE3-chained.

$ ./bin/agt journal verify
{ "ok": true }
```

Event count grew from 8 (M0.5) to 9 (M1.a) — the new `policy.decision`
fires before each tool invocation, recording the trust-ladder verdict.

## Verified invariants

Beyond the existing M0.5 invariants:

- **No tool runs without a policy decision** —
  `kernel/agent.TestRun_ToolCallRoundtrip` now asserts the exact kind
  sequence `[..., policy.decision, tool.invoked, tool.result, ...]`.
- **Deny verdicts skip tool.invoked entirely** —
  `kernel/agent.TestRun_PolicyDeny_SkipsToolInvoke` verifies that on a
  deny verdict, `tool.invoked` is absent from the journal and the synthetic
  `tool.result` carries the deny reason for the model.
- **Hard-deny survives even at L4** — `kernel/edict.TestDecide_HardDenyAlwaysWins`.
- **Unknown capabilities default-deny** — `TestDecide_UnknownCapability_DefaultDeny`.
- **File tool resists `..` and symlink escapes** —
  `TestContainment_RejectsDotDot`, `TestContainment_RejectsAbsoluteOutsideRoot`.
- **HTTP tool default-deny holds** — `TestHostDenied_DefaultDeny`,
  `TestHostDenied_AllowList`, `TestRejectsNonHTTPSchemes`.
- **Ollama dialect roundtrip** — tools, system prompt, tool-call IDs
  preserved through canonical↔Ollama translation.

## Cumulative status

```
21 packages | ~9,200 lines source | ~100 tests passing | 2 deps (allowlisted)
```

| Subsystem | LoC | Tests |
|---|---:|---:|
| `kernel/{ulid,event,journal,state,bus,agent,runtime,controlplane}` | 4,117 | 63 |
| `kernel/edict` | 538 | 14 |
| `plugins/providers/{mock,anthropic,ollama}` | 1,034 | 13 |
| `plugins/tools/{shell,file,http}` | 1,346 | 35 |
| `cmd/{agezt,agt}` | 718 | 8 |
| `internal/{brand,paths}` | 102 | 1 |
| `tools/{jsonschemagen,depscheck}` | 633 | (jsonschemagen: 3 + e2e) |
| **Module total** | **~8,500 source + tests** | **~140 tests** |

## Deviations from spec (intentional)

1. **Live approval routing is not wired.** Edict's `Ask` levels fold to
   `Allow + WouldAsk=true` (or to `Deny` under `AskPolicy=AskDeny`). The
   journal captures every would-have-asked moment so audits remain
   honest; HITL routing lands with Pulse (M1.b — needs a notification
   surface).
2. **Anthropic provider is still non-streaming.** Same as M0.5; streaming
   added with the Governor.
3. **HTTP method set is GET/POST only.** PUT/DELETE/PATCH deferred until
   the trust ladder grows scopes for write-class HTTP.
4. **File tool `patch` op (unified diff) deferred.** Write replaces.
5. **No Warden process containment yet** — shell/file/http run in the
   kernel process. Edict's hard-deny is the only kill-switch for
   shell-level damage today. Warden namespace+container profiles arrive
   in M1.b.

## Open items for M1.b

- **Warden v1** (TASKS P1-WARD-01) — namespace/cgroups/seccomp profile;
  container profile for the browser tool.
- **Governor v1** (TASKS P1-CONDUIT-01..04) — provider registry,
  subscription→cost→latency routing, USD-microcent budget, fallback chain.
- **DAG scheduler + Planner** (TASKS P1-SCHED-01..03, P1-PLAN-01) —
  loop becomes a node type; tool/llm/loop/gate nodes; intent→DAG.
- **Browser tool** (TASKS P1-TOOL-04) — Playwright/CDP in a container.
- **Telegram channel** (TASKS P4-CHAN-01) + plugin-host out-of-process flow.
- **Pulse v1** (TASKS P3-*) — observers, salience, initiative, briefing.
- **Live approval routing** — uses the channel + Pulse paths above to
  promote `WouldAsk` to a real prompt.

## Pointers

- Tests: `go test ./...` (140+ pass, vet clean, depscheck OK)
- Demo (offline mock): `AGEZT_PROVIDER=mock ./bin/agezt`, then `agt run "..."`
- Demo (real Ollama): `AGEZT_PROVIDER=ollama AGEZT_MODEL=llama3.2 ./bin/agezt`
- Demo (real Anthropic): `ANTHROPIC_API_KEY=… ./bin/agezt`
- Strict-mode demo: append `AGEZT_HTTP_ALLOWED_HOSTS=api.github.com,…` to scope http
- Force-deny everything but L4: would need an in-code flag; surface via
  CLI in M1.b along with `agt trust set <cap> <level>`
- Next milestone report: `PHASE-M1.b-REPORT.md`
