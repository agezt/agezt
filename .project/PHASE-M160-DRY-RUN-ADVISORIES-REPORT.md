# M160 — `agt run --dry-run` advisories: warn before you spend

## Why
M159's dry-run resolves *what* a run would do (model, system, timeout, tools) but
reports capabilities as bare facts (`supports_tools: false`). The operator still
has to know that a `tool_call=false` model with tools enabled will have its tool
calls ignored — and, under the M25 strict gate (`AGEZT_MODEL_STRICT=on`), be
*rejected* before any provider call. Likewise an effective model that isn't in the
catalog has unverified capabilities, and a small context window risks overflow on
a long run. These are exactly the failures a dry-run should catch — preemptively,
before a token is spent.

## What
`buildRunPlan` now derives a `warnings []string` from the resolved primitives and
attaches it to the plan (omitted when empty). The thresholds and wording mirror
`catalog.Model.AgentWarnings`, with one important refinement: the tool-use warning
fires **only when the run actually has tools enabled** — with `--no-tools` the
model never needs to call anything, so warning about it would be noise.

### kernel/controlplane/dryrun.go
- New `ContextLimit int` field on `runPlanInput` (model's context window, 0 = unknown).
- New `smallContextThreshold = 8192` const (documents the shared threshold).
- Warning derivation, before assembling the plan map:
  - **unknown model** (`!ModelKnown`) → "model %q is not in the catalog — its
    capabilities … are unverified; a run may fail in ways a dry-run can't predict".
  - **tool-use mismatch** (`ModelKnown && !SupportsTools && len(effectiveTools) > 0`)
    → "model %q does not advertise tool-use (tool_call=false), but N tool(s) are
    enabled — calls may be ignored; under AGEZT_MODEL_STRICT=on this run would be
    rejected before any provider call (see `agt provider check --caps`)".
  - **small context** (`ModelKnown && 0 < ContextLimit < 8192`) → "model %q has a
    small context window (%d tokens) — long runs with memory/tools may overflow it".
- `plan["warnings"]` set only when non-empty.

### kernel/controlplane/server.go (`handleRun`)
- The dry-run branch now also reads `in.ContextLimit = m.Limit.Context` for a
  catalog-known model.

### cmd/agt/main.go (`runDryRunMode`)
- Human output gains a `warnings:` section (`  ! <warning>` per line) when present.
  `--json` already carries the `warnings` array via the raw plan.

## Tests (+6, all passing)
`TestBuildRunPlan_Warnings` (5 subtests):
- unknown model warns;
- `tool_call=false` with tools enabled warns AND mentions `AGEZT_MODEL_STRICT`;
- `tool_call=false` with `--no-tools` does **not** warn (the key refinement);
- small context (4096) warns;
- healthy known model (tool_call=true, 200k ctx) → `warnings` key omitted entirely.

## Live proof (offline mock + hand-written custom catalog, one daemon)
A `custom.json` defined `notool-model` (tool_call=false, 200k), `tiny-model`
(tool_call=true, 4096), `good-model` (tool_call=true, 200k, vision):
- `--dry-run "x"` (default mock, not in catalog) → "model \"mock\" is not in the
  catalog …".
- `--dry-run --model notool-model "x"` → "does not advertise tool-use … but 7
  tool(s) are enabled … under AGEZT_MODEL_STRICT=on this run would be rejected …".
- `--dry-run --model notool-model --no-tools "x"` → `tools: none (--no-tools)`, **no**
  tool-use warning.
- `--dry-run --model tiny-model "x"` → "small context window (4096 tokens) …".
- `--dry-run --model good-model "x"` → zero warnings sections.

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command or env var.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./... -count=1` — **FAIL 0**, **1511 tests** (was 1505; +6), 61 packages.

## Result
A dry-run no longer just describes the run — it flags the ways that run would fail
or be rejected (unknown model, tool-use mismatch under strict mode, context
overflow), so an operator fixes the model/flags before committing budget.
