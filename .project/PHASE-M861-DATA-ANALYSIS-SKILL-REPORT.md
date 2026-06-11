# PHASE M861 — Built-in data-analysis skill bundle

**Status:** shipped
**Milestone:** M861 (numbered to stay clear of concurrent in-progress
M858/M859 work in the tree).
**Theme:** A third out-of-the-box skill bundle (after browser-use M852 and
computer-use M853): real number-crunching over data files with pandas. Extends
#34 ("more capabilities") and pairs naturally with the Data Lake.

## What shipped

A built-in `data-analysis` bundle, seeded active at startup by the existing
`plugins/builtinskills` seeder (no daemon wiring change — `builtinBundles` +
`go:embed` only):

- `SKILL.md` — when and how to load CSV/Excel/JSON/Parquet into pandas, run the
  helper, and go further by writing pandas directly.
- `scripts/setup.sh` — `pip install pandas matplotlib openpyxl pyarrow`.
- `scripts/analyze.py` — a stateless helper: loads a file (format inferred from
  extension), prints a JSON summary (shape, dtypes, `describe()`, head), and
  optionally a group-by aggregate and a saved chart PNG. The agent runs it via
  `code_exec`.
- `reference/recipes.md` — cleaning, joins, pivots, time series, charts→artifacts,
  and analyzing a Data Lake collection.

## Why this milestone is conflict-free

A new bundle touches **only** `plugins/builtinskills/` — never `cmd/agezt/main.go`
or `kernel/runtime`/`agent`/`governor` (the files a concurrent session is
actively editing for M858/M859). The seeder auto-loads it. It also tests in
isolation: `go test ./plugins/builtinskills/` compiles just this package +
`kernel/skill`, so verification never compiles the in-flux Go kernel.

## Verification

- **Isolated gate:** `go vet`, `staticcheck`, and a linux build of
  `./plugins/builtinskills/` clean; `python -m py_compile analyze.py` passes; the
  package test suite green — `SeedAll` installs **browser-use + computer-use +
  data-analysis**, and `TestSeedAll_InstallsDataAnalysis` asserts the bundle's
  `analyze.py` / `setup.sh` / `recipes.md` are materialized. No new Go dep; no new
  env. `.gitattributes` already forces LF on `plugins/builtinskills/**`.
- Full `go build ./...` and a daemon smoke were deliberately skipped — they'd
  compile the concurrent in-progress Go edits.

## Notes
- Three seeded bundles now ship out of the box. Future ones (pdf-tools, git-ops,
  …) drop into `builtinBundles` the same way.
