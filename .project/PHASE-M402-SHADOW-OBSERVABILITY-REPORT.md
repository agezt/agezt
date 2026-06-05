# M402 — Shadow-ladder observability: surface shadow evidence (SPEC-05 §5.2 / SPEC-07)

## Context
M399–M401 made the skill trust ladder self-driving (auto-shadow → shadow-eval →
auto-promote, alongside auto-quarantine). But the evidence that drives it —
`shadow_evals` / `shadow_wins` — was invisible: `agt skill` and the web Skills
panel showed only the status chip, so an operator couldn't see a shadow skill's
progress toward auto-promotion. This is the final slice of the arc: make the
ladder watchable.

## What
- **`kernel/controlplane/skill.go`** — `skillView`'s metrics map now includes
  `shadow_evals` / `shadow_wins` (the wire already carried uses/successes/failures).
- **`cmd/agt/skill.go`** — `renderSkillLine` appends `· shadow <wins>/<evals>` for
  a shadow skill that has been evaluated (via a new `shadowProgress` helper that
  reads the metrics map; nothing shown before the first eval, or for non-shadow).
- **`kernel/webui/dashboard.html`** — the Skills panel renders `shadow <wins>/<evals>`
  next to a shadow skill's name (textContent only — XSS-safe).

## Verification
- **`cmd/agt/skill_test.go`** `TestRenderSkillLine_ShadowProgress`: a shadow skill
  with 2 wins / 3 evals → `shadow 2/3`; an active skill and a not-yet-evaluated
  shadow skill show no progress.
- **`kernel/webui/webui_test.go`** `TestDashboard_RendersShadowProgress`: the
  embedded HTML reads `shadow_evals` / `shadow_wins` and renders `shadow "`;
  `TestDashboard_NoUnsafeDOMSinks` still green.
- **Negative control:** removing the shadow-progress append in `renderSkillLine`
  → the CLI test FAILs (no `shadow 2/3`); restored byte-identical.
- **Live demo** (echo mock, auto-shadow + shadow-eval on): imported
  `diagnose-ci.md` → shadow; two matching runs accrued evals; `agt skill list`
  shows `[shadow] diagnose-failing-ci … · shadow 0/2`, and the web Skills panel
  (Playwright) shows `diagnose-failing-ci shadow 0/2` (0 wins because the echo
  mock judges NO).
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2228** passing (was 2226; +2). No CHANGELOG beyond the surface (the env flags
  and behaviour were documented in M399–M401; this just renders existing data).

## Scope notes
- **The SPEC-05 auto-shadow-test arc is COMPLETE**: draft →[M399]→ shadow
  →[M400 eval]→ wins →[M401 promote]→ active →[M387]→ quarantined, with the
  evidence now visible in both `agt skill` and the web Skills panel. The whole
  skill lifecycle is self-driving and observable.
