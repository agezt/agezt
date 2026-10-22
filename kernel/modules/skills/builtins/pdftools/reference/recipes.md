# pdf-tools recipes

The helper (`scripts/pdf.py`) covers info/text/tables/merge/split. For anything
else, write `pypdf`/`pdfplumber`/`pymupdf` directly in `code_exec`. Patterns:

## Read an invoice / statement

```sh
python scripts/pdf.py '{"op":"text","path":"invoice.pdf"}'      # the prose
python scripts/pdf.py '{"op":"tables","path":"invoice.pdf"}'    # line items as rows
```
Hand the rows to the **data-analysis** skill to total/aggregate them in pandas.

## Mine a long report for a section

```sh
python scripts/pdf.py '{"op":"info","path":"report.pdf"}'             # how many pages?
python scripts/pdf.py '{"op":"text","path":"report.pdf","pages":"10-14","max_chars":6000}'
```

## Merge / split

```sh
python scripts/pdf.py '{"op":"merge","inputs":["jan.pdf","feb.pdf","mar.pdf"],"out":"q1.pdf"}'
python scripts/pdf.py '{"op":"split","path":"q1.pdf","pages":"1-4","out":"jan-only.pdf"}'
```

## Scanned PDF (text comes back empty) → OCR

A scan is images, not text. Install tesseract via the computer-use skill, then:
```sh
# easiest: ocrmypdf adds a text layer in place
pip install ocrmypdf && ocrmypdf in.pdf out.pdf
python scripts/pdf.py '{"op":"text","path":"out.pdf"}'
```
Or render + OCR page by page with pymupdf + pytesseract if you need control.

## Render pages to images (for vision / thumbnails)

```python
import fitz  # pymupdf
doc = fitz.open("doc.pdf")
for i, page in enumerate(doc):
    page.get_pixmap(dpi=150).save(f"page-{i+1}.png")
```
Register the PNGs with the `artifacts` tool so they show in Files.

## Fill a form

```python
from pypdf import PdfReader, PdfWriter
r = PdfReader("form.pdf"); w = PdfWriter(); w.append(r)
w.update_page_form_field_values(w.pages[0], {"name": "Ada Lovelace", "date": "2026-06-11"})
with open("filled.pdf", "wb") as fh: w.write(fh)
```

## Tips
- `pages` is 1-based: `"1-3"`, `"2"`, `"1,4,6"`. Omit it to act on the whole doc.
- Extracted tables are raw rows — clean headers/types in pandas before computing.
- Save generated PDFs with the `artifacts` tool so the operator sees them in Files.
