# PHASE M866 — Built-in pdf-tools skill bundle

**Status:** shipped
**Milestone:** M866 (numbered to stay clear of concurrent in-progress
M858/M859 work in the tree).
**Theme:** Backlog #34 (more out-of-the-box capability) — closes a gap both the
data-analysis (M861) and web-research (M865) bundles explicitly defer on: getting
data out of PDFs.

## What shipped

A built-in `pdf-tools` bundle, seeded active at startup by the existing
`plugins/builtinskills` seeder (no daemon wiring change — `builtinBundles` +
`go:embed` only):

- `SKILL.md` — when to mine a PDF programmatically, the helper ops, the scanned-PDF
  (OCR) escalation path, and handoffs to data-analysis (tables → pandas) and
  artifacts (generated PDFs → Files).
- `scripts/setup.sh` — `pip install pypdf pdfplumber`.
- `scripts/pdf.py` — one JSON-spec helper with five ops: `info` (pages +
  metadata), `text` (per-page extract with a 1-based `pages` range + `max_chars`),
  `tables` (pdfplumber rows per page), `merge` (inputs[] → out), `split` (page
  range → out). Prints JSON; one shared `parse_pages` for `"1-3"`/`"2"`/`"1,4,6"`.
- `reference/recipes.md` — invoices/statements, mining a section, merge/split,
  OCR for scans (ocrmypdf / pymupdf+pytesseract), render-to-images for vision,
  form-fill.

## Why this milestone is conflict-free

A new bundle touches **only** `plugins/builtinskills/` — never `cmd/agezt/main.go`
or `kernel/runtime`/`agent`/`governor` (the files a concurrent session is editing
for M858/M859). The seeder auto-loads it. It tests in isolation:
`go test ./plugins/builtinskills/` compiles just this package + `kernel/skill`.

## Verification

- **Isolated gate:** `go vet` clean; linux cross-build of `./plugins/builtinskills/`
  clean; `gofmt -l` empty; `python -m py_compile pdf.py` passes; `sh -n setup.sh`
  clean. Package suite green — `TestSeedAll_InstallsPDFTools` asserts the bundle
  seeds **active** and materializes `pdf.py` / `setup.sh` / `recipes.md`;
  bundle-count assertions now cover seven bundles.
- No new Go dep; no new env. `.gitattributes` already forces LF on
  `plugins/builtinskills/**`. Full `go build ./...` / daemon smoke deliberately
  skipped — they'd compile the concurrent in-progress Go edits.

## Notes
- Seven seeded bundles now ship out of the box: browser-use, computer-use,
  data-analysis, docker-services, git-ops, web-research, pdf-tools. They compose:
  web-research → pdf-tools for PDF links, pdf-tools → data-analysis for tables,
  pdf-tools → computer-use for OCR.
