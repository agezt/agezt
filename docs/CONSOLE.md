# The Agezt Console (Web UI)

The console is the browser cockpit for a running `agezt` daemon: see what the agent is
doing, talk to it, and steer it — all backed by the same journaled, policy-gated,
reversible control plane as the `agt` CLI. This guide is a map of what you can *do* from
it, with emphasis on the operator controls.

> Everything here is also doable from `agt` on the CLI. The console just makes it visible
> and clickable.

## Opening it

The console is off by default. Start the daemon with a loopback address:

```bash
AGEZT_WEB_ADDR=127.0.0.1:8787 ./bin/agezt
```

The startup banner prints a tokenized URL — `http://127.0.0.1:8787/?token=<hex>`. Open
it. The token authenticates the session; treat the URL like a password and keep the
address on loopback unless you deliberately expose it.

- **⌘K / Ctrl-K** opens the command palette: jump to any view, start a new chat, open a
  run, or trigger appearance/config export-import.
- **Theme, accent colour and console name** are per-device (stored in your browser) and
  apply instantly. **Persona, prompt library and routing** live on the daemon.

## The views, at a glance

Grouped in the sidebar:

- **Converse** — Chat (streaming answers with live tool calls, reasoning and real cost
  inline; per-conversation persona/model; retry, regenerate, edit-&-resend, pin, rename,
  export), Inbox (every channel thread + send a message), Agent Board, **Approvals**
  (grant/deny the agent's pending ask-class actions — human-in-the-loop).
- **Monitor** — Mission Control, Health, Activity, **Autonomy** (the proactive
  heartbeat), Alerts, Live Stream, Insights, Runs, Budget.
- **Agents** — Agents, Sandbox, Flow Studio, Replay, Analyst, **Search** (the journal).
- **Automation** — Schedules, Standing orders.
- **Knowledge** — Memory, World model, Skills, Reflection.
- **System** — Overview (Dashboard), System, Persona, Prompts, Config Center, Config,
  Providers, Models, Routing, Tools, Catalog, **Policy**, Cache, **Backup**.

## Steering the proactive heartbeat — *Autonomy*

The heartbeat (Pulse) is what makes the agent act unprompted: every beat it polls its
observers, scores what changed, and informs or asks you. The **Autonomy** view's heartbeat
card gives you the full control surface:

- **Pause / Resume** — the master switch. Paused, the daemon is reactive-only; in-flight
  work finishes.
- **Beat now** — fire one heartbeat immediately instead of waiting for the cadence. Works
  even while paused (an explicit one-off "think now").
- **Cadence** (`10s … 1h`) — how often the agent checks in, changed live.
- **Dial** (`quiet` / `balanced` / `chatty`) — how much reaches you. *quiet* = only
  alerts/actions; *balanced* = notifications and up; *chatty* = digests too.
- **Flush digest (N)** — appears only when the agent is holding lower-priority briefs for
  the periodic digest; delivers them now.

**Cadence and dial persist across restart** (they're saved to the config store). Pause
state, like other runtime state, resets when the daemon restarts. If the card shows "Pulse
is disabled", set `AGEZT_PULSE` to enable the heartbeat.

## Backup & restore

Two complementary scopes, both under **Backup** (and in ⌘K):

- **Appearance** — theme, accent, console name. Per-device (this browser).
- **Daemon configuration** — persona, prompt library, routing chains. Importing replaces
  them on whichever daemon the console is connected to.
- **Full snapshot** — *everything* customizable in one file: persona, prompts, routing,
  standing orders, schedules, memory and the world model. **Export** for backup/migration;
  **Restore** replays every section. The restore confirm spells out the counts and the
  caveat: config is replaced, standing orders & schedules are *added* (re-restoring onto a
  populated daemon duplicates them), and memory & the world model *dedupe*. Best for
  seeding a fresh daemon or moving to a new machine.

Each autonomy/knowledge domain also has its own **Export / Import** in its view, so you can
move just one:

| View | Export/Import | Re-import behaviour |
|---|---|---|
| Standing | orders | additive (duplicates) |
| Schedules | schedules | additive (duplicates) |
| Memory | facts | idempotent (content-addressed dedupe) |
| World | entities + relations | idempotent (dedupe; relations resolved by name) |

## Security & trust

- **Policy** view → *Capability policy*: set each capability's trust level, the engine-wide
  ask mode, and runtime hard-deny rules. **Test a decision** dry-runs the edict engine —
  pick a capability + an input and see whether it would be **ALLOW / ASK / DENY** (and via
  which hard-deny rule) without changing anything. Use it to understand a block, or to
  check a deny rule before/after adding it.
- **Policy** view → *Secret redaction*: paste text and the live secret-scrubber reports
  whether it would redact it, into which categories (api keys, JWTs, …), and shows the
  masked result. The probe text is sent in the request body, never a URL, and the response
  never echoes the matched secret.
- **Search** view → **verify integrity**: walks the journal's tamper-evident hash chain and
  confirms it's intact (green) or reports a break (red). The journal is the daemon's
  append-only source of truth; this makes that audit guarantee checkable in one click.

## Transparency — "why did it do that?"

In **Search** (the journal browser), expand any event and click **trace cause**. It walks
the event's *causation chain* — from the root cause down to this event — crossing
correlation boundaries the filters can't, so you can see e.g. a heartbeat tick → the
initiative it raised → the run that acted. A sub-agent's parent run is surfaced too.

## Editing knowledge & autonomy

Most stores are fully CRUD-able from their views:

- **Standing** — create / edit / pause / remove orders; per-order history.
- **Schedules** — create / edit (interval, daily, window, once); preview next fire times.
- **Memory** — teach a fact; revise (supersede) one.
- **World** — add entities, relate them, edit aliases/attrs.
- **Skills** — author and revise skills.
- **Prompts** — manage the prompt library (also import/export).

## Safety notes

- The token in the URL authenticates the whole session — keep it private and the address
  on loopback.
- **GET** routes are read-only by construction (enforced by a test); **POST** routes are
  the only ones that mutate.
- "Runtime-only" controls (pause) reset on restart; cadence and dial persist.
- Restoring a full snapshot or importing standing/schedules onto a *non-empty* daemon
  duplicates the additive domains — prefer it for fresh daemons.
