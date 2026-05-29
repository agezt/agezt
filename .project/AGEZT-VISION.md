# Agezt — Vision & Architecture (v0.1 draft)

> **Tek cümle:** Out-of-process plugin kernel'i üstünde çalışan; deterministik DAG + LLM-loop orchestration, event-sourced denetlenebilir memory ve görsel (React Flow) programlanabilirlik sunan; içinde bağımsız otonom agent'ların yaşadığı, her dilde genişletilebilir bir **agent platform işletim sistemi**.

> **Felsefe:** İçi çok güçlü, dışı çok sade. Çekirdek tek statik Go binary, near-zero dependency, stdlib-first (#NOFORKANYMORE). Karmaşıklık progresif açılır: yeni kullanıcı tek komutla başlar, power-user her katmana iner.

---

## 1. Neden var? (Rakipleri yendiğimiz eksenler)

| Eksen | OpenClaw (TS, ~247k★) | Hermes Agent (Py, ~134k★) | **Agezt** |
|---|---|---|---|
| Otonomluk modeli | Stochastic LLM-loop | Stochastic LLM-loop + Curator | **Deterministik DAG + içinde bounded LLM-loop** |
| Runtime | Node/Bun, dependency ağır | Python venv, dependency ağır | **Tek statik Go binary, zero-dep** |
| Self-improvement | ClawHub skills (audit yok) | Curator (7g cron, markdown, audit zayıf) | **Event-sourced, versiyonlu, shadow-test, geri-alınabilir** |
| Plugin izolasyonu | In-process (crash → kernel düşer) | In-process | **Out-of-process gRPC (crash → supervisor kaldırır)** |
| Genişletilebilirlik | TS skill | Python plugin | **Polyglot SDK: Go/TS/Py/Rust** |
| Güvenlik | Default makinede çalışır (kriz yaşadı) | 5-layer ama opsiyonel | **Default-deny, izolasyon profilleri** |
| Görsel programlama | Yok | Yok | **React Flow visual orchestrator** |

**Asıl fark:** İkisi de "akıllı bir loop". Agezt "denetlenebilir bir runtime". Aynı zekâyı veriyoruz ama her şey görünür, tekrar-üretilebilir ve geri-alınabilir.

---

## 2. Çekirdek mimari — minimal kernel, her şey plugin

Kernel yalnızca **6 sorumluluk** taşır; gerisi plugin'dir:

```
┌──────────────────────────────────────────────────────────────┐
│                         AGEZT KERNEL                          │
│  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐  │
│  │ Lifecycle  │ │  Journal   │ │ Plugin Host│ │    DAG     │  │
│  │ /Supervisor│ │(event truth│ │ (gRPC/std) │ │ Scheduler  │  │
│  │            │ │ +BLAKE3)   │ │            │ │            │  │
│  └────────────┘ └────────────┘ └────────────┘ └────────────┘  │
│  ┌────────────┐ ┌────────────┐                                │
│  │ Policy Gate│ │  Conduit   │   Internal Event Bus           │
│  │  (Edict)   │ │  Registry  │   (in-proc channels → journal) │
│  └────────────┘ └────────────┘                                │
└──────────────────────────────────────────────────────────────┘
        │ gRPC / stdio (HashiCorp go-plugin deseni)
        ▼
┌──────────────────────────────────────────────────────────────┐
│  PLUGINS (her biri ayrı process, crash-isolated)              │
│  Channel · Provider · Tool · CodingAgent · Memory · Storage · │
│  Tunnel    (Telegram/Anthropic/shell/ClaudeCode/Flint/PG/...) │
└──────────────────────────────────────────────────────────────┘
        ▲ aynı socket protokolü
┌───────────────┬────────────────┬─────────────────────────────┐
│   CLI (agt)   │  Web UI (React) │  SDK (Go/TS/Py/Rust)        │
│  Bubble Tea   │  Flow Studio +  │  create-agezt-plugin       │
│      TUI      │  Unified Inbox  │  20 satırda plugin          │
└───────────────┴────────────────┴─────────────────────────────┘
```

### Kernel'in 6 sorumluluğu
1. **Lifecycle / Supervisor** — agent'ları spawn/suspend/resume/restart eder. Hafif aktör modeli: her agent = goroutine + mailbox. Crash'te journal'dan state replay ederek kaldığı yerden devam.
2. **Journal** — tek gerçek kaynağı (single source of truth). Append-only JSONL + BLAKE3 hash chain. State = event'lerin fold'u. Time-travel debug + tam reproducibility + her mutasyon geri-alınabilir.
3. **Plugin Host** — plugin'leri subprocess olarak başlatır, gRPC ile konuşur. `.proto` contract'ları → polyglot SDK auto-gen. Plugin çökerse kernel ayakta kalır.
4. **DAG Scheduler** — görevi DAG'a derler (tool-node / llm-node / gate-node / loop-node), topolojik sıra + paralel branch çalıştırır.
5. **Policy Gate (Edict)** — YAML policy-as-code. Hangi tool, kim, hangi izolasyon, hangi onay. Her karar journal'a.
6. **Conduit Registry** — provider/tool/memory plugin'lerinin kayıt defteri; runtime'da keşif ve yönlendirme.

---

## 3. Otonom agent modeli (hafif aktör)

- **Mailbox:** her agent kendi mesaj kuyruğunu işler; agent↔agent iletişim event bus üzerinden.
- **Agent tipleri:** `resident` (7/24, channel dinler) · `ephemeral` (görev biter ölür) · `scheduled` (cron tetikler) · `reactive` (event tetikler).
- **Supervision (hafif):** crash → policy'ye göre restart + journal replay. (Tam OTP supervision tree ileri faza; contract baştan hazır.)
- **DAG + LLM-loop hibrit:** iskelet deterministik DAG, "akıl" gereken düğümler `loop-node` içinde bounded LLM-loop. Öngörülebilirlik + zekâ bir arada.

---

## 4. Event Bus & Socket katmanı

- **Internal bus:** in-process Go channels, subject routing (`agent.*.task.done`), her event journal'a da yazılır. Tek-node, basit, hızlı. (NATS/Redis/ChimeraMQ pluggable — contract mesh-ready.)
- **Socket:** Unix domain socket (yerel CLI↔kernel) + gRPC (uzak) + WebSocket/SSE (UI & uzak SDK). `agt attach` ile herhangi bir çalışan agent'a canlı bağlan.
- **Tek truth:** UI canlı highlight, Live Monitor, agent koordinasyonu — hepsi tek event journal'dan beslenir. Ayrı sistem yok.

---

## 5. Persistence — pluggable driver, 3 katman

| Katman | Default (embedded, zero-dep) | Pluggable |
|---|---|---|
| **Journal** (event truth) | CobaltDB + JSONL | Postgres |
| **State / index** | SQLite | Postgres |
| **Memory / RAG** | embedded fact store + FTS | Flint Vector (semantic) + Redis (cache) |

**Memory katmanı** Hermes'in `MEMORY.md` + FTS5 + Honcho üçlüsünü yener: event-sourced fact store + semantic recall + dialectic user-model — hepsi versiyonlu ve geri-alınabilir.

---

## 6. Plugin tipleri (7) ve SDK

`Channel` · `Provider` · `Tool` · `CodingAgent` · `Memory` · `Storage` · `Tunnel`

- **CodingAgentPlugin** (rakiplerde first-class değil): Claude Code / Codex / Aider / Cursor köprüleri bir node tipi olarak çağrılır — senin coding-agent portföyüne birebir oturur.
- **TunnelPlugin:** Cloudflare Tunnel / Tailscale / WireRift / Karadul mesh ile UI & gateway dışarı açılır.
- **Custom tool/db:** `.proto` contract → 20 satır SDK → `agt plugin add ./x`. Kernel hiçbirini bilmez, sadece contract'ı bilir.
- **SDK'lar:** `agezt-sdk-go|ts|py|rust`, `create-agezt-plugin`. ClawHub/agentskills.io'nun type-safe & polyglot versiyonu.

---

## 7. Üç sade yüzey ("basit ama güçlü" sözleşmesi)

- **CLI:** `agt run "şunu yap"` — sıfır config ilk çalıştırma (embedded DB, local model auto-detect). Gerisi opsiyonel flag.
- **Web UI (React 19 + Tailwind 4 + shadcn + React Flow):**
  1. **Flow Studio** — DAG'ı sürükle-bırak çiz, node = plugin, çalışırken canlı highlight.
  2. **Unified Inbox** — tüm channel'lar tek sadeleştirilmiş akış.
  3. **Live Monitor** — agent/session, token/maliyet/latency/trace.
  4. **Memory Explorer** — ne hatırlıyor, skill kütüphanesi, versiyon + revert.
- **SDK:** contract'ı doldur, bitti.

---

## 8. Binary stratejisi (tek binary ↔ sınırsız plugin uzlaşması)

`agezt` ana binary = kernel + native plugin'ler (statik gömülü, tek dosya, zero-dep). 3rd-party plugin'ler `~/.agezt/plugins/` altında ayrı process; kernel keşfeder ve subprocess başlatır. **Çekirdek hâlâ tek statik binary, platform sınırsız genişler.**

---

## 9. Faz planı

| Faz | İçerik | Çıktı |
|---|---|---|
| **F0 Kernel** | lifecycle + journal + plugin host (gRPC) + DAG scheduler + `.proto` + go-SDK | Görevi DAG'a derle, çalıştır, journal'a yaz, replay et |
| **F1 Conduit+Arsenal** | ProviderPlugin (Anthropic+Ollama), ToolPlugin (fs/shell/http), CLI | Tek görev uçtan uca |
| **F2 Memory+Forge** | pluggable memory, event-sourced fact/RAG, auditable self-improvement | Curator-killer |
| **F3 Channels+Inbox** | ChannelPlugin'ler + Unified Inbox UI | Çok kanaldan yönetim |
| **F4 Flow Studio** | React Flow visual orchestrator + Live Monitor | Görsel programlama |
| **F5 Warden+Edict** | sandbox profilleri, policy engine, multi-agent paralellik | Governance |
| **F6 Tunnels+SDK** | TunnelPlugin, TS/Py/Rust SDK, CodingAgent köprüleri | Platform genişlik |
| **F7 Mesh+Migration** | gossip mesh (federe bus), `agt migrate openclaw\|hermes` | Ölçek + kullanıcı çekme |

---

## 10. Non-goals (ilk fazlar)

- Tam OTP supervision tree (hafif supervision yeter).
- Multi-node mesh (contract'lar hazır, implementasyon F7).
- Kendi LLM eğitimi/RL (Hermes'in aksine; biz runtime'ız, model değil).
- GUI ile model fine-tuning.

---

*Sonraki adım: bu onaylanınca → modüler SPEC suit (KERNEL-SPEC, EVENT-SPEC, PLUGIN-CONTRACTS `.proto`, her plugin tipi spec'i, UI-SPEC) → IMPLEMENTATION → TASKS → BRANDING → README → PROMPT.md.*
