# office-docs recipes

The helper (`scripts/office.py`) builds `.docx` and `.xlsx` from a JSON spec. For
images, charts-in-Excel, styles, or templates, use `python-docx`/`openpyxl`
directly. Patterns:

## A report with a heading, prose, and a table

```sh
python scripts/office.py '{"op":"docx","out":"weekly.docx","title":"Weekly Report",
  "blocks":[
    {"type":"heading","text":"Highlights","level":1},
    {"type":"para","text":"Strong week across the board."},
    {"type":"bullets","items":["Revenue +12%","Churn -2%"]},
    {"type":"table","header":["Metric","Value"],"rows":[["MRR","$42k"],["Users","1203"]]}
  ]}'
```

## A multi-sheet workbook

```sh
python scripts/office.py '{"op":"xlsx","out":"q2.xlsx","sheets":{
  "Revenue":[["Month","Amount"],["Apr","38000"],["May","45000"],["Jun","51000"]],
  "Headcount":[["Team","People"],["Eng","12"],["Sales","6"]]
}}'
```
The first row of each sheet is bolded and the top row is frozen.

## From a pandas DataFrame (with data-analysis)

```python
import pandas as pd
df = pd.read_csv("sales.csv")
df.to_excel("sales.xlsx", index=False)          # openpyxl backend
# or as a Word table: pass df.columns + df.values.tolist() as header/rows to the helper
```

## Embed a chart image in a Word doc

```python
from docx import Document
doc = Document(); doc.add_heading("Spend", 1)
doc.add_picture("spend.png")                      # a chart from data-analysis/image-tools
doc.save("report.docx")
```

## Charts inside Excel

```python
from openpyxl import Workbook
from openpyxl.chart import BarChart, Reference
wb = Workbook(); ws = wb.active
for row in [["M","V"],["Jun",42],["Jul",51]]: ws.append(row)
ch = BarChart(); ch.add_data(Reference(ws, min_col=2, min_row=1, max_row=3), titles_from_data=True)
ch.set_categories(Reference(ws, min_col=1, min_row=2, max_row=3))
ws.add_chart(ch, "D2"); wb.save("charted.xlsx")
```

## The reporting pipeline
data-analysis (numbers) → office-docs (.docx/.xlsx) → email-tools (send) /
archive-tools (bundle) → artifacts tool (Files). Save the file with `artifacts`
so the operator sees it in the Files view.
