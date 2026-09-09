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
