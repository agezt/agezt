# M397 — Extractive head-snippet in the context-compaction stub (SPEC-10 §3)

## Context
When context budgeting (M393–M396) elides an old tool output, it replaced the
whole output with `[tool output elided to fit context budget: N chars]` — a bare
byte count. The model lost every hint of *what* was dropped (the file that was
read, the command that ran), which is exactly the grounding a later turn might
need. The SPEC-10 §3 follow-up names "LLM-summarise elided spans"; an
abstractive/LLM summary is a larger feature (a summarisation call, model choice,
cost/latency). This milestone takes the deterministic, dependency-free
sub-slice: keep a short *extractive* preview of the head of the dropped output.

## What
- **`kernel/agent`** — `headSnippet(s, n)`: first `n` runes of `s` with internal
  whitespace runs collapsed to single spaces, ellipsis when truncated — always
  single-line. `elidedHeadSnippetChars = 80`. The compaction stub becomes
  `[tool output elided to fit context budget: N chars · head: "the first 80
  chars…"]`. `%q` keeps the preview escaped and single-line; the constant
  `elidedStubPrefix` is unchanged, so idempotency (don't re-elide a stub) and the
  `len(stub) >= orig` "eliding wouldn't help" guard both still hold.

## Verification
- **`kernel/agent/compact_internal_test.go`**
  `TestCompactMessages_StubKeepsHeadPreview`: a multi-line tool output is elided;
  the stub keeps the recognisable head (`FILE: deploy.yaml`), stays single-line
  (newlines collapsed), keeps the `elidedStubPrefix`, is smaller than the
  original, and is bounded near the snippet length. `TestHeadSnippet`:
  pass-through / whitespace-collapse / truncate-with-ellipsis. Existing
  compaction tests (oldest-first, protect-first, idempotent) still pass — the
  prefix-based re-elide guard is intact.
- **Negative control:** neutering `headSnippet` to return `""` → the preview
  assertions FAIL (`head: ""`); restored byte-identical.
- **Test-robustness fix (found via this change):** `kernel/runtime`'s
  `TestRun_AutoContextBudgetFromCatalog` / `TestRun_ContextProtectFirstPlumbsThrough`
  / `TestRun_AutoBudgetOffForUnknownModel` were eliding a fixed **~80-char policy
  *denial* message** (the `dump` tool was default-denied, so its real 2000-char
  output never reached the transcript). The old ~50-char stub happened to be
  smaller than that denial; the new head-snippet stub correctly won't shrink an
  80-char message. Fixed the tests to *allow* the dump tool (a small
  `edict.Options{Levels: {"dump": LevelAllow}}` engine) so the genuine large
  output flows and compaction is exercised robustly, independent of stub size.
  The M395 runtime negative control still bites under the fixed tests.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2206** passing (was 2204; +2). No CHANGELOG — the stub lives inside the
  loop's model context, not a user-facing CLI/env surface.

## Scope notes
- This is the extractive half of "summarise elided spans". The abstractive/LLM
  half (replace the head snippet with a model-written précis) remains a larger,
  deferred SPEC-10 §3 follow-up needing a summarisation call.
