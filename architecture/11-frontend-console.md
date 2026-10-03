# 11 — Frontend console (`frontend/`)

**Scope:** `frontend/` — the React single-page web console ("Agezt · Console"): `package.json`, Vite/Vitest/Playwright/knip/tsconfig configs, `scripts/*.cjs`, `e2e/*.spec.ts`, and all of `src/**` (265 non-test `.ts/.tsx` files + `index.css` + one README, 166 test files). `node_modules/` and `test-results/` ignored. The Go side that serves the bundle and proxies `/api/*` (`kernel/webui`) is documented in [03-control-plane-and-http.md](03-control-plane-and-http.md); build/CI tooling outside `frontend/` in [01-daemon-boot-cmd-agezt.md](01-daemon-boot-cmd-agezt.md).

All facts below were read from the working tree on 2026-10-02.

---

## Responsibilities at a glance

- **Operator cockpit** for the `agezt` daemon: chat with agents, watch runs/events live, manage agents/roster/skills, automation (workflows, schedules, standing orders, pulse/autonomy), governance (approvals, Edict policy, overseer, council), knowledge (memory, world model, data lake, artifacts), connections (providers/keys, fallback chains, channels, MCP, ACP), admin (setup wizard, Config Center, rollback backups).
- **Holds no authoritative state.** Every datum comes from the daemon over `/api/*` (JSON, proxied by `kernel/webui` to the control plane) or the single SSE stream `/events`. Only per-device preferences and chat transcripts live in `localStorage`.
- **Ships inside the daemon binary.** `vite build` writes to `../kernel/webui/dist`, which is committed and `//go:embed all:dist`-ed (`kernel/webui/embed.go`). No Node toolchain is needed to `go build`.
- **Runs under a strict CSP** (`script-src 'self'; connect-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:` — `kernel/webui/webui_security_auth.go`): no inline scripts, no inline assets, self-hosted fonts, a dependency-free Markdown renderer with no raw-HTML path.
- **Navigation is data**: `src/nav.tsx` is the entire information architecture (8 sections → 27 rows → 36 views), and five guard tests pin it to docs, help topics and the e2e specs.

---

## 1. Stack and build pipeline

### 1.1 Stack (`frontend/package.json`, name `agezt-webui` v1.1.0, ESM)

| Layer | Packages |
|---|---|
| Runtime | `react` / `react-dom` ^19.2, `@radix-ui/react-tabs`, `@radix-ui/react-tooltip`, `lucide-react` (icons), `@xyflow/react` ^12 (React Flow — workflow canvas, world graph, delegation graph), `@monaco-editor/react` (code viewer; editor bundle fetched from CDN, see Gotchas), `class-variance-authority` + `clsx` + `tailwind-merge` (shadcn-style `cn()`), `@fontsource-variable/{inter,jetbrains-mono,space-grotesk}` |
| Build | `vite` ^8 (rolldown), `@vitejs/plugin-react`, `tailwindcss` ^4 + `@tailwindcss/vite`, `typescript` ^7 (`tsc --noEmit` only) |
| Test | `vitest` ^4 (+ `@vitest/coverage-v8`), `jsdom`, `@testing-library/react`/`dom`, `@playwright/test` ^1.62 |
| Hygiene | `knip` 6.34 (dead exports/deps) |
| `overrides` | `dompurify` 3.4.16, `undici` 8.11.2 (transitive pins) |

No router library, no state library, no chart library, no data-fetching library: routing is a hash router in `App.tsx`, state is React state + a few module-level stores (`useSyncExternalStore`), charts are hand-written SVG (`Sparkline`, `Widgets`).

### 1.2 npm scripts

| Script | Command | Purpose |
|---|---|---|
| `dev` | `vite` | Dev server; proxies `/api` and `/events` to `http://127.0.0.1:8787` (`vite.config.ts`). |
| `build` | `tsc --noEmit && vite build` | Typecheck then emit to `../kernel/webui/dist`. |
| `typecheck` | `tsc --noEmit` | `tsconfig.json` includes `src`, `e2e` and `vite.config.ts` (e2e included deliberately after a syntax error in a spec shipped green). |
| `test` | `vitest run` | Unit + component tests (`src/**/*.test.{ts,tsx}`); node env by default, jsdom per-file via `// @vitest-environment jsdom`. |
| `test:coverage:voice` | `vitest run --coverage --config vitest.voice-coverage.config.ts` | 100% statement/branch/function/line ratchet over 8 Voice/Jarvis source files. |
| `test:e2e` | `playwright test` | Drives the real embedded SPA; needs `AGEZT_WEBUI_URL` (with `?token=`) from the harness. |
| `deadcode` | `knip --reporter json` | Unused files/exports/deps; `knip.json` lists `main.tsx` + every nav view module as entries. Currently clean. |
| `nav:shape` / `nav:coverage` / `nav:audit` | `node scripts/*.cjs` | Static nav audits (see §8.3). |

### 1.3 Build → embed → serve

```
frontend/src/**  --tsc --noEmit-->  OK
     |
     v  vite build (rolldown, plugin-react, @tailwindcss/vite)
kernel/webui/dist/index.html + dist/assets/*.{js,css,woff2}   (~102 hashed assets, ~2.5 MB, committed)
     |   emptyOutDir:true (embed.go lives outside dist, never wiped)
     |   sourcemap:false, assetsInlineLimit:0 (no data: inlining -> CSP-safe)
     |   codeSplitting groups: react-vendor(40) > ui-vendor(30: radix/lucide/cva/clsx/twmerge)
     |                         > flow-vendor(20: @xyflow) > vendor(10); minSize 8 KiB
     v
kernel/webui/embed.go   //go:embed all:dist  -> var distFS embed.FS
     v
agezt daemon (kernel/webui) serves SPA shell + assets, sets CSP, proxies /api/* to control plane,
serves /events SSE                      -> see 03-control-plane-and-http.md
```

- **`Makefile`**: `frontend-build` (`cd frontend && npm run build`), `frontend-test`, `frontend-deadcode`, `webui-e2e` (`scripts/webui-e2e.sh`), `webui-e2e-ps` (`scripts/webui-e2e.ps1`).
- **CI (`.github/workflows/ci.yml`)**: `frontend-dist-in-sync` (`npm ci --ignore-scripts && npm run build`, then `git diff --exit-code -- kernel/webui/dist/` — fails a PR whose committed dist drifts from sources), `frontend-dist-rebuild` (rebuilds and commits dist when needed), `frontend-test` (`deadcode` + `test` + `test:coverage:voice`), `webui-e2e` (Playwright against the shipped dist — deliberately no `npm run build` there: "we drive what actually ships").
- **Fonts**: `main.tsx` imports the three `@fontsource-variable` packages for their `@font-face` side effects; Vite emits same-origin hashed woff2 (`font-src 'self'`). `src/@types/fontsource.d.ts` declares the modules. Repo `.gitattributes` must mark `*.woff2 binary` for the committed dist.
- **Stale-build trap**: because the UI is embedded at build time, "the new UI feature isn't there" almost always means the running daemon was built from a checkout whose `dist` predates the change. The sidebar footer shows `/api/version` (`version`, `revision`, `built`, `build_modified` → a trailing `+`) to make this visible.

---

## 2. Boot sequence and provider tree (`src/main.tsx`)

1. Before first paint (no flash): `applyTheme()` (`lib/theme`), `applyAccentHue(loadAccentHue())` (`lib/accent`), `applyConsoleTitle(loadConsoleName())` (`lib/brand`), `applyAdvanced()` (`lib/advanced`).
2. Module load of `app/api/api.ts` runs `readAndScrubToken()`: reads `?token=` from `location.search`, removes it with `history.replaceState` (so the credential never stays in history/title/bookmarks/Referer) and keeps it **in memory only** as `TOKEN`.
3. Render tree:

```
<StrictMode>
  <UIProvider>                 components/ui/feedback.tsx   toast / confirm / prompt modals
    <AuthGate>                 features/auth/components/Login.tsx   GET /api/authmeta
      <EventsProvider>         app/events/events.tsx        the ONE EventSource (/events?st=…)
        <GlobalActivityProvider>  lib/globalActivity.tsx    in-flight runs: /api/runs seed + SSE fold
          <ChatProvider>       lib/chatStore.tsx            chat engine (survives navigation)
            <App/>             App.tsx                      shell + hash router
```

`AuthGate` renders a probe screen, then either the `Login` lock screen (password required and not authed) or its children — so **no data provider (and no `EventSource`) starts before login**.

---

## 3. App shell (`src/App.tsx`, `src/components/AppNav.tsx`, `src/nav.tsx`)

### 3.1 Layout

```
┌ CommandPalette (⌘K overlay) · HelpDrawer (right sheet) · Setup overlay (first run) · MiniChat (floating) ┐
│ Header: ☰(mobile) ConsoleName ConnectionChip ActivityChip … Chat NotifyToggle ApprovalsBell AlertBell    │
│         ⌘K Help Halt Resume Inspector(🐞 + live LLM count) AccentPicker AdvancedToggle ThemeToggle       │
│ Vitals strip (running · spend today · schedules · skills · approvals · halted) — polls /api/status 5 s   │
│ FleetNowBar (live runs ticker + activity sparkline, from SSE + GlobalActivity)                            │
├──────────────┬───────────────────────────────────────────────────────────────────────────────────────┤
│ SectionNav   │ <main overflow-auto>                                                                    │
│  rail (8 big │   ViewTabs (row facets, only if row has >1 view)                                        │
│  icons) +    │   <div data-view-root data-view=… key=hash class="view-enter">                         │
│  row list    │      <Suspense> IncidentPage | AgentPage | NAV view </Suspense>                         │
│  + build ftr │                                                                                         │
├──────────────┴───────────────────────────────────────────────────────────────────────────────────────┤
│ Inspector (Ctrl+Shift+I) / InspectorClosedBar — bottom dock: LLM calls, tool traces, raw events        │
└───────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

- Below `lg` the sidebar is hidden; the hamburger opens a real modal drawer (focus moved in, Tab trapped, body scroll locked, Esc closes, focus restored).
- Each section has a fixed OKLCH hue in `AppNav.tsx` `SECTION_HUE` (talk 255, observe 150, automate 55, govern 25, fleet 290, knowledge 195, connect 215, admin 230) used for rail tint, active row and tab underline.
- Rail badges: Observe shows `unseenAlerts`; Govern shows `activeRunCount`; the "Oversight" row shows the active run count.

### 3.2 Routing — hash router, view id is the permanent address

| Hash | Resolution | Renders |
|---|---|---|
| `#<viewId>` (optionally `?query`) | `viewFromHash()` in `nav.tsx`: strips `#`/`#/`, drops `?…`, exact match in `NAV` | that view's `render` |
| `#<retiredId>` | `VIEW_ALIASES[id]` | the absorbing view (e.g. `#files`→`artifacts`, `#config`→`configcenter`, `#dashboard`/`#overview`→`mission`, `#health`/`#alerts`/`#activity`/`#replay`→`runs`, `#search`/`#taste`→`memory`, `#inbox`/`#messages`→`channels`, `#wizards`→`setup`, `#storage`→`artifacts`, `#prompts`→`skills`) |
| unknown / empty | fallback | `"mission"` (Mission Control) |
| `#agent/<slug>` | `agentSlugFromHash` (`features/agents/lib/agentnav.ts`, prefix `AGENT_HASH_PREFIX="agent/"`, URI-decoded) | full-page `AgentPage`; nav highlights `agents` |
| `#incident/<id>` | `incidentIdFromHash` (`features/incidents/lib/incidentnav.ts`, prefix `incident/`) | full-page `IncidentPage`; nav highlights `autonomy` |

- **Navigate verb:** `setActive(id)` in App resolves aliases *before* storing (so an in-app link to a retired id never flashes the fallback view), clears agent/incident routes, and calls `goToView` (`lib/nav.ts`), which normalises to a single leading `#` and no-ops when the hash is already current. External hash changes (`hashchange`: back/forward, `openAgent`, `openIncident`, manual edits) re-run `viewFromHash`.
- **Remount per navigation:** the view wrapper is keyed by `incident/<id>` | `agent/<slug>` | full hash, so every navigation re-mounts and replays the `.view-enter` animation; query-string changes on the same view also remount.
- **Lazy loading:** every view is `React.lazy` (`lazyNamed(loader, exportName)` for named exports; `lazy()` for `Chat`, `Jarvis`, `MissionControl`, `Approvals` which default-export); `RouteLoading` is the Suspense fallback.
- **Cross-component deep links**: `focusRun(corr)` (`features/runs/lib/runfocus.ts`, module-level store) + `setActive("runs")` opens a run row; Artifacts reads `#files?path=…` to open its file-manager mode.

### 3.3 Navigation IA (verified against `nav.tsx` and `docs/CONSOLE-IA.md` §3)

Law (nav.tsx comments + CONSOLE-IA §3): **a section is a job, a row is a noun, a tab is a facet**. `NavGroup{id,label,icon,rows}` → `NavRow{id,label,icon,views}` → `NavItem{id,label,icon,render,keywords}`. A row with one view navigates straight there; a row with several renders `ViewTabs` (each tab sets its own view hash, so folding costs no deep link). Derived maps: `ROWS`, `NAV` (flat views in sidebar order), `rowForView`, `groupForView` (view→section id), `sectionForView` (view→section label). Measured surface: **8 sections, 27 rows, 36 views** — matches CONSOLE-IA §3.1 "27 destinations, 36 views".

`REMOVED_VIEW` placeholder + `REMOVED_VIEW_IDS` (board, messages, inbox, health, alerts, overview, insights, cache, tools, providers, budget, flow, workboard, okr, wizards, seats, conductor, search, taste, storage, persona, routing, catalog, toolbox, toolforge) document retired ids; **no live nav entry renders `REMOVED_VIEW`** (asserted by `nav.test.ts`).

### 3.4 Complete view table

Endpoints are the literal paths each view module and its in-feature sub-components call (`getJSON`/`postJSON`/`postAction`/`fetch`/`usePanel`/pagers). SSE = subscribes to the global `/events` stream via `useEvents()`.

| Section › Row | View id | Component file | What it shows | Backend endpoints |
|---|---|---|---|---|
| Talk › Jarvis | `jarvis` | `features/jarvis/components/Jarvis.tsx` (default export) | Presence/companion page (headerless): who is online (roster readiness), what is running, recent runs, voice readiness; not a chat surface. Polls 10 s/30 s. | `/api/agents`, `/api/runs`, `/api/voice/status` |
| Talk › Chat | `chat` | `features/chat/components/Chat.tsx` (re-export shim) → `features/chat/impl/Chat.tsx` | Full chat: conversation sidebar (pin/rename/filter, channel sessions), streamed answers with tool chips/timeline, model/agent/execution-profile/persona pickers, steer/BTW/queue while running, edit-and-resend, regenerate/continue, context gauge + summary fold, attachments, mic + auto-speak. | `/api/run` (POST, streamed SSE via `lib/chat.streamRun`), `/api/run/steer`, `/api/cancel_run`, `/api/chat/summarize`, `/api/config`, `/api/memory/forget`, `/api/catalog`, `/api/prompts`, `/api/suggestions`, `/api/routing`, `/api/execution_profile_check`, `/api/inbox` + `/api/artifacts` + `/api/send` (ChannelSessions), `/api/skills` + `/api/memory` + `/api/runs` (AttachPicker), `/api/transcribe`, `/api/tts`; SSE |
| Talk › Voice | `voice` | `features/voice/components/Voice.tsx` (+ `VoiceSetup.tsx`) | Hands-free voice mode: orb state machine (wake word → listen/VAD → transcribe → run → streaming TTS, barge-in), agent picker, inline STT/TTS provider setup (7 `AGEZT_STT_*`/`AGEZT_TTS_*` fields). | `/api/agents`, `/api/voice/status`, `/api/transcribe`, `/api/tts`, `/api/run`, `/api/config/values`, `/api/config/schema`, `/api/config/set` |
| Observe › Monitor | `mission` | `features/observe/components/MissionControl.tsx` (default export) | "System right now": live event tail, events/sec + token throughput, spend today, attention items, recent runs. | `/api/runs`, `/api/agents`, `/api/approvals`, `/api/status`, `/api/spend/today`, `/api/attention`; SSE |
| Observe › Monitor | `feed` | `components/EventFeed.tsx` | Live Stream: the journal firehose colour-coded by category (`lib/eventmeta`), filter by category/text/correlation, pause, expand payload. | SSE only |
| Observe › Runs | `runs` | `features/runs/components/Runs.tsx` (+ `components/RunDetail.tsx`, `FlightRecorder`, `DelegationGraph`) | Cursor-paged run history with bucket filters, live phase per running run, expandable run detail (tool calls with policy verdicts, steer/pause/step/resume, rollback drawer, replay). Polls 10 s. | `/api/runs` (`useRunsPager`, page size `RUN_PAGE_SIZE`), `/api/cancel_run`, `/api/run/steer`, `/api/run/resume`, `/api/run/pause`, `/api/run/step`, `/api/rollback/checkpoints`, `/api/rollback/apply`, `/api/journal`; SSE |
| Automate › Workflows | `workflows` | `features/workflows/components/Workflows.tsx` → `page.tsx` | Workflow list + React Flow DAG editor (node panels, per-node test, retry/timeout settings, templates), AI copilot draft/refine, runs drawer, enable/run/remove. | `/api/workflows`, `/api/workflows/show`, `/api/workflows/save`, `/api/workflows/run`, `/api/workflows/runs`, `/api/workflows/test_node`, `/api/workflows/enable`, `/api/workflows/remove`, `/api/workflows/templates`, `/api/workflows/draft`, `/api/workflows/refine` |
| Automate › Triggers | `schedules` | `features/schedules/components/Schedules.tsx` → `page.tsx` (+ `lib/shared.ts`) | Typed cron jobs (interval/daily/once/window/continuous) with next fire, targets (agent/workflow/tool/system task), attention reasons, dry-run forecast, fire history. | `/api/schedules`, `/api/schedule/add`, `/api/schedule/edit`, `/api/schedule/enable`, `/api/schedule/run`, `/api/schedule/remove`, `/api/schedule/test`, `/api/schedule/system_tasks`, `/api/schedule/fires` (pager), `/api/agents`, `/api/workflows`, `/api/tools_catalog` |
| Automate › Triggers | `standing` | `features/standing/components/Standing.tsx` → `page.tsx` | Standing orders (cron or journal-subject wake rules): create/edit/enable/fire/remove, "why did it fire", attention/resume issues. | `/api/standing`, `/api/standing/add`, `/api/standing/edit`, `/api/standing/enable`, `/api/standing/fire`, `/api/standing/remove`, `/api/standing/why`, `/api/agents` |
| Automate › Autonomy | `autonomy` | `features/autonomy/components/Autonomy.tsx` (+ `lib/autonomy.ts`, `components/DoctorIncidentTrees.tsx`) | Timeline of everything the daemon did on its own (bursts folded), Pulse control (beat now, cadence, proactivity dial, quiet hours, watch/unwatch, probe, pause/resume, flush), doctor incident trees → `#incident/<id>`. | `/api/autonomy`, `/api/pulse`, `/api/pulse/{resume,pause,beat,cadence,dial,flush,watch,unwatch,probe,quiet}`; SSE |
| Govern › Approvals | `approvals` | `features/govern/components/Approvals.tsx` (default export) | Full-page HITL queue (long form of the header bell) with filters/history; approve/deny. Polls 15 s. | `/api/approvals`, `/api/decide`; SSE |
| Govern › Policy | `policy` | `features/policy/components/Policy.tsx` → `page.tsx` | "Capability policy": per-capability Edict trust level (allow/ask/deny), mode, hard-deny rules, test-a-decision form, secret-redaction tester, policy decision log. | `/api/policy`, `/api/edict_show`, `/api/edict/set_mode`, `/api/edict/set_level`, `/api/edict/deny_add`, `/api/edict/deny_rm`, `/api/edict/test`, `/api/redact/test`, `/api/policy_log` |
| Govern › Oversight | `overseer` | `features/overseer/components/Overseer.tsx` | Supervisory dashboard: what is running (live run context), roster fleet (windowed 60), who needs help (board help requests); retire/revive/enable. Polls 15 s. | `/api/runs`, `/api/agents`, `/api/board/help`, `/api/agents/enable`, `/api/agents/retire`, `/api/agents/revive`; SSE |
| Govern › Oversight | `council` | `features/council/components/Council.tsx` (+ `lib/council.ts`, `lib/councilStore.ts`) | Council of Elders: members, ask a question, watch opinions per round + chair consensus live (store survives navigation). | `/api/council/members`, `/api/council/set`, `/api/council/ask`, `/api/journal`; SSE (`council.*` folded at App level) |
| Agents › Agents | `agents` | `features/agents/components/Agents.tsx` (+ `components/Fleet.tsx`, `lib/fleet.ts`) | Unified census of every durable agent/automation (roster, standing, schedules, workflows, system engines) as FleetCards with trigger chips; Live tab = run monitor. Polls 6 s. | `/api/agents` (`useAgentsPager`), `/api/runs`, `/api/standing`, `/api/schedules`, `/api/workflows`, `/api/pulse`, `/api/agents/wake`, `/api/agents/enable`; SSE |
| Agents › Roster | `roster` | `features/agents/components/Roster.tsx` (+ `roster/*`) | Agent identities: create/edit (soul, model/@chain, fallbacks, budget, trust), guardians + noise policy, graveyard, retire/revive/remove with cascade impact preview, wake/repair. Polls 8 s with AbortController. | `/api/agents`, `/api/agents/add`, `/api/agents/edit`, `/api/agents/impact`, `/api/agents/retire`, `/api/agents/revive`, `/api/agents/enable`, `/api/agents/wake`, `/api/agents/repair`, `/api/agents/remove`, `/api/agents/capabilities`, `/api/skills`, `/api/board`, `/api/schedules`, `/api/schedule/enable` |
| Agents › Skills | `skills` | `features/skills/components/Skills.tsx` → `page.tsx` | Learned procedures/bundles: hygiene, quarantine/promote/archive/share/revert, import, author form, diff vs parent. | `/api/skills`, `/api/skills/hygiene`, `/api/skill/{quarantine,promote,archive,share,revert,import}` |
| Agents › Capabilities | `market` | `features/market/components/Market.tsx` → `page.tsx` (+ `lib/market.ts`) | Capability packs: browse, sources add/remove, sync, pre-install vet report, streamed install/uninstall. | `/api/market`, `/api/market/show`, `/api/market/sources`, `/api/market/source/add`, `/api/market/source/remove`, `/api/market/sync`, `/api/market/install`, `/api/market/uninstall` |
| Agents › Capabilities | `execution-profiles` | `features/execution-profiles/components/ExecutionProfiles.tsx` → `page.tsx` | Shell/interpreter execution profiles: inventory, health checks, policy/back-end config values. Polls 10 s. | `/api/execution_profiles`, `/api/execution_profile_check`, `/api/config/values`, `/api/config/set` |
| Agents › Sandbox | `sandbox` | `features/sandbox/components/Sandbox.tsx` | Code-exec projects agents built (windowed 60), file previews, delete; warden execution log + netguard block log. Polls 8 s. | `/api/sandbox`, `/api/sandbox/delete`, `/api/sandbox_file`, `/api/warden_log` + `/api/netguard_log` (pagers) |
| Knowledge › Memory | `memory` | `features/memory/components/Memory.tsx` → `page.tsx` | Memory records (cursor-paged, multi-select bulk forget), teach/revise facts, prune/tidy/clean, promote to shared, supersede, audit, operator profile rebuild, memory op log. | `/api/memory` (`useMemoryPager`), `/api/memory/{add,audit,forget,prune,tidy,clean,promote,bulk_forget,supersede}`, `/api/profile/rebuild`, `/api/memory_log` (pager) |
| Knowledge › World | `world` | `features/world/components/World.tsx` → `page.tsx` (+ `components/WorldGraph.tsx`) | World-model entities and relations (table + React Flow graph), add/edit/relate/forget, world op log. | `/api/world`, `/api/world/{add,relate,forget,edit}`, `/api/world_log` (pager) |
| Knowledge › Data & Files | `data` | `features/data/components/Data.tsx` → `page.tsx` (+ `lib/datalakedate.ts`) | Data Lake collections and records (windowed 60), insert/update/delete, per-agent attribution filter. | `/api/data/collections`, `/api/data/records`, `/api/data/insert`, `/api/data/update`, `/api/data/delete` |
| Knowledge › Data & Files | `artifacts` | `features/artifacts/components/Artifacts.tsx` → `page.tsx` (+ `lib/artifacts.tsx`, `components/FileManagerWorkspace.tsx`) | Artifact gallery by category (images/html/markdown/pdf/…) with previews, download, delete, collect (disk reclaim); "workspace" mode = 3-pane file manager (`#files?path=` alias). | `/api/artifacts`, `/api/artifact/raw`, `/api/artifact/collect`, `/api/artifact/delete`, `/api/files/tree`, `/api/files/raw` |
| Knowledge › Thinking Partners | `research` | `features/knowledge/components/Research.tsx` | Single-shot multi-source research: sub-questions, sources, verified claims. | `/api/research/ask` |
| Knowledge › Thinking Partners | `analyst` | `features/knowledge/components/Analyst.tsx` | Read-only journal analytics: top kinds, actors, distributions. | `/api/journal?limit=…` |
| Knowledge › Thinking Partners | `reflect` | `features/knowledge/components/Reflect.tsx` | Agent self-talk: memory writes/supersedes + journal grouped by subject, lessons/corrections highlighted. | `/api/memory/audit?limit=120`, `/api/journal?limit=80` |
| Connect › Providers & Models | `models` | `features/models/components/Models.tsx` → `page.tsx` (+ `features/api-keys/*`) | Models & Keys: catalog per provider (models.dev sync), multi-key keyring per provider env (add/activate/remove, fingerprint only), ChatGPT subscription OAuth card. | `/api/catalog`, `/api/catalog/sync`, `/api/provider/keys`, `/api/provider/keys/{add,activate,remove}`, `/api/provider/oauth/{status,start,import,logout}` |
| Connect › Routing | `chains` | `features/workflows/components/Chains.tsx` (+ `lib/chains.ts`) | Named fallback ladders (`@name`), default chain, usage map (who references each chain, dangling refs), per-model health dots. | `/api/chains`, `/api/chains/set`, `/api/catalog` |
| Connect › Channels | `channels` | `features/channels/components/Channels.tsx` → `page.tsx` | ~27 comm channels with multi-account connect forms (fields/probes), channel OAuth, WhatsApp gateway QR, test send, webhook delivery log. | `/api/channels`, `/api/channel/account/set`, `/api/channel/account/remove`, `/api/channel/oauth/start`, `/api/channel/oauth/status`, `/api/whatsappgw/status`, `/api/whatsappgw/qr`, `/api/send`, `/api/webhook_log` (pager) |
| Connect › Integrations | `mcp` | `features/mcp/components/Mcp.tsx` → `page.tsx` | MCP servers (stdio + remote HTTP), curated preset catalog with categories/search, add/attach/detach/enable/remove. Polls 8 s. | `/api/mcp`, `/api/mcp/{add,attach,detach,enable,remove}` |
| Connect › Integrations | `acp` | `features/agents/components/ACPAgents.tsx` (+ `lib/acp.ts`) | Read-only discovery of installed ACP coding agents usable via the `acp_agent` tool. | `/api/acp/agents` |
| Connect › Integrations | `connections` | `features/connections/components/Connections.tsx` → `page.tsx` | "Am I connected": providers, channels, MCP servers, peer nodes; quick key add + provider connect/reload. | `/api/catalog`, `/api/channels`, `/api/mcp`, `/api/nodes`, `/api/provider/keys/add`, `/api/config/set`, `/api/provider/reload`, `/api/provider/connect` |
| Admin › Setup | `setup` | `features/setup/components/Setup.tsx` → `page.tsx` (+ `lib/setup.ts`) | First-run wizard (also auto-opened as an overlay until a provider is usable): pick provider, key/OAuth, models, fallback chain, task routing, optional channel. | `/api/catalog`, `/api/catalog/sync`, `/api/routing`, `/api/routing/set`, `/api/chains`, `/api/chains/set`, `/api/config/set`, `/api/provider/reload`, `/api/provider/keys/add`, `/api/channel/account/set` |
| Admin › Config Center | `configcenter` | `features/configcenter/components/ConfigCenter.tsx` → `page.tsx` (+ `components/ConfigInventory.tsx`) | Schema-driven grouped config editor with provenance, apply-mode/reload boundaries, per-agent skill config; raw "effective configuration" inventory folded under a Disclosure (`#config` alias). | `/api/config/schema`, `/api/config/values`, `/api/config/set`, `/api/configcenter/list`, `/api/configcenter/set`, `/api/configcenter/delete`, `/api/config` |
| Admin › Backups | `backup` | `features/admin/components/Backups.tsx` | Rollback checkpoints (taken before file-mutating ops), filter by run/kind, confirm-then-apply. | `/api/rollback/checkpoints`, `/api/rollback/apply` |

**Non-nav detail routes**

| Route | Component | Shows | Endpoints |
|---|---|---|---|
| `#agent/<slug>` | `features/agents/components/AgentPage.tsx` → `AgentDetail.tsx` (+ `agentdetail/*`) | Per-agent Command Center: glance MetricWidgets + 6 grouped tabs `overview · activity · wiring · mind · model · diag` (`PRIMARY_TABS` in `agentdetail/shared.tsx`; Wiring = triggers+comms, Mind = soul+memory+skills+files, Diagnostics = diag+repair incl. Self-Repair/Iterate with Undo). Polls 6 s. | `/api/agents`, `/api/runs`, `/api/standing`, `/api/schedules`, `/api/workflows`, `/api/pulse`, `/api/memory`, `/api/skills`, `/api/policy`, `/api/policy_log`, `/api/tool_log`, `/api/approvals_log`, `/api/provider_log`, `/api/edict_show`, `/api/tools_catalog`, `/api/agents/{permissions,capabilities,edit,enable,retire,revive,remove,impact,wake,task,repair,repair_status,escalations,activity}`, `/api/board`, `/api/board/send`, `/api/board/ack`, `/api/routing`, `/api/routing/set`, `/api/reaper/scan`, `/api/standing/{add,fire,enable,remove,why}`, `/api/schedule/{run,enable,remove,test}`, `/api/memory/promote`, `/api/skill/files`, `/api/skill/share`, `/api/configcenter/access`, `/api/chains`, `/api/catalog`, `/api/run` (repair runs) |
| `#incident/<id>` | `features/incidents/components/IncidentPage.tsx` | Doctor/escalation incident tree: lineage, phases, resolution history, resolve via delegate or force-chain presets, notes, repair. | `/api/autonomy?limit=250`, `/api/journal?kind=info&limit=800`, `/api/agents`, `/api/agents/{escalations,enable,revive,resolve,retire,repair,wake}`, `/api/board/send`; SSE |

**Shell-level endpoints** (App + header widgets): `/api/version`, `/api/catalog` (first-run check), `/api/runs` + `/api/agents` (⌘K, refreshed on palette open), `/api/halt`, `/api/resume`, `/api/status` + `/api/budget` (Vitals, 5 s), `/api/approvals` + `/api/decide` (ApprovalsBell, 15 s), `/api/sse-token`, `/events`, `/api/authmeta`, `/api/login`, `/api/pulse` (JarvisPresenceCard), `/api/suggestions` (SuggestionsBar), `/api/config` (chat default model). ⌘K config export/import: `/api/persona`, `/api/prompts`, `/api/routing` + `/set` variants (`features/configcenter/lib/configbackup.ts`).

### 3.5 Command palette (⌘K / Ctrl+K)

- `components/CommandPalette.tsx` renders; `lib/commands.ts` ranks: exact label substring wins (`-1000 + index`), else subsequence fuzzy score over `label + group + keywords` (gap penalty); empty query returns input order. Grouped display with flat keyboard index (↑/↓/Enter/Esc).
- Items built in `App.tsx`: one per `NAV` view (group = section label, hint = row label when the row has >1 view, keywords = row label + `NavItem.keywords`), fixed actions (New chat, Halt all runs [confirm], Resume, Help for this page, Toggle theme, Toggle Advanced mode, Export/Import appearance, Export/Import configuration), "Open agent" (every roster profile from `/api/agents`, tagged guardian/retired), "Open run" (8 most recent `/api/runs` → `focusRun` + `runs`).
- **Keywords are mandatory**: `nav.test.ts` requires >2 non-label words per view and checks real operator queries ("api key"→models, "cron"→schedules, "deny"→policy, "disk"/"file manager"→artifacts, "webhook"→workflows, "tts"→voice, "guardian"→roster, "raw"→configcenter, "daemon"→runs).
- `?` (outside text inputs) toggles the help drawer; Ctrl+Shift+I toggles the Inspector.

### 3.6 Help drawer

`components/HelpDrawer.tsx` (right sheet; full width on phones, 30rem tablets, 40% desktop) renders `helpTopicFor(viewId)` from `app/help`. `HELP` = spread of six section files (`converse.ts`, `monitor.ts`, `agents.ts`, `automation.ts`, `knowledge.ts`, `system.ts`) of `HelpTopic{title,intro,sections[{heading,paragraphs?,items?[{term,desc}]}],tips?,related?[{id,label}]}`; `related` chips call `onNavigate`. Topic ids = every `NAV` id plus detail routes `agent` and `incident`; unknown ids get `FALLBACK_TOPIC`. App passes `viewId="agent"` while on `#agent/<slug>`.

---

## 4. Data layer

### 4.1 HTTP client (`src/app/api/api.ts`, barrel `app/api/index.ts`)

| Export | Behaviour |
|---|---|
| `readAndScrubToken()` | Reads + scrubs `?token=`; module constant `TOKEN` (memory only, never storage). |
| `authHeaders(h?)` | Adds `Authorization: Bearer <TOKEN>` when a token exists. Password-login sessions instead rely on the session cookie minted by `/api/login`. |
| `getJSON<T>(path, params?, {signal?, timeoutMs?})` | GET with query; composes caller signal + optional timeout via `AbortSignal.any`; abort → `HTTPError(0, url, "request aborted[ (timeout)]")`. |
| `postJSON<T>(path, body, {signal?})` | POST JSON body. |
| `postAction<T>(path, params?)` | POST with query-string args, no body (the "allowlisted mutating command" shape used by forget/promote/decide/halt…). |
| `HTTPError{status,url}` | Thrown on `!res.ok`; message is the server's `{error}` field or `HTTP <status>`. |
| `eventsURLAsync()` | Fetches an ephemeral SSE token from `/api/sse-token` once (cached promise; on failure resets the cache and falls back to `TOKEN`), returns `/events?st=<token>`. EventSource cannot send headers, hence the separate short-lived token. |

Raw `fetch` is used only where streaming/binary is needed: `lib/chat.ts` (`/api/run` stream), `lib/files.ts` (`/api/files/raw` blob), `features/artifacts/*` (raw bytes), `features/market/lib/market.ts` (streamed install), `features/voice/lib/{voice,tts}.ts` (`/api/transcribe` upload, `/api/tts` audio), `components/FileMention.tsx`, `FileManagerWorkspace.tsx`.

**Auth / 401 handling**: there is **no global 401 interceptor**. `AuthGate` prevents the 401 storm up front by probing `/api/authmeta` (`{password_required, authed}`) and showing `Login` (`POST /api/login {password}` then re-probe; detects "strict mode" where a cookie alone still 401s and tells the user to open the tokened URL). A failing probe falls through to the app (never locks out an otherwise usable console). After that, each view surfaces errors through its own error state/toast; `HTTPError.status` lets callers branch (e.g. `lib/files.ts` treats 404 as "route absent → stub tree").

Query-string args reach the control plane as text; numeric query keys must be registered in `kernel/webui` `numericQueryArgs` (see 03) or the typed accessor 502s.

### 4.2 Live events (`src/app/events/events.tsx`)

- `EventsProvider` owns the **single `EventSource`** for the app (opened after `eventsURLAsync()` resolves; closed on unmount). Each message is JSON `AgentEvent{id?,seq?,ts_unix_ms?,subject?,actor?,kind?,correlation_id?,payload?}`; malformed frames are dropped.
- State: `events` ring buffer newest-first capped at `MAX_FEED = 300`; `connected` (onopen/onerror); `lastEventAt`. Listeners in a ref `Set` → `subscribe(fn)` is referentially stable (safe as an effect dependency) and returns an unsubscribe.
- `connectionState({connected,lastEventAt}, now)` → `live | stale | disconnected` with `STALE_MS = 15_000` ("connected but no event yet" counts as stale). `ConnectionChip` ticks locally so App doesn't re-render every second.
- Consumers: App (LLM-call counter for the Inspector badge, `ingestCouncilEvent`, `ingestConductorEvent`, alert counting), `GlobalActivityProvider`, `ChatProvider`, `AlertBell`, `NotifyToggle` (desktop notifications for approval/failure/halt/budget, live only — no backfill), `Inspector`, `FleetNowBar`, `EventFeed`, MissionControl, Runs, Overseer, Autonomy, Approvals, Agents/AgentDetail (`features/agents/lib/agentlive.ts` patches enabled/retired/status in place and decides when to reload: `doctor.auto_repair`, `roster.created/updated/removed`, `task.received/completed/failed`), IncidentPage, Council.
- The common view recipe is "subscribe → debounce → refetch the list endpoint" (e.g. `overseerShouldRefresh`), plus an interval poll as a safety net (intervals: Vitals 5 s, Schedules 5 s/8 s, Agents/AgentPage/Autonomy 6 s, Roster/Standing/MCP/Sandbox 8 s, Runs/ExecutionProfiles/MissionControl 10 s, Approvals/ApprovalsBell/Overseer 15 s, Jarvis 10 s/30 s).

### 4.3 Global activity (`src/lib/globalActivity.tsx` + `src/lib/activity.ts`)

`GlobalActivityProvider` seeds `ActivityState` from `/api/runs` (`seedFromRuns`) so work that started before the tab opened is visible, then folds every SSE event (`foldActivityEvent`). Revision counters prevent a slow snapshot from clobbering newer live state; it re-seeds after every reconnect (drops runs whose terminal event was missed). `summarize()` → `{running, …}` feeds the header ActivityChip, Govern badge and FleetNowBar.

### 4.4 Chat engine (`src/lib/chatStore.tsx`, `lib/chat.ts`, `lib/conversations.ts`)

- `ChatProvider` lives **above** the router so a streaming reply survives navigation; `useChat()` exposes `ChatEngine` (`send`, `retry`, `continueRun`, `editAndResend`, `stop`, `newChat`, conversation select/remove/rename/pin, model/agent/executionProfile/persona, session-only grants `autoApproveForge` and `trustWebContent`, queue, steer).
- `streamRun(body, onFrame, signal)` POSTs `/api/run` `{intent, model?, history?, system?, agent?, execution_profile?, auto_approve_caps?, prompt_injection_trust?}` and parses the SSE body (`parseSSEChunk`); frames are forwarded agent events plus synthetic `open`/`error`/`done` envelopes, folded by `foldChatFrame` into a `ChatTurn` (timeline, tools, compaction, context).
- Stop = abort the fetch **and** `POST /api/cancel_run`; steer/BTW = `/api/run/steer`; long histories fold via `/api/chat/summarize` (`buildHistoryWithSummary`, `HISTORY_SUMMARY_KEEP`).
- Persistence: `localStorage["agezt.chat.store.v2"]` (migrates legacy `agezt.chat.thread.v1`).
- `MiniChat` (floating overlay, controlled by App) and the Chat view share this engine.

### 4.5 Pagination (`src/app/cursor-pager/cursorPager.ts`, `components/ui/load-more-footer.tsx`)

Owner law: no list fetches/renders unbounded.

- `useCursorPager<T>(path, itemsKey, idKey, limit=50, params?)` → `{paged, error, loading, loadMore, loadingMore, moreError, hasMore, reload}`. First page via `usePanel(path, {limit,…})`; `loadMore` re-requests with `cursor=<next_cursor>`, dedups by `idKey` (rows with a null id are kept, never collapsed — a fixed bug where `String(undefined)` made them duplicates).
- Wrappers: `useAgentsPager` (`/api/agents`, `profiles`, `slug`, 100), `useMemoryPager` (`/api/memory`, `records`, `id`, 100), `useWebhookLogPager` (`deliveries`), `useWardenLogPager` (`executions`), `useNetguardLogPager` (`blocks`), `useWorldLogPager`/`useMemoryLogPager` (`ops`) — all `seq`-keyed; `useScheduleFiresPager` (`/api/schedule/fires`, `fires`, `correlation_id`). Runs has its own `useRunsPager` in `Runs.tsx`.
- `LoadMoreFooter` renders "Load N more" / spinner / non-blocking error / "— end of <label> —". `LogHistoryPanel` standardises header + list + footer for the log pagers (used by Channels, Memory, Sandbox, World).
- **Client windowing** where the API has no cursor: `*_WINDOW = 60` constants (Agents cards, Roster cards, Artifacts, Data records, Market packs, Overseer fleet, Sandbox projects/files, Schedules rows, Skills, Standing, World rows, AgentDetail memory/skills tabs) and `WORKFLOW_WINDOW = 40`; header counts stay computed over the full list.
- `usePanel(path, params)` (`lib/usePanel.ts`) is just mount-fetch + manual `reload` (no polling or auto retry, despite the cursorPager doc comment claiming "polling, auth, error retry, live-event reload").

### 4.6 Browser storage keys (all per-device)

`agezt-theme`, `agezt-accent-hue`, `agezt-console-name`, `agezt-advanced`, `agezt.notify.enabled`, `agezt.chat.store.v2` (+ legacy `agezt.chat.thread.v1`), `agezt.chat.autospeak`, `agezt.voice.wake`, `agezt.voice.agent`, `agezt.setup.skipped`. The console token is **never** stored.

---

## 5. Design system

### 5.1 Tokens (`src/index.css`, Tailwind v4)

- `@import "tailwindcss"` + React Flow CSS; `@source not` excludes tests and `e2e/` from class scanning; `@custom-variant dark (&:is(.dark *))` (dark = `.dark` class on `<html>`; `index.html` ships `class="dark"`).
- `:root` (light) and `.dark` define OKLCH tokens: `--background --foreground --card --panel --muted --border --accent(oklch(L C var(--accent-hue,255))) --accent-2(hue 305) --good(150) --warn(80) --bad(25)` and elevation `--elev-1/2/3`. Only the accent **hue** is user-tunable (`lib/accent.ts` writes `--accent-hue`); lightness/chroma stay per theme.
- `@theme inline` maps them to utilities: `bg-background … text-accent bg-accent2 text-good/warn/bad`, `shadow-e1/e2/e3`, `font-sans` (Inter Variable), `font-mono` (JetBrains Mono Variable), `font-display` (Space Grotesk Variable — all `h1–h6`), and a deliberately tight radius scale (2–4 px for every `rounded-*`).
- Utility classes: `.glass` (theme-aware translucent card), `.text-gradient`, `.glow-accent`, `.accent-rule`, `.gradient-rule`, `.card-lift`, `.focus-glow`, `.pill-soft`, `.skeleton` shimmer, motion keyframes (`view-enter`, `stagger-in`, `msg-in`, `work-pulse`, `toast-in`, `modal-in`, `help-drawer-in`, `nav-drawer-in`, `breathe`, `think-dot`, `now-in`) — all disabled under `prefers-reduced-motion`. Dark body gets an aurora `::before` backdrop.
- **Tone language** (`lib/tone.ts`): `Tone = good | warn | bad | accent | muted` (a meaning, not a colour) → `toneText/toneChip/toneBg/tonePlate/toneSurface/toneBorder/toneBar`, `toneForRate`, `toneForStatus`. Opacity scale: borders /30 /40 /60, tints /5 /10 /15 /20.

### 5.2 Theming and appearance

`lib/theme.ts` — module-level `current` + listener set + `useSyncExternalStore` so header toggle and ⌘K never desync; first run follows `prefers-color-scheme` (default dark), explicit toggles persist. Same pattern for `lib/advanced.ts` (`.advanced` class on `<html>`, calm by default), `lib/accent.ts` (`ACCENTS` presets), `lib/brand.ts` (console name → `document.title`). `lib/appearance.ts` bundles theme+accent+name for ⌘K export/import.

### 5.3 `components/ui/` primitives

| File | Exports | Contract |
|---|---|---|
| `page.tsx` | `Page`, `PageWidth`, `PageMode` | The scroll-safe scaffold: `mode="scroll"` (`min-h-full`, the app's single `<main overflow-auto>` scrolls) or `"fill"` (`h-full min-h-0`, inner panes scroll); `width` `readable`(max-w-5xl) / `wide`(110rem) / `full`; header drawn from `icon/title/description/actions` via `PageHeader`. |
| `page-header.tsx` | `PageHeader` | Gradient-ringed icon, display title, one-line description, actions slot. |
| `section-panel.tsx` | `SectionPanel` | One titled block: tone-tinted icon plate, title, one-line `status`, `actions`, body. Replaced 21 page-local `*Panel`s. |
| `disclosure.tsx` | `Disclosure`, `Advanced` | Progressive disclosure (grid 0fr↔1fr animation, aria-expanded/controls, children stay mounted). Its `Advanced` is a collapsed "Advanced" Disclosure preset. |
| `advanced.tsx` | `Advanced`, `Calm` | **Different** `Advanced`: renders children only when global Advanced mode is on (`Calm` = inverse). Only referenced by tests today. |
| `metric-widget.tsx` | `MetricWidget`, `MetricGrid`, `StatTile` | KPI tile (number, label, icon, pulse, sparkline) and the one compact stat tile. |
| `segmented.tsx` | `Segmented`, `ToggleChip`, `FilterToken` | Exclusive-choice pill row with radiogroup semantics + keyboard; toggle chip; filter token. |
| `tab-nav.tsx` | `TabNav`, `TabDef` | Radix Tabs with icon/label/count — switches **panels** (must be controlled; an uncontrolled version once rendered blank panels). |
| `load-more-footer.tsx` | `LoadMoreFooter` | See §4.5. |
| `feedback.tsx` | `UIProvider`, `useUI`, `ConfirmOptions`, `PromptOptions` | `toast(text, kind)`, `confirm(opts) → Promise<boolean>`, `prompt(opts) → Promise<string|null>`; replaces every `alert/confirm/prompt`. |
| `Modal.tsx` | `Modal`, `ModalProps` | Accessible modal: Esc, backdrop click, focus trap, restore focus. |
| `skeleton.tsx` | `Skeleton`, `SkeletonCard`, `SkeletonList`, `SkeletonGrid` | Content-shaped loading placeholders. |
| `empty.tsx` | `EmptyState` | Dashed friendly empty panel with hint/action. |
| `workspace.tsx` | `Workspace`, `WorkspaceColumn` | 3-pane shell (240/320/420 widths) for fill-mode management pages. |
| `card.tsx` | `Card`, `CardHeader`, `CardTitle`, `CardBody` | Lifted surface; `glass`, `interactive` variants. |
| `button.tsx` | `Button` (cva variants; `accent` = brand gradient) | |
| `badge.tsx` | `Badge`, `statusVariant` | Soft tinted status chip. |
| `input.tsx` | `Input` | Styled text input. |
| `tooltip.tsx` | `TooltipProvider` | Radix tooltip provider (App wraps once). |

Shared non-ui helpers views must reuse: `bytes()` / `money()` (microcents ÷ 1e9) / `pct()` / `fmtCount()` (`app/format`), `cn/clip/prettyJSON/fmtTime/fmtDateTime/fmtWhen/fmtAgo/fmtDue` (`app/utils`), `lib/tone`.

---

## 6. File-by-file reference (every non-test source file)

### 6.1 Root of `src/` and config

| File | What it does |
|---|---|
| `main.tsx` | Entry: imports fonts + `index.css`, applies theme/accent/title/advanced pre-paint, mounts the provider tree (§2). |
| `App.tsx` | App shell: hash router state (`active`, `agentSlug`, `incidentId`, `hashKey`), alias-resolving `setActive`, keyboard shortcuts (⌘K, `?`, Ctrl+Shift+I), ⌘K command list, first-run Setup overlay (`/api/catalog` + `anyCredentialed`, `agezt.setup.skipped`), alert/LLM counters, layout (§3.1), mobile drawer with focus trap, appearance/config import inputs. |
| `nav.tsx` | The IA: `NAV_GROUPS`, `ROWS`, `NAV`, `rowForView`, `groupForView`, `sectionForView`, `VIEW_ALIASES`, `viewFromHash`, `REMOVED_VIEW`, `REMOVED_VIEW_IDS`, lazy view bindings, exported lazy `Setup`/`AgentPage`/`IncidentPage`. |
| `index.css` | Tailwind v4 entry, design tokens, utilities, animations (§5.1). |
| `vite-env.d.ts` | Vite client types + `@fontsource-variable/inter` module declaration. |
| `@types/fontsource.d.ts` | Module declarations for the JetBrains Mono / Space Grotesk font packages. |
| `../index.html` | SPA shell (`<html class="dark">`, `#root`, module script). |
| `../vite.config.ts` | Plugins, `@` alias, dev proxy, build to `../kernel/webui/dist`, chunk groups. |
| `../vitest.config.ts` | Node env, `@` alias, `src/**/*.test.{ts,tsx}`. |
| `../vitest.voice-coverage.config.ts` | 100% coverage ratchet for voice/Jarvis (paths re-mapped after the feature-slice move; comment explains it silently enforced nothing before). |
| `../playwright.config.ts` | `testDir ./e2e`, 1 worker, chromium, retries 2 on CI, trace on first retry. |
| `../knip.json` | Entry list (main + every lazily imported view module). |
| `../tsconfig.json` | Strict, bundler resolution, `@/*` paths, includes `e2e`. |

### 6.2 `src/app/` — cross-cutting core (each subfolder has an `index.ts` barrel `export *`)

| File | What it does |
|---|---|
| `api/api.ts` | HTTP client, token scrubbing, SSE token, `HTTPError` (§4.1). |
| `api/index.ts` | Barrel. |
| `events/events.tsx` | `EventsProvider`, `useEvents`, `AgentEvent`, `connectionState`, `STALE_MS` (§4.2). |
| `events/index.ts` | Barrel. |
| `cursor-pager/cursorPager.ts` | `useCursorPager` + endpoint wrappers (§4.5). |
| `cursor-pager/index.ts` | Barrel. |
| `export/export.ts` | `conversationToMarkdown`, `slugify`, `downloadText` (Blob + object URL download). |
| `export/index.ts` | Barrel. |
| `format/format.ts` | `money`, `pct`, `fmtCount`, `byDescValue`, `bytes` (undefined → "—", 0 → "0 B"). |
| `format/index.ts` | Barrel. |
| `utils/utils.ts` | `cn`, `clip`, memoised `prettyJSON`, time formatters `fmtTime/fmtDateTime/fmtWhen/fmtAgo/fmtDue`. |
| `utils/index.ts` | Barrel. |
| `help/help.ts` | `HELP` aggregate, `FALLBACK_TOPIC`, `helpTopicFor`. |
| `help/types.ts` | `HelpTopic` (+ private `HelpItem`, `HelpSection`). |
| `help/converse.ts` | Topics: voice, jarvis, chat, artifacts, data. |
| `help/monitor.ts` | Topics: mission, autonomy, feed, runs. |
| `help/agents.ts` | Topics: agents, agent, roster, overseer, council, mcp, acp, sandbox. |
| `help/automation.ts` | Topics: workflows, schedules, standing. |
| `help/knowledge.ts` | Topics: memory, world, skills, research, analyst, reflect. |
| `help/system.ts` | Topics: setup, channels, market, skills (duplicate key — `System` spreads last and overrides `knowledge.ts`'s `skills`), backup, configcenter, connections, models, chains, approvals, policy, execution-profiles, incident. |
| `help/index.ts` | Barrel. |

### 6.3 `src/components/` — shared shell + cross-view components

| File | What it does |
|---|---|
| `AppNav.tsx` | `SectionNav` (rail + row list + build footer), `ViewTabs` (row facets as tablist), `Header` (header bar, halt/resume via `postAction`). |
| `CommandPalette.tsx` | ⌘K overlay UI (§3.5). |
| `HelpDrawer.tsx` | Page-aware help sheet (§3.6). |
| `Inspector.tsx` | Bottom debug dock: folds `llm.request/response/token`, `tool.invoked/result` and raw events into LLM-call, tool-trace and event tabs; `InspectorClosedBar`. |
| `MiniChat.tsx` | Floating chat panel bound to `ChatProvider`; controlled/uncontrolled modes. |
| `Vitals.tsx` | Monitoring strip polling `/api/status` + `/api/budget` every 5 s; chips deep-link (running→`activity` alias, schedules, skills, approvals). |
| `FleetNowBar.tsx` | Live-runs ticker: `liveRunsFromEvents`, `mergeLiveRunsWithActivity`, `fleetNowSummary`, activity sparkline buckets. |
| `ActivityChip.tsx` | Header "something is working" spinner + count → Overseer. |
| `ConnectionChip.tsx` | live/stale/disconnected chip with its own 1 s tick. |
| `AlertBell.tsx` | Counts warning/critical alerts (`lib/alerts.classifyAlert`) from SSE; click resets and sets `#alerts` (alias → runs). |
| `ApprovalsBell.tsx` | Pending HITL approvals (polls `/api/approvals` 15 s), inline decide via `/api/decide`; `PendingApproval`, `approvalLabel`. |
| `NotifyToggle.tsx` | Opt-in desktop notifications (`lib/notify`), `useDesktopNotifications`. |
| `ConsoleName.tsx` | Inline-renamable header title (`lib/brand`). |
| `ThemeToggle.tsx` / `AdvancedToggle.tsx` / `AccentPicker.tsx` | Appearance controls bound to `lib/theme` / `lib/advanced` / `lib/accent`. |
| `AgentAvatar.tsx` | Deterministic-hue monogram with live status halo. |
| `AgentPicker.tsx` | Chat's roster-agent selector (`/api/agents`); `agentDirectCallable`. |
| `ModelPicker.tsx` | Grouped/searchable model picker with pinned routing chain first and `@chain` options (`/api/chains`, `/api/catalog`). |
| `ModelChip.tsx` | `ModelChip` (model id or `@chain` with expanded tooltip), `HealthDot`. |
| `AttachPicker.tsx` | Composer paperclip modal: attach a skill/memory/run as context (`lib/attach`). |
| `SuggestionsBar.tsx` | Backend-provided prompt suggestions (`/api/suggestions`) with icon map. |
| `MicButton.tsx` | Record → `/api/transcribe` → composer text; graceful degradation. |
| `Markdown.tsx` | Renders `lib/markdown` AST as escaped React; file-mention chips; code via `MonacoView`. |
| `MonacoView.tsx` | Lazy `@monaco-editor/react` wrapper with eager `<pre data-testid="monaco-fallback">` until mount. |
| `DataView.tsx` | Shape-driven renderer for JSON/widget blocks (table/kv/list, raw fallback); `ToolOutput`. |
| `JsonView.tsx` | `JsonView`, `KeyValue`, `Muted`, `ErrorText`. |
| `Panel.tsx` | Fetch-and-render-prop shell (`Panel`, `Stats`, `Row`, `Count`); with `icon` it uses `Page`. |
| `ActionButton.tsx` | POST an allowlisted command then reload, with optional confirm and toasts. |
| `LogDetail.tsx` | Lazy "click for log" drill-down for `*_log` routes. |
| `LogHistoryPanel.tsx` | Cursor-paged log list + `LoadMoreFooter`. |
| `RunDetail.tsx` | Run detail cards: phase steps, `ToolCallRow` with policy verdict, `SteerControls` (steer/pause/step/resume/cancel), `RunRollbackDrawer`, `RunDetailLoader` (journal fetch + `features/runs/lib/rundetail.deriveDetail`), `remoteArtifactsFromArc`. |
| `FlightRecorder.tsx` | Scrubber/player over `lib/replay.buildReplay` steps with cumulative tokens/cost/tools (no production importer today — see Gotchas). |
| `DelegationGraph.tsx` | React Flow tidy tree of a run's sub-agent delegation (`lib/delegation`). |
| `DoctorIncidentTrees.tsx` | Renders doctor incident trees (from `features/autonomy/lib/autonomy`) with badges; opens incidents. |
| `EventFeed.tsx` | Live Stream view (§3.4). |
| `Fleet.tsx` | `FleetCard`, `FleetDetail`, `TriggerChip`, wake/repair/live ops summaries over `lib/fleet`; wake/enable actions. |
| `FileManagerWorkspace.tsx` | 3-pane file manager over `lib/files` (`/api/files/tree`, raw); `tooLargeReason`. |
| `FileMention.tsx` | Chat file chip + `FileDetail` modal reading file bytes. |
| `ConfigInventory.tsx` | Read-only effective `AGEZT_*` values by area (`/api/config`), folded into Config Center. |
| `JarvisPresenceCard.tsx` | Compact Jarvis entry card (`/api/pulse`) — no production importer today. |
| `Sparkline.tsx` | Pure-SVG smoothed area/line (`sparkPaths`). |
| `AnimatedNumber.tsx` | Eased count-up (reduced-motion aware; initial render = target, rAF-driven afterwards — does not settle under jsdom). |
| `Widgets.tsx` | `BreakdownBar` SVG/CSS widget. |
| `WorldGraph.tsx` | Circle-layout React Flow graph of world entities/relations. |
| `ui/*` | See §5.3 (19 files: `Modal, advanced, badge, button, card, disclosure, empty, feedback, input, load-more-footer, metric-widget, page, page-header, section-panel, segmented, skeleton, tab-nav, tooltip, workspace`). |

### 6.4 `src/lib/` — pure logic + small stores (mostly unit-tested, no JSX)

| File | What it does |
|---|---|
| `accent.ts` | Accent hue presets, load/apply/save, `useAccent`. |
| `acp.ts` | ACP inventory types, `acpCensus`, `acpUsageHint`. |
| `activity.ts` | Live run fold: `seedFromRuns`, `foldActivityEvent`, `summarize`, `buildTree` (parent/sub-agent). |
| `advanced.ts` | Global Advanced mode store (`.advanced` class), `useAdvanced`. |
| `alerts.ts` | Proactive-signal classifier (`classifyAlert`, `isAlert`, `LEVEL_ORDER`), `daemonHalted`, `recentAttentionAlerts`, `attentionAlertCount`. |
| `appearance.ts` | Appearance bundle export/parse/apply. |
| `attach.ts` | `AttachRef` mappers (`skillToRef`, `memoryToRef`, `runToRef`) and context preamble (`buildContext`, `withContext`). |
| `brand.ts` | Console name store → `document.title`. |
| `catalog.ts` | Joins tool inventory + trust levels + usage (`joinCatalog`, `levelTone`) — tests only (Catalog view retired). |
| `chat.ts` | Chat frame model + fold, token estimate (`CHARS_PER_TOKEN`), SSE chunk parser, history/summary builders, `streamRun`. |
| `chatStore.tsx` | `ChatProvider`, `useChat`, `ChatEngine`, `collectLearned` (§4.4). |
| `commands.ts` | `CommandItem`, `filterCommands` fuzzy ranker. |
| `conductor.ts` | Fold of `conductor.*` events into `ConductorRun` (thinker/worker/verifier). |
| `conductorStore.ts` | Module-level conductor run store; only `ingestConductorEvent` (called by App) and `conductorRunsSnapshot` remain — the Conductor view is retired, so this is write-only in production. |
| `conversations.ts` | Conversation `Store` model, persistence (`agezt.chat.store.v2`), per-thread persona/model/agent/profile/summary/queue accessors, rename/pin/filter/sort/start/delete. |
| `delegation.ts` | Tidy-tree layout of runs by `parent_correlation`. |
| `eventmeta.ts` | Event kind → category + hue (`CATEGORIES`, `categoryOf`, `isErrorKind`). |
| `files.ts` | File-tree hook (`useFileTree` with cache), path helpers (`isPathSafe`, `joinPath`, …), `rawFileURL`, `fetchFileBlob`; 404 → deterministic stub tree. |
| `fleet.ts` | Unified agent census (`buildFleet`, `fleetCensus`, `filterFleetEntities`, trigger derivation, identity/authority/resilience labels, `statusKind`). |
| `globalActivity.tsx` | `GlobalActivityProvider`, `useGlobalActivity` (§4.3). |
| `insights.ts` | Run analytics aggregation (`computeInsights`) — tests only (Insights view retired). |
| `intent.ts` | `humanizeIntent`: extracts the real ask from transcript/composed intents for list titles. |
| `language.ts` | Extension/language tables (`CODE_EXTS`, `TEXT_EXTS`, `MENTION_EXTS`), `fileMentionRegex`, `languageFor`. |
| `liveruncontext.ts` | Per-correlation live phase/tool/model/wake context (`buildLiveRunContexts`, `liveWakeLabel`) shared by Overseer and Runs. |
| `markdown.ts` | Tiny dependency-free Markdown → AST (`parseMarkdown`, `parseInline`, `safeHref`). |
| `monaco.ts` | `PINNED_MONACO_VERSION`, `MONACO_CDN_BASE` (jsdelivr), `ensureLoader()` (runs on import). |
| `nav.ts` | `normaliseHash`, `goToHash`, `goToView`. |
| `notify.ts` | Desktop-notification preference + `notifyEventClassify`. |
| `queue.ts` | Pure chat queue ops (`addQueued`, `removeQueued`, `moveQueued`, `dequeueFront`). |
| `replay.ts` | `buildReplay`: journaled arc → cumulative `ReplayStep`s. |
| `routingSuggest.ts` | `suggestChains`: one best keyed model per provider per task — tests only (Routing view retired). |
| `snapshot.ts` | Full daemon snapshot export/restore across persona/prompts/routing/standing/schedules/memory/world — tests only (no UI entry point now). |
| `telemetry.ts` | Per-second event buckets + rolling rates — tests only (MissionControl does its own fold). |
| `theme.ts` | Theme store (§5.2). |
| `tone.ts` | Tone → class maps (§5.1). |
| `toolbox.ts` | CLI Toolbox census/filter helpers — tests only (Toolbox view retired). |
| `usePanel.ts` | Mount-fetch hook with `reload`. |

### 6.5 `src/features/` — one folder per domain (`components/` = UI, `lib/` = pure logic)

Pattern from the "Day 23 god-file split": a feature's nav entry is a small `<Name>.tsx` re-export shim; the implementation is `page.tsx` (still 400–1500 lines with inline sub-components); exported interfaces live in `types.ts`.

**admin**

| File | What it does |
|---|---|
| `admin/components/Backups.tsx` | Rollback checkpoint list/filter/apply (`/api/rollback/*`). |

**agents** (largest feature, ~14k lines)

| File | What it does |
|---|---|
| `components/Agents.tsx` | Agents view: fleet census + Live tab; `summarizeRoots`, `filterRoots`, re-exports run-status helpers from `lib/fleet`. |
| `components/Roster.tsx` | Roster view (1.5k lines): list/filter/sort, create/edit modals, guardian noise roll-up, graveyard, lifecycle actions, polling with abort. |
| `components/AgentPage.tsx` | `#agent/<slug>` page: loads profile + census, renders `AgentDetail` in page mode. |
| `components/AgentDetail.tsx` | Per-agent Command Center: header metrics, 6 tabs, fetches all per-agent diagnostics (§3.4). |
| `components/AgentActivity.tsx` | Agent's own runs + spend/last-active + filtered live log (`/api/agents/activity`, `/api/runs`). |
| `components/AgentRepair.tsx` | Self-Repair/Iterate ×N: runs the agent on its own failure brief, auto-applies proposed soul/model/fallback changes with Undo; `repairReadinessPassport`, `stripForEdit`. |
| `components/ACPAgents.tsx` | ACP agents view. |
| `components/agentdetail/Overview.tsx` | Actionable overview: last failure, how it runs, budgets, attention, identity card, lifecycle panel under Advanced. |
| `components/agentdetail/MindTab.tsx` | Soul, standing instructions, tasklist, owned memory/skills/files (inner tab row). |
| `components/agentdetail/ModelTab.tsx` | Primary model + fallback chain, global per-task chain, provider/fallback events; edit via `/api/agents/edit`. |
| `components/agentdetail/TriggersTab.tsx` | Schedules/standing orders bound to the agent with forecast (`/api/schedule/test`), fire/enable/remove, "why". |
| `components/agentdetail/comms.tsx` | `CommsTab` mailbox (board send/ack) + mailbox wake/priority helpers. |
| `components/agentdetail/MemoryTab.tsx` | Agent memory (window 60), promote to shared. |
| `components/agentdetail/SkillsTab.tsx` | Private skills (window 60), share. |
| `components/agentdetail/FilesTab.tsx` | Skill/workdir files (`/api/skill/files`). |
| `components/agentdetail/DiagTab.tsx` | Diagnostics escape hatch: posture, permissions, denials, approvals, tool errors, health, overrides, repair (`/api/agents/repair`). |
| `components/agentdetail/CapabilityPanel.tsx` | `CapabilityControlPanel`: per-agent tool allow/deny, high-impact lockdown, config access (`/api/agents/capabilities`, `/api/configcenter/access`). |
| `components/agentdetail/LifecyclePanel.tsx` | `LifecycleInterventionPanel`: retire/revive/remove with cascade presets and impact (`/api/agents/impact`). |
| `components/agentdetail/capability.tsx` | Pure capability/permission helpers (effective permissions, CSV list ops, USD↔microcents, config override text). |
| `components/agentdetail/lifecycle.tsx` | Pure lifecycle/removal/repair summary helpers. |
| `components/agentdetail/tasks.tsx` | `AgentTaskList`, `OperationalTaskList`, `ToolPolicyBox`. |
| `components/agentdetail/shared.tsx` | Shared row types, `DetailTab`/`PRIMARY_TABS`, small widgets (`StatePill`, `BudgetBar`, `Stat`, `MiniPolicy`, `LifecycleConfigEditor`, `ConfigOverrideBox`, `AgentNowPanel`), `editableAgentProfile`. |
| `components/roster/form.tsx` | `RosterModal`, `NewAgentForm`, `EditAgentForm`, `profileFields` (model/`@chain` mapping). |
| `components/roster/cards.tsx` | `ImpactList`, `CascadeOption`, `AgentKindBadge`, `IdentityPill`. |
| `components/roster/filters.ts` | Roster sort/filter, needs-repair/attention predicates. |
| `components/roster/guardians.ts` | Guardian safety/noise contract + quieting payloads. |
| `components/roster/passports.ts` | Lifecycle/graveyard/task/hierarchy/wake/repair summaries. |
| `components/roster/removal.ts` | Removal cascade presets, plan, impact, decision summaries. |
| `components/roster/shared.ts` | Roster wire types, action result toasts, `slugOk` (mirrors kernel slug rule ≤64), `trustRank`, `usdToMc`. |
| `lib/agent.ts` | `agentHue`, `initials`. |
| `lib/agentactivity.ts` | Agent run correlations, event matching, activity pulse/operational state. |
| `lib/agentdetail.ts` | 2k lines of pure deep-panel helpers (scope/correlation filters, `summarizeAgent`, repair/escalation/provider-routing summaries, runtime status). |
| `lib/agentlive.ts` | SSE → in-place agent patches + reload trigger. |
| `lib/agentnav.ts` | `#agent/<slug>` addressing. |
| `lib/agentrepair.ts` | Repair brief builder + proposal parser/applier. |

**api-keys** (shared key-entry primitives; import via barrel)

| File | What it does |
|---|---|
| `components/ApiKeyField.tsx` | The one password-style key input (reveal, set/pinned badges, "Get one" link, Enter-to-save). |
| `components/ChatGPTSignInCard.tsx` | ChatGPT-subscription OAuth flow (`/api/provider/oauth/{status,start,import,logout}`), polling, cancel, disconnect. |
| `components/KeyListItem.tsx` | One keyring entry (label, fingerprint, activate, delete). |
| `hooks/useApiKeySubmit.ts` | `/api/provider/keys/{add,activate,remove}` + toast/refresh/busy. |
| `index.ts` | Barrel. |
| `README.md` | Usage law: every key/OAuth entry goes through this module. |

**artifacts**: `components/Artifacts.tsx` (re-export shim) · `components/page.tsx` (gallery + workspace mode, `CATEGORY_META`, `groupByCategory`, `fileManagerHash`, `looksLikePath`) · `lib/artifacts.tsx` (artifact domain: classification `categoryOf/isImage/isPdf/textKind/isRunInternal`, `rawURL`, `downloadArtifact`, `BlobArtifact`; moved out of the old Files view so other modules don't import from a screen).

**auth**: `components/Login.tsx` (`AuthGate`, `Login`, probe screen).

**autonomy**: `components/Autonomy.tsx` (view + `PulseControl`, `cadenceLabel`) · `lib/autonomy.ts` (timeline grouping `groupConsecutive`, doctor incident grouping/trees/phase/source labels).

**channels**: `components/Channels.tsx` (shim) · `components/page.tsx` (`Channels`, `ConnectForm`, `POPULAR_CHANNELS`) · `components/types.ts` (`ChannelField`, `MediaCaps`, `ChannelProbe`, `ChannelAccount`, `ChannelRow`) · `components/ChannelSessions.tsx` (chat-sidebar live per-user channel sessions, `MESSAGE_TAIL`) · `lib/channelSessions.ts` (merge `/api/inbox` threads by channel_kind+channel_id+sender).

**chat**: `components/Chat.tsx` (shim; explains `impl/` was renamed from `legacy/` because a dead-code report mistook it for a duplicate) · `impl/Chat.tsx` (the chat surface) · `impl/message.tsx` (`MessageRow`, `UserBubble` with edit, `AssistantBubble`) · `impl/conversation.tsx` (`ConversationItem`, `EmptyState`, `QueuePanel`, `lastAssistantTools`) · `impl/context.tsx` (context gauge chip/modal, `CompactionNote`, cached catalog) · `impl/pickers.tsx` (`ExecutionProfilePicker`, `ConversationPersona`, `PromptLauncher`, `FallbackNote`, `SteerNote`, `TurnMeta`, `SummaryDivider`) · `impl/useChatSession.ts` · `impl/useComposer.ts` (draft + staged attachments) · `impl/useContextWindow.ts` (scroll pinning) · `impl/useConversationControls.ts` (sidebar filter) · `impl/useConversationRouting.ts` (pinned routing chain from `/api/routing`) · `impl/useSteering.ts` (steer/note) · `impl/useVoice.ts` (auto-speak via server TTS with browser fallback).

**configcenter**: `components/ConfigCenter.tsx` (shim) · `components/page.tsx` (`ConfigCenter`, `FieldRow`, reload-boundary summaries, agent-config scope labels) · `components/types.ts` (`FieldType`, `ApplyMode`, `Field`, `ValueEntry`) · `lib/configbackup.ts` (persona+prompts+routing bundle for ⌘K export/import).

**connections**: `components/Connections.tsx` (shim) · `components/page.tsx` (`Connections`, `ConnectivityStrip`).

**council**: `components/Council.tsx` · `lib/council.ts` (pure `council.*` fold: seats, rounds, opinions, progress) · `lib/councilStore.ts` (module-level store: `startCouncilRun`, `ingestCouncilEvent`, `applyCouncilResult`, `hydrateCouncilRun`, `useCouncilStore`, `genCouncilCorr`).

**data**: `components/Data.tsx` (shim) · `components/page.tsx` (`Data`, record attribution helpers) · `lib/datalakedate.ts` (lenient date keys because the lake stores fields verbatim).

**execution-profiles**: `components/ExecutionProfiles.tsx` (shim) · `components/page.tsx` (view + rollup/status tone helpers) · `components/types.ts` (profile/check/config value types).

**govern**: `components/Approvals.tsx`.

**incidents**: `components/IncidentPage.tsx` · `components/IncidentBadges.tsx` (`IncidentBadges`, `incidentPhaseBadgeClass`) · `lib/incidents.ts` (incident meta from events/autonomy items, resolution presets/history, delegate/force-chain drafts and validation) · `lib/incidentevents.ts` (`isIncidentFamilyEvent`, badge items, summaries) · `lib/incidentnav.ts` (`#incident/<id>`).

**jarvis**: `components/Jarvis.tsx`.

**knowledge**: `components/Research.tsx`, `components/Analyst.tsx`, `components/Reflect.tsx`.

**market**: `components/Market.tsx` (shim) · `components/page.tsx` (view) · `lib/market.ts` (`fetchPackDetails`, `VetReport`, `streamMarket` streamed install/uninstall frames, `stepFromFrame`).

**mcp**: `components/Mcp.tsx` (shim) · `components/page.tsx` (view, `CATALOG` presets, `NewServerForm`, validators `serverNameOk/urlOk`, parsers `splitArgs/parseEnv/parseHeaders`) · `components/types.ts` (`MCPServer`, `CatalogCategory`, `CatalogEntry`).

**memory**: `components/Memory.tsx` (shim) · `components/page.tsx` (view, `TeachFactForm`, `ReviseFactForm`, `parseMemoryJSON`).

**models**: `components/Models.tsx` (shim) · `components/page.tsx` (Models & Keys view) · `lib/models.ts` (catalog flatten/filter/group, `pinnedOptions`, `findModelContext`, `modelHealth`, `fmtContext`).

**observe**: `components/MissionControl.tsx`.

**overseer**: `components/Overseer.tsx` (+ `overseerShouldRefresh`).

**policy**: `components/Policy.tsx` (shim) · `components/page.tsx` (`Policy`, `DenyAddForm`, `PolicyTestForm`, `RedactionCheckForm`).

**runs**: `components/Runs.tsx` (view, `useRunsPager`, `runBucket/runCounts/runMatches`, `RUN_PAGE_SIZE`) · `lib/rundetail.ts` (assemble tool calls across `policy.decision`/`tool.invoked`/`tool.result` by `call_id`, `mergeEvents`, `deriveDetail`) · `lib/runfocus.ts` (cross-view "open this run" signal).

**sandbox**: `components/Sandbox.tsx` (+ `isBuildNoise`).

**schedules**: `components/Schedules.tsx` (shim) · `components/page.tsx` (view, `NewScheduleForm`, intent/payload hints) · `components/types.ts` (`ScheduleTarget`) · `lib/shared.ts` (wire types, counts, attention reasons, health passports, system-task presets/fallbacks, `SCHEDULE_ROW_WINDOW`).

**setup**: `components/Setup.tsx` (shim; overlay + page modes) · `components/page.tsx` (wizard) · `lib/setup.ts` (`providerKeyEnv`, `anyCredentialed`, `rankProviders`, chain/fallback/task-routing helpers).

**skills**: `components/Skills.tsx` (shim) · `components/page.tsx` (view, `AuthorSkillForm`, `scanSkill`, `lineDiff`, `diffSkillAgainstParent`).

**standing**: `components/Standing.tsx` (shim) · `components/page.tsx` (view, `NewOrderForm`, `EditOrderForm`, attention/frequency/resume issue helpers, `initiativeEnforcement`).

**voice**: `components/Voice.tsx` · `components/VoiceSetup.tsx` · `index.ts` (barrel exposing `transcribeAudio` etc.) · `lib/voiceSession.ts` (hands-free state machine with injected `VoiceIO`; `createBrowserVoiceIO` = MediaRecorder/AudioContext/VAD/wake word) · `lib/sentenceChunker.ts` (streaming text → speakable sentences, skips code fences) · `lib/tts.ts` (`/api/tts` with SpeechSynthesis fallback, stoppable `Utterance` for barge-in) · `lib/speech.ts` (browser SpeechSynthesis wrapper) · `lib/voice.ts` (`transcribeAudio` → `/api/transcribe`) · `lib/voiceStatus.ts` (`getVoiceReadiness` via `/api/voice/status`, browser capabilities; browser SpeechRecognition is only for wake word, never an STT fallback) · `lib/voiceCatalog.ts` (curated OpenAI-audio-compatible STT/TTS providers and voices).

**workflows**: `components/Workflows.tsx` (shim re-exporting page helpers) · `components/page.tsx` (list + React Flow editor, `CopilotPanel`, `RunsDrawer`, `toFlow/fromFlow`, `portsForNode`) · `components/types.ts` (`Wf`, `WfNode`, `WfEdge`, `WfSettings`, `WfTemplate`, `WfRun`…) · `components/Chains.tsx` (Fallback Chains view, `ChainUsage`) · `lib/chains.ts` (pure chain reducers: `isChainRef`, `chainRef`, `validateChainName`, rename/delete/move).

**world**: `components/World.tsx` (shim) · `components/page.tsx` (view + add/edit/relate forms, `kindBreakdown`, `parseWorldJSON`) · `components/types.ts` (`WorldEntity`).

### 6.6 Tests (166 files, grouped)

- `src/lib/*.test.ts`, `src/app/**/*.test.ts`, `features/*/lib/*.test.ts` — pure-logic unit tests (node env).
- `src/components/**/*.test.tsx`, `features/*/components/*.test.tsx` — jsdom component tests via Testing Library (one per view; `Runs.pager.test.tsx`, `chatStore.cancel.test.tsx`, `RunDetail.steer.test.tsx`, `files.treecache.test.ts`, `conductorStore.retention.test.ts` cover specific regressions).
- `components/missing-smoke.test.tsx` — smoke for AdvancedToggle, ModelChip, Panel skeleton, Markdown XSS escaping.
- Voice: `voiceSession.browser.test.ts` etc., all under the 100% coverage ratchet.
- Guard tests at `src/` root: see §8.

---

## 7. Key flows

### 7.1 Page load

1. Daemon prints `http://host:port/?token=…`; browser loads `index.html` + hashed assets (CSP `'self'`).
2. `api.ts` scrubs the token from the URL into memory.
3. `AuthGate` → `GET /api/authmeta` → lock screen or app.
4. `EventsProvider` → `GET /api/sse-token` → `new EventSource("/events?st=…")`.
5. `GlobalActivityProvider` → `GET /api/runs` seed; `ChatProvider` loads conversations from localStorage + `GET /api/config`.
6. App → `GET /api/version`, `GET /api/catalog` (setup needed?), resolves `location.hash` → lazy view chunk → view fetches its endpoints.

### 7.2 Chat turn

1. `send(intent)` → append user `Msg` + empty `ChatTurn`; `streamRun` `POST /api/run` with history (summary-folded).
2. Response is `text/event-stream`; each `data:` frame → `foldChatFrame` → re-render the assistant bubble (timeline, tool chips, model, fallback notes).
3. Concurrently the same run's events arrive on `/events` → GlobalActivity/ActivityChip/FleetNowBar/Inspector update.
4. `done` frame finalizes; queued messages auto-send (`dequeueFront`); auto-speak via `/api/tts` if enabled. Stop → abort + `/api/cancel_run`.

### 7.3 Navigation

`click row` → `onSelect(row.views[0].id)` → `setActive` (alias-resolve) → `goToView` sets hash → `hashchange` → `viewFromHash` → keyed remount → `Suspense` → view. ⌘K/`openAgent`/`openIncident`/help chips all reduce to setting the hash.

---

## 8. Guard tests and e2e

### 8.1 Vitest guards (all 43 assertions pass on the current tree)

| Test | Enforces |
|---|---|
| `src/nav.test.ts` | 8 section labels in fixed order (Talk, Observe, Automate, Govern, Agents, Knowledge, Connect, Admin); view ids unique and NAV = flattened groups; **≤ 6 rows per section**; unique non-empty rows; `rowForView` consistent; representative view→section mappings; Agents and Roster stay separate rows; labels operators know; a multi-view row's label must match its first tab's render; every `VIEW_ALIASES` source is not live and its target is live; every view has >2 keyword synonyms beyond its label; real ⌘K queries resolve (§3.5); no live entry renders `REMOVED_VIEW`; `REMOVED_VIEW_IDS` non-empty; **no component rendered by two nav entries** (the rule that retired activity/replay/prompts). |
| `src/nav-docs.test.ts` | Every "N sections / rows / destinations / sidebar entries / views / nav items" number in prose of `README.md`, `docs/CONSOLE-IA.md`, `docs/CONSOLE.md` equals what `nav.tsx` measures (8 / 27 / 36); fenced code blocks are exempt ("prose is normative, fences are illustrative"); asserts non-empty extraction first. Its header still says "currently RED by design" — stale; it is green. |
| `src/consoledoc.test.ts` | `docs/CONSOLE.md` "The views, at a glance" section names every section (bold), every row and every view label, and no stale section names (Converse, Monitor, Automation, Observe, Build — unless live). |
| `src/designsystem.test.ts` | Over `src/components/**` only: one titled-section primitive (no hand-rolled `grid size-8 … rounded-lg border` icon plate outside `ui/section-panel`); no private `*ToneCls = {` maps outside `lib/tone`; no local `Metric/Tile/Stat/StatCard` functions outside `ui/metric-widget`; no `humanSize/fmtBytes/formatBytes`; no hand-rolled pill toggle buttons outside `ui/segmented`; no stat tiles repeating ≥2 filter-chip labels; `<Page title>` must relate to the nav label; tone opacities snap to border 30/40/60 and bg 5/10/15/20; no `content: null` TabNav-as-filter; no own `text-gradient` `<h2>` inside `<Page>`. |
| `src/e2enav.test.ts` | Parses `e2e/webui.spec.ts` `openView(section, item[, row])` calls and checks each names a real section/row/facet; the section lists pinned in `views.spec.ts` (`const GROUPS = [...] as const`) and `webui.spec.ts` (`for (const job of [...])`) equal `NAV_GROUPS` labels in order. |
| `src/app/help/help.test.ts` | Help topic for every NAV id + `agent` + `incident`; no orphan topics; each topic substantial (intro >40 chars, sections with body, item desc >20 chars); `related` ids resolve; unknown id → `FALLBACK_TOPIC`. |
| `vitest.voice-coverage.config.ts` | 100% coverage on Voice/Jarvis sources. |
| `knip` (`npm run deadcode`) | No unused files/exports/deps (test imports count as usage). |

### 8.2 Playwright e2e (`frontend/e2e/`, against a real keyless daemon booted by `scripts/webui-e2e.{sh,ps1}` with `AGEZT_DEMO_ECHO=1`, isolated `AGEZT_HOME`, Web UI on :18787)

| Spec | Covers |
|---|---|
| `webui.spec.ts` | Deep: truthful Jarvis/Voice readiness without speech providers; shell + live SSE indicator; all 8 sections reachable; Chat; Standing orders; Runs run detail ("Final answer", `[echo] hello e2e`); World; Autonomy (Beat now, cadence, dial); Schedules (daemon cron presets); Policy (Test decision, Secret redaction); Agents (Presence/Next wake/Spend today/"How does this run?"); zero console errors under CSP. |
| `views.spec.ts` | Wide: walks every section → row → tab read from the DOM; asserts `[data-view-root]` is non-empty and no console errors; ≥35 views visited (`SKIP` = "Flow Studio", now a non-existent label). Exists because an uncontrolled `TabNav` once blanked Dashboard/Runs/Status while all unit tests passed. |
| `nav-audit.spec.ts` | Every nav view renders real content (no "This view was retired" placeholder, no stuck "Loading…"); retired ids resolve to their alias targets. |
| `nav-screenshots.spec.ts` | Saves `.tmp/nav-screenshots/<id>.png` per view for eyeball review (`scripts/nav-screenshots.ps1`, :18800). |
| `api-keys.spec.ts` | Models & Keys, Connections, Channels, Voice, Setup mount cleanly with the shared api-keys primitives (`scripts/webui-smoke.ps1`, :18789). |

### 8.3 Static scripts (`frontend/scripts/`)

| Script | What it does |
|---|---|
| `dump-nav.cjs` | Parses `nav.tsx` bindings, resolves each render target to a source file and prints PASS/FAIL per row. |
| `audit-nav-shape.cjs` | Per-section/row view-count report from `nav.tsx` text. |
| `audit-help-coverage.cjs` | Regex cross-check of NAV ids vs help topic keys (superseded by `help.test.ts`; its `SKIP_KEYS` includes real-looking ids like `agent`, `alerts`). |

---

## 9. Extension points

- **Add a view**: create `features/<area>/components/<Name>.tsx` (use `Page`, `SectionPanel`, `StatTile`, `Segmented`, `Disclosure`, `lib/tone`, `app/format`); add a `lazyNamed` binding and a `NavItem{id,label,icon,render,keywords}` in an existing row (or a new row; keep ≤6 rows per section) in `nav.tsx`; add the module to `knip.json` `entry`; add a `HelpTopic` under the matching `app/help/<section>.ts`; update the counts/maps in `README.md`, `docs/CONSOLE.md` ("The views, at a glance") and `docs/CONSOLE-IA.md` (nav-docs/consoledoc fail otherwise); keywords need >2 non-label words. e2e picks it up from the DOM automatically. Rebuild and commit `kernel/webui/dist`.
- **Retire / rename a view**: remove it from `NAV_GROUPS`, add `VIEW_ALIASES[old] = target` (and optionally `REMOVED_VIEW_IDS`), delete or move its help topic (orphan check), update docs counts and `e2e/nav-audit.spec.ts` / `nav-screenshots.spec.ts` lists.
- **New paginated list**: add a wrapper next to `useAgentsPager` in `app/cursor-pager/cursorPager.ts` (pick `itemsKey` + `idKey`), render `LoadMoreFooter`; without server cursors use a `*_WINDOW = 60` client window.
- **New live reaction**: `const { subscribe } = useEvents(); useEffect(() => subscribe(fn), [subscribe])`; filter by `kind`/`subject`, debounce, refetch. For state that must outlive the view, use a module-level store above the router (pattern: `councilStore`, `chatStore`, `runfocus`) and ingest from `App.tsx`.
- **New API-key / OAuth entry**: use `@/features/api-keys` (`ApiKeyField`, `KeyListItem`, `useApiKeySubmit`, `ChatGPTSignInCard`).
- **New ⌘K action**: append to `actions` in `App.tsx` `commands` with `group: "Action"` and keywords.
- **New appearance pref**: mirror `lib/theme.ts` (module state + listeners + `useSyncExternalStore` + pre-paint apply in `main.tsx`), add it to `lib/appearance.ts`.

---

## 10. Gotchas / invariants

- **The view id is the permanent address.** Hash, help-topic key and ⌘K id never change on reorganisation; rows/tabs are presentation only. A retired id must alias, never fall through (the fallback looks like "the app lost the page").
- **Fallback view is `mission`, not chat.** `viewFromHash` comments still say "falling back to chat", and App comments call `NAV[0]` "Chat"; `NAV[0]` is actually `jarvis`. Code is authoritative.
- **Unseen-alert badge cannot be acknowledged.** `App.tsx` resets `seenAlerts` only when `active === "alerts"`, but `alerts` is now an alias to `runs`, so `active` never equals `"alerts"`; the Observe rail badge (`unseenAlerts`) only ever grows within a session. `SectionNav`'s per-row badge keyed on row id `"alerts"` is dead (no such row). `AlertBell` keeps its own counter and resets on click, so the header bell is fine.
- **Two different `Advanced` components**: `ui/disclosure.tsx` `Advanced` (collapsible fold, used by views) vs `ui/advanced.tsx` `Advanced` (renders only in global Advanced mode; production-unused). Import the right one.
- **`designsystem.test.ts` scans only `src/components/`.** All 30+ views live in `src/features/`, so the "one SectionPanel / one tone map / one StatTile / Page-title-matches-nav" rules are not enforced on views at all; the title check is effectively vacuous (it looks for `components/<id>.tsx`).
- **Not every view uses `Page`.** Chat and Jarvis (intentionally headerless), plus MissionControl, EventFeed, Approvals, Research, Analyst, Reflect, Backups and the Setup page render their own shells — the "every nav view sits on Page" claim from the responsive overhaul no longer holds after the Day 25/26 re-adds.
- **Monaco vs CSP.** `lib/monaco.ts` points the loader at `cdn.jsdelivr.net`, while the daemon CSP is `script-src 'self'; connect-src 'self'`. Under the embedded daemon the editor bundle cannot load, so operators see `MonacoView`'s `<pre>` fallback; the real editor only appears under `vite dev` (no CSP). Self-hosting `monaco-editor/min/vs` is the documented fix.
- **`lib/files.ts` stub fallback** (pre-"Slice 5") still returns a deterministic fake tree on 404 even though `kernel/webui/files_route*.go` now exists — a misconfigured route would show fake files instead of an error.
- **Token handling**: never put the console token in storage or URLs; SSE uses the ephemeral `/api/sse-token` value (`st=`). Password-only sessions rely on the cookie; "strict mode" needs both.
- **No global 401 interceptor**; `AuthGate` is the only gate. A session that expires mid-use surfaces as per-view errors, not a redirect to Login.
- **Events buffer is 300 newest.** Anything needing history must fetch `/api/journal` or a list endpoint; the buffer is for live UI only. `NotifyToggle` deliberately ignores backfill.
- **`useCursorPager` dedup**: rows without an id are always kept; ids are compared as strings.
- **`AnimatedNumber` under jsdom** only settles on the initial render (rAF does not fire) — do not wrap shared metric widgets in it or count-based view tests break.
- **Dist must be rebuilt and committed** with every frontend change (CI `frontend-dist-in-sync`); rebase conflicts in `kernel/webui/dist` are resolved by `npm run build` + `git add dist`, never by hand-merging hashed files.
- **Voice coverage config is path-pinned**; moving a voice/Jarvis file requires updating `vitest.voice-coverage.config.ts` or the 100% ratchet silently covers nothing (it already happened once).
- **`help/system.ts` and `help/knowledge.ts` both define `skills`**; the spread order in `help.ts` makes `system.ts` win.
- **Stale comments to distrust**: `cursorPager.ts` says `usePanel` polls/retries (it does not); `AgentPage.tsx` lists 11 tabs (there are 6 grouped tabs); `Workflows.tsx` mentions `features/workflows/index.ts` (does not exist); `views.spec.ts` skips "Flow Studio" (no such view since Day 23); `nav-docs.test.ts` says it is RED (it is green); `FileManagerWorkspace.tsx` says it is reached from `Files.tsx` (now Artifacts' workspace mode).

---

## 11. Dependencies and layering

- Intended layering: `app/*` (core) ← `lib/*` + `components/*` (shared) ← `features/*` (domains) ← `nav.tsx` / `App.tsx`.
- **Actual inversions**: shared layers import features — `components/{AgentAvatar,Fleet,FleetNowBar}` → `features/agents`; `components/{DoctorIncidentTrees,EventFeed,FlightRecorder,RunDetail}` → `features/incidents`; `components/DoctorIncidentTrees` → `features/autonomy`; `components/{ModelChip,ModelPicker}` → `features/models` + `features/workflows`; `components/RunDetail` → `features/runs`; `components/MicButton` → `features/voice`; `components/FileManagerWorkspace` → `features/artifacts`; `lib/{activity,alerts,chat,replay,telemetry}` → `features/runs`; `lib/{alerts,replay}` → `features/incidents`; `lib/routingSuggest` → `features/models`; `lib/snapshot` → `features/{configcenter,memory,schedules,standing,world}`.
- **Feature cycles**: `agents ↔ incidents`, `autonomy ↔ incidents`. Other cross-feature edges: agents → models, workflows; channels → api-keys, artifacts; chat → channels, models, voice; connections/models/setup → api-keys; overseer → incidents, runs; voice → api-keys, configcenter; workflows → models.
- **Production-unreferenced modules** (imported only by their own tests; knip treats tests as usage so it stays green): `lib/catalog.ts`, `lib/insights.ts`, `lib/routingSuggest.ts`, `lib/snapshot.ts`, `lib/telemetry.ts`, `lib/toolbox.ts`, `components/FlightRecorder.tsx`, `components/JarvisPresenceCard.tsx`, `components/ui/advanced.tsx`. `lib/conductor.ts` + `lib/conductorStore.ts` are reached only through App's write-only `ingestConductorEvent` (the Conductor view is retired). These are leftovers of retired views (Catalog, Insights, Routing, Backup snapshot, Toolbox, Conductor, Dashboard), not guards.
- Backend coupling: every path in §3.4 is served by `kernel/webui` (direct routes: files, artifacts, rollback, transcribe, tts, voice status, session/login, OAuth) or proxied to `kernel/controlplane` — see [03-control-plane-and-http.md](03-control-plane-and-http.md). Domain semantics: runs/agent loop [04](04-agent-runtime.md); policy/routing/keys/config [05](05-governance-routing-security.md); memory/world/datalake/board/artifacts [06](06-data-memory-state.md); schedules/standing/pulse/market/MCP/ACP/workflows/channels/voice [07](07-autonomy-and-extensibility.md); providers [08](08-providers.md); channels [09](09-channels.md); tools [10](10-tools.md).
