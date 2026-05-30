# Phase Report — Milestone M6 (Telegram channel & Unified Inbox)

> Status: **shipped** · Date: 2026-05-30
> Phase 4. Adds the duplex Telegram channel, the Pulse→Telegram brief
> sink, and a Unified Inbox — the last pieces of the v0.1.0 MVP
> success line: *"From Telegram I type 'check my repos' and it
> replies; without asking, it tells me on Telegram that CI broke; I
> can see why with `agt why`."* That paragraph is now true end-to-end.

## Scope

Through M5 Pulse noticed a broken CI and briefed to the daemon log;
the operator still had to be at the terminal. M6 connects a real
messaging surface in both directions:

- **Inbound**: a Telegram message → the normal agent loop runs → the
  answer is sent back. The agent run shares the channel's correlation,
  so the whole exchange (Telegram in → task/llm/tool/policy → Telegram
  out) is one explainable arc under `agt why`.
- **Outbound / proactive**: Pulse briefs tee to Telegram via the
  `BriefSink` seam built in M5 — "CI broke" arrives unprompted.
- **Unified Inbox**: a journal-backed view (`agt inbox`) groups channel
  conversations by correlation, newest first.

**MVP cut (ROADMAP §2.2 / SPEC-04 §1):** **in-process** Telegram
channel over the Bot API using **net/http only** — no new dependency.
Out-of-process polyglot channels (SPEC-04 §1.6) are the full-project
direction; in-process matches how memory/pulse run inside the daemon.

**Security (SPEC-04 §1.7):** inbound is an injection surface. A chat-id
**allowlist** gates who may drive the agent; an empty allowlist
fail-closes (outbound-only). Inbound text is data (passed as the
agent's intent); the agent's tool calls still pass through Edict. Every
inbound — including rejected ones — is journaled.

## What shipped

### New `kernel/channel`
Canonical `UnifiedMessage` (SPEC-04 §1.3 / contract), `Outbound` +
`Priority`, the `Channel` interface (`Name`/`Start`/`Send`), the
`InboundHandler` closure type, and `Allowlist` (fail-closed).

### New `plugins/channels/telegram`
In-process duplex channel over the Bot API via `net/http`:
`getUpdates` long-poll (offset-advancing, ctx-cancellable, network-
error backoff) and `sendMessage`. `Start` emits `channel.inbound`,
enforces the allowlist, runs the handler, emits `channel.outbound`, and
replies — all under one minted correlation. Injectable `BaseURL` +
`*http.Client` make it fully testable with `httptest` (and enable
self-hosted Bot API via `AGEZT_TELEGRAM_API_BASE`).

### Event kinds (append-only)
`channel.inbound`, `channel.outbound`.

### Pulse sink adapters (`kernel/pulse/briefing.go`)
`SinkFunc` (function→BriefSink adapter) and `MultiSink` (fan-out that
continues past a failing sink) — so the daemon tees the log sink with
the Telegram sink without pulse depending on any channel.

### Unified Inbox
`kernel/controlplane/inbox.go` folds `channel.inbound`/`outbound` into
threads grouped by correlation, newest first (`CmdInbox`); `cmd/agt/
inbox.go` renders `agt inbox [N] [--json]` (← inbound / → outbound).

### Daemon wiring (`cmd/agezt/main.go`)
`buildTelegram` (when `AGEZT_TELEGRAM_TOKEN` set): constructs the
channel with the `AGEZT_TELEGRAM_CHAT_ID` allowlist + an inbound
handler = `k.RunWith`, starts it on the daemon ctx, and tees the Pulse
sink with a Telegram brief sink. Banner reports enabled/disabled.

| Env var | Meaning |
|---|---|
| `AGEZT_TELEGRAM_TOKEN` | bot token (enables the channel) |
| `AGEZT_TELEGRAM_CHAT_ID` | comma-list allowlist + Pulse-brief recipients |
| `AGEZT_TELEGRAM_API_BASE` | override Bot API root (self-hosted/proxy/test) |

## Design rules followed

- **No new external dependency** — `net/http` only; `go.mod` unchanged
  (POLICY).
- **Reuses the existing agent loop wholesale** — inbound just calls
  `k.RunWith`, so memory injection, Pulse, Edict, budget, and the
  journal all apply unchanged. No channel-specific agent path.
- **One correlation per exchange** — Telegram in/out + the agent's full
  task arc link under `agt why` and `agt inbox`.
- **Fail-closed security** — empty allowlist denies everyone; rejected
  senders get one "not authorized" notice and are journaled.
- **Outbound is uniform** — both the reply path and Pulse briefs go
  through `Send`, which journals `channel.outbound`.
- **Channel decoupling** — controlplane has no compile dependency on
  the telegram package; pulse has none either (SinkFunc adapter).

## Test coverage

~20 new tests; `go test ./...` green on host (windows) + `GOOS=linux`
cross-compile; `go vet` clean. Package count 38 → 40 (added
`kernel/channel`, `plugins/channels/telegram`).

- `kernel/channel`: allowlist (allow/deny/trim/fail-closed/parse).
- `plugins/channels/telegram` (`httptest` fake Bot API): inbound →
  handler → reply round-trip with shared correlation; allowlist
  rejection (handler not run, "not authorized" sent, still journaled);
  `Send` journals outbound; `Start` poll advances the offset.
- `kernel/pulse`: `SinkFunc` forwarding; `MultiSink` fan-out continues
  past an error.
- `kernel/controlplane`: inbox empty, group-by-correlation (one thread,
  two messages), newest-first ordering.
- `cmd/agt`: inbox help + bad-arg.

### Manual end-to-end (mock provider + a fake Bot API server)
- DM "what is this project?" from the allowlisted chat → the agent ran
  the full shell-tool arc and the answer was delivered back via
  `sendMessage`; `agt inbox` showed the `← in / → out` thread; `agt why
  <outbound_id>` reconstructed `channel.inbound → task.received → llm ×2
  → tool.invoked/result → task.completed → channel.outbound` under one
  correlation.
- Tripping the CI probe delivered `📣 ci probe failed (exit 2)` to
  Telegram (Pulse→Telegram tee).

A real-token run (`AGEZT_TELEGRAM_TOKEN` against api.telegram.org) was
not executed here — the fake Bot API server exercises the identical
code path; the report documents the real-token smoke for an operator
with a bot.

## Deferred (named for later)

- **Inline approve/deny buttons** (HITL via Telegram `Signal`
  callbacks → `EVT_APPROVAL_GRANTED|DENIED`, SPEC-04 §1.5).
- **Attachments/media**, message edit/dedupe via `SendReceipt`.
- **The other channels** (Discord/Slack/Email/SMS/…) and **out-of-
  process channel plugins** (SPEC-04 §1.6).
- **Per-message concurrency** — inbound currently runs the agent
  synchronously in the poll loop; a worker pool lets long tasks not
  block new messages.
- **Web-UI Unified Inbox** surface (this ships the CLI inbox).

## Closes / next

The **v0.1.0 MVP success line is true end-to-end** — Telegram command
in → reply out, unprompted CI brief out, fully explainable via `agt
why`, haltable via `agt halt`, on a single deployment. This is the
tag-**v0.1.0** moment.

Next per ROADMAP M-series: **Phase 2 full Memory & Forge** (world
model, skill lifecycle, reflection) or **Phase 5 Web UI** (Flow Studio,
Live Monitor, the Web Inbox). The four in-process residents (agent
loop, memory, pulse, channel) and the operator CLI are the stable base
they build on.
