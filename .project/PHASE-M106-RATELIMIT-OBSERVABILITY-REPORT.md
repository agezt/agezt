# Phase Report — Milestone M106 (rate-limit observability + primary cap)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 / governor.

## Why

The governor enforces a per-minute call-rate cap and journals a `rate.limited`
event each time it refuses a call — but that signal was only reachable via
`agt journal grep --kind rate.limited`. An operator running cost/abuse-controlled
workloads couldn't see, per tenant, whether callers were being throttled. Silent
throttling is an SRE blind spot. Worse, the cap could only be set per *tenant*
(`AGEZT_TENANT_RATE_PER_MIN`); the **primary** governor had no rate limit at all.

## What shipped

- **`AGEZT_RATE_PER_MIN=<n>`** — a per-minute call cap for the PRIMARY governor
  (0/unset = unlimited; malformed = hard startup error, mirroring the other
  numeric knobs). Closes the gap where only tenants could be rate-limited, and
  makes the throttle path demoable on the primary.
- **`agt ratelimit log [N] [--tenant <id>] [--since <dur>] [--json]`** — a
  timeline of throttle events (ts, used, limit/min).
- **`agt ratelimit stats [--tenant <id>] [--since <dur>] [--json]`** — total
  throttled, the configured limit, and the worst observed overshoot.
- Both are tenant-routed (via `kernelFor(tenantOf(req))`) and on the tenant-token
  allowlist, so a tenant can read its OWN throttle health.

## Tests

- `TestRateLimitLogAndStats` — a governor capped at 1/min, wired to the kernel
  bus, throttles repeated runs; stats reports `throttled >= 1` with the right
  limit and the log returns rows.
- `TestRateLimitStats_Empty` — a fresh kernel reports zero throttles.

Test count: **1362 → 1364**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_RATE_PER_MIN=1 agezt &
$ agt run "task 1"; agt run "task 2"; agt run "task 3"
$ agt ratelimit stats
  throttled : 3 call(s) refused
  limit     : 1/min
$ agt ratelimit log
  2026-06-01 23:43:42  throttled  used=1  limit=1/min
  ...
```
