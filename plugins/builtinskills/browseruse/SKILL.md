---
name: browser-use
description: Drive a real headless browser — navigate, click, type, submit forms, screenshot, and extract from JavaScript-rendered pages (when browser.read's plain fetch isn't enough)
triggers: [browser, web, navigate, click, screenshot, scrape, form, login, spa, javascript]
tools: [code_exec, shell, artifacts]
---

# Browser use — full headless automation

When a page needs JavaScript, a click, a form submission, or a screenshot, the
plain `browser.read` fetch isn't enough. This skill drives a **real headless
Chromium** via Playwright, which you run through the `code_exec` tool. It is
headless-first: you act, take a screenshot, look at it, and act again.

## One-time setup (install Playwright)

Run the setup script once via `code_exec` (or the `shell` tool). It installs
Playwright and its Chromium browser into the sandbox:

```
node scripts/setup.sh        # or: bash scripts/setup.sh
```

(Use `skill op=files browser-use` to see the bundle directory, then run
`scripts/setup.sh` from there. First run downloads Chromium — it can take a
minute. If `npx playwright install` reports a missing OS dependency, install it
with the `shell` tool, then re-run — you have full machine permission.)

## Driving the browser

`scripts/browse.mjs` is a stateless driver: each call opens a page, runs your
ordered list of **actions**, optionally screenshots, and returns extracted text +
the screenshot path as JSON. Pass the spec as a single JSON argument.

Run it through `code_exec` (language: node), e.g.:

```
node scripts/browse.mjs '{"url":"https://example.com","actions":[{"type":"click","selector":"text=More information"}],"screenshot":true,"extract":"text"}'
```

### Spec fields

- `url` (required) — the page to open.
- `actions` (optional) — ordered steps, each `{type, ...}`:
  - `{"type":"click","selector":"<css-or-text>"}`
  - `{"type":"fill","selector":"<css>","value":"<text>"}` — type into an input.
  - `{"type":"press","selector":"<css>","key":"Enter"}`
  - `{"type":"wait","ms":1000}` or `{"type":"wait","selector":"<css>"}`
  - `{"type":"goto","url":"<url>"}` — navigate again (e.g. after a click).
- `screenshot` (optional, default true) — save a PNG to the artifacts dir.
- `extract` (optional) — `"text"` (visible text, default), `"html"` (full HTML),
  or a CSS selector (the matched elements' text).
- `timeout_ms` (optional, default 30000).

Selectors accept Playwright syntax: CSS (`#id`, `.class`, `input[name=q]`) or
text (`text=Sign in`).

### Output (JSON on stdout)

```
{ "ok": true, "url": "<final-url>", "title": "...", "text": "<extracted>",
  "screenshot": "<absolute path to the PNG>" }
```

The screenshot is written under the sandbox; copy or register it with the
`artifacts` tool if you want it to appear in the Files view. To **see** the page
state before deciding your next action, read the screenshot back (it's a real
PNG you can attach).

## Loop

1. `goto` the URL, screenshot, read the screenshot + text.
2. Decide the next action (click/fill/press) from what you see.
3. Call again with the next actions. Repeat until done.

See `reference/actions.md` for richer patterns (login flows, waiting for SPA
content, extracting tables, downloading files).
