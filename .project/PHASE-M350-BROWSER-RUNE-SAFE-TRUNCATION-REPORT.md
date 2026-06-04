# M350 — Browser tool: rune-safe extracted-text truncation

## Why
Direct follow-up to M349, and the highest-impact instance of the same bug. The
browser tool fetches a page, extracts text with `HTMLToText`, and caps it at
`MaxChars` (default 64 KiB) before returning it to the model. That cap was a raw
byte slice (`text = text[:maxChars]`), which splits a multi-byte UTF-8 rune when
the cut lands mid-character — emitting invalid UTF-8.

This is worse than the M349 cases because the input is **arbitrary web content**:
any non-English page (and most pages have smart quotes, emoji, accented letters,
non-Latin scripts) is full of multi-byte runes, so a 64 KiB cut very frequently
lands mid-rune. The corrupted text then flows straight to the LLM and into the
journaled tool result / web UI. M349 fixed `cadence.short` and `coding.truncate`
but did not catch this one; M350 closes it.

## What
Production fix + lock-in test.
- **`plugins/tools/browser/browser.go`** — the `MaxChars` truncation now backs the
  byte cut up to a UTF-8 rune start (`utf8.RuneStart`), dropping a straddling rune
  whole instead of splitting it. The byte bound (memory safety) is preserved.
- **`browser_test.go`** — `TestInvoke_TruncatesOnRuneBoundary`: serves a page whose
  extracted text is 1000×`ş` (all 2-byte runes) with `MaxChars = 51` (odd → the
  cut necessarily lands inside a rune). Asserts `truncated_text=true`, the returned
  text is valid UTF-8 (`utf8.ValidString`), and contains no `�` replacement char.

## Verification
- `go test ./plugins/tools/browser -run 'TruncatesOnRuneBoundary|TruncatesLongContent' -v`
  — both pass (the existing ASCII truncation test is unaffected).
- `gofmt -l` clean; `go vet ./plugins/tools/browser/` clean; `GOOS=linux go build
  ./...` exit 0. Full suite **2078** passing (was 2077; +1), `go test ./...` exit
  0. `go.mod` / `go.sum` unchanged. CHANGELOG updated (extends the M349 entry).

## Scope notes
- Behaviour change limited to truncated multi-byte text; ASCII / under-cap pages
  are byte-identical to before.
- With M349 + M350, every string truncation that reaches a user or the model is
  now rune-safe: journal answer, tool-log preview, schedule intent, coding diff,
  and browser page text. The `rawBody[:MaxFetchBytes]` cut on the *raw HTML* is
  intentionally left byte-based — it feeds the HTML parser, which tolerates a
  truncated trailing byte, and never reaches the model as text.
