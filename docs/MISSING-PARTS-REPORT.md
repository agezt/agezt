# AGEZT — Missing Parts Report (Eksik Parçalar — Ham Envanter)

> **Tarih:** 2026-07-04 (last updated: 2026-07-06)
> **Dal:** `main` (HEAD: `ef7b412d`)
> **Statü:** ARCHIVED — Branch `refactor/c4-agentdetail-phase0` merged into `main` and deleted. Content retained for historical reference; see `docs/SYSTEM-AUDIT-REPORT.md` for current audit state and `docs/MISSING-PARTS-PLAN.md` for the action plan.
> **Diğer referanslar:** `docs/SYSTEM-AUDIT-REPORT.md`, `docs/MISSING-PARTS-PLAN.md`, `docs/OPENCLAW-HERMES-ROADMAP.md`, `docs/JARVIS-VISION-2026.md`, `docs/REFACTORING-INDEX.md`, `docs/GRAVEYARD-POLICY.md`.

---

## 0. Conventions (Sözleşmeler)

Bu rapor **canonical data sheet**'tir. Her kalem şu şema ile:

```
### F-NN <kalem adı>
- **Öncelik:** P0 | P1 | P2 | P3
- **Aşama:** SPEC-YY §X | Jarv-P0/P1/P2 | NEXT §N | Phase-0 hygiene
- **Durum:** open | in-progress | done | deferred | needs-design
- **Sahip (rol):** <rol adı>
- **Kanıt (kod/doküman):**
  - `<dosya yolu>`
  - `<path-to-doc>.md §X`
- **M-rapor (varsa):** `PHASE-M???-...-REPORT.md` (disk'te disk taramasıyla doğrulanır)
- **Bağımlılıklar:** <diğer F-/N- kalemleri, PR'lar>
- **Açıklama:** <bir paragraf>
- **Son not:** <YYYY-MM-DD: serbest metin, kapanınca/ertelenince sabitle>
```

### Kısaltmalar

| Kısaltma | Anlam |
|---|---|
| `F-` | Functional gap — kapsam dışı/işlenmemiş özellik (SPEC'de tanımlı). |
| `N-` | NEXT.md tail item — `NEXT.md`'de açıkça işaretlenen takip işi. |
| `H-` | Hygiene — repo hygiene ile ilgili kalem (worktree, doküman, CI). |
| `D-` | Doc gap — sadece dokümantasyon borcu. |
| `P0`/`P1`/`P2`/`P3` | Öncelik (Phase'lerdeki tanımla eşleşir, bkz. `MISSING-PARTS-PLAN.md`). |
| **Jarv** | `JARVIS-VISION-2026.md`'daki §6 Eksen A/B referansı. |

### Durum Makinesi

```
open ──▶ in-progress ──▶ done
     │                  ▶ deferred (gerekçe ile)
     │                  ▶ needs-design (spesifikasyon tıkandı)
     └──▶ deferred (gerekçe: örn. dış fetch başarısız, owner sign-off gerek)
```

`done` yalnızca commit hash'i ile sabitlenir: `done → commit <hash>`.

---

## 1. Sayaç

| Kategori | open | in-progress | needs-design | done | deferred | Toplam |
|---|---|---|---|---|---|---|
| F (functional gap) | 20 | 2 | 2 | 0 | 4 | 28 |
| N (NEXT.md tail)    | 5  | 0 | 0 | 0 | 1 | 6 |
| H (hygiene)         | 0  | 0 | 0 | 6 | 0 | 6 |
| D (doc gap)         | 3  | 0 | 0 | 0 | 0 | 3 |
| **Toplam**          | **28** | **2** | **2** | **6** | **5** | **43** |

> **Not:** Bu sayım dosya revizyonu sırasında güncellenmelidir. "done" olan kalemler için kapatma commit-hash'i §6'da kayıt edilir.

---

## 2. Functional Gaps (F-N)

### F-01 Mobil companion (PWA)
- **Öncelik:** P0 (Jarv P0-1)
- **Aşama:** JARVIS-VISION §6 P0-1, `docs/OPENCLAW-HERMES-ROADMAP.md` Phase 0
- **Durum:** open
- **Sahip:** webui-owner (F-01'de)
- **Kanıt:** Kod tabanında `pwa`/`mobile`/`companion` anahtar kelime eşleşmeleri sıfıra yakın; yalnızca `.dev-home/.../artifacts/index/art-01KWBV0T4C03GXR12WPWABGV7G.json` (artefakt JSON).
- **M-rapor:** Yok
- **Bağımlılıklar:** F-02 (node registry çekirdeği zaten `kernel/peer` mevcut)
- **Açıklama:** OpenClaw ayırt edici gücü — iOS/Android push node (PWA veya native), onay/inbox/voice/run-durumu, share-page webhook hedefi. AGEZT'te yok.
- **Son not:** 2026-07-04 — `frontend/manifest.json` ve Service Worker pattern'i ile başlanabilir.

### F-02 Masaüstü tepsi / menü-çubuğu companion
- **Öncelik:** P0 (Jarv P0-1)
- **Aşama:** JARVIS-VISION §6 P0-1
- **Durum:** open
- **Sahip:** cli-owner
- **Kanıt:** Kod tabanında "tray" anahtar kelimesi 0; ancak `kernel/peer` peer altyapısı var.
- **M-rapor:** Yok
- **Bağımlılıklar:** `kernel/peer` (mevcut)
- **Açıklama:** Başlat/durdur, sağlık, onaylar, push-to-talk, tünel durumu.
- **Son not:** 2026-07-04 — Yeni `cmd/tray/` önerilir.

### F-03 Canlı tarayıcı sekmesi oturumu
- **Öncelik:** P0 (Jarv P0-2)
- **Aşama:** JARVIS-VISION §6 P0-2, OPENCLAW-HERMES Phase 1
- **Durum:** in-progress
- **Sahip:** browser-tool-owner + browseruse-skill-owner
- **Kanıt:**
  - `plugins/tools/browser/browser.go` (346 satır) — wrappers mevcut (`open/snapshot/click/type/wait/screenshot/downloads/cookies/tabs/close`)
  - `plugins/builtinskills/browseruse/scripts/browse.mjs` (395 satır) — `stale` referansı 0
- **M-rapor:** Yok (henüz)
- **Bağımlılıklar:** Browser action provider
- **Açıklama:** Kalıcı canlı Chromium tab process + DOM stale-ref invalidation + çok-adımlı tab lifecycle eksik; E2E fixture'ları eksik.
- **Son not:** 2026-07-04 — `BrowserTool.Driver` interface'i için `browse.mjs`'in driver bölümüne bakılacak.

### F-04 Skill LLM Curator (shadow mod)
- **Öncelik:** P0 (Jarv P0-3)
- **Aşama:** JARVIS-VISION §6 P0-3
- **Durum:** open
- **Sahip:** skill-kernel-owner
- **Kanıt:**
  - `kernel/skill/` (var)
  - `PHASE-M401-SKILL-AUTOPROMOTE-REPORT.md` (auto-promote, deterministic)
  - Hermes'in ayırt edici gücü.
- **M-rapor:** Yok (henüz)
- **Bağımlılıklar:** M399-M401 altyapısı
- **Açıklama:** Yalnızca deterministik küratör var. Shadow-eval ve auto-promote var ama bunlar LLM-judged değil. Hermes parite.
- **Son not:** 2026-07-04 — LLM-judge bir yardımcı-model olarak gölgede çalışır; asla silmez (her zaman archive/revertable).

### F-05 Dolu skill pazarı (ClawHub/agentskills.io)
- **Öncelik:** P1 (Jarv P1-5)
- **Aşama:** JARVIS-VISION §6 P1-5
- **Durum:** open
- **Sahip:** market-owner
- **Kanıt:**
  - `plugins/builtinmarket/` (altyapı)
  - `kernel/market/` (registry)
  - `plugins/builtinmarket/...` ve `kernel/market` ile imzalı/BLAKE3 doğrulamalı kurulum hazır.
- **M-rapor:** Yok
- **Bağımlılıklar:** Trust UX (imza, risk card, scanner bulguları)
- **Açıklama:** Binlerce paketlik yaşayan hub yok.

### F-06 VSCode IDE eklentisi (asgari)
- **Öncelik:** P1 (Jarv P1-6)
- **Aşama:** JARVIS-VISION §6 P1-6
- **Durum:** open
- **Sahip:** acp-owner + ext-owner
- **Kanıt:** `kernel/acp/` ACP yüzeyi mevcut; shipped eklenti yok.
- **M-rapor:** Yok
- **Bağımlılıklar:** ACP server
- **Açıklama:** VSCode marketplace'e yayınlanabilir paket; ACP üzerinden bağlanır.

### F-07 Batch processing yüzeyi (100-1000 paralel)
- **Öncelik:** P1 (Jarv P1-7)
- **Aşama:** OPENCLAW-HERMES §6 P1-7
- **Durum:** open
- **Sahip:** workflow-owner
- **Kanıt:** Workflow aracılığıyla dolaylı; kod tabanında doğrudan `batch_*.go` yok.
- **M-rapor:** Yok
- **Bağımlılıklar:** workboard (M-*)
- **Açıklama:** Doğrudan yüzey yok; workflow ile dolaylı.

### F-08 Credential pool + otomatik rotasyon
- **Öncelik:** P1
- **Aşama:** OPENCLAW-HERMES matrix 🟡
- **Durum:** open
- **Sahip:** creds-owner
- **Kanıt:** `kernel/creds/keyring.go` + 28 dosya (`aws.go`, `sigv4.go`, `machineid_*.go`, `pbkdf2_test.go`, `kdf_known_answer_internal_test.go`, vb.); çoklu anahtar havuzu yok, otomatik rotasyon yok.
- **M-rapor:** Yok
- **Bağımlılıklar:** M172 PBKDF2, M303 nonce
- **Açıklama:** Tek-anahtar kullanan keyring; havuzlanmış çoklu-anahtar keyring + zaman-tabanlı rotasyon eksik.

### F-09 Bağlam `@dosya/klasör/diff/URL` referansları
- **Öncelik:** P1 (Jarv P1-4)
- **Aşama:** JARVIS-VISION §6 P1-4
- **Durum:** open
- **Sahip:** chat-owner + context-kernel-owner
- **Kanıt:** Kod tabanında `chat_summarize` var; `@mention` parser yok; AGENTS.md/CLAUDE.md/SOUL.md injection-taramalı import yok.
- **M-rapor:** Yok
- **Bağımlılıklar:** `kernel/runtime/context_budget`
- **Açıklama:** Hermes'in ayırt edici gücü; şu anda kısmi yok.

### F-10 `agt migrate openclaw|hermes` komutu
- **Öncelik:** P1
- **Aşama:** SPEC-13 §1.3, ROADMAP Phase 9
- **Durum:** open
- **Sahip:** cmd-owner
- **Kanıt:** `cmd/agt/vault_migrate_test.go` var; gerçek `cmd/agt/migrate.go` yok.
- **M-rapor:** Yok
- **Bağımlılıklar:** SPEC-13 §1.3
- **Açıklama:** Sadece vault migrate var; OpenClaw/Hermes profile+memory+skill import komutu yok.

### F-11 Derin araştırma harness'i
- **Öncelik:** P2 (Jarv P2-8)
- **Aşama:** JARVIS-VISION §6 P2-8; `docs/DEEP-RESEARCH-HARNESS-PLAN.md`, `docs/DEER-FLOW-IMPLEMENTATION-PLAN.md`
- **Durum:** needs-design
- **Sahip:** research-tool-owner + council-owner + conductor-owner
- **Kanıt:**
  - `plugins/tools/research/`, `plugins/tools/council/`, `plugins/tools/conductor/` (kısmi)
  - `docs/DEEP-RESEARCH-HARNESS-PLAN.md` (plan)
  - `docs/DEER-FLOW-IMPLEMENTATION-PLAN.md` (plan)
- **M-rapor:** Yok
- **Bağımlılıklar:** Puls e + worldmodel + workflow (mevcut)
- **Açıklama:** Çok-kaynaklı fan-out → derin okuma → çelişki/adversarial doğrulama → alıntılı sentez. **En büyük stratejik açık.**

### F-12 Anticipatory otonomi
- **Öncelik:** P2 (Jarv P2-9)
- **Aşama:** JARVIS-VISION §6 P2-9
- **Durum:** needs-design
- **Sahip:** pulse-owner
- **Kanıt:** Pulse observer + Initiative (M999), Reaper (M903) mevcut.
- **M-rapor:** Yok
- **Bağımlılıklar:** worldmodel decay
- **Açıklama:** pulse + worldmodel + memory → kullanıcının bir sonraki ihtiyacını önceden hazırlama (brifing/taslak/uyarı).

### F-13 Society-of-agents üretimleştirme
- **Öncelik:** P2 (Jarv P2-10)
- **Aşama:** JARVIS-VISION §6 P2-10
- **Durum:** in-progress (UI mevcut)
- **Sahip:** workflow-owner + frontend-owner
- **Kanıt:** `frontend/src/views/Council.tsx`, `frontend/src/views/Conductor.tsx` UI mevcut.
- **M-rapor:** Yok
- **Bağımlılıklar:** F-11 (research harness ile yakından bağlantılı)
- **Açıklama:** Canlı çok-ajanlı muhakeme + delegasyon grafiği + workboard lane entegrasyonu ile olgunlaştırılacak.

### F-14 K8s job lifecycle
- **Öncelik:** P2 (Jarv P2-11)
- **Aşama:** OPENCLAW-HERMES Phase 1 §Terminal/sandbox; JARVIS-VISION P2-11
- **Durum:** open
- **Sahip:** exec-profile-owner
- **Kanıt:** `kernel/runtime/exec_profile/` (kısmi).
- **M-rapor:** Yok
- **Bağımlılıklar:** Lokal/SSH/Daytona exec-profile (mevcut)
- **Açıklama:** Shell/code_exec routing için pod lifecycle tamamlanmadı.

### F-15 Saga / compensation birinci-sınıf
- **Öncelik:** P3
- **Aşama:** SPEC-14 §1 Phase 6
- **Durum:** open
- **Sahip:** runtime-owner + workflow-owner
- **Kanıt:** `kernel/runtime`, `kernel/workflow`, `kernel/resume`. Declarative saga model yok.
- **M-rapor:** Yok
- **Bağımlılıklar:** workflow engine
- **Açıklama:** Temel retry/checkpoint var, full saga ters-invoke hâlâ kısmi.

### F-16 Multi-tenant RBAC granüler rol modeli
- **Öncelik:** P3
- **Aşama:** SPEC-14 §4 Phase 6; SPEC-09 §6
- **Durum:** open
- **Sahip:** tenant-owner + edict-owner
- **Kanıt:** `kernel/tenant/tenant.go`. Granüler rol yok.
- **M-rapor:** Yok
- **Bağımlılıklar:** Edict user-dimension (F-17)
- **Açıklama:** Multi-tenant var; "tek-kişilik için bunyruk olmayan rol" yok.

### F-17 Single-instance RBAC + Edict user-dimension
- **Öncelik:** P3
- **Aşama:** SPEC-14 §4 Phase 6
- **Durum:** deferred (F-16 ile çakışır; F-16 kapandığında aç)
- **Sahip:** edict-owner
- **Kanıt:** `kernel/edict/` policy engine mevcut.
- **M-rapor:** Yok
- **Bağımlılıklar:** F-16
- **Açıklama:** Tek instance içinde çok-kullanıcı modeli.

### F-18 Escalation chains
- **Öncelik:** P3
- **Aşama:** SPEC-14 §6 Phase 8
- **Durum:** open
- **Sahip:** alerter-owner + pulse-owner
- **Kanıt:** `kernel/alerter/alerter.go`, `kernel/alerter/alerter_test.go` (mevcut).
- **M-rapor:** Yok
- **Bağımlılıklar:** channel push (mevcut, M922)
- **Açıklama:** Alert → ack izleme → kanaldan kanala geçiş; ack tracking.

### F-19 External vault entegrasyonu
- **Öncelik:** P3
- **Aşama:** SPEC-14 §7 Phase 8
- **Durum:** open
- **Sahip:** creds-owner + plugin-owner
- **Kanıt:** `kernel/creds/creds.go`, vault enc + rotate var. Pluggable secret-provider interface yok.
- **M-rapor:** Yok
- **Bağımlılıklar:** F-28 (rotation)
- **Açıklama:** Org için pluggable secret backend (HashiCorp Vault, AWS Secrets Manager, vb.).

### F-20 Widget SDK + Sandbox render
- **Öncelik:** P3
- **Aşama:** SPEC-12 §5-7 (Phase 5+7+8)
- **Durum:** open
- **Sahip:** webui-owner + runtime-owner
- **Kanıt:** SPEC-12; frontend `views/Chat.tsx` Markdown kullanıyor, widget render değil.
- **M-rapor:** Yok
- **Bağımlılıklar:** Frontend i18n (F-25)
- **Açıklama:** Sandboxed (iframe + sıkı CSP); widget'lar konuşmayı zenginleştiren interaktif öğeler.

### F-21 Widget marketplace
- **Öncelik:** P3
- **Aşama:** SPEC-12 §4-5 (Phase 8)
- **Durum:** deferred (F-20 ile bağımlı)
- **Sahip:** market-owner
- **Kanıt:** SPEC-12 §4-5.
- **M-rapor:** Yok
- **Bağımlılıklar:** F-20
- **Açıklama:** Widget'lar için marketplace (F-05 ile paylaşımlı altyapı).

### F-22 Widget scaffold (`create-agezt-plugin` widget modu)
- **Öncelik:** P3
- **Aşama:** SPEC-12 §5 (Phase 8)
- **Durum:** deferred (F-20-F-21 ile bağımlı)
- **Sahip:** sdk-owner
- **Kanıt:** SPEC-12 §5.
- **M-rapor:** Yok
- **Bağımlılıklar:** F-20, F-21
- **Açıklama:** Scaffolder widget için şablon üretir.

### F-23 Capability eval harness
- **Öncelik:** P3
- **Aşama:** SPEC-14 §3 (Phase 5)
- **Durum:** open
- **Sahip:** eval-owner
- **Kanıt:** SPEC-14 §3.
- **M-rapor:** Yok
- **Bağımlılıklar:** M399 shadow-eval
- **Açıklama:** Tool/skill başarı oranı ölçümü; journal'dan türetilmiş scenario'lar.

### F-24 Eval-driven reflection kapaması
- **Öncelik:** P3
- **Aşama:** SPEC-14 §3 (Phase 8)
- **Durum:** deferred (F-23 ile bağımlı)
- **Sahip:** reflect-owner
- **Kanıt:** SPEC-14 §3.
- **M-rapor:** Yok
- **Bağımlılıklar:** F-23
- **Açıklama:** Reflection eval sonuçlarını tüketir.

### F-25 UI i18n (TR default'a ek)
- **Öncelik:** P3
- **Aşama:** SPEC-14 §8 (Phase 8)
- **Durum:** open
- **Sahip:** webui-owner
- **Kanıt:** `frontend/package.json` i18n kütüphanesi yok (i18next, react-intl, @formatjs hepsi yok).
- **M-rapor:** Yok
- **Bağımlılıklar:** Yok
- **Açıklama:** Şu anda hardcoded İngilizce; İngilizce default + Türkçe-ready + locale-aware formatting.

### F-26 OpenTelemetry export
- **Öncelik:** P3
- **Aşama:** SPEC-14 §9 (Phase 5-8)
- **Durum:** open
- **Sahip:** observability-owner
- **Kanıt:** `go.sum`'da `otel`/`opentelemetry` yok; SPEC-14 §9.
- **M-rapor:** Yok
- **Bağımlılıklar:** Yok
- **Açıklama:** Traces/metrics/logs external collector'a (otel/jaeger/tempo) export.

### F-27 FinOps views / cost attribution
- **Öncelik:** P3
- **Aşama:** SPEC-14 §9 + SPEC-10 §6
- **Durum:** open
- **Sahip:** governor-owner
- **Kanıt:** `kernel/controlplane/budget.go` + `kernel/governor/*budget*.go` (mevcut); "finops"/"cost attribution" anahtar kelimeleri 0.
- **M-rapor:** Yok
- **Bağımlılıklar:** Cost aggregation store
- **Açıklama:** Tenant/agent/task başına maliyet dağılımı ve trend.

### F-28 Codec/encryption auto-rotation lifecycle
- **Öncelik:** P3
- **Aşama:** SPEC-14 §7 (Phase 7)
- **Durum:** open
- **Sahip:** creds-owner
- **Kanıt:** `kernel/creds/rotate_test.go` var; automation policy'si yok.
- **M-rapor:** Yok
- **Bağımlılıklar:** F-19, F-08
- **Açıklama:** Vault enc key ve API anahtarları için otomatik rotasyon politikası.

---

## 3. NEXT.md Tail Items (N-N)

### N-01 Workflow → agent-node wake
- **Öncelik:** P1
- **Aşama:** NEXT §2 son; MISSING-PARTS-PLAN P1-E
- **Durum:** open
- **Sahip:** controlplane-owner + workflow-owner
- **Kanıt:** `kernel/workflow`, `kernel/controlplane` mevcut; `standing_fired`'a benzer `workflow_fired` subject yok.
- **M-rapor:** Yok
- **Bağımlılıklar:** M-1 autonomy runbook
- **Açıklama:** Bir workflow bir agent'ı direkt uyandırmak için yol yok.

### N-02 "Why quieted" audit event
- **Öncelik:** P3
- **Aşama:** NEXT §6; MISSING-PARTS-PLAN P4-D
- **Durum:** open
- **Sahip:** guardian-owner + alerter-owner
- **Kanıt:** `plugins/builtinguardians/`, `cmd/agt/doctor.go`.
- **M-rapor:** Yok
- **Bağımlılıklar:** F-18
- **Açıklama:** Doctor quiet patch fires için `policy.quiet_patch_fired` olayı.

### N-03 Guardian schedule düşük-frekans doğrulaması
- **Öncelik:** P3
- **Aşama:** NEXT §6
- **Durum:** open
- **Sahip:** guardian-owner + scheduler-owner
- **Kanıt:** `plugins/builtinguardians/SeedAll` 8h cooldown.
- **M-rapor:** Yok
- **Bağımlılıklar:** Yok
- **Açıklama:** Sürekli tetikleyen guardian olmadığının periyodik doğrulanması (seeder 8h).

### N-04 Auto-archive (graveyard) destructive yol — sign-off
- **Öncelik:** P3 → deferred (owner sign-off gerek)
- **Aşama:** NEXT §7 son; `docs/GRAVEYARD-POLICY.md`
- **Durum:** deferred (owner sign-off)
- **Sahip:** roster-owner + owner
- **Kanıt:** `docs/GRAVEYARD-POLICY.md`, `kernel/controlplane/roster.go`.
- **M-rapor:** Yok
- **Bağımlılıklar:** Uyumluluk — `docs/GRAVEYARD-POLICY.md`
- **Açıklama:** Destructive auto-archive için sign-off. Şu an yalnızca **rapor** (graveyard_scan system task).
- **Son not:** 2026-07-04 — Sign-off gelene kadar defer.

### N-05 Config center per-agent görünürlük (4'lü)
- **Öncelik:** P1
- **Aşama:** NEXT §5; MISSING-PARTS-PLAN P1-F
- **Durum:** open
- **Sahip:** configcenter-owner + frontend-owner
- **Kanıt:** `kernel/controlplane/configcenter_handler.go`, `frontend/src/components/AgentDetail.tsx`.
- **M-rapor:** Yok
- **Bağımlılıklar:** Yok
- **Açıklama:** owned / shared-allowlisted / hidden secrets / excluded dörtlü.

### N-06 Yüksek-risk tool APPROVALS per-agent yüzey
- **Öncelik:** P3
- **Aşama:** NEXT §5; MISSING-PARTS-PLAN P4-B
- **Durum:** open
- **Sahip:** controlplane-owner + frontend-owner
- **Kanıt:** `kernel/agent/agent.go`, `/api/approvals` log; deny'ler per-agent var (M-ın); approvals per-agent yok.
- **M-rapor:** Yok
- **Bağımlılıklar:** F-17 (user-dimension)
- **Açıklama:** Deny'lerin yanı sıra approvals yüzeyi.

---

## 4. Hygiene Items (H-N)

### H-01 Stale worktree'ler (22 → 1 kilitli)
- **Öncelik:** P0 (Phase 0 hygiene)
- **Aşama:** MISSING-PARTS-PLAN P0-1
- **Durum:** done 2026-07-04 (destructive onay alındı; 1 kilitli kalıntı — reboot/kilit çözümünden sonra temizlenebilir)
- **Sahip:** ops-hygiene
- **Kanıt:**
  - `git worktree list` → 3 worktree (1 ana + deep-research + rebased-main)
  - Klasör tarama (önceki): 22 klasör
  - 2026-07-04 tur 1: 16 boş orphan (0-byte) silindi.
  - 2026-07-04 tur 2 (destructive onay alındıktan sonra): 3 dolu orphan silindi — `anim` (10 MB / 0.1s), `m951-webui-modernize` (161 MB / 1.7s), `ci-verify` (187 MB / 3.5s). **Toplam 358 MB boşaldı.**
  - **Kalıntı:** `.worktrees/m1002-resume` (0-byte, başka process kilidi — Windows Defender veya SearchService tutuyor olabilir; reboot veya kilit çözümünden sonra tekrar denenebilir).
- **M-rapor:** Yok
- **Bağımlılıklar:** Yok
- **Açıklama:** `git worktree prune` + listelenmeyen worktree'lerin recursive silinmesi. Toplam 19 orphan silindi. Ana hedef olan "git worktree list temizliği + 22→2 orphan azaltma" tamamlandı; son 1 kalıntı zararsız.

### H-02 SPEC başlıkları canonicalize — tamamlandı
- **Öncelik:** P0 (Phase 0 hygiene)
- **Aşama:** MISSING-PARTS-PLAN P0-2
- **Durum:** done → 2026-07-04
- **Sahip:** docs-curator
- **Kanıt:** `.project/SPEC-01..16-*.md` — `Status: Active · Domain: github.com/agezt/agezt · License: MIT` (16/16).
- **Son not:** 2026-07-04 — Tüm 16 SPEC canonicalize edildi.

### H-03 SPEC-IMPLEMENTATION-STATUS.md oluştur — tamamlandı
- **Öncelik:** P1
- **Aşama:** MISSING-PARTS-PLAN P0-4
- **Durum:** done → 2026-07-04
- **Sahip:** docs-curator
- **Kanıt:** `docs/SPEC-IMPLEMENTATION-STATUS.md` (mevcut, 17.5 KB / 317 satır); 13 shipped + 2 partial + 1 design-only + 0 not-started; SPEC-12 widget 0 M-raporla ayrıksı; §3 çapraz matris (SPEC ↔ F-/N-/H-/D-) eklendi.
- **Bağımlılıklar:** Yok
- **Açıklama:** 16 SPEC × {tamamlandı/kısmi/eksik} matrisi.

### H-04 CHANGELOG.md reorg planı — tamamlandı (plan only)
- **Öncelik:** P2
- **Aşama:** MISSING-PARTS-PLAN P0-5
- **Durum:** done → 2026-07-04 (plan oluştu; uygulama 4 PR'lık incremental sprint)
- **Sahip:** release-mgr
- **Kanıt:** `docs/CHANGELOG-REORG-PLAN.md` (12.4 KB / 312 satır). Plan: 100'lük M-aralıklarla dilimleme; PR-1 `tools/changelog-split`, PR-2 `tools/changelog-lint`, PR-3 dosya yazma, PR-4 migration helper.
- **Bağımlılıklar:** Yok
- **Açıklama:** CHANGELOG'u milestone başına bölünmüş dosyalara ayır. **Uygulama** (PR-3) bu plandaki adımlara göre olur; H-04 planı kapsamında done.

### H-05 `make check` yeşil doğrulaması — tamamlandı
- **Öncelik:** P0
- **Aşama:** MISSING-PARTS-PLAN P0-6
- **Durum:** done → 2026-07-04
- **Sahip:** her agent (PR gate)
- **Kanıt:**
  - `go run ./tools/jsonschemagen` — exit 0
  - `go vet ./...` — exit 0
  - `go run ./tools/depscheck` — "OK: 24 core dependencies, all justified"
  - `go run ./tools/sdkparity -check docs/SDK-PARITY.md` — exit 0 (re-generate sonrası)
  - `npm test` (vitest) — 1453/1453 passed, 176 test file (~26 s)
  - `npm run typecheck` — exit 0
  - `npm run build` — 390 ms, 2167 modül
  - `go test -count=1 -p=1 -short ./...` — **tüm paketler yeşil** (~180+ paket)
  - `go run ./tools/deadcodecheck` — **OK: no unexpected dead code**
  - `staticcheck ./...` — **clean**
- **Bağımlılıklar:** Yok
- **Açıklama:** PowerShell eşdeğerini yeşil doğrula. Ana CI gate (test+build+lint+deadcode) yeşil. Windows'ta ilk paralel `go test ./...` socket-buffer hatası verebildiği için `-p=1` ile çalıştırıldı; NEXT.md §Current Validation Commands ile uyumlu.

### H-06 `.dev-home/.gitignore` doğrulaması — tamamlandı (zaten ignore ediliyor)
- **Öncelik:** P1
- **Aşama:** MISSING-PARTS-PLAN P0-7
- **Durum:** done → 2026-07-04 (kök `.gitignore` zaten `.dev-home/` ignore ediyor, doğrulandı)
- **Sahip:** ops-hygiene
- **Kanıt:** Kök `.gitignore` line 101: `.dev-home/` pattern. `git check-ignore -v` 12 farklı `.dev-home/{config.json,creds.json,agentgw.secret,sandbox/,journal/,datalake/,memory/,roster/,artifacts/...}` path için hepsini IGNORED olarak işaretliyor; `git ls-files` untracked. SYSTEM-AUDIT-REPORT §5.3'teki tahmin doğrulandı.
- **Bağımlılıklar:** Yok
- **Açıklama:** `git check-ignore` ile doğrula; ignore değilse `.gitignore` ekle. **Zaten ignore ediliyor — ek bir eylem gerekmez.**

---

## 5. Doc Gaps (D-N)

### D-01 `.project/TASKS.md` v0.1 taslak yenileme
- **Öncelik:** P2
- **Aşama:** SYSTEM-AUDIT-REPORT §4.3
- **Durum:** open
- **Sahip:** docs-curator
- **Kanıt:** `.project/TASKS.md` (mevcut, "Status: Draft v0.1"); tüm checklist `[ ]` todo.
- **Bağımlılıklar:** Yok
- **Açıklama:** 1000+ M-faz raporu ile senkronize değil; ya yeniden yaz ya "archived" işaretle.

### D-02 Yarışan SPEC özetleri (README/SPEC bridge)
- **Öncelik:** P2
- **Aşama:** SYSTEM-AUDIT-REPORT §4.5
- **Durum:** open
- **Sahip:** docs-curator
- **Kanıt:** `JARVIS-VISION-2026.md`, `OPENCLAW-HERMES-ROADMAP.md` rakip-parite matrisleri var; ama AGEZT-internal SPEC ↔ kod durum matrisi yok.
- **Bağımlılıklar:** H-03 ile paylaşımlı
- **Açıklama:** README'de SPEC-01..16 kısa özet tablosu.

### D-03 Yarışan geçiş tabloları (`STATUS-*.md`)
- **Öncelik:** P3
- **Aşama:** SYSTEM-AUDIT-REPORT §4.4
- **Durum:** open
- **Sahip:** docs-curator
- **Kanıt:** `.project/STATUS-2026-06-03-POST-M{249,255,257,265}.md` (mevcut); yeni milestone geçişleri için canonical format.
- **Bağımlılıklar:** Yok
- **Açıklama:** Milestone geçişlerinde status snapshot.

---

## 6. Status Log (Kapanmış / Ertelenmiş Kalemler)

Bu bölüm **done** ve **deferred** kalemlerin tarihli kaydını tutar. Yeni kapanan kalemler buraya taşınır.

### Kapalı (done)

| ID | Tarih | Commit | Kapanış notu |
|---|---|---|---|
| **H-02** SPEC başlık canonicalize | 2026-07-04 | (bu oturum) | 16/16 SPEC `Active · Domain: github.com/agezt/agezt · License: MIT` (SPEC-09 dışında `Language: English` eklendi). |
| **H-01** Stale worktree'ler | 2026-07-04 | (bu oturum) | 19 orphan silindi (16 boş + 3 dolu, toplam 358 MB). `git worktree list` temiz: ana + `deep-research` + `rebased-main`. `m1002-resume` (0-byte) Windows process kilidi yüzünden silinemedi (zararsız kalıntı; reboot/kilit çözümünden sonra temizlenebilir). |
| **H-03** SPEC-IMPLEMENTATION-STATUS.md | 2026-07-04 | (bu oturum) | `docs/SPEC-IMPLEMENTATION-STATUS.md` (17.5 KB / 317 satır); 13 shipped + 2 partial + 1 design-only. |
| **H-04** CHANGELOG.md reorg planı | 2026-07-04 | (bu oturum) | `docs/CHANGELOG-REORG-PLAN.md` (12.4 KB / 312 satır) oluştu. 646 KB tek dosyayı 100'lük M-aralıklarla dilimleme stratejisi; 4 PR'lık uygulama planı. |
| **H-05** `make check` yeşil doğrulaması | 2026-07-04 | (bu oturum) | Ana CI gate (test+build+lint+deadcode) YEŞİL: jsonschemagen, vet, depscheck (24 OK), sdkparity (re-gen sonrası), vitest 1453/1453, typecheck, build 390ms, `go test -count=1 -p=1 -short ./...` tüm paketler yeşil, `go run ./tools/deadcodecheck` clean, `staticcheck ./...` clean. |
| **H-06** `.dev-home/.gitignore` doğrulaması | 2026-07-04 | (bu oturum) | Kök `.gitignore` line 101'deki `.dev-home/` pattern ile tüm runtime state (config.json, creds.json, agentgw.secret, journal, datalake, sandbox, vb.) zaten ignore ediliyor. 12 farklı dosya/dizin için `git check-ignore` hepsi IGNORED, `git ls-files` untracked. |

### Ertelenmiş (deferred)

| ID | Tarih | Gerekçe | Yeniden açma koşulu |
|---|---|---|---|
| **F-17** Single-instance RBAC + Edict user-dimension | 2026-07-04 | F-16 ile çakışıyor; F-16 kapanınca aç | F-16 PR'da tamamlanınca |
| **F-21** Widget marketplace | 2026-07-04 | F-20'e bağımlı | F-20 done olunca |
| **F-22** Widget scaffold | 2026-07-04 | F-20 + F-21'e bağımlı | F-20 ve F-21 done olunca |
| **F-24** Eval-driven reflection | 2026-07-04 | F-23'e bağımlı | F-23 done olunca |
| **N-04** Graveyard destructive auto-archive | 2026-07-04 | `docs/GRAVEYARD-POLICY.md` ile uyumlu; yıkıcı yol için owner sign-off gerek | Owner sign-off + `docs/GRAVEYARD-POLICY.md` policy netleşmesi |

### Disk'te sırası bekleyen (notes/limitations)

- Dış fetch başarısız (project memory `#fetch #github #undici #network`); rakip dokümanları (`openclaw.ai`, `hermes-agent.nousresearch.com`) repo-içi `OPENCLAW-HERMES-ROADMAP.md` üzerinden eşleştirildi.
- "Sahip (rol)" alanları **operational role** isimleridir; PR açılırken tek bir kişi/agent atanır.

---

## 7. Çapraz Doküman Haritası

| Bu rapor | ↔ | Hedef |
|---|---|---|
| F- / N- | ↔ | `SYSTEM-AUDIT-REPORT.md` §3 (envanterin denetimci özeti) |
| F- / N- | ↔ | `MISSING-PARTS-PLAN.md` §3-7 (Phase'ler) |
| H- / D- | ↔ | `MISSING-PARTS-PLAN.md` §2 (Phase 0 hygiene) |
| Kapanan kalemler | ↔ | `MISSING-PARTS-PLAN.md` §2 "CLOSED date" ibaresi |
| TBD → canonical dönüşümleri | ↔ | `H-02` §6 log + commit hash |

Yeni kalem buraya eklendikçe, **diğer dokümanlarda çapraz-güncelleme** PR'a eklenir.

---

## 8. Kalem İstatistikleri (snapshot 2026-07-04)

- **Toplam kalem:** 43 (28 F + 6 N + 6 H + 3 D)
- **Open:** 28 (F=20, N=5, H=0, D=3)
- **In-progress:** 2 (F-03 tarayıcı sekmesi, F-13 society-of-agents)
- **Needs-design:** 2 (F-11 derin araştırma, F-12 widgets)
- **Done:** 6 (H-01 worktree, H-02 SPEC canonicalize, H-03 SPEC-IMPLEMENTATION-STATUS, H-04 CHANGELOG reorg planı, H-05 make check, H-06 .dev-home gitignore)
- **Deferred:** 5 (F-17 RBAC, F-21 widget market, F-22 widget scaffold, F-24 eval reflection, N-04 graveyard destructive)
- **Disk'teki en yüksek M-rapor:** M923 (CHANGELOG'da M1002 referansı var)
- **Toplam F-03 ile F-28'in dosya başına hit'i:** SYSTEM-AUDIT-REPORT §3'te doğrulanmış.

> **2026-07-04 spotlight düzeltmesi:** §1 Sayaç tablosu önceki yazımda F=24/2/0/2, N=5/0/0/1, H=0/0/6/0, D=2/0/0/0 olarak gösteriliyordu; toplam satırı (36/2/1/3/42) tutarsızdı. Gerçek değerler: F=20/2/2/0/4=28, N=5/0/0/0/1=6, H=0/0/0/6/0=6, D=3/0/0/0/0=3; toplam=43. `needs-design` sütunu eklendi.

---

*Bu envanter yaşayan dokümandır. Yeni kalem buraya eklendikçe karşılığı `SYSTEM-AUDIT-REPORT.md` §3 ile `MISSING-PARTS-PLAN.md` çapraz-güncellenir. Çift kayıt policy dışıdır.*
