# PHASE M896 — Built-in office-docs skill bundle

**Status:** shipped
**Milestone:** M896 (session range M889–M899; branched from `origin/main`,
concurrent local-main arc untouched).
**Theme:** Backlog **#34** — a sixteenth built-in skill bundle: generate Word
`.docx` and Excel `.xlsx` deliverables — the polished-output step the data/PDF
bundles stop short of.

## What shipped

A built-in `office-docs` bundle, seeded active at startup by the existing
`plugins/builtinskills` seeder (`builtinBundles` + `go:embed` only):

- `SKILL.md` — the two ops, the block/sheet specs, the reporting pipeline
  (data-analysis → office-docs → email/archive → artifacts), and the
  use-the-library-directly pointer for images/styles/templates.
- `scripts/setup.sh` — `pip install python-docx openpyxl`.
- `scripts/office.py` — one JSON-spec helper, two ops: `docx` (build a Word doc
  from typed blocks — heading / para / bullets / table, with a styled table),
  `xlsx` (build a workbook from `sheets{name:[[row]]}` or `rows[]`, bolding the
  header row and freezing the top row; sheet names capped at Excel's 31 chars).
- `reference/recipes.md` — report with table, multi-sheet workbook, from a pandas
  DataFrame, embed a chart image in Word, charts inside Excel, the pipeline.

## Why this milestone is conflict-free

A new bundle touches **only** `plugins/builtinskills/`. The seeder auto-loads it.
It tests in isolation: `go test ./plugins/builtinskills/`. Branched from
`origin/main` (my M862–M895), concurrent local-main arc untouched.

## Verification

- **Isolated gate:** `go vet` clean; linux cross-build of `./plugins/builtinskills/`
  clean; `gofmt -l` empty; `python -m py_compile office.py` passes; `sh -n setup.sh`
  clean. Package suite green — `TestSeedAll_InstallsOfficeDocs` asserts the bundle
  seeds **active** and materializes `office.py` / `setup.sh` / `recipes.md`;
  bundle-count assertions now cover sixteen bundles.
- No new Go dep; no new env. `.gitattributes` already forces LF on
  `plugins/builtinskills/**`. Full `go build ./...` deliberately skipped.

## Notes
- Sixteen seeded bundles now ship. office-docs completes the reporting pipeline:
  data-analysis crunches → office-docs formats a `.docx`/`.xlsx` → email-tools
  sends or archive-tools bundles → artifacts surfaces it in Files. The
  agent-level upgrade is "hand the operator a real document," not just a CSV/PDF.
