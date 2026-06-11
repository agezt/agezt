# Phase M840 — self-healing watchdog (`agezt watchdog`)

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "bizim daemon
durduğunda tekrar onu ayağa kaldıracak 2. bir daemon/servis lazım … agent.exe
down olursa geri kaldıracak bir servis lazım, agezt kendi yapmalı."

## What shipped

A new **`agezt watchdog`** subcommand — the "keep it alive" service. It is the
**same binary** supervising itself (so "agezt kendi yapmalı"): it spawns `agezt
daemon`, waits for it, and respawns it whenever it exits, so a crash brings the
daemon back on its own. Run it instead of `agezt daemon`, or install it as a
Windows service / scheduled task.

- **Backoff** — exponential from 1 s, capped at 30 s; the count resets after the
  daemon has run cleanly for ≥60 s (so an occasional restart doesn't slow the
  next one).
- **Crash-loop guard** — if the daemon restarts more than 6 times within 2
  minutes, the watchdog gives up with a clear error instead of spinning forever.
- **Clean shutdown** — SIGINT/SIGTERM stop the watchdog AND kill the supervised
  daemon (it does not respawn after an intentional stop).
- The supervise loop is factored behind a `proc` interface with injectable
  clock/sleep, so the restart policy is unit-testable without real processes.

`cmd/agezt/watchdog.go` (new) + a `watchdog` case in the command dispatch + a help
line.

## Verification

- **Unit** (`cmd/agezt`): `nextDelay` backoff schedule (base→2×→4×→cap);
  supervise loop restarts then stops cleanly on cancel; crash-loop guard gives up
  after exceeding `maxCrashes`; a running child is killed on cancel.
- **Live** (isolated home): `agezt watchdog` spawned the daemon (pid 116664);
  killing that pid produced `daemon exited after 17s (exit status 1); restarting
  in 1s` → `daemon started (pid 86352)` and the daemon came back ready —
  self-healing confirmed end-to-end.

## Gate

cmd/agezt tests green; vet + staticcheck + linux cross-build clean; gofmt swept.
go.mod unchanged. No new env var.

## Note / next

This is the manual-but-same-binary supervisor. A follow-up could add a one-shot
installer (`agezt watchdog --install` → Windows Task Scheduler / systemd unit) so
it survives reboots automatically; for now the operator runs `agezt watchdog`.
