# M239 — Actionable CLI error for a crashed (stale-socket) daemon

## Why
A first-run / post-crash UX papercut. When `agt` can't reach the daemon there are
two cases, and they gave two very different errors:

- **Never started** — no runtime addr file. `controlplane.NewClient` fails, and
  `dial()` prints a clear hint: *"start the daemon with `agezt`."* Good.
- **Crashed** — the daemon died but left its `control.addr` file behind.
  `NewClient` is purely file-based (it doesn't connect), so it **succeeds** on the
  stale file; the failure then surfaces only when the command makes its own call,
  as a cryptic transport error (e.g. "connection refused") with **no hint**.

So a user whose daemon crashed got an opaque error from every command, while a
user who simply hadn't started it got clear guidance. Same underlying situation
("the daemon isn't up"), inconsistent help.

## What
- **`cmd/agt/main.go`** — `dial()` is refactored into `dialBase(base, stderr)`,
  which after building the client does a cheap liveness probe (`CmdStatus`, 2s
  timeout). On a **transport** failure it prints the same actionable hint and
  returns nil — so the crashed case now reads "daemon recorded but not responding
  … (re)start the daemon" instead of a raw "connection refused" on each command.
  A **server-side** error (`*controlplane.ErrServerError`, e.g. a bad token) is
  distinguished via `errors.As` and is **not** treated as a crash: the daemon is
  alive, so the client is returned and the command surfaces the real error.

The probe is one extra loopback round-trip per command — sub-millisecond and
imperceptible for a human-invoked CLI. `agt doctor` is unaffected: it builds its
client via `ProbeExisting` / `NewClient` directly (not `dial()`) precisely so it
can *report* the stale state rather than bail.

## Files
- `cmd/agt/main.go` — `dialBase` + liveness probe + `errors` import (edited).
- `cmd/agt/dial_test.go` — 2 tests (new): a stale socket (a real free port
  grabbed then closed, written as the recorded addr) returns nil with the
  "(re)start" hint; a base with no runtime files returns nil with the
  start hint.

## Verification
- `go test ./cmd/agt/` — green; **full suite green with no regression** from the
  added probe (the ~40 commands that use `dial()` all benefit; none depended on
  `dial()` skipping the call). Full suite **1779 → 1781** (+2), 66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet ./cmd/agt/` clean.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.

## Scope notes
- The probe distinguishes transport vs server errors, so a wrong tenant token is
  not misreported as a crash — the command still runs and surfaces the auth
  error.
