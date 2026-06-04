# M387 — Skill auto-quarantine on repeated failure (SPEC-05 §5)

## SPEC audit (read-vs-code)
SPEC-05 §5 / `skill.go`: `StatusQuarantined` is "pulled from production by a
regression or repeated failure (v1: operator-driven; **auto-quarantine
deferred**)." The Forge already had the pieces — `Metrics{Successes, Failures}`,
a manual `Quarantine`, and `RecordOutcome` (whose own comment said "the hook
auto-quarantine will later read") — but two links were missing:

1. **Attribution.** `RecordOutcome` had **no production caller** — failure
   metrics never bumped in a real run, so any auto-quarantine reading them could
   never fire. Verified by grepping callers (only List/Get/Promote/Quarantine/
   Revert/Create were wired).
2. **The trigger.** Nothing turned a failure record into a quarantine.

`RunWith` already retrieves the run's activated skills (`forge.Activate` → `hits`)
and knows the outcome (`agent.Run`'s `err`), so both links are a clean wire.

## What
- **`kernel/skill/forge.go`** — `RecordOutcome(corr, ids, success)` now, on a
  failure, calls `maybeAutoQuarantine`: an ACTIVE skill is pulled when it crosses
  BOTH a minimum failure count (default 3) AND a failure rate (default ≥50%) —
  conservative so a mostly-successful skill isn't yanked. Reuses the existing
  `Quarantine` (transition + `skill.quarantined` journal, carrying the failing
  run's correlation). `SetAutoQuarantine(min, rate)` tunes/disables it;
  `Default*` constants hold the thresholds.
- **`kernel/runtime/runtime.go`** — captures the activated skill ids from
  `Activate` and, after the run, calls `forge.RecordOutcome(corr, ids, err==nil)`
  — the production caller. Best-effort; never changes the run result.
- **`cmd/agezt/main.go`** — on by default; `AGEZT_SKILL_AUTOQUARANTINE=off`
  disables (banner line + config inventory entry).

## Verification
- **`kernel/skill/autoquarantine_test.go`** (4 tests, capturing bus + journal):
  3 failures @ 100% → quarantined + `skill.quarantined` with an auto reason
  carrying the run correlation; 10 successes + 3 failures (23% rate) → stays
  active; a SHADOW skill with 5 failures → not touched (active-only); disabled
  via `SetAutoQuarantine(0,0)` → stays active after 6 failures.
- **Negative control:** neutering the `maybeAutoQuarantine` call in
  `RecordOutcome` → the threshold test FAILs (status active, want quarantined);
  restored `forge.go` byte-identical.
- **Live demo** (daemon, `AGEZT_DEMO_LOOP=1` so each run fails via max-iters):
  imported a `loop-demo-runner` SKILL.md, promoted to active, ran "run loop demo"
  3× → `skill.activated` ×3, metrics `failures:3/successes:0`, status
  **quarantined**, journal reason `auto-quarantine: 3/3 runs failed (100%)`.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2172** passing (was 2168; +4). CHANGELOG (Added, user-visible).

## Scope notes
- Closes the **auto-quarantine** half of SPEC-05's deferred automation. The other
  half — **auto shadow-test** (auto-promote a draft to shadow + evaluate before
  active) — remains a larger feature (a shadow-execution/eval harness) and is
  recorded as deferred in next.md.
- Threshold tuning is on/off via env today; per-threshold env knobs are a trivial
  follow-up if an operator needs them.
