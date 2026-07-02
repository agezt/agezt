# AGEZT → Jarvis: Stratejik Durum ve Yol Haritası Raporu

> Tarih: 2026-07-02
> Kapsam: Tüm proje (Go çekirdek + CLI + gömülü React web konsolu) OpenClaw ve
> Hermes Agent'e karşı; "her ikisinin yapabildiğini yap, ikisinin de ötesinde
> bir Jarvis ol" hedefi doğrultusunda.
> Yöntem: İddialar doküman değil **kodla doğrulandı** (iki bağımsız keşif taraması
> + rakip resmi dokümanları). Sayılar bu checkout'tan alınmıştır.

---

## 1. Yönetici Özeti

**Sonuç:** AGEZT bir "demo iskeleti" değil; gerçekten uygulanmış, olağanüstü geniş
bir *agentic operating system*. Ölçek doğrulandı:

- ~**170.000 satır** Go (test hariç), 5.583 `.go` dosyası, **63 kernel paketi**
- **25 kanal**, **15 sağlayıcı ailesi**, **29 tool**, **16 yerleşik skill paketi**, **~70 CLI komut grubu**
- Gömülü React konsolu: **67 gerçek view**, ~73.000 satır TS/TSX, **~473 Vitest testi** (view başına ~1:1 kapsam), tek-EventSource canlı akış
- **Sıfır** `panic("unimplemented")`, çekirdekte parmakla sayılır TODO — placeholder ekran yok

**Rakiplere karşı konum:**

- **Yönetişim / denetlenebilirlik / geri-alınabilirlik** (Edict politikası, BLAKE3
  hash-zincirli journal, `agt why`, rollback, kalıcı ajan kimliği): AGEZT **her
  iki rakibin de açıkça önünde**. Bu, güvenilir bir Jarvis'in temel taşı.
- **Kanal / sağlayıcı / tool / workflow / zamanlama / bellek genişliği**: **parite
  veya üstü**.
- **Proaktiflik (Pulse + Initiative), dünya modeli (worldmodel), çoklu-ajan
  konsey/kondüktör**: AGEZT'te **var**, rakiplerde ya yok ya zayıf — gerçek
  farklılaştırıcı zemin.

**Kapatılması gereken paritenin altındaki boşluklar (öncelik sırasıyla):**

1. **Yerel cihaz deneyimi** — mobil (iOS/Android) + masaüstü tepsi/menü-çubuğu
   companion'ı yok. OpenClaw'ın en görünür üstünlüğü. *(kendi paritel deftercinde
   "missing")*
2. **Populer/dolu bir skill pazarı** — altyapı var, ClawHub gibi binlerce paketli
   yaşayan bir hub yok.
3. **Canlı sürekli tarayıcı sekmesi** — Playwright gerçek ama kalıcı çok-adımlı
   canlı sekme + DOM stale-ref oturumu değil (kısmi).
4. **LLM tabanlı skill curator** — Hermes'in otomatik konsolidasyonu; AGEZT'te
   yalnızca deterministik küratör var.
5. **Bağlam ergonomisi** — `@dosya/klasör/diff/URL` referansları ve
   AGENTS.md/CLAUDE.md/SOUL.md içe aktarımı birinci sınıf değil.

**Paritenin ötesine geçiren en büyük tek fırsat:** **derin araştırma harness'i
yok** (kodda doğrulandı — hiçbir çok-kaynaklı, çelişki-doğrulamalı araştırma motoru
yok). Bir Jarvis için bu, proaktiflik kadar kritik ve rakiplerin de zayıf olduğu
bir alan.

---

## 2. Doğrulanmış Mevcut Durum (kanıt tabanlı)

### 2.1 Otonomi yığını — hepsi gerçek çoklu-dosya kod

| Yetenek | Kanıt | Olgunluk |
|---|---|---|
| Bellek (vektör + konsolidasyon + retention + profil) | `kernel/memory/` (19 dosya) | Olgun |
| Dünya modeli (varlık/ilişki + decay + resolve) | `kernel/worldmodel/` (10) | Gerçek, **farklılaştırıcı** |
| Pulse (proaktif motor: briefing/health/observers/salience/reaper) | `kernel/pulse/` (18) | Olgun |
| Pulse Initiative (gözlem → aksiyon, yönetişimli) | `pulse-initiative` (M999) | Gerçek, **farklılaştırıcı** |
| Standing orders (cron tetiklemeli daimî talimatlar) | `kernel/standing/` (6) | Gerçek |
| Cadence / typed schedules | `kernel/cadence/` (9) + `kernel/scheduler/` | Gerçek |
| Workflow engine (DAG + template) | `kernel/workflow/` (7) + FlowStudio | Gerçek |
| Guardians / self-repair / watchdog | `plugins/builtinguardians/` + `cmd/agezt/{auto_repair,watchdog}.go` | Gerçek |
| Voice (STT/TTS: cartesia/deepgram/elevenlabs + hands-free mod) | `kernel/stt/` + `plugins/providers/voice/` + `Voice.tsx` | Gerçek |
| Kullanıcı profili (operatörü öğrenme) | `user-profile` (M1000) | Gerçek, **farklılaştırıcı** |
| Yönetişimli otonomi (trust tavanı, edict, onaylar) | `kernel/controlplane/autonomy.go` + `kernel/edict/` | Gerçek, **moat** |

### 2.2 Tarayıcı otomasyonu — gerçek Playwright (yalnızca fetch değil)

- `browser.read`: SSRF-korumalı stdlib fetch (netguard, her yönlendirme atlamasında
  allowlist, 4MB cap). Ucuz varsayılan.
- `browser.action`: **gerçek headless Chromium Playwright köprüsü**
  (`plugins/builtinskills/browseruse/scripts/browse.mjs`, 395 satır). 4 profil
  (isolated/session/user-attached/remote-cdp), fiil tool'ları
  (`browser.open/snapshot/click/type/wait/screenshot/downloads/cookies/tabs/close`),
  ekran görüntüsü + indirme → artifact store. Node/Playwright kurulumu gerektirir
  (opt-in). **Eksik:** kalıcı canlı-sekme süreci + DOM stale-ref invalidation.

### 2.3 Web konsolu — sevk edilebilir, geniş, placeholder yok

67 view; hepsi canlı API/SSE'ye bağlı. Dikkat çekenler: `Jarvis.tsx` (ortam
asistanı), `Council.tsx` + `Conductor.tsx` (çoklu-ajan orkestrasyon), `Voice.tsx`,
`Overseer.tsx`, `Workboard.tsx`, `World.tsx`, `Autonomy.tsx`. React 19 + Tailwind
v4 (oklch token), `@xyflow/react` grafikler, kendi chart kütüphanesi, custom router.
Çift-tema tutarlı tasarım sistemi. ~473 curated test.

---

## 3. Rakip Analizi

### 3.1 OpenClaw (kişisel AI asistan gateway'i — "the lobster way")

Güçlü olduğu yer: kişisel gateway paketlemesi + cihaz deneyimi + skill dağıtımı.

- **Kanallar:** Discord, iMessage, Signal, Slack, Telegram, WhatsApp, WebChat +
  eklenti (Matrix, Teams, Twitch, Nostr, Zalo…)
- **Tarayıcı:** gerçek Chrome/CDP (Playwright), JS-ağır siteler, oturumlu login,
  çok-adımlı akışlar
- **Skills + ClawHub:** SKILL.md tabanlı, **binlerce skill'lik** kamu pazarı
- **Bellek:** Markdown + arama backend'leri + Honcho
- **Otomasyon:** cron + heartbeat scheduler, standing orders, taskflow
- **Medya/ses:** üretim + anlama + TTS/STT (çok sağlayıcılı)
- **35+ sağlayıcı**, çoklu web-arama sağlayıcısı (Brave/DDG/Exa/Tavily)
- **CİHAZLAR (ayırt edici):** iOS/Android node'ları (kamera, cihaz komutları,
  ekran kaydı, konum) + macOS menü çubuğu companion'ı + Windows Hub
- Ev otomasyonu, local-first gizlilik, DM allowlist/pairing güvenliği, çoklu-ajan
  yönlendirme

### 3.2 Hermes Agent (NousResearch — agent CLI/gateway)

Güçlü olduğu yer: skill self-improvement + terminal backend ergonomisi + dayanıklı
çoklu-ajan Kanban.

- **Skills + Curator:** ajanların ürettiği skill'leri periyodik **LLM ile** gözden
  geçiren, budayan, konsolide eden, active→stale→archived taşıyan yardımcı-model job'u
- **Bellek:** built-in + Honcho + Mem0 + RetainDB (pluggable)
- **Bağlam dosyaları:** `.hermes.md`, `AGENTS.md`, `CLAUDE.md`, `SOUL.md`,
  `.cursorrules` otomatik keşfi; **`@` bağlam referansları** (dosya/klasör/diff/URL)
- **Checkpoints & rollback:** dosya değişikliği öncesi otomatik snapshot + `/rollback`
- **Kanban (ayırt edici):** her proje için ayrı SQLite board; profil lane'leri,
  bağımlılıklar, heartbeat, comment, blocking, crash recovery; `kanban_*` toolset
- **Batch processing:** yüzlerce/binlerce prompt'u paralel
- Subagent delegasyonu, sandbox'lı Python kod yürütme (programatik tool erişimi),
  event hooks, voice, tarayıcı (Browserbase/BrowserUse/local CDP), vision/image paste,
  görsel üretim (FAL.ai 9 model), TTS (10 sağlayıcı), MCP, provider routing/fallback,
  **credential pools**, prompt caching, OpenAI-uyumlu API sunucusu,
  **IDE entegrasyonu (VSCode/Zed/JetBrains via ACP)**, SOUL.md kişilik, skin/tema, plugin

---

## 4. Karşılaştırma Matrisi

Efsane: 🟢 AGEZT önde · 🟡 parite · 🔴 AGEZT geride

| Alan | AGEZT | OpenClaw | Hermes | Durum |
|---|---|---|---|---|
| Yönetişim/politika (Edict) | ✔ çekirdek | zayıf | orta (hooks) | 🟢 |
| Denetim izi (hash-zincir journal + `why`) | ✔ | log | log | 🟢 |
| Kalıcı ajan kimliği | ✔ roster | session | profile | 🟢 |
| Geri-alma/rollback | ✔ (dosya/skill/workflow/config) | kısmi | ✔ (/rollback) | 🟡 |
| Typed schedules + standing | ✔ | ✔ | ✔ | 🟡 |
| Workflow/DAG motoru | ✔ + görsel canvas | Lobster | — | 🟢 |
| Bellek | ✔ vektör+konsolidasyon | Markdown/Honcho | pluggable | 🟡 |
| Dünya modeli | ✔ | — | — | 🟢 |
| Kanallar | 25 | ~7+eklenti | mesajlaşma | 🟢/🟡 |
| Sağlayıcılar | 15 aile | 35+ | çok + pools | 🟡 |
| Tarayıcı otomasyonu | Playwright (opt-in) | Chrome/CDP her zaman | çok backend | 🟡 |
| Skill lifecycle + forge | ✔ içerik-adresli | SKILL.md | ✔ | 🟢 |
| Skill **LLM curator** | deterministik | — | ✔ LLM | 🔴 |
| Skill **pazarı (dolu hub)** | altyapı var | ClawHub (binlerce) | hub | 🔴 |
| Çoklu-ajan iş kuyruğu | Workboard (kısmi UX) | routing | Kanban (olgun) | 🟡 |
| Batch processing (100-1000 paralel) | workflow ile dolaylı | — | ✔ | 🔴 |
| Credential pools (anahtar rotasyonu) | çoklu-anahtar keyring | — | ✔ | 🟡 |
| Voice/STT/TTS | ✔ 3 vendor + hands-free | ✔ | ✔ 10 | 🟡 |
| Görsel üretim | image provider | ✔ | FAL.ai 9 | 🟡 |
| MCP | ✔ + 43 preset katalog | ✔ | ✔ | 🟡 |
| OpenAI-uyumlu API + SDK'lar | ✔ (Py/TS/Rust) + ACP | — | ✔ | 🟢 |
| Çoklu-tenant izolasyon | ✔ | — | — | 🟢 |
| **Mobil cihaz node'u** | — | ✔ (kamera/konum/ekran) | — | 🔴 |
| **Masaüstü tepsi/companion** | web-only | ✔ (menü çubuğu/Hub) | dashboard | 🔴 |
| **IDE eklentisi** | ACP yüzeyi var | — | ✔ shipped | 🔴 |
| Bağlam `@` referansları + context-file import | kısmi | — | ✔ | 🔴 |
| **Proaktiflik + aksiyon alan Initiative** | ✔ (Pulse) | heartbeat | scheduled | 🟢 |
| **Derin araştırma harness'i** | — | web-arama | web-arama | 🔴 (ikisinde de zayıf → fırsat) |

---

## 5. Paritenin Ötesi: Gerçek Jarvis Farklılaştırıcıları

Bir "Jarvis"i rakip bir chat-agent'tan ayıran şey; genişlik değil, **güvenilir
proaktif otonomi + derin muhakeme + ortam varlığı**. AGEZT'in zaten sahip olduğu
zemin bu yönde eşsiz:

1. **Yönetişimli proaktiflik = moat.** Pulse gözlemliyor, Initiative *aksiyon*
   alabiliyor, her aksiyon Edict ile sınırlı ve journal'da izlenebilir. Ne
   OpenClaw'ın heartbeat cron'u ne Hermes'in zamanlanmış görevleri bu düzeyde
   *yönetişimli inisiyatife* sahip. Jarvis'in "sen istemeden doğru olanı yapması"
   ancak güvenle mümkün — o güveni veren denetim/geri-alma katmanı AGEZT'te var.

2. **Dünya modeli + bellek → öngörü.** worldmodel (varlık/ilişki/decay) tek başına
   rakiplerde yok. Bunu proaktifliğe bağlayınca "anticipatory" davranış (kullanıcının
   bir sonraki ihtiyacını önceden hazırlama) mümkün olur.

3. **Society-of-agents (Council/Conductor).** Kanban'ın ötesinde bir *muhakeme*
   katmanı: birden çok ajanın tartışıp uzlaştığı, bir kondüktörün orkestre ettiği
   yapı zaten UI'da var. Bunu üretimleştirmek Hermes Kanban'ı geçmenin yolu.

4. **Ortam varlığı (ambient).** Jarvis.tsx + hands-free voice + wake-word +
   çok-kanallı erişim. Cihaz companion'ı eklenince "her yerde hazır" asistan olur.

5. **Denetlenebilir geri-alınabilirlik.** "Eylemden önce yetki, eylemden sonra
   nedensellik/kanıt/geri-alma." Bu, kurumsal ve kişisel güvenin temel farkı.

---

## 6. Öncelikli Yol Haritası

### P0 — Paritenin en görünür boşlukları (0–60 gün)

1. **Cihaz/companion katmanı (en yüksek etki).**
   - Node registry zaten var → üzerine hafif **masaüstü tepsi companion'ı**
     (başlat/durdur, sağlık, onaylar, push-to-talk, bildirim, tünel durumu).
   - **PWA/mobil companion**: onaylar, inbox/alert, sesli mesaj, run durumu,
     paylaş-sayfası webhook hedefi. (OpenClaw mobil node'una fonksiyonel yanıt.)
   - Cihaz-yönlendirme politikası: hangi node tarayıcı/shell/HA/medya çalıştırabilir.

2. **Canlı tarayıcı sekmesi oturumu.** `browser.action` fiillerini kalıcı canlı
   Chromium sürecine + DOM-seviyesi stale-ref invalidation'a çıkar; login/iframe/
   indirme/SPA/cookie-taşıma için E2E fixture'lar. (OpenClaw "her zaman açık tarayıcı"
   deneyimine yanıt.)

3. **Skill LLM Curator (shadow modda).** Deterministik küratörün üzerine, kullanım
   metriklerine bakıp konsolidasyon/patch **öneren** yardımcı-model job'u; asla
   silmez, hep archive/revertable. (Hermes Curator paritesi.)

### P1 — Ergonomi + pazar (60–120 gün)

4. **Bağlam ergonomisi.** `@dosya/klasör/diff/URL` bağlam referansları +
   AGENTS.md/CLAUDE.md/SOUL.md/.cursorrules **injection-taramalı içe aktarma**.
   Migrasyon: OpenClaw workspace + Hermes MEMORY/USER/SOUL importer'ları (dry-run,
   yedeksiz üzerine yazmaz).

5. **Marketplace güven + dağıtım.** Skills/plugins/MCP/kanal/exec-profile/workflow
   için birleşik pazar UI'si + **paket başına güven kartı** (yayıncı kimliği, imza,
   BLAKE3, istenen izinler, dosyalar, install script, ağ domainleri, scanner bulguları,
   karantina, update policy). Opsiyonel ClawHub/agentskills import + ön-tarama.

6. **IDE eklentisi.** ACP yüzeyi mevcut → VSCode (asgari) eklentisi ship et.

7. **Batch + credential pools.** Adlandırılmış toplu-işlem yüzeyi (yüzlerce prompt,
   workboard üzerine); çoklu-anahtar keyring'e otomatik rotasyon/dağıtım.

### P2 — Jarvis'i geçiren farklılaştırıcılar (120 gün+)

8. **Derin araştırma harness'i (en büyük "ötesi" fırsatı).** Çok-kaynaklı fan-out
   arama → derin okuma → **çelişki/adversarial doğrulama** → alıntılı sentez. Pulse
   + worldmodel + workflow üzerine oturur; DeerFlow raporundaki desenler (middleware
   + deferred tool discovery + citation) rehber. Ne OpenClaw ne Hermes'te güçlü.

9. **Anticipatory otonomi.** worldmodel + memory + pulse → kullanıcının bir sonraki
   ihtiyacını önceden hazırlama (brifing, hazır taslak, uyarı) — yönetişim sınırında.

10. **Society-of-agents üretimleştirme.** Council/Conductor'ı canlı çok-ajanlı
    muhakeme + delegasyon grafiği + Workboard lane entegrasyonu ile olgunlaştır;
    grafik-bağımlılık görünümü, satır-içi artifact/diff önizleme, "insana sor" akışı.

11. **Cloud terminal artifact yaşam döngüsü.** K8s job lifecycle döngüsünü
    tamamla (exec-profile paritesini kapat).

---

## 7. En Kritik Üç Vurgu

1. **Kod hazır, ürünleşme ve cihaz erişimi eksik.** AGEZT'in geride kaldığı yerler
   neredeyse tamamen *çalışma-zamanı/dağıtım* (mobil, tepsi, dolu pazar, canlı sekme)
   — çekirdek yetenek değil. Bu, kapatması hızlı ama görünür boşluklar.

2. **AGEZT'in kazanma pozisyonu "daha çok tool" değil.** Yönetişim + nedensellik +
   geri-alma + kalıcı kimlik + proaktiflik. Rakipler bu eksende yapısal olarak zayıf;
   mesajlaşma ve demolar bu üstünlüğü öne çıkarmalı.

3. **Tek en büyük "ötesi" hamlesi: derin araştırma + anticipatory proaktiflik.**
   İkisi de rakiplerde zayıf, AGEZT'in zemini (pulse/worldmodel/workflow/journal) buna
   hazır. Gerçek "Jarvis" hissi buradan gelir.

---

## 8. Sonuç

AGEZT bugün **paritenin çok yakınında ve birçok eksende önde** olan olgun bir
agentic OS. "Her ikisinin yaptığını yapmak" için gereken iş, sıfırdan yetenek değil,
**beş görünür boşluğun ürünleştirilmesi**: cihaz/companion, canlı tarayıcı, LLM
curator, bağlam ergonomisi, dolu güvenli pazar. "İkisinin ötesinde bir Jarvis olmak"
içinse zemin zaten benzersiz: **yönetişimli proaktiflik + dünya modeli + denetlenebilir
geri-alma** üzerine **derin araştırma harness'i** ve **anticipatory otonomi** eklemek.
