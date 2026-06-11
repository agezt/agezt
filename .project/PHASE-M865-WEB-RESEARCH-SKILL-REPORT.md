# PHASE M865 — Built-in web-research skill bundle

**Status:** shipped
**Milestone:** M865 (numbered to stay clear of concurrent in-progress
M858/M859 work in the tree).
**Theme:** Backlog #34 (more out-of-the-box capability): a sixth built-in skill
bundle that gives agents a disciplined multi-source web-research workflow —
gather, extract, cite, synthesize — instead of answering from memory or trusting
one page.

## What shipped

A built-in `web-research` bundle, seeded active at startup by the existing
`plugins/builtinskills` seeder (no daemon wiring change — `builtinBundles` +
`go:embed` only):

- `SKILL.md` — the discipline (frame the question, diversify sources, extract
  don't skim, cite as you go, note disagreement), when to use the `fetch` tool vs
  the batch helper vs escalating to **browser-use** for JS/gated pages, and how to
  synthesize a sourced answer.
- `scripts/setup.sh` — `pip install requests beautifulsoup4` (+ optional
  `trafilatura` for cleaner main-text extraction; non-fatal if it won't install).
- `scripts/extract.py` — fetches one or more URLs and prints title + clean main
  text per URL as JSON (`{results:[{url,status,title,text,chars}], errors:[…]}`).
  Uses trafilatura when present, falls back to BeautifulSoup (drops
  script/style/nav, collapses whitespace); one bad URL never sinks the batch.
- `reference/recipes.md` — search strategy, batch extract, citation format,
  handling JS-heavy/gated pages and PDFs, summarize-before-synthesize, honest gaps.

## Why this milestone is conflict-free

A new bundle touches **only** `plugins/builtinskills/` — never `cmd/agezt/main.go`
or `kernel/runtime`/`agent`/`governor` (the files a concurrent session is editing
for M858/M859). The seeder auto-loads it. It tests in isolation:
`go test ./plugins/builtinskills/` compiles just this package + `kernel/skill`.

## Verification

- **Isolated gate:** `go vet` clean; linux cross-build of `./plugins/builtinskills/`
  clean; `gofmt -l` empty; `python -m py_compile extract.py` passes; `sh -n setup.sh`
  clean. Package suite green — `TestSeedAll_InstallsWebResearch` asserts the bundle
  seeds **active** and materializes `extract.py` / `setup.sh` / `recipes.md`;
  bundle-count assertions now cover six bundles.
- No new Go dep; no new env. `.gitattributes` already forces LF on
  `plugins/builtinskills/**`. Full `go build ./...` / daemon smoke deliberately
  skipped — they'd compile the concurrent in-progress Go edits.

## Notes
- Six seeded bundles now ship out of the box: browser-use, computer-use,
  data-analysis, docker-services, git-ops, web-research. The skill explicitly
  hands off to browser-use when raw fetch returns too little — the bundles
  compose rather than overlap.
