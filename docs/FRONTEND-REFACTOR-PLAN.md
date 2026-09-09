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
- **Day 11** (bu commit): `features/agents/components/{Roster,roster/*}` — agents feature'ı tamamlandı.
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

## 10. Sonraki adaylar (Day 11+)

- **Day 11**: `views/Roster.tsx` + `views/roster/*` → `features/agents/components/Roster.tsx` + `features/agents/components/roster/*` (import'lar zaten forward-compatible, sadece move + rewrite).
- **Day 12**: `features/workflows/` (Workflows.tsx + helpers — God file riski yüksek, önce haritala)
- **Day 13**: `features/schedules/` + `features/execution-profiles/`
- **Day 14**: `features/memory/` + `features/standing/` + `features/configcenter/`
- **Day 15**: `features/skills/` + `features/market/` + `features/setup/`
- **Day 16-20**: kalan 17 feature (autonomy, data, mcp, models, artifacts, channels, policy, world, overseer, sandbox, connections, toolforge, health, dashboard, jarvis, …)
- **Day 5b** (henüz yapılmadı): `app/help/` topic split (`converse/monitor/agents/automation/knowledge/system`).
- **Day 26+**: `views/` thin-page indirgeme + `App.tsx` + `nav.tsx` refactor + bundle/code-splitting.
