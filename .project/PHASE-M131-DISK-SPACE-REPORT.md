# M131 — Disk-space observability (`agt disk` + doctor check)

## Why
This is a **real operational risk**, not polish. The journal is append-only:
segments rotate at 64 MB but are **never deleted**, so total journal size grows
forever. On the ROADMAP's stated deploy target — a "$5 VPS" — the #1 silent
failure mode is a full disk: once it fills, the journal can no longer write,
which means the daemon stops recording what it does (and durable-before-publish
writes start erroring). Nothing in the operator's go-to surfaces showed this
coming: there's an opt-in Pulse `DiskObserver`, but `agt doctor` (the first-look
diagnostic) and `agt status` were silent on disk.

## What
- **`CmdDiskStats`** — the daemon reports its **own** journal size on disk
  (sum of segment files) and the free/total bytes of the filesystem its base dir
  lives on. The daemon must report this (not the client) because the journal is on
  the daemon's host, which may differ from where `agt` runs.
- **Cross-platform free-space without coupling**: `controlplane` deliberately does
  not import `kernel/pulse` (see `pulse_control.go`). So the disk probe is
  *injected*: `Server.SetDiskFree(pulse.DiskUsage)` at daemon startup (cmd/agezt
  already imports pulse). `pulse.DiskUsage` is stdlib-only (`syscall.Statfs` on
  unix, `GetDiskFreeSpaceExW` on Windows). When not wired, the handler reports
  `disk_available: false` rather than failing.
- **`agt disk [--json]`** — direct inspection: journal size, free/total, percent,
  with a low-space nudge.
- **`agt doctor` disk check** — `diskCheckFromStats`: OK normally, **WARN under
  10%** free, **FAIL under 3%** (the journal will soon fail to write). Both name
  the remedy: archive a window with `agt journal export`, then free space.

## Files
- `kernel/controlplane/server.go` — `DiskFreeFunc` type, `diskFree` field,
  `SetDiskFree`.
- `kernel/controlplane/disk.go` (new) — `handleDiskStats`, `dirSize` (recursive,
  best-effort segment-size sum).
- `kernel/controlplane/protocol.go` — `CmdDiskStats`; `server.go` dispatch.
- `cmd/agezt/main.go` — `srv.SetDiskFree(pulse.DiskUsage)`.
- `cmd/agt/doctor.go` — `checkDisk`, `diskCheckFromStats`, `humanBytes`, thresholds;
  wired into `runDoctorChecks`.
- `cmd/agt/disk.go` (new) — `cmdDisk`; dispatch in `main.go`.
- Tests: `cmd/agt/doctor_test.go` (`TestDiskCheckFromStats`, `TestHumanBytes`);
  `kernel/controlplane/disk_test.go` (handler with injected probe → journal bytes
  + 25% free; no-probe → unavailable).

## Live proof (offline mock, Windows host)
```
$ agt disk
base dir : …/home
journal  : 8.9 KB
disk     : 1.1 TB free of 1.8 TB (60.5%)

$ agt doctor
  [OK  ] disk             : journal 8.9 KB; disk 60% free (1.1 TB)
```
Real numbers via the Windows `GetDiskFreeSpaceExW` path — the cross-platform probe
works. The WARN (<10%) / FAIL (<3%) verdicts are unit-tested (can't fill a 1.8 TB
disk in a live demo).

## Verification
- 55 packages `ok`, **FAIL 0**; **1424 tests**.
- `go vet` clean, `GOOS=linux go build ./...` ok, `go.mod` / `go.sum` unchanged.
- gofmt: all touched/new files clean under LF.
