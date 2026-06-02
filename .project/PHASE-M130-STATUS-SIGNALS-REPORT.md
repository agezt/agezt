# M130 — `agt status` autonomy + actionable signals

## Why
`agt status` is the operator's most-used at-a-glance command — "is the daemon up,
what's it doing?". It reported version/uptime/halt/active-runs/tools/memory/world/
skills/journal/delegation, but was silent on two things an operator most needs to
see immediately:

1. **Scheduled autonomy** — a daemon can be armed with recurring scheduled intents
   (it will act on its own), yet status never showed whether any existed. "Is this
   thing going to do something at 09:00?" required `agt schedule list`.
2. **Pending HITL approvals** — a run blocked waiting for the operator to approve a
   tool call is *the* actionable signal, and it was invisible in status. You'd only
   discover it by running `agt approvals`.

Plus, in multi-tenant mode, the tenant count is basic context that was missing.

## What
`handleStatus` now also returns (all cheap in-memory reads, no journal walk):
- `schedules: {total, enabled}` — folded from the cadence store's `List()`.
- `pending_approvals` — `Approvals().PendingCount()`.
- `tenants` — `tenants.Count()`, **only** when a registry is wired (absent in
  single-tenant mode so there's no misleading "tenants: 0" line).

`agt status` renders:
- `schedules : N (M enabled)` — shown only when N > 0 (quiet for single-shot use).
- `tenants   : N` — only when multi-tenancy is on.
- `approvals : K PENDING — answer with agt approvals` when K > 0, else
  `approvals : none pending` (always shown — "0 waiting" is itself useful).

## Files
- `kernel/controlplane/status.go` — fold schedules/approvals/tenants into the
  result; tenants conditional on `s.tenants != nil`.
- `cmd/agt/status.go` — render the three new signals.
- `kernel/controlplane/status_test.go` — extended `TestStatus_ReturnsExpectedShape`
  (fresh kernel: schedules.total 0, pending_approvals 0, tenants field absent) +
  new `TestStatus_SchedulesAndTenants` (two schedules one paused → total 2 /
  enabled 1; a wired registry → tenants 1).

## Live proof (offline mock)
```
Single-tenant, two schedules armed:
  schedules : 2 (2 enabled)
  approvals : none pending
  (no tenants line — multi-tenancy off)

Multi-tenant, one tenant:
  tenants   : 1
  approvals : none pending
```
The enabled-count-with-pause path (total 2 / enabled 1) is covered by the unit
test (the live `schedule pause` id-extraction was incidental and not needed for
the rendering proof).

## Verification
- 55 packages `ok`, **FAIL 0**; **1420 tests**.
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: all touched files clean under LF.
