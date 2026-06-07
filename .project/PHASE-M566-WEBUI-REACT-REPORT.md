# Phase M566 — Web UI rebuilt as an embedded React 19 + Vite SPA

**Type:** Architecture / feature (user directive; realizes frozen decision A4)
**Date:** 2026-06-07
**Branch:** `feat-webui-react`

## Goal

Rebuild the Agezt Web UI on a modern React stack — React 19, Vite, Tailwind CSS
v4, shadcn/Radix primitives, lucide-react, dark/light, React Flow, responsive —
while keeping it **embedded in the single Go binary** (built to static assets,
`go:embed`-ded, served by `kernel/webui`; no Node at runtime). This realizes
decision **A4** (`.project/DECISIONS.md:13`, "TypeScript + React 19 + Vite for the
Web UI"); the prior hand-rolled server-rendered `dashboard.html` was the MVP cut.
Standing requirement for all future webui work (memory `webui-react-stack`).

PR 1 of a phased migration: establish the whole toolchain + serving + CI and ship
a flagship subset; remaining read panels port to bespoke React views in follow-ups.

## What shipped

### Frontend (`frontend/`, new — Vite + React 19 + TS)
- Stack: React 19.2, Vite 6.4, Tailwind v4.3 (`@tailwindcss/vite`), shadcn-style
  UI primitives over Radix, lucide-react, **@xyflow/react 12.11** (React Flow),
  cva/clsx/tailwind-merge. Dark/light via a class toggle (persisted).
- `vite.config.ts`: builds to `../kernel/webui/dist`, `emptyOutDir`, `base "/"`,
  `sourcemap:false`, `assetsInlineLimit:0` (external hashed assets, no inline JS →
  serveable under `script-src 'self'`).
- App shell: responsive header (live indicator, Halt/Resume, theme toggle) + nav
  sidebar (horizontal scroll on small screens). Single shared `EventSource`
  (`EventsProvider`) feeds the live feed and Flow Studio's per-node updates.
- Views: live **Event Feed**, **Status**, **Runs** (+ expandable event-arc detail
  via `/api/journal`), **Budget**, and **Flow Studio** rebuilt on **React Flow**
  (intent→Generate→edit JSON→DAG→Refine→Run, with live node recolour from
  `node.*`/`plan.*`; loop = box, gate = hexagon, top-down depth layout). The other
  ~12 read panels render via a generic JSON view (their `/api/*` routes already
  exist) so nothing is lost during the port.

### Go serving (`kernel/webui/`)
- `embed.go`: `//go:embed all:dist` (lives outside `dist/` so `emptyOutDir` can't
  wipe it). `New` sub-roots it via `fs.Sub`.
- `handleSPA`: serves `index.html` at `/` and any non-API/asset path (client deep
  links), `Cache-Control: no-cache`. `handleAssets`: serves `/assets/*` with
  **explicit, OS-independent Content-Type** (stdlib `mime.TypeByExtension` returns
  `text/plain` for `.css` on Windows → browsers refuse it under nosniff) and a long
  immutable cache. Assets + favicon are **public** (the browser loads them as
  subresources and can't attach the `?token=`); the data surfaces (`/events`,
  `/api/*`) and the page shell stay token-gated.
- Static CSP replaces the per-request nonce: `default-src 'none'; script-src
  'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; …` — stricter for
  scripts (no inline at all); `'unsafe-inline'` only on style-src (React Flow/Radix
  inject runtime inline transforms, no code execution).
- Removed the old `dashboard.html` embed + `newCSPNonce`.

### Build / CI
- `Makefile` `frontend-build` target (`npm ci && npm run build`); kept separate so
  `go build`/`go test` stay Node-free (the bundle is committed).
- `.github/workflows/ci.yml` `frontend-dist-in-sync` job (ubuntu, modeled on
  `codegen-in-sync`): rebuild + `git diff --exit-code -- kernel/webui/dist/`.
- `.gitignore` `/frontend/node_modules/`; `.gitattributes` keeps `dist/` bytes
  (`-text`) so EOL rewrites can't dirty the in-sync diff.

## Verification

- **Unit:** `kernel/webui` tests reworked — deleted the ~15 HTML/JS-marker tests,
  added SPA tests (index served + #root + `/assets/` ref; deep-link → index;
  asset served + immutable cache + JS MIME; assets public; static CSP no-nonce;
  embedded dist present). All pass.
- **Full gate:** `GOMAXPROCS=3 go test ./... -p 2 -count=1` exit 0 (75 pkgs).
  gofmt (staged blobs) / vet / staticcheck clean. `go.mod`/`go.sum` unchanged.
- **Build reproducibility:** two consecutive `vite build`s produce byte-identical
  `dist/` (same content hashes) — the basis for the in-sync CI gate.
- **Runtime smoke (executable proof, criterion-7):** booted the real daemon,
  served the SPA over HTTP — `/` returns the shell (#root + `/assets/` refs, static
  CSP), assets 200 with correct MIME + immutable cache + public, deep link
  `/runs` → index, `/api/plan/run` executed a plan end to end (journaled
  `plan.*`/`node.*`), 0 panics.
- **Real browser (Playwright):** loaded the console under the strict CSP — **0
  console errors**; the full shell rendered (header, nav, theme toggle, Flow
  Studio); pasting a plan JSON drew the **React Flow DAG** live (`scan → gate →
  fix` with edges + zoom controls); SSE showed ● live.

## Counts

- Packages: 75 (unchanged). Tests (funcs + subtests): 2431 → **2425** (net −6 from
  replacing HTML-marker tests with SPA tests).
- No Go dependency added (`go:embed` is stdlib; npm deps are build-time only).

## Out of scope (documented follow-ups)

- Port the remaining read panels (config, cache, providers, tools, policy,
  schedules, world node-link graph, skills, standing, memory, inbox, reflect,
  approvals) from the generic JSON view to bespoke React views — 2–3 per follow-up
  PR; Go side unchanged.
- Run-detail modals / drill-downs (provider/tool/policy logs) as React views.
- Optional Vitest/Playwright suite in CI for the React side.
- If cross-platform `dist` byte-reproducibility proves fragile in CI, downgrade the
  in-sync job to a build-succeeds + presence check (the matrix-wide
  `TestEmbeddedDistPresent` already guards presence).
