# M399 — Skill auto-shadow: draft→shadow auto-staging (SPEC-05 §5.2)

## Context
SPEC-05 §5.2's trust ladder is `draft —(shadow-test passes)→ shadow —(N
successful real uses, gated)→ active`, with a regression path back to
quarantined. M387 already automated the demotion side (active→quarantined on
repeated failure). This milestone is the **first rung of the promotion side**:
auto-staging a well-formed draft to shadow. It's the foundation of the larger
auto-shadow-test feature; the richer shadow→active gate (run the skill alongside
real execution, compare its proposed actions, promote only if it would have
helped) needs a shadow-execution harness and is the next milestone(s).

## What
- **`kernel/skill/shadowtest.go`** — pure `ShadowTest(sk) (ok, reason)`: the
  deterministic gate on draft→shadow. v1 is a *structural* test — the draft must
  be well-formed and **retrievable** — not the execution-comparison that gates
  shadow→active. A draft fails when it could never function: empty/short body
  (`< ShadowTestMinBodyChars = 16`), or no description AND no triggers (retrieval
  scores on description+triggers, so it could never be surfaced).
- **`kernel/skill/forge.go`** — `Forge.autoShadow` + `SetAutoShadow(on)`;
  `maybeAutoShadow(corr, sk)` advances a fresh draft that passes `ShadowTest` to
  shadow. `Create` calls it after persisting the draft and re-reads the record so
  the returned skill reflects the staged status. `Promote` refactored to
  `promoteWithReason` so the auto path records the gate reason on `skill.promoted`
  (a manual promote's payload is unchanged — empty reason omits the field).
- **`cmd/agezt/main.go`** — `AGEZT_SKILL_AUTOSHADOW=on` (off by default) +
  banner line + config inventory entry.
- **`cmd/agt/skill_md.go` / `skill_import.go`** — the import success line now
  reports the status the skill actually landed in (`installed as a new shadow`
  when auto-staged), instead of a hard-coded "draft" that contradicted the
  `status:` line.

## Verification
- **`kernel/skill/shadowtest.go`** `TestShadowTest` (6 cases): well-formed /
  triggers-only / description-only pass; empty-body / short-body /
  no-retrieval-surface fail with a reason.
- **`kernel/skill/autoshadow_test.go`**: `TestAutoShadow_StagesWellFormedDraft`
  (Create with auto-shadow on → status shadow, `skill.promoted` carries
  from=draft/to=shadow/an auto reason/the creating run's correlation);
  `TestAutoShadow_RejectsDraftFailingShadowTest` (no desc/triggers → stays draft);
  `TestAutoShadow_DisabledLeavesDraft` (off by default → draft).
- **Negative control:** removing the `promoteWithReason` call in
  `maybeAutoShadow` → the staging test FAILs (status draft); restored
  byte-identical. Full `kernel/skill` package still green (existing
  auto-quarantine / transition-matrix / retrieval-pool tests unaffected — Create
  stays draft when auto-shadow is off, which is the default).
- **Live demo** (mock provider, `AGEZT_SKILL_AUTOSHADOW=on`): `agt skill import
  diagnose-ci.md` → "installed as a new shadow", `agt skill list` shows
  `[shadow] diagnose-failing-ci`, journal `skill.promoted` carries
  `"reason":"auto-shadow: shadow-test passed"`.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged.
  `TestConfigEnvVars_CoversCmdAgeztReads` green. Full suite **2218** passing (was
  2208; +10). CHANGELOG (Added, user-visible).

## Scope notes
- Trust-ladder automation status: demotion active→quarantined (M387, on by
  default); promotion draft→shadow (M399, opt-in). **Remaining for the full
  auto-shadow-test:** the shadow→active gate — a shadow-execution harness that
  runs a shadow skill alongside a real run, scores whether it would have helped,
  and auto-promotes after N gated successes (plus its observability). That is the
  next milestone arc.
