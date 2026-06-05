# M401 — Shadow→active auto-promotion: closing the trust-ladder loop (SPEC-05 §5.2)

## Context
M399 staged drafts to shadow; M400 evaluated shadow skills (LLM-judge "would it
have helped") into `shadow_evals`/`shadow_wins`. This milestone consumes that
evidence: the SPEC-05 §5.2 `shadow —(N successful real uses, gated)→ active`
arrow. With it, the full lifecycle — draft → shadow → active → quarantined — is
self-driving (auto-quarantine M387 is the demotion side; this is its mirror).

## What
- **`kernel/skill/forge.go`** — `apMinWins`/`apWinRate` thresholds
  (`DefaultAutoPromoteMinWins = 3`, `DefaultAutoPromoteRate = 0.5`),
  `SetAutoPromote(minWins, rate)`, and `maybeAutoPromote(corr, sk)`: a SHADOW
  skill whose shadow record crosses BOTH the min win COUNT and the win RATE is
  promoted to active via `promoteWithReason` (the reason names the gate).
  `RecordShadowOutcome` calls it after recording a win — the exact mirror of
  `maybeAutoQuarantine` being called after a failure.
- **`cmd/agezt/main.go`** — `AGEZT_SKILL_AUTOPROMOTE` (on by default, `=off`
  disables) + banner + config inventory. On-by-default is safe because it is
  inert unless shadow evaluation (opt-in, M400) is feeding wins.

## Verification
- **`kernel/skill/shadoweval_test.go`**:
  `TestRecordShadowOutcome_AutoPromotesAfterWins` (2 wins → still shadow; 3rd win
  at 100% → active, `skill.promoted` shadow→active with an auto-promote reason and
  the run's correlation); `TestRecordShadowOutcome_NoPromoteWhenMixedVerdicts`
  (below the win count → stays shadow); `TestRecordShadowOutcome_AutoPromoteDisabled`
  (`SetAutoPromote(0,0)` → never promotes).
- **Negative control:** removing the `maybeAutoPromote` call in
  `RecordShadowOutcome` → the promotion test FAILs (stays shadow after 3 wins);
  restored byte-identical.
- **Live demo** (mock with `AGEZT_DEMO_ECHO=1` — a non-exhausting Responder — and
  all three ladder flags on): the banner shows auto-shadow / shadow-eval /
  auto-promote on; `agt skill import` → shadow; running a matching task fires
  `skill.shadow_evaluated {evals:1, helped:false, wins:0}` live under the run's
  correlation — proving the whole runtime → ShadowEvaluate → judge →
  RecordShadowOutcome → maybeAutoPromote chain executes end-to-end. (The echo mock
  judges NO, so no promotion fires live; the promotion-on-wins decision is proven
  by the unit test, the standard capturing-fake split for mock-limited verdicts.)
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged.
  `TestConfigEnvVars_CoversCmdAgeztReads` green. Full suite **2226** passing (was
  2222; +4). CHANGELOG (Added, user-visible).

## Scope notes
- **The skill trust ladder is now fully self-driving:** draft →[M399 auto-shadow]→
  shadow →[M400 shadow-eval]→ wins →[M401 auto-promote]→ active →[M387
  auto-quarantine]→ quarantined. Remaining polish for the arc: **observability** —
  surface shadow evidence (`shadow_evals`/`shadow_wins`, shadow_evaluated events)
  in `agt skill` and the web Skills panel so an operator can watch the ladder.
