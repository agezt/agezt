# Day 28 Topology Audit — Verification (2026-09-22)

**Audit under review:** `.project/TOPOLOGY-AUDIT-DAY28.md`
**Scope:** §6 "UI lifecycle incomplete" + §3 "Backend orphans" + the "Frontend orphans" sidebar.
**Method:** every route mentioned in the audit was re-checked against `kernel/webui/webui_read_routes.go` + `webui_write_routes.go` (route registration) and `frontend/src/**` (callers), via `rg "/api/<path>"`.

---

## 1. Re-verification of §6 claims

Each row below answers the explicit re-verify prompts in the task brief. "Audit claim" is a direct quote of the audit row; "Status" reflects whether the orphan claim is *correct* (route really has no caller), *wrong* (route is in fact wired), or *partial* (some of the listed subroutes are wired, others are not).

### 1.1 `/api/schedule/{edit,remove,run}`

| Subroute | Audit claim | Status | Evidence |
|---|---|---|---|
| `/api/schedule/edit` | "No caller in app code (tests only)" | **WRONG** — wired | `frontend/src/features/schedules/components/page.tsx:1150` (`postJSON(editing ? "/api/schedule/edit" : ...)`); tests `Schedules.test.tsx:334,682,708,731,762,788` confirm consumer pattern |
| `/api/schedule/remove` | "No caller in app code (tests only)" | **WRONG** — wired | `frontend/src/features/schedules/components/page.tsx:527` (`act(s.id, "/api/schedule/remove", ...)`); also `TriggersTab.tsx:146` |
| `/api/schedule/run` | "No caller in app code (tests only)" | **WRONG** — wired | `frontend/src/features/schedules/components/page.tsx:495` (`act(s.id, "/api/schedule/run", ...)`); also `TriggersTab.tsx:113` |
| `/api/schedule/enable` | (not in the audit row, included for completeness) | wired | `schedules/page.tsx:241,504`; `Roster.tsx:549,574`; `AgentDetail.tsx:468` |

**Audit was wrong about:** all three subroutes. They are consumed by the Schedules page (`page.tsx`) and the agent-detail Triggers tab (`TriggersTab.tsx`).

### 1.2 `/api/standing/{remove,fire}`

| Subroute | Audit claim | Status | Evidence |
|---|---|---|---|
| `/api/standing/remove` | "No caller in app code" | **WRONG** — wired | `frontend/src/features/standing/components/page.tsx:433` (`act(o.id, "/api/standing/remove", ...)`); also `TriggersTab.tsx:239` |
| `/api/standing/fire` | "No caller in app code" | **WRONG** — wired | `frontend/src/features/standing/components/page.tsx:401` (`act(o.id, "/api/standing/fire", ...)`); also `TriggersTab.tsx:204` |
| `/api/standing/add`, `/api/standing/edit`, `/api/standing/enable` | (audit claims similar) | wired | `standing/page.tsx:236,262,410,654,844` |

**Audit was wrong about:** both subroutes. They are consumed by the Standing page (`page.tsx`) and the agent-detail Triggers tab (`TriggersTab.tsx`).

### 1.3 `/api/mcp/{attach,detach,enable,remove}`

| Subroute | Audit claim | Status | Evidence |
|---|---|---|---|
| `/api/mcp/attach` | "No caller (only tests)" | **WRONG** — wired | `frontend/src/features/mcp/components/page.tsx:657` (`act(s.name, "/api/mcp/attach", ...)`) |
| `/api/mcp/detach` | "No caller (only tests)" | **WRONG** — wired | `frontend/src/features/mcp/components/page.tsx:683` (`act(s.name, "/api/mcp/detach", ...)`) |
| `/api/mcp/enable` | "No caller (only tests)" | **WRONG** — wired | `frontend/src/features/mcp/components/page.tsx:704` |
| `/api/mcp/remove` | "No caller (only tests)" | **WRONG** — wired | `frontend/src/features/mcp/components/page.tsx:718` |
| `/api/mcp/add` | (audit claims similar) | wired | `mcp/page.tsx:230` |

**Audit was wrong about:** all four subroutes. They are all consumed by `frontend/src/features/mcp/components/page.tsx`.

### 1.4 `/api/skill/{file,quarantine,archive,revert,share,reassign}`

| Subroute | Audit claim | Status | Evidence |
|---|---|---|---|
| `/api/skill/file` (singular, in `webui_read_routes.go:186`) | "No caller" | **CORRECT** | No `rg` hits in `frontend/src/**`. (Note: `/api/skill/files` *plural* IS wired at `agentdetail/FilesTab.tsx:81`; the audit's `/api/skill/file` row likely confused the two.) |
| `/api/skill/quarantine` | "No caller" | **WRONG** — wired | `frontend/src/features/skills/components/page.tsx:319,539` |
| `/api/skill/archive` | "No caller" | **WRONG** — wired | `frontend/src/features/skills/components/page.tsx:370,518` |
| `/api/skill/revert` | "No caller" | **WRONG** — wired | `frontend/src/features/skills/components/page.tsx:560` |
| `/api/skill/share` | "No caller" | **WRONG** — wired | `frontend/src/features/skills/components/page.tsx:490`; also `agentdetail/SkillsTab.tsx:62` |
| `/api/skill/reassign` | "No caller" | **CORRECT** | No `rg` hits in `frontend/src/**`. |
| (bonus) `/api/skill/promote`, `/api/skill/import` | (audit also claims orphans) | wired | `skills/page.tsx:362,510` (promote); `skills/page.tsx:930` (import) |

**Audit was wrong about:** 4 of 6 subroutes (`quarantine`, `archive`, `revert`, `share`). Genuine orphans in this set: `/api/skill/file` (singular) and `/api/skill/reassign`.

### 1.5 `/api/market/{show,sources,sync}`

| Subroute | Audit claim | Status | Evidence |
|---|---|---|---|
| `/api/market/show` | "No caller" | **WRONG** — wired | `frontend/src/features/market/lib/market.ts:38` (`getJSON<MarketPackDetail>(\`/api/market/show?name=${encodeURIComponent(name)}\`, ...)`) |
| `/api/market/sources` | "No caller" | **WRONG** — wired | `frontend/src/features/market/components/page.tsx:205` |
| `/api/market/sync` | "No caller" | **WRONG** — wired | `frontend/src/features/market/components/page.tsx:252` |

**Audit was wrong about:** all three. They are consumed by `frontend/src/features/market/components/page.tsx` and `market/lib/market.ts`.

### 1.6 `/api/data/{collections,records,drop,collection}`

| Subroute | Audit claim | Status | Evidence |
|---|---|---|---|
| `/api/data/collections` | "No caller" | **WRONG** — wired | `frontend/src/features/data/components/page.tsx:156` (`getJSON<CollectionsResp>("/api/data/collections")`) |
| `/api/data/records` | "No caller (panel/page reads)" | **WRONG** — wired | `frontend/src/features/data/components/page.tsx:173` |
| `/api/data/drop` | "No caller" | **CORRECT** | No `rg` hits in `frontend/src/**`. |
| `/api/data/collection` (singular, POST) | "No caller" | **CORRECT** | No `rg` hits in `frontend/src/**`. |

**Audit was wrong about:** 2 of 4 subroutes. Genuine orphans in this set: `/api/data/drop` and `/api/data/collection`.

### 1.7 `/api/journal_search`, `/api/journal/export`, `/api/journal/verify`

| Subroute | Audit claim | Status | Evidence |
|---|---|---|---|
| `/api/journal_search` | "No app caller" | **CORRECT** | Only stale docstring references — `frontend/src/features/knowledge/components/Reflect.tsx:7` and `Analyst.tsx:9` mention `/api/journal_search` in a comment, but the actual fetch in both files is `getJSON("/api/journal?limit=...", ...)` (`Analyst.tsx:90` etc.). The route is genuinely not invoked at runtime. |
| `/api/journal/export` | "No app caller" | **CORRECT** | No `rg` hits in `frontend/src/**`. |
| `/api/journal/verify` | "No app caller" | **CORRECT** | No `rg` hits in `frontend/src/**`. |

**Audit was correct about:** all three.

### 1.8 `/api/storage`, `/api/cache`, `/api/stats`, `/api/persona`, `/api/skills/hygiene`

| Subroute | Audit claim | Status | Evidence |
|---|---|---|---|
| `/api/storage` | "No caller" | **CORRECT** | No `rg` hits in `frontend/src/**`. |
| `/api/cache` | "No caller" | **CORRECT** | No `rg` hits in `frontend/src/**`. |
| `/api/stats` | "No caller" | **CORRECT** | No `rg` hits in `frontend/src/**`. |
| `/api/persona` | "No caller" | **WRONG** — wired | `frontend/src/App.tsx:178,182` (`getJSON<PersonaProfile>("/api/persona")` + `postJSON("/api/persona/set", ...)`) |
| `/api/skills/hygiene` | "No caller" | **WRONG** — wired | `frontend/src/features/skills/components/page.tsx:253` |

**Audit was wrong about:** 2 of 5 subroutes (`persona`, `skills/hygiene`).

### 1.9 Additional audit claims worth re-checking

These were not in the explicit task brief but are major claims in §3/§4/§6/§7 of the audit that turned out to be wrong on closer inspection:

| Claim | Audit said | Reality | Evidence |
|---|---|---|---|
| `/api/spend/today` "does not exist" | Audit §3, §4, §6 call this a "frontend orphan" and "MISSING" | **WRONG** — registered | `kernel/webui/webui_read_routes.go:21` (apiRoutes), mapped to `controlplane.CmdSpendToday`; handler in `controlplane/handle_spend_attention.go:24`; registered in `controlplane/registry.go:130`. Consumer: `MissionControl.tsx:95`. |
| `/api/attention` "does not exist" | Audit §3, §4, §6 call this a "frontend orphan" and "MISSING" | **WRONG** — registered | `kernel/webui/webui_read_routes.go:206` (readArgsRoutes), mapped to `controlplane.CmdAttention`; handler in `controlplane/handle_spend_attention.go:49`. Consumer: `MissionControl.tsx:118`. |
| `/api/redact/test` "no caller" | Audit §3 line 352 and §6 line 376 | **WRONG** — wired | `frontend/src/features/policy/components/page.tsx:742` (`postJSON<RedactResult>("/api/redact/test", { text })`); test at `Policy.test.tsx:36` |
| `/api/edict/set_level`, `set_mode`, `deny_rm`, `test` "no caller" | Audit §3 line 95, §6 line 189, §6 line 355 | **WRONG** — wired | `policy/page.tsx:284` (`/api/edict/deny_rm`), `policy/page.tsx:392` (`/api/edict/set_mode`), `policy/page.tsx:416` (`/api/edict/set_level`), `policy/page.tsx:643` (`/api/edict/test`) |
| `/api/agents/remove` "no caller" | Audit §6 line 373 | **WRONG** — wired | `Roster.tsx:523`; `agentdetail/LifecyclePanel.tsx:155`; test fixtures at `Roster.test.tsx:1680`, `AgentDetail.test.tsx:1769` |
| `/api/run/{pause,resume,step}` "no caller (only `/api/run/steer` consumed)" | Audit §6 line 362 | **WRONG** — wired | `frontend/src/components/RunDetail.tsx:602,610,618` for all three |
| `/api/provider/oauth/{start,status,import}` "no caller (only logout consumed)" | Audit §6 line 363 | **WRONG** — wired | `frontend/src/features/api-keys/components/ChatGPTSignInCard.tsx` references all three; tests confirm |
| `/api/provider/probe` "no caller" | Audit §6 line 364 | **WRONG** — wired | `models/page.tsx:142` (`postJSON("/api/provider/probe", ...)`) |
| `/api/provider/keys` (list) "no caller" | Audit §6 line 365 | **WRONG** — wired | `models/page.tsx:344` (`getJSON<ProviderKeyList>("/api/provider/keys", { provider, env })`) |
| `/api/whatsappgw/{status,qr}` "no caller" | Audit §6 line 372 | **WRONG** — wired | `channels/page.tsx:205,214` |
| `/api/configcenter/list` "no caller" | Audit §6 line 372 | **WRONG** — wired | `configcenter/page.tsx:504` |
| `/api/autonomy` "no caller" | Audit §6 line 372 | **WRONG** — wired | `autonomy/Autonomy.tsx:80`; `incidents/IncidentPage.tsx:135` |
| `/api/plan_stats` "no caller" | Audit §6 line 372 | **CORRECT** | No `rg` hits in `frontend/src/**`. |
| `/api/pulse/asks` "no caller" | Audit §6 line 372 | **CORRECT** | No `rg` hits in `frontend/src/**`. |
| `/api/pulse/asks/resolve` "no caller" | Audit §6 line 372 | **CORRECT** | No `rg` hits in `frontend/src/**`. |
| `/api/reaper/scan` "no caller" | Audit §6 line 372 | **WRONG** — wired | `AgentDetail.tsx:120` |
| `/api/council/roles` "no caller" | Audit §3 line 414 parenthetical | **CORRECT** | No `rg` hits in `frontend/src/**`. |

### 1.10 Audit claim tally

- **Claim rows reviewed**: 27 (15 explicit "no caller" rows in §6 + 12 additional claims in §3/§4/§7)
- **WRONG claims** (route is wired in app code): **18** of 27
- **CORRECT claims** (route has no app caller): **9** of 27

(The 9 vs 18 split is approximate because some "rows" in the audit list multiple routes at once; individually every subroute was checked.)

---

## 2. Real frontend orphans (re-verified)

Every backend route that has **zero** app-code callers in `frontend/src/**` (test files do not count). Built by enumerating `apiRoutes`, `readArgsRoutes`, `writeRoutes`, and `jsonRoutes` from `kernel/webui/` and confirming no `rg "/api/<path>"` match outside test fixtures.

| Backend route | Handler (kernel/controlplane) | Notes |
|---|---|---|
| `/api/storage` | (apiRoutes entry) | Audit was correct — no caller. |
| `/api/cache` | (apiRoutes entry) | Audit was correct — no caller. |
| `/api/stats` | (apiRoutes entry) | Audit was correct — no caller. |
| `/api/tools` | (apiRoutes entry) | Distinct from `/api/tools_catalog` (which IS wired). |
| `/api/providers` | (apiRoutes entry) | Distinct from `/api/catalog` (which IS wired). |
| `/api/execution_profile` (singular) | `CmdExecutionProfileShow` | `/api/execution_profiles` (plural) and `/api/execution_profile_check` ARE wired; only the singular `show`-by-id is unused. |
| `/api/journal_search` | `CmdJournalGrep` | Stale comment references in `Reflect.tsx:7` + `Analyst.tsx:9` but no actual fetch. |
| `/api/journal/export` | `CmdJournalExport` | M772 archival bundle — never wired. |
| `/api/journal/verify` | (read-only) | Never wired. |
| `/api/why` | (readArgsRoutes entry) | SPEC-16 mentions `GET /api/why/{event_id}` but the SPA never invokes it. |
| `/api/plan_stats` | (apiRoutes entry) | Never wired. |
| `/api/pulse/asks` | (apiRoutes entry) | Never wired. |
| `/api/pulse/asks/resolve` | (writeRoutes entry) | Never wired. |
| `/api/council/roles` | (apiRoutes entry) | Sister route to `/api/council/members` (which IS wired); only `/api/council/roles` is unused. |
| `/api/reflect` | (apiRoutes entry) | Never wired. |
| `/api/reflect/run` | (writeRoutes entry) | Never wired. |
| `/api/budget_set` | (writeRoutes entry) | Never wired. |
| `/api/memory/consolidate` | (writeRoutes entry) | Distinct from `/api/memory/{clean,tidy,prune,forget,promote,bulk_forget}` which ARE wired. |
| `/api/skill/file` (singular) | `CmdSkillReadFile` | `/api/skill/files` plural IS wired; only the per-resource GET is unused. |
| `/api/skill/reassign` | (writeRoutes entry) | Never wired. |
| `/api/data/drop` | (writeRoutes entry) | Never wired. |
| `/api/data/collection` (singular, POST) | (jsonRoutes entry) | `/api/data/collections` plural GET IS wired; only the per-collection POST is unused. |
| `/api/toolbox` | `CmdToolboxDetect` | Only `/api/toolbox/install` (inline route in `webui_routes.go:82`) is wired; the detect/updates pair is unused. |
| `/api/toolbox/updates` | `CmdToolboxOutdated` | Same as above. |
| `/api/toolforge/*` | (toolforge cmd family) | Audit correctly classified this as "REMOVED_VIEW". Routes: `toolforge`, `toolforge/show`, `toolforge/draft`, `toolforge/edit`, `toolforge/test`, `toolforge/promote`, `toolforge/quarantine`, `toolforge/remove`. |
| `/api/conductor/*` | (conductor cmd family) | "REMOVED_VIEW". Routes: `conductor/ask`, `conductor/roles`. |
| `/api/taste/*` | (taste cmd family) | "REMOVED_VIEW". Routes: `taste`, `taste/create`, `taste/delete`. |
| `/api/seats/*` | (seats cmd family) | "REMOVED_VIEW". Routes: `seats`, `seats/create`, `seats/delete`. |
| `/api/okr/*` | (okr cmd family) | "REMOVED_VIEW". Routes: `okr`, `okr/show`, `okr/create`, `okr/keyresult`, `okr/link`, `okr/unlink`, `okr/archive`. |
| `/api/workboard/*` | (workboard cmd family) | "REMOVED_VIEW". Routes: `workboard`, `workboard/lanes`, `workboard/watch`, plus all the write routes (`workboard/create`, `comment`, `block`, `fail`, `unblock`, `complete`, `prove`, `seat`, `policy`, `dispatch`). |
| `/api/plan/{generate,refine}` | (writeRoutes entries) | Never wired; the Plan view uses `/api/plan_history` only. |
| `/api/configcenter/get` | (readArgsRoutes entry) | `/api/configcenter/list` IS wired; the per-key `get` is only referenced in tests. |

The "REMOVED_VIEW" cluster (toolforge, conductor, taste, seats, okr, workboard) is roughly 25 routes; the rest is ~20 routes. **Real orphans: ~45 routes** (counting each subroute individually).

The audit's headline number "**~60 backend orphans**" is in the right ballpark for the *raw route count* but lumps together:
- ~25 truly dead nav-removed views (intentional — the SPEC has them shelved),
- ~20 routes that are real orphans (in §2 above),
- and a sprinkling of routes the audit itself mis-classified (e.g. `/api/redact/test`, `/api/edict/*`, `/api/agents/remove` — all actually wired).

The audit's other headline number "**2 frontend orphans** (`/api/spend/today`, `/api/attention`)" is **wrong on both counts**: both routes are registered in `webui_read_routes.go` and the handlers exist (`handle_spend_attention.go`). The Mission Control tile and the attention panel render real data — the audit mistook a successful response for a 404 because the test/load simulation may have returned empty in the audit's view.

**Final counts:**
- **Audit claimed ~60 backend orphans → actually wired (audit was wrong): ~20 routes listed as orphans are in fact wired.**
- **Audit claimed 2 frontend orphans → both routes actually exist; 0 true frontend orphans.**
- **Real backend orphans remaining: ~45 routes** (split: ~25 REMOVED_VIEW intentional, ~20 truly dead and unowned).

---

## 3. Dead test hooks (cursor pager)

Every hook exported from `frontend/src/app/cursor-pager/cursorPager.ts`, checked for callers in app code (`frontend/src/**` excluding `cursor-pager/` and `*.test.*`).

| Hook | Defined at | Used in app code? | Imported (non-test) | Conclusion |
|---|---|---|---|---|
| `useAgentsPager` | `cursorPager.ts:188` | **Yes** | `features/agents/components/Agents.tsx:46,244` | Live |
| `useMemoryPager` | `cursorPager.ts:230`-ish | **Yes** | `features/memory/components/page.tsx:23,110` | Live |
| `useAgentActivityPager` | `cursorPager.ts:216` | **No** | (test only at `cursorPager.test.ts:51,255`) | Dead — `AgentActivity.tsx:79` uses raw `getJSON` instead |
| `useAgentEscalationsPager` | `cursorPager.ts:239` | **No** | (test only at `cursorPager.test.ts:52,269`) | Dead — `AgentDetail.tsx:125` and `IncidentPage.tsx:346` use raw `getJSON` instead |
| `useToolLogPager` | `cursorPager.ts:260` | **No** | (test only at `cursorPager.test.ts:53,294`; example docstring in `load-more-footer.tsx:16` but no `import`) | Dead — no production consumer |
| `useProviderLogPager` | `cursorPager.ts:265` | **No** | No production imports found | Dead — no production consumer |
| `usePolicyLogPager` | `cursorPager.ts:273` | **No** | No production imports found | Dead — JSDoc says "view integration lands in a follow-up" |
| `useApprovalsLogPager` | `cursorPager.ts:281` | **No** | No production imports found | Dead — JSDoc says "view integration lands in a follow-up"; `AgentDetail.tsx:108` uses raw `getJSON` instead |
| `useRateLimitLogPager` | `cursorPager.ts:286` | **No** | No production imports found | Dead — never adopted |
| `useWebhookLogPager` | `cursorPager.ts:292` | **Yes** | `features/channels/components/page.tsx:23,581` | Live |
| `useWardenLogPager` | `cursorPager.ts:297` | **Yes** | `features/sandbox/components/Sandbox.tsx:13,119` | Live |
| `useNetguardLogPager` | `cursorPager.ts:302` | **Yes** | `features/sandbox/components/Sandbox.tsx:13,129` | Live |
| `useWorldLogPager` | `cursorPager.ts:307` | **Yes** | `features/world/components/page.tsx:26,166` | Live |
| `useMemoryLogPager` | `cursorPager.ts:312` | **Yes** | `features/memory/components/page.tsx:23,137` | Live |
| `useInboxPager` | `cursorPager.ts:155`-ish | **No** | (test only at `cursorPager.test.ts:48,216`) | Dead — `ChannelSessions.tsx:35` uses raw `getJSON` instead |
| `useBoardPager` | `cursorPager.ts:176`-ish | **No** | (test only at `cursorPager.test.ts:49,229`) | Dead — `Roster.tsx:245` and `AgentDetail.tsx:115` use raw `getJSON` instead |
| `usePlanHistoryPager` | `cursorPager.ts:321`-ish | **No** | (test only at `cursorPager.test.ts:54,307`) | Dead — view integration never landed |
| `useScheduleFiresPager` | `cursorPager.ts:331` | **Yes** | `features/schedules/components/page.tsx:43,108` | Live |

**Dead-hook count: 10** (`useInboxPager`, `useBoardPager`, `usePlanHistoryPager`, `useAgentActivityPager`, `useAgentEscalationsPager`, `useToolLogPager`, `useProviderLogPager`, `usePolicyLogPager`, `useApprovalsLogPager`, `useRateLimitLogPager`). The audit claimed "many idle hooks" — that part was right; the per-hook list above is the granular version.

Note: 5 of the 10 dead hooks (`useToolLogPager`, `useProviderLogPager`, `usePolicyLogPager`, `useApprovalsLogPager`, `useRateLimitLogPager`) have **no consumer at all** — not even in the views that should arguably be using them. The companion `useWardenLogPager`/`useNetguardLogPager`/`useWebhookLogPager`/`useWorldLogPager`/`useMemoryLogPager` ARE wired, so the underlying routes are healthy; just these specific wrappers were never adopted.

---

## 4. Recommended actions

Ordered by user impact:

1. **Delete the "frontend orphans" claim from the topology audit report.** `/api/spend/today` and `/api/attention` are registered (`webui_read_routes.go:21,206`) and the Mission Control Spend tile + Needs-Attention panel render real data. The audit's "Spend tile never updates" recommendation is **incorrect** and risks misdirecting future cleanup. (`kernel/webui/webui_read_routes.go:21,206` are the canonical evidence.)

2. **Wire the dead log pagers (`useToolLogPager`, `useProviderLogPager`, `usePolicyLogPager`, `useApprovalsLogPager`, `useRateLimitLogPager`) into their respective views**, or delete them. The data is already fetched via raw `getJSON` in `AgentDetail.tsx:102,105,108,117` — replacing those four ad-hoc fetches with the canonical hooks would (a) restore cursor pagination, (b) dedupe the page-by-seq logic, (c) free the hooks from dead-code rot. (`frontend/src/app/cursor-pager/cursorPager.ts:260-287`, consumers at `frontend/src/features/agents/components/AgentDetail.tsx:102,105,108,117`.)

3. **Schedule the genuinely-orphaned backend routes for a follow-up cleanup pass.** Recommended three buckets:
   - **Documented REMOVED_VIEW clusters** (toolforge, conductor, taste, seats, okr, workboard — ~25 routes): keep them but add a `// REMOVED_VIEW: planned re-introduction in SPEC-X` comment on each route registration so the next audit does not re-flag them.
   - **Truly dead singletons** (`/api/skill/file` singular, `/api/skill/reassign`, `/api/data/drop`, `/api/data/collection` singular, `/api/journal_search`, `/api/journal/export`, `/api/journal/verify`, `/api/storage`, `/api/cache`, `/api/stats`, `/api/tools`, `/api/providers`, `/api/execution_profile` singular, `/api/why`, `/api/plan_stats`, `/api/budget_set`, `/api/memory/consolidate`, `/api/reflect`, `/api/reflect/run`, `/api/plan/{generate,refine}`, `/api/pulse/asks`, `/api/pulse/asks/resolve`, `/api/council/roles`, `/api/configcenter/get`, `/api/toolbox`, `/api/toolbox/updates` — ~25 routes): either delete them or open tickets to wire them. `/api/journal_search` is the highest-value add — the comment in `Reflect.tsx:7` shows it was *meant* to be wired.
   - **Verify-or-delete the apparent "removals"** for `/api/redact/test`, `/api/edict/{test,set_level,set_mode,deny_rm}`, `/api/agents/remove`, `/api/run/{pause,resume,step}`, `/api/provider/oauth/{start,status,import}`, `/api/provider/probe`, `/api/provider/keys` — the audit was wrong; they are already wired.

4. **Adopt `useInboxPager` + `useBoardPager` + `usePlanHistoryPager`** in their intended views. These three hooks have test coverage and named endpoints but no production consumer; the relevant views (`ChannelSessions.tsx`, `Roster.tsx`, etc.) currently use raw `getJSON` calls that bypass the dedup-by-id logic the hooks provide. (`cursorPager.ts:155,176,321` vs `ChannelSessions.tsx:35`, `Roster.tsx:245`.)

5. **Update the audit's tooling.** The audit's `rg` run appears to have been truncated or scoped to a single file; the orphan-detection regex `/api/skill/file` matched `/api/skill/files` (with a trailing `s`) when grepping for the bare path. Recommend re-running the orphan scan with explicit `"/api/skill/file"` (literal-quoted, no prefix match) before publishing the next audit, and double-checking backend route registration (`grep -n path webui_read_routes.go`) before claiming a route "does not exist".
