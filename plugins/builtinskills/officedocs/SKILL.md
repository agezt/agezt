---
name: office-docs
description: Generate Microsoft Office documents — a Word .docx (headings, paragraphs, bullet lists, tables) and an Excel .xlsx workbook (sheets of rows with a styled header) — when a task needs a polished, shareable report or spreadsheet instead of raw CSV, PDF, or plain text
triggers: [docx, word, xlsx, excel, spreadsheet, report, document, office, workbook, deliverable]
tools: [code_exec, shell, artifacts]
---

# office-docs — produce Word and Excel deliverables

When a task should hand someone a real **document** — a formatted report in Word,
a spreadsheet in Excel — don't stop at CSV or plain text. This skill builds
`.docx` and `.xlsx` files via `python-docx` and `openpyxl`, through `code_exec`
(python).

## One-time setup

Run `scripts/setup.sh` once (via code_exec or shell): installs `python-docx` and
`openpyxl`. Use `skill op=files office-docs` to find the bundle directory.

## The helper

`scripts/office.py` takes a JSON spec with an `op` and prints JSON. Ops:

```sh
# Build a Word document from blocks:
python scripts/office.py '{"op":"docx","out":"report.docx","title":"Weekly report","blocks":[
  {"type":"heading","text":"Summary","level":1},
  {"type":"para","text":"Revenue is up 12% week over week."},
  {"type":"bullets","items":["Signups +8%","Churn -2%","NPS 41"]},
  {"type":"table","header":["Metric","Value"],"rows":[["Revenue","$42k"],["Users","1,203"]]}
]}'

# Build an Excel workbook (one or more sheets of rows):
python scripts/office.py '{"op":"xlsx","out":"data.xlsx","sheets":{
  "Sales":[["Month","Amount"],["Jun","42000"],["Jul","51000"]],
  "Users":[["Day","Signups"],["Mon","8"],["Tue","11"]]
}}'
```

### Spec fields
- `op` — `docx` | `xlsx`.
- **docx:** `out` (default `document.docx`), `title` (optional document title),
  `blocks` — a list of `{type, …}`:
  - `heading` `{text, level=1}`, `para` `{text}`, `bullets` `{items:[…]}`,
    `table` `{header:[…], rows:[[…]]}`.
- **xlsx:** `out` (default `workbook.xlsx`), and either `sheets`
  (`{name: [[row],[row]]}`) or `rows` (`[[…]]`, written to a sheet named by
  `sheet`, default `Sheet1`). The first row of each sheet is styled as a bold
  header with a frozen top row.

### Output (JSON on stdout)
```
{ "ok": true, "op": "docx", "out": "report.docx", "blocks": 4 }
{ "ok": true, "op": "xlsx", "out": "data.xlsx", "sheets": ["Sales","Users"] }
```

## The reporting pipeline

office-docs is the polished-output step:
- Crunch numbers with **data-analysis** → write the table into an `.xlsx`/`.docx`.
- Chart with data-analysis / **image-tools** → embed the PNG (write `python-docx`
  directly: `doc.add_picture("chart.png")`).
- Then **email-tools** to send it, or **archive-tools** to bundle several.
- Save the file with the `artifacts` tool so it shows in the Files view.

## Going further

The helper is a fast start, not a cage — for styles, headers/footers, images,
charts-in-Excel, cell formatting, or templates, use `python-docx`/`openpyxl`
directly in `code_exec`. See `reference/recipes.md`.
