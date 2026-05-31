# Phase Report — Milestone M26 (`agt doctor` model-readiness check)

> Status: **shipped** · Date: 2026-05-31
> SPEC-08 §3.3. The capability triad (M23 inspect · M24 advise · M25 enforce)
> now lands where an operator actually looks when something's wrong: the
> `agt doctor` preflight checklist.

## Why

M23–M25 gave the catalog's capability data three surfaces: an on-demand view
(`--caps`), a boot banner advisory, and an opt-in hard gate. But the operator
reflex for "something's off" is `agt doctor` — the one command that aggregates
base-dir, daemon, version-skew, journal, tools, and halt state into a single
OK/WARN/FAIL checklist. It said nothing about whether the *running model* can
actually drive the tool loop. Someone debugging "my agent isn't calling tools"
would run `agt doctor`, see all-green, and be none the wiser — the most relevant
fact was the one check that didn't exist.

M26 adds it, reusing the exact `catalog.Model.AgentWarnings` the rest of the
triad uses, so doctor can never disagree with `--caps` or the boot banner.

## What shipped

- **`model` in `CmdStatus` (`kernel/controlplane/status.go`)** — the status
  response now carries `s.k.Model()`, the daemon's configured model. Independently
  useful (`agt status` shows the model) and the input the doctor check needs.
- **`checkModelReadiness` (`cmd/agt/doctor.go`)** — a new doctor check: reads the
  model from status, looks it up in the catalog (read from disk, best-effort),
  and runs `AgentWarnings`. OK when the model advertises tool-use; WARN (with the
  advisory text + a remediation hint pointing at `AGEZT_MODEL` /
  `AGEZT_MODEL_STRICT`) when it doesn't. Conservative: an offline/mock model, a
  nil/unsynced catalog, or a model the catalog doesn't list is an informational
  OK — never a false FAIL. Wired into `runDoctorChecks` between the tools and
  halt checks, only when the daemon is reachable.

No `go.mod` change, no new control-plane command (a field on the existing
status), no new event kind. The check function takes the catalog as a parameter,
so it is unit-tested in isolation.

## Proven

- **Unit (`cmd/agt`):** `checkModelReadiness` over a synthetic catalog — a
  tool-capable model → OK; a tool-less known model → WARN with a tool-use detail
  and a non-empty hint; an unknown-to-catalog model → OK; an empty/`mock` model →
  OK; a nil catalog → OK (no false alarm in any of the conservative cases).
- **Live (daemon, custom catalog):** booting on `acme-mini` (`tool_call=false`),
  `agt doctor` prints `[WARN] model readiness : acme-mini — … does not advertise
  tool-use …` (summary `6 ok, 1 warning, 0 failed`); booting on `acme-pro`
  (`tool_call=true`), it prints `[OK] model readiness : acme-pro (agent-ready:
  advertises tool-use)` (`7 ok, 0 warnings`). `agt status --json` now includes
  `"model": "acme-pro"`.

1 new test; suite **1197** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Arc — capability data, fully surfaced

| M | Surface | Where |
|---|---|---|
| 23 | inspect | `agt provider check --caps` |
| 24 | advise | daemon boot banner |
| 25 | enforce | Governor strict gate (opt-in) |
| **26** | **diagnose** | **`agt doctor` / `agt status`** |

The same `AgentWarnings` signal now reaches the operator at every touchpoint —
choosing a model, booting, running, and diagnosing — with one source of truth and
no contradiction between surfaces.

## Deferred — named

- **Per-tenant model readiness** — `agt doctor` reports the primary daemon's
  model; a `--tenant` form could assess a tenant kernel's model once doctor grows
  tenant-aware checks.
- **Capability matrix** (`--caps --all`) — still the open M23 item for comparing
  models side by side.
- **Down-routing** (M25) — the larger routing change remains deferred.
