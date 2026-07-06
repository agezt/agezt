# AGEZT — Missing Parts Plan (Eksik Parçalar İçin Eylem Planı)

> **Tarih:** 2026-07-04 (last updated: 2026-07-06)
> **Dal:** `main` (HEAD: `ef7b412d`)
> **Statü:** ARCHIVED — Branch `refactor/c4-agentdetail-phase0` merged into `main` and deleted. Content retained for historical reference; see `docs/SYSTEM-AUDIT-REPORT.md` for current audit state.
> **Diğer referanslar:** `docs/OPENCLAW-HERMES-ROADMAP.md`, `docs/JARVIS-VISION-2026.md`, `docs/REFACTORING-INDEX.md`, `docs/GRAVEYARD-POLICY.md`, `docs/PLUGIN-SECURITY.md`, `docs/OPERATIONS.md`, `docs/COMPARISON.md`, `docs/THREAT-MODEL.md`.

---

## 0. Bağlam

`docs/NEXT.md` §0 der ki:

> "Do not declare the project complete. Continue making concrete progress."
>
> "For the current missing-parts audit and execution plan, see `docs/MISSING-PARTS-REPORT.md` and `docs/MISSING-PARTS-PLAN.md`."

Bu dosya **execution plan**'dır. Üçlüsü artık: `SYSTEM-AUDIT-REPORT.md` (denetim), `MISSING-PARTS-REPORT.md` (ham envanter), `MISSING-PARTS-PLAN.md` (bu dosya). İlerideki agent'ın başvuru noktası üçlüsü:

- **`docs/SYSTEM-AUDIT-REPORT.md`** — denetimci özeti + sayısal zemin + düzeltme geçmişi.
- **`docs/MISSING-PARTS-REPORT.md`** — kalem-bazlı ham envanter (F/N/H/D şeması, durum makinesi).
- **`docs/MISSING-PARTS-PLAN.md`** — bu dosya; önceliklendirme, sahiplik, demo gate'ler.

**`NEXT.md` §0 referansı artık tam:** hem `MISSING-PARTS-REPORT.md` hem `MISSING-PARTS-PLAN.md` disk'te mevcut.

### 0.1 Kullanım Kuralı

- Bu doküman **yaşayan** olmalı: her Phase geçildiğinde durumu, gerçekleşen veya yeniden planlanan kalemleri güncellemek için bir PR açılmalı.
- Bir kalem **CLOSED** olunca "→ CLOSED commit-hash" ibaresi ile sabitlenmeli.
- Yeni kalem buraya eklenirse **PR ile** gelmeli, doğrudan main'e düşmemelidir (multi-agent ortamında).

### 0.2 Sınırlamalar

- Dış fetch başarısız (project memory `#fetch #github #undici #network`). Bu plan repo-içi kanıt + `OPENCLAW-HERMES-ROADMAP`/`JARVIS-VISION` ile kuruludur.
- "Owner sign-off gerek" ibareleri: ilgili dokümanlarla (örn. `GRAVEYARD-POLICY.md`) çelişmediğinden emin olunmalı.

---

## 1. Sahiplik ve Roller

| Rol | Sorumluluk |
|---|---|
| **Plan sahibi** | Bir sonraki lead agent — bu dokümanı yaşatır. |
| **Phase 0 hygiene** | ilk 5 gün; düşük-risk, hiçbir özellik değişmez. |
| **Dilim (slice) sahipleri** | Her refactor/PR'ı ayrı bir kişi/agent üstlenir; commit sonrası bu dokümana kapatır. |
| **Jarvis Eksen-B** | F-11…F-14 için ayrı bir çalışma başlatılır; P0 alındıktan sonra tetiklenir. |

---

## 2. Phase 0 — Hygiene & Doc Reorg (Hafta içi)

**Hedef:** Disk ve dokümanı daha temiz bir zemine taşı; multi-agent hatalarını azalt; SPEC ↔ kod matrisini oluştur.

### 2.1 Görevler

| ID | Görev | Kapsam | Çıktı | Süre |
|---|---|---|---|---|
| P0-1 | Worktree prune | `git worktree prune` sonra `git worktree list`'te görünmeyen 19 worktree'i `--force` ile sil | `.claude/worktrees/` 20 → 1 (deep-research); `.worktrees/` 2 → 1 (rebased-main) | 0.5 g | **CLOSED 2026-07-04** (destructive onay alındı) — `git worktree prune -v` + 19 orphan silindi (16 boş + 3 dolu: `anim` 10 MB, `m951-webui-modernize` 161 MB, `ci-verify` 187 MB → toplam 358 MB boşaldı). `m1002-resume` (0 byte) Windows process kilidi — reboot/kilit çözümünden sonra temizlenebilir (zararsız). |
| P0-2 | 16 SPEC başlığını güncelle | `.project/SPEC-{01..16}-*.md` ilk 4-7 satırı `Draft v0.1 · Domain/Repo: TBD` → `Active · Domain: github.com/agezt/agezt · License: MIT` | Sed/script veya manuel PR | 0.5 g | **CLOSED 2026-07-04** (TBD→canonical, dil: SPEC-09 dışında "Language: English" eklendi) |
| P0-3 | `MISSING-PARTS-REPORT.md` oluştur | `SYSTEM-AUDIT-REPORT.md` Section 3'ün ham envanterini canonicalize et | Dosya (~24 KB / 596 satır) | 1 g | **CLOSED 2026-07-04** (43 kalem: 28 F + 6 N + 6 H + 3 D; §6 status log + §7 çapraz-link + §8 istatistik). |
| P0-4 | `SPEC-IMPLEMENTATION-STATUS.md` oluştur | 16 SPEC × {tamamlandı/kısmi/eksik} matrisi | Markdown tablosu | 1 g | **CLOSED 2026-07-04** (13 shipped + 2 partial + 1 design-only; SPEC-12 widget 0 M-raporla ayrıksı). |
| P0-5 | CHANGELOG.md reorg (planlama) | 646 KB CHANGELOG'u milestone başına böl; "Unreleased" + Phase M1300'ler | Plan + ilk bölünmüş dosya | 1 g (plan), sonra incremental | **CLOSED 2026-07-04 (PLAN)** — `docs/CHANGELOG-REORG-PLAN.md` (12.4 KB / 312 satır) oluştu. Hedef: 100'lük M-aralıklarla dilimleme, tools/changelog-split + tools/changelog-lint araçları. **Uygulama adımları** (4 PR, 2-4 g): (PR-1) `tools/changelog-split`, (PR-2) `tools/changelog-lint`, (PR-3) reorg, (PR-4) ops migration helper. Ana CHANGELOG.md → 50 KB hedef. |
| P0-6 | CI gate'ini doğrula | `make check` (Windows-safe eşdeğeri) yeşil mi? | PR'da `make check` çıktısı | 0.5 g | **CLOSED 2026-07-04** — `jsonschemagen`, `go vet ./...`, `depscheck` (24 deps OK), `sdkparity -check` (regen sonrası), `npm test` (1453/1453 passed, 176 dosya), `npm run typecheck`, `npm run build` (390 ms, 2167 modül), `go test -count=1 -p=1 -short ./...` (tüm paketler yeşil), `tools/deadcodecheck` (**OK: no unexpected dead code**), `staticcheck ./...` (**clean**). NOT: İlk paralel `go test ./...` Windows'ta socket-buffer hatası verdi (`TestRunsList_RowCarriesAnswerPreview`); `-p=1` ile düzeldi — Windows için beklenen davranış (NEXT.md §Current Validation Commands). |
| P0-7 | `.dev-home/.gitignore` doğrula | `sandbox/projects/weather-card/.deps/` git-ignore mi | Çıktı + düzeltme PR | 0.5 g | **CLOSED 2026-07-04** — Kök `.gitignore` line 101'deki `.dev-home/` pattern ile tüm runtime state (config.json, creds.json, agentgw.secret, journal, datalake, sandbox `.deps/`, vb.) zaten ignore ediliyor. 12 farklı dosya/dizin için `git check-ignore` hepsi IGNORED, `git ls-files` untracked. **Ek eylem gerekmez.** |

### 2.2 Demo Gate (Phase 0 → Phase 1 arası)

- `git worktree list` çıktısı ana dizine paralel **yalnızca 1 + 1** (kanalizasyon).
- 16 SPEC başlığı canonical biçimde.
- 3 ana doküman üçlüsü (`REPORT`/`AUDIT`/`PLAN`) PR-ready.
- CHANGELOG reorg planı üretildi; ayrı owner onayı / uygulama PR'ı bekliyor.

### 2.3 Sahiplik

| ID | Sahip | Not |
|---|---|---|
| P0-1 | ops-hygiene | `git worktree prune` + (19) `git worktree remove --force`. PR açıp `WORKTREE-ASSESSMENT.md`'i "stale" listesinden temizle. |
| P0-2 | docs-curator | 16 dosya başlığını güncelle; SPEC-09 ve SPEC-11'i özellikle doğrula (V0.1 TBD). |
| P0-3 | bu oturumdaki agent | `MISSING-PARTS-REPORT.md` hemen üretilebilir; SYSTEM-AUDIT-REPORT §3'ü kopyala-yapıştır. |
| P0-4 | docs-curator | Yeni matris; SPEC'lere cross-link ekle. |
| P0-5 | release-mgr | M1300+ için önce "Unreleased" blokunu sabitle. |
| P0-6 | her agent | PR'da otomatik gate. |
| P0-7 | ops-hygiene | `.gitignore` test: `git check-ignore -v .dev-home/sandbox/projects/weather-card/.deps/numpy/__init__.py` |

---

## 3. Phase 1 — Dilim ve Refactor (Sıradaki 1-2 sprint)

**Hedef:** Mevcut branch'i (`refactor/c4-agentdetail-phase0`) temizle; sıradaki refactor dilimini seç; NEXT takip işlerini kapat.

### 3.1 Dilimler (board)

| ID | Refactor / Kalem | Kaynak plan | Çıktı | Süre |
|---|---|---|---|---|
| P1-A | **C4 — Chat decomposition** | `docs/REFACTOR-C4-CHAT-DECOMPOSITION.md` (mevcut branch) | PR mergable | devam | **IN-PROGRESS 2026-07-04** — P0 barrel shim tamamlandı: `frontend/src/views/Chat.tsx` → barrel, gerçek içerik `frontend/src/views/Chat/Chat.tsx`, `frontend/src/views/Chat/index.tsx` eklendi. Ardından mekanik dilimler çıkarıldı: `frontend/src/views/Chat/message.tsx` (message grubu) + `frontend/src/views/Chat/context.tsx` (`barTone`, `ContextChip`, `ContextModal`, `CompactionNote`) + `frontend/src/views/Chat/pickers.tsx` (`ExecutionProfilePicker`, `ConversationPersona`, `PromptLauncher`, `FallbackNote`, `SummaryDivider`, `SteerNote`, `TurnMeta`) + `frontend/src/views/Chat/conversation.tsx` (`ConversationItem`, `QueuePanel`, `EmptyState`, `lastAssistantTools`). **P5 başlangıcı da atıldı:** `frontend/src/views/Chat/useChatSession.ts` eklendi; ardından alt-hook extraction ilerledi: `frontend/src/views/Chat/useComposer.ts`, `frontend/src/views/Chat/useVoice.ts`, `frontend/src/views/Chat/useContextWindow.ts`, `frontend/src/views/Chat/useConversationRouting.ts`, `frontend/src/views/Chat/useSteering.ts`, `frontend/src/views/Chat/useConversationControls.ts` eklendi. `Chat/Chat.tsx` artık yerel UI state/effect/handler kümelerinin büyük kısmını bu hook'lardan destructure ediyor. Root dosya tüm bu modülleri import/re-export ederek test yüzeyini koruyor. Gate: `npm run typecheck`, 11 Chat testi, **tam `npm test` frontend suite** ve `npm run build` yeşil. **P5'in planlanan alt-dilimleri tamamlandı.** Sonraki adım: `C4-clean` (import/comment kalıntıları) veya gerekiyorsa `useExecutionProfile` / `usePersona` gibi daha mikro özel hook’lar. |
| P1-B | **A3 — kernel/httpserver extraction** | `docs/REFACTOR-A3-HTTPSERVER-PLAN.md` | Yeni paket, geriye-uyumlu import | 3-5 g |
| P1-C | **B5 — auth split** | `docs/REFACTOR-A3-B5-AUTH-HTTPSERVER-PLAN.md` | Auth middleware ayrımı | 3-5 g |
| P1-D | **C2 — lib/ keep-vs-colocate** | `docs/REFACTOR-C2-LIB-CLASSIFICATION.md` | Classification rule, PR | 2 g |
| P1-E | **N-1 workflow→agent wake** | NEXT §2 son | Yeni wake subject wiring | 1-2 g |
| P1-F | **N-5 config center per-agent** | NEXT §5 | `kernel/controlplane/configcenter_handler.go` UI genişletme + test | 2 g |

### 3.2 Öncelik ve Bağımlılık

```
P1-A (devam) → P1-D (classification) → P1-B (httpserver) → P1-C (auth split)
P1-E, P1-F (NEXT takip işleri) → herhangi bir dilimde paralel yapılabilir
P1-G (doğrulama gate) — her dilimden sonra `make check`
```

### 3.3 Demo Gate

- C4 PR mergable (varsa birden fazla alt-PR).
- A3 PR mergable, B5 PR mergable.
- N-1, N-5 kapandı.
- `make check` her dilimde yeşil.

### 3.4 Sahiplik

- **Dilim sahipleri** ayrı agent/PR; her PR "phase" etiketi ile.
- P1-E için `kernel/controlplane/standing.go` ve `kernel/workflow` arasındaki wake-subject wiring'i incele; `standing_fired`'daki runbook builder'a benzer şekilde "workflow_fired" oluştur.
- P1-F için `frontend/src/components/AgentDetail.tsx`'in Diagnostics tab'ına 4'lü açık görünüm.

---

## 4. Phase 2 — Görünür Kapılar (Eksen A, 0-60 gün)

**Hedef:** OpenClaw'ın mobil/tepsi ayrıcalığını, Hermes'in LLM curator'unu, canlı tarayıcı sekmesini karşıla. Pazar penceresi daralıyor.

### 4.1 P0 Öncelikli Dilimler

| ID | Eksik | Sprint / süre | Çıktı | Sahiplik |
|---|---|---|---|---|
| P2-A | **F-3 Canlı tarayıcı sekmesi** | 8-10 g | `browser.action`'ı kalıcı Chromium tab process'e çıkar; DOM stale-ref invalidation; E2E fixture | browser-tool owner + browseruse skill owner |
| P2-B | **F-4 LLM Skill Curator** (shadow mod) | 6-8 g | `kernel/skill/curator_llm.go`: kullanım metrikleri üzerinden patch/consolidate **öneren** LLM job; asla silmez | skill kernel owner |
| P2-C | **F-1 Mobil companion (PWA)** | 5-7 g | Web UI'dan PWA; push notification opt-in; share-target; onay/inbox/run-durumu | webui owner |
| P2-D | **F-2 Masaüstü tepsi companion** | 5-7 g | Küçük Go binary; node registry'den daemon'a bağlan; onaylar, tünel, HALT butonu | cli owner |

### 4.2 P1 Dilimler (paralel)

| ID | Eksik | Sprint / süre | Çıktı | Sahiplik |
|---|---|---|---|---|
| P2-E | **F-6 VSCode eklentisi (asgari)** | 8-10 g | VSCode marketplace'e yayınlanabilir paket; ACP üzerinden bağlanır | acp owner |
| P2-F | **F-9 Bağlam `@` referansları + AGENTS.md/CLAUDE.md/SOUL.md injection-taramalı import** | 6-8 g | `chat_summarize`/`@mention` parser; secure loader (injection tarama) | chat owner |

### 4.3 Demo Gate

- **F-3**: E2E test: open page → inspect → click → type → wait → screenshot (with persistent tab session); M-fazı (`PHASE-M???-BROWSER-LIVE-TAB-REPORT.md`) ile.
- **F-4**: Shadow curator önerisi → kullanıcı onayı → `skill.shadow_eval` → active; bir skill üzerinde tam dolaşım.
- **F-1**: PWA Android Chrome'da yüklenebilir; share-target çalışır.
- **F-2**: macOS menü çubuğu + Windows tray binary çalışır.
- **F-6**: VSCode `Agezt` view; chat & run drill.
- **F-9**: Chat'te `@file.txt` → dosya içeriği context'e enjekte edilir; AGENTS.md'den tanımlar yüklenir.

### 4.4 Sahiplik

- F-3 → `plugins/tools/browser/` + `plugins/builtinskills/browseruse/` (iki paket; interface'i yeni `BrowserTool.Driver` ile).
- F-4 → `kernel/skill/curator*`; LLM-judge için `cmd/agt` veya standalone CLI.
- F-1 → `frontend/` PWA manifesti; Service Worker ekleme.
- F-2 → Yeni `cmd/tray/`.
- F-6 → Yeni `ide-plugins/vscode/` (multi-repo).
- F-9 → `frontend/src/views/Chat.tsx` parser + `kernel/runtime/context_budget` loader.

---

## 5. Phase 3 — Jarvis Farklılaştırıcıları (Eksen B, 60-120+ gün)

**Hedef:** Rakipleri geçen — derin araştırma, anticipatory, society-of-agents prod.

### 5.1 Dilimler

| ID | Eksik | Sprint | Çıktı | Sahiplik |
|---|---|---|---|---|
| P3-A | **F-11 Derin araştırma harness'i** | 12-15 g | `plugins/tools/research/` + `plugins/tools/council/` + `plugins/tools/conductor/` birleşik: çok-kaynaklı fan-out → derin okuma → adversarial doğrulama → alıntılı sentez | research-tool owner |
| P3-B | **F-14 K8s job lifecycle** | 8-10 g | `kernel/runtime/exec_profile/k8s.go`; pod lifecycle, exit handling, artifact fetch | exec-profile owner |
| P3-C | **F-12 Anticipatory otonomi** | 10-12 g | Pulse observer'ı; worldmodel'den "hazır taslak" türetir; öneri subject yayınlar | pulse owner |
| P3-D | **F-13 Society-of-agents prod** | 12-15 g | Council.tsx + Conductor.tsx canlı çok-ajanlı muhakeme + workboard lane + delegasyon grafiği | workflow owner + frontend owner |

### 5.2 Demo Gate

- **F-11**: Bir araştırma görevi (örn. "AGEZT'i OpenClaw ile karşılaştır") → 5+ kaynak, çelişki tablosu, citation'lı yanıt.
- **F-14**: `agt run --exec-profile k8s` ile bir pod'da çalışan run, artifact'lar pod'dan geri alınır.
- **F-12**: Bir pulse observer "yarın toplantın var" çıktısı 1 saat önceden kullanıcıya ulaşır.
- **F-13**: Bir karmaşık görev (örn. "kitap araştır + bölüm özetini PDF yap") council + conductor ile dağılır, workboard lane'ler akar, sonuç birleşir.

---

## 6. Phase 4 — Tasarım Aşamasındaki Backlog (Phase 0'ı kapatır, sonra yıllık rotasyona girer)

**Giriş:** SPEC-12/13/14/15/16 içinde "Phase 6/8" işaretli; bunlar Phase 0–3 kapandıkça serbest bırakılır.

### 6.1 Dilimler

| ID | Eksik | SPEC | Phase | Not |
|---|---|---|---|---|
| P4-A | F-15 Saga / compensation birinci-sınıf | SPEC-14 §1 | Phase 6 | `kernel/runtime/saga/`; declarative step + reverse step; workflow ile orkestrasyon |
| P4-B | F-16 Multi-tenant RBAC granüler rol | SPEC-14 §4 | Phase 6 | `kernel/edict/dimension_user.go`; rol/grup ile policy |
| P4-C | F-17 Single-instance RBAC + Edict user-dimension | SPEC-14 §4 | Phase 6 | (P4-B ile örtüşür) |
| P4-D | F-18 Escalation chains | SPEC-14 §6 | Phase 8 | `kernel/alerter/chain.go` |
| P4-E | F-19 External vault entegrasyonu | SPEC-14 §7 | Phase 8 | Plug-gear vault adapter interface |
| P4-F | F-20 Widget SDK + Sandbox render | SPEC-12 | Phase 5+7+8 | `frontend/src/widgets/`; iframe + CSP |
| P4-G | F-21 Widget marketplace | SPEC-12 | Phase 8 | `kernel/market` widget loader |
| P4-H | F-22 Widget scaffold | SPEC-12 | Phase 8 | `tools/create-agezt-plugin` widget mod |
| P4-I | F-23 Capability eval harness | SPEC-14 §3 | Phase 5 | `cmd/eval/`; scenario + success-rate |
| P4-J | F-24 Eval-driven reflection | SPEC-14 §3 | Phase 8 | `kernel/reflect/` → eval consumer |
| P4-K | F-25 UI i18n (TR) | SPEC-14 §8 | Phase 8 | `frontend/src/i18n/` + `react-i18next` |
| P4-L | F-26 OpenTelemetry export | SPEC-14 §9 | Phase 5-8 | `go.opentelemetry.io/otel` entegrasyonu |
| P4-M | F-27 FinOps views / cost attribution | SPEC-14 §9 | Phase 5-8 | `cmd/agt finops` + dashboard |
| P4-N | F-28 Codec/encryption auto-rotation | SPEC-14 §7 | Phase 7 | `kernel/creds/rotation.go` policy |

### 6.2 Bu dilimler için ayrı ayrı plan PR'ları beklenmez; her biri Phase 2/3 ile birleştirildiğinde ilerler.

---

## 7. NEXT.md Takip İşleri (yukarıdaki dilimlerden hangisine girerse)

| NEXT ID | Hedef | Atandığı Phase | Not |
|---|---|---|---|
| N-1 | Workflow → agent wake | Phase 1 (P1-E) | |
| N-2 | "Why quieted" audit event | Phase 4 (P4-D ile birlikte) | |
| N-3 | Guardian schedule düşük-frekans doğrulaması | Phase 4 (P4-D ile birlikte) | |
| N-4 | Auto-archive destructive yol → owner sign-off | **DEFERRED** — `GRAVEYARD-POLICY.md` ile uyumlu, ayrı PR | |
| N-5 | Config center per-agent görünürlük | Phase 1 (P1-F) | |
| N-6 | Yüksek-risk tool APPROVALS per-agent yüzey | Phase 4 (P4-B ile birlikte) | |

---

## 8. Strateji ve "Ötesi" Fırsatlar

### 8.1 F-11 (Derin Araştırma Harness) — En Büyük Stratejik Açı

`docs/JARVIS-VISION-2026.md` doğru tespit ediyor:

> "En büyük 'ötesi' hamlesi: derin araştırma + anticipatory proaktiflik. İkisi de rakiplerde zayıf, AGEZT'in zemini (pulse/worldmodel/workflow/journal) buna hazır."

Phase 3'te Phase 2'den ÖNCE başlatılabilir çünkü düşük risk taşıyan bir **research skill**'i olarak prototip edilebilir. Ama deployment sırası Phase 3'te.

### 8.2 Zamanlama Pencereleri

- **Sprint 0**: Phase 0 hygiene (4-5 g)
- **Sprint 1-2**: Phase 1 dilimleri (10-20 g)
- **Sprint 3-8**: Phase 2 görünür kapılar (60 g)
- **Sprint 9-12**: Phase 3 Jarvis farklılaştırıcıları (60+ g)
- **Phase 4 backlog**: Sprint 5+ ile paralel

---

## 9. Çapraz Dokümanlar

Bu plan şu kaynaklarla tutarlı olmalı:

- **`docs/SYSTEM-AUDIT-REPORT.md`** — ham envanter ve sayısal zemin.
- **`docs/MISSING-PARTS-REPORT.md`** — kalem bazlı envanter.
- **`docs/JARVIS-VISION-2026.md`** — strateji ve Eksen A/B.
- **`docs/OPENCLAW-HERMES-ROADMAP.md`** — rakip parite ve Phase 1-2 sıralaması.
- **`docs/REFACTORING-INDEX.md`** — A1/A2/A3/B5/C2/C4 dilimleri.
- **`docs/GRAVEYARD-POLICY.md`** — N-4 için sign-off barı.
- **`docs/NEXT.md`** — top priorities + Immediate Context (komşu koruyucu).

Bir kalem taşınırsa, **diğer dokümanlar da güncellenmeli**. PR'da tek dosya değişikliği policy dışıdır.

---

## 10. Kapanış

Bu plan, AGEZT'in bir **agentic operating system**'den **Jarvis-class proaktif operatör asistanı**'na evrilmesinin **ilk somut yol haritasıdır**. Tüm kalemler kod-tabani kanıta, dosya referansına ve SPEC ↔ commit eşlemesine dayanır. Dış fetch başarısız olduğundan rakip iddiaları repo-içi dokümanlarla sınırlıdır.

**İlk Sprint:** Phase 0 (4-5 gün) → Phase 1 dilimlerinden 1-2'sini (en azından C4'ün devamı ve N-1).

**Phase 2'ye giriş koşulu:** P0-A/B/C/D closed + `make check` yeşil + hiçbir stale worktree.

**Başarı metriği (6 ay sonra):** (a) F-3 + F-4 + F-6 canlı, (b) Ekim 2026'ya kadar en az 1 stable release, (c) dış kaynak markete en az 50 yayınlanmış skill.

---

*Bu plan `docs/NEXT.md` §0 referansını kapatır. Revizyon notları `MISSING-PARTS-REPORT.md`'deki Kalem-Statü tablosunda yaşar.*
