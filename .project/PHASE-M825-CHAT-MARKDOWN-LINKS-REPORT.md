# Phase M825 — chat markdown: links + strikethrough

**Date:** 2026-06-11 · **Status:** DONE · **Trigger:** owner: "chat arayüzünde
markdown formatter lazım ayrıca".

## Why

Chat already renders the finished answer through the `<Markdown>` component
(lib/markdown's tiny AST), which covered fenced code, inline code, bold/italic,
lists, headings, GFM tables and blockquotes — but the inline parser did NOT
handle **links** (`[text](url)`) or strikethrough (`~~…~~`). LLM answers emit
links constantly, so they showed as literal `[text](url)`. Extending the shared
component fixes Chat AND the Agent Board (M820), both of which use it.

## What changed (`frontend/src/lib/markdown.ts` + `components/Markdown.tsx`)

- New inline tokens `link {v, href}` and `del {v}`; `INLINE_RE` extended (code →
  link → strong → del → em; code/link before emphasis so `*` in a URL/label
  doesn't split).
- **Link safety:** `safeHref()` admits only `http(s)://` and `mailto:` — a
  `[x](javascript:…)` link is never turned into an anchor; it falls through to
  literal text. No raw-HTML path; still CSP-safe. Links render as
  `<a target="_blank" rel="noopener noreferrer nofollow">`.
- Strikethrough renders as `<del>`.

## Tests

- markdown.test.ts: `[text](href)` → link token; `~~old~~` → del token;
  unsafe-scheme link stays literal text; `safeHref` admits http/https/mailto and
  rejects `javascript:`. Full vitest 506 green.

## Gate

vitest 506/… green; tsc clean; `kernel/webui/dist` rebuilt (LF). Frontend-only.

## Note

If the owner was seeing NO markdown at all in chat, that was a stale embedded
webui — the repo `agezt.exe` is rebuilt each milestone; restarting picks up the
current bundle.
