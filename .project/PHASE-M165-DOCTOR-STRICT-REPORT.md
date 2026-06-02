# M165 — `agt doctor --strict`: exit non-zero on warnings too

## Why
M162–M164 added advisory (WARN) checks for the security/autonomy surface: a failing
schedule, an egress block, request throttling. But `agt doctor` exits 0 on
warnings by design — they're advisories. That's right for an interactive operator,
but it means **monitoring and CI can't alert on them**: a cron job running `agt
doctor` only learns about hard FAILs, never that a scheduled job has started
erroring or a tool is being blocked from internal addresses.

`--strict` closes that gap: it makes any WARN exit 1, so an unattended caller can
treat the advisory signals as actionable. This is exactly what makes the new WARN
checks useful outside an interactive session.

## What
`cmd/agt/doctor.go`:
- `cmdDoctor` parses `--strict` (alongside `--json`); help updated.
- `doctorExitCode(worst checkStatus, strict bool) int` — the pure mapping: FAIL
  always → 1; WARN → 1 only under `--strict`; otherwise 0. Both renderers call it,
  so text and JSON agree.
- `renderDoctorText(checks, strict, w)` — when a `--strict` run is WARN-worst, prints
  `strict: warnings treated as failures (exit 1)` so the operator isn't left
  wondering why a warning-only run "failed".
- `renderDoctorJSON(checks, strict, w)` — adds `"strict"` and `"ok"` (the
  strict-aware exit verdict: false when this run will exit non-zero). `"healthy"` is
  unchanged (still tracks FAILs only) — the two fields answer different questions:
  `healthy` = "is anything broken?", `ok` = "will this invocation pass?".

No new control-plane command, no new event kind — purely a CLI exit-policy flag.

## Tests (+4, all passing)
- `TestDoctorExitCode` — the pure mapping across all six (worst × strict) cases.
- `TestDoctorSummaryExit` — extended to 6 cases: WARN is exit 0 lenient / exit 1
  strict; FAIL is exit 1 in both; OK is 0 in both.
- `TestDoctorJSONShape` — extended: a strict WARN-only run reports `ok:false`,
  `strict:true`, but `healthy:true` (the fields are independent).

## Live proof (offline mock daemon)
A daemon seeded with `AGEZT_SCHEDULE='1s=…'`, left ~28s so a firing failed (the
mock exhausted) → a WARN-worst doctor run. Exit codes measured directly (no pipe,
per the `$?`-is-the-pipe's-last-command gotcha):
- `agt doctor` → **exit 0** (warnings don't fail by default).
- `agt doctor --strict` → **exit 1**, text prints `strict: warnings treated as
  failures (exit 1)`; `--json --strict` shows `"ok": false, "strict": true,
  "worst": "WARN", "healthy": true`.
- A healthy daemon (no failing schedule) `agt doctor --strict` → **exit 0** (strict
  doesn't manufacture failures).

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command or env var.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on both touched files.
- `go test ./... -count=1` — **FAIL 0**, **1530 tests** (was 1526; +4), 61 packages.

## Result
`agt doctor --strict` turns the advisory checks into CI/monitoring gates: an
unattended `agt doctor --strict` in a cron or pipeline now fails the moment a
schedule starts erroring, the egress guard blocks a tool, or a caller gets
throttled — instead of those signals sitting silently until someone looks.
