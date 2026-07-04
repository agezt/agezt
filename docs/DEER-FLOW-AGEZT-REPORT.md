# DeerFlow -> AGEZT Alınabilecekler Raporu

> İnceleme tarihi: 2026-06-26  
> DeerFlow kaynak: https://github.com/bytedance/deer-flow  
> İncelenen commit: `7a6c4a994a86583d2a3c056ee9d0f157d4f030c2` (`fix(channels): serialize per-chat thread creation to avoid duplicate threads (#3799)`)  
> AGEZT kaynakları: `README.md`, `docs/ARCHITECTURE.md`, `docs/NEXT.md`, `kernel/runtime`, `kernel/agent`, `kernel/skill`, `kernel/workflow`, `frontend/src/views/FlowStudio.tsx`

## Kısa sonuç

DeerFlow 2.0, AGEZT'nin yerine geçecek bir mimari değil. DeerFlow Python/LangGraph tabanlı bir "super agent harness"; AGEZT ise Go tabanlı, event-journal/policy/durable-agent merkezli bir agentic operating system. Bu yüzden alınacak şey LangGraph veya Python runtime değil; DeerFlow'un ürünleştirdiği bazı desenler:

- agent davranışını küçük, gözlemlenebilir middleware parçalarına ayırma,
- skill aktivasyonunu açık, denetlenebilir ve UI'den ayrılmış hale getirme,
- sub-agent/backend/frontend sözleşmelerini ortak fixture ile pinleme,
- MCP ve geniş tool kataloglarında şema yükünü deferred discovery ile azaltma,
- context compaction sırasında skill talimatlarını kaybetmeme,
- setup/doctor/onboarding'i coding-agent dostu hale getirme,
- docs ve demo yüzeyini "gerçek çalışan harness" olarak anlatma.

AGEZT zaten DeerFlow'dan daha güçlü olduğu alanlara sahip: durable roster identity, typed schedules, wake evidence, Edict policy, BLAKE3 hash-chain journal, plugin governance, multi-channel surface, SDK parity ve single static Go daemon. Raporun önerisi: DeerFlow'u çekirdek olarak kopyalamak yerine, bu güçlü zeminin üzerine DeerFlow'un ergonomi ve contract disiplinini seçerek almak.

## DeerFlow ne yapıyor?

DeerFlow 2.0 kendini "Deep Exploration and Efficient Research Flow" ve "super agent harness" olarak konumlandırıyor. Repo yapısı:

- Backend: Python 3.12+, FastAPI, LangGraph/LangChain agent runtime, `backend/packages/harness/deerflow/*`.
- Frontend: Next.js 16 / React 19 / TypeScript, Nextra docs, thread/artifact UI.
- Runtime kavramları: lead agent, middleware chain, sandbox, skills, subagents, memory, run manager, event store, checkpointer, MCP/tool search.
- Operasyon: `make setup`, `make doctor`, Docker/provisioner sandbox, config hot reload boundary, coding-agent oriented `Install.md`.

DeerFlow'un tasarım belgelerinde ana fikir "framework değil harness": lead agent + tool routing + middleware + sandbox + skills + memory + subagents + config bir arada, uzun görevler için hazır bir çalışma ortamı.

## AGEZT ile fark

| Alan | DeerFlow | AGEZT için çıkarım |
|---|---|---|
| Runtime dili | Python + LangGraph/LangChain | AGEZT'nin Go/std-lib-first çizgisini koru. LangGraph almak ters yönde bağımlılık getirir. |
| Agent modeli | Lead agent ve named custom agents, thread/checkpoint merkezli | AGEZT'nin durable roster identity modeli daha güçlü; DeerFlow'dan thread ergonomisi ve named-agent UX fikirleri alınabilir. |
| Tool governance | Middleware guardrails, sandbox audit, deferred tools | AGEZT'de Edict/journal daha güçlü; DeerFlow'un schema deferral ve contract fixture desenleri eklenmeli. |
| Skills | `SKILL.md`, slash activation, tool allowlists, skill rescue | AGEZT Forge/skill sistemi daha auditable; explicit activation ve compaction rescue alınmalı. |
| Subagents | `task` tool, status contract, token usage aggregation | AGEZT `delegate`/`delegate_await` daha gelişmiş; frontend/backend result contract fixture eksikliği kapatılmalı. |
| Context | Summarization middleware, skill rescue, dynamic context reminders | AGEZT context budget/elision var; skill/resource preservation katmanı güçlendirilmeli. |
| Docs/onboarding | Website docs, demos, `Install.md`, setup wizard | AGEZT'nin docs seti güçlü ama adoption akışı DeerFlow kadar "deneyip gör" odaklı değil. |

## Alınması önerilenler

### 1. Backend/frontend ortak contract fixtures

DeerFlow'da `contracts/subagent_status_contract.json`, backend `status_contract.py`, frontend `subtask-result.ts` ve iki tarafın testleri aynı fixture'ı kullanıyor. Amaç, backend'in "task sonucu başarılı/başarısız" formatını değiştirmesiyle frontend kart lifecycle'ının sessiz kırılmasını engellemek.

AGEZT karşılığı:

- `subagent.spawned`, `subagent.completed`, `delegate_await`, `policy.decision`, `agent.wake.autonomy_runbook`, `approval.*`, `run status` gibi UI'nin fold ettiği event/payload kontratları için `contracts/*.json` fixture seti ekle.
- Go testleri ve frontend Vitest testleri aynı fixture'ı okusun.
- Özellikle `last_autonomy_runbook`, mailbox wake badge, delegated/doctor lineage ve policy denial passport gibi yüzeyler bu kontratlara bağlansın.

Öncelik: yüksek. Etki büyük, risk düşük.

### 2. Explicit skill activation

DeerFlow'da kullanıcı `/skill-name task` yazdığında `SkillActivationMiddleware` ilgili `SKILL.md` içeriğini güvenli kökten okuyor, UI'den gizlenen hidden context olarak modele veriyor ve activation event'i auditliyor. AGEZT şu anda skill retrieval'i otomatik anahtar kelime/recency skoru ile yapıyor; bu iyi ama operatöre deterministik "bu run'da şu skill'i kullan" yolu eksik kalıyor.

AGEZT karşılığı:

- `agt run --skill <name> "<intent>"` ve Web UI chat içinde `/skill-name ...` desteği.
- Sadece active ve agent-scope'a uygun skill'ler aktive edilsin.
- `skill.activation` veya mevcut skill event ailesine ek payload ile correlation'a yazılsın.
- Aktivasyon context'i UI konuşma geçmişine normal user text olarak düşmesin; ama `agt why`/journal üzerinden görülebilsin.
- Skill resources için mevcut `skill op=files/read` yönlendirmesi korunmalı.

Öncelik: yüksek. AGEZT'nin Forge/skill iddiasını daha kullanılabilir yapar.

### 3. Context compaction sırasında skill/resource rescue

DeerFlow summarization middleware'i skill file read sonuçlarını korumaya çalışıyor; eski mesajları özetlerken son yüklenen skill talimatlarının kaybolmasını engelliyor. AGEZT'de `ContextBudget`, `ContextProtectFirst/Last` ve `SummarizeElided` var; bu iyi bir temel ama skill/resource okuma sonuçları özel korunuyor mu net değil.

AGEZT karşılığı:

- `compactMessages` benzeri context budget yolunda `skill` tool sonuçlarını ve injected skill block'larını özel sınıf olarak işaretle.
- Son N skill/resource read veya toplam token/char bütçesi kadar "protected context" olarak sakla.
- Skill rescue olduğunda `context.compacted` payload'ına `skill_rescued_count`, `skill_rescued_chars` gibi alanlar ekle.

Öncelik: yüksek. Uzun görevlerde skill davranışının bozulmasını önler.

### 4. Deferred MCP/tool schema discovery

DeerFlow'un `tool_search` deseni, MCP tool'larının tamamını modele baştan bind etmiyor. Tool isimlerini kısa listeliyor, model ihtiyaç duyunca `tool_search` ile şemayı promote ediyor. Promotion state catalog hash ile scope ediliyor ve policy filtering'den sonra kuruluyor.

AGEZT karşılığı:

- AGEZT'de MCP attach sonrası tool'lar gelecek run'da callable oluyor; geniş MCP kataloglarında tool schema kalabalığı büyüyebilir.
- Mevcut `ToolSelector`/semantic discovery ile uyumlu olacak şekilde `tool_discover` veya `mcp_tool_search` modu eklenebilir.
- Kritik kural: discovery catalog'u Edict/agent allowlist filtering'den sonra oluşturulmalı; model izinli olmayan tool'un şemasını arama ile görememeli.
- Promotion state run/correlation scoped olmalı, stale catalog hash tool açmamalı.

Öncelik: orta-yüksek. MCP marketplace büyüdükçe gerekli hale gelir.

### 5. Middleware/pipeline sırası için invariant dokümantasyonu

DeerFlow lead agent dosyasında middleware sırası ve tracing callback yerleşimi explicit invariant olarak yazılmış. AGEZT'de agent loop bilinçli olarak first-party ve monolitik; bu avantaj. Fakat policy, tool timeout, context compaction, steering, tool memo, artifact offload, observation taint, run lifecycle gibi sıralama bağımlılıkları arttıkça invariant'ların test ve dokümanla sabitlenmesi daha değerli olur.

AGEZT karşılığı:

- Büyük refactor önermiyorum. Önce `kernel/agent` için "loop phase order" dokümanı ve küçük golden tests eklenmeli.
- Sonra yalnızca düşük riskli alanlarda internal hook/pipeline tipleri çıkarılabilir: `BeforeModel`, `BeforeToolGate`, `AfterToolResult`, `BeforeCompact`.
- Edict gating ve journal order kesinlikle yerinden oynatılmamalı: önce deterministic policy decision, sonra tool invoke, sonra result.

Öncelik: orta. Doğru yapılırsa bakım kolaylaşır; acele refactor riskli.

### 6. Config reload boundary registry

DeerFlow `reload_boundary.py` ile hangi config alanlarının hot reload, hangilerinin restart gerektirdiğini tek kaynakta tutuyor. AGEZT'de provider reload, catalog/vault reload, runtime env ve daemon startup ayrımları var; ama operatörün "bu değişiklik restart ister mi?" sorusuna tek registry daha iyi cevap verir.

AGEZT karşılığı:

- `kernel/configcenter` veya docs altında "startup-only/runtime-reloadable" registry.
- `agt doctor` ve Web UI Config Center bu registry'den uyarı üretsin.
- Provider/catalog/vault reload mevcut davranışları bu registry'ye bağlansın.

Öncelik: orta. Day-2 ops kalitesini artırır.

### 7. Sandbox güvenlik mesajları ve effective isolation görünürlüğü

DeerFlow local sandbox için host bash'in güvenli boundary olmadığını açıkça söylüyor ve bash subagent'i default kapatıyor. AGEZT'de warden, netguard, Edict ve tool effects var; daha güçlü ama kullanıcıya "bu platformda fiili izolasyon nedir?" daha görünür olmalı.

AGEZT karşılığı:

- `agt doctor` ve Web UI Tools/Sandbox ekranında requested vs effective isolation.
- Shell/code_exec/file tool için "host-level", "warden-limited", "containerized", "remote" gibi net rozetler.
- Windows/macOS/Linux farklarını saklamadan göster.

Öncelik: orta.

### 8. Coding-agent oriented onboarding

DeerFlow `Install.md` dosyasını özellikle coding agent'ların repoyu kurması için yazmış: idempotent, Docker-first, secret okumama, exact next command. AGEZT'de `agt quickstart` ve docs var; ama dış coding agent'a verilecek tek dosyalık setup prompt'u ayrıca değerli.

AGEZT karşılığı:

- `Install.md` (mevcut) veya ileride ayrı bir `CODING-AGENT-INSTALL.md` türevi.
- "Secret-bearing `.env` okunmaz", "önce `make build`/`agt quickstart`", "daemon başlatmadan önce exact next command döndür" gibi kurallar.
- `agt doctor` çıktısını daha actionable hale getirecek küçük checklist.

Öncelik: orta.

### 9. Public demo gallery ve artifact-first docs

DeerFlow frontend docs içinde gerçek demo thread/artifact örnekleri taşıyor. AGEZT'nin runnable autonomous demos'u var ama daha çok repo içi test kanıtı gibi duruyor. Dış kullanıcı için "bu ne üretti?" daha hızlı görünmeli.

AGEZT karşılığı:

- `examples/autonomous/` (mevcut runnable demos) veya ileride Web UI içinde "Demos" sayfası.
- Policy denial, mailbox delegation, typed schedule, plugin governance demolarının beklenen event timeline'ı ve artifact çıktısı.
- Her demo için "hangi AGEZT iddiasını kanıtlıyor?" bölümü.

Öncelik: orta.

### 10. Maintainer-orchestrator güvenlik paterni

DeerFlow'un maintainer orchestrator notları iyi bir güvenlik dersi veriyor: agent'ı önce comment-only gibi geri alınabilir yüzeye kapat, public çıktı için confidence+severity gate kullan, mevcut diff'i doğrulamadan konuşma.

AGEZT karşılığı:

- "repo guardian" / "PR reviewer" gibi AGEZT system agent'ları için reversible-surface policy.
- İlk aşamada yorum/draft üretir; branch/merge/release yetkisi yok.
- Bu pattern AGEZT'nin Edict/trust ceiling modeliyle çok uyumlu.

Öncelik: orta-düşük, ama iyi bir showcase olur.

## Alınmaması gerekenler

- LangGraph/LangChain runtime'ı AGEZT çekirdeğine taşımayın. AGEZT'nin first-party Go loop'u, policy/journal düzeni ve single-binary hedefiyle ters düşer.
- Next.js/Nextra frontend'e geçmeyin. AGEZT'nin embedded Vite SPA yaklaşımı daemon dağıtımıyla daha uyumlu.
- DeerFlow'un büyük `config.example.yaml` karmaşıklığını olduğu gibi kopyalamayın. AGEZT'de config yüzeyi büyürse config center/doctor üzerinden kademeli açılmalı.
- DeerFlow'un thread merkezli mental modelini AGEZT agent identity modelinin önüne koymayın. AGEZT'de thread/chat, durable agent kimliğinin altında kalmalı.
- Host-local sandbox'ı güvenliymiş gibi pazarlamayın. DeerFlow bu konuda dürüst; AGEZT de platforma göre effective isolation'ı açık göstermeli.

## Önerilen uygulama sırası

1. Contract fixtures: `contracts/autonomy_runbook.json`, `contracts/subagent_result.json`, `contracts/policy_decision.json`; Go + frontend tests.
2. Explicit skill activation: CLI `--skill`, chat `/skill`, journal event, agent-scope validation.
3. Skill rescue in context compaction: protected skill/resource messages, `context.compacted` payload genişletmesi.
4. Deferred MCP discovery design doc + minimal implementation behind feature flag.
5. Config reload boundary registry + doctor warning.
6. Demo gallery and coding-agent install doc.
7. Optional internal loop phase invariant docs/tests before any middleware refactor.

## Hızlı kazanımlar

- AGEZT docs index'inden bu raporu görünür yap.
- README'deki bazı doc referanslarını kontrol et: önceki incelemede eksik bir “system review” referansı tespit edilmişti; bu checkout'ta doğru hedef artık `docs/SYSTEM-AUDIT-REPORT.md`. Benzer şekilde `docs/NEXT.md` içindeki `docs/MISSING-PARTS-REPORT.md` ve `docs/MISSING-PARTS-PLAN.md` referansları artık doğrulanmış durumda.
- DeerFlow tarzı `Install.md` eklemek düşük maliyetli ve dış agent'ların AGEZT'yi denemesini kolaylaştırır.

## Kaynak dosyalar

DeerFlow:

- https://github.com/bytedance/deer-flow/tree/7a6c4a994a86583d2a3c056ee9d0f157d4f030c2
- `README.md`
- `Install.md`
- `backend/packages/harness/deerflow/agents/lead_agent/agent.py`
- `backend/packages/harness/deerflow/agents/middlewares/tool_error_handling_middleware.py`
- `backend/packages/harness/deerflow/agents/middlewares/skill_activation_middleware.py`
- `backend/packages/harness/deerflow/agents/middlewares/summarization_middleware.py`
- `backend/packages/harness/deerflow/agents/middlewares/deferred_tool_filter_middleware.py`
- `backend/packages/harness/deerflow/tools/builtins/tool_search.py`
- `backend/packages/harness/deerflow/sandbox/security.py`
- `backend/packages/harness/deerflow/config/reload_boundary.py`
- `contracts/subagent_status_contract.json`
- `frontend/src/content/en/introduction/core-concepts.mdx`
- `frontend/src/content/en/harness/design-principles.mdx`

AGEZT:

- `README.md`
- `docs/ARCHITECTURE.md`
- `docs/NEXT.md`
- `docs/COMPARISON.md`
- `kernel/agent/agent.go`
- `kernel/runtime/runtime.go`
- `kernel/runtime/subagent.go`
- `kernel/skill/skill.go`
- `kernel/skill/retrieve.go`
- `kernel/workflow/workflow.go`
- `plugins/tools/mcptool/tool.go`
- `frontend/src/views/FlowStudio.tsx`
