# Phase Report — Milestone 1.gg (Pulse v3: `--last <duration>`)

> Status: **shipped** · Date: 2026-05-29
> Picks one item off the M1.aa "Pulse v3+" deferral list.
> Continues [PHASE-M1.ff-REPORT.md](PHASE-M1.ff-REPORT.md).

## Scope

Pulse v2 (M1.aa) gave operators `--since <seq>` for historical
replay. That works well for "what happened after the run with
seq=1500", but the more common operator question is *time-relative*:

> "What happened in the last 5 minutes?"

Seq is meaningless without prior context — the operator has to
`agt journal head`, subtract a guess at the recent traffic, then
ballpark a seq. M1.gg adds **`--last <duration>`** as pure
client-side sugar that resolves to a wall-clock cutoff the server
filters by:

```
agt pulse --last 5m        # last 5 minutes, then live
agt pulse --last 1h30m     # last 90 minutes
agt pulse --last 30s --kind task.completed | jq   # last 30s, completed tasks only
```

Under the hood, the CLI computes `time.Now() - dur` and sends
`since_ts_ms = <unix-ms>` over the wire. The server filters journal
events by `ev.TSUnixMS >= sinceTSMs` during replay. Live events
are unaffected — the cutoff governs replay only.

| Concern | Status |
|---|---|
| Server `since_ts_ms` arg parsed alongside existing `since` | ✅ |
| Replay filter respects ts cutoff per-event | ✅ tested |
| Future cutoff → empty replay; live stream still works | ✅ tested |
| `since` + `since_ts_ms` compose as AND (intersection, not union) | ✅ tested |
| `agt pulse --last <go-duration>` flag (5m, 1h30m, 45s, etc.) | ✅ |
| Help text updated; status line shows cutoff in HH:MM:SS local | ✅ |
| Zero behaviour change when neither flag is set | ✅ tested (existing tests) |

## Why AND composition (not OR)

When an operator sets both `--since 1500` and `--last 5m`, we keep
only events that pass BOTH cutoffs. Two design choices considered:

- **AND (chosen):** intersection. "Events since seq 1500 AND in
  the last 5 minutes" — gives the more restrictive set. Predictable
  behaviour: each flag *narrows* the result, never widens it.
- **OR:** union. "Events since seq 1500 OR in the last 5 minutes."
  Hard to reason about ("did I get extra events because of seq, or
  because of time?"), and easy to accidentally include way more than
  intended.

The AND default mirrors how `--subject` + `--kind` already
compose — each filter narrows. Composition becomes monotonic in
the result-set sense.

The composing-flags doc-comment on `replayHistorical` is the
single source of truth for this semantics, so a future fourth
filter (kind / subject / since / since_ts_ms / something-new) has
a clear precedent.

## Files

### `kernel/controlplane/pulse.go` — small edits

1. New optional arg `since_ts_ms` parsed alongside `since`.
2. The `if since >= 0` guard widened to `if since >= 0 || sinceTSMs >= 0`
   so a `--last`-only invocation triggers replay (no `--since`
   needed alongside).
3. `replayHistorical` signature gained a `sinceTSMs int64`
   parameter; the filter check is `if sinceTSMs >= 0 && ev.TSUnixMS < sinceTSMs`,
   identical structure to the existing seq check.

The dedup-via-`lastReplayed` machinery from M1.aa carries over
unchanged. The time filter is purely additive — an event passing
the seq filter still needs to pass the ts filter (and vice versa).

### `cmd/agt/pulse.go` — `--last` flag

- New flag parsing: `--last <go-duration>` calls
  `time.ParseDuration`, refuses non-positive values.
- Resolved at flag-parse time to a Unix-ms timestamp; the wire
  carries the resolved timestamp (not the duration), so a slow
  network or a deferred connect doesn't accidentally shift the
  window from "5 minutes ago" to "5 minutes ago at connect time."
- Status line added: `replaying since 14:32:07` so operators see
  the cutoff that's actually in effect.
- Help text lists both flags + notes the AND composition.

### `kernel/controlplane/pulse_v3_test.go` — new (~165 LoC, 3 tests)

| Test | Locks in |
|---|---|
| `TestPulse_SinceTSMs_ReplaysOnlyRecent` | Events older than cutoff are skipped; newer events replay. Cutoff falls strictly between two runs (verified by inserted sleep). |
| `TestPulse_SinceTSMs_FutureCutoffSkipsAll` | Cutoff in the future replays nothing; live stream after subscribe still arrives (regression guard analogous to M1.aa's high-since test). |
| `TestPulse_SinceTSMs_ComposesWithSince` | Both filters applied as AND: `since=999999` + `since_ts_ms=0` replays nothing (since-filter wins, no leakage from ts-filter to union). |

The three M1.aa pulse-v2 tests remain unchanged and still pass —
v3 sits cleanly on top of v2 without regression.

## Operator workflow examples

**Investigate "what happened in the last 5 minutes":**

```
agt pulse --last 5m
       replaying since 14:32:07
  14:32:08.104  seq=1500  llm.request           ...
  14:32:08.842  seq=1501  llm.response          ...
  ...
# transitions to live after replay drains
```

**Last 30 seconds of completed-task events only:**

```
agt pulse --last 30s --kind task.completed --json | jq
```

**"Anything in the last hour with seq >= 1500":**

```
agt pulse --since 1500 --last 1h
       replaying from seq=1500
       replaying since 13:32:07
# Replays only events satisfying BOTH cutoffs (AND).
```

**Audit: what completed in the last business day?**

```
agt pulse --last 8h --kind task.completed | jq '. | {ts,actor,subject}'
```

## Test summary

```
go test ./kernel/controlplane/ -v -count=1 -run TestPulse_SinceTSMs
(3 tests — all PASS)

go test ./... -count=1
(35 packages — all PASS, no regressions)
```

+3 from M1.gg.

## What's intentionally NOT in M1.gg

- **TUI dashboard.** Still on the Pulse v3+ list. The JSON output
  (`agt pulse --last 5m --json`) is now expressive enough that a
  TUI is a downstream rendering choice (textualize / bubbletea
  / etc.), separable from the kernel.
- **`--until <seq>` / `--until <ts>`.** Pure replay window without
  going live. Useful for "extract events between A and B." Would
  fit naturally next to `--since` / `--last` but introduces a new
  semantics (terminate replay early vs. continue to live), so
  defer to its own phase if demand surfaces.
- **Replay rate limit.** A million-event journal could still swamp
  the client. Wait for a real complaint before adding the knob.
- **Subject indexing for fast `--since` jump.** O(n) journal scan
  is still the implementation; for a busy long-running daemon
  this becomes meaningful. A sidecar `~/.agezt/journal.idx`
  would let `--since` jump straight to a byte offset — separate
  phase.
- **Server-side duration parsing.** The CLI resolves `--last 5m` to
  a Unix-ms timestamp before sending. Doing the parse server-side
  (`since_dur_secs` on the wire) would shift the "5 minutes ago"
  anchor to the server's wall clock at receive time, which feels
  more "correct" but creates a clock-skew failure mode that
  client-side resolution avoids. Stick with timestamps on the wire.
- **Composing-mode override.** No flag to switch AND→OR. AND is
  the right default; if a real operator scenario demands OR
  (e.g. "events since seq 1500 OR in the last 5m — show me
  recovery context plus the recent bit"), revisit then.

## Files touched

- [kernel/controlplane/pulse.go](../kernel/controlplane/pulse.go) — `since_ts_ms` arg + ts filter in `replayHistorical`; widened signature.
- [cmd/agt/pulse.go](../cmd/agt/pulse.go) — `--last` flag + status line + help.
- [kernel/controlplane/pulse_v3_test.go](../kernel/controlplane/pulse_v3_test.go) — new, 3 tests.

Zero changes to bus, journal, governor, agent loop, providers, or
any plugin. M1.gg sits cleanly on top of M1.aa's replay infrastructure.

## Deferrals after M1.gg

Unchanged from M1.ff, minus the `--last` item just shipped:

- Pulse v3+: TUI dashboard, `--until <seq|ts>`, replay rate
  limit, subject indexing.
- Planner v2 (re-planning, sub-planners, planner-side tools).
- Plugin sandboxing, hot-reload, streaming, callbacks (signing
  done; rest remain).
- Browser: JS rendering, screenshots, search, cookies.
- Non-Anthropic body shapes on Bedrock.
- Vault: OS-keychain integration, argon2.
- MCP bridge v2 (resources/sampling/progress/cancellation/SSE/image).
- Routing extensions (TaskRouteRequire, per-task-type budgets,
  per-task-type model overrides) — wait for demand.
- AWS extensions (SSO, assume-role, web identity, process creds,
  IMDSv1) — wait for demand.
- Plugin signing extensions (GPG, TOFU, sandboxing, per-plugin
  tool allowlist) — wait for demand.
