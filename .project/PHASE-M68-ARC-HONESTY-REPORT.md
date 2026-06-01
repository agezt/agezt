# Phase Report — Milestone M68 (honest tool & policy rendering in the task arc)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability · **bug fix**.

## Why

`agt runs show <corr>` renders a run as a task arc (intent → rounds → tools →
answer). Two of its branches read journal payload fields that the agent loop
never writes, so the arc silently lied:

1. **`tool.result` read `payload["is_error"]`** — but the agent journals the flag
   as `"error"` (agent.go:503). Result: **every tool call rendered `ok`, even
   when it failed.** An operator reading the arc to find what broke saw nothing.
2. **`policy.decision` read `payload["decision"]`** (a string) — but the agent
   journals `{allow (bool), hard_denied, reason}` (agent.go:421). Result: **every
   policy line rendered a blank verdict** (`policy: shell `).

Both are silent-wrong-output bugs: no error, no crash, just a view that doesn't
match reality — the worst kind for an observability surface.

## What shipped (all client-side, in `renderTaskArc`)

- **tool.result honours `error`** — failed calls now render `ERROR`; successful
  ones `ok`. The actual bug fix.
- **policy.decision derives the verdict** from `{allow, hard_denied}` →
  `allow` / `DENY` / `DENY(hard)`, and appends the `reason`. The arc now agrees
  with `agt edict log`'s rendering of the same events.
- **Enrichment** — `tool.invoked` shows a compact input excerpt and `tool.result`
  a compact output excerpt, via a new `arcPreview` helper (strings pass through,
  raw-JSON inputs are re-marshaled; whitespace-collapsed, 80-rune cap). The arc
  now says WHAT ran and what it returned, not just that something did.

## Design decisions

- **Read what the producer writes.** Both fixes are grounded in the exact
  `publish(...)` payloads in agent.go — the single source of truth for field
  names. The new tests assert against those same field names so a future rename
  on either side fails loudly.
- **Mirror the existing previews.** `arcPreview` uses the same collapse +
  rune-cap approach as the server-side `previewString` (M66) and
  `extractAnswerPreview`, so every excerpt across the CLI reads alike.

## Tests

- `TestRenderTaskArc_ToolResultHonoursErrorField` — a tool.result with
  `error:true` renders `ERROR` (and explicitly asserts it is NOT `ok`), with the
  input + output excerpts.
- `TestRenderTaskArc_PolicyDecisionVerdict` — a hard denial renders
  `DENY(hard) net (egress blocked)`; an allow renders `allow shell`.

Test count: **1309 → 1311**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean (added lines).

## Live proof

```
$ agt runs show <corr>
  ...
  policy: allow      shell  (level L2; AskPolicy=AskAllow (would prompt in MVP))
  tool.invoked: shell  {"command":"dir"}
  tool.result : ok  Volume in drive D is New Volume … Directory of D…
```
(Before M68: the policy line was `policy: shell ` with a blank verdict, and
tool.result always said `ok` with no output.)
