# Phase M815 — alert mute window + per-source routing

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** alert polish (last
listed backlog item) — give the operator control over WHEN and WHICH
alerts reach the channels.

## What

The M782 alerter pushed every warning/critical to the channels with only
a level gate + dedup + flood cap. M815 adds two operator controls:

- **Mute window** (`AGEZT_ALERT_NOTIFY_MUTE`, e.g. `0-7`): a daily quiet
  window during which WARNING alerts are held — the operator silences
  overnight pings. **CRITICAL alerts always break through** (a budget
  blowout or a halt at 3am is exactly what you DO want to hear). Reuses
  Pulse's `QuietHours` "START-END" 24h form (incl. midnight wrap).
- **Per-source routing** (`AGEZT_ALERT_NOTIFY_MUTE_SOURCES`, e.g.
  `provider,egress`): silence whole alert categories at ANY level —
  muting a category means "I never want these". Sources are the
  classifier's existing labels: run, egress, budget, provider, kernel.

Both gate inside `Notifier.Handle` before dedup/rate (a muted alert
costs nothing downstream). The banner reflects them: "on (level≥warning
→ channels; … ; muted 0-7 (criticals still break through); sources
muted: egress,provider)". Both env vars are in `configEnvVars` (guard
test) and the Config Center "Alert Notifications" section, so they're
UI-settable.

## Tests (alerter + settings + config guard; full battery green)

- MuteWindow: at 03:00 inside 0-7 a warning is muted and a critical
  breaks through; at 09:00 the warning flows (2 delivered, not 3)
- MuteSources: provider (warning) and kernel (CRITICAL) muted → dropped;
  budget (critical) and run (warning) flow (2 delivered)
- ParseMuteSources: comma/space tolerant, lowercased, empty → nil
- settings schema + control-plane configEnvVars guard green
- Full Go suite, vet, staticcheck, linux cross-build green; frontend
  untouched (492 vitest)

## Smoke (isolated AGEZT_HOME, real daemon)

`AGEZT_ALERT_NOTIFY=1` + webhook channel + `AGEZT_ALERT_NOTIFY_MUTE=0-7`
+ `AGEZT_ALERT_NOTIFY_MUTE_SOURCES=provider,egress` → banner: "on
(level≥warning → channels; repeats suppressed; flood-capped; muted 0-7
(criticals still break through); sources muted: egress,provider)". The
config is read, sorted, and surfaced exactly as configured.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; vitest 492; go.mod unchanged; two new AGEZT_*
env vars (both in configEnvVars + the Config Center).

## Backlog now

The only remaining listed items are owner-gated (CI billing → green
badge → v1.0.0) and the provider-embeddings opt-in (which needs an
embeddings-capable keyed provider — verify availability before
building, to avoid burning the owner's budget on a wire that may not
exist).
