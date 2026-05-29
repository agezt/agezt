# Agezt — Master Vision & Architecture (v0.2 draft)

> **Tek cümle:** Out-of-process plugin kernel'i üstünde çalışan; iki kalbi olan (reaktif: sen tetiklersin / proaktif: kendi tetikler) bir **agentic işletim sistemi** — deterministik DAG + LLM-loop orchestration, event-sourced denetlenebilir memory, kendi kendini geliştirme, alt-agent doğurma, abonelik/limit yönetimi ve görsel (React Flow) programlanabilirlik ile gerçek bir Jarvis.

> **Felsefe:** İçi çok güçlü, dışı çok sade. Çekirdek tek statik Go binary, near-zero dependency, stdlib-first (#NOFORKANYMORE). Karmaşıklık progresif açılır: yeni kullanıcı tek komutla başlar, power-user her katmana iner.

> **Agezt:** Antik Roma'da magistrate'in önünde yürüyen, otorite ve düzeni taşıyan görevli. "Otonom ama otorite altında" — sistemin özü.

---

## 1. Neden var? (Rakipleri yendiğimiz eksenler)

| Eksen | OpenClaw (TS, ~247k★) | Hermes Agent (Py, ~134k★) | **Agezt** |
|---|---|---|---|
| Otonomluk modeli | Stochastic LLM-loop | Stochastic LLM-loop + Curator | **Deterministik DAG + bounded LLM-loop** |
| Tetikleme | Reaktif (sen/cron) | Reaktif (sen/cron) | **İki kalp: reaktif + proaktif (Pulse)** |
| Runtime | Node/Bun, dep ağır | Python venv, dep ağır | **Tek statik Go binary, zero-dep** |
| Self-improvement | ClawHub (audit yok) | Curator (markdown, audit zayıf) | **Event-sourced, versiyonlu, shadow-test, geri-alınabilir** |
| Plugin izolasyonu | In-process (crash→kernel düşer) | In-process | **Out-of-process gRPC (crash→supervisor kaldırır)** |
| Genişletilebilirlik | TS skill | Python plugin | **Polyglot SDK: Go/TS/Py/Rust** |
| Önem ayrımı | Yok | Yok | **Salience filter (gürültüyü keser)** |
| İnisiyatif | Yok (sadece hatırlatıcı) | Yok | **Initiative engine (kendi çözer/sorar)** |
| Güvenlik | Default makinede çalışır | 5-layer opsiyonel | **Default-deny + trust ladder + panik freni** |
| Görsel programlama | Yok | Yok | **React Flow visual orchestrator** |
| Açıklanabilirlik | Yok | Zayıf | **`agt why` — her kararın tam zinciri** |

**Asıl fark:** İkisi de "akıllı bir loop". Agezt "denetlenebilir, proaktif bir varlık". Aynı zekâ, ama her şey görünür, geri-alınabilir, ve sen sormasan da çalışır.

---

## 2. İki kalp

```
        REAKTİF KALP                          PROAKTİF KALP (Pulse)
   sen / cron / event tetikler            sistem kendi kendini tetikler
            │                                        │
            ▼                                        ▼
       ┌─────────┐                            ┌─────────────┐
       │ Planner │                            │   Pulse     │ ◄── ritim (nabız)
       │ niyet→  │                            │  Engine     │
       │  DAG    │                            └──────┬──────┘
       └────┬────┘                                   │
            │                          ┌─────────────┼─────────────┐
            │                          ▼             ▼             ▼
            │                    ┌──────────┐ ┌──────────┐ ┌──────────┐
            │                    │Observers │ │ Salience │ │Initiative│
            │                    │(izle)    │→│ (öne)    │→│(çöz/sor) │
            │                    └──────────┘ └──────────┘ └────┬─────┘
            │                                                   │
            └──────────────────┬────────────────────────────────┘
                               ▼
                    ORTAK ÇEKİRDEK (bölüm 3)
                               │
                               ▼
                    ┌───────────────────────┐
                    │  Briefing Composer     │ → Telegram/mail/UI/ses
                    │ (doğru kanal+ton+sıklık)│
                    └───────────────────────┘
```

İkisi de aynı çekirdeği paylaşır; fark sadece **tetikleyenin kim olduğu**.

---

## 3. Çekirdek mimari — minimal kernel, her şey plugin

Kernel yalnızca **6 sorumluluk** taşır; gerisi plugin'dir:

```
┌──────────────────────────────────────────────────────────────┐
│                         AGEZT KERNEL                          │
│  Lifecycle/Supervisor · Journal(event truth+BLAKE3) ·          │
│  Plugin Host(gRPC/stdio) · DAG Scheduler ·                     │
│  Policy Gate(Edict) · Conduit Registry                         │
│  ───────────── Internal Event Bus (in-proc → journal) ─────────│
└──────────────────────────────────────────────────────────────┘
        │ gRPC / stdio (HashiCorp go-plugin deseni)
        ▼
┌──────────────────────────────────────────────────────────────┐
│  PLUGINS (her biri ayrı process, crash-isolated)              │
│  Channel · Provider · Tool · CodingAgent · Memory · Storage · │
│  Tunnel    (Telegram/Anthropic/browser/ClaudeCode/Flint/PG/..)│
└──────────────────────────────────────────────────────────────┘
        ▲ aynı socket protokolü (Unix/gRPC/WS)
┌───────────────┬────────────────┬─────────────────────────────┐
│   CLI (agt)   │  Web UI (React) │  SDK (Go/TS/Py/Rust)        │
│  Bubble Tea   │ Flow Studio +   │  create-agezt-plugin       │
│      TUI      │ Inbox + Monitor │  20 satırda plugin          │
└───────────────┴────────────────┴─────────────────────────────┘
```

### Kernel'in 6 sorumluluğu
1. **Lifecycle / Supervisor** — agent spawn/suspend/resume/restart. Hafif aktör: her agent = goroutine + mailbox. Crash'te journal'dan state replay.
2. **Journal** — tek gerçek kaynağı. Append-only JSONL + BLAKE3 chain. State = event fold'u. Time-travel + reproducibility + her mutasyon geri-alınabilir.
3. **Plugin Host** — plugin'leri subprocess başlatır, gRPC ile konuşur. `.proto` → polyglot SDK auto-gen. Plugin çökerse kernel ayakta.
4. **DAG Scheduler** — görevi DAG'a derler (tool/llm/gate/loop-node), topolojik + paralel çalıştırır.
5. **Policy Gate (Edict)** — YAML policy-as-code + trust ladder. Her karar journal'a.
6. **Conduit Registry** — provider/tool/memory plugin kayıt defteri; runtime keşif & yönlendirme.

---

## 4. Niyet → eylem zinciri (reaktif kalp)

1. **Planner (meta-agent)** — niyeti alır, capability envanterine bakar, deterministik DAG kurar. Eksik capability varsa Forge'a ürettirir ya da plugin önerir. *Plan önce çıkar, görünür, onaylanabilir.*
2. **DAG Scheduler** çalıştırır; `loop-node` içinde LLM akıl yürütür, `tool-node` browser/dosya/ses/video çalıştırır.
3. Gerekirse **alt-agent spawn** (`ephemeral`), iş biter ölür, sonuç event bus'a.
4. **Governor** hangi LLM'i hangi limitte/abonelikte kullanacağını yönetir.
5. Her adım **journal**'a; sonuç **channel**'lardan döner.

---

## 5. Pulse Engine — proaktif çekirdek (Jarvis'in kalbi)

Sürekli atan nabız. Her pulse'ta sistem sorar: *"Ne değişti? Önemli mi? Bir şey yapmam/Ersin'in bilmesi gerekir mi?"*

- **Observers** — arka planda koşan hafif reactive agent'lar. İzler: portföy repoları (CI/issue/advisory), sistem sağlığı (disk/servis), channel'lar (gelen mail/mesaj), dış dünya (RSS/X/fiyat/haber), iç durum (bütçe/takılan agent). Ham sinyali değil **anlamlı delta**yı yazar.
- **Salience filter** — her delta LLM-node'dan geçer: kayda değer mi, eşik üstü mü, Ersin biliyor mu, ne kadar acil? Skorlar. Düşük→sadece journal; yüksek→aksiyon. Eşik ayarlanır (sadece kritik / günlük özet / her şey).
- **Initiative engine** — önemliyse sana sormadan karar verir: kendi çözer (CI kırık→düzelt, PR aç, haber ver) mi, yoksa önce sorar (prod DB silinecek→dur, onay) mı? Edict + Governor + trust ladder belirler.
- **Briefing composer** — doğru kanal/ton/sıklık. Acil→anında Telegram; önemli→sabah brifingi; bilgi→haftalık özet.

---

## 6. Güven & kontrol katmanları (otonom sistemde olmazsa olmaz)

- **Trust ladder** — her aksiyon tipinin güven seviyesi: "PR aç ama merge etme", "haber ver ama benim adıma mesaj atma", "$10'a kadar harca, üstü onay". Sen yükselttikçe özerklik artar.
- **Dead-man's switch / panik freni** — `agt halt` her şeyi anında dondurur (state kaybolmaz). Anomalide (saatte 1000 tool call) otomatik freeze.
- **Explainability** — `agt why <event>` kararın tam zincirini gösterir: hangi observer, neden yüksek salience, hangi LLM, hangi policy. Event-sourcing bunu bedava verir.
- **Simulation / dry-run** — riskli DAG'ı çalıştırmadan "ne olurdu"yu simüle eder, onay ister.

---

## 7. Zekâ katmanları

- **World Model / context graph** — projeler, repolar, kişiler, tercihler, alışkanlıklar, ilişkiler grafiği. "Portföy"/"Trabzonspor" ne demek bilir. Flint Vector + graph. Salience'ın beyni: önemi *senin dünyana* göre tartar. (Hermes'in Honcho'sundan ileri.)
- **Reflection loop (meta-biliş)** — günlük/haftalık öz-değerlendirme: hangi görevde başarısız, hangi tahmin tuttu, Ersin neyi reddetti/beğendi. Davranış kalibrasyonu ("brifingleri siliyor→sıklığı azalt"). Forge'un üst seviyesi.
- **Forge (self-improvement)** — görev sırasında skill üretir, shadow-test eder, promote/quarantine/revert state machine. Versiyonlu, content-addressed, geri-alınabilir. Curator-killer.
- **Standing Orders / Goals** — kalıcı üst-düzey hedefler: "her sabah repoları tara, kırık CI'yı düzelt, Telegram'dan özet at." Chronos + reactive + Planner birlikte canlı tutar.

---

## 8. Governor — abonelik & limit yönetimi (rakiplerde yok)

Her provider: subscription mi / API-key mi / rate-limit ne? Governor:
- Limitli kaynağı korur (token/istek bütçesi).
- Aboneliği önceler ("Claude Max var → önce onu").
- Limit dolarsa fallback chain ("Anthropic bitti → OpenRouter → local Ollama").
- Maliyeti journal'a yazar, bütçe aşımında Pulse'a sinyal.

"Kaynak limitliyse uy, yoksa yarattır" tam burada. Edict'in alt-modülü.

---

## 9. Persistence — pluggable driver, 3 katman

| Katman | Default (embedded, zero-dep) | Pluggable |
|---|---|---|
| **Journal** (event truth) | CobaltDB + JSONL | Postgres |
| **State / index** | SQLite | Postgres |
| **Memory / RAG / world model** | embedded fact store + FTS | Flint Vector (semantic) + Redis (cache) |

---

## 10. Plugin tipleri (7) ve SDK

`Channel` · `Provider` · `Tool` · `CodingAgent` · `Memory` · `Storage` · `Tunnel`

- **CodingAgentPlugin** (rakiplerde first-class değil): Claude Code/Codex/Aider/Cursor köprüleri bir node tipi olarak çağrılır.
- **TunnelPlugin:** Cloudflare Tunnel / Tailscale / WireRift / Karadul mesh.
- **Tool örnekleri:** browser (Playwright/CDP), dosya/rapor (docx/pdf/xlsx), kod, görsel, ses (STT/TTS), video-gen. Ağırlar out-of-process → kernel şişmez.
- **Custom tool/db:** `.proto` → 20 satır SDK → `agt plugin add ./x`. Kernel sadece contract'ı bilir.
- **SDK'lar:** `agezt-sdk-go|ts|py|rust`, `create-agezt-plugin`.

---

## 11. Ambient surfaces (Jarvis'in "bedenleri")

Aynı çekirdek, farklı yüzeyler: CLI/TUI · Web UI · sesli (wake-word→STT→agent→TTS) · menü-bar/tray app · mobil push · e-postaya doğal cevap. **Handoff:** konuşmayı Telegram'da başlat, CLI'da devam et — session lineage journal'da.

---

## 12. Web UI — React 19 + Tailwind 4 + shadcn + React Flow

1. **Flow Studio** — DAG sürükle-bırak, node=plugin, çalışırken canlı highlight (journal'dan).
2. **Unified Inbox** — tüm channel'lar tek sadeleştirilmiş akış.
3. **Live Monitor** — agent/session, token/maliyet/latency/trace.
4. **Memory Explorer** — ne hatırlıyor, skill kütüphanesi, versiyon + revert.

---

## 13. CLI — `agt`

`agt run "..."` · `agt flow edit` · `agt plugin add/list` · `agt channel status` · `agt journal replay` · `agt why <event>` · `agt halt` · `agt memory search` · `agt tunnel up cloudflare` · `agt agent spawn` · `agt pulse status` · `agt migrate openclaw|hermes`. Sıfır config ilk çalıştırma (embedded DB + local model auto-detect).

---

## 14. Ekosistem (ileri)

- **Marketplace** — plugin/skill/standing-order/workflow paylaşımı. Type-safe, versiyonlu, imzalı. ClawHub-killer.
- **Multi-tenant** — bir instance çok kişiye, her birinin world-model'i izole (API Cerberus deseni).
- **Federated mesh** — laptop/VPS/GPU box tek varlık; agent ve yük node'lar arası gezer (Karadul/SWIM). Contract'lar baştan mesh-ready.

---

## 15. Binary stratejisi

`agezt` ana binary = kernel + native plugin'ler (statik gömülü, tek dosya, zero-dep). 3rd-party plugin'ler `~/.agezt/plugins/` altında ayrı process. **Çekirdek tek statik binary, platform sınırsız genişler.**

---

## 16. Faz planı

| Faz | İçerik |
|---|---|
| **F0 Kernel** | lifecycle + journal + plugin host (gRPC) + DAG scheduler + `.proto` + go-SDK |
| **F1 Conduit+Arsenal** | Provider (Anthropic+Ollama), Tool (fs/shell/http/browser), Governor v1, CLI, tek görev uçtan uca |
| **F2 Memory+Forge** | pluggable memory + world model, event-sourced fact/RAG, auditable self-improvement |
| **F3 Pulse** | Observers + Salience + Initiative + Briefing, trust ladder, `agt halt`/`agt why` |
| **F4 Channels+Inbox** | ChannelPlugin'ler (Telegram/mail/WhatsApp) + Unified Inbox UI |
| **F5 Flow Studio** | React Flow orchestrator + Live Monitor |
| **F6 Warden+Edict** | sandbox profilleri, policy engine, multi-agent paralellik, simulation |
| **F7 Tunnels+SDK** | TunnelPlugin, TS/Py/Rust SDK, CodingAgent köprüleri, ambient surfaces |
| **F8 Reflection+Marketplace** | reflection loop, standing orders, marketplace |
| **F9 Mesh+Migration** | gossip mesh, multi-tenant, `agt migrate` |

---

## 17. MVP KESİTİ — acımasız %80 (gerçekçi başlangıç)

Yukarıdaki her şeyi birden inşa etmek sistemi kendi ağırlığı altında ezer. **Bu 7 şey ayaktaysa Jarvis'in ruhu çalışır, gerisi katman katman eklenir:**

1. **Kernel + Journal + Plugin Host (gRPC)** — her şeyin temeli. Event-sourcing en başta doğru kurulmalı, sonradan eklenemez.
2. **DAG Scheduler + Planner (basit)** — niyet→plan→çalıştır. LLM-loop tek node tipiyle başlasın yeter.
3. **2 Provider + Governor v1** — Anthropic (aboneliğin) + bir local/OpenRouter fallback. Limit/abonelik mantığı baştan, sonradan acı verir.
4. **4 Tool** — shell, file, http, browser. Bu dördüyle "dünyanın çoğu işi" zaten yapılır.
5. **1 Channel: Telegram (çift yönlü)** — hem komut al hem proaktif haber ver. Jarvis hissinin kanalı.
6. **Pulse v1 + Salience + 2 Observer** — repo/CI izle + sistem sağlığı izle. Salience eşiği "sadece kritik". Bu, "sen sormasan da haber veren" özün minimumu.
7. **`agt halt` + `agt why`** — gün bir, kontrol ve güven. Otonom bir şeyi panik freni olmadan açma.

**MVP dışı (bilinçli ertelenen):** Flow Studio UI, çoklu channel, mesh, marketplace, multi-tenant, ambient sesli, reflection loop, simulation, WASM in-process tool, polyglot SDK (önce go-SDK yeter). Hepsi MVP'yi bozmadan üstüne eklenecek şekilde contract'larda yer ayrıldı.

**MVP başarı testi:** "Telegram'dan 'portföyü kontrol et' yazınca repoları tarayıp özet dönsün; ben hiçbir şey yazmasam da kırık CI'yı kendisi fark edip Telegram'dan haber versin; ne yaptığını `agt why` ile görebileyim; çıldırırsa `agt halt` ile durdurabileyim." Bu cümle çalışıyorsa MVP bitti.

---

*Sonraki adım: bu master onaylanınca → modüler SPEC suit. Sıra önerisi: PLUGIN-CONTRACTS (`.proto` + event şeması) → KERNEL-SPEC → PULSE-SPEC → her plugin-tipi spec → UI-SPEC → IMPLEMENTATION → TASKS → BRANDING → README → PROMPT.md.*
