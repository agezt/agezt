# Phase Report — Milestone M79 (error-message breakdown in `agt tool stats`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

`agt tool stats` reported how MANY calls errored (total + per-tool), but never
WHAT the errors were. An operator seeing "errored: 12" couldn't tell whether it's
one tool timing out twelve times or twelve distinct failures. M79 adds an
errors-by-message breakdown — the tool analogue of `runs stats`' `failed_by_reason`
— so the actual failure modes (denied / not-available / timeout / a tool's own
error string) are visible at a glance.

## What shipped

- **Server `errors_by_message`** — `handleToolStats` buckets each failed call's
  output message (already whitespace-collapsed + capped by `decodeToolResult`),
  empty messages under `(no message)`. Returned as `{message → count}`.
- **CLI breakdown** — an `errors by message:` block, most-frequent first (stable
  alphabetical tiebreak), shown only when there are failures.

## Design decisions

- **Reuse the capped output as the bucket key.** The same 100-rune preview the
  log shows, so the breakdown keys read identically to a `tool log --errors` row
  — no second truncation policy to reason about.
- **Most-frequent first.** Sorted client-side (the JSON map is unordered) so the
  dominant failure mode leads, the way `runs stats` surfaces the top fail reason.
- **Only on failure.** Successful calls never enter the bucket, and the block is
  omitted entirely when nothing failed — no empty "errors by message:" noise.

## Tests

- `TestToolStats_ErrorsByMessage` — two `boom` failures + one `denied by policy`
  + one success → `boom: 2`, `denied by policy: 1`, and the success is not bucketed.

Test count: **1321 → 1322**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_EDICT_DENY=shell:dir agt run "summarize"   # forces a denied shell call
$ agt tool stats
  errored   : 1
  error     : 100.0%
  by tool:
    shell            1 call(s), 1 error(s)
  errors by message:
     1  tool call denied by policy: hard-deny rule matched: operator[1]
```
