# archive-tools recipes

The helper (`scripts/arc.py`) covers list/extract/zip/tar with a zip-slip guard,
using only the Python standard library. For more, use `zipfile`/`tarfile`
directly. Patterns:

## Peek inside before extracting

```sh
python scripts/arc.py '{"op":"list","path":"download.zip"}'
```

## Extract safely

```sh
python scripts/arc.py '{"op":"extract","path":"download.tar.gz","dest":"work"}'
```
The helper refuses any member whose path escapes `dest` (zip slip) and aborts.

## Bundle outputs into one deliverable

```sh
python scripts/arc.py '{"op":"zip","inputs":["report.pdf","charts/","summary.csv"],"out":"deliverable.zip"}'
```
Folders are added recursively. Then register `deliverable.zip` with the
`artifacts` tool so it appears in the Files view.

## Roll up logs as tar.gz

```sh
python scripts/arc.py '{"op":"tar","inputs":["logs/"],"out":"logs-2026-06.tar.gz"}'
```

## Password-protected zip (helper doesn't cover it)

```python
import pyzipper  # pip install pyzipper
with pyzipper.AESZipFile("secret.zip", "w", encryption=pyzipper.WZ_AES) as z:
    z.setpassword(b"hunter2"); z.writestr("note.txt", "classified")
```

## Selective extraction

```python
import zipfile
with zipfile.ZipFile("big.zip") as z:
    for n in z.namelist():
        if n.endswith(".csv"):
            z.extract(n, "csvs")
```

## The output pipeline

archive-tools is the last step of the visual/data pipelines: bundle the PNGs from
**image-tools**, the CSVs/charts from **data-analysis**, or a **pdf-tools** export
into one zip, then hand it to the `artifacts` tool. To gather a Data Lake export +
its charts for the operator, zip them together.
