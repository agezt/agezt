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

`scripts/browse.mjs` opens a page, runs your ordered list of **actions**,
optionally screenshots, and returns extracted text, a compact interactive
snapshot, browser event summaries, downloads, and the screenshot path as JSON.
The default `"isolated"` profile is stateless; `"session"` carries cookies and
storage through an AGEZT-managed persistent context directory. Pass the spec as
a single JSON argument.

Run it through `code_exec` (language: node), e.g.:

```
node scripts/browse.mjs '{"url":"https://example.com","actions":[{"type":"click","selector":"text=More information"}],"screenshot":true,"extract":"text"}'
```

### Spec fields

- `url` (required) — the page to open.
- `actions` (optional) — ordered steps, each `{type, ...}`:
  - `{"type":"click","selector":"<css-or-text>"}`
  - `{"type":"fill","selector":"<css>","value":"<text>"}` — type into an input.
  - `{"type":"type","selector":"<css>","value":"<text>"}` — keystroke-style typing.
  - `{"type":"press","selector":"<css>","key":"Enter"}`
  - `{"type":"select","selector":"<css>","value":"<option-value>"}`
  - `{"type":"check","selector":"<css>"}` / `{"type":"uncheck","selector":"<css>"}`
  - `{"type":"hover","selector":"<css>"}`
  - `{"type":"scroll","y":800}` or `{"type":"scroll","selector":"<css>","y":800}`
  - `{"type":"wait","ms":1000}` or `{"type":"wait","selector":"<css>"}`
  - `{"type":"goto","url":"<url>"}` — navigate again (e.g. after a click).
- `screenshot` (optional, default true) — save a PNG to the artifacts dir.
- `full_page` (optional, default false) — full-page screenshot instead of viewport.
- `snapshot` (optional, default true) — return compact `{ref, role, name, selector}`
  entries for visible interactive elements.
- `snapshot_limit` (optional, default 60, max 200).
- `events` (optional, default true) — return console/page/network/request-failed summaries.
- `event_limit` (optional, default 50, max 200 per bucket).
- `downloads` (optional, default true) — accept downloads and return saved paths.
- `cookies` (optional, default false) — return final-page browser cookies,
  including values. Use `browser.cookies` when this is the only thing needed.
- `profile` (optional, default `"isolated"`) — `"isolated"`, `"session"`,
  `"user-attached"`, or `"remote-cdp"`. The first-party `browser.action` tool
  supports `"session"` with `session_id` for AGEZT-managed cookie/session
  carryover. `user-attached` and `remote-cdp` only work when the operator enabled
  the matching environment variables; `user_data_dir` and `cdp_url` are injected
  by tool config, not accepted from model input.
- `session_id` (optional) — required for first-party `profile=session` calls;
  close it with `browser.close`.
- `tab_id` (optional) — with first-party `profile=session`, stores the final URL
  under the AGEZT-managed session so later calls can omit `url` and reuse the
  saved tab ref. It also stores the latest snapshot refs for wrapper calls that
  pass `ref` instead of `selector`. This is persistent URL/ref state, not a live
  browser tab.
- `ref` (first-party wrappers only) — snapshot ref such as `e1`; resolves to the
  saved selector for the same `session_id` + `tab_id`. Missing refs ask for a
  fresh `browser.snapshot`.
- `browser.tabs` lists saved tab refs for a `session_id`.
- `browser.close` closes the whole managed session, or just one saved tab ref
  when `tab_id` is supplied.
- `viewport` (optional) — `{ "width": 1280, "height": 720 }`.
- `extract` (optional) — `"text"` (visible text, default), `"html"` (full HTML),
  or a CSS selector (the matched elements' text).
- `timeout_ms` (optional, default 30000).

Selectors accept Playwright syntax: CSS (`#id`, `.class`, `input[name=q]`) or
text (`text=Sign in`).

### Output (JSON on stdout)

```
{ "ok": true, "url": "<final-url>", "title": "...", "text": "<extracted>",
  "snapshot": [{ "ref": "e1", "role": "button", "name": "Sign in",
    "selector": "#signin" }],
  "screenshot": "<absolute path to the PNG>",
  "downloads": [{ "suggested_filename": "report.csv", "path": "..." }],
  "events": { "console": [], "page_errors": [], "network": [],
    "request_failed": [] } }
```

When this driver is used through the first-party `browser.action` tool, the
screenshot and downloads are also copied into the Files view as artifacts. When
you run the script manually through `code_exec`, it returns local paths; copy or
register those paths yourself if you want durable Files-view entries. To **see**
the page state before deciding your next action, read the screenshot back (it's
a real PNG you can attach).

## Loop

1. `goto` the URL, screenshot, read the screenshot + text.
2. Decide the next action (click/fill/press) from what you see.
3. Call again with the next actions. Repeat until done.

See `reference/actions.md` for richer patterns (login flows, waiting for SPA
content, extracting tables, downloading files).
