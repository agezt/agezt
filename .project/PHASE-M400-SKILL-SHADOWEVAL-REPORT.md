# M400 — Skill shadow evaluation: judge shadow skills against completed runs (SPEC-05 §5.2)

## Context
M399 staged drafts to shadow. SPEC-05 §5.2 then says a shadow skill "runs
alongside real execution **without affecting outcomes** ... compared to what
actually happened. Only promoted if it would have helped." Executing a shadow
skill with real tools would cause real side effects (violating "without affecting
outcomes") and need a dry-run sandbox, so the offline-safe primitive is an
**LLM-judge**: after a real run, ask whether each relevant shadow skill *would
have helped* — no tools run, nothing mutates. This milestone is the evaluation
half; the shadow→active auto-promotion gate that reads its evidence is M401.

## What
- **`kernel/skill/retrieve.go`** — refactored to `retrieveMatching(…, include)`;
  added `RetrieveShadow` (the shadow-status candidate set), `Retrieve` unchanged
  (active-only, M388 lock-in still holds).
- **`kernel/skill/skill.go`** — `Metrics.ShadowEvals` / `ShadowWins`, kept
  separate from production Successes/Failures.
- **`kernel/skill/forge.go`** — `ShadowEvaluate(ctx, corr, provider, model,
  intent, outcome, limit)`: retrieves the relevant shadow skills and, per
  candidate, asks the model a one-word YES/NO "would it have helped" (system
  prompt `shadowJudgeSystem`), parsed by `parseShadowVerdict` (conservative —
  anything ambiguous is NO). `RecordShadowOutcome(corr, id, helped)` bumps the
  shadow counters (shadow-status only) and journals `skill.shadow_evaluated`.
- **`kernel/event/kinds.go`** — `KindSkillShadowEval = "skill.shadow_evaluated"`.
- **`kernel/runtime/runtime.go`** — `Config.ShadowEval`; `maybeShadowEval` runs
  after a *successful* run (best-effort, bounded to `shadowEvalLimit = 2`
  candidates), mirroring the `maybeForge` post-run hook.
- **`cmd/agezt/main.go`** — `AGEZT_SKILL_SHADOWEVAL=on` (off by default) + banner
  + config inventory entry.

## Verification
- **`kernel/skill/shadoweval_test.go`**: `TestParseShadowVerdict` (YES/NO/ambiguous
  → conservative); `TestRecordShadowOutcome_BumpsCountersAndJournals` (evals/wins
  + 2 events); `TestRecordShadowOutcome_OnlyShadowSkills` (active skill never
  credited); `TestShadowEvaluate_JudgesRelevantShadowSkill` (capturing mock
  returns YES → relevant shadow skill gets a win + a `skill.shadow_evaluated`
  event under the run's correlation; an irrelevant shadow skill is untouched);
  `TestShadowEvaluate_NoProviderErrors`.
- **Negative control:** removing the `sk.Status != StatusShadow` guard in
  `RecordShadowOutcome` → the shadow-only test FAILs (active skill credited);
  restored byte-identical. Full `kernel/skill` package green.
- **Live demo** (mock provider, `AGEZT_SKILL_AUTOSHADOW=on AGEZT_SKILL_SHADOWEVAL=on`):
  imported `diagnose-ci.md` → shadow; ran "the ci build is failing, diagnose it"
  → the journal shows **3** `routing.decision` under the run's correlation, the
  third carrying `task_type:"shadow-eval"` — the post-run judge call firing
  end-to-end through the Governor. (The scripted demo mock is exhausted by the
  run's two turns, so the judge call returns `ErrExhausted` and the verdict isn't
  recorded live; the record+event path is covered by the capturing-fake
  integration test above — the goal-allowed mode for mock-limited paths.)
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged.
  `TestConfigEnvVars_CoversCmdAgeztReads` green. Full suite **2222** passing (was
  2218; +4). CHANGELOG (Added, user-visible).

## Scope notes
- Trust-ladder automation: demotion active→quarantined (M387); promotion
  draft→shadow (M399); **shadow evaluation (M400)**. **Next (M401): shadow→active
  auto-promotion** — read `shadow_evals`/`shadow_wins` and promote a shadow skill
  that crosses a gated win count + rate (mirrors `maybeAutoQuarantine`), closing
  the ladder loop. After that: observability (surface shadow evidence in
  `agt skill`/web).
