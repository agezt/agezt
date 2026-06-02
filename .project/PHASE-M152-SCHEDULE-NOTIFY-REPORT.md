# M152 — Scheduled runs deliver their answer to a channel

## Why
The autonomy axis (schedules) and the channel axis (Telegram/Slack/Discord) existed
side by side but didn't meet: a scheduled intent fired through the governed loop and
its answer went **only to the journal**. So the canonical Jarvis use case — "every
morning, summarise new commits and tell me" — didn't actually *tell* you; you had to
go read `agt runs` or the agent had to be prompted to call the `notify` tool itself
(unreliable: the model might not). The channel sender and allowlists from M142/M143
were right there; the scheduled-run path just discarded the answer
(`_, err := k.RunWith(...)`). Wiring the answer to the channels closes the proactive
loop: schedule → run → deliver.

## What
- **`cmd/agezt/main.go`** — `buildCadence` gained an optional
  `onAnswer(ctx, id, answer)` callback; its `RunFunc` now captures the answer and,
  on success, invokes the callback. In `main`, the callback is built only when
  `AGEZT_SCHEDULE_NOTIFY=on` AND at least one channel has an allowlist; it delivers
  via `deliverScheduled`.
- **`deliverScheduled(ctx, send, targets, id, answer)`** — a testable helper that
  prefixes the answer with `[scheduled: <id>]` (so the operator knows which job
  produced it) and sends it to every configured recipient across channel kinds
  (sorted for deterministic delivery), reusing the same `channelSend` closure and
  `notifyTargets` allowlists as `agt send` / the `notify` tool. Empty answers and a
  nil sender are no-ops; returns the delivery count.
- **`kernel/controlplane/config.go`** — `AGEZT_SCHEDULE_NOTIFY` added to
  `configEnvVars` (M127 drift guard).

Off by default (no callback → unchanged silent behavior). Only successful runs with
a non-empty answer are delivered.

## Files
- `cmd/agezt/main.go` — `onAnswer` param on `buildCadence`; answer capture in the
  RunFunc; `deliverScheduled`; the `AGEZT_SCHEDULE_NOTIFY` gate at the call site.
- `kernel/controlplane/config.go` — env var in `configEnvVars`.
- `cmd/agezt/main_test.go` — `TestDeliverScheduled`.

## Tests (+1, all passing)
- `TestDeliverScheduled` — delivers to all recipients across kinds with the
  `[scheduled: id]` prefix + the answer; an empty/whitespace answer delivers
  nothing; a nil sender is a safe no-op.

## Live proof (offline mock, real booted daemon + fake Discord API)
Booted with `AGEZT_SCHEDULE='1s=summarise the project'`, `AGEZT_SCHEDULE_NOTIFY=on`,
and Discord configured against a fake API; waited for the first cadence tick:

```
banner:  schedule : 1 schedule(s): every 1s → "summarise the project"

(after the tick)
CHANNEL GOT: {"content":"[scheduled: sched-01KT4G…]\n[offline-mock] I ran a directory
             listing via the shell tool. This project is Agezt — an open-source, …"}
```

The scheduled intent fired, ran the agent, and its answer was delivered to the
channel with the schedule-id prefix — proactive autonomy reaching the operator.

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files; M127 drift guard passes.
- `go test ./...` — **FAIL 0**, **1481 tests** (was 1480; +1), 61 packages.

## Result
Scheduled intents are now genuinely proactive: their answers reach the operator's
chats automatically (opt-in, prefixed by job id), joining the autonomy and channel
axes into the "Jarvis tells you things on a timer" loop — without relying on the
agent to remember to message you.
