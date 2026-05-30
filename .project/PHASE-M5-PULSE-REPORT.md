# Phase Report — Milestone M5 (Pulse v1 — the proactive heart)

> Status: **shipped** · Date: 2026-05-30
> Implements the Pulse v1 spine (SPEC-03 §9 MVP cut): the second
> heartbeat that triggers itself and, every beat, asks "what changed?
> · is it important? · should I act or tell the user?". This is the
> Phase 3 milestone — the piece that turns Agezt from a tool you call
> into a presence that notices. Telegram delivery (Phase 4) closes the
> v0.1.0 Jarvis loop; this milestone briefs to the CLI/log.

## Scope

Through M3 (memory-lite) Agezt only acted when asked. M5 adds the
four-stage proactive pipeline from SPEC-03 §1, each stage emitting its
own journaled event so the whole chain is explainable:

```
tick → ① observers → ② salience → ③ initiative → ④ briefing
       (notice)       (weigh)       (decide)       (communicate)
```

**Demo gate met (verified end-to-end):** an operator-configured probe
goes green→red unprompted → `📣 BRIEF [alert] ci probe failed` on the
daemon log → `agt why <briefing_id>` reconstructs
`observer.delta → salience.scored → initiative.taken → briefing.sent`
→ `agt pulse pause` / `agt halt` stop it. The Jarvis moment: it
noticed, judged, and told the operator without being asked.

## What shipped

### New package `kernel/pulse`
- `pulse.go` — core types: `Delta` (Source/Kind/Summary/Before/After/
  Hints, with `Severity()` + `IssueKey()`), the `Observer` interface,
  `Disposition` (`drop|digest|notify|alert|act`), `Dial`
  (`quiet|balanced|chatty`), `Score`, `Delivery`.
- `engine.go` — the resident `Engine`: `Start(ctx)` (heartbeat ticker,
  stops on ctx cancel), `tickOnce` (test-drivable single beat), the
  stage pipeline, digest accumulation/flush, and the control surface
  (`Status`/`StatusMap`/`Pause`/`Resume`). Each delta gets its own
  correlation (so `agt why` walks one issue's chain) plus a causation
  link to the originating tick.
- `salience.go` — rules-first scorer (severity → value+disposition),
  novelty suppression via a state-backed seen-cache (ns `pulse_seen`),
  the dial→delivery routing table, and an **optional** cheap-LLM refine
  (TaskType `salience`, off unless `AGEZT_PULSE_LLM=on`).
- `briefing.go` — `BriefSink` interface + `LogSink` default, single +
  digest composition (grouped by source), and `QuietHours` (wrap-aware;
  only alert/act break through).
- `observers.go` — `ProbeObserver` (runs an operator command via the
  Warden; green↔red transitions only, last-exit persisted in ns
  `pulse_probe` so it survives restarts) and `DiskObserver` (free-space
  threshold crossings). `ParseProbeSpec` / `ParseQuietHours` config
  parsers.
- `diskusage_unix.go` / `diskusage_windows.go` — stdlib-only
  cross-platform free-space (`syscall.Statfs` / `GetDiskFreeSpaceExW`);
  **no new dependency**.

### New event kinds (append-only, DECISIONS B0b)
`pulse.tick`, `observer.delta`, `salience.scored`, `initiative.taken`,
`briefing.sent`, `pulse.paused`, `pulse.resumed`.

### Control plane + CLI
- `kernel/controlplane/pulse_control.go` — `CmdPulseStatus/Pause/Resume`
  handlers behind a `PulseController` interface the daemon injects via
  `Server.SetPulse`, so controlplane never imports `kernel/pulse`. When
  Pulse is off, `status` reports `enabled:false` and pause/resume error
  cleanly.
- `cmd/agt/pulse.go` + `pulse_control.go` — `agt pulse status|pause|
  resume`. The bare-`agt pulse` live-tail (and its `--subject/--kind/
  --json` flags) is **unchanged**; routing only triggers for the three
  control verbs.

### Daemon wiring `cmd/agezt/main.go`
`buildPulse` constructs the engine + observers from env config and the
daemon runs it on the daemon ctx (so `agt halt`/SIGTERM/`agt shutdown`
stop Pulse with everything else) and injects it into the control plane.

Config (same env style as M3's `AGEZT_MEMORY`):

| Env var | Meaning | Default |
|---|---|---|
| `AGEZT_PULSE=off` | disable the engine entirely | on |
| `AGEZT_PULSE_CADENCE` | beat interval | `60s` |
| `AGEZT_PULSE_DIAL` | `quiet`/`balanced`/`chatty` | `balanced` |
| `AGEZT_PULSE_QUIET_HOURS` | e.g. `22-7` (only alerts break through) | off |
| `AGEZT_PULSE_PROBE` | `name=ci;argv=make test` (green↔red detector) | none |
| `AGEZT_PULSE_DISK` | `/:10` (alert under 10% free) | none |
| `AGEZT_PULSE_LLM=on` | enable the cheap-LLM salience refine | off |

## Design rules followed

- **No new external dependency.** stdlib + existing kernel packages
  only; `go.mod` unchanged (POLICY).
- **Pulse borrows governance, owns none.** It reuses the kernel bus,
  Warden, state store, and provider — no side-channel (SPEC-03 §5.1).
- **Every stage journaled, durable-before-publish.** Full provenance:
  `agt why <brief>` reconstructs tick→delta→score→initiative→brief.
- **Observers report deltas, not raw data**, and only on transition —
  no flooding the bus every beat (SPEC-03 §3.1). A probe that is
  *already* red at startup establishes a baseline silently (an
  already-broken thing isn't news).
- **Initiative v1 is inform-or-ask only** — `act` is downgraded to
  `ask`; no autonomous fixing yet (the safest first step, SPEC-03 §9.4).
- **Anti-annoyance built in:** novelty seen-cache suppresses repeats,
  the dial gates what surfaces, quiet hours hold non-alerts, digests
  coalesce low-priority items.
- **Decoupled control plane** via an injected interface — controlplane
  has zero compile-time dependency on `kernel/pulse`.

## Test coverage

~30 new tests; `go test ./...` green on host (windows) and
`GOOS=linux` cross-compile; `go vet` clean. Package count 37 → 38
(added `kernel/pulse`).

- `kernel/pulse`: full tick→brief chain + shared correlation; dial
  routing (quiet suppresses notify); novelty suppression; quiet-hours
  hold vs alert-breaks-through; digest flush; pause/resume journaling;
  status snapshot; observer-error-is-not-fatal; ctx-cancel stops the
  loop; probe green→red→green transitions + restart-persistence + high
  severity; disk threshold crossings + critical; config parsers.
- `kernel/controlplane`: status disabled-vs-wired, pause/resume drive
  the engine, pause-while-disabled errors.
- `cmd/agt`: control-verb help/arg-validation; tail routing stays
  scoped to the three verbs.

### Manual end-to-end (mock provider, no API key)
A file-toggled probe (`argv=ls /tmp/ci_ok`) established a green
baseline, then removing the marker produced `📣 BRIEF [alert] ci probe
failed (exit 2)` within one beat; `agt why <id>` showed the full
4-event chain; `agt pulse status` showed beats advancing; `agt pulse
pause` froze the beat counter and `resume` un-froze it; `agt halt` and
`AGEZT_PULSE=off` both stopped/disabled it.

## Deferred (named for the next milestones)

- **Chronos triggers + standing orders** (P3-CHRON-01/STAND-01) — a
  standing order is a named, persistent Pulse config kept alive by
  Chronos.
- **Adaptive cadence** (SPEC-03 §2) — v1 is fixed-interval.
- **Autonomous `act`** + reversal detection + initiative rate-limiting
  (SPEC-03 §5/§8) — needs the trust ladder + reversal path solid first.
- **Telegram/channel briefing** — the `BriefSink` seam is ready; the
  Telegram sink lands in **Phase 4** and closes the v0.1.0 Jarvis loop.
- **World-model relevance/decay** (needs full Memory) and
  **reflection-driven salience recalibration** (SPEC-03 §7).

## Closes / next

Closes the "Agezt only acts when asked" gap — the proactive spine is
live and explainable. **Next: Phase 4 — Telegram & Unified Inbox**: a
duplex Telegram channel (command in, brief out) plus the Pulse→Telegram
sink. When "I write nothing, yet it tells me on Telegram that CI broke,
and I can see why with `agt why`" is true end-to-end, the MVP ships as
**v0.1.0**.
