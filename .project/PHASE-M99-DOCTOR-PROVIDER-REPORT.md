# Phase Report — Milestone M99 (`agt doctor` provider-health check)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 / operator health.

## Why

M89/M90 exposed provider routing data (`agt provider stats`), but — exactly like
the warden's sandbox-downgrade signal before M98 — the most consequential fact
buried there only shows up if an operator goes looking: the daemon has been
**silently falling back** from its primary model provider to a secondary. A high
fallback rate means the primary keeps failing (bad key, an outage, a rate cap)
and the agent is quietly running degraded. That belongs in `agt doctor`, the
go-to "is everything healthy?" diagnostic, as an actionable WARN.

## What shipped

- **`checkProvider` doctor check** — calls `CmdProviderStats` and verdicts:
  - **WARN** when any routed call fell back to a secondary (rate + a hint that
    *names the worst-offending primary* so the operator knows which key/provider
    to look at),
  - **OK** for no fallbacks / no routing yet.
- **`providerCheckFromStats`** — the pure verdict logic, unit-testable without a
  live daemon (mirrors M98's `sandboxCheckFromStats` split).
- **`topFailingProvider`** — picks the provider with the most fallbacks from the
  `fallbacks_by_primary` map (deterministic tie-break by name), feeding the hint.

## Design decisions

- **Synthesis, not another fold** — turns the M90 numbers into a judgement inside
  the one command operators already run; the second of the doctor-as-single-pane
  checks after M98's sandbox.
- **Name the culprit.** A bare "fallbacks happened" is weak; surfacing *which*
  provider is failing turns the WARN into a next action.
- **Best-effort, never a false FAIL.** A missing daemon/stats call or zero routing
  is an informational OK; the check only WARNs on real measured fallbacks.

## Tests

- `TestProviderCheckFromStats` — fallbacks → WARN with the worst provider named;
  no fallbacks → OK; no routing → OK.
- `TestTopFailingProvider` — max-count selection; empty/nil → "".

Test count: **1345 → 1347**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_DEMO_FAIL_PRIMARY=1 agezt &   # primary provider always fails
$ agt run "summarize the project" ; agt doctor
  [WARN] provider routing : 2/2 routed call(s) fell back to a secondary provider (100%)
         ↳ "mock-failshim" is failing most often (bad key, outage, or rate limit);
           check `agt provider stats`
```
