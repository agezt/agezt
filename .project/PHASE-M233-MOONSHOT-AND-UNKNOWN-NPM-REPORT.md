# M233 — Moonshot AI + actionable error for unknown provider packages

## Why
M232 fixed DeepSeek but revealed a *class* of bug: any provider whose npm package
isn't enumerated in `catalog.FamilyFromNPM` classifies as `FamilyUnknown` and is
refused by `compat.Build` — even when it speaks the OpenAI API. Two follow-ups:

1. **Moonshot AI (Kimi)** is the next popular vendor with the same problem. It
   ships an official `@ai-sdk/moonshotai` package (base URL
   `https://api.moonshot.ai/v1`, `MOONSHOT_API_KEY`) and is OpenAI-compatible, so
   models.dev classifies it under that package — which agezt didn't enumerate, so
   it would be refused exactly as DeepSeek was.

2. The **error message** for the unknown-family case claimed it was "unreachable
   for any models.dev catalog entry" — a claim DeepSeek and Moonshot both
   disprove. A misleading error on a reachable path is itself a defect: it tells
   the operator the situation is impossible instead of how to fix it.

## What
- **`kernel/catalog/types.go`** — `"moonshotai"` added to the
  `FamilyOpenAICompatible` case in `FamilyFromNPM`.
- **`plugins/providers/compat/compat.go`**:
  - `compatVendorBaseURL` gains `moonshotai → https://api.moonshot.ai/v1`.
  - The unknown-family error in `Build`'s default case is rewritten: it no longer
    claims the branch is unreachable, and instead points the operator at the
    escape hatch — *"If it speaks the OpenAI API (most do), set its npm to
    `openai-compatible` in custom.json."* That makes any future unenumerated
    OpenAI-compatible vendor a one-line operator fix rather than a dead end.

## Files
- `kernel/catalog/types.go` — classify `moonshotai` (edited).
- `plugins/providers/compat/compat.go` — moonshot base URL + reworded error (edited).
- `kernel/catalog/catalog_test.go` — `@ai-sdk/moonshotai` added to `TestFamilyFromNPM` (edited).
- `plugins/providers/compat/compat_m233_test.go` — 2 tests (new):
  `TestMoonshot_NowFirstClass` (family + URL + Build succeeds) and
  `TestBuild_UnknownNPMGivesActionableError` (an unknown npm still fails, but the
  message names `openai-compatible` + `custom.json` and no longer says
  "unreachable").

## Verification
- `go test ./kernel/catalog/ ./plugins/providers/compat/` — green; full suite
  **1761 → 1763** (+2), 66 packages.
- `gofmt -l` (CRLF-normalised) clean; `go vet` clean on both packages.
- `GOOS=linux go build ./...` clean; `go.mod` / `go.sum` unchanged.
- Moonshot base URL verified against the official `@ai-sdk/moonshotai` docs
  (`https://api.moonshot.ai/v1`); an explicit catalog `api` still overrides it.

## Scope notes
- Adding `moonshotai` is safe whichever package models.dev uses: it only changes
  the classification of the `@ai-sdk/moonshotai` package (previously
  `FamilyUnknown`); a Moonshot entry carried under the generic
  `openai-compatible` npm already worked and is unaffected.
- The reworded error generalises the fix: vendors agezt hasn't enumerated yet
  are now self-serviceable via `custom.json` without a code change.
