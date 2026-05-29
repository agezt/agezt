# Phase Report — Milestone 1.aa (Pulse v2: replay + dropped notice)

> Status: **shipped** · Date: 2026-05-29
> Post-v1 work per the M1.u/M1.y "Pulse v2" deferral list.
> Continues [PHASE-M1.z-REPORT.md](PHASE-M1.z-REPORT.md).

## Scope

Pulse v1 (M1.u) shipped the live event stream. Two known gaps:

1. **No history.** `agt pulse` always started "now"; events
   that fired before the subscribe were invisible. Operators
   investigating an issue had to either be subscribed already
   or fall back to `agt why <event_id>` which requires the
   event id (chicken/egg).
2. **Silent drops.** When a subscriber couldn't keep up, the
   bus dropped events to that subscriber and incremented a
   counter no one read. The operator saw an "incomplete" view
   they couldn't tell was incomplete.

M1.aa ships both:

```
agt pulse --since 0              — replay everything in the journal, then go live
agt pulse --since 1500           — replay from seq 1500 onward, then live
agt pulse --since 0 --kind task.completed  — replay + live, filtered
```

And a **synthetic `agezt.pulse.dropped` event** that appears in
every pulse stream whenever the subscription buffer overflows,
carrying the count of skipped events.

| Concern | Status |
|---|---|
| `pulse_subscribe` accepts optional `since` arg (int64) | ✅ tested |
| Journal walked once, filtered by pattern + kinds + since | ✅ tested |
| Subscribe happens BEFORE replay so mid-walk events buffer (not lost) | ✅ |
| Replayed events deduplicated against the live stream via `lastReplayed` cutoff | ✅ tested |
| Ephemeral events (Seq=0) bypass dedup (never overlap with durable replay) | ✅ |
| Since past journal head → no false suppression of live events | ✅ tested (regression caught by test) |
| Kind filter applied uniformly to replay AND live | ✅ tested |
| `bus.MatchSubject` exported for cross-package pattern matching | ✅ |
| `agt pulse --since N` flag wired through to the wire | ✅ |
| Dropped-events monitor: 1s ticker checks `sub.Dropped` counter | ✅ |
| Synthetic notice (subject=`agezt.pulse.dropped`, ephemeral) on drop delta | ✅ |
| Notice payload: `dropped_since_last_notice` + `dropped_total` | ✅ tested (shape) |
| No false positives during normal load | ✅ tested (1.5s soak, zero notices) |

## Changes

### 1. `kernel/bus/bus.go` — `MatchSubject` exported

```go
// MatchSubject reports whether subject matches pattern using the
// bus's NATS-style wildcard rules. Exported for callers (notably
// the controlplane's pulse historical-replay path) that need to
// filter journal events by the same pattern the live subscription
// uses, without going through the full Subscribe machinery.
func MatchSubject(pattern, subject string) bool
```

Tiny wrapper around the existing internal `matches` helper.
Critical for the replay path so journal events are filtered by
the same wildcard semantics live subscribers see.

### 2. `kernel/controlplane/pulse.go` — `since` + dedup + drop monitor

Three new responsibilities in the handler:

**Parse `since`.** Optional `int64` arg; `-1` (or missing)
disables replay.

**Replay before live.** Calls `replayHistorical` which walks
`Journal.Range`, filters by pattern + kinds + `seq >= since`,
writes matching events to the client. Returns the highest seq
written (or `-1` if none) for the dedup pass.

**Dedup live events.** The live loop checks `lastReplayed`: if
non-negative and the event is durable (non-ephemeral) with
`Seq <= lastReplayed`, skip — already delivered. Ephemeral events
(streaming tokens) never journal, so the check would always
pass them anyway; explicit `IsEphemeral()` guard makes intent clear.

**Drop monitor.** A 1-second ticker checks `sub.Dropped.Load()`.
When it grows, the handler emits a synthetic ephemeral event:
```json
{
  "subject": "agezt.pulse.dropped",
  "kind": "agezt.pulse.dropped",
  "actor": "agezt",
  "payload": {"dropped_since_last_notice": 5, "dropped_total": 12},
  "seq": 0, "hash": ""    // ephemeral
}
```

Operators see the notice in the stream; JSON consumers parse
the payload; both know their view became incomplete.

Three design choices worth recording:

**Why subscribe before replay** (counterintuitive). If we
replayed first and subscribed second, any event published
*during* the journal walk would be lost — not in the journal
when we read it, not in the subscription that didn't exist
yet. Subscribing first means those events buffer in `sub.C`;
the live loop drains them after replay (with dedup to drop the
ones replay already emitted).

**Why `-1` sentinel for "nothing replayed"** instead of
`since - 1`. First test catch: `--since 999999` on a journal
with 50 events sets `lastWritten=999998`, which then made the
live loop skip every event (all have `seq < 999998`). The
fix: track whether *any* event matched; if none, return `-1`
so the live loop disables dedup entirely.

**Why per-pulse drop notices, not bus-wide.** The drop is
specific to one subscriber's buffer overflow, not a global
bus event. Publishing it through `bus.PublishStreaming` would
notify every other subscriber (including subscribers who
didn't lose anything). Per-pulse delivery via direct write
keeps the notice scoped to who needs to know.

### 3. `cmd/agt/pulse.go` — `--since` flag

```go
case "--since":
    n, err := strconv.ParseInt(...)
    since = n
```

Non-negative int validation; passed through `reqArgs["since"]`.
Help text updated.

The CLI also prints a `replaying from seq=N` line at subscribe
time when `--since` is set, so operators see both that the
flag was honoured and what cutoff is in effect.

### 4. `kernel/controlplane/pulse_v2_test.go` — 5 new tests

| Test | Locks in |
|---|---|
| `TestPulse_HistoricalReplay` | Two tasks run before subscribe; `--since 0` replays everything; third task arrives live; seqs strictly monotonic (no dupes between replay + live) |
| `TestPulse_HistoricalReplay_HighSinceSkipsOlderEvents` | `since=999999` → no replay; live events from a second run arrive correctly (regression: caught the `since-1` dedup bug) |
| `TestPulse_HistoricalReplay_HonoursKindFilter` | Replay filters by kind: only `task.completed` arrives, no other kinds leak |
| `TestPulse_DroppedNoticeDoesNotFireWithoutDrops` | 1.5s soak with one normal run → zero `agezt.pulse.dropped` events (false-positive guard) |
| `TestPulse_DropNoticePayload` | Locks in the JSON shape: `dropped_since_last_notice` + `dropped_total` keys |

The drop-notice path itself is hard to exercise without a
synthetic load test (the buffer is 4096 vs. a test's handful of
events). The false-positive guard is the meaningful test here;
the JSON shape test pins the operator-facing contract.

## Test summary

```
go test ./kernel/controlplane/ -v -count=1 -run TestPulse
(13 pulse tests — all PASS, including the 5 new v2 tests)

go test ./... -count=1
(all packages PASS)
```

Total suite: **534 passing** (from 529 after M1.z). +5 from M1.aa.

## Operator workflow examples

**Investigate a completed run after the fact:**

```
agt run "do the thing"
# ... happens ...
# you notice something odd, but the run is over:
agt pulse --since 0 --kind task.completed --kind tool.invoked --kind tool.result | jq
# replays all task/tool events from journal start, then continues live
```

**"What happened between seq 1500 and now?":**

```
agt pulse --since 1500
       replaying from seq=1500
  14:32:07.412  seq=1500  llm.request           ...
  14:32:08.104  seq=1501  llm.response          ...
  ...
  # transitions to live after replay drains
```

**Watch for drops while running a heavy workload:**

```
agt pulse --kind agezt.pulse.dropped
# stays mostly silent; lights up if the kernel publishes faster
# than your terminal can consume:
  14:32:07.412  seq=eph    agezt.pulse.dropped     subject=agezt.pulse.dropped  actor=agezt
```

The `seq=eph` marker (from M1.u's renderer) identifies the
synthetic as ephemeral so operators don't confuse it with a
real journaled drop record.

## What's intentionally NOT in M1.aa

- **TUI dashboard.** Still on the Pulse v2+ list. Easy to build
  on top of the JSON output (`agt pulse --json` already gives a
  stream of canonical event JSON); a TUI is a separate
  rendering layer that doesn't need to live in the agezt
  binary.
- **`--until <seq>` / `--last <duration>`.** Pure replay
  without going live. Useful for "extract these 100 events,
  hand to support" — a separate command (`agt journal export`?)
  fits better than overloading pulse.
- **Replay rate limit.** Replaying a million-event journal
  could swamp the client. v3 could add a `replay_rate` arg
  (events/sec ceiling). Not blocking real use cases today.
- **Subject indexing for fast since-walk.** Journal range is
  O(n) over every event; subject + kind filter is applied
  per-event. For a busy long-running daemon this becomes
  meaningful. A subject-indexed sidecar file would let
  `--since` jump straight to a seq. Future infrastructure.
- **Multi-client drop coalescing.** Currently each pulse
  client has its own dropped counter and its own notice
  cadence. Two clients with the same pattern both get notices
  about *their own* drops, which is correct but means a
  multi-client deployment may produce many notices.

## Files touched

- [kernel/bus/bus.go](../kernel/bus/bus.go) — added exported `MatchSubject`.
- [kernel/controlplane/pulse.go](../kernel/controlplane/pulse.go) — `since` parsing, `replayHistorical`, dedup logic, drop monitor (~80 LoC added).
- [kernel/controlplane/pulse_v2_test.go](../kernel/controlplane/pulse_v2_test.go) — new (~210 LoC, 5 tests).
- [cmd/agt/pulse.go](../cmd/agt/pulse.go) — `--since` flag + status line.

Zero changes to the journal, the bus subscription mechanism,
the agent loop, or any provider. Pulse v2 sits cleanly on top
of v1's existing primitives.

## Deferrals after M1.aa

Unchanged from M1.y/M1.z lists, minus the two items just shipped:

- Pulse v3+ (TUI dashboard, until/last flags, replay rate
  limit, subject indexing — as above).
- Planner v2 (re-planning, sub-planners, planner-side tools).
- Plugin sandboxing, signing, hot-reload, streaming, callbacks.
- Browser: JS rendering, screenshots, search, cookies.
- AWS credential-provider chain.
- Non-Anthropic body shapes on Bedrock.
- **MCP bridge plugin.** Picking up next — high ecosystem value.
- Vault: OS-keychain integration, passphrase rotation, argon2.
- Per-task-type routing.
