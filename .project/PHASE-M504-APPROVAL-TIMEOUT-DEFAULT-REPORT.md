# M504 — Mutation testing approval: pin the default-timeout guard

## Context
Fifteenth package in the mutation pass: `kernel/approval` (the human grant/deny gate
for capabilities that require approval). Run with `GOMAXPROCS=3` (CPU-capped). Score
0.732 — well-tested. The grant/deny/timeout/cancel resolution paths are covered by the
end-to-end tests; the genuine gap was in construction.

## The genuine gap (closed)
`New(cfg)` defaults an unset timeout: `timeout := cfg.Timeout; if timeout <= 0 {
timeout = DefaultTimeout }`. A Registry built from a bare `Config{}` (no Timeout set —
the common case) must run with the 5-minute default; a 0 timeout makes every submitted
approval auto-deny instantly (the timeout fires at once). Every end-to-end test passes
an explicit Timeout (2s, 80ms, …), so the default path was unpinned — the mutation
`if timeout <= 0 → < 0` **survived**, leaving the zero value un-defaulted.

(`Decision.IsTerminal`'s `return true` also survived but is unused in production and a
trivial classifier — testing it would be padding, so it was deliberately skipped.)

## Fix
`kernel/approval/timeout_default_internal_test.go` (white-box `package approval`, so it
reads the resolved field deterministically with no real-timer race):
`TestNew_DefaultsUnsetTimeout` — `New(Config{}).timeout == DefaultTimeout`, a negative
Timeout also defaults, and an explicit positive Timeout is honored.

## Negative control (manual, CPU-capped)
Applying the survivor (`<= 0 → < 0`) makes the test fail: `timeout = 0s, want
DefaultTimeout 5m0s`; restored byte-for-byte (`git diff --ignore-all-space` on
approval.go empty); passes again.

## Verification / gate
- New test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — fifteen packages (M490–M504)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime, tenant, worldmodel, approval — plus the controlplane primary-token
auth gate verified solid. The recurring gap class is the unset/zero-value default
guard and the off-by-one boundary that end-to-end tests skip because they always pass
explicit, over-the-line values.
