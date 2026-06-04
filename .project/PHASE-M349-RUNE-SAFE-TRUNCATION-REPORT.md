# M349 — Rune-safe display truncation

## Why
Priority-A (output correctness) + a genuine, user-relevant bug. Two user-facing
string truncations sliced on a **byte** boundary (`s[:n]`), which splits a
multi-byte UTF-8 rune when the cut lands mid-character, emitting invalid UTF-8:

- `kernel/cadence.short` — shortens a schedule intent for `agt schedule` /
  `describe` output and the cadence log lines.
- `plugins/tools/coding.truncate` — bounds the coding tool's diff / agent output.

This matters concretely here: the operator works in **Turkish**, whose characters
(ç ş ğ ı ö ü, all 2-byte UTF-8) appear in schedule intents and diffs — so a cut at
byte 48 routinely lands mid-rune. The journal's own answer truncation
(`truncateForJournal`) and the tool-log preview (`previewString`) were already
rune-safe; these two display helpers were the outliers.

## What
Production fix + lock-in tests.
- **`cadence.short`** now truncates to 48 *runes* (via `[]rune`) instead of 48
  bytes — identical for ASCII, correct for multi-byte. Comment notes the Turkish
  motivation.
- **`coding.truncate`** keeps its byte bound (`MaxDiffBytes`) but backs the cut up
  to a UTF-8 rune start (`utf8.RuneStart`) so a straddling rune is dropped whole
  rather than split.
- Tests (white-box):
  - `TestShort_RuneSafeOnMultiByteIntent` — 60×`ş` → output is valid UTF-8, exactly
    48 kept runes + the ellipsis, no split rune.
  - `TestTruncate_RuneSafeAtByteBoundary` — padding placed so byte `max` is the
    continuation byte of a `ş`; asserts the result is valid UTF-8, drops the
    straddling rune whole, and contains no `�` replacement char.

## Verification
- New tests pass; existing `TestDescribe` and the coding-tool tests still pass
  (ASCII intents/diffs under the cap are returned unchanged — no behaviour change
  for the common case).
- `gofmt -l` clean; `go vet` clean (the new code); `GOOS=linux go build ./...`
  exit 0. Full suite **2077** passing (was 2075; +2), `go test ./...` exit 0.
  `go.mod` / `go.sum` unchanged. CHANGELOG updated (user-facing fix).

## Scope notes
- Behaviour change is limited to multi-byte strings longer than the cap; ASCII and
  short strings are byte-identical to before.
- Completes the codebase's UTF-8 truncation discipline: journal answer, tool-log
  preview, schedule intent, and coding diff are now all rune-safe.
