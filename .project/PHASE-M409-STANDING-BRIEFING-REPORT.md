# M409 — Standing-order briefing: deliver the result to a channel (SPEC-16 §4)

## Context
A standing order carries a `briefing.channel`. M404–M408 fired orders (bounded by
budget + trust) but discarded the run's answer. This delivers it: when an order
fires and produces a result, brief it to the order's channel — closing the loop
"fire on a trigger → act → tell me". It reuses the exact channel allowlist +
sender path that `SCHEDULE_NOTIFY` already uses (`channelSend` + `notifyTargets`),
so it's not a new external wire.

## What
- **`kernel/standing/runner.go`** — `BriefText(o, answer) (text, ok)`: the pure
  decision + formatting. A briefing is produced only when the order names a
  channel AND the answer is non-empty; the text is prefixed with the order name.
- **`cmd/agezt/main.go`** — `buildStandingRunner` takes a `brief` callback; the
  FireFunc, after the run, calls `brief(o.BriefingChan, BriefText(...))`. The
  runner wiring moved to after `channelSend`/`notifyTargets` are built. A
  `briefTargets` map merges the notify channels with the outbound webhook's
  allowlist, so `--channel webhook` reaches a (locally testable) endpoint too.

## Verification
- **`kernel/standing/runner_test.go`** `TestBriefText`: channel + non-empty answer
  → prefixed text + true; no channel → false; empty answer → false.
- **Negative control:** removing the channel/answer gate in `BriefText` → the
  empty-answer case FAILs; restored byte-identical.
- **Live demo** (mock `AGEZT_DEMO_ECHO=1`, outbound webhook →
  `AGEZT_WEBHOOK_OUTBOUND_URL` at a local sink, `AGEZT_WEBHOOK_CHANNELS=ops`): a
  cron standing order `--channel webhook` fired twice; the journal carried two
  `channel.outbound` events on `channel.outbound.webhook` whose text was exactly
  `"[standing: morning brief]\n[echo]\nsummary line"` — the order name + the run's
  answer delivered to the channel.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2256** passing (was 2255; +1). CHANGELOG (Added, user-visible).

## Scope notes
- **SPEC-16 §4 Chronos standing orders is now functionally complete:** model +
  CRUD (M403), event triggers (M404), `why` (M405), web panel (M406), cron
  triggers (M407), budget + trust ceilings (M408), and briefing delivery (M409).
  A persistent goal can now fire on time or on an event, act bounded by its
  budget and trust ceiling, and report the result to a channel — the full
  trigger → act → brief loop. The richer observers/salience scoring (vs running
  the plan directly) remains a Pulse-integration enhancement, not a gap in the
  core standing-order behaviour.
