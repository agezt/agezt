# M164 — Rate-limit health check in `agt doctor`

## Why
Completing the security-subsystem sweep into the single-pane diagnostic (M162
schedules, M163 netguard), the per-minute rate limiter (M14 quotas / M106 stats)
had no `agt doctor` check.

A `rate.limited` event means a tenant exceeded its per-minute request cap and was
refused. Sustained throttling is an operational smell: a caller is undersized for
its workload, or something is hammering the daemon. To the operator it otherwise
shows up only as mysterious intermittent run failures — surfacing it directly lets
them raise the cap or pace the caller before that happens.

## What
`cmd/agt/doctor.go`:
- `checkRateLimit(ctx, client)` — calls `CmdRateLimitStats` (M106) scoped to the
  last 24h; a call failure is an informational OK (never a false FAIL), mirroring
  `checkNetguard`/`checkWebhooks`.
- `rateLimitCheckFromStats(res)` — the pure, testable verdict:
  - `throttled <= 0` → OK "no requests throttled in the last 24h".
  - else → WARN "N request(s) throttled in the last 24h", and when a cap was
    recorded, the richer "(cap L/min, peak W)". Hint points to `agt ratelimit log`.
- Generalized `netguardLookbackMS` → `doctorRecentWindowMS` (a small refactor): the
  24h self-clearing window is now shared by both log-folding security checks
  (netguard + ratelimit), which have no rate denominator and so would otherwise WARN
  forever on a single historical event. `checkNetguard` updated to use it.
- Wired into `runDoctorChecks` after `checkNetguard`.

Reuses the existing `CmdRateLimitStats` response (`{throttled, limit_per_min,
worst_used, window_ms}`) — no control-plane change.

## Tests (+1, all passing)
`TestRateLimitCheckFromStats`: no-throttle OK; missing-key OK (best-effort);
throttling with cap/peak → WARN with count+cap+peak in the detail; throttling
without a recorded cap → WARN with the simpler detail (no cap clause). The existing
`TestNetguardCheckFromLog` still passes after the const rename.

## Live proof (offline mock daemon)
- **OK path (binary):** a clean mock daemon → `agt doctor` shows
  `[OK  ] ratelimit : no requests throttled in the last 24h` (alongside the
  netguard and sandbox rows), proving the wiring, the real `CmdRateLimitStats` call,
  and the 24h window end to end. The const rename left netguard's row intact
  (`[OK  ] netguard : no egress blocked in the last 24h`).
- **WARN path:** rate-limiting only triggers under a configured per-minute cap with
  a caller exceeding it (M14 multi-tenant quotas), not reachable from the offline
  single-tenant mock. The WARN verdict is proven by the unit test above against the
  real `CmdRateLimitStats` response shape (the same shape the M106 handler emits).

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command or env var.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on both touched files.
- `go test ./... -count=1` — **FAIL 0**, **1526 tests** (was 1525; +1), 61 packages.

## Result
With M162–M164 the `agt doctor` health matrix now covers the full security/ops
surface — sandbox, provider, exposure, **netguard**, **ratelimit**, approvals,
budget, webhooks, schedules, disk, channels, halt — so an operator sees throttling,
egress blocks, and failing automation in one place instead of discovering them as
downstream symptoms.
