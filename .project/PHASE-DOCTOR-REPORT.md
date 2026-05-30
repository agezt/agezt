# Phase Report — `agt doctor` (hardening toward v0.1.0)

> Status: **shipped** · Date: 2026-05-30
> ROADMAP §2.1 names `agt doctor` + zero-config first run as an **always-on
> MVP essential**; SPEC-08 §3.3 specifies it checks version skew and flags
> incompatibilities. This lands it — the first step of hardening toward the
> **v0.1.0** tag (the MVP success test, ROADMAP §2.2).

## Scope

A single preflight command an operator runs first when something feels
wrong — or on a fresh install to confirm the system is ready. It runs a
checklist and prints each result as **OK / WARN / FAIL** with a remediation
hint, then a summary line. Exit **0** when nothing failed (warnings are
advisories, not failures), **1** when any check FAILed, **2** on bad args.
`--json` emits the machine form for CI / scripts.

## What shipped

### `cmd/agt/doctor.go` — client-side diagnostics
A pure CLI command (no daemon round-trip beyond reusing existing read
commands), so it degrades honestly when the daemon is down:

| Check | OK | WARN | FAIL |
|---|---|---|---|
| **base directory** | exists + writable (proven by a write probe) | not created yet (run `agezt` once) | unresolvable / not a dir / not writable |
| **daemon** | running and reachable | — | no control-plane socket, or `status` call failed |
| **version skew** | client and daemon aligned | client/daemon version or protocol differ | — |
| **journal** | BLAKE3 hash chain verified (+head seq) | — | `journal_verify` failed (tamper/truncation) |
| **tools** | N registered | 0 registered (no capabilities) | — |
| **halt state** | running | system HALTED (resume with `agt resume`) | — |

Local checks (base dir) always run; daemon-dependent checks run when the
daemon is up and **collapse to a single `daemon` FAIL** when it isn't,
rather than emitting six identical "unreachable" lines.

### Wiring
`cmd/agt/main.go` — `doctor` dispatch case + a help line. No other change.

## Design rules followed

- **No new control-plane command, no new event kind, no `go.mod` change.**
  Every daemon check reuses an existing command (`CmdStatus`,
  `CmdJournalVerify`); the version-skew logic mirrors `agt status`. `doctor`
  is a *composition* of surfaces that already exist — the smallest possible
  addition for an MVP essential.
- **Honest degradation** — builds the control-plane client directly (not
  `dial()`), so "daemon down" is a reported check result, not a printed
  error that aborts the rest.
- **Exit-code contract** — warnings never fail the command (a fresh install
  with 0 tools or an uninitialised base dir is *informative*, not broken);
  only a FAIL (broken journal, unreachable daemon) returns 1, so CI can gate
  on it.

## Test coverage

7 new tests (`cmd/agt`): help/arg exit codes; `checkBaseDir` branches
(writable→OK, resolve-error→FAIL, missing→WARN) exercised directly; the
summary exit-code contract (ok/warn→0, fail→1) for both text and JSON
renderers; JSON shape. `go test ./...` green on host (windows) + `GOOS=linux`
cross-compile; `go vet` + `gofmt -l` clean.

### Manual end-to-end (mock provider)
- **No daemon:** base dir OK, `daemon` FAIL (control-plane call refused) with
  hint → **exit 1**.
- **Daemon up:** all six checks OK (`6 registered` tools, hash chain verified
  at head seq=14, versions aligned) → **exit 0**.
- **`--json`:** well-formed `{checks:[…], healthy:true, worst:"OK"}`.

## Deferred (named for later)

- **Plugin/SDK version skew** (SPEC-08 §3.3 full form) — needs the plugin
  registry + SDK version surface; v1 covers kernel↔client skew (the skew
  that actually bites today).
- **Credential/provider readiness check** ("is at least one provider
  credentialed?") — overlaps `agt provider check`; could fold a lightweight
  form in once a control-plane provider-readiness query exists.
- **Disk-space / port-availability preflight**, and a `--fix` mode that
  offers to remediate (e.g. clear a stale socket).

## Closes / next

`agt doctor` is in place — the operator-facing "is this healthy?" gate the
MVP success test assumes. Combined with the cognitive loop (Phase 2) and the
Web UI (M10), the remaining path to **v0.1.0** is release mechanics: a
`CHANGELOG`, a version stamp (`brand.Version` is still `0.0.0-m0`), and the
end-to-end MVP success-test walkthrough (ROADMAP §2.2).
