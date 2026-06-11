# PHASE M897 — Popular MCP servers catalog (one-click examples)

**Status:** shipped
**Milestone:** M897 (session range M889–M899; branched from `origin/main`).
**Theme:** Backlog **#39** (MCP parity) — the owner asked for a set of popular MCP
servers shipped as examples. This adds a curated, one-click catalog to the MCP
view so the operator can stand up a well-known server without hunting for the
right `command`/`args`.

## What shipped

A **Popular servers** gallery in `frontend/src/views/Mcp.tsx`:

- `CATALOG` — 14 curated presets (name, command, args, description, optional
  `needs`): everything, filesystem, fetch, memory, git, github, postgres, sqlite,
  puppeteer, brave, slack, gdrive, time, thinking. Names obey the kernel rule
  (≤16 lowercase alnum — they become the `mcp_<name>_<tool>` prefix).
- A **Popular servers** toggle button opens a card gallery; each card shows the
  description, the exact `command args`, and an amber **needs:** line flagging a
  path/secret to supply. **Use** prefills the existing register form (name /
  command / args / description) so the operator reviews/adjusts (e.g. sets the
  filesystem path) then registers via the unchanged `/api/mcp/add`. Cards for
  already-registered servers show an **added** badge and disable **Use**.
- `NewServerForm` gained an optional `initial` prop (seeds the fields); the form
  is `key`-remounted per preset so picking a different one re-seeds it.
- A note explains the scrubbed-environment caveat for `needs … (env)` servers.

## Why this milestone is conflict-free

Purely frontend. Touches **only** `frontend/src/views/Mcp.tsx`, its test, and the
rebuilt `kernel/webui/dist`. **No new route** — it reuses the existing
`/api/mcp/add`. No Go change; the concurrent session's kernel arc is untouched.

## Verification

- **Frontend gate:** `tsc --noEmit` clean; `vitest run src/views/Mcp` green (10/10)
  — two new tests assert every preset has a kernel-valid name + command + args +
  description, and that names are unique. `vite build` emits the committed-LF dist.
- No Go change → contested kernel packages not compiled; full `go build ./...`
  deliberately skipped.

## Notes
- This is the *catalog/examples* half of #39. The deeper backend parity (passing
  scoped secrets/env to credentialed servers, SSE transport, lazy
  context-efficient management) remains in the Go kernel — the concurrent session
  shipped a plugin capability manifest (M900) toward that; the rest is gated on
  its reconcile. The catalog is honest about which presets need a secret today.
