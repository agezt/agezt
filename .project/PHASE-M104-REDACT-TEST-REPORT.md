# Phase Report — Milestone M104 (`agt redact test`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-06 (secret hygiene).

## Why

Secret redaction (M15) scrubs secrets from every event before it enters the
hash-chained journal — but an operator had no way to confirm it catches THEIR
secret shape. A vault key in an unusual format, or a rotated token, might slip
through a gap and land in the permanent record. `agt redact test <string>`
answers "will this actually be protected?" against the LIVE redactor.

## What shipped

- **`agt redact test <string> [--json]`** — exercises the live redactor and
  reports: would it be scrubbed, the redacted form (safe to print), which
  built-in pattern categories matched, and whether a configured literal hit.
  Exit 0 = would redact, 3 = would NOT (scriptable for CI secret-hygiene), 2 =
  usage.
- **`CmdRedactTest`** server handler — uses `Bus().Redactor()` (new getter) so
  the check reflects the running config (built-in patterns + configured
  literals). It never echoes the raw input — only the redacted form — so the
  response is safe even when the input is a real secret.
- **`redact.MatchedCategories(s)`** — named the previously-anonymous built-in
  patterns and derived the redaction list from the labelled list, so the rules
  and their labels can never drift. Pure, daemon-free, testable.

## Design notes

- **Never leak which literal matched.** A configured-literal hit is reported as
  a boolean (`literal_hit`), never the secret name, so the diagnostic itself
  can't become an oracle.
- Patterns and labels share one source (`namedPatterns`); `patterns` is derived,
  pinned by a test so a future pattern addition stays labelled.

## Tests

- `TestMatchedCategories` — each built-in category matches its shape; prose and
  empty input match nothing.
- `TestPatternsDerivedFromNamed` — anything `Redact` scrubs has a label.
- `TestRedactTest` (control-plane) — disabled when no redactor; pattern hit
  flagged + category returned + scrubbed form; configured literal → `literal_hit`;
  prose → not redacted.

Test count: **1357 → 1360**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt redact test "my anthropic key sk-ant-api03-abc…0123"
redacted ✓ — this string would be scrubbed before journaling.
  result: my anthropic key [REDACTED]
  matched pattern(s): openai/anthropic-key            (exit 0)
$ agt redact test "deploy the staging server at noon"
NOT redacted — this string would pass through to the journal unchanged.  (exit 3)
```
