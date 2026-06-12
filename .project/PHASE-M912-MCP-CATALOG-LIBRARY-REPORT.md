# PHASE M912 — MCP Catalog Library: 43 Verified Popular Servers + Category Browser

**Status:** shipped · frontend-only (conflict-free with the concurrent kernel arc)
**Builds on:** M897 (popular-servers gallery), M898 (per-server env), M904 (remote presets), M906 (lazy loading)

## What

The owner asked for a serious built-in MCP library — "agezt içine dahili bir çok MCP koyalım,
insanlar seçip çalıştırsın" — seeded from a 35-server wish list. This phase grows the
popular-servers gallery from 17 to **43 presets** and makes it browsable:

1. **Catalog expansion (CATALOG in `frontend/src/views/Mcp.tsx`).** Every entry's package name,
   install command, env var names, and remote URL was verified against npm/PyPI, vendor docs and
   the official `modelcontextprotocol/servers` repo (2026-06). Notable curation decisions:
   - The owner's wish list contained several packages that are **archived** (`server-puppeteer`,
     `server-slack`, `server-brave-search`, `server-everart`, `server-sentry`…) or **never
     existed** (`server-sendgrid`, `server-bluesky`, `server-sftp`, `server-googli`,
     `server-trello`, `server-jira`, `server-pagerduty`, `server-datadog` under the
     `@modelcontextprotocol` scope). Archived-but-still-working ones stay (postgres, gdrive,
     google-maps, github stdio); dead/fictional ones were replaced by maintained equivalents:
     puppeteer → `@playwright/mcp` (official Microsoft), slack → `slack-mcp-server` (korotovsky),
     brave → `@brave/brave-search-mcp-server` (official Brave), sentry → `@sentry/mcp-server`
     (official), jira/trello-class needs → `mcp-atlassian` + `airtable-mcp-server` + Linear.
   - **OAuth-only hosted servers** (Linear, Notion remote, Stripe remote, Vercel…) can't attach
     through our headers-only Streamable HTTP transport, so each is offered in a shape that works:
     Notion/Stripe/Supabase/Neon via their official stdio packages with a token, Linear via the
     `mcp-remote` stdio bridge (browser OAuth on first attach). Bearer-friendly remotes
     (GitHub, Hugging Face, Context7, DeepWiki) stay native remote presets.
   - Skipped deliberately: `@elastic/mcp-server-elasticsearch` (deprecated upstream),
     `@e2b/mcp-server` ("no longer supported" on npm; agezt has its own code_exec sandbox),
     Cloudflare hosted (SSE-only endpoints; kernel speaks Streamable HTTP).
2. **Category browser.** `CatalogEntry.category` (`core | web | data | dev | apps`),
   `CATEGORY_LABELS`, and a pure `filterCatalog(entries, cat, query)` drive filter chips
   (All (43) / Core / Web & search / Databases / Dev & cloud / Apps & docs) plus a free-text
   search box over name + description, with a "no presets match" empty state.
3. **Secret ergonomics.** Presets carry the exact env/header key names (`TAVILY_API_KEY`,
   `MDB_MCP_CONNECTION_STRING`, `SLACK_MCP_XOXC_TOKEN`…), so "Use" prefills `KEY=` lines in the
   per-server env field (M898) — the operator pastes the value and registers; the base
   environment stays scrubbed.

## Category × count

| Category | Presets |
|---|---|
| Core (official reference) | everything, filesystem, fetch, memory, git, time, thinking (7) |
| Web & search | playwright, duckduckgo, brave, tavily, exa, firecrawl, googlemaps, youtube (8) |
| Databases & vectors | postgres, sqlite, mongodb, redis, supabase, neon, qdrant, chroma, pinecone (9) |
| Dev & cloud | github, githubremote, kubernetes, awsdocs, azure, sentry, deepwiki, context7, huggingface (9) |
| Apps & docs | notion, linear, atlassian, slack, gdrive, airtable, stripe, obsidian, excel, arxiv (10) |

## Invariants kept

- Names obey the kernel rule (`^[a-z][a-z0-9]{0,15}$`) — enforced by an existing test that runs
  over every entry; uniqueness and one-transport-shape-per-entry tests likewise still pass.
- No kernel/Go changes; register/attach flow, scrubbed env, policy gating untouched.

## Tests

`Mcp.test.tsx`: +3 — category validity & non-empty categories, 35+ size floor, `filterCatalog`
unit coverage (category ∧ query, case-insensitivity, no-hit) and a component test driving the
chips + search box. Full frontend suite green; `kernel/webui` Go tests green; production build
embedded into `kernel/webui/dist`.

## Cross-session note

Implemented in an isolated git worktree because the concurrent session (M911 roster cards) and
this one share the main working copy; uncommitted edits there were mutually clobbered once. Both
PRs touch `kernel/webui/dist` + `CHANGELOG.md` — whichever merges second must rebase and rebuild
the frontend.
