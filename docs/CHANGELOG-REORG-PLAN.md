# CHANGELOG Reorg Planı (P0-5)

> **Tarih:** 2026-07-04
> **Dal:** `refactor/c4-agentdetail-phase0` (HEAD: `967de333`)
> **Statü:** ACTIVE — `docs/MISSING-PARTS-PLAN.md` P0-5 görevi için üretildi.
> **Diğer referanslar:** `docs/MISSING-PARTS-PLAN.md`, `docs/MISSING-PARTS-REPORT.md` (H-04).

---

## 1. Mevcut Durum (verified 2026-07-04)

| Metrik | Değer |
|---|---|
| `CHANGELOG.md` boyut | **646,076 bytes (≈631 KB)** |
| Toplam satır | **7,914** |
| Üst-düzey bölümler (`## `) | 3 |
| Alt-bölümler (`### `) | 57 |
| Yayınlanmış sürümler | `[1.0.0] 2026-06-03`, `[0.1.0] 2026-05-30` |
| Unreleased bölümü | Lines 12–5795 (5,784 satır / **~635 KB**) |
| `[1.0.0]` bölümü | Lines 5796–7835 (2,040 satır / ≈6.7 KB) |
| `[0.1.0]` bölümü | Lines 7836–7914 (79 satır / ≈0.3 KB) |

**Sorun:** `[Unreleased]` bloğu **5.7K+ satır** ve neredeyse tüm M-faz raporlarının özetini içeriyor. PR incelemelerinde diff'i anlamak zor; yeni M-faz commit'leri bu bloğa satır eklenmesine neden oluyor.

### 1.1 Unreleased İç Yapısı

57 alt-bölüm var, Keep a Changelog kategorilerinde dağılmış:
- `### Added`: ~24 blok
- `### Fixed`: ~15 blok
- `### Changed`: ~7 blok
- `### Security`: ~2 blok
- `### Code quality`: ~1 blok
- `### Tests`: ~1 blok

Her blok, M-faz çalışmalarının toplu açıklaması. Örnek: `### Added — positioning, security, and SDK parity documentation` (line 14).

---

## 2. Hedef

**Hedefler:**
1. Ana `CHANGELOG.md` ~50 KB altına düşürülsün (sadece TOC + son 1.0.0 + yeni Unreleased).
2. Unreleased içeriği **milestone aralıklarına** göre dilimlensin.
3. CI / lint bu yeni yapıyı doğrulayabilsin.
4. PR diff'leri küçük ve okunabilir kalır (her PR'da yalnız ilgili milestone dosyasına eklenir).

**Olmayan hedefler (non-goals):**
- CHANGELOG içeriğini **silmek yok** — bölmek/splitting yapacağız, kaldırmak değil.
- Keep a Changelog formatını bozmak yok — bölüm başlıkları ve kategoriler aynı kalır.
- M faz-rapor dosyaları (`PHASE-M*.md`) dokunulmaz; onlar zaten ayrı.
- Geçmişe retroactive `git tag` eklemek yok (örn. `v1.0.1` veya benzeri) — stabilite gereği.

---

## 3. Strateji: Yapı Bölme (Splitting)

Önerdiğim yapı:

```
CHANGELOG.md                                  (≈50 KB — TOC + Unreleased + 1.0.0 + 0.1.0)
├── CHANGELOG/
│   ├── README.md                            (TOC, milestone bağlantıları, bakım kuralı)
│   ├── unreleased/
│   │   ├── current.md                       (~100 KB — aktif geliştirme, son ~30 gün)
│   │   ├── m600-m699.md                     (~50 KB — eski M-blokları)
│   │   ├── m700-m799.md
│   │   ├── m800-m899.md
│   │   ├── m900-m999.md
│   │   └── m1000+.md
│   ├── v1.0.0.md                           (≈7 KB)
│   ├── v0.1.0.md                           (≈0.3 KB)
│   └── REORG-LOG.md                        (bölünme tarihçesi)
```

### 3.1 Yapının Dayanağı

- **Mirror imkanı**: `git log -- CHANGELOG.md` takibi zorlaşırsa, milestone dosyaları daha küçük olduğundan diff sorunsuz olur.
- **GitHub render**: GitHub `CHANGELOG/README.md` ve `CHANGELOG/v1.0.0.md` ayrı ayrı bağlanılabilir; marketplace/release pages için de uygun.
- **Lint**: `make check` veya `.github/workflows/ci.yml` bir helper (örn. `tools/changelog-lint`) ile Unreleased'in current.md altında olduğunu doğrulayabilir.

### 3.2 Milestone Aralığı Nasıl Belirlenir?

**Veri dayanağı:** Disk'te 697 `PHASE-M*.md` dosyası var, M1'den M923'e. CHANGELOG'daki 5.7K satır içerik 100+ M-faz'ı kapsıyor.

**Pratik kural:**
- 100'luk aralıklar (M100-199, M200-299, ...) **yeterli dilimleme** sağlar.
- 50'lik aralıklar (M50-99, M100-149, ...) **daha küçük dosyalar** verir.
- 25'lik aralıklar çok küçük olur: 50+ dosya, yönetilmesi zor.

**Öneri:** 100'lük aralıklar. 10'ar kadar dosya, ~100 KB her biri (Unreleased bloğunun mantıklı dilimleri).

### 3.3 Unreleased Bloğunun Parçalanması

Unreleased 5,784 satır. Bu satırları **mevcut zaman-stampr'ları** veya **M-sayı referansları** ile zenginleştirilmiş bir yardımcı ile dilimlemek için:

1. **Her paragrafın M-sayı referansını çıkart**: regex `(PHASE-M\d+|M\d{3,4})` ile. Çoğu girişte zaten `M-XXX` geçiyor.
2. **M-sayı belirsiz olan paragrafları** (örn. eki "added dep doc alignment") en yakın komşu M-referansına ata.
3. **Dilim dosyalarına** satırları yerleştir.

Bu bölünme **scripted** yapılabilir. Plan + script taslağı aşağıda §5'te.

---

## 4. Yapısal Tasarım Detayı

### 4.1 `CHANGELOG.md` Ana Dosyası (≤50 KB)

```
# Changelog
[intro paragraphs — 8-10 satır]

## [Unreleased] — currently at /CHANGELOG/unreleased/current.md
See CHANGELOG/unreleased/current.md for the in-flight changes.

## [1.0.0] — 2026-06-03
[full content (2,040 satır / ≈7 KB)]

## [0.1.0] — 2026-05-30
[full content (79 satır / ≈0.3 KB)]

## Older
See CHANGELOG/ dir for milestone-level subdivisions.
```

### 4.2 `CHANGELOG/unreleased/current.md`

Mevcut geliştirme bloğu (son 30 gün). Her PR buraya **tek bir veya birkaç madde** olarak girer.

### 4.3 `CHANGELOG/unreleased/mXXX-mYYY.md`

Her aralık dosyası `### /` ile başlayan `### Added` / `### Fixed` ... kategorileri içerir. Aralık dosyalarına "current.md → mXXX-mYYY'ye taşındı, <tarih>" notu eklenir.

### 4.4 `CHANGELOG/README.md`

```
# Changelog

Bu dizinde Agezt (`agezt` daemon + `agt` CLI) için per-milestone changelog yer alır.

## Yapı

- `v0.1.0.md`, `v1.0.0.md` — yayınlanmış sürümler.
- `unreleased/current.md` — aktif geliştirme (son ~30 gün).
- `unreleased/mXXX-mYYY.md` — eski M-blok aralıkları, reorg sırasında oluştu.
- `REORG-LOG.md` — milestone dosyalarının bölünme tarihçesi.

## PR Sırası

1. Yeni özellik, fix veya değişiklik → `unreleased/current.md` eklenir (PR-açılırken).
2. Current ~30 günlük döngüyü geçince → ilgili `mXXX-mYYY.md` aralığına taşınır.
3. Yeni sürüm kesildiğinde → yeni `vX.Y.Z.md` dosyası oluşturulur.
```

### 4.5 `CHANGELOG/REORG-LOG.md`

```
# 2026-07-04 — Reorg v1

`CHANGELOG.md` ~646 KB tek-dosyadan milestone-aralık dosyalarına bölündü.

## Mapping

- `unreleased/current.md`: son ~30 günlük Unreleased kısmı.
- `unreleased/m100-m199.md`: M100-M199 referanslı tüm paragraflar.
- `unreleased/m200-m299.md`: ...
- ... (her aralık)
- `v1.0.0.md` ve `v0.1.0.md`: orijinal versiyon blokları değişmeden.

## Araçlar

- `tools/changelog-split` (PR'da eklenecek): Unreleased'i otomatik dilimler.
- `tools/changelog-lint` (PR'da eklenecek): TOC + dosya varlığını doğrular.

## Migrating Back

Ana `CHANGELOG.md` yine de satır içi tutulur (back-compat), ama "silinen" değil. Mirror tooling gerektiğinde otomatik oluşturabilir.
```

---

## 5. Uygulama Adımları (sequence)

### 5.1 PR-1: Araç — `tools/changelog-split`

`tools/changelog-split/` paketi:

```go
package main
// Reads `CHANGELOG.md`, parses ## [Unreleased] block,
// extracts per-paragraph M-references (regex),
// splits content into milestone-range files,
// emits `CHANGELOG/unreleased/*.md`.
// Dry-run mode, --verify mode, --emit mode.
```

Kullanım:
```
go run ./tools/changelog-split --dry-run  # show planned output
go run ./tools/changelog-split --emit     # write files
go run ./tools/changelog-split --verify   # check current state matches
```

**Testler:**
- `TestSplitByMRange` — verilen M-references arasındaki paragrafları doğru dosyaya atar.
- `TestNoMReference` — referanssız paragraflar current.md'ye yazılır.
- `TestMergedSections` — birden çok paragraf aynı M-referansını paylaştığında birleştirilir.

### 5.2 PR-2: Araç — `tools/changelog-lint`

```go
package main
// Checks CHANGELOG.md structure:
// - ## [Unreleased] kısa, /CHANGELOG/unreleased/current.md'ye referans.
// - CHANGELOG/ dizini altında milestone dosyaları mevcut.
// - Her milestone dosyası `### Added`, `### Fixed`, etc. kategoriler içerir.
// - En az bir v0/v1 sürümü ayrı dosyada.
// - REORG-LOG.md mevcut.
```

`make check` veya CI gate'ine eklenir.

### 5.3 PR-3: Reorg — dosyaları yaz + CHANGELOG.md'yi kısalt

- `tools/changelog-split --emit` çalıştır.
- `CHANGELOG.md` üst-düzey içeriği sadece TOC + v1.0.0 + v0.1.0 versiyonları kalacak.
- `CHANGELOG/README.md` + `CHANGELOG/REORG-LOG.md` oluştur.

### 5.4 PR-4: Migration helper (opsiyonel)

Eski PR'lar için *eğer yazar Unreleased'e commit ettiyse* ve Unreleased yeniden şiştiyse, otomatik migrate komutu:

```
make changelog-migrate  # Unreleased içeriğini milestone dosyalarına taşır
```

---

## 6. Demo Gate (Phase 0 → Phase 1)

- ✅ `tools/changelog-split --verify` çalışıyor: `current.md` ile milestone dosyaları arasında split doğru.
- ✅ `tools/changelog-lint` CI gate'inde yeşil: `CHANGELOG.md` anahtar başlıkları mevcut.
- ✅ Toplam CHANGELOG ağacı ana `CHANGELOG.md` <50 KB.
- ✅ Mevcut `git log` ile her milestone PR değişikliği yalnızca ilgili milestone dosyasına dokunur.

### 6.1 Süre Tahmini

| Adım | Süre | Bağımlılık |
|---|---|---|
| PR-1 `tools/changelog-split` | 1-2 g | yok |
| PR-2 `tools/changelog-lint` | 0.5 g | PR-1 |
| PR-3 reorg (dosya yazma) | 0.5 g | PR-1, PR-2 (kuru çalıştırma) |
| PR-4 migration helper (opsiyonel) | 0.5 g | PR-3 |
| **Toplam** | **2-4 g** | |

---

## 7. Riskler ve Azaltmaları

| Risk | Olasılık | Azaltma |
|---|---|---|
| Bölünme sırasında M referansı olmayan paragraflar kaybolur | Orta | `--no-m-reference` → `current.md` (default); PR review'da görünür; lint ile Unreleased kısa kalır. |
| Eski PR'lar yeni yapıyı bilmeden ana `CHANGELOG.md`'ye ekleme yapar | Düşük-Orta | PR-2 lint → CI fail; PR-3'te katı bir "Unreleased sadece referans" kuralı konur. |
| Çoklu agent eş zamanlı reorg çakışması | Düşük | PR-4 migration helper ile koordine; her PR'da `tools/changelog-lint` invariant test. |
| Tarih format kaybı (özellikle `[1.0.0]` ve `[0.1.0]`) | Düşük | Ana `CHANGELOG.md` orijinal blokları saklar; alt dosyalar mirror. |
| Keep-a-Changelog format sapması | Çok Düşük | Yapı bu formatın "alternatif representasyonu"; içerik aynı. |

---

## 8. Senaryo Sonrası (post-org)

Bu PR'lar land edildikten sonra:

- `git log -- CHANGELOG/unreleased/current.md` ile aktif geliştirmenin diff'i küçük ve okunabilir.
- Yeni M-faz tamamlandığında: PHASE-M*.md raporları + `unreleased/mXXX-mYYY.md` güncelleme + ana `CHANGELOG.md`'de Unreleased satırı yok.
- Sürüm kesildiğinde: `CHANGELOG/vX.Y.Z.md` yeni dosya + ana `CHANGELOG.md`'ye satır.
- `make check` ve CI her PR'da invariant doğrular.

### 8.1 PR Politikaları (sonradan)

- **Yeni eklenen CHANGELOG satırı** ya `unreleased/current.md` (yeni özellik) veya `unreleased/mXXX-mYYY.md` (geçmişe ekleme) veya `vX.Y.Z.md` (sürüm patch) — başka dosya değil.
- **Eski CHANGELOG.md'ye append** → lint fail eder.
- **Tooling güncellemesi** (örn. `changelog-add` helper) PR'la gelir.

---

## 9. Snapshot (2026-07-04)

Bu plan aşamasında:

- Disk'te 1 dosya: `CHANGELOG.md` (646 KB, 7,914 satır)
- Reorg sonrası (beklenen):
  - `CHANGELOG.md` ≤ 50 KB
  - `CHANGELOG/README.md` ≈ 1 KB
  - `CHANGELOG/REORG-LOG.md` ≈ 1 KB
  - `CHANGELOG/v0.1.0.md` ≈ 0.3 KB
  - `CHANGELOG/v1.0.0.md` ≈ 7 KB
  - `CHANGELOG/unreleased/current.md` ≈ 100 KB
  - `CHANGELOG/unreleased/m100-m199.md`, `m200-m299.md`, ... (10 dosya, ortalama 50 KB)

Toplam: ≈ 50 + 1 + 1 + 0.3 + 7 + 100 + 50×10 ≈ **660 KB toplam** (mirror maliyeti), ama ana dosya %92 azalır.

---

## 10. Açık Sorular / Karar Bekleyenler

- **100'lük aralıklar vs 50'lik**: 100'lük daha yönetilebilir ama daha büyük dosyalar. Karar: **100'lük** ile başla, gerekirse 50'liğe geç.
- **`REORG-LOG.md` şart mı**: PR review için yararlı, ama her bölünme için büyüyebilir. Karar: **mevcut tut**, eski girişleri "summary" olarak birleştir.
- **CI gate erken mi**: `tools/changelog-lint` PR-1/PR-2 ile birlikte merge'lenirse CI henüz yok. Öneri: PR-2'de `--off` flag ile başla; PR-3 sonrası `--on` yap.

---

*Bu plan tamamen "release hygiene" kapsamındadır. Yeni özellik eklemez; sadece mevcut 646 KB'ı yönetilebilir ~10 dosyaya böler.*
