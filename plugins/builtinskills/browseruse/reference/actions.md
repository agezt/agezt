# browser-use patterns

All examples run `scripts/browse.mjs` via `code_exec` (language: node) with a
single JSON spec argument. The driver is stateless — pass the full action list
each call.

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

## See before you act

Always screenshot, then read the PNG back to decide the next step. The driver
returns `screenshot` (an absolute path). Register it with the `artifacts` tool to
make it appear in the Files view, or read it directly to look at the page.

## Extracting structure

- A table: `"extract": "table tr"` returns each row's text.
- A list of links/titles: `"extract": "a.result-title"`.
- The whole DOM for parsing: `"extract": "html"`, then parse with code.

## When a step fails

The driver returns `{ "ok": false, "error": "..." }` (e.g. a selector timed
out). Re-screenshot to see the actual page, adjust the selector (try a `text=`
selector), add a `wait`, and retry. You have full machine permission — if
Chromium reports a missing OS library, install it with the `shell` tool and
re-run `scripts/setup.sh`.
