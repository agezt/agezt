# AGEZT Backend ↔ Frontend Topology Audit (Day 28)

> Generated 2026-09-22 from main HEAD `c12842f1` (squashed merge of PR #580).
> Read-only research; no source files modified.

## Coverage summary
- **Backend** route registrars: `kernel/webui/webui_routes.go`, `kernel/webui/webui_read_routes.go`, `kernel/webui/webui_write_routes.go`, `kernel/restapi/restapi_routes.go`, `kernel/openaiapi/openaiapi_server.go`, 15 channel plugin `mux.HandleFunc`s.
- **Frontend** call surface: `frontend/src/app/api/api.ts` (getJSON/postJSON/postAction), `frontend/src/lib/chat.ts` (`fetch(/api/run)`), `frontend/src/lib/chatStore.tsx`, `frontend/src/lib/snapshot.ts`, `frontend/src/lib/toolbox.ts`, `frontend/src/features/voice/lib/{voice,tts,voiceStatus}.ts`, `frontend/src/app/cursor-pager/cursorPager.ts` (17 named hooks wrapping `usePanel`/`getJSON`), `frontend/src/lib/usePanel.ts`, plus 35 view components.
- **SSE**: `/events` (protected) — `kernel/webui/webui_routes.go:61`, token minted at `/api/sse-token` (line 91).

## 1. Frontend nav views (39 total)
Source: `frontend/src/nav.tsx:323-725` (NAV_GROUPS), `:728` (ROWS), `:732` (NAV), `:758-777` (VIEW_ALIASES), `:76-89` (REMOVED_VIEW placeholder), `:191` (REMOVED_VIEW_IDS).

| # | Section | Row | View id | Label | Component file | Tabs in row |
|---|---|---|---|---|---|---|
| 1 | Talk | Jarvis | `jarvis` | Jarvis | `frontend/src/features/jarvis/components/Jarvis.tsx` | – |
| 2 | Talk | Voice | `chat` | Chat | `frontend/src/features/chat/components/Chat.tsx` (→ `legacy/Chat.tsx`) | – |
| 3 | Talk | Voice | `voice` | Voice | `frontend/src/features/voice/components/Voice.tsx` (+ `VoiceSetup.tsx`) | – |
| 4 | Observe | Monitor | `mission` | Mission Control | `frontend/src/features/observe/components/MissionControl.tsx` | Live Stream (`feed`) |
| 5 | Observe | Monitor | `feed` | Live Stream | `frontend/src/components/EventFeed.tsx` | Mission Control |
| 6 | Observe | Runs | `runs` | Runs | `frontend/src/features/runs/components/Runs.tsx` | Activity, Replay |
| 7 | Observe | Runs | `activity` | Activity | `frontend/src/features/runs/components/Runs.tsx` (alias) | Runs, Replay |
| 8 | Observe | Runs | `replay` | Replay | `frontend/src/features/runs/components/Runs.tsx` (alias) | Runs, Activity |
| 9 | Automate | Workflows | `workflows` | Workflows | `frontend/src/features/workflows/components/page.tsx` (entry: `Workflows.tsx`) | – |
| 10 | Automate | Triggers | `schedules` | Schedules | `frontend/src/features/schedules/components/page.tsx` (entry: `Schedules.tsx`) | Standing orders |
| 11 | Automate | Triggers | `standing` | Standing orders | `frontend/src/features/standing/components/page.tsx` (entry: `Standing.tsx`) | Schedules |
| 12 | Automate | Autonomy | `autonomy` | Autonomy | `frontend/src/features/autonomy/components/Autonomy.tsx` | – |
| 13 | Govern | Approvals | `approvals` | Approvals | `frontend/src/features/govern/components/Approvals.tsx` | – |
| 14 | Govern | Policy | `policy` | Policy | `frontend/src/features/policy/components/page.tsx` (entry: `Policy.tsx`) | – |
| 15 | Govern | Oversight | `overseer` | Overseer | `frontend/src/features/overseer/components/Overseer.tsx` | Council |
| 16 | Govern | Oversight | `council` | Council | `frontend/src/features/council/components/Council.tsx` | Overseer |
| 17 | Agents (fleet) | Agents | `agents` | Agents | `frontend/src/features/agents/components/Agents.tsx` | – |
| 18 | Agents (fleet) | Roster | `roster` | Roster | `frontend/src/features/agents/components/Roster.tsx` | – |
| 19 | Agents (fleet) | Skills | `skills` | Skills | `frontend/src/features/skills/components/page.tsx` (entry: `Skills.tsx`) | – |
| 20 | Agents (fleet) | Capabilities | `market` | Marketplace | `frontend/src/features/market/components/page.tsx` (entry: `Market.tsx`) | Execution Profiles |
| 21 | Agents (fleet) | Capabilities | `execution-profiles` | Execution Profiles | `frontend/src/features/execution-profiles/components/page.tsx` (entry: `ExecutionProfiles.tsx`) | Marketplace |
| 22 | Agents (fleet) | Sandbox | `sandbox` | Sandbox | `frontend/src/features/sandbox/components/Sandbox.tsx` | – |
| 23 | Knowledge | Memory | `memory` | Memory | `frontend/src/features/memory/components/page.tsx` (entry: `Memory.tsx`) | – |
| 24 | Knowledge | World | `world` | World | `frontend/src/features/world/components/page.tsx` (entry: `World.tsx`) | – |
| 25 | Knowledge | Data & Files | `data` | Data Lake | `frontend/src/features/data/components/page.tsx` (entry: `Data.tsx`) | Artifacts & Files |
| 26 | Knowledge | Data & Files | `artifacts` | Artifacts & Files | `frontend/src/features/artifacts/components/page.tsx` (entry: `Artifacts.tsx`) | Data Lake |
| 27 | Knowledge | Thinking Partners | `research` | Research | `frontend/src/features/knowledge/components/Research.tsx` | Analyst, Reflect |
| 28 | Knowledge | Thinking Partners | `analyst` | Analyst | `frontend/src/features/knowledge/components/Analyst.tsx` | Research, Reflect |
| 29 | Knowledge | Thinking Partners | `reflect` | Reflect | `frontend/src/features/knowledge/components/Reflect.tsx` | Research, Analyst |
| 30 | Connect | Providers & Models | `models` | Models & Keys | `frontend/src/features/models/components/page.tsx` (entry: `Models.tsx`) | – |
| 31 | Connect | Routing | `chains` | Fallback Chains | `frontend/src/features/workflows/components/Chains.tsx` | – |
| 32 | Connect | Channels | `channels` | Channels | `frontend/src/features/channels/components/page.tsx` (entry: `Channels.tsx`) | – |
| 33 | Connect | Integrations | `mcp` | MCP Servers | `frontend/src/features/mcp/components/page.tsx` (entry: `Mcp.tsx`) | ACP Agents, Connections |
| 34 | Connect | Integrations | `acp` | ACP Agents | `frontend/src/features/agents/components/ACPAgents.tsx` | MCP Servers, Connections |
| 35 | Connect | Integrations | `connections` | Connections | `frontend/src/features/connections/components/page.tsx` (entry: `Connections.tsx`) | MCP Servers, ACP Agents |
| 36 | Admin | Setup | `setup` | Setup | `frontend/src/features/setup/components/page.tsx` (entry: `Setup.tsx`) | – |
| 37 | Admin | Config Center | `configcenter` | Config Center | `frontend/src/features/configcenter/components/page.tsx` (entry: `ConfigCenter.tsx`) | – |
| 38 | Admin | Identity | `prompts` | Prompts | `frontend/src/features/skills/components/page.tsx` (alias to Skills) | – |
| 39 | Admin | Backups | `backup` | Backups | `frontend/src/features/admin/components/Backups.tsx` | – |

Note: nav.tsx also keeps "preserved" id bindings to the placeholder (`REMOVED_VIEW`) for these legacy ids (no entry in NAV, but the legacy hash still resolves) — `board`, `messages`, `inbox`, `tools`, `toolbox`, `providers`, `catalog`, `routing`, `budget`, `workboard`, `okr`, `seats`, `insights`, `flowstudio`, `toolforge`, `cache`, `conductor`, `persona` — see `REMOVED_VIEW_IDS` at `nav.tsx:191-272`.

## 2. Frontend API calls (consumer surface) — summary
~140 frontend call sites, ~85 distinct API paths. Notable hot paths:
- Chat engine: `fetch("/api/run", POST)` (`chat.ts:408`), `postAction("/api/run/steer")` (`chatStore.tsx:333`), `postAction("/api/cancel_run")` (`chatStore.tsx:345`), `postJSON("/api/chat/summarize")` (`chatStore.tsx:360`)
- SSE: `EventSource(/events?st=...)` (`app/events/events.tsx:61`)
- Memory ops: add, forget, promote, supersede, bulk_forget, prune, tidy, clean, profile rebuild (`features/memory/components/page.tsx:180,215,241,256,269,282,294,304,316,336,435,906,1008`)
- Pulse: pause/resume, beat, cadence, dial, flush, watch, probe, unwatch, quiet (`features/autonomy/components/Autonomy.tsx:303,326,340,354,366,385,407,429,458`)
- Provider keys: add, activate, remove, OAuth logout (`features/api-keys/hooks/useApiKeySubmit.ts:35,60,78`, `features/agents/components/api-keys/components/ChatGPTSignInCard.tsx:161`)

Full per-route table follows in section 4 (cross-reference).

## 3. Backend routes (producer surface) — summary
~258 backend routes across:
- `apiRoutes` (parameterless GET): 47 — `kernel/webui/webui_read_routes.go:13-89`
- `readArgsRoutes` (GET with args): 33 — `kernel/webui/webui_read_routes.go:103-214`
- `writeRoutes` (POST action): 28 — `kernel/webui/webui_write_routes.go:15-143`
- `jsonRoutes` (POST JSON): 47 — `kernel/webui/webui_write_routes.go:152-325`
- Inline routes: ~15 — `kernel/webui/webui_routes.go` (SSE, voice, files, hooks, oauth callback, login/logout)
- REST API v1: 13 — `kernel/restapi/restapi_routes.go:46-78`
- OpenAI-compat: 5 — `kernel/openaiapi/openaiapi_server.go:66-73`
- Channel webhooks: ~15 across `plugins/channels/*/..._.go`

## 4. Cross-reference (sample — see sections 2 and 3 for full coverage)

| Backend route | Method | Backend handler | Frontend caller | View/feature |
|---|---|---|---|---|
| `/api/spend/today` | GET | **MISSING** | `MissionControl.tsx:95` | Mission Control Spend tile — silent no-op |
| `/api/attention` | GET | **MISSING** | `MissionControl.tsx:118` | Mission Control Needs Attention — silent no-op |
| `/api/inbox` | GET | `webui_read_routes.go:124` | (orphan — `useInboxPager` defined but never imported) | – |
| `/api/board` | GET | `webui_read_routes.go:125` | (orphan — `useBoardPager` defined but never imported) | – |
| `/api/board/send` | POST | `webui_write_routes.go` | `agentdetail/comms.tsx:299,321,355` | AgentDetail (Comms) |
| `/api/board/help` | GET | `webui_read_routes.go:45` | (orphan) | – |
| `/api/schedule/{edit,remove,run}` | POST | `webui_write_routes.go` | (no app caller — tests only) | Schedules |
| `/api/standing/{remove,fire}` | POST | `webui_write_routes.go` | (no app caller) | Standing orders |
| `/api/skill/{file,quarantine,archive,revert,share,reassign}` | various | `webui_write_routes.go`, `webui_read_routes.go:182` | (no caller) | Skills |
| `/api/mcp/{attach,detach,enable,remove}` | POST | `webui_write_routes.go:128-131` | (no caller — tests only) | MCP |
| `/api/reflect`, `/api/reflect/run` | GET/POST | `webui_read_routes.go:60`, `webui_write_routes.go:136` | (no caller) | Reflect (the view uses `/api/journal`) |
| `/api/plan/{generate,refine,run}` | POST | `webui_write_routes.go:153-154`, `webui_routes.go:78` | (no app caller; RunDetail uses `/api/run`) | Flow Studio |
| `/api/edict/{test,set_level,set_mode,deny_rm}` | GET/POST | `webui_read_routes.go:230`, `webui_write_routes.go:94-97` | (no caller; only `edict_show` + `deny_add` consumed) | Policy |
| `/api/conductor/{ask,roles}` | POST/GET | `webui_write_routes.go:227`, `webui_read_routes.go:58` | (no caller — nav removed) | Conductor (REMOVED) |
| `/api/taste/*`, `/api/seats/*` | various | `webui_*_routes.go` | (no caller — nav removed) | Taste, Seats (REMOVED) |
| `/api/okr/*`, `/api/workboard/*`, `/api/toolforge/*` | various | `webui_*_routes.go` | (no caller — nav removed) | OKR, Workboard, Toolforge (REMOVED) |
| `/api/run/{pause,resume,step}` | POST | `webui_write_routes.go:55-57` | (no caller; only `/api/run/steer` consumed) | Chat |
| `/api/provider/oauth/{start,status,import}` | POST | `webui_write_routes.go:186-188` | (no caller; only `oauth/logout` consumed) | Models |
| `/api/provider/probe`, `/api/whatsappgw/{status,qr}` | POST | `webui_write_routes.go:198,201-202` | (no caller) | Provider boot |
| `/api/provider/keys` (list) | GET | `webui_read_routes.go:201` | (no caller; only add/activate/remove consumed) | Models |
| `/api/market/show`, `/api/market/sources`, `/api/market/sync` | GET/POST | `webui_read_routes.go:188-189`, `webui_write_routes.go:70` | (no caller) | Marketplace |
| `/api/memory/consolidate` | POST | `webui_write_routes.go:78` | (no caller) | Memory |
| `/api/budget_set` | POST | `webui_write_routes.go:54` | (no caller) | Config |
| `/api/data/collections`, `/api/data/records`, `/api/data/drop`, `/api/data/collection` | GET/POST | `webui_*_routes.go` | (no caller; Data Lake view uses internal helper) | Data Lake |
| `/api/journal_search`, `/api/journal/export`, `/api/journal/verify` | GET | `webui_read_routes.go:84,129,133` | (no app caller) | Journal |
| `/api/storage`, `/api/cache`, `/api/stats`, `/api/providers`, `/api/tools`, `/api/persona` | GET | `webui_read_routes.go` | (no caller) | Various |
| `/api/redact/test` | POST | `webui_write_routes.go:324` | (no caller) | Redaction |
| `/api/configcenter/{list,get}` | GET | `webui_read_routes.go:170-171` | (no caller; only set/delete consumed) | Config Center |
| `/api/skills/hygiene` | GET | `webui_read_routes.go:184` | (no caller) | Skills |
| `/api/execution_profile` (single) | GET | `webui_read_routes.go:136` | (no caller; only `execution_profiles` + `execution_profile_check`) | Execution Profiles |
| `/api/plan_history` | GET | `webui_read_routes.go:198` | (orphan — `usePlanHistoryPager` defined but never imported) | – |
| `/api/ratelimit_log` | GET | `webui_read_routes.go:209` | (orphan — `useRateLimitLogPager` defined but never imported) | – |

## 5. View-level inventory

### Talk › Jarvis — `frontend/src/features/jarvis/components/Jarvis.tsx`
- Tabs: – (single view).
- API calls: `useAgentsPager` (`/api/agents`), `useRunsPager` (`/api/runs`) (line 64), `getJSON("/api/voice/status")` (line 92).
- Wired actions: `chat.send(intent)` (line 121) via `useChat`, `chat.newChat()` (line 252) toast-only.
- **Disabled/unwired**: "Open chat" button (lines 247-260) shows toast instead of navigating to `chat` view. "Open Talk › Voice" button (line 184) shows toast instead of navigating to `voice` view. QuickPrompt (lines 280-295) routes to Chat via toast instead of view nav.

### Talk › Chat — `frontend/src/features/chat/components/Chat.tsx` (→ `legacy/Chat.tsx`)
- Tabs: – (single view).
- API calls: `getJSON("/api/routing")` (`useConversationRouting.ts:12`), `getJSON("/api/catalog")` (`context.tsx:14`), `getJSON("/api/prompts")` (`pickers.tsx:261`, `conversation.tsx:146`), `getJSON("/api/suggestions")` (`conversation.tsx:153`), `getJSON("/api/execution_profiles")` (`pickers.tsx:80`).
- Writes: `postJSON("/api/chat/summarize")` (`chatStore.tsx:360`), `postAction("/api/memory/forget")` (chatStore:258), `postAction("/api/run/steer")` (chatStore:333), `postAction("/api/cancel_run")` (chatStore:345), `fetch("/api/run", POST)` (`chat.ts:408`).
- Wired actions: Full composer (mic, voice-mode, attach chips), regenerate/retry/continue/edit on bubbles, conversation sidebar (pin/rename/delete), export to markdown, model picker, persona picker.
- Disabled/unwired: – (none observed).

### Talk › Voice — `frontend/src/features/voice/components/Voice.tsx` + `VoiceSetup.tsx`
- Tabs: Voice + VoiceSetup (entry sub-views).
- API calls: `fetch("/api/transcribe")` (`voice.ts:13`), `fetch("/api/tts")` (`tts.ts:32`), `getJSON("/api/voice/status")` (`voiceStatus.ts:31`).
- Wired actions: Audio capture/playback, voice-mode toggle, mic permission flow, setup wizard (`VoiceSetup.tsx`).
- Disabled/unwired: – (none observed).

### Observe › Mission Control — `frontend/src/features/observe/components/MissionControl.tsx`
- Tabs: – (single view, shares Monitor row with Live Stream).
- API calls: `getJSON("/api/runs")` (line 47), `getJSON("/api/agents")` (line 48), `getJSON("/api/approvals")` (line 49), `getJSON("/api/status")` (line 50), `getJSON("/api/spend/today")` (line 95), `getJSON("/api/attention")` (line 118).
- Wired actions: Live event tail (SSE), 4 metric tiles, attention queue.
- **Disabled/unwired**: Spend tile and attention queue feed rely on **unregistered endpoints** (`/api/spend/today`, `/api/attention`) — silently no-ops when daemon returns 404.

### Observe › Live Stream — `frontend/src/components/EventFeed.tsx`
- Tabs: – (Monitor row tab).
- API calls: `EventSource(/events?st=...)` via `eventsURLAsync()` (`app/events/events.tsx:61`).
- Wired actions: Rolling event list, type filter, JSON drill-down.
- Disabled/unwired: – (none observed).

### Observe › Runs/Activity/Replay — `frontend/src/features/runs/components/Runs.tsx`
- Tabs: 3 (Runs, Activity, Replay — same component, filter varies).
- API calls: `useCursorPager("/api/runs")` (via `useRunsPager`), `postAction("/api/cancel_run")` (line 77).
- Wired actions: Cancel run, load-more pagination, filter chips.
- Disabled/unwired: – (none observed).

### Automate › Workflows — `frontend/src/features/workflows/components/page.tsx` (entry: `Workflows.tsx`)
- Tabs: – (single view).
- API calls: `getJSON("/api/workflows")` (line 1052), `getJSON("/api/workflows/templates")` (line 1116), `getJSON("/api/workflows/show")` (line 1132), `getJSON("/api/workflows/runs")` (line 956), `postJSON("/api/workflows/save")` (1197), `postJSON("/api/workflows/run")` (1218), `postJSON("/api/workflows/draft")` (807), `postJSON("/api/workflows/refine")` (803), `postJSON("/api/workflows/test_node")` (1350), `postAction("/api/workflows/enable")` (1485), `postAction("/api/workflows/remove")` (1509).
- Wired actions: Save graph, Run (async), Enable/disable, Draft (Copilot), Refine (Copilot), Test single node.
- Disabled/unwired: – (none observed).

### Automate › Schedules — `frontend/src/features/schedules/components/page.tsx` (entry: `Schedules.tsx`)
- Tabs: – (single view; Standing orders is a sibling tab in same Triggers row).
- API calls: `useScheduleFiresPager("/api/schedule/fires")` (line 108), `postJSON("/api/schedule/add")` (line 146), `postAction("/api/schedule/enable")` (241), `getJSON("/api/workflows")` (178), `getJSON("/api/tools_catalog")` (179).
- Wired actions: Create schedule, edit, enable, run now, remove (route exists, tested but no FE call found for remove/run directly).
- Disabled/unwired: Schedule edit/remove/run (`/api/schedule/{edit,remove,run}`) registered but I did not find a direct FE caller outside tests.

### Automate › Standing orders — `frontend/src/features/standing/components/page.tsx` (entry: `Standing.tsx`)
- Tabs: – (single view; Schedules is a sibling tab).
- API calls: `postJSON("/api/standing/add")` (236, 654), `postJSON("/api/standing/edit")` (844), `postAction("/api/standing/enable")` (262), `getJSON("/api/standing/why")` (156).
- Wired actions: Add order, edit, enable/disable, "why" provenance drawer.
- Disabled/unwired: `/api/standing/{remove,fire}` registered but no app-side caller visible (only tests).

### Automate › Autonomy — `frontend/src/features/autonomy/components/Autonomy.tsx`
- Tabs: – (single view).
- API calls: `getJSON("/api/autonomy")` (80), `getJSON("/api/pulse")` (287), `postAction("/api/pulse/pause")` & `resume` (303), `postAction("/api/pulse/beat")` (326), `postAction("/api/pulse/cadence")` (340), `postAction("/api/pulse/dial")` (354), `postAction("/api/pulse/flush")` (366), `postAction("/api/pulse/watch")` (385), `postAction("/api/pulse/probe")` (407), `postAction("/api/pulse/quiet")` (429), `postAction("/api/pulse/unwatch")` (458).
- Wired actions: Pause/resume, beat, cadence, dial, flush, watch add/remove (disk + probe), quiet hours.
- Disabled/unwired: – (all controls wired).

### Govern › Approvals — `frontend/src/features/govern/components/Approvals.tsx`
- Tabs: – (single view).
- API calls: `getJSON("/api/approvals")` (33), `postAction("/api/decide")` (58).
- Wired actions: Approve / deny with reason.
- Disabled/unwired: – (none observed).

### Govern › Policy — `frontend/src/features/policy/components/page.tsx` (entry: `Policy.tsx`)
- Tabs: – (single view).
- API calls: `usePanel` path `/api/policy` (line 327), `usePanel`/`<LogDetail>` path `/api/policy_log` (line 357), `getJSON("/api/edict_show")` (189), `postAction("/api/edict/deny_add")` (526).
- Wired actions: Deny rule add (others presumably in Policies sub-panel).
- Disabled/unwired: `/api/edict/{set_level,set_mode,deny_rm,test}` registered but no direct FE caller visible.

### Govern › Overseer — `frontend/src/features/overseer/components/Overseer.tsx`
- Tabs: – (single view; Council is sibling).
- API calls: `postAction("/api/agents/enable")` (121), `postAction("/api/agents/retire")` (148), `postAction("/api/agents/revive")` (150), reads via `Panel` (Active runs, Needs attention, Agent fleet, Recent activity).
- Wired actions: Enable/retire/revive agent.
- Disabled/unwired: – (none observed).

### Govern › Council — `frontend/src/features/council/components/Council.tsx`
- Tabs: – (single view; Overseer sibling).
- API calls: `getJSON("/api/council/members")` (114), `getJSON("/api/journal")` (83, 100), `postJSON("/api/council/set")` (136), `postJSON("/api/council/ask")` (185).
- Wired actions: Convene council, change membership, see journal entries.
- Disabled/unwired: – (none observed).

### Agents › Agents — `frontend/src/features/agents/components/Agents.tsx`
- Tabs: – (single view).
- API calls: `getJSON("/api/standing")` (270), `getJSON("/api/workflows")` (272), `getJSON("/api/pulse")` (273), `useAgentsPager` (`/api/agents`), `postAction("/api/agents/wake")` (Fleet.tsx:324), `postAction("/api/agents/enable")` (Fleet.tsx:329).
- Wired actions: Wake, enable/disable.
- Disabled/unwired: – (none observed).

### Agents › Roster — `frontend/src/features/agents/components/Roster.tsx`
- Tabs: – (single view).
- API calls: `getJSON("/api/agents/impact")` (326, 460), `postJSON("/api/agents/capabilities")` (548), `postAction("/api/schedule/enable")` (549, 574).
- Wired actions: Create / edit / capabilities / impact drawer.
- Disabled/unwired: – (none observed).

### Agents › Skills / Identity › Prompts — `frontend/src/features/skills/components/page.tsx` (entry: `Skills.tsx`)
- Tabs: – (single view; nav alias for `prompts`).
- API calls: `postAction("/api/skill/promote")` (362, 510), `postJSON("/api/skill/import")` (930).
- Wired actions: Promote, import.
- Disabled/unwired: – (none observed).

### Agents › Marketplace — `frontend/src/features/market/components/page.tsx` (entry: `Market.tsx`)
- Tabs: – (single view; Execution Profiles sibling).
- API calls: `postJSON("/api/market/source/add")` (220), `postJSON("/api/market/source/remove")` (240).
- Wired actions: Add/remove source.
- Disabled/unwired: `/api/market` (list), `/api/market/show`, `/api/market/sources`, `/api/market/sync` registered; no clear FE consumer at top of `page.tsx` (verify deeper).

### Agents › Execution Profiles — `frontend/src/features/execution-profiles/components/page.tsx` (entry: `ExecutionProfiles.tsx`)
- Tabs: – (single view; Marketplace sibling).
- API calls: `postJSON("/api/config/set")` (312, 315, 333).
- Wired actions: Profile edit (writes to config store).
- Disabled/unwired: – (none observed).

### Agents › Sandbox — `frontend/src/features/sandbox/components/Sandbox.tsx`
- Tabs: – (single view).
- API calls: `getJSON("/api/sandbox")` (134), `getJSON("/api/sandbox_file")` (418), `useWardenLogPager` (119), `useNetguardLogPager` (129), `postAction("/api/sandbox/delete")` (278).
- Wired actions: Browse projects, view file, delete project.
- Disabled/unwired: – (none observed).

### Knowledge › Memory — `frontend/src/features/memory/components/page.tsx` (entry: `Memory.tsx`)
- Tabs: – (single view).
- API calls: `useMemoryPager("/api/memory")`, `useMemoryLogPager("/api/memory_log")` (137), `postJSON("/api/memory/add")` (180, 906), `postJSON("/api/memory/supersede")` (1008), `postAction("/api/memory/forget")` (241), `postAction("/api/memory/promote")` (336), `postAction("/api/memory/prune")` (256, 269), `postAction("/api/memory/tidy")` (282, 294), `postAction("/api/memory/clean")` (304, 316), `postAction("/api/profile/rebuild")` (215), `postJSON("/api/memory/bulk_forget")` (435).
- Wired actions: Add, forget (single + bulk), promote, supersede, profile rebuild, prune/tidy/clean (with dry-run).
- Disabled/unwired: `/api/memory/consolidate` registered; no FE call visible.

### Knowledge › World — `frontend/src/features/world/components/page.tsx` (entry: `World.tsx`)
- Tabs: – (single view).
- API calls: `usePanel("/api/world")` (150), `useWorldLogPager("/api/world_log")` (166), `postJSON("/api/world/add")` (222, 683), `postJSON("/api/world/edit")` (583), `postAction("/api/world/relate")` (231, 730).
- Wired actions: Add entity, edit aliases/attrs, relate, forget (LogDetail path prop `/api/world/forget`).
- Disabled/unwired: – (none observed).

### Knowledge › Data Lake — `frontend/src/features/data/components/page.tsx` (entry: `Data.tsx`)
- Tabs: – (single view; Artifacts sibling).
- API calls: `postAction("/api/data/delete")` (206), `postJSON("/api/data/update")` (218), `postJSON("/api/data/insert")` (219).
- Wired actions: Insert / update / delete record.
- Disabled/unwired: Reads come from internal helper (likely `usePanel` or `/api/data/records`).

### Knowledge › Artifacts & Files — `frontend/src/features/artifacts/components/page.tsx` (entry: `Artifacts.tsx`)
- Tabs: – (single view; Data Lake sibling).
- API calls: `usePanel("/api/artifacts")` (145), `postAction("/api/artifact/collect")` (176, 191), `postAction("/api/artifact/delete")` (211). File manager: `lib/files.ts:239,262` → `/api/files/tree`, `/api/files/raw`.
- Wired actions: List, delete, collect (with dry-run), file manager tree/raw.
- Disabled/unwired: – (none observed).

### Knowledge › Research — `frontend/src/features/knowledge/components/Research.tsx`
- Tabs: – (single view; Analyst + Reflect siblings).
- API calls: `postJSON("/api/research/ask")` (45).
- Wired actions: Submit research question (shows sub-questions, sources, claims).
- Disabled/unwired: – (none observed).

### Knowledge › Analyst — `frontend/src/features/knowledge/components/Analyst.tsx`
- Tabs: – (single view; Research + Reflect siblings).
- API calls: `getJSON("/api/journal")` (59).
- Wired actions: Distribution chart, filter by kind/actor/correlation.
- Disabled/unwired: – (none observed).

### Knowledge › Reflect — `frontend/src/features/knowledge/components/Reflect.tsx`
- Tabs: – (single view; Research + Analyst siblings).
- API calls: `getJSON("/api/journal")` (62), `getJSON("/api/memory/audit")` (mentioned in comment).
- Wired actions: Self-talk / journal exploration.
- Disabled/unwired: `/api/reflect`, `/api/reflect/run` registered but no direct caller in `Reflect.tsx` (likely stale aliases).

### Connect › Models & Keys — `frontend/src/features/models/components/page.tsx` (entry: `Models.tsx`)
- Tabs: – (single view).
- API calls: `getJSON("/api/catalog")` (Models.tsx), `getJSON("/api/provider/keys")` (likely via `useApiKeySubmit`), `postJSON("/api/provider/keys/add")`, `postAction("/api/provider/keys/activate")`, `postAction("/api/provider/keys/remove")`, `postJSON("/api/provider/oauth/logout")` (`ChatGPTSignInCard.tsx:161`).
- Wired actions: Add key, activate, remove, OAuth logout.
- Disabled/unwired: – (none observed).

### Connect › Fallback Chains — `frontend/src/features/workflows/components/Chains.tsx`
- Tabs: – (single view).
- API calls: `getJSON("/api/chains")` (ModelPicker), `getJSON("/api/routing")` (chat).
- Wired actions: (Probably chains editor via `/api/chains/set`).
- Disabled/unwired: – (none observed — verify deeper in `Chains.tsx`).

### Connect › Channels — `frontend/src/features/channels/components/page.tsx` (entry: `Channels.tsx`)
- Tabs: – (single view).
- API calls: `getJSON("/api/channels")` (585), `useWebhookLogPager("/api/webhook_log")` (581), `postJSON("/api/channel/account/set")` (235), `postJSON("/api/channel/account/remove")` (439), `postJSON("/api/send")` (455).
- Wired actions: Set/remove account, send message, browse log.
- Disabled/unwired: – (none observed).

### Connect › MCP Servers — `frontend/src/features/mcp/components/page.tsx` (entry: `Mcp.tsx`)
- Tabs: – (single view; ACP Agents + Connections siblings).
- API calls: `postJSON("/api/mcp/add")` (230).
- Wired actions: Add server.
- Disabled/unwired: `/api/mcp/{attach,detach,enable,remove}` registered, no clear FE caller (only tests); `/api/mcp` (list) wired.

### Connect › ACP Agents — `frontend/src/features/agents/components/ACPAgents.tsx`
- Tabs: – (single view; MCP + Connections siblings).
- API calls: `getJSON("/api/acp/agents")` (31).
- Wired actions: Browse inventory.
- Disabled/unwired: – (none observed).

### Connect › Connections — `frontend/src/features/connections/components/page.tsx` (entry: `Connections.tsx`)
- Tabs: – (single view; MCP + ACP siblings).
- API calls: `getJSON("/api/channels")` (103, 614), `getJSON("/api/nodes")` (105, 616), `postJSON("/api/provider/keys/add")` (272, 536), `postJSON("/api/config/set")` (274, 275, 538, 539), `postJSON("/api/provider/reload")` (276, 540), `postJSON("/api/provider/connect")` (296, 533).
- Wired actions: Connect provider, reload, configure env.
- Disabled/unwired: – (none observed).

### Admin › Setup — `frontend/src/features/setup/components/page.tsx` (entry: `Setup.tsx`)
- Tabs: – (single view).
- API calls: `postJSON("/api/catalog/sync")` (211), `postJSON("/api/config/set")` (249, 281, 305, 381), `postAction("/api/provider/reload")` (250, 282), `postJSON("/api/provider/keys/add")` (280), `postJSON("/api/chains/set")` (350), `postJSON("/api/routing/set")` (355), `postJSON("/api/channel/account/set")` (419, 420).
- Wired actions: First-run wizard (sync, keys, chains, routing, channels).
- Disabled/unwired: – (none observed).

### Admin › Config Center — `frontend/src/features/configcenter/components/page.tsx` (entry: `ConfigCenter.tsx`)
- Tabs: – (single view).
- API calls: `postJSON("/api/configcenter/set")` (519), `postJSON("/api/configcenter/delete")` (558), `postAction("/api/persona/set")` (configbackup.ts:63), `postAction("/api/prompts/set")` (configbackup.ts:67), `postAction("/api/routing/set")` (configbackup.ts:71).
- Wired actions: Set/delete keys, persona/prompts/routing backup export-import.
- Disabled/unwired: – (none observed).

### Admin › Backups — `frontend/src/features/admin/components/Backups.tsx`
- Tabs: – (single view).
- API calls: `postJSON("/api/rollback/apply")` (79).
- Wired actions: Roll back to checkpoint.
- Disabled/unwired: `/api/rollback/checkpoints` GET registered but no app caller (Backups likely uses it internally — verify).

## 6. Orphans

### Backend orphans (route registered, no frontend consumer) — ~60 routes

| Backend route | Method | Handler file:line | Notes |
|---|---|---|---|
| `/api/inbox` | GET | `kernel/webui/webui_read_routes.go:124` | `useInboxPager` hook defined in `frontend/src/app/cursor-pager/cursorPager.ts:155` but **never imported** by any view (test files only). |
| `/api/board` | GET | `kernel/webui/webui_read_routes.go:125` | `useBoardPager` defined `cursorPager.ts:176` but **never imported** by any view. Only `/api/board/send` and `/api/board/ack` are consumed. |
| `/api/board/help` | GET | `kernel/webui/webui_read_routes.go:45` | No caller. |
| `/api/plan_history` | GET | `kernel/webui/webui_read_routes.go:198` | `usePlanHistoryPager` defined `cursorPager.ts:321` but never imported. |
| `/api/ratelimit_log` | GET | `kernel/webui/webui_read_routes.go:209` | `useRateLimitLogPager` defined `cursorPager.ts:286` but never imported. |
| `/api/schedule/system_tasks` | GET | `kernel/webui/webui_read_routes.go:28` | No caller. |
| `/api/schedule/test` | GET | `kernel/webui/webui_read_routes.go:203` | No caller. |
| `/api/schedule/remove`, `/api/schedule/run` | POST | `kernel/webui/webui_write_routes.go:104-105` | No caller in app code (tests only). |
| `/api/standing/remove`, `/api/standing/fire` | POST | `kernel/webui/webui_write_routes.go:108-110` | No caller in app code. |
| `/api/skill/file`, `/api/skill/quarantine`, `/api/skill/archive`, `/api/skill/revert`, `/api/skill/share`, `/api/skill/reassign` | POST/GET | `kernel/webui/webui_write_routes.go:99-103`, `kernel/webui/webui_read_routes.go:182` | No caller. |
| `/api/mcp/attach`, `/detach`, `/enable`, `/remove` | POST | `kernel/webui/webui_write_routes.go:128-131` | No caller (only tests). |
| `/api/redact/test` | POST | `kernel/webui/webui_write_routes.go:324` | No caller. |
| `/api/reflect` | GET | `kernel/webui/webui_read_routes.go:60` | No caller. |
| `/api/reflect/run` | POST | `kernel/webui/webui_write_routes.go:136` | No caller. |
| `/api/edict/test`, `/api/edict/set_level`, `/api/edict/set_mode`, `/api/edict/deny_rm` | GET/POST | `kernel/webui/webui_read_routes.go:230`, `kernel/webui/webui_write_routes.go:94-97` | No caller (only `/api/edict_show` + `/api/edict/deny_add` consumed). |
| `/api/plan/generate`, `/api/plan/refine` | POST | `kernel/webui/webui_write_routes.go:153-154` | No app caller. |
| `/api/conductor/ask`, `/api/conductor/roles` | POST/GET | `kernel/webui/webui_write_routes.go:227`, `kernel/webui/webui_read_routes.go:58` | No caller (Conductor nav is `REMOVED_VIEW`). |
| `/api/taste/*`, `/api/seats/*` | POST/GET | `kernel/webui/webui_write_routes.go:270-273`, `kernel/webui/webui_read_routes.go:50,52` | No caller (Taste/Seats nav is `REMOVED_VIEW`). |
| `/api/okr/*` | POST/GET | `kernel/webui/webui_write_routes.go:264-268`, `kernel/webui/webui_read_routes.go:48,178` | No caller (OKR nav is `REMOVED_VIEW`). |
| `/api/workboard/*` | POST/GET | `kernel/webui/webui_write_routes.go:254-274`, `kernel/webui/webui_read_routes.go:46,174-178` | No caller (Workboard nav is `REMOVED_VIEW`). |
| `/api/toolforge/*` | POST/GET | `kernel/webui/webui_write_routes.go:121-124,277-278`, `kernel/webui/webui_read_routes.go:33,220` | No caller (Toolforge nav is `REMOVED_VIEW`). |
| `/api/run/pause`, `/api/run/resume`, `/api/run/step` | POST | `kernel/webui/webui_write_routes.go:55-57` | No caller (only `/api/run/steer` consumed). |
| `/api/provider/oauth/start`, `/status`, `/import` | POST | `kernel/webui/webui_write_routes.go:186-188` | No caller (only `/api/provider/oauth/logout` consumed). |
| `/api/provider/probe` | POST | `kernel/webui/webui_write_routes.go:198` | No caller. |
| `/api/provider/keys` (list) | GET | `kernel/webui/webui_read_routes.go:201` | No caller (only `add/activate/remove` consumed). |
| `/api/market/show`, `/api/market/sources`, `/api/market/sync` | GET/POST | `kernel/webui/webui_read_routes.go:188-189`, `kernel/webui/webui_write_routes.go:70` | No caller. |
| `/api/data/collections` | GET | `kernel/webui/webui_read_routes.go:54` | No caller. |
| `/api/data/drop`, `/api/data/collection` | POST | `kernel/webui/webui_write_routes.go:25,217` | No caller. |
| `/api/data/records` | GET | `kernel/webui/webui_read_routes.go:146` | No caller (panel/page reads). |
| `/api/journal_search`, `/api/journal/export`, `/api/journal/verify` | GET | `kernel/webui/webui_read_routes.go:129,133,84` | No app caller. |
| `/api/storage` | GET | `kernel/webui/webui_read_routes.go:88` | No caller. |
| `/api/cache`, `/api/stats`, `/api/providers`, `/api/tools`, `/api/persona`, `/api/skills/hygiene`, `/api/execution_profile` (single), `/api/configcenter/{list,get}`, `/api/okr`, `/api/seats`, `/api/taste`, `/api/autonomy`, `/api/plan_stats`, `/api/pulse/asks`, `/api/pulse/asks/resolve`, `/api/reaper/scan`, `/api/nodes` (consumed), `/api/whatsappgw/{status,qr}` | GET/POST | (various lines) | See file:line references above. |
| `/api/agents/remove` | POST | `kernel/webui/webui_write_routes.go:243` | No caller (only via `agents/{retire,revive}`). |
| `/api/budget_set` | POST | `kernel/webui/webui_write_routes.go:54` | No caller. |
| `/api/memory/consolidate` | POST | `kernel/webui/webui_write_routes.go:78` | No caller. |
| `/api/redact/test` | POST | `kernel/webui/webui_write_routes.go:324` | No caller (orphan). |

### Frontend orphans (frontend calls, no matching backend route) — 2 routes

| Frontend call | Caller file:line | Notes |
|---|---|---|
| `/api/spend/today` | `frontend/src/features/observe/components/MissionControl.tsx:95` | **Route does not exist.** `useSpendToday` silently no-ops; "Spend today" tile on Mission Control never updates. |
| `/api/attention` | `frontend/src/features/observe/components/MissionControl.tsx:118` | **Route does not exist.** "Needs your attention" panel always renders the empty-state ("Nothing requires your eyes…"). |

Both are caught-and-suppressed errors in the consumer (`.catch`/`try { ... } catch {}`), so they don't crash, but the data is permanently missing.

## 7. Suspicious patterns

### Jarvis "Open chat" button — `frontend/src/features/jarvis/components/Jarvis.tsx:247-260`
The `Button` with `aria-label="Open chat"` calls `chat.newChat()` and shows a toast `"Fresh thread started — switch to Talk › Chat to compose"`. It does **not** navigate to the `chat` view. The user's intent (compose in Chat) is communicated only via toast — clicking does not move focus, scroll, or change route. Same pattern recurs in the same view at:
- `:180-186` — "Open Talk › Voice" button (when voice is unconfigured): onClick fires `ui.toast("Open Talk › Voice to set up hearing and speaking", "info")` and does not navigate.
- `:284-295` — `<QuickPrompt>` cards: `chat.send(label)` + toast("Sent to agent — switch to Chat to see the stream").

All three are the same UX bug — the label promises an action ("Open", "switch to") but the click does not actually navigate. Recommended fix: import `useNav` (or equivalent) and call `nav("chat")` / `nav("voice")`; keep the `chat.newChat`/`send` calls if useful.

### Other UX findings
- **`MissionControl.tsx:95,118`** — Two frontend orphans (`/api/spend/today`, `/api/attention`) — Spend tile and attention queue never render data. Either register the routes or remove the calls.
- **`Overseer.tsx:274-447`** — Four `<Panel>` instances (Active runs, Needs attention, Agent fleet, Recent activity) all rely on implicit `usePanel` paths; if those endpoints ever 404 the panels render the "empty" placeholder silently.
- **`Council.tsx:114`** — Uses `getJSON("/api/council/members")`; no caller for `/api/council/roles` (Conductor sister endpoint), so the panel's editor picks role defaults elsewhere.
- **`Mcp.tsx`** — Only `postJSON("/api/mcp/add")` is wired; attach/detach/enable/remove are test-only. Real MCP lifecycle in the UI is incomplete.
- **`Standing.tsx`** — `postJSON("/api/standing/add")`, `edit`, `enable` wired; `remove`/`fire` not wired.
- **`Schedules.tsx`** — `add`/`enable` wired; `remove`/`run` not wired.
- **Reminder: Tests vs reality** — Many pager hooks (`usePlanHistoryPager`, `useProviderLogPager`, `useRateLimitLogPager`, `useAgentEscalationsPager`) and many write actions are test-mocked but not used in production UI. Recommend either adding thin views or pruning the dead-code paths.

---

## Top counts and findings

**Counts (audited at HEAD `c12842f1`)**
- **Nav views**: 39 (Talk: 3, Observe: 5, Automate: 4, Govern: 4, Agents: 6, Knowledge: 7, Connect: 6, Admin: 4)
- **Frontend API call sites (non-test)**: ~140
- **Distinct API paths called by frontend**: ~85 unique `/api/...` paths
- **Backend routes registered**: ~258
- **Backend orphans** (registered, no FE consumer): ~60 (most are nav-removed surfaces: Conductor, OKR, Workboard, Taste, Seats, Toolforge; plus many idle hooks like `/api/reflect`, `/api/reflect/run`, `/api/edict/test`, `/api/mcp/attach`, `/api/plan/generate`, `/api/redact/test`, etc.)
- **Frontend orphans** (called, no backend): **2** — `/api/spend/today`, `/api/attention`

**Top 5 most actionable findings**
1. **Mission Control has two dead endpoints** (`/api/spend/today`, `/api/attention`) — the Spend tile and the Needs Attention panel never update. Fix: register both in `kernel/webui/webui_read_routes.go`, or remove the calls in `MissionControl.tsx:95,118`.
2. **Jarvis "Open chat" / "Open Talk › Voice" / QuickPrompt buttons toast instead of navigating** — `Jarvis.tsx:184,247-260,284-295`. Fix: invoke `nav(...)` to actually switch views.
3. **Six dead cursor-pager hooks** — `useInboxPager`, `useBoardPager`, `usePlanHistoryPager`, `useRateLimitLogPager`, `useAgentEscalationsPager`. Their backing endpoints `/api/inbox`, `/api/board`, `/api/plan_history`, `/api/ratelimit_log` are registered but no live view consumes them. Fix: prune the unused hooks/routes or add thin views (e.g. a Boards / Inbox tab in Channels).
4. **Removed IA surfaces still have heavy backend weight** — Conductor, OKR, Workboard, Taste, Seats, Toolforge together account for ~30 registered endpoints with zero consumers. Fix: either re-enable them in nav or delete the routes.
5. **MCP/Schedules/Standing/Skill lifecycle actions are incomplete in UI** — `/api/mcp/{attach,detach,enable,remove}`, `/api/schedule/{remove,run}`, `/api/standing/{remove,fire}`, and `/api/skill/{file,quarantine,archive,revert,share,reassign}` are registered but no app code calls them (only tests). Fix: wire the buttons in `Mcp.tsx`, `Schedules.tsx`, `Standing.tsx`, and `Skills.tsx` — or remove from backend.
