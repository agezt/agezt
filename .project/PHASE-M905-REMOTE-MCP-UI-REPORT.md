# Phase M905 — Remote MCP server UI (#39)

## Goal
Make the M904 remote (Streamable HTTP) MCP transport reachable from the Web UI:
the operator should add a hosted MCP endpoint as easily as a local stdio server,
with the same one-click catalog, and see which transport each server uses.

## What shipped — `frontend/src/views/Mcp.tsx`

### Register form — stdio/remote toggle
- A two-tab transport selector ("Local (stdio)" / "Remote (HTTP)") drives which
  fields show:
  - **stdio:** Command + Arguments + Environment (unchanged).
  - **remote:** URL + Headers (`Name: value` per line, e.g.
    `Authorization: Bearer …`).
- Validation: `serverNameOk(name)` plus a valid `command` (stdio) **or** a valid
  http(s) URL with a host (remote, `urlOk`). The wire `server` object carries
  `url`+`headers` for remote, `command`+`args`+`env` for stdio — never both.
- `tool_allow` (M899) applies to both transports.

### Catalog — remote presets
- `CatalogEntry` now models either shape (`command`/`args` **or** `url`); a
  `transportOf` helper reports which. Three hosted presets added (GitHub's hosted
  MCP, DeepWiki, Context7) with prefilled `Authorization:` header lines where
  needed. Picking a remote preset flips the form to Remote mode and prefills URL +
  blank header lines. Cards badge `remote` and show the URL.

### Server list
- Each row badges `remote` for http servers, shows the URL (instead of the
  command line), and lists redacted `header_keys` (`headers: Authorization`) the
  same way env keys are shown. The attach confirmation explains the HTTP
  connection rather than a spawned process.

### Pure helpers (exported, unit-tested)
`parseHeaders` (`Name: value` → map, keeps later `:`, drops blanks/`#`), `urlOk`
(http(s)+host), `transportOf`.

## Tests — `frontend/src/views/Mcp.test.tsx`
- `parseHeaders`, `urlOk` unit tests.
- CATALOG test made transport-aware (each preset has exactly one shape; remote
  URLs valid) + asserts at least one remote preset exists.
- New `NewServerForm` remote test: toggling to Remote hides the command field,
  requires a valid URL, and posts the `{url, headers}` shape with **no** command.
- 16 tests pass.

## Gate
`tsc --noEmit` ✓ · `vitest run Mcp.test.tsx` (16) ✓ · `vite build` → embedded
`kernel/webui/dist` rebuilt ✓ · `go build ./...` + `kernel/webui` test green with
the new dist ✓ · dist committed LF (`.gitattributes eol=lf`). go.mod unchanged.

## Notes
Header values are secrets: the form warns they're stored and never shown again
(use a low-scope token); the backend already redacts them to `header_keys` on
every read. This completes the operator-facing half of #39's remote-MCP parity.
Still deferred under #39: the older HTTP+SSE two-stream transport (the `mcpbridge`
plugin already speaks it) and a fully-lazy on-demand MCP dispatcher.
