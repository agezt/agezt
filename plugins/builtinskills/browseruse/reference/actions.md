# browser-use patterns

All examples run `scripts/browse.mjs` via `code_exec` (language: node) with a
single JSON spec argument. The default `"isolated"` profile is stateless — pass
the full action list each call. First-party `browser.action` can also use
`"profile":"session"` plus `session_id` for AGEZT-managed cookie/session
carryover. Add `tab_id` when you want AGEZT to remember the final URL and let
later calls continue without repeating `url`. The first-party wrappers also save
snapshot refs so a later call can pass `ref` instead of repeating a selector.

## Read a JavaScript-rendered page (SPA)

When `browser.read` returns an almost-empty shell, render it for real:

```json
{ "url": "https://example-spa.com/dashboard",
  "actions": [{ "type": "wait", "selector": "main [data-loaded]" }],
  "extract": "text" }
```

## Search and read results

```json
{ "url": "https://duckduckgo.com/",
  "actions": [
    { "type": "fill", "selector": "input[name=q]", "value": "agezt agentic os" },
    { "type": "press", "selector": "input[name=q]", "key": "Enter" },
    { "type": "wait", "selector": "[data-testid=result]" }
  ],
  "extract": "[data-testid=result]" }
```

## Log in, then act

```json
{ "url": "https://app.example.com/login",
  "actions": [
    { "type": "fill", "selector": "#email", "value": "me@example.com" },
    { "type": "fill", "selector": "#password", "value": "..." },
    { "type": "click", "selector": "text=Sign in" },
    { "type": "wait", "selector": "text=Welcome" },
    { "type": "goto", "url": "https://app.example.com/reports" }
  ],
  "screenshot": true, "extract": "text" }
```

Pull secrets from the config/vault rather than hard-coding them; never echo a
password in a message.

For multi-call authenticated flows through the first-party tool, use
`"profile":"session"` and the same `session_id` on each call, then close it with
`browser.close` when done. Add `"tab_id":"main"` on the first call to save the
final URL and snapshot refs, then later call `browser.click/type/wait/downloads`
with the same `session_id` and `tab_id`, no `url`, and `ref` from the latest
snapshot. The tab ref is persistent URL/ref state, not a live browser tab.

```json
{ "url": "https://app.example.com/login",
  "profile": "session",
  "session_id": "work",
  "tab_id": "main",
  "actions": [
    { "type": "fill", "selector": "#email", "value": "me@example.com" },
    { "type": "click", "selector": "text=Continue" },
    { "type": "wait", "selector": "main" }
  ],
  "snapshot": true }
```

List saved tab refs with `browser.tabs`:

```json
{ "session_id": "work" }
```

Close a single saved tab ref with `browser.close`:

```json
{ "session_id": "work", "tab_id": "main" }
```

Follow-up first-party wrapper call:

```json
{ "profile": "session",
  "session_id": "work",
  "tab_id": "main",
  "ref": "e3",
  "snapshot": true }
```

## See before you act

Use both the compact `snapshot` and the PNG. `snapshot` gives clickable refs with
selectors for visible interactive elements; the PNG shows layout and visual
state. The first-party `browser.action` tool copies screenshots and downloads
into the Files view; manual `code_exec` use returns local paths that you can read
or register yourself.

```json
{ "url": "https://app.example.com",
  "snapshot": true,
  "snapshot_limit": 40,
  "screenshot": true,
  "extract": "text" }
```

## Menus, checkboxes, selects, and scrolling

```json
{ "url": "https://app.example.com/settings",
  "actions": [
    { "type": "hover", "selector": "text=Account" },
    { "type": "click", "selector": "text=Preferences" },
    { "type": "select", "selector": "select[name=timezone]", "value": "Europe/Istanbul" },
    { "type": "check", "selector": "input[name=weekly_summary]" },
    { "type": "scroll", "y": 900 }
  ],
  "snapshot": true,
  "events": true,
  "extract": "text" }
```

## Extracting structure

- A table: `"extract": "table tr"` returns each row's text.
- A list of links/titles: `"extract": "a.result-title"`.
- The whole DOM for parsing: `"extract": "html"`, then parse with code.

## Downloads and event diagnostics

Downloads are accepted by default. If a click triggers a file download, the
driver returns `downloads` with saved paths. It also returns bounded browser
events by default: `console`, `page_errors`, `network`, and `request_failed`.
Disable them with `"downloads": false` or `"events": false` when you only need
plain extracted text.

## Cookie inspection

Cookies are sensitive and are not returned by default. Use first-party
`browser.cookies` or set `"cookies": true` only when you need to inspect browser
session state.

```json
{ "profile": "session",
  "session_id": "work",
  "tab_id": "main",
  "cookies": true }
```

## When a step fails

The driver returns `{ "ok": false, "error": "..." }` (e.g. a selector timed
out). Re-screenshot to see the actual page, adjust the selector (try a `text=`
selector), add a `wait`, and retry. You have full machine permission — if
Chromium reports a missing OS library, install it with the `shell` tool and
re-run `scripts/setup.sh`.
