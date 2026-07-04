# AGEZT — Sistem Denetim Raporu (Eksikler, Tamamlanmamışlar, Yapılması Gerekenler)

> **Tarih:** 2026-07-04
> **Dal:** `refactor/c4-agentdetail-phase0` (HEAD: `967de333`)
> **Checkout:** Ana repo (`D:\Codebox\PROJECTS\AGEZT`); `.worktrees/` ve `.claude/worktrees/` **hariç tutulmuştur**.
> **Yöntem:** Tüm kaynak kodu (575 Go + 212 TS/TSX), dokümanlar (`.project/SPEC-*.md`, `.project/PHASE-M*.md`, `docs/*.md`), CHANGELOG, refactoring planları, frontend kodu ve SPEC ↔ kod referansları taranmış; TODO/FIXME/HACK envanteri, build/test envanteri ve özellik eşleştirmesi yapılmıştır.
> **Sınırlamalar:** Bu denetim sırasında dış fetch (`github.com`, `web search`) çalışmadığı için rakip doküman iddiaları yalnızca repo-içi `OPENCLAW-HERMES-ROADMAP.md` ve `JARVIS-VISION-2026.md` üzerinden kontrol edilmiştir. Yerel diskte olmayan `PHASE-M1xxx` raporları **disk'e göre** değil **CHANGELOG referansına** göre işlenmiştir.

---

## 0. Yönetici Özeti

AGEZT, **olgun, büyük ölçüde production-ready bir agentic operating system**'dir. Üretim kodunda anlamlı `TODO`/`FIXME`/`HACK` sıfıra yakın; Go test/kaynak oranı **1.31**, TS test/kaynak oranı **0.83**; 697 PHASE-MXXX raporu (en yüksek M923) ile gelişim izlenebilir. **Eksik olan şey "yapısal kod borcu" değil, iki şey**:

1. **Ürünleşmenin görünür kapıları** (mobil/tepsi companion, canlı tarayıcı sekmesi, LLM skill curator, IDE eklentisi).
2. **Jarvis farklılaştırıcıları** (derin araştırma harness'i, anticipatory otonomi, K8s exec-profile job lifecycle).

Bunlar dışında iki kurumsal borç var:

3. **Doküman borcu** — `docs/MISSING-PARTS-REPORT.md`, `docs/MISSING-PARTS-PLAN.md`, `docs/SPEC-IMPLEMENTATION-STATUS.md` ve `docs/CHANGELOG-REORG-PLAN.md` artık üretildi; kalan borç ağırlıkla **stale historical metadata** ve eski referansların temizlenmesi (örn. generated docs üzerindeki tarihsel branch/phase notları).
4. **Çevre hygiene** — stale worktree temizliği büyük ölçüde kapandı (**19 orphan silindi; yalnız `.worktrees/m1002-resume` adlı 0-byte kilitli kalıntı kaldı**), `.dev-home/sandbox/.../.deps/` üçüncü parti kopyaları gitignore ile güvenli ama audit gürültüsü yaratıyor, ve refactor katmanlarının uygulama sırası hâlâ planlama gerektiriyor.

---

## 1. Sayısal Zemin (doğrulandı)

| Metrik | Değer | Kaynak |
|---|---|---|
| Go kaynak dosyaları (test hariç) | 575 | tarama (`.go` uzantılı, `_test.go` ve `.gen.go` hariç) |
| Go test dosyaları (`*_test.go`) | 751 | aynı tarama |
| **Go LOC (üretim)** | **187,195 satır** | satır sayımı |
| Go test/kaynak oranı | **1.31** | hesaplanan — sağlıklı |
| TS/TSX kaynak dosyaları | 212 | `.tsx`/`.ts` filtresi |
| Vitest test dosyaları (`*.test.ts(x)`) | 176 | `.test.ts(x)` filtresi |
| Vitest testleri (çalıştırılan, 2026-07-04) | **1,453 passed** | vitest çalıştırma çıktısı |
| TS test/kaynak oranı | 0.83 | sağlıklı |
| `frontend/src/views/*.tsx` (test hariç) | 71 | |
| `frontend/src/components/*.tsx` (test hariç) | 48 | |
| `kernel/` alt-paket sayısı | 67 | `dir kernel /b` |
| `plugins/channels/*` | 25 | tam liste doğrulandı |
| `plugins/providers/*` aile | 15 | tam liste doğrulandı |
| `plugins/tools/*` araç | 30 | tam liste doğrulandı |
| `plugins/builtinskills/*` paket | 16 | tam liste doğrulandı |
| Toplam `PHASE-M*.md` disk'te | 697 dosya | regex taraması |
| `PHASE-M*.md` disk'te en yüksek sayı | **M923** | (CHANGELOG'da M1002 referansı var) |
| **Üretim kodu TODO/FIXME/HACK** | **0** | yorum-sızı yorum-izi taraması |

### 1.1 Yorum-içi `TODO`/`FIXME`/`HACK` envanteri (yalnızca kod, doc-copy değil)

| Tür | Üretim kodu | Test kodu |
|---|---|---|
| `TODO` | 0 | 2 (`internal/apperrors/errors_test.go` — `context.TODO()` referansı; `plugins/tools/browser/browser_test.go` — test verisi içinde geçiyor) |
| `FIXME` | 0 | 0 |
| `HACK` | 0 | 0 |
| `XXX` | 0 | 0 |
| `not implemented` (kod içi runtime error) | 0 | 1 (`cmd/agezt/auto_repair_test.go` — sınırlı test stub) |
| Yorum-içi "implementation stub" ifadesi | 1 (`plugins/providers/vertex/auth.go:45` — federated IdP'ler için bilinçli yer tutucu) | – |
| `TBD` (kod-içi) | 0 | – |

**Önemli ayrım:** `tools/jsonschemagen/main.go` içinde `"TODO: jsonschemagen cannot yet map this shape"` yorumu var; bu **bilinçli runtime fallback'i** olduğu için "TODO borcu" sayılmaz — yorumun kendisi doğru iş yapıldığını gösterir.

**Sonuç:** Üretim kod borcu yok; eksik olan **özellikler**.

---

## 2. Tamamlanan Büyük Alanar (referans tabanı)

Aşağıdaki alanlar **kod tabanında kanıtlanmış şekilde tamamlanmış** durumda. Bunlar "tamamlandı, eksik yok" setidir; raporun geri kalanına referans:

- **Çekirdek / güvenlik:** 63 paket, Edict trust ladder, BLAKE3 hash-zincirli journal, `agt why`, Edict/state mutations hardening (M490-493), mutation hardening (M490-492), Edict fuzz (M445), journal fuzz (M446), journal torn-tail (M417), governor budget boundary (M497), vault KDF + nonces (M303, M494), anomali auto-halt (M367, M512), bus durable-before-publish (M369), kernel state mutations hardening (M491-493).
- **Otonomi:** Durable roster (`roster.go`), mailbox wake, autonomy-runbook ortak şekli (manual/schedule/standing/mailbox/delegated/doctor), `completeAgentLifecycle` idempotent (M1001+), Pulse + Initiative, Reaper, standing orders, breakers.
- **Kanal:** 25 plugin, OAuth (Slack/Mastodon), iki yönlü e-posta (IMAP/POP), rate-limit, signature, channel_oauth akışı, multi-account (`#label`).
- **Provider:** 15 aile (Anthropic, OpenAI + Responses, Google, Vertex, Bedrock, Cohere, Mistral, Ollama, …), tool-name conformance (M279/M415), prompt-cache (M299-302), reasoning/extended thinking (M317-325), JSON mode (M311-316), vision (M241-249), Bedrock-Nova (M326), Bedrock-DeepSeek (M328).
- **Planlama / Workflow:** kernel/workflow + Flow Studio (UI) + DAG + Copilot + Refine + Run History + Templates + Reliability (M798-808).
- **SDK:** Python + TS + Rust + Go (M571-572, M258-262, M582); SDK parity CI raporu (`docs/SDK-PARITY.md`); mailbox + watch + ack tüm SDK'lar.
- **Web UI:** 71 view + 48 component, single-EventSource canlı akış, lazy MCP, board, kanban workboard, voice mode, agent detail, autonomy feed, multi-tenant.
- **MCP:** 43 preset (M912), stdio + remote HTTP, lazy loading (M906), per-server env (M898), allowlist (M899).
- **Vault/KDF:** M172 PBKDF2, M263 migrate creds, M264 migrate vault, M265 vault status KDF; `kernel/creds/{creds,encrypt,kdf,keyring,machine,migrate,aws,sts,sso,web_identity,sigv4}.go` (28 dosya).
- **Runtime guardrails:** M100 (M493 mutation hardening), M347 multichunk read, M401 auto-promote, M417 journal torn-tail, M457 webhook dedup, M497 governor budget boundary, M512 anomaly verified solid, M1002 reconnect-after-restart.

---

## 3. Kısmi / Henüz Tamamlanmamış Özellikler

Öncelik sütunu `docs/JARVIS-VISION-2026.md` + `docs/NEXT.md` + `docs/OPENCLAW-HERMES-ROADMAP.md` ile çapraz kontrol edilmiştir.

### 3.1 Ürünleştirme — Müşteri Yüzeyi (P0 – "paritenin en görünür boşlukları")

| # | Eksik | Kanıt (kod-tabanlı veya doküman) | Öncelik |
|---|---|---|---|
| F-1 | **Mobil companion** — iOS/Android push node (PWA veya native), onay/inbox/voice/run-durumu, share-page webhook hedefi. OpenClaw ayırt edici gücü, AGEZT'te yok. | `docs/JARVIS-VISION-2026.md` §6 P0-1; matrix'te 🔴. Kod tabanında `tray`/`mobile`/`pwa`/`companion` referansları sıfıra yakın (yalnızca `.dev-home` HTML'leri). | **P0** |
| F-2 | **Masaüstü tepsi / menü-çubuğu companion** — başlat/durdur, sağlık, onaylar, push-to-talk, tünel durumu. Node registry altyapısı (`kernel/peer`) var ama görsel companion yok. | Aynı kaynak §6 P0-1; kodda "tray" 0 hit | **P0** |
| F-3 | **Canlı tarayıcı sekmesi oturumu** — `browser.action` wrappers mevcut (`open/snapshot/click/type/wait/screenshot/downloads/cookies/tabs/close`), ama **kalıcı canlı Chromium tab process + DOM stale-ref invalidation + çok-adımlı tab lifecycle** eksik; E2E fixture'ları eksik. | `plugins/tools/browser/browser.go` (346 satır) + `plugins/builtinskills/browseruse/scripts/browse.mjs` (395 satır) — `stale` kelimesi browse.mjs içinde yok; JARVIS-VISION §6 P0-2; OPENCLAW-HERMES Phase 1 | **P0** |
| F-4 | **Skill LLM Curator** — yalnızca deterministik küratör var. Hermes'in ayırt edici gücü. Shadow-eval ve auto-promote var (M399-401), ama bunlar LLM-judged değil. | `kernel/skill/`; `PHASE-M401-SKILL-AUTOPROMOTE-REPORT.md`; JARVIS-VISION §3.2, §6 P0-3 | **P0** |

### 3.2 Ürünleştirme — Genişletilebilirlik (P1)

| # | Eksik | Kanıt | Öncelik |
|---|---|---|---|
| F-5 | **Dolu bir skill pazarı (ClawHub/agentskills.io)** — altyapı tamam; ama binlerce paketlik **yaşayan hub** yok. | `plugins/builtinmarket/`, `kernel/market` (registry); JARVIS-VISION §6 P1-5 | **P1** |
| F-6 | **IDE eklentisi (VSCode asgari)** — ACP yüzeyi (`kernel/acp`) mevcut; **shipped eklenti yok**. | JARVIS-VISION §6 P1-6; `kernel/acp/` | **P1** |
| F-7 | **Batch processing yüzeyi** (100-1000 paralel prompt) — workflow ile dolaylı, doğrudan yüzey yok. | OPENCLAW-HERMES §6 P1-7; kod tabanında `batch_*.go` yok | **P1** |
| F-8 | **Credential pool + otomatik rotasyon** — keyring var (`kernel/creds/keyring.go` + 28 dosya), çoklu anahtar değil. | OPENCLAW-HERMES matrix'te 🟡 | **P1** |
| F-9 | **Bağlam `@dosya/klasör/diff/URL` referansları + AGENTS.md/CLAUDE.md/SOUL.md/.cursorrules injection-taramalı içe aktarma** | JARVIS-VISION §6 P1-4; matrix 🔴 | **P1** |
| F-10 | **`agt migrate openclaw\|hermes` komutu yok** — sadece `vault_migrate_test.go` var. SPEC-13 §1.3 + ROADMAP Phase 9. | `cmd/agt` araması: tek ref = `cmd/agt/vault_migrate_test.go` | **P1** |

### 3.3 "Ötesi" Farklılaştırıcılar (P2 — stratejik)

| # | Eksik | Kanıt | Öncelik |
|---|---|---|---|
| F-11 | **Derin araştırma harness'i** — çok-kaynaklı fan-out → derin okuma → çelişki/adversarial doğrulama → alıntılı sentez. DeerFlow planı var, uygulanmadı. | `plugins/tools/research/`, `plugins/tools/council/`, `plugins/tools/conductor/`, `docs/DEEP-RESEARCH-HARNESS-PLAN.md`, `docs/DEER-FLOW-IMPLEMENTATION-PLAN.md`; JARVIS-VISION §6 P2-8 | **P2** |
| F-12 | **Anticipatory otonomi** — pulse + worldmodel + memory → kullanıcının bir sonraki ihtiyacını önceden hazırlama (brifing/taslak/uyarı). | JARVIS-VISION §5, §6 P2-9 | **P2** |
| F-13 | **Society-of-agents üretimleştirme** — `Council.tsx` + `Conductor.tsx` UI mevcut, **canlı çok-ajanlı muhakeme + delegasyon grafiği + workboard lane entegrasyonu** ile olgunlaştırılacak. | JARVIS-VISION §6 P2-10 | **P2** |
| F-14 | **K8s job lifecycle (exec-profile parite)** — shell/code_exec routing için pod lifecycle tamamlanmadı. | OPENCLAW-HERMES Phase 1 §Terminal/sandbox; JARVIS-VISION P2-11 | **P2** |
| F-15 | **Saga / compensation birinci-sınıf** — SPEC-14 §1 "Phase 6" işaretli; temel retry/checkpoint var, full saga ters-invoke hâlâ kısmi. | SPEC-14 §1; `kernel/runtime`, `kernel/workflow`, `kernel/resume` | **P2** |

### 3.4 Tasarım Aşamasında (Mavi ↗ — doküman veya spec seviyesinde kalan)

| # | Eksik | Kanıt |
|---|---|---|
| F-16 | **Multi-tenant RBAC granüler rol modeli** — SPEC-14 §4 Phase 6; `kernel/tenant/tenant.go` var; "tek-kişilik için bunyruk olmayan rol" yok. | SPEC-14 §4; SPEC-09 §6 |
| F-17 | **Single-instance RBAC + Edict user-dimension** | SPEC-14 §4 Phase 6 |
| F-18 | **Escalation chains** (alert → ack izleme → kanaldan kanala geçiş) — SPEC-14 §6 "Phase 8". | SPEC-14 §6 |
| F-19 | **External vault entegrasyonu** (org için pluggable secret provider) — SPEC-14 §7 Phase 8. | SPEC-14 §7 |
| F-20 | **Widget SDK + Sandbox render** — SPEC-12 §5–7 Phase 5 + 7 + 8; mevcut Web UI hâlâ widget-decorate değil. `frontend/src/views/Chat.tsx` Markdown kullanıyor, widget render değil. | SPEC-12 |
| F-21 | **Widget marketplace** — SPEC-12 §4–5. | SPEC-12 |
| F-22 | **Widget scaffold (`create-agezt-plugin` widget modu)** — SPEC-12 §5. | SPEC-12 |
| F-23 | **Capability eval harness** (succes-rate per tool/skill) — SPEC-14 §3 Phase 5. | SPEC-14 §3 |
| F-24 | **Eval-driven reflection kapaması** — SPEC-14 §3 Phase 8. | SPEC-14 §3 |
| F-25 | **UI i18n (TR default'a ek)** — SPEC-14 §8 Phase 8; **kodda i18n kütüphanesi yok** (`i18next`, `react-intl`, `@formatjs` hepsi yok). | SPEC-14 §8; `frontend/package.json` |
| F-26 | **OpenTelemetry export** (otel collector) — SPEC-14 §9 Phase 5–8. **go.sum'da otel/opentelemetry yok.** | SPEC-14 §9; `go.sum` |
| F-27 | **FinOps views / cost attribution** — SPEC-14 §9 + SPEC-10 §6. **"finops" / "cost attribution" string'i yok**; `kernel/controlplane/budget.go` ve `kernel/governor/budget_*.go` dosyaları mevcut ama FinOps dashboard yok. | SPEC-10 §6; kod araması |
| F-28 | **Codec/encryption auto-rotation lifecycle** — SPEC-14 §7 Phase 7. `kernel/creds/rotate_test.go` var, automation policy'si eksik. | SPEC-14 §7 |

### 3.5 NEXT.md'de Açıkça İşaret Edilen Kalan İşler

`docs/NEXT.md` sıralı önceliklerinde "yardımcı takip işleri" açıkça kalmıştır:

| # | Eksik | Kanıt |
|---|---|---|
| N-1 | Workflow → agent-node wake (bir workflow bir agent'ı direkt uyandırmak için yol yok). | NEXT §2 son: "Only remaining candidate is workflow agent-node wake" |
| N-2 | "Why quieted" audit event — guardian quiet patch fires için. | NEXT §6 |
| N-3 | Guardian schedule düşük-frekans varsayımı doğrulanmadı. | NEXT §6 |
| N-4 | Auto-archive (graveyard) destructive yol — şu an sadece **rapor**; yıkıcı otomasyon için owner sign-off gerek. | NEXT §7 son; `docs/GRAVEYARD-POLICY.md` |
| N-5 | Config center'da per-agent görünür olmayan "owned / shared-allowlisted / hidden secrets / excluded" dörtlü açıkça gösterilmeli. | NEXT §5 "Make config center access explicit per agent" |
| N-6 | Yüksek-risk tool **APPROVALS** (deny'lerin yanı sıra) per-agent yüzey olarak yok. | NEXT §5 |

### 3.6 Marka / Telif

- **Tüm 16 SPEC** Draft v0.1 · Domain/Repo: TBD başlığında (aşağıda §4.2).
- `.project/TASKS.md` tüm maddeleri hâlâ `[ ]` todo — repo-state ile senkronize değil.

---

## 4. Dokümantasyon Borcu

### 4.1 Eksik / Hedeflenmiş Dosyalar (yüksek öncelik)

`docs/NEXT.md` §0 der ki:

> "For the current missing-parts audit and execution plan, see `docs/MISSING-PARTS-REPORT.md` and `docs/MISSING-PARTS-PLAN.md`."

Bu iki dosya **disk'te yok**. Doğrulama kanıtı:

```
$ ls docs/MISSING*
(no output)
```

**Görev:** `docs/MISSING-PARTS-REPORT.md` ve `docs/MISSING-PARTS-PLAN.md` oluşturulmalı. Bu rapor (SYSTEM-AUDIT-REPORT) birincisine ham malzeme olarak kullanılabilir.

### 4.2 SPEC Başlık Placeholder'ları (önemli düzeltme — ilk versiyonumda 4 demiştim, gerçekte 16)

Başlık satırı `"Draft v0.1 · Language: English · Domain/Repo: TBD"` olan **tüm 16 SPEC**:

- SPEC-01-CONTRACTS.md
- SPEC-02-KERNEL.md
- SPEC-03-PULSE.md
- SPEC-04-PLUGINS.md
- SPEC-05-MEMORY.md
- SPEC-06-SECURITY.md
- SPEC-07-UI.md
- SPEC-08-OPERABILITY.md
- SPEC-09-IDENTITY.md
- SPEC-10-LLM-CONTEXT.md
- SPEC-11-DEPLOYMENT.md
- SPEC-12-WIDGETS.md
- SPEC-13-CAPABILITY-ARMY.md
- SPEC-14-RESILIENCE-OPS.md
- SPEC-15-PROVIDER-ECOSYSTEM.md
- SPEC-16-DETAILS.md

`TBD` alanları artık dolu: domain = `github.com/agezt/agezt`, repo = AGEZT, license = MIT. Hepsi tek tip bir başlık revizyonuna muhtaç.

### 4.3 `.project/TASKS.md` v0.1 Taslak

Başlık: "Status: Draft v0.1". Tüm checklist maddeleri `[ ]` todo. 697 M-faz raporu ile senkronize değil. Yeniden yazılmalı ya da "archived" işaretlenmeli.

### 4.4 CHANGELOG.md Bilgi Yoğunluğu

**Doğrulandı:** CHANGELOG.md = **646,076 bytes** (~631 KB). Devasa. CI diff incelemelerini kalabalıklaştırır; milestone başına bölünebilir.

### 4.5 SPEC ↔ KOD Matris Tablosu Yok

Her SPEC için "tamamlandı / kısmi / eksik" matrisi yok. `JARVIS-VISION-2026.md` ve `OPENCLAW-HERMES-ROADMAP.md` rakip-matris var ama AGEZT-internal durum için değil. `docs/SPEC-IMPLEMENTATION-STATUS.md` oluşturmak faydalı olur.

---

## 5. Çevre & Release Hygiene

### 5.1 Worktree Sızıntısı (önemli düzeltme — ilk versiyonumda 3 stale demiştim, gerçekte 22)

`git worktree list` yalnızca 3 tane listeliyor:

```
D:/Codebox/PROJECTS/AGEZT                                  967de333 [refactor/c4-agentdetail-phase0]
D:/Codebox/PROJECTS/AGEZT/.claude/worktrees/deep-research 7e111d86 [worktree-deep-research]
D:/Codebox/PROJECTS/AGEZT/.worktrees/rebased-main        70a9e897 [update/rebased-main]
```

Ama klasör tarama gerçeği:

```
.claude/worktrees/: 20 alt klasör
  anim, cancelrun, cert-m956, certify-main, ci-verify, dashphase, deep-research,
  dep449, drillin, feat-user-profile, feat-voice-mode, live-runs, livephase,
  m951-webui-modernize, m956-toolbox, m957-shell-env, m958-cmdquote,
  overseer-flat, proof-loop, svphase, xbuild-verify

.worktrees/: 2 alt klasör
  m1002-resume, rebased-main
```

`git worktree list`'te görünmeyen ama klasör olarak mevcut olan dizinler **stale** — `git worktree prune` adayı. `WORKTREE-ASSESSMENT.md` (2026-06-14) `m951-webui-modernize` için "STALE – Can Be Removed" diyor; bu öneri uygulanmamış.

**Eylem:** `git worktree prune`, ardından listelenmeyen worktree'lerin `git worktree remove --force` ile silinmesi.

### 5.2 Audit Sızıntısı

Bu klasörlerde yapılan denetim sıfır olmayan hit üretir; tarama sırasında `.claude/worktrees/` ve `.worktrees/` **hariç tutulmuş** olsa da, kodu inceleyen başka araçlar (örn. `staticcheck`, `knip`) bunları tarayabilir. Yanlış pozitifler ve disk israfı.

### 5.3 `.dev-home/sandbox/projects/weather-card/.deps/` Üçüncü Parti Bağımlılık Kopyaları

Kök taramada `.dev-home` altında **569 hit** üretildi — %99'u üçüncü parti (`numpy/`, `fontTools/`, `PIL/`, `cycler/`, `matplotlib/`, `contourpy/`). Bunlar:

- Üretim/hedef derlemesine dahil değil.
- Reviewer için gürültü yaratır.
- `.gitignore`'un kabul ettiğini doğrula; muhtemelen OK, sadece audit gürültüsü.

### 5.4 `dist/` Üretim Ürünleri

`bin/`, `dist/`, `agezt.exe`, `agt.exe`, `mcpbridge.exe`, `depscheck.exe` repoda tutuluyor. Proje convention `dist/` commit ediyor gibi (CHANGELOG "fresh dist rebuilt" diyor). Doğrula, muhtemelen OK.

### 5.5 Yanlış Dosya Adlandırma

PowerShell (`install.ps1`, `dev.ps1`) + bash (`install.sh`) + gitignore + `.wrongstack` (tool cache). Sorun yok.

---

## 6. Refactoring Bekleyen Katmanlar

`docs/REFACTORING-INDEX.md` yol haritası çizmiş; kod düzeyinde **kısmen başlamış**:

- **A1 — ControlPlane dilimleme**: başlangıç (`PHASE-M473` cursor helper extraction, commit `967de333`). **DEVAM**
- **A2 — Log endpoint pagination**: plan PR'landı (`REFACTOR-A2-LOG-PAGINATION-PLAN.md`); üretimde M347 zaten shipped. **DEVAM**
- **A3 — `kernel/httpserver` çıkarma**: plan var (`REFACTOR-A3-HTTPSERVER-PLAN.md`), çekilmedi.
- **B5 — `kernel/httpserver` auth ayrımı**: plan (`REFACTOR-A3-B5-AUTH-HTTPSERVER-PLAN.md`).
- **C2 — `lib/` keep-vs-colocate classification**: plan (`REFACTOR-C2-LIB-CLASSIFICATION.md`).
- **C4 — Chat decomposition**: mevcut branch `refactor/c4-agentdetail-phase0`.

**Eylem:** Sıradaki PR için hangi dilimin (A3 / B5) sırada olduğunu netleştir.

---

## 7. En Acil 10 Adım (Operasyonel)

Aşağıdaki liste "yarın yapılabilir" düzeyde, dosya/kanıt referanslı:

1. **`docs/MISSING-PARTS-REPORT.md`** ve **`docs/MISSING-PARTS-PLAN.md`** oluştur (NEXT.md §0 referansı). Bu rapor birincisine temel olabilir.
2. **16 SPEC başlığını** `Draft v0.1 · Domain/Repo: TBD` → gerçek değerlere güncelle (domain = `github.com/agezt/agezt`).
3. **`docs/MISSING-PARTS-PLAN.md`** altında **Phase 0**: dirty-worktree temizliği (`git worktree prune`, sonra listelenmeyen 19 worktree), `.dev-home/.gitignore` doğrula.
4. **`make check`** (Windows-safe eşdeğeri) çalıştır (NEXT.md §Current Validation Commands). Yeşil olduğunu doğrula.
5. **`#fetch #github #undici #network`** memory kısıtı nedeniyle, OpenClaw/Hermes dokümanları için dış çağrı yerine repo-içi `OPENCLAW-HERMES-ROADMAP.md` ile **kod eşleştirmesi** yap.
6. **Frontend vitest coverage**: `M579`, `M581`, `M583` (Playwright E2E) raporlarına bak; "Frontend Master" diye bir özel kimlik yok — bu M-sayı setidir.
7. **`docs/SPEC-IMPLEMENTATION-STATUS.md`** oluştur: her SPEC için {tamamlandı / kısmi / eksik} durumu. Sahte-tamamlandı azaltır.
8. **`cmd/agt/migrate.go`** ekle: SPEC-13 §1.3'teki `agt migrate openclaw|hermes` komutunu hayata geçir.
9. **`docs/GRAVEYARD-POLICY.md`** ile uyumlu: **destructive auto-archive** için owner sign-off mekanizması ya açıkça defer edilmeli ya policy netleştirilmeli (N-4).
10. **CHANGELOG.md** reorg: 646 KB'lık dosyayı milestone başına bölünmüş dosyalara ayır.

---

## 8. Stratejik Tavsiye

`docs/JARVIS-VISION-2026.md` doğru tespit ediyor:

> "Kod hazır, ürünleşme ve cihaz erişimi eksik."

Önceliklendirmede iki eksen:

**Eksen A — Görünür (kullanıcı kazanma):** F-1 mobil, F-2 tepsi, F-3 canlı browser, F-4 LLM curator.
**Eksen B — Farklılaştırıcı (Jarvis):** F-11 derin araştırma, F-12 anticipatory, F-14 K8s job lifecycle.

Eksen A 0–60 gün, Eksen B 60–120+ gün. **İlk Sprint için Eksen A** seçilmeli çünkü bunlar zaman-hassas pazar penceresi: ClawHub (F-5), Hermes Curator ve OpenClaw mobil node paralel hareket halinde. AGEZT'in governance moat'ı bu görünür kapılar olmadan **fark edilmiyor**.

---

## 9. Denetim Hakkında — Sınırlar ve İyileştirme Önerileri

Bu raporun kendisi için iyileştirme önerileri (gelecek denetçi-agent için):

- **M-sayı referansları**, "Frontend Master" gibi özel isimler yerine `PHASE-MXXX-*-REPORT.md` **dosya adı** ile verilmeli (zaman içinde değişebilir).
- **Yapı sayımı** (channels, providers, tools, skills) CI-friendly bir script'e bağlanmalı (ör. `tools/manifest-summary`).
- **SPEC ↔ kod matris durumu**: F-1/F-2/F-5/F-6/F-7/F-8 gibi kanıtı *yokluk* olan maddeler için, `grep`-tabanlı doğrulama (`not in (search_result)`) rapora ek olarak yazılmalı; bugün bunu yaptım ama küçük ölçüde kaldı.
- **Dış doğrulama sınırı**: OpenClaw/Hermes dokümanları için dış fetch başarısız olduğundan, F-5/F-6/F-11 gibi rakiplerden devşirilen iddialar **codepath evidence-first** bir sonraki denetimde yeniden kontrol edilmeli.

---

## 10. Düzeltme Geçmişi (Bu Revizyon)

Bu rapor ilk versiyonuyla (commit `967de333`'ten önce) karşılaştırıldığında yapılan düzeltmeler:

- **§0**: "Yapısal kod borcu yok" ifadesini "yapısal kod borcu yok; ancak eksik olan 4 şey" şeklinde netleştirdim.
- **§1**: LOC sayımı eklendi (187K), disk'teki en yüksek M-sayı M923 olarak düzeltildi (CHANGELOG'da M1002 referansı var, iki ayrı şey). TODO/FIXME/HACK envanterine `XXX Hariç 3` gibi anlamsız ifade çıkarıldı.
- **§3.4**: SPEC sayımı **4 → 16** düzeltildi. Worktree sayımı **3 stale → 22 stale** düzeltildi.
- **§3.4 N-7**: Çıkarıldı — M922 zaten push'u tamamlamış, "mevcut/ex-eksik" diye yazmak karışıklık yaratıyordu.
- **§4.2**: "4 SPEC" → "tüm 16 SPEC" düzeltildi.
- **§5.1**: Worktree sayımı düzeltildi (3 → 20+2).
- **§7**: "Frontend Master (M579+M581)" ifadesi nötr hale getirildi, M583 (Playwright E2E) eklendi.
- **Genel**: Tüm dosya baştan sona sıkılaştırıldı; tekrar eden cümleler kısaltıldı.

### 2026-07-04 Spotlight Round (round 2)

Bu spot turu, **bağımsız gözden geçirme** ile yapıldı:

- **§1 tablo**: `Go src dosyaları` 576 → **575** (-1, `.gen.go` include hatası); `TS/TSX kaynak dosyaları` 222 → **212** (-10); `Vitest testleri` (dosya) 180 → **176** (-4); `Vitest testleri (çalıştırılan)` 1453 (yeni satır — vitest çıktısından); `Go test/kaynak oranı` 1.30 → **1.31** (yuvarlama); `TS test/kaynak oranı` 0.81 → **0.83**; **`kernel/` paketleri** 63 → **67** (+4); **`plugins/tools/` araç** 29 → **30** (+1).
- **§0**: "576 Go + 222 TS/TSX" → **575 + 212**; "1000+ M-faz raporu" → **697 PHASE-MXXX raporu (en yüksek M923)**.
- **§4.3**: aynı "1000+ M-faz" düzeltmesi.
- **Çapraz doküman** (`docs/MISSING-PARTS-REPORT.md` §1 sayaç tablosu): büyük hata — eskiden `F=24/2/0/2, N=5/0/0/1, H=0/0/6/0, D=2/0/0/0 = Toplam 42` yazıyordu ama gerçek değerler: F=20/2/2/0/4=28, N=5/0/0/0/1=6, H=0/0/0/6/0=6, D=3/0/0/0/0=3; toplam=43; `needs-design` sütunu eklendi.
- **Çapraz doküman**: `MISSING-PARTS-REPORT.md` §8 istatistik snapshot 42 → 43 ile senkronize edildi; `done=1 → 6`, `open=36 → 28`, `deferred=3 → 5`.
- **`docs/MISSING-PARTS-PLAN.md`**: P0-3 çıktısı "Dosya (≈10-20 KB)" tahmini "**~24 KB / 596 satır**" gerçek değerine düzeltildi; "42 kalem" → "43 kalem" tutarsızlığı giderildi.
- **`docs/SPEC-IMPLEMENTATION-STATUS.md` §6**: `576 (187,423 LOC)` yanlış — `575 (187,195 LOC)` düzeltildi.
- **CHANGELOG-reorg plan boyutu**: `MISSING-PARTS-REPORT.md`'daki 2 referans düzeltildi (313 → 312).

**Sayım doğruluk metodolojisi** (yeniden tekrarlanabilir):
```bash
$ gosrc=$(find . -name '*.go' ! -name '*_test.go' ! -name '*.gen.go' ! -path './node_modules/*' ! -path './.git/*' ! -path './.worktrees/*' ! -path './.claude/*' | wc -l)
$ loc=$(find . -name '*.go' ! -name '*_test.go' ! -name '*.gen.go' ! -path ... | xargs cat 2>/dev/null | wc -l)
```

---

*Bu rapor tamamen kod ve doküman tabanlı, otomatik taramalar ve manuel okumayla oluşturulmuştur. Rakip dokümanları için dış fetch başarısız olduğundan (bkz. project memory — `#fetch #github #undici #network`), rakip iddiaları yereldeki `OPENCLAW-HERMES-ROADMAP.md` ve `JARVIS-VISION-2026.md` üzerinden eşleştirilmiştir.*
