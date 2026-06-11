---
name: archive-tools
description: Pack and unpack archives — zip and tar/tar.gz — list what's inside without extracting, create an archive from files, and extract safely (guarding against path-traversal), when a task hands you a .zip/.tar.gz or needs to bundle outputs
triggers: [zip, unzip, archive, tar, tar.gz, tgz, compress, extract, unpack, bundle, gzip]
tools: [code_exec, shell, artifacts]
---

# archive-tools — pack and unpack archives safely

When a task hands you a `.zip` or `.tar.gz` to open, or needs to bundle a folder
of outputs into one downloadable file, do it programmatically. This skill runs
through `code_exec` (python) and uses only the **standard library** — no install.

## No setup needed

`zipfile`, `tarfile`, and `shutil` ship with Python. Use `skill op=files
archive-tools` to find the bundle directory, then run the helper.

## The helper

`scripts/arc.py` takes a JSON spec with an `op` and prints JSON. Ops:

```sh
# List entries without extracting:
python scripts/arc.py '{"op":"list","path":"bundle.zip"}'

# Extract into a directory (path-traversal-guarded):
python scripts/arc.py '{"op":"extract","path":"bundle.tar.gz","dest":"out"}'

# Create a zip from files/folders:
python scripts/arc.py '{"op":"zip","inputs":["report.pdf","charts/"],"out":"deliverable.zip"}'

# Create a tar.gz:
python scripts/arc.py '{"op":"tar","inputs":["logs/"],"out":"logs.tar.gz"}'
```

### Spec fields
- `op` (required) — `list` | `extract` | `zip` | `tar`.
- `path` — the archive (list/extract). Format inferred from the extension
  (`.zip`, `.tar`, `.tar.gz`/`.tgz`).
- `dest` — extract target dir (created if absent; default `"extracted"`).
- `inputs` — files/dirs to pack (zip/tar). Folders are added recursively.
- `out` — output archive path (zip/tar).

### Output (JSON on stdout)
```
{ "ok": true, "op": "list", "entries": ["a.txt","sub/b.csv"], "count": 2 }
{ "ok": true, "op": "extract", "dest": "out", "files": 12 }
{ "ok": true, "op": "zip", "out": "deliverable.zip", "files": 5 }
```

## Safety: extract is path-traversal-guarded

A malicious archive can carry entries like `../../etc/passwd` ("zip slip"). The
helper **refuses** any member that would land outside `dest` and aborts the
extract. When you write your own extraction in `code_exec`, do the same — resolve
each member against `dest` and skip anything that escapes it.

## Going further

The helper is a fast start, not a cage — for password-protected zips, streaming
large archives, or selective extraction, use `zipfile`/`tarfile` directly. Pairs
with everything: bundle the PNGs from **image-tools**, the CSVs from
**data-analysis**, or a **pdf-tools** export into one archive, then register it
with the `artifacts` tool so it shows in the Files view. See
`reference/recipes.md`.
