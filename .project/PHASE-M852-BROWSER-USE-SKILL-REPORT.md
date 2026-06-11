# PHASE M852 — Browser-use, out of the box (built-in skill bundle)

**Status:** shipped
**Milestone:** M852
**Theme:** Full agentic browser automation — navigate, click, type, submit
forms, screenshot, extract from JavaScript-rendered pages — working **out of the
box**. Owner ask: full browser-use / computer-use (#43/#44), headless-first,
Playwright-style "see when needed".

## Architecture decision

A native Go interactive browser tool would mean bundling Chromium (chromedp/rod)
or a Playwright-go dep — a multi-MB engine that violates AGEZT's **single static
binary, one external dep (BLAKE3)** ethos (DEPENDENCIES.md lists browser engines
as explicit anti-deps for core). And M847/M848 already made agents able to
**install and run anything** (npm, Playwright) via the always-on `code_exec`
sandbox (net-on) + shell.

So browser-use ships as a **built-in skill bundle** the daemon seeds at startup —
zero Go deps, on-ethos, and immediately usable. The agent gets a ready, active
`browser-use` skill whose Playwright driver it runs through `code_exec`.

## What shipped

- **`plugins/builtinskills`** — bundles baked into the binary via `go:embed` and
  seeded into the Forge on boot. `SeedAll` reads each embedded bundle's SKILL.md +
  resources, `Create`s the skill (content-addressed → idempotent) and promotes it
  to **active** so it's in the retrieval pool. Best-effort: a seed failure never
  blocks startup. Daemon prints `built-in skills : seeded (browser-use)`.
- **The `browser-use` bundle** (agentskills.io shape, M847):
  - `SKILL.md` — how to set up and drive the browser; the see/act loop.
  - `scripts/setup.sh` — `npm install playwright` + `npx playwright install
    chromium` (idempotent; the agent runs it once via code_exec/shell; full
    machine permission to install OS deps if needed).
  - `scripts/browse.mjs` — a stateless Playwright driver: opens a fresh headless
    Chromium, runs an ordered action list (goto/click/fill/press/wait),
    optionally screenshots, extracts (text/html/selector), and emits JSON. The
    agent passes a JSON spec and iterates: screenshot → look → act.
  - `reference/actions.md` — login flows, SPA waits, search, table/list
    extraction, failure recovery.

## Surface

- `plugins/builtinskills/builtinskills.go` (embed + `SeedAll`/`seedOne`) +
  `builtinskills_test.go`.
- `plugins/builtinskills/browseruse/{SKILL.md, scripts/setup.sh,
  scripts/browse.mjs, reference/actions.md}`.
- `cmd/agezt/main.go` — `builtinskills.SeedAll(k.Forge(), "")` after the skill
  tool binds; startup line.
- `.gitattributes` — `plugins/builtinskills/** text eol=lf` (the embedded shell
  script must run on Linux).

## Verification

- **Gate:** `go build`, `go vet`, `staticcheck`, linux cross-build clean;
  `builtinskills` tests green; `node --check browse.mjs` passes. No new Go dep
  (go.mod unchanged); no new env.
- **Unit:** `SeedAll` installs `browser-use` **active** with its 3 bundle files
  materialized on disk and a non-empty driver; re-seed is idempotent (dedupes on
  content address, stays active).
- **Boot smoke (isolated home):** daemon logs `built-in skills : seeded
  (browser-use)`; `agt skill list` shows `[active] browser-use`; `agt skill files`
  lists `scripts/browse.mjs`, `scripts/setup.sh`, `reference/actions.md` + the
  bundle dir; files present on disk.
- **The live Playwright install + a real navigation** can't run in the build
  sandbox (no network → npm/Chromium download blocked). The driver is
  syntax-validated and uses standard Playwright APIs; the seeding/activation/
  materialization path is fully verified.

## Notes
- Computer-use (#44) is largely already present: the shell + code_exec tools
  install and run anything under the default-allow posture, and the M848 briefing
  tells agents so. This bundle is the browser slice; future built-in bundles
  (e.g. a desktop-control bundle with pyautogui/scrot) drop into the same seeder.
- Each agent installs Playwright once into its sandbox; the driver is stateless
  so there's no hidden session to manage.
