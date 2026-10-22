---
name: data-analysis
description: Analyze spreadsheets and data files — CSV, Excel, JSON, Parquet — with pandas, summarize and aggregate them, and make charts, when a task needs real number-crunching instead of eyeballing
triggers: [data, csv, excel, spreadsheet, analyze, pandas, chart, plot, aggregate, statistics, json]
tools: [code_exec, shell, artifacts]
---

# Data analysis — crunch real data with pandas

When a task involves a data file (CSV, Excel, JSON, Parquet) or columns of
numbers, don't eyeball it — load it into pandas and compute. This skill runs
through the `code_exec` tool (language: python).

## One-time setup

Run `scripts/setup.sh` once (via code_exec or shell): it installs `pandas`,
`matplotlib`, and `openpyxl` (for Excel). Use `skill op=files data-analysis` to
find the bundle directory.

## Quick analysis with the helper

`scripts/analyze.py` loads a file and prints a structured summary as JSON —
shape, columns/dtypes, numeric describe(), and the head. Optional group-by
aggregation and a chart. Run it via code_exec:

```
python scripts/analyze.py '{"path":"data.csv"}'
```

### Spec fields

- `path` (required) — the data file. Format is inferred from the extension
  (`.csv`, `.tsv`, `.json`, `.xlsx`/`.xls`, `.parquet`).
- `describe` (optional, default true) — include `df.describe()` for numeric cols.
- `group_by` (optional) — `{"by":"<col>","agg":"sum","column":"<numeric col>"}`
  → a grouped aggregate (agg ∈ sum/mean/count/min/max).
- `chart` (optional) — `{"kind":"bar"|"line","x":"<col>","y":"<col>","out":"chart.png"}`
  → save a PNG you can register with the `artifacts` tool to show in Files.
- `head` (optional, default 10) — rows to preview.

### Output (JSON on stdout)

```
{ "ok": true, "rows": 1234, "cols": ["a","b"], "dtypes": {...},
  "describe": {...}, "head": [...], "group_by": {...}?, "chart": "<path>"? }
```

## Going further

For anything the helper doesn't cover, write your own pandas in `code_exec` — the
helper is a fast start, not a cage. See `reference/recipes.md` for joins, time
series, pivot tables, and cleaning patterns. To analyze a Data Lake collection,
export it (the `db` tool / `/api/data/records`) to JSON first, then point
`analyze.py` at it.
