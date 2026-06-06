# M510 — Mutation testing webhook: pin the 2xx success-window upper edge

## Context
Twenty-first package in the mutation pass: `kernel/webhook` (the outbound-webhook
dispatcher — bus subscription, HMAC-SHA256 signing, retry/backoff, delivery journaling,
egress-guarded client). Run with `GOMAXPROCS=3` (CPU-capped). Score 0.477, 72 survived;
working tree restored clean after the run (`git checkout -- .` + `rm -f report.json`).

## The genuine gap (closed)
A delivery counts as successful only inside the 2xx window:

```
if err == nil && status >= 200 && status < 300 {   // deliver (host path)
func (r ProbeResult) OK() bool { return r.Err == "" && r.Status >= 200 && r.Status < 300 }
```

The existing tests cover the **lower** edge and clear failures (200 delivered, 500/503
failed) but **never the upper edge**: no case sits on 299 vs 300. So `< 300 → <= 300`
survived on *both* copies of the boundary — under the mutant a status **300**
("Multiple Choices", which Go's client does NOT auto-follow) is wrongly treated as a
delivered/OK 2xx, masking a failed delivery instead of retrying and journaling it.

## Fix
Added `status_boundary_test.go` (`package webhook`, internal):
- `TestProbeOK_StatusBoundary` — table over 199/200/201/299/300/400/500 pinning both
  edges of `ProbeResult.OK` (200 and 299 true; 199 and 300 false).
- `TestDispatch_Status300IsFailure` — a sink returning 300 on every attempt must spend
  all MaxAttempts and journal `webhook.failed`, never `webhook.delivered`.

## Negative control (manual, CPU-capped)
- deliver path `status < 300 → <= 300`: FAIL (`TestDispatch_Status300IsFailure` — 300
  journaled as delivered).
- `OK()` `r.Status < 300 → <= 300`: FAIL (`TestProbeOK_StatusBoundary` — OK(300) true).
Restored byte-for-byte (`git diff --ignore-all-space` on webhook.go empty); passes again.

## Verification / gate
- Tests pass; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — twenty-one packages (M490–M510)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime, tenant, worldmodel, approval, memory, skill, standing, catalog, plugin,
webhook — plus the controlplane primary-token auth gate verified solid. Recurring
closeable class again: an inclusive range whose *upper* edge end-to-end tests skip
because their status inputs (200 / 500) sit clear of the 299↔300 boundary.
