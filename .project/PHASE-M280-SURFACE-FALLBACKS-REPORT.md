# M280 — Make silent provider fallbacks visible

## Why
M279 exposed a sharp edge: the always-on mock fallback caught a real provider
error (a 400 on every request) and silently served every run from the mock. The
only trace was a `provider.fallback` event buried in the journal. An operator
would see runs "succeed" while the real model was never used. This milestone
surfaces fallbacks on the three operator health surfaces so the next such
degradation is caught at a glance — the hardening noted at the end of M279.

## What
- **`kernel/controlplane/status.go`** — `countProviderFallbacks()` folds the
  journal for `provider.fallback` events, returning the count and the most recent
  reason (truncated). `CmdStatus` gains `provider_fallbacks: {count, last_reason}`.
- **`cmd/agt/status.go`** — `agt status` renders a `fallbacks: N  ⚠ …` line with
  the last reason when count > 0 (quiet at zero — the healthy case).
- **`cmd/agt/doctor.go`** — `checkProviderFallbacks` (+ pure
  `providerFallbackCheck`) raises a `provider-fallbacks` WARN folded from
  `CmdJournalStats` `by_kind`, mirroring the existing mesh-loop check.
- The Web UI Status panel renders the whole status map, so it carries
  `provider_fallbacks` automatically.

## Files
- `kernel/controlplane/status.go` — fold + status field (edited).
- `cmd/agt/status.go` — render line (edited).
- `cmd/agt/doctor.go` — `checkProviderFallbacks` / `providerFallbackCheck` (edited).
- `kernel/controlplane/status_fallbacks_test.go` — 1 test (new): publishing two
  `provider.fallback` events makes `CmdStatus` report count 2 + the latest
  reason; zero before any.
- `cmd/agt/doctor_fallbacks_test.go` — 2 tests (new): `providerFallbackCheck`
  stays quiet at zero/absent and WARNs (with a hint) on a non-zero count.

## Verification
- `go test ./cmd/agt/ ./kernel/controlplane/ -run 'Fallback|Fallbacks'` — green;
  full suite **1887 → 1890** (+3), 68 packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on all touched files; `go vet` clean;
  `GOOS=linux build` clean; `go.mod` / `go.sum` unchanged.
- **Live-proven** with a deliberately-broken provider (the real gateway + a wrong
  key → 401 on every call → mock fallback):
  - `agt status` →
    `fallbacks : 3  ⚠ a provider errored; runs served by a backup` /
    `last: openai: status 401: {"error":"Invalid API key"}`;
  - `agt doctor` → `[WARN] provider-fallbacks : 3 provider fallback(s) — a
    primary provider errored and a backup served the run`;
  - the healthy fixed daemon (real gpt-5.5) shows **no** fallback line / no WARN.

## Scope notes
- Pure read-side fold over an existing event; no new event, no new dependency.
- Counts are lifetime-to-date (whole journal). A windowed variant
  (`--since`) could follow if noise becomes an issue, but a non-zero lifetime
  count is itself the signal worth investigating.
- Closes the loop opened by M279: the bug that hid behind the mock fallback would
  now be flagged by `agt status` and `agt doctor` immediately.
