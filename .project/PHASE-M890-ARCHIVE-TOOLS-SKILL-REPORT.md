# PHASE M890 — Built-in archive-tools skill bundle

**Status:** shipped
**Milestone:** M890 (this session's range is M889+; concurrent session holds
M880–M888 + M900).
**Theme:** Backlog **#34** (more out-of-the-box capability) — a tenth built-in
skill bundle: pack/unpack zip and tar(.gz) archives, with a path-traversal guard.
The natural last step of the output pipeline (bundle artifacts into one file).

## What shipped

A built-in `archive-tools` bundle, seeded active at startup by the existing
`plugins/builtinskills` seeder (no daemon wiring change — `builtinBundles` +
`go:embed` only). **Zero pip dependencies** — it uses only the Python stdlib
(`zipfile`/`tarfile`/`shutil`), so there's no `setup.sh`:

- `SKILL.md` — when to pack/unpack, the helper ops, and the zip-slip safety note.
- `scripts/arc.py` — one JSON-spec helper, four ops: `list` (entries without
  extracting), `extract` (path-traversal-guarded — refuses any member that would
  land outside `dest`), `zip` (create from files/dirs, recursive), `tar` (`.tar`
  or `.tar.gz` by extension). Archive names keep the input folder and use forward
  slashes on every platform.
- `reference/recipes.md` — peek/extract, bundle a deliverable, roll up logs,
  password-protected zips (pyzipper), selective extraction, the output pipeline.

## Why this milestone is conflict-free

A new bundle touches **only** `plugins/builtinskills/` — never `cmd/agezt/main.go`
or `kernel/runtime`/`agent`/`governor`. The seeder auto-loads it. It tests in
isolation: `go test ./plugins/builtinskills/`.

## Verification

- **Isolated gate:** `go vet` clean; linux cross-build of `./plugins/builtinskills/`
  clean; `gofmt -l` empty; `python -m py_compile arc.py` passes. Package suite green
  — `TestSeedAll_InstallsArchiveTools` asserts the bundle seeds **active** and
  materializes `arc.py` / `recipes.md`; bundle-count assertions now cover ten
  bundles.
- **Functional smoke (stdlib, ran locally):** zip a folder → `list` shows
  `src/a.txt`,`src/b.txt` → `extract` restores `unpacked/src/{a,b}.txt`; tar.gz
  round-trips the same. This caught and fixed a cross-platform `arcname` bug
  (a trailing `/` collapsed the folder prefix when `os.sep` is `\`); now both
  separators are stripped and paths normalize to `/`.
- No new Go dep; no new env. `.gitattributes` already forces LF on
  `plugins/builtinskills/**`. Full `go build ./...` deliberately skipped.

## Notes
- Ten seeded bundles now ship: browser-use, computer-use, data-analysis,
  docker-services, git-ops, web-research, pdf-tools, image-tools, sql-db,
  archive-tools. archive-tools closes the output pipeline — gather image-tools
  PNGs / data-analysis CSVs / pdf-tools exports into one zip → artifacts → Files.
