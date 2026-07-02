# AGEZT Derin Araştırma Harness'i — Uygulama Planı

> Tarih: 2026-07-02
> İlgili: [`JARVIS-VISION-2026.md`](JARVIS-VISION-2026.md) §6 P2-8 (en büyük "ötesi" fırsatı)
> Konum: `kernel/research/` (yeni) + `plugins/tools/research/` (yeni) + workflow şablonu + view

## Neden bu, ve neden şimdi

Bir Jarvis'i chat-agent'tan ayıran iki eksen: **yönetişimli proaktiflik** ve **derin
muhakeme**. Birincisinde (Pulse+Initiative) AGEZT önde; ikincisinde bir boşluk var:
kodda hiçbir çok-kaynaklı, çelişki-doğrulamalı **araştırma motoru yok**. OpenClaw ve
Hermes de burada zayıf (yalnız web-arama). Bu yüzden bu, hem paritenin ötesine geçiren
hem de rakiplerin karşılık veremeyeceği bir hamle.

**Kilit içgörü: bu sıfırdan bir iş değil — mevcut olgun primitiflerin kompozisyonu.**

| Araştırma adımı | AGEZT'te hazır primitif | Dosya |
|---|---|---|
| Soru → alt-sorular / plan | `planner` (LLM→DAG) | `kernel/planner/planner.go` |
| URL keşfi | `websearch` (keyless DDG, SSRF-korumalı) | `plugins/tools/websearch/websearch.go` |
| Sayfa getir + metne çevir | `browser.read` / `browser.action` | `plugins/tools/browser/` |
| Çok-kaynak üçgenleme (konsensüs) | **`council`** (farklı modeller → uzlaşı) | `plugins/tools/council/` + `kernel/runtime` |
| Çelişki/adversarial doğrulama | **`conductor`** (Thinker/Worker/**Verifier**) | `plugins/tools/conductor/` + `kernel/runtime` |
| Orkestrasyon | `workflow` DAG motoru | `kernel/workflow/` |
| Kalıcılık (bulgu/varlık) | `memory` + `worldmodel` | `kernel/memory/`, `kernel/worldmodel/` |
| Denetim + alıntı zinciri | hash-zincirli journal + `why` | `kernel/journal/`, `cmd/agt/why.go` |
| Yönetişim (bütçe/politika/HITL) | governor + edict + approvals | `kernel/governor/`, `kernel/edict/` |

Rakip harness'lerin (ör. DeerFlow) yeniden inşa ettiği her şey AGEZT'te zaten var;
**bize gereken tek şey bunları bir araştırma sözleşmesinde birleştiren ince bir katman.**

## Mimari

`research` yeni bir kernel paketi + agent-facing tool + workflow şablonu olarak yaşar.
Kendi LLM çağrılarını yapmaz; mevcut tool'ları governed `RunTool` üzerinden çağırır.

```
research.Run(question, opts)
  │
  ├─ 1. PLAN     planner → alt-sorular + araştırma DAG'ı (loop/gate node)
  │
  ├─ 2. GATHER   her alt-soru için (fan-out, workflow paralel node):
  │                websearch(query) → aday URL'ler
  │                browser.read(url) → kaynak metni (+ untrusted işareti korunur)
  │                → Source{url, title, text, fetched_at, hash}
  │
  ├─ 3. EXTRACT  her kaynaktan iddia çıkar → Claim{text, source_id, confidence}
  │
  ├─ 4. VERIFY   her önemli iddia için conductor (Verifier rolü):
  │                "bu iddiayı çürüt; belirsizse reddet" → CONFIRMED | REFUTED
  │                çelişen kaynaklar council'a → uzlaşı + azınlık görüşü notu
  │
  ├─ 5. SYNTH    yalnız CONFIRMED iddialardan alıntılı sentez (her cümle → source_id)
  │
  └─ 6. PERSIST  bulgular → memory; varlık/ilişki → worldmodel; her adım → journal
                  → ReportArtifact{markdown, sources[], claims[], confidence}
```

### Yönetişim sınırı (moat — rakiplerde yok)
- Her `websearch`/`browser.read` çağrısı Edict politikasından geçer, journal'a yazılır.
- Kaynak metni `ObservationUntrusted` işaretini korur → prompt-injection guard devrede.
- Bütçe: `budget.total` benzeri tavan; `governor` circuit-breaker; adım/kaynak/derinlik capları.
- Alıntılanamayan cümle sentезe **giremez** (citation zorunluluğu, halüsinasyon freni).
- Sonuç artifact'ı + tam kaynak listesi + `why <event>` ile uçtan uca izlenebilir.

## Veri tipleri (`kernel/research/research.go`)

```go
type Source struct {
    ID        string    // ulid
    URL       string
    Title     string
    Text      string    // browser.read çıktısı (untrusted)
    Hash      string    // BLAKE3(text) — yeniden-getiride değişim tespiti
    FetchedAt time.Time
    Rank      int        // websearch sırası
}

type Claim struct {
    ID        string
    Text      string
    SourceIDs []string   // destekleyen kaynaklar
    Verdict   string     // "unverified" | "confirmed" | "refuted" | "contested"
    Note      string     // council azınlık görüşü / verifier gerekçesi
}

type Report struct {
    Question   string
    SubQuestions []string
    Sources    []Source
    Claims     []Claim
    Markdown   string     // alıntılı sentez
    Confidence float64
    Budget     BudgetUse  // token/adım/kaynak sayacı
}
```

## Fazlar

### Faz 1 — Çekirdek harness (MVP, ~1 hafta)
- `kernel/research/` paketi: `Run()` = plan → gather → extract → synth (VERIFY henüz basit).
- `plugins/tools/research/` agent-facing tool (`research` fiili); `main.go`'da kaydet
  (websearch/browser ile aynı `out[...]` deseni, satır ~7615 civarı).
- Edict `CapResearch` ekseni (websearch benzeri, low-risk read + fan-out capı).
- `agt research "<soru>" [--depth N] [--max-sources M] [--json]` CLI + `agt why` uyumu.
- Testler: mock provider + sabit fixture sayfaları (network yok), fan-out capı, citation zorunluluğu.

### Faz 2 — Adversarial doğrulama (asıl farklılaştırıcı, ~1 hafta)
- VERIFY adımını `conductor`'a bağla: her yüksek-etkili iddiayı Verifier rolüyle çürütmeye çalış.
- Çelişen kaynakları `council`'a ver → uzlaşı + azınlık notu (`Claim.Note`).
- `Verdict` alanını doldur; `confidence` = confirmed/total oranından türet.
- Sentez yalnız `confirmed`/`contested(uzlaşılı)` iddiaları kullanır.

### Faz 3 — Orkestrasyon + kalıcılık (~1 hafta)
- Workflow şablonu `research.v1` (`kernel/workflow/templates`): fan-out gather node'ları,
  gate node = "yeterli güvenilir kaynak var mı?", yoksa yeni alt-sorularla döngü (loop node).
- Bulguları `memory`'ye yaz (subject-dedupe, per-agent private-by-default), varlık/ilişkileri
  `worldmodel`'e; rapor → `artifact` store.
- Pulse entegrasyonu: standing order / schedule ile periyodik araştırma ("her sabah X konusunu tara").

### Faz 4 — Konsol yüzeyi (~3-4 gün)
- Yeni view `Research.tsx` (67→68): soru kutusu, canlı adım akışı (SSE), kaynak kartları
  (güven rozeti + verdict çipi), alıntılı rapor, `why` derin-link. Mevcut tasarım sistemi
  (PageHeader + glass + ModelChip) ve `events.tsx` SSE hub'ı yeniden kullanılır.
- `views/Research.test.tsx` (view başına ~1:1 test disiplinine uygun).

## Kabul kriterleri
- Bir soru → ≥3 bağımsız kaynaktan üçgenlenmiş, her cümlesi alıntılı bir rapor üretir.
- En az bir yanlış/çelişkili iddia VERIFY adımında `refuted`/`contested` olarak yakalanır
  (adversarial doğrulamanın çalıştığının kanıtı; fixture ile test edilir).
- Tüm araştırma bütçe tavanına uyar; aşımda temiz durur, çökmez.
- Her kaynak getirimi + her iddia verdict'i journal'da; `agt why <report_event>` tam zinciri gösterir.
- Kaynak metni untrusted işaretini korur; injection-guard tetiklenirse rapora not düşülür.

## Yapılmayacaklar (sınır)
- Planner'ı mid-execution recursive re-planner'a çevirme — mevcut "tek LLM çağrısı = statik DAG"
  sözleşmesini koru; döngü workflow loop node'u ile, meta-agent ile değil.
- Alıntısız cümle üretme (halüsinasyon freni sözleşmesi).
- Kaynakları güvenilir sayma — hepsi untrusted, verdict'ler kanıtla gelir.
- Ayrı bir LLM istemcisi ekleme — tüm model çağrıları mevcut governor/provider yolundan.

## Sıradaki (bu plandan sonra)
Aynı "mevcut primitifleri birleştir" yaklaşımı P0 boşluklarına da uygulanır:
- **Cihaz/companion**: node registry + tünel + approvals API üzerine tepsi/PWA (yeni kod az).
- **Canlı tarayıcı sekmesi**: `browser.action` sürücüsünü kalıcı süreç + stale-ref'e çıkar.
- **LLM curator**: mevcut deterministik küratör + `council` shadow-mod konsolidasyon.
