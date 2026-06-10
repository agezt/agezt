# Phase M782 — Alert → Channel Notifications

**Date:** 2026-06-10 · **Status:** DONE · **PR:** feat-m782-alert-notify

## What

Push warning/critical alerts — the exact set the console's Alerts view classifies
(run failures, blocked egress, budget ceiling trips, provider rate limits, daemon
halts) — to the configured channels (Telegram, Slack, Discord, email, Signal, …)
the moment they happen. Completes the M777–M781 alert arc: badge and cockpit strip
inform you *in* the console; this informs you when you're *away from it*.

## Design

- **`kernel/alerter`** (new, ~270 LOC): a bus subscriber in the `kernel/anomaly`
  pattern (subscribe `">"`, panic-recovered goroutine, ctx-scoped).
  - `Classify` mirrors `frontend/src/lib/alerts.ts` kind-for-kind:
    `task.failed`/`netguard.blocked`/`rate.limited` → warning,
    `budget.exceeded`/`halt` → critical. Pulse-originated kinds
    (`observer.delta`, `briefing.sent`) are **deliberately excluded** — the Pulse
    engine already delivers its own briefs through the same sinks; handling them
    here would double every heartbeat signal.
  - Delivery reuses the existing **Pulse sink infrastructure**: a
    `pulse.Brief` with `DispAlert` (send-now, high-priority — the disposition
    that breaks through digests/quiet hours), correlation threaded for `agt why`,
    `IssueKey = alert/<kind>`. Kernel never imports plugins; the daemon injects
    the same combined `MultiSink` it builds for Pulse.
  - **Spam-safe by construction**: per-alert (kind+correlation) cooldown
    (default 5m) suppresses repeats; a global sliding-window flood cap
    (default 12 per 10m) bounds cascading failures. Dedup map is size-bounded
    (prune at 4096) for long daemon lives.
- **Wiring** (`cmd/agezt/main.go`): the per-channel sink list at the Pulse build
  site is factored into `channelSinks` and shared; `buildAlertNotify` reads the
  env knobs and starts the watcher; a boot-banner status line reports
  on/disabled/no-channel.
- **Config**: opt-in `AGEZT_ALERT_NOTIFY=1`; `AGEZT_ALERT_NOTIFY_LEVEL`
  (`critical` narrows; default warning+), `AGEZT_ALERT_NOTIFY_COOLDOWN`,
  `AGEZT_ALERT_NOTIFY_MAX`. All four registered in `controlplane.configEnvVars`
  (M127 guard test) and as a new built-in **Alert Notifications** section in the
  Config Center schema (`kernel/settings/schema.go`).

## Tests (8 new, all green)

- `TestClassify_MirrorsConsoleAlertRules` — 10-case table: five notify kinds with
  payload field extraction (reason/error fallback, ip—reason join, tool→egress
  source default); tool.invoked / observer.delta / briefing.sent are NOT alerts.
- `TestHandle_MinLevelGate` — critical-only mode drops warnings.
- `TestHandle_DedupCooldown` — same kind+run suppressed inside the cooldown;
  different run and elapsed cooldown deliver (injected clock, deterministic).
- `TestHandle_RateCap` — 8 distinct alerts at cap 3 → exactly 3; cap frees after
  the window slides.
- `TestBrief_RendersSeverityAndDisposition` — 🚨/⚠ titles, DispAlert, correlation,
  IssueKey, body with source line.
- `TestStart_DeliversFromRealBus` — end-to-end over a real journal-backed bus:
  published task.failed reaches the sink; tool.invoked doesn't.
- `TestStart_NilGuards` — no bus / no sink → nothing starts.

Also: `gofmt` drift in `kernel/settings/schema.go` (pre-existing on main, struct
tag alignment) fixed while touching the file.

## Runtime smoke (isolated daemon — never the real ~/.agezt)

Isolated `AGEZT_HOME` + demo-echo provider + webhook channel (outbound-only)
pointed at a local capture listener, `AGEZT_WEBHOOK_ALLOW_PRIVATE=1`:

1. **Enabled** (`AGEZT_ALERT_NOTIFY=1`): boot banner
   `alert notify : on (level≥warning → channels; repeats suppressed; flood-capped)`.
   Real `agt halt` → listener captured
   `{"channel_id":"ops","priority":"notify","text":"📣 🚨 daemon halted\nsource: kernel",…}`. ✅
2. **Cooldown live**: `agt resume` + immediate second `agt halt` → no second POST. ✅
3. **Negative control** (notify unset): banner
   `alert notify : disabled (set AGEZT_ALERT_NOTIFY=1 …)`; `agt halt` → POST count
   still 1. ✅
4. 0 panics; clean `agt shutdown`; smoke dir removed.

## Gate

- Full suite: `GOMAXPROCS=3 go test -p 2 ./...` — **all green** (84 pkgs).
- `go vet ./...` clean; `go build ./...` clean.
- gofmt verified on the **staged LF blobs** (Windows CRLF working-copy noise
  ignored per protocol).
- `go.mod`/`go.sum` unchanged (stdlib only). No frontend change (dist untouched).
- CI: org Actions billing still exhausted (every job fails at startup, steps=0)
  → full local CI-equivalent battery run + documented on the PR, merged under
  arc authority (same as PRs #136–#225).

## Follow-ups (optional)

- Per-channel alert routing (e.g. criticals → SMS, warnings → Slack).
- Live (no-restart) toggle via the mu-guarded-kernel-field pattern (M710 recipe).
- Mute window / per-kind opt-out.
