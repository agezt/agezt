# Frontend Refactor Sprint — Day 1 Discovery (2026-09-10)

> 30-day plan (Yol B tarzı, Go kernel/runtime sprint'inin analogu).
> Wire contract (HTTP surface, `agezt-contract.jsonc`) değişmez;
> görsel/etkileşim davranışı sıfır regresyon. Saf internal refactor.

## 1. Mevcut yapı (Day 0)

```
frontend/src/
  @types/        1 file    (vite-env.d.ts)
  components/  127 files   (yeniden kullanılabilir UI bileşenleri)
    agentdetail/  15 files  (AgentDetail zaten modüler; iyi örnek)
    ui/          30 files  (UI primitives — button, dialog, vb.)
  lib/         144 files   (iş mantığı + utilities — DÜZ LİSTE)
  views/       161 files   (sayfa/ekran — çoğu 400-1500 LOC)
    Chat/        12 files
    roster/       7 files
    schedules/    1 file
  App.tsx                579 lines  (tüm `lib/`'i içeriyor)
  nav.tsx                800 lines  (navigation hub)
  main.tsx                (entry)
  vite-env.d.ts
```

**Toplam:** 441 dosya (TS/TSX).

### 1.1 God dosya haritası (≥400 LOC, non-test)

58 dosya. En büyük 12:

| LOC | Dosya | Kategori |
|---:|---|---|
| 2795 | `lib/help.ts` | Yardım/dokümantasyon |
| 1903 | `lib/agentdetail.ts` | Agent iş mantığı + bileşen |
| 1533 | `views/Workflows.tsx` | Sayfa |
| 1467 | `views/Schedules.tsx` | Sayfa |
| 1467 | `views/Roster.tsx` | Sayfa |
| 1323 | `views/IncidentPage.tsx` | Sayfa |
| 1287 | `views/ExecutionProfiles.tsx` | Sayfa |
| 1202 | `views/Board.tsx` | Sayfa |
| 1020 | `views/Memory.tsx` | Sayfa |
| 1019 | `views/Standing.tsx` | Sayfa |
| 1007 | `views/ConfigCenter.tsx` | Sayfa |
| 997 | `views/Skills.tsx` | Sayfa |

### 1.2 `lib/` cluster analizi (top 6)

| Cluster | Count | Tahmini feature |
|---|---:|---|
| `agen*` | 12 | `features/agents/` |
| `voic*` | 9 | `features/voice/` |
| `inci*` | 6 | `features/incidents/` |
| `chat*` | 5 | `features/chat/` |
| `event*` | 4 | `lib/events/` (cross-cutting) |
| `mark*` | 4 | `lib/markdown/` (cross-cutting) |

### 1.3 Mevcut iyi örnekler (zaten carve-out edilmiş)

- `components/agentdetail/` (15 dosya) — AgentDetail modüler; **şablon olarak kullanılacak**
- `views/Chat/` (12 dosya) — Chat feature organize
- `views/roster/` (7 dosya) — Roster partial

### 1.4 Cross-cutting concern'ler (tüm `lib/`'te inline, çıkarılması gerek)

- `lib/api.ts` — HTTP client (`postAction`, `getJSON`); `features/`'e taşınmayacak, `@/lib/api/` core'da kalır
- `lib/events.ts` — global event hook (`useEvents`); tüm feature'ler kullanıyor
- `lib/theme.ts` / `lib/appearance.ts` — görsel tema/state
- `lib/nav.ts` — navigation routing (`goToView`, `viewFromHash`)
- `lib/format.ts` — formatters (Bytes, Time, vb.); cmd/agt/format.go'nun TS karşılığı
- `lib/help.ts` (2795 satır) — per-topic help modüllerine bölünecek
- `lib/commands.ts` — command palette
- `lib/globalActivity.ts` — global activity stream
- `lib/chatStore.ts` — chat state (cross-cutting ama Chat feature'ine taşınmayacak)

## 2. Hedef yapı (Day 30)

```
frontend/src/
  @types/                    # vite-env + custom .d.ts
  app/                       # cross-cutting core (App.tsx, nav.tsx, main.tsx)
    api/                     # HTTP client + SDK
    events/                  # useEvents global hook
    theme/                   # theme + appearance
    nav/                     # navigation primitives (goToView, viewFromHash)
    format/                  # Bytes / Time / Duration formatters
    commands/                # command palette
    help/                    # per-topic help modules
    activity/                # global activity stream
  components/                # reusable UI primitives (button, dialog, etc.)
    ui/                      # (existing)
    agentdetail/             # (existing — template)
  features/                  # DOMAIN-OWNED bundles
    agents/                  # agent detail, roster, agentnav, agentlive, ...
    voice/                   # voice, voiceSession, voiceStatus, voiceCatalog
    incidents/               # incidents, incidentevents, incidentnav
    chat/                    # chat, chatStore, chains (cross-cutting core stays in app/)
    workflows/               # Workflows.tsx + helpers
    schedules/               # Schedules.tsx + cron logic
    execution-profiles/      # ExecutionProfiles.tsx
    workboard/               # Board.tsx
    memory/                  # Memory.tsx
    standing/                # Standing.tsx
    configcenter/            # ConfigCenter.tsx
    skills/                  # Skills.tsx
    market/                  # Market.tsx
    setup/                   # Setup.tsx
    autonomy/                # Autonomy.tsx
    data/                    # Data.tsx
    mcp/                     # Mcp.tsx
    models/                  # Models.tsx
    artifacts/               # Artifacts.tsx
    channels/                # Channels.tsx
    policy/                  # Policy.tsx
    world/                   # World.tsx
    council/                 # Council.tsx
    overseer/                # Overseer.tsx
    sandbox/                 # Sandbox.tsx
    voice-setup/             # VoiceSetup.tsx
    incidents-page/          # IncidentPage.tsx
    connections/             # Connections.tsx
    toolforge/               # Toolforge.tsx
    health/                  # Health.tsx
    runs/                    # Runs.tsx
    dashboard/               # Dashboard.tsx
    jarvis/                  # Jarvis.tsx
    chat-message/            # Chat/message.tsx (extract from Chat/)
  views/                     # THIN pages (<200 LOC) — just routing + layout
    index.tsx                # NAV table — maps views to features
    EachView.tsx             # one-liner: import + render feature
  App.tsx                    # entry — only imports from `app/`
  nav.tsx                    # entry — only imports from `app/`
  main.tsx                   # entry
```

## 3. Carve-out şablonu (Go'daki `Manager`/`Accessor`/`Runner` analogu)

Her `features/{name}/` için:

```
features/agents/
  index.ts               # barrel — public surface
  components/            # co-located UI
    AgentDetail.tsx
    Roster.tsx
    ...
  hooks/                 # custom hooks (TS karşılığı: Go'daki Manager/Accessor)
    useAgents.ts          # public hook (analog: kernelAPI.NewCorrelation())
    useAgentStream.ts     # live event stream
  lib/                   # private business logic (TS karşılığı: package-level helpers)
    compute.ts            # pure functions
    format.ts             # feature-specific formatters
  types.ts               # shared types for this feature
  test/                  # co-located test helpers + integration tests
  README.md              # feature owner + entry points
```

**Public surface sadece:** `index.ts` export'ları + `hooks/` içindeki hook'lar.
**`App.tsx`/`nav.tsx`:** sadece `features/{name}/index.ts`'i import eder (Go'daki `kernelapi.New()` gibi).

## 4. Faz planı (30 gün)

### Phase 1 — Discovery (Day 1-2) ← BURADA
- [x] God dosya haritası (58 dosya ≥400 LOC)
- [x] `lib/` cluster analizi (top 6 cluster)
- [x] Mevcut iyi örnekler (`components/agentdetail/` template)
- [ ] Cross-cutting concern final listesi
- [ ] Per-feature dependency graph (kim kimi import ediyor)

### Phase 2 — Foundation (Day 3-7)
- Day 3: `app/` iskeleti (api, events, theme, nav, format, commands, activity, help)
- Day 4: `features/` iskeleti (her yeni feature için `index.ts` + `hooks/` + `types.ts` şablonu)
- Day 5: `lib/help.ts` (2795) → `app/help/` modüllerine böl
- Day 6: CI gate (knip deadcode, vitest, typecheck — frontend sprint boyunca)
- Day 7: Documentation (`docs/FRONTEND-REFACTOR-PLAN.md` güncelleme + per-feature README'ler)

### Phase 3 — Feature carve-outs (Day 8-25)
Sırayla (önce küçük, sonra büyük):
- Day 8-9: `features/voice/` (~9 lib dosyası + VoiceSetup.tsx + Voice.tsx)
- Day 10-11: `features/chat-message/` (Chat/message.tsx çıkarma)
- Day 12-13: `features/incidents/` (6 lib + IncidentPage.tsx)
- Day 14-16: `features/agents/` (12 lib + Roster.tsx + AgentDetail.tsx — en büyük)
- Day 17-18: `features/workflows/` (Workflows.tsx + helpers)
- Day 19-20: `features/schedules/` (Schedules.tsx + cron logic)
- Day 21-22: `features/execution-profiles/` + `features/workboard/`
- Day 23: `features/memory/` + `features/standing/` + `features/configcenter/`
- Day 24: `features/skills/` + `features/market/` + `features/setup/`
- Day 25: kalan 17 feature (autonomy, data, mcp, models, artifacts, channels, policy, world, council, overseer, sandbox, connections, toolforge, health, runs, dashboard, jarvis)

### Phase 4 — Integration (Day 26-30)
- Day 26: `views/` thin page'lere indirgenir (her biri <200 LOC)
- Day 27: `App.tsx` + `nav.tsx` refactor (sadece `app/` + `features/index.ts`'i import)
- Day 28: knip deadcode check temiz, vitest full yeşil, typecheck yeşil
- Day 29: Bundle size before/after ölçümü + perf regression yok
- Day 30: Final PR + docs

## 5. Acceptance (her feature için)

- [ ] `features/{name}/index.ts` barrel exports + sadece public surface
- [ ] Tüm eski `lib/{name}*` dosyaları silinmiş (knip deadcode onaylar)
- [ ] Tüm eski `views/{Name}.tsx` `views/{name}/index.tsx` thin-page'e indirgenmiş
- [ ] `App.tsx` + `nav.tsx` doğrudan `features/{name}/index.ts`'i import ediyor
- [ ] Co-located tests (`*.test.tsx`) yeşil
- [ ] TypeScript strict mode (`tsc --noEmit`) yeşil
- [ ] knip deadcode yeşil
- [ ] Görsel davranış değişmedi (smoke test + Playwright e2e)
- [ ] Per-feature README.md güncel

## 6. Açık öğeler (sprint başında)

- Cross-cutting `lib/` hangi kısmı `app/`'e, hangi kısmi `features/`'e gidecek? (Day 1-2 son karar)
- Feature ownership — takım atamaları (sprint'te tek geliştirici modu, kendim yapacağım)
- Test stratejisi — co-located vs central (co-located tercih, central test/ utilities)
- Bundle splitting — Vite code-splitting (her feature lazy load olabilir; Day 25'te değerlendirilir)

## 7. Day 2 planı

- Per-feature dependency graph çıkarmak (kim kimi import ediyor — `lib/agents.ts` kimlere bağımlı, vb.)
- `lib/help.ts` (2795 satır) içindeki fonksiyonları kategorize etmek (agent/workspace/commands/...)
- Cross-cutting concern final listesi + `app/`'e taşınacak dosyaların envanteri

## 8. İlk commit beklentisi (Day 7 sonu)

- `app/` iskeleti + 1 feature (`features/voice/` veya `features/chat-message/`) tamamen taşınmış
- Testler yeşil, knip temiz, typecheck temiz
- Bir kez review/PR akışı test edilmiş
- Kalan 27 feature için kalıp net

## 9. Sprint ilerlemesi (commit başına)

- **Day 1-2** (plan): `docs/FRONTEND-REFACTOR-PLAN.md` — 441 dosya, 58 god file, top-6 lib cluster analizi, hedef layout.
- **Day 3** (`030ecb34`): `lib/utils.ts` (104) + `lib/api.ts` (152) → `app/utils/` + `app/api/`.
- **Day 4** (`d0db493d`): `lib/events.tsx` + `lib/format.ts` + `lib/cursorPager.ts` + `lib/export.ts` → `app/{events,format,cursor-pager,export}/`.
- **Day 5a** (`451a1033`): `lib/help.ts` (2795) → `app/help/` (5 dosya; topic split Day 5b'de).
- **Day 6** (`db31cc8f`): `features/voice/` — ilk feature carve-out (29 dosya, 87 test).
- **Day 7** (`64b0b28f`): `features/incidents/` (31 dosya, 27 test).
- **Day 8** (`5fc5af85`): `features/council/` (10 dosya, 14 test).
- **Day 9** (`7e17d0e3`): `features/runs/` — 5 lib + 1 sayfa + 2 test taşıma, 23 dosyada import path bulk-replace, doc-block yorumları güncel.
  - Yeni: `frontend/src/features/runs/{index,types}.ts` + `components/{Runs.tsx, Runs.test.tsx, Runs.pager.test.tsx}` + `lib/{rundetail.ts, rundetail.test.ts, runfocus.ts, runfocus.test.ts}` (9 dosya).
  - Silinen: `frontend/src/lib/{rundetail,runfocus}{,.test}.ts` + `frontend/src/views/Runs{,.test,.pager.test}.tsx` (7 dosya).
  - Güncellenen: 23 dosyada import path rewrite (`@/lib/{rundetail,runfocus}` → `@/features/runs/lib/...`, `@/views/Runs` → `@/features/runs/components/Runs`).
  - Doğrulama: `tsc --noEmit` temiz, `vitest run` 191 dosya / 1628 test yeşil (34.4s).
  - Yardımcı: `scripts/dev/rewrite-runs-imports.py` (gelecek benzer bulk-rewrite'ler için şablon).
  - Kalan: `views/Runs.tsx` → `features/runs/components/Runs.tsx` göçü tamam; `RunDetail.tsx` (841 LOC) hâlâ `components/`'te paylaşılan bileşen — runs feature'ın dış yüzeyine bağımlı birden fazla sayfa (Dashboard, Mission, Inbox, Overseer, Replay) var; Runs sayfası `RunDetail`'ı doğrudan kullanmıyor (kendi tablo render'ı), bu yüzden şimdilik `components/RunDetail.tsx` ortak yerde kalıyor.
- **Day 10** (`ee7267da`): `features/agents/` — en büyük feature carve-out (Day 10/11 split).
  - **Taşınan (40 dosya)**:
    - `lib/agent*.{ts,test.ts}` (12 dosya, 4468 satır) → `features/agents/lib/` (6 lib çifti: agent, agentactivity, agentdetail, agentlive, agentnav, agentrepair)
    - `components/AgentDetail.{tsx,test.tsx}` (2, 3712) + `AgentActivity.{tsx,test.tsx}` (2, 385) + `AgentRepair.{tsx,test.tsx}` (2, 575) → `features/agents/components/`
    - `components/agentdetail/*` (16 dosya, 211K byte) → `features/agents/components/agentdetail/`
    - `views/Agents.{tsx,test.tsx}` (2, 927) + `views/ACPAgents.{tsx,test.tsx}` (2, 312) + `views/AgentPage.{tsx,test.tsx}` (2, 289) → `features/agents/components/`
  - **Silinen**: 28 dosya (12 lib + 6 component + 16 subdir dosyası + 6 view/test çifti)
  - **Yeni**: `features/agents/{index,types}.ts` (barrel + type re-exports)
  - **Güncellenen**: 57 dosyada import path rewrite (43 string match + 14 regex subpath match)
    - lib: `@/lib/agent*` → `@/features/agents/lib/agent*` (6 pattern)
    - components: `@/components/AgentDetail|AgentActivity|AgentRepair` → `@/features/agents/components/...` (3 pattern)
    - components/agentdetail: regex ile `@/components/agentdetail(/...)?` → `@/features/agents/components/agentdetail(/...)?`
    - views: `@/views/Agents|ACPAgents|AgentPage` → `@/features/agents/components/...` (3 pattern)
  - **Çapraz feature kullanıcılar**: `nav.tsx`, `App.tsx`, `components/AgentAvatar.tsx` (avatar shared, agent.ts'ten hue+initials kullanır), `components/Fleet.tsx` + `FleetNowBar.tsx` (openAgent + summarizeAgentRuntimeStatus), `features/incidents/components/IncidentPage.tsx` (AgentRepair + openAgent + agentdetail types) — hepsi rewrite edildi.
  - **Cross-feature component'ler yerinde**: `AgentAvatar` (8 kullanıcı: chat, fleet, fleetnowbar, overseer, message, agentdetail, agents, roster) ve `AgentPicker` (3 kullanıcı: chat, standing, kendi test'i) `components/`'te kalıyor.
  - **Doğrulama**: `tsc --noEmit` temiz, `vitest run` 191 dosya / 1628 test yeşil (40.2s).
  - **Yardımcı**: `scripts/dev/rewrite-agents-imports.py` — regex round-2 (agentdetail subpath'leri) dahil 2-pass tasarım.
  - **Kalan (Day 11)**: `views/Roster.tsx` (1499 satır) + `views/roster/*` (7 dosya, 1720 satır) — Roster'ın import'ları forward-compatible olarak bu commit'te güncellendi (`@/lib/agentdetail` → `@/features/agents/lib/agentdetail`); Day 11'de `views/Roster.tsx` → `features/agents/components/Roster.tsx` taşıması + bulk rewrite yapılacak.
- **Day 11** (`a83732a0`): `features/agents/components/{Roster,roster/*}` — agents feature'ı tamamlandı.
  - **Taşınan (9 dosya, 5043 satır)**:
    - `views/Roster.tsx` (1499 satır) + `views/Roster.test.tsx` (1824 satır) → `features/agents/components/`
    - `views/roster/*` (7 dosya, 1720 satır: cards, filters, form, guardians, passports, removal, shared) → `features/agents/components/roster/`
  - **Yeni barrel surface**: `features/agents/index.ts` artık `Roster`'ı da export ediyor (4 sayfa görünümü: Agents, ACPAgents, AgentPage, Roster)
  - **Güncellenen**: 21 dosyada `@/views/Roster` → `@/features/agents/components/Roster` rewrite
    - `nav.tsx` (lazy import), `views/Wizards.tsx` (NewAgentForm + usdToMc)
    - `features/agents/components/AgentDetail.tsx` (multi-import, en büyük consumer — 5 sembol: agentEnableToast, agentRetireToast, agentReviveToast, agentSchedulePressurePassport, guardianQuietPolicyPayload, + 4 tip)
    - `features/agents/components/agentdetail/*` (12 dosya — CapabilityPanel, LifecyclePanel, Overview, comms, capability, lifecycle, tasks, shared, MindTab, ModelTab, DiagTab, TriggersTab — hepsi `AgentProfile` tipini kullanıyor)
    - `features/agents/components/{Agents,AgentPage,AgentRepair,AgentRepair.test}.tsx` (4 dosya)
    - `features/incidents/components/IncidentPage.tsx` (cross-feature — AgentProfile tipi)
    - `features/agents/components/Roster.test.tsx` (self-import, 35+ sembol)
  - **Doğrulama**: `tsc --noEmit` temiz, `vitest run` 191 dosya / 1628 test yeşil (34.75s)
  - **Yardımcı**: `scripts/dev/rewrite-roster-imports.py`
  - **Agents feature tamamlandı**: 49 dosya, ~11K satır (12 lib + 28 component + 9 Roster/roster + 2 barrel). 5. feature, en büyüğü.
  - **Boş kalan**: `views/roster/` (boş subdir, 7 dosya taşındı) — git'te untracked olarak kalacak, ileride temizlenir.
- **Day 12** (`52aa777f`): `features/workflows/` — 6. feature, orta boy (~2600 satır, 5 dosya).
  - **Taşınan (5 dosya, 2628 satır)**:
    - `lib/chains.{ts,test.ts}` (2, 152 satır) → `features/workflows/lib/`
    - `views/Workflows.tsx` (1597 satır — god file) + `views/Workflows.test.tsx` (478 satır) → `features/workflows/components/`
    - `views/Chains.tsx` (401 satır) → `features/workflows/components/`
  - **Yeni**: `features/workflows/{index,types}.ts` (barrel + 8 tip re-export + 9 helper re-export)
  - **Güncellenen**: 9 dosyada import path rewrite
    - `lib/chains` → `@/features/workflows/lib/chains`: 4 consumer (components/ModelChip, components/ModelPicker, features/agents/.../ModelTab, features/agents/.../roster/form)
    - `views/Workflows` → `@/features/workflows/components/Workflows`: 3 (self test, nav lazy, views/missing-imports smoke test)
    - `views/Chains` → `@/features/workflows/components/Chains`: 3 (self, nav lazy, views/missing-imports smoke test)
  - **Public surface (workflows barrel)**: 2 sayfa (Workflows, Chains) + 9 helper/component re-export (CopilotPanel, RunsDrawer, toFlow, fromFlow, portsForNode, summarize, workflowChainKind, workflowRunSourceLabel, runToStatus) + 9 tip (Wf, WfNode, WfEdge, WfSettings, WorkflowChainKind, WfTemplate, WfRunNodeEvent, WfRun, ChainUsage)
  - **Cross-feature consumers** (4): ModelChip + ModelPicker (shared model widget'lar), features/agents/ModelTab + features/agents/roster/form (chain-ref UI) — hepsi rewrite edildi
  - **God file riski**: `Workflows.tsx` 1597 satır — 8 tip + 5 helper + 4 sub-component (WfNodeView, NodePanel, CopilotPanel, RunsDrawer) + Workflows() default + LastRun/freshID/TemplatePicker glue. Plan'a göre Day 26+ integration'da bölünecek (flow.ts, nodes.tsx, panels.tsx, CopilotPanel.tsx, RunsDrawer.tsx, Workflows.tsx); bugün sadece move.
  - **Doğrulama**: `tsc --noEmit` temiz, `vitest run` 191 dosya / 1628 test yeşil (36.6s)
  - **Yardımcı**: `scripts/dev/rewrite-workflows-imports.py`
- **Day 13** (`63ae84c9`): `features/schedules/` + `features/execution-profiles/` — 7. ve 8. feature, küçük + orta boy (toplam 3984 satır, 6 dosya).
  - **Taşınan (6 dosya, 3984 satır)**:
    - `views/Schedules.tsx` (1518 — god file) + `views/Schedules.test.tsx` (1234) → `features/schedules/components/`
    - `views/schedules/shared.ts` (616) → `features/schedules/lib/`
    - `views/ExecutionProfiles.tsx` (1343) + `views/ExecutionProfiles.test.tsx` (290) → `features/execution-profiles/components/`
  - **Yeni**: `features/schedules/{index,types}.ts` + `features/execution-profiles/{index,types}.ts` (4 barrel dosyası)
  - **Güncellenen**: 5 dosyada import path rewrite
    - `@/views/Schedules` → `@/features/schedules/components/Schedules`: 3 (nav lazy, views/Wizards.tsx NewScheduleForm, self test)
    - `@/views/schedules/shared` → `@/features/schedules/lib/shared`: 2 (self test + lib/snapshot.ts parseSchedulesJSON consumer)
    - `@/views/ExecutionProfiles` → `@/features/execution-profiles/components/ExecutionProfiles`: 2 (nav lazy + self test)
    - Schedules.tsx internal relative `./schedules/shared` → `../lib/shared` (move sırasında path fix)
  - **Public surface (schedules barrel)**: Schedules + NewScheduleForm (Wizards'ta kullanılır) + 4 form helper (scheduleSelectedAgentIssue, scheduleIntentFieldHint, schedulePayloadContract, scheduleFormCadenceLabel) + 2 tip (ScheduleTarget, ScheduleAgent)
  - **Public surface (execution-profiles barrel)**: ExecutionProfiles + 6 helper (profileStatusTone, checkStatusTone, checksByProfileID, executionProfileRollup, executionProfilePolicyFromConfigValues, executionProfileBackendFromConfigValues) + 7 tip
  - **Cross-feature consumers** (2): `views/Wizards.tsx` (NewScheduleForm) + `lib/snapshot.ts` (parseSchedulesJSON) — hepsi rewrite edildi
  - **God file riski**: `Schedules.tsx` 1518 satır (page + 4 helper + NewScheduleForm sub-component) + `ExecutionProfiles.tsx` 1343 satır (page + 6 helper + 7 tip) — her ikisi de Day 26+'da bölünecek
  - **Doğrulama**: `tsc --noEmit` temiz, `vitest run` 191 dosya / 1628 test yeşil (33.14s)
  - **Yardımcı**: `scripts/dev/rewrite-schedules-execution-profiles-imports.py`
- **Day 14** (bu commit): `features/memory/` + `features/standing/` + `features/configcenter/` — 9. 10. 11. feature, orta boy (toplam 4396 satır, 8 dosya + 1 lib).
  - **Taşınan (8 dosya, 4396 satır)**:
    - `views/Memory.tsx` (1073 — god file) + `views/Memory.test.tsx` (361) → `features/memory/components/`
    - `views/Standing.tsx` (1071 — god file) + `views/Standing.test.tsx` (498) → `features/standing/components/`
    - `views/ConfigCenter.tsx` (1061 — god file) + `views/ConfigCenter.test.tsx` (332) → `features/configcenter/components/`
    - `lib/configbackup.{ts,test.ts}` (2, 168 satır) → `features/configcenter/lib/`
  - **Yerinde (cross-feature shared)**: `components/ConfigInventory.tsx` (169 satır — 8+ kullanıcı) → `components/`'te kaldı
  - **Yeni**: 3 feature için 6 barrel dosyası (3 index + 3 types)
  - **Güncellenen**: 11 dosyada import path rewrite
    - `@/lib/configbackup` → `@/features/configcenter/lib/configbackup`: 4 (App.tsx, views/Backup.tsx, lib/snapshot.ts, self test)
    - `@/views/Memory` → `@/features/memory/components/Memory`: 2 (lib/snapshot.ts parseMemoryJSON, self test)
    - `@/views/Standing` → `@/features/standing/components/Standing`: 3 (lib/snapshot.ts parseStandingJSON, views/Wizards.tsx NewOrderForm, self test)
    - `@/views/ConfigCenter` → `@/features/configcenter/components/ConfigCenter`: 3 (self test, features/voice/VoiceSetup.tsx FieldRow+Field+ValueEntry, features/voice/VoiceSetup.test.tsx vi.mock)
  - **Public surface (memory barrel)**: Memory + parseMemoryJSON + TeachFactForm + ReviseFactForm
  - **Public surface (standing barrel)**: Standing + NewOrderForm (Wizards consumer) + parseStandingJSON (snapshot consumer) + 6 formatter (initiativeEnforcement, standingResumeIssue, standingAttentionReasons, standingNeedsAttention, standingAttentionCount, standingFrequencyIssue)
  - **Public surface (configcenter barrel)**: ConfigCenter + FieldRow (voice consumer) + 4 helper (reloadBoundariesFromSections, summarizeReloadBoundaries, agentConfigScopeLabel, summarizeAgentConfigEntries) + 3 tip (Field, ValueEntry, ConfigBundle)
  - **Cross-feature consumers** (6): App.tsx, views/Backup.tsx, lib/snapshot.ts, views/Wizards.tsx, features/voice/VoiceSetup.tsx, features/voice/VoiceSetup.test.tsx — hepsi rewrite edildi
  - **God file riski**: 3 god file (Memory 1073, Standing 1071, ConfigCenter 1061) — hepsi Day 26+'da bölünecek
  - **Doğrulama**: `tsc --noEmit` temiz, `vitest run` 191 dosya / 1628 test yeşil (34.10s)
  - **Yardımcı**: `scripts/dev/rewrite-memory-standing-configcenter-imports.py`
- **Day 15** (bu commit): `features/skills/` + `features/market/` + `features/setup/` — 12. 13. 14. feature, orta boy (toplam 3628 satır, 6 view + 3 lib).
  - **Taşınan (9 dosya, 3628 satır)**:
    - `views/Skills.tsx` (1047 — god file) + `views/Skills.test.tsx` (216) → `features/skills/components/`
    - `views/Market.tsx` (929 — god file) + `views/Market.test.tsx` (191) → `features/market/components/`
    - `lib/market.{ts,test.ts}` (2, 117 satır) → `features/market/lib/`
    - `views/Setup.tsx` (952 — god file) + `views/Setup.test.tsx` (293) → `features/setup/components/`
    - `lib/setup.ts` (137 satır) → `features/setup/lib/`
  - **Yeni**: 3 feature için 6 barrel dosyası (3 index + 3 types)
  - **Internal relative path fix**: features/setup/components/Setup.tsx 2 yerde `@/lib/setup` → `../lib/setup`; features/market/components/Market.tsx + Market.test.tsx + lib/market.test.ts `@/lib/market` → relative (move sırasında)
  - **Güncellenen**: 5 dosyada import path rewrite (cross-feature)
    - `App.tsx` → `@/features/setup/lib/setup` (anyCredentialed + SetupCatalog tip)
    - `views/Wizards.tsx` → `@/features/setup/components/Setup` (Setup component)
    - `views/Skills.test.tsx` → `@/features/skills/components/Skills` (self, 7 sembol)
    - `features/setup/components/Setup.test.tsx` → `@/features/setup/components/Setup` (self)
    - `nav.tsx` → 3 view (lazyNamed)
  - **Public surface (skills barrel)**: Skills + AuthorSkillForm + 5 helper (skillMatches, isWorkshopProposal, scanSkill, lineDiff, diffSkillAgainstParent)
  - **Public surface (market barrel)**: Market + 3 tip (MarketStep, PackDetails, VetReport) — sadece 1 sayfa export
  - **Public surface (setup barrel)**: Setup + 9 fonksiyon (anyCredentialed, defaultSetupFallbacks, mergeSetupTaskRouting, providerKeyEnv, rankProviders, setupFallbackCandidates, setupModelChain, setupTaskSelection, uniqueSetupChainName) + 4 tip (SetupModel, SetupProvider, SetupCatalog, SetupFallbackCandidate) — Setup.tsx'in re-export pattern'i korundu
  - **Cross-feature consumers** (2): App.tsx (lib/setup), views/Wizards.tsx (Setup page)
  - **God file riski**: 3 god file (Skills 1047, Market 929, Setup 952) — hepsi Day 26+'da bölünecek
  - **Doğrulama**: `tsc --noEmit` temiz, `vitest run` 191 dosya / 1628 test yeşil (42.46s)
  - **Yardımcı**: `scripts/dev/rewrite-skills-market-setup-imports.py`
- **Day 16** (bu commit): `features/autonomy/` + `features/overseer/` + `features/sandbox/` — 15. 16. 17. feature, orta boy (toplam 3599 satır, 8 dosya + 1 lib).
  - **Taşınan (8 dosya, 3599 satır)**:
    - `views/Autonomy.tsx` (863) + `views/Autonomy.test.tsx` (460) → `features/autonomy/components/`
    - `lib/autonomy{,.test.ts}` (853) → `features/autonomy/lib/`
    - `views/Overseer.tsx` (554) + `views/Overseer.test.tsx` (182) → `features/overseer/components/`
    - `views/Sandbox.tsx` (485) + `views/Sandbox.test.tsx` (202) → `features/sandbox/components/`
  - **Yeni**: 3 feature için 6 barrel dosyası (3 index + 3 types)
  - **Internal relative path fix**: `features/autonomy/components/Autonomy.tsx` + `lib/autonomy.test.ts` 2 yerde `@/lib/autonomy` → relative
  - **Güncellenen**: 9 dosyada import path rewrite
    - `@/lib/autonomy` → `@/features/autonomy/lib/autonomy`: 5 (views/Activity.tsx, components/DoctorIncidentTrees.tsx, features/incidents/lib/incidents.ts, features/incidents/components/IncidentPage.tsx, features/incidents/components/IncidentBadges.tsx) — incidents feature heavy consumer
    - `@/views/Autonomy` → `@/features/autonomy/components/Autonomy`: 2 (self test, nav)
    - `@/views/Overseer` → `@/features/overseer/components/Overseer`: 2 (self test, nav)
    - `@/views/Sandbox` → `@/features/sandbox/components/Sandbox`: 2 (self test, nav)
  - **Yorum temizliği**: `features/incidents/index.ts` cross-feature-deps notu (Day 16 öncesi yazılmış "lib/autonomy owned by autonomy feature (carved out later)" — yeni path'e güncellendi)
  - **Public surface (autonomy barrel)**: Autonomy + PulseControl + cadenceLabel + AutonomyItem tip
  - **Public surface (overseer barrel)**: Overseer + overseerShouldRefresh
  - **Public surface (sandbox barrel)**: Sandbox + isBuildNoise
  - **Cross-feature consumers** (5): views/Activity, components/DoctorIncidentTrees, features/incidents/* (3 dosya) — hepsi rewrite edildi
  - **Doğrulama**: `tsc --noEmit` temiz, `vitest run` 191 dosya / 1628 test yeşil (39.86s)
  - **Yardımcı**: `scripts/dev/rewrite-autonomy-overseer-sandbox-imports.py`

## 10. Sonraki adaylar (Day 17+)

- **Day 17**: kalan 8 feature batch (data, mcp, models, artifacts, channels, policy, world, connections, toolforge, health, dashboard, jarvis, …)
- **Day 5b** (henüz yapılmadı): `app/help/` topic split (`converse/monitor/agents/automation/knowledge/system`).
- **Day 26+**: `views/` thin-page indirgeme + `App.tsx` + `nav.tsx` refactor + bundle/code-splitting + 9 god file bölünmeleri (Workflows 1597, Schedules 1518, ExecutionProfiles 1343, Memory 1073, Standing 1071, ConfigCenter 1061, Skills 1047, Market 929, Setup 952 → 9 ayrı 4-6 dosyalı split).
