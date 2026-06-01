# Phase Report — Milestone M58 (Boot-banner the delegation caps)

> Status: **shipped** · Date: 2026-06-01 · SPEC-12 multi-agent.

## Why

M49 surfaced the delegation ceilings in `agt status` (on demand). But the daemon
boot banner — which already echoes governor, policy, timeouts, tenancy — said
nothing about delegation governance, so an operator reading the startup log
couldn't see the active caps. M58 adds the line.

## What shipped

- **`delegationBanner(k)` (`cmd/agezt/main.go`)** — renders the effective
  delegation ceilings from `k.SubAgentLimits()` (the same M49 source): `off
  (AGEZT_SUBAGENT=off)` when disabled, else `depth≤1, fan-out ≤3, spend $0.5000`
  (0 fan-out/spend → `unbounded`).
- **Boot banner line** — `delegation       : …` printed alongside `policy engine`.

## Design decisions

- **Reuse `SubAgentLimits()`.** Same effective values as `agt status` (M49), so
  the banner and the live query never disagree. No new state.

## Tests

- `cmd/agezt/main_test.go::TestDelegationBanner` — disabled → `off…`; capped →
  `depth≤1, fan-out ≤3, spend $0.5000`; unset caps → `unbounded` fan-out + spend.

Test count: **1295 → 1296**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_SUBAGENT_FANOUT=3 AGEZT_SUBAGENT_SPEND_CAP=0.50 agezt
  delegation       : depth≤1, fan-out ≤3, spend $0.5000
```

## What's next
1. `agt runs list` answer preview column (LOW).
2. `agt runs stats` spend percentiles (LOW).
