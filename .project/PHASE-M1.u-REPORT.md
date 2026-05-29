# Phase Report — Milestone 1.u (Pulse v1 — live event tail)

> Status: **shipped** · Date: 2026-05-29
> Per DECISIONS A-OBS (operator observability) and the "what is the
> daemon doing right now?" gap left by `why <event_id>` (which
> requires you to know an event already happened).
> Continues [PHASE-M1.n.x-REPORT.md](PHASE-M1.n.x-REPORT.md).

## Scope

`agt why <event_id>` walks the correlation chain of an event that
already happened. `agt journal verify` checks the BLAKE3 chain.
Neither lets an operator see the daemon's bus traffic in real
time. Before M1.u, the only "what's happening now?" answer was
the inline rendering inside `agt run` — useful while a run is in
flight on *your* terminal, useless if you're investigating a
slow run someone else launched, or watching for tool denials
across all runs.

M1.u ships:

```
agt pulse                         # all events
agt pulse --subject agent.>       # filter by NATS-style subject pattern
agt pulse --kind llm.response     # filter by event Kind (repeatable)
agt pulse --json                  # one JSON event per line (pipe to jq)
```

Long-lived control-plane subscription. The daemon streams events
to the client until either side closes the connection; Ctrl+C
exits cleanly.

| Concern | Status |
|---|---|
| New control-plane command: `pulse_subscribe` | ✅ |
| Server handler subscribes to bus pattern, streams events | ✅ tested |
| Server-side `kinds` filter (events excluded never cross the socket) | ✅ tested |
| Default pattern (`>`) when args missing | ✅ tested |
| Client-disconnect detection via Read-watcher goroutine | ✅ tested |
| 10-min handleConn read deadline cleared for the long-lived stream | ✅ |
| Server-context cancellation unblocks the handler | ✅ |
| New client method: `StreamUntilCancel` (open-ended sibling of `Stream`) | ✅ |
| Ctrl+C → ctx cancel → conn close → clean nil return | ✅ tested |
| CLI subcommand `agt pulse` with `--subject`/`--kind`/`--json`/`-h` | ✅ |
| Human format shows ts, seq (or `eph` for ephemeral), kind, subject, actor | ✅ |
| JSON format outputs canonical `event.Event` for jq / log shipping | ✅ |
| Help text updated | ✅ |

## Changes

### 1. `kernel/controlplane/protocol.go` — new command

```go
// Pulse: live operator observability (M1.u). Long-lived
// subscription that streams bus events to the client until either
// side closes the connection. Args: pattern (default ">"),
// optional kinds filter ([]string). Never sends RespResult — the
// client terminates the stream by closing the conn.
CmdPulseSubscribe = "pulse_subscribe"
```

### 2. `kernel/controlplane/pulse.go` — new file (server handler)

The handler does three things:

1. **Parse args.** `pattern` defaults to `>`; `kinds` is an optional
   `[]string` of event.Kind names to allow through. Both come from
   the wire-decoded `map[string]any`.
2. **Subscribe.** `s.k.Bus().Subscribe(pattern, 4096)` — 4× the
   buffer CmdRun uses, since pulse may match every event on the
   bus, not just one run's events.
3. **Pump events** until any of three exit conditions:
   - server context cancelled (daemon shutdown)
   - client disconnected (`clientGone` channel closed by Read watcher)
   - subscription dropped (channel closed) → error to client

Two non-obvious choices, both documented in the file:

**Why the Read-watcher goroutine.** A naive "stop when writeResp
fails" works for chatty streams (the next event triggers a write
that fails on broken pipe) but hangs forever on a quiet stream
where the operator subscribed with a narrow filter that matches
nothing. The watcher does a `conn.Read(buf[:1])` loop; pulse never
expects input, so *any* Read error means the client closed.

**Why `_ = conn.SetReadDeadline(time.Time{})` first thing.**
handleConn pins a 10-minute read deadline for short-lived commands;
without clearing it the Read in the watcher goroutine would
spuriously return a timeout after 10 minutes and trigger a false
disconnect.

### 3. `kernel/controlplane/server.go` — split `writeResp`

Existing handlers want fire-and-forget writes; pulse needs the
error to detect client disconnect. Split:

```go
// Method form — fire-and-forget, unchanged signature.
func (s *Server) writeResp(conn net.Conn, resp Response) {
    _ = writeResp(conn, resp)
}

// Free function — returns the error. Used by long-lived handlers.
func writeResp(conn net.Conn, resp Response) error { ... }
```

Every other handler keeps its current call pattern; pulse uses
the free function form.

### 4. `kernel/controlplane/client.go` — `StreamUntilCancel`

`Stream`'s open-ended sibling. Returns when:
- ctx cancelled → returns nil (operator-clean exit)
- server sends RespError → returns `*ErrServerError`
- server closes conn → returns wrapped read error

The trick: net.Conn reads don't honor ctx. A small watcher goroutine
closes the conn when ctx is done; the subsequent Read returns an
error, ctx.Err() is non-nil, and we return nil to distinguish "user
hit Ctrl+C" from "the daemon died on us."

### 5. `cmd/agt/pulse.go` — CLI subcommand

Flag parsing (kept tiny — no `flag` package), pattern/kind args
forwarded to the server, two renderers:

**Human format** — one line per event, fits a 100-column terminal:
```
14:32:07.412  seq=42     agent.spawned          subject=agent.01H...  actor=kernel
14:32:07.413  seq=43     llm.request            subject=llm.01H...    actor=agent
14:32:07.812  seq=eph    llm.token              subject=llm.01H...    actor=agent
14:32:08.104  seq=44     llm.response           subject=llm.01H...    actor=agent
```

The `seq=eph` marker for `IsEphemeral()` events (streaming token
chunks) prevents confusion — those have `Seq=0` and no Hash, and
mixing them with `seq=0` from a fresh-journal first event would be
ambiguous.

**JSON format** — one canonical `event.Event` per line. Pipe to
`jq` for ad-hoc queries or to a log shipper.

`signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` makes
Ctrl+C a first-class concern — every other CLI command finishes in
milliseconds, but pulse is the first one where the operator needs
to *stop* it cleanly.

### 6. `cmd/agt/main.go` — dispatch + help

```go
case "pulse":
    return cmdPulse(args[1:], stdout, stderr)
```

Plus two new help lines.

### 7. `kernel/controlplane/pulse_test.go` — 4 tests

| Test | What it locks in |
|---|---|
| `TestPulse_StreamsEvents` | Subscribe → trigger a run → pulse sees ≥3 events of the run, ending with `task.completed` |
| `TestPulse_FiltersByKind` | Filter `{task.completed}` — only events of that kind cross the socket, every observed Kind matches |
| `TestPulse_ContextCancelExitsCleanly` | Cancel ctx → StreamUntilCancel returns nil (not an error), within 2 sec |
| `TestPulse_DefaultPatternMatchesAll` | Omit `pattern` in args → server defaults to `>`, all events observable |

The first three tests share a pattern: spawn pulse in a goroutine
with a 50ms warm-up sleep before triggering a run. The sleep is
unavoidable — we need the subscription registered with the bus
before any publish; without the warm-up, a fast publish races the
subscribe and the test flakes. 50ms is generous (subscribe is
microseconds) but doesn't slow the suite noticeably.

## Test summary

```
go test ./kernel/controlplane/ -v -count=1 -run TestPulse
=== RUN   TestPulse_StreamsEvents           PASS  (0.16s)
=== RUN   TestPulse_FiltersByKind           PASS  (0.16s)
=== RUN   TestPulse_ContextCancelExitsCleanly  PASS  (0.10s)
=== RUN   TestPulse_DefaultPatternMatchesAll   PASS  (0.06s)

go test ./... -count=1
(all packages PASS)
```

Total suite: **454 passing** (from 450 after M1.n.x). +4 from the
new pulse tests.

## Operator workflow examples

**Watch all tool denials in real time** (e.g., while a teammate is
testing a new tool's policy):
```
agt pulse --kind warden.tool.denied
```

**Watch one specific run's events** while another terminal runs it:
```
# terminal 1:
agt run "investigate that flaky test"
# terminal 2:
agt pulse --subject agent.01H...   # paste the run's correlation prefix
```

**Pipe to jq for ad-hoc queries** (e.g., count provider calls in
the last minute):
```
agt pulse --kind llm.response --json | jq -r '.subject' | sort | uniq -c
```

**Ship to a log aggregator**:
```
agt pulse --json | nc loki.internal 1234
```

The `--json` shape is the canonical `event.Event` struct from
`kernel/event`, so anything that already understands agezt's
event schema (journal export, the eventual Pulse v2 web UI, etc.)
can consume it.

## What's intentionally NOT in M1.u (Pulse v1 → Pulse v2 deferrals)

- **Dropped-events synthetic event.** If a slow operator can't keep
  up with the stream, the bus drops events to that subscriber
  silently. Pulse v2 should emit a synthetic `agezt.pulse.dropped`
  event so the operator's view contains its own gap notice.
- **Historical replay.** Pulse v1 starts at "now"; events from
  before the subscribe never appear. A `--since SEQ` or `--since 5m`
  flag would replay from the journal first, then transition to live
  — Pulse v2 territory.
- **TUI surface.** A curses-style live dashboard (running tasks,
  provider RPS, recent denials) would build on top of pulse. Pulse
  v1 ships only the line-streaming primitive; the dashboard is
  someone else's afternoon.
- **Subject indexing.** `agt pulse --subject 'agent.>' --tail 100`
  to show the last 100 events in a subject — needs journal indexes
  we haven't built. Future.
- **Multi-client fairness.** All pulse subscribers share the same
  4096-event buffer slot; if two operators run `agt pulse` against
  a busy daemon, both compete for the same drop calculus. Not a
  real concern at one or two operators; would matter at scale.

## Files touched

- [kernel/controlplane/protocol.go](../kernel/controlplane/protocol.go) — one new const.
- [kernel/controlplane/pulse.go](../kernel/controlplane/pulse.go) — new (~100 LoC).
- [kernel/controlplane/pulse_test.go](../kernel/controlplane/pulse_test.go) — new (~200 LoC, 4 tests).
- [kernel/controlplane/server.go](../kernel/controlplane/server.go) — split `writeResp` into method + free function; one dispatch case added.
- [kernel/controlplane/client.go](../kernel/controlplane/client.go) — `StreamUntilCancel` method added (~50 LoC).
- [cmd/agt/pulse.go](../cmd/agt/pulse.go) — new (~150 LoC).
- [cmd/agt/main.go](../cmd/agt/main.go) — dispatch case + 2 help lines.

No kernel changes, no provider changes, no agent-loop changes.
Pulse is built entirely on top of the existing bus.Subscribe
plumbing — the value is in exposing it to operators.

## Deferrals after M1.u

- **Pulse v2** — historical replay, dropped-events synthetic,
  TUI dashboard (as above).
- **Planner** — scheduler integration; multi-step plan execution.
  Next pickup.
- **Bedrock SigV4** + non-Anthropic body shapes (M1.m.x).
- **OS-keychain vault encryption.**
- **Browser tool**, **out-of-process plugin host.**

Picking up **planner** next — scheduler-driven multi-step
execution is the next "operator-visible capability" after live
observability, and the existing scheduler package has had the
plumbing waiting since M1.b.
