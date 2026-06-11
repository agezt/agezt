# PHASE M879 — Built-in image-tools skill bundle

**Status:** shipped
**Milestone:** M879 (numbered above the concurrent session's M868–M876 arc).
**Theme:** Backlog **#34** (more out-of-the-box capability) — an eighth built-in
skill bundle that completes the visual pipeline: manipulate images with Pillow.

## What shipped

A built-in `image-tools` bundle, seeded active at startup by the existing
`plugins/builtinskills` seeder (no daemon wiring change — `builtinBundles` +
`go:embed` only):

- `SKILL.md` — when to transform an image programmatically, the helper ops, the
  OCR escalation (tesseract via computer-use), and handoffs to artifacts/Files.
- `scripts/setup.sh` — `pip install Pillow`.
- `scripts/img.py` — one JSON-spec helper, eight ops: `info` (size/mode/format/
  EXIF), `resize` (aspect-preserving when one of w/h given), `convert` (format
  from out extension, RGBA→RGB for JPEG), `crop`, `thumb`, `rotate`, `grayscale`,
  `annotate` (stamp text). Prints JSON.
- `reference/recipes.md` — shrink screenshots, batch thumbnails, OCR, EXIF
  stripping, watermark/compose, and the visual pipeline (browser-use → image-tools,
  pdf-tools → image-tools, data-analysis charts → image-tools).

## Why this milestone is conflict-free

A new bundle touches **only** `plugins/builtinskills/` — never `cmd/agezt/main.go`
or `kernel/runtime`/`agent`/`governor` (the files a concurrent session is editing).
The seeder auto-loads it. It tests in isolation:
`go test ./plugins/builtinskills/` compiles just this package + `kernel/skill`.

## Verification

- **Isolated gate:** `go vet` clean; linux cross-build of `./plugins/builtinskills/`
  clean; `gofmt -l` empty; `python -m py_compile img.py` passes; `sh -n setup.sh`
  clean. Package suite green — `TestSeedAll_InstallsImageTools` asserts the bundle
  seeds **active** and materializes `img.py` / `setup.sh` / `recipes.md`;
  bundle-count assertions now cover eight bundles.
- No new Go dep; no new env. `.gitattributes` already forces LF on
  `plugins/builtinskills/**`. Full `go build ./...` / daemon smoke deliberately
  skipped — they'd compile the concurrent in-progress Go edits.

## Notes
- Eight seeded bundles now ship out of the box: browser-use, computer-use,
  data-analysis, docker-services, git-ops, web-research, pdf-tools, image-tools.
  The visual ones compose: a browser screenshot or a rendered PDF page flows into
  image-tools for crop/resize/annotate, then to the artifacts tool for Files.
