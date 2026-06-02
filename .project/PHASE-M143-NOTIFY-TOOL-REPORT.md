# M143 — `notify` tool (proactive agent messaging)

## Why
Until now the agent could only speak to the operator at the END of a run (the
final reply) or via Pulse's autonomous briefs. A long task — "audit the repo,
fix what you find, open a PR" — ran silently for minutes with no way to say "I've
started", "I hit X, proceeding", or "done, here's the link". M142 gave the
*operator* manual egress (`agt send`); M143 gives the *agent* the dual: the
ability to ping the operator mid-run. This is the Jarvis "keep me posted"
behaviour, and it composes directly with the channels (M138–M139) and the
channel-send plumbing (M142).

## What
A new in-process agent tool, `plugins/tools/notify`, implementing `agent.Tool`:

- **Definition** — `notify(text, channel?)`. `text` is the message; optional
  `channel` restricts delivery to one kind (telegram|slack|discord), else it goes
  to all configured channels. The description names the configured kinds and tells
  the model the recipients are fixed.
- **Invoke** — delivers `text` to the operator's configured recipients via the
  injected sender, returns a summary ("notified the operator (N recipient(s) across
  …)"); an unconfigured `channel`, empty `text`, or a total delivery failure is an
  IsError result the model sees (and can adjust to), not a hard error.

### Security — the recipient is config-pinned (SPEC-04 §1.7)
Outbound from a (potentially prompt-injected) agent is an exfiltration/spam risk
if the agent can choose the destination. So the tool **never** takes a recipient
id: destinations are pinned to each channel's operator-configured allowlist, and
the agent supplies only the text. The tool can therefore only ever message the
operator's OWN chats. It is gated by a new Edict capability `CapNotify`, **allowed
by default** (the agent talking to its owner is low-risk) but raisable/denyable by
the operator like any capability. Every send is journaled as `channel.outbound`,
so the agent's proactive messages are as auditable as everything else.

## Wiring
- **Edict** (`kernel/edict/edict.go`, `toolmap.go`) — `CapNotify = "notify"` added
  to the capability set, `DefaultLevels` (LevelAllow), `AllCapabilities`, and the
  tool→capability map. (Edict default-denies unknown capabilities, so registration
  is mandatory, not cosmetic.)
- **Tool** (`plugins/tools/notify/notify.go`) — `New(send, targets)` returns nil
  when no channel has a non-empty allowlist (tool disabled) so it's only advertised
  when it can actually do something.
- **Daemon** (`cmd/agezt/main.go`) — after the channels are built, the
  channel-send closure (shared with M142's `SetChannelSender`) and the per-kind
  allowlist ids are assembled, and the tool is registered into the live tool map
  (`k.Tools()["notify"]`) at boot, before any run. An `AGEZT_DEMO_NOTIFY=1` escape
  hatch scripts the offline mock to call it (added to `configEnvVars` for the M127
  drift guard).

## Files
- `kernel/edict/edict.go`, `kernel/edict/toolmap.go` — `CapNotify` + mapping.
- `plugins/tools/notify/notify.go` (new) — the tool.
- `cmd/agezt/main.go` — registration + `AGEZT_DEMO_NOTIFY` demo hook.
- `kernel/controlplane/config.go` — `AGEZT_DEMO_NOTIFY` in `configEnvVars`.
- `plugins/tools/notify/notify_test.go` (new), `kernel/edict/toolmap_test.go`.

## Tests (+6, all passing)
- notify: disabled (nil) when no targets / nil sender; sends to all configured
  targets; channel filter limits to one kind and errors on an unconfigured kind;
  empty text rejected; a total delivery failure is an error result; the Definition
  lists the configured kinds.
- edict: `CapabilityForToolCall("notify")` → `CapNotify` (and `remote_run` added to
  the same table).

## Live proof (offline mock, real booted daemon + fake Discord API)
Booted with Discord configured (allowlist `D9`), `AGEZT_DEMO_NOTIFY=1`, and a fake
Discord API:

```
banner:  notify tool      : enabled (1 channel(s) the agent can ping)

$ agt tool list
  notify   Proactively send a short message to the operator over a configured chat
           channel (discord) … you cannot choose arbitrary recipients. …

$ agt run "do the long task"
  --- final answer ---
  [offline-mock] I pinged you over the configured channel, then finished the task.

  fake Discord API: POST /channels/D9/messages -> {"content":"Starting the long task — I'll report back when it's done."}

$ agt inbox --channel discord
  ── discord/D9
     → Starting the long task — I'll report back when it's done.
```

Full proactive loop confirmed: agent → `notify` tool → configured allowlist → real
HTTP POST → journaled `channel.outbound` → visible in `agt inbox`.

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- M127 env drift guard passes with `AGEZT_DEMO_NOTIFY`; Edict capability drift guards pass.
- `go test ./...` — **FAIL 0**, **1460 tests** (was 1454; +6), 61 packages.

## Result
The channels are now a true two-way Jarvis surface: the operator drives the agent
in, the agent replies and — new — proactively pings the operator mid-task, the
operator pushes ad-hoc messages out, and Pulse briefs autonomously. The agent's
proactive voice is safe by construction (recipient pinned to the owner's own chats)
and fully auditable.
