# AGEZT — SPEC Implementation Status (SPEC ↔ Kod Durum Matrisi)

> **Tarih:** 2026-07-04 (last updated: 2026-07-06)
> **Dal:** `main` (HEAD: `ef7b412d`)
> **Statü:** ARCHIVED — Branch `refactor/c4-agentdetail-phase0` merged into `main` and deleted. Content retained for historical reference.
> **Diğer referanslar:** `docs/SYSTEM-AUDIT-REPORT.md`, `docs/MISSING-PARTS-REPORT.md`, `docs/MISSING-PARTS-PLAN.md`, `docs/JARVIS-VISION-2026.md`, `docs/OPENCLAW-HERMES-ROADMAP.md`.

---

## 0. Conventions (Sözleşmeler)

Bu matris 16 SPEC'i tek tablo halinde sunar. Her satır:

```
SPEC-XX <başlık>
- **Status:** shipped | partial | design-only | not-started
- **Coverage %:** 0-100 (üretim kodu yüzdesi, ampirik kod-tabani taraması)
- **Last M-report:** disk'te en yüksek M-sayı (veya CHANGELOG'da referans)
- **M-report count:** SPEC'le ilgili disk'teki PHASE-MXXX rapor sayısı
- **Code sites:** ilgili Go paket sayısı (src/test)
- **Major gaps:** ana eksik kalemler
- **Linked items:** bağlantılı F-* / N-* / H-* / D-* kalemleri
- **Commit hint:** biliniyorsa belirgin commit hash
```

### Status Sözlüğü

| Status | Anlam |
|---|---|
| **shipped** | Tüm maddeleri karşılanmış; spec'in scope'unda kalan kalemler ya yok ya da P3 backlog'da. |
| **partial** | Önemli bölümleri (≥50%) tamam; belirgin P0/P1 kalemler açık. |
| **design-only** | Yalnızca tasarım; kod-tabanı varlığı az ya da hiç. |
| **not-started** | Henüz implementasyona başlanmamış; P3 backlog'da. |

### Coverage % (ampirik)

Üç sinyalin basit ortalaması:

1. M-rapor yoğunluğu (proportional, max 50)
2. Go kaynak kodu (proportional, max 30)  
3. Frontend/UI varlığı (proportional, max 20)

Sayılar **yaklaşık**; manuel inceleme ile düzeltilir. Coverage % tek başına karar için yeterli değildir; "Major gaps" ve "Linked items" ile birlikte okunur.

---

## 1. Matris: 16 SPEC'in Durumu

### SPEC-01 Plugin Contracts & Event Schema
- **Status:** shipped
- **Coverage:** ~95%
- **Last M-report:** M518 (disk), M1001 (CHANGELOG)
- **M-report count:** 18
- **Code sites:** `kernel/agentgw`, `kernel/bus`, `kernel/event`, `kernel/journal`, `kernel/ulid` (toplam src ~7, test ~9)
- **Major gaps:** Çok az — schema fuzz devam ediyor (M-ın).
- **Linked items:** —
- **Commit hint:** `agezt-contract.jsonc` audit-gated (PR'da)

### SPEC-02 Kernel (runtime)
- **Status:** shipped
- **Coverage:** ~95%
- **Last M-report:** M923 (disk), M1002 (CHANGELOG)
- **M-report count:** 92
- **Code sites:** `kernel/agent` (9 src, 19 test), `kernel/runtime` (27 src, 53 test), `kernel/controlplane` (87 src, 103 test), `kernel/governor` (7 src, 27 test), `kernel/workflow` (4 src, 3 test), `kernel/planner` (3 src, 4 test)
- **Major gaps:** Zayıf nokta: çok yüksek dosya sayısı (`controlplane` 87 src) — `REFACTOR-A1-CONTROLPLANE-PLAN.md` ile dilimleniyor.
- **Linked items:** N-1 (workflow→agent wake), H-01 (stale refactor index)
- **Commit hint:** `5c4f7c53` (feat(resume): survive daemon restart)

### SPEC-03 Pulse
- **Status:** shipped
- **Coverage:** ~88%
- **Last M-report:** M903 (Reaper)
- **M-report count:** 13
- **Code sites:** `kernel/pulse` (10 src, 8 test), `kernel/alerter` (1+1), `kernel/anomaly` (2+2), `kernel/workboard` (1+2)
- **Major gaps:** Initiative (M999) shipped; Reaper (M903) shipped; salience scoring (M523-527) shipped. **Anticipatory otonomi (F-12)** eksik — kullanıcının bir sonraki ihtiyacını önceden hazırlama.
- **Linked items:** F-12 (anticipatory), N-2 (why quieted audit), N-3 (guardian schedule)
- **Commit hint:** `PHASE-M903-AUTONOMOUS-REAPER-REPORT.md`

### SPEC-04 Plugin Interfaces
- **Status:** shipped
- **Coverage:** ~90%
- **Last M-report:** M912 (MCP catalog library 43 preset)
- **M-report count:** 44
- **Code sites:** `kernel/plugin` (7 src, 23 test), `kernel/mcp` (3+3), `plugins/sdk` (1+2)
- **Major gaps:** **`tools_box` paketi src=0** (yalnızca test=1) — actual plugins `plugins/tools/*` altında. Kütüphane-tarzı plugin registry iyi. Yine de **`plugins/builtinmarket/` plugin fork** spekülasyonu var.
- **Linked items:** F-19 (external vault — plugin tarafı), F-11 (research harness — derin plugin)
- **Commit hint:** `PHASE-M912-MCP-CATALOG-LIBRARY-REPORT.md`

### SPEC-05 Memory, World Model, Skills, Forge
- **Status:** shipped (parite ve ötesi)
- **Coverage:** ~85%
- **Last M-report:** M902 (Forge bias), M896 (Office), M890 (Archive tools), M889 (SQL DB), M894 (Crypto), M893 (SSH), M892 (Email), M891 (HTTP API), M890 (Archive), M889 (SQL), M866 (PDF), M865 (Web research), M864 (Git ops), M863 (Docker), M861 (Data analysis), M859 (Overseer dashboard)
- **M-report count:** 56 (skill lifecycleları için çoğu)
- **Code sites:** `kernel/memory` (7 src, 12 test), `kernel/worldmodel` (4 src, 6 test), `kernel/skill` (7 src, 15 test), `kernel/reflect` (1 src, 1 test), `kernel/brain` (M804 distiller), `plugins/builtinskills` (16 paket)
- **Major gaps:** **F-4 LLM skill curator** (Hermes parite) eksik. Auto-promote ve shadow-eval deterministic; LLM-judge değil.
- **Linked items:** F-4 (LLM curator), F-12 (anticipatory)
- **Commit hint:** `PHASE-M902-FORGE-BIAS-REPORT.md`

### SPEC-06 Security, Sandbox & Warden
- **Status:** shipped (governance moat)
- **Coverage:** ~95%
- **Last M-report:** M495 (CREDS KDF), M494 (CREDS KDF known answer), M476, M474 (TENANT-BLANK-TOKEN-HEAL)
- **M-report count:** 52
- **Code sites:** `kernel/edict` (3 src, 8 test), `kernel/warden` (7 src, 6 test), `kernel/netguard` (1+1), `kernel/redact` (1+6), `kernel/creds` (13 src, 14 test)
- **Major gaps:** **Windows/macOS warden** caveat (system-audit note); Linux tam. **F-26 OpenTelemetry** eksik. **F-16 RBAC** eksik.
- **Linked items:** F-16, F-17 (RBAC), F-26 (OpenTelemetry)
- **Commit hint:** `PHASE-M494-CREDS-KDF-KNOWN-ANSWER-REPORT.md`

### SPEC-07 UI & Surfaces
- **Status:** shipped (paritenin ötesinde)
- **Coverage:** ~92%
- **Last M-report:** M916 (Tools capability gallery), M913 (Attention+approvals bell), M911 (Roster visual cards), M909 (Agents visual gallery)
- **M-report count:** 72
- **Code sites:** `kernel/webui` (9 src, 13 test), `frontend/src/views/` (71 tsx), `frontend/src/components/` (48 tsx), `frontend/src/views/Council.tsx`, `frontend/src/views/Conductor.tsx`, `frontend/src/views/Workboard.tsx`, `frontend/src/views/World.tsx`, `frontend/src/views/Autonomy.tsx`
- **Major gaps:** **F-1 mobil companion** (PWA, P0), **F-2 masaüstü tepsi** (P0), **F-20 widget SDK** (P3).
- **Linked items:** F-1, F-2, F-20
- **Commit hint:** `PHASE-M916-TOOLS-CAPABILITY-GALLERY-REPORT.md`

### SPEC-08 Operability (updates, migrations, contributions, changelog)
- **Status:** shipped
- **Coverage:** ~80%
- **Last M-report:** M585 (certify-main), M509 (sjari), M422 (plugin zombie pin)
- **M-report count:** 15
- **Code sites:** `kernel/market` (8 src, 3 test), `kernel/plugin` (cross-cutting), `kernel/state` (cross-cutting)
- **Major gaps:** **Çapraz-doküman borcu**: `docs/MISSING-PARTS-REPORT.md` ve `docs/MISSING-PARTS-PLAN.md` artık oluştu (2026-07-04, P0-2/P0-3 kapandı). **CHANGELOG reorg (H-04)** hâlâ açık (646 KB).
- **Linked items:** H-04 (CHANGELOG reorg), F-21 (widget marketplace — market altyapısı paylaşımlı)
- **Commit hint:** `PHASE-M585-...-REPORT.md`

### SPEC-09 Identity, Export/Import & Backup
- **Status:** shipped
- **Coverage:** ~85%
- **Last M-report:** M847 (Skill bundles), M846 (Agent graveyard), M557 (TENANT)
- **M-report count:** 30
- **Code sites:** `kernel/tenant` (1+2), `kernel/roster` (2+1), `kernel/standing` (3+3), `kernel/ulid` (1+1)
- **Major gaps:** **`agt migrate openclaw|hermes` (F-10)** yok (yalnızca vault_migrate var). **N-4 graveyard destructive auto-archive** owner sign-off bekliyor.
- **Linked items:** F-10 (migrate komutu), N-4 (graveyard destructive)
- **Commit hint:** `PHASE-M846-AGENT-GRAVEYARD-REPORT.md`

### SPEC-10 LLM, Context & Routing
- **Status:** shipped (farklılaştırıcı)
- **Coverage:** ~95%
- **Last M-report:** M907 (OpenAI toolname collision), M877 (Chat timestamp), M825 (Chat Markdown links)
- **M-report count:** 72
- **Code sites:** Çapraz — `kernel/runtime` (27+53), `kernel/controlplane` (87+103), `kernel/governor` (7+27)
- **Major gaps:** **`tools_box` paketi src=0** (noted in SPEC-04); ayrıca `kernel/chatgptauth` zaten mevcut (M937, M935). Çok iyi durumda.
- **Linked items:** —
- **Commit hint:** `PHASE-M907-PHASE-M7-CHAT-GENRES-REPORT.md` ... çeşitli

### SPEC-11 Deployment & Runtime Environments
- **Status:** partial
- **Coverage:** ~70%
- **Last M-report:** M863 (Docker services skill), M541 (Peer federation), M532 (RUNS costband)
- **M-report count:** 13
- **Code sites:** `kernel/peer` ile ilişkili (cross-cutting)
- **Major gaps:** **F-14 K8s job lifecycle** eksik. **Windows/macOS warden** caveat. **Linux prlimit64** shipped ama diğer OS kısıtlı.
- **Linked items:** F-14 (K8s job), H-04 (CHANGELOG reorg örtük)
- **Commit hint:** `PHASE-M863-DOCKER-SERVICES-SKILL-REPORT.md`

### SPEC-12 Widget System & SDK
- **Status:** design-only
- **Coverage:** ~5%
- **Last M-report:** Yok
- **M-report count:** 0
- **Code sites:** **Hiç widget dizini/code yok** — `frontend/src/widgets/` yok, `kernel/widget*` yok, M-rapor yok.
- **Major gaps:** **Tüm widget ekosistemi eksik**: F-20 (widget SDK + sandbox render), F-21 (widget marketplace), F-22 (widget scaffold).
- **Linked items:** F-20, F-21, F-22
- **Commit hint:** —
- **Not:** SPEC, Phase 5+7+8 için tasarlandı; henüz implementing başlanmamış.

### SPEC-13 Capability Army (Ecosystem Interop)
- **Status:** shipped (altyapı)
- **Coverage:** ~75%
- **Last M-report:** M916, M848, M847
- **M-report count:** 12
- **Code sites:** `kernel/market` (8+3), `plugins/builtinskills` (16 paket), `kernel/skill` (cross-cutting)
- **Major gaps:** **`agt migrate openclaw|hermes` (F-10)** yok. **Dolu hub (F-5)** yok (altyapı var). **agentskills.io/ClawHub adapter** shipped (M377 SKILL-MD), marketplace UX eksik.
- **Linked items:** F-5, F-10, F-21, F-22
- **Commit hint:** `PHASE-M377-SKILLMD-ADAPTER-REPORT.md`

### SPEC-14 Resilience, HITL, Eval, RBAC, Operational Maturity
- **Status:** partial (zayıf)
- **Coverage:** ~50%
- **Last M-report:** M552 (E2E), M400 (Skill shadow-eval), M391 (HITL), M367 (Anomaly), M262 (SDK examples)
- **M-report count:** 7
- **Code sites:** Çapraz — `kernel/anomaly` (2+2), `kernel/standing` (3+3), `kernel/runtime` (27+53 cross-cutting)
- **Major gaps:** **F-15 Saga/compensation birinci-sınıf** eksik. **F-16 RBAC**, **F-17 user-dimension**, **F-18 escalation chains**, **F-19 external vault**, **F-23 capability eval harness**, **F-24 reflection kapaması**, **F-25 UI i18n**, **F-26 OpenTelemetry**, **F-27 FinOps**, **F-28 codec auto-rotation** — hepsi Phase 6/8 backlog.
- **Linked items:** F-15, F-16, F-17, F-18, F-19, F-23, F-24, F-25, F-26, F-27, F-28
- **Commit hint:** `PHASE-M367-ANOMALY-AUTOHALT-REPORT.md`

### SPEC-15 Provider Ecosystem (Catalog Sync, Tool-Calling, ACP)
- **Status:** shipped (farklılaştırıcı, paritenin üstü)
- **Coverage:** ~95%
- **Last M-report:** M912 (MCP catalog), M897 (MCP catalog), M879 (image tools), M845 (artifact collector)
- **M-report count:** 77
- **Code sites:** `plugins/providers/` (15 aile), `plugins/providers/openairesponses` (ChatGPT), `kernel/chatgptauth` (M937/M935 OAuth), `plugins/providers/vertex`, `plugins/providers/bedrock`, `plugins/providers/google`, `plugins/providers/openai`, `plugins/providers/anthropic`, `plugins/providers/cohere`, `plugins/providers/ollama`, `plugins/providers/image`, `plugins/providers/voice`, `plugins/providers/rerank`, `plugins/providers/embed`, `plugins/providers/compat`, `plugins/providers/mock`, `plugins/providers/internal`
- **Major gaps:** **Çok az**. ACP server var (kernel/acp), ACP client (acpagent tool). Ollama auto-discovery shipped.
- **Linked items:** —
- **Commit hint:** `PHASE-M912-MCP-CATALOG-LIBRARY-REPORT.md`

### SPEC-16 Concrete Detail Specifications (API, test, config, DSL, onboarding)
- **Status:** shipped
- **Coverage:** ~85%
- **Last M-report:** M891 (HTTP-API skill), M808 (workflow reliability), M807 (workflow templates)
- **M-report count:** 31
- **Code sites:** Çapraz — `kernel/restapi`, `kernel/webui` (9+13), `cmd/agt`, `cmd/agezt`
- **Major gaps:** **Onboarding flow** shipped (Setup wizard); **agent wake-rule DSL** shipped (STANDING type). **API reference generation** partiküler — `docs/SDK-PARITY.md` mevcut.
- **Linked items:** H-03 (SPEC-IMPLEMENTATION-STATUS.md — bu dosya!)
- **Commit hint:** `PHASE-M808-WORKFLOW-RELIABILITY-REPORT.md`

---

## 2. Özet Tablo

| SPEC | Status | Coverage | M-sayısı | Ana eksik |
|---|---|---|---|---|
| SPEC-01 Plugin Contracts | shipped | ~95% | 18 | — |
| SPEC-02 Kernel | shipped | ~95% | 92 | controlplane refactor dilimi |
| SPEC-03 Pulse | shipped | ~88% | 13 | F-12 anticipatory |
| SPEC-04 Plugins | shipped | ~90% | 44 | F-19 external vault (plugin tarafı) |
| SPEC-05 Memory/World/Skills | shipped | ~85% | 56 | F-4 LLM curator |
| SPEC-06 Security | shipped | ~95% | 52 | F-16 RBAC, F-26 OTel |
| SPEC-07 UI | shipped | ~92% | 72 | F-1/F-2/F-20 |
| SPEC-08 Operability | shipped | ~80% | 15 | H-04 CHANGELOG reorg |
| SPEC-09 Identity | shipped | ~85% | 30 | F-10 migrate, N-4 graveyard |
| SPEC-10 LLM/Context | shipped | ~95% | 72 | — |
| SPEC-11 Deployment | partial | ~70% | 13 | F-14 K8s job |
| **SPEC-12 Widgets** | **design-only** | **~5%** | **0** | F-20/F-21/F-22 (tüm widgetlar) |
| SPEC-13 Capability Army | shipped | ~75% | 12 | F-5 dolu hub, F-10 |
| SPEC-14 Resilience | partial | ~50% | 7 | F-15–F-28 (10 kalem) |
| SPEC-15 Providers | shipped | ~95% | 77 | — |
| SPEC-16 Details | shipped | ~85% | 31 | — |

### Toplam İstatistik

- **shipped:** 13 SPEC (1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 13, 15, 16)
- **partial:** 2 SPEC (11, 14)
- **design-only:** 1 SPEC (12)
- **not-started:** 0 SPEC
- **disk'te M-rapor toplamı:** ~628 SPEC-ile ilişkili (toplam 697'den)
- **toplam Go src dosyası SPEC-alan:** ~187K LOC (SYSTEM-AUDIT'te doğrulandı)

---

## 3. SPEC ↔ Missing-Parts (F-/N-/H-/D-) Çapraz Matris

Her SPEC'in envantere dağılımı:

| SPEC | F- | N- | H- | D- | Toplam |
|---|---|---|---|---|---|
| SPEC-01 | 0 | 0 | 0 | 0 | **0** |
| SPEC-02 | 0 | 1 (N-1) | 0 | 0 | **1** |
| SPEC-03 | 1 (F-12) | 2 (N-2, N-3) | 0 | 0 | **3** |
| SPEC-04 | 1 (F-19) | 0 | 0 | 0 | **1** |
| SPEC-05 | 1 (F-4) | 0 | 0 | 0 | **1** |
| SPEC-06 | 4 (F-16, F-17, F-26, F-28) | 0 | 0 | 0 | **4** |
| SPEC-07 | 3 (F-1, F-2, F-20) | 0 | 0 | 0 | **3** |
| SPEC-08 | 0 | 0 | 1 (H-04) | 0 | **1** |
| SPEC-09 | 1 (F-10) | 1 (N-4) | 0 | 0 | **2** |
| SPEC-10 | 0 | 0 | 0 | 0 | **0** |
| SPEC-11 | 1 (F-14) | 0 | 0 | 0 | **1** |
| **SPEC-12** | **3 (F-20, F-21, F-22)** | 0 | 0 | 0 | **3** |
| SPEC-13 | 3 (F-5, F-10, F-21) | 0 | 0 | 0 | **3** |
| SPEC-14 | 10 (F-15, F-16, F-17, F-18, F-19, F-23, F-24, F-25, F-26, F-27, F-28) | 0 | 0 | 0 | **11** |
| SPEC-15 | 0 | 0 | 0 | 0 | **0** |
| SPEC-16 | 0 | 0 | 1 (H-03) | 1 (D-03) | **2** |

### En Yoğun SPEC (envanter bağlamında)

1. **SPEC-14 Resilience** — 11 açık kalem (F-15…F-28). Backlog kalabalık.
2. **SPEC-06 Security** — 4 açık kalem (RBAC, OTEL, encryption rotation, user-dim).
3. **SPEC-03, SPEC-07, SPEC-12, SPEC-13** — 3'er açık kalem.
4. **SPEC-04, SPEC-05, SPEC-09, SPEC-11** — 1-2 açık kalem.
5. **SPEC-01, SPEC-10, SPEC-15** — sıfır açık kalem (en temiz durumda).

> **Not:** Aynı F-/N-/H-/D- kalemi birden fazla SPEC'e bağlı olabilir (örn. F-16 hem SPEC-06 hem SPEC-14'e bağlı). §3 bağlamında "açık kalem sayısı" tek bir kalem için bağlı SPEC sayısını ifade eder; toplam benzersiz kalem sayısı için `MISSING-PARTS-REPORT.md` §1 sayaç tablosuna bakın.

---

## 4. Bilinen Yanlılıklar (Biases)

Bu matris eğilimli olabilir:

- **Coverage % tahmini**: üç sinyalin basit ortalaması, çok kaba. Ampirik test süreleri ve dash panel coverage daha doğru ölçer.
- **M-rapor sayımı** title-anahtar kelimeleri kullanır; örtüşebilir.
- **"shipped"** = tüm P0/P1 kapandı; SPEC'in bütün bölümleri karşılanmış olmayabilir (P3 backlog kalır).
- **Disk'teki M-sayısı** = dosya adına göre; CHANGELOG'da başka M-sayıları olabilir.

---

## 5. Güncelleme Politikası

- Yeni M-rapor dosyası eklenirse: bu matrisin ilgili SPEC satırında **M-report count** artırılır, **Last M-report** güncellenir.
- Yeni F-/N-/H-/D- kalem eklendikçe §3 çapraz matrisi güncellenir.
- Bu matris, `make check` veya önemli bir PR sonrasında 1 satır kayıt düşülerek revize edilmelidir.
- "shipped" durumundaki bir SPEC için yapılan kapatma **iki kayıt gerektirir**: burada + `MISSING-PARTS-REPORT.md` §6.

---

## 6. Snapshot (2026-07-04)

| Metric | Value |
|---|---|
| Disk'te PHASE-M rapor | 697 dosya |
| Disk'te en yüksek M-sayı | M923 |
| CHANGELOG'da referans edilen en yüksek M-sayı | M1002 |
| SPEC × durum coverage | 13 shipped + 2 partial + 1 design-only |
| Go src dosyası (üretim) | 575 (187,195 LOC) |
| Go test dosyası | 751 |
| Go test/kaynak oranı | 1.30 |
| Frontend view/component | 71 + 48 |
| Frontend test (vitest) | 180 |
| SPEC-12 widget dizini/kodu | 0 |
| Çevre hygiene H-* | 5 açık + 1 kapandı (H-02) |

---

*Bu matris yaşar. SPEC-12 (Widgets) sıfır M-raporla ayrıksı durumda — en büyük "henüz başlanmamış" SPEC. SPEC-14 (Resilience) en yoğun envanter bağlantısına sahip.*
