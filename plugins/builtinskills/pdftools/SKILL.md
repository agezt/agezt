---
name: pdf-tools
description: Work with PDF files — pull out the text and tables, read metadata and page counts, merge or split documents, and extract pages — when a task hands you a PDF (a report, invoice, paper, statement) instead of clean data
triggers: [pdf, document, invoice, report, statement, extract text, tables, merge, split, page, ocr, scan]
tools: [code_exec, shell, artifacts]
---

# pdf-tools — get the data out of PDFs

When a task hands you a PDF — an invoice to read, a report to mine, a paper to
quote, statements to merge — don't guess at its contents. Pull the text and
tables out programmatically. This skill runs through `code_exec` (python).

## One-time setup

Run `scripts/setup.sh` once (via code_exec or shell): installs `pypdf` (merge,
split, metadata) and `pdfplumber` (text + tables). Use `skill op=files pdf-tools`
to find the bundle directory.

## The helper

`scripts/pdf.py` takes a JSON spec with an `op` and prints JSON. Ops:

```sh
# Metadata + page count:
python scripts/pdf.py '{"op":"info","path":"doc.pdf"}'

# Extract text (all pages, or a range):
python scripts/pdf.py '{"op":"text","path":"doc.pdf","pages":"1-3","max_chars":8000}'

# Extract tables (per page, as rows):
python scripts/pdf.py '{"op":"tables","path":"invoice.pdf","pages":"1"}'

# Merge several PDFs into one:
python scripts/pdf.py '{"op":"merge","inputs":["a.pdf","b.pdf"],"out":"merged.pdf"}'

# Split out a page range into a new file:
python scripts/pdf.py '{"op":"split","path":"doc.pdf","pages":"2-4","out":"slice.pdf"}'
```

### Spec fields
- `op` (required) — `info` | `text` | `tables` | `merge` | `split`.
- `path` — the source PDF (for info/text/tables/split).
- `pages` — 1-based range like `"1-3"`, `"2"`, or `"1,4,6"`; omit for all.
- `max_chars` — truncate extracted text (text op).
- `inputs` / `out` — for merge (`inputs[]` → `out`) and split (`out`).

### Output (JSON on stdout)
```
{ "ok": true, "op": "text", "pages": 3, "text": "...", "chars": 1234 }
{ "ok": true, "op": "tables", "tables": [ {"page":1,"rows":[[...],[...]]} ] }
{ "ok": true, "op": "merge", "out": "merged.pdf", "pages": 12 }
```

## Scanned PDFs (no extractable text)

If `text` comes back nearly empty, the PDF is a **scan** (images, not text). OCR
it: install `tesseract` via the computer-use skill, render pages to images, and
run OCR — or convert with `ocrmypdf`:
```sh
ocrmypdf doc.pdf doc-ocr.pdf && python scripts/pdf.py '{"op":"text","path":"doc-ocr.pdf"}'
```

## Going further

The helper is a fast start, not a cage — for form-filling, redaction, or
image-per-page rendering, write `pypdf`/`pdfplumber`/`pymupdf` directly in
`code_exec`. To analyze extracted tables, hand them to the **data-analysis**
skill (load into pandas). Save any generated PDF with the `artifacts` tool so it
shows in Files. See `reference/recipes.md`.
