# data-analysis recipes

The helper (`scripts/analyze.py`) covers load → summarize → group-by → chart. For
everything else, write pandas directly in `code_exec`. Common patterns:

## Clean before you compute

```python
import pandas as pd
df = pd.read_csv("data.csv")
df.columns = df.columns.str.strip().str.lower()      # tidy headers
df = df.drop_duplicates()
df["amount"] = pd.to_numeric(df["amount"], errors="coerce")  # bad cells → NaN
df = df.dropna(subset=["amount"])
```

## Aggregate / pivot

```python
df.groupby("category")["amount"].agg(["sum", "mean", "count"])
df.pivot_table(index="month", columns="category", values="amount", aggfunc="sum")
```

## Join two files

```python
a = pd.read_csv("orders.csv"); b = pd.read_csv("customers.csv")
merged = a.merge(b, on="customer_id", how="left")
```

## Time series

```python
df["date"] = pd.to_datetime(df["date"])
monthly = df.set_index("date").resample("ME")["amount"].sum()
```

## Charts → artifacts

Save a PNG and register it with the `artifacts` tool so it shows in the Files
view:

```python
import matplotlib; matplotlib.use("Agg")
import matplotlib.pyplot as plt
ax = monthly.plot(kind="line", figsize=(8, 4.5), title="Spend / month")
ax.figure.tight_layout(); ax.figure.savefig("spend.png")
```

## Analyze a Data Lake collection

Export the collection to JSON (the `db` tool, or `GET /api/data/records`), write
it to a file, then `python scripts/analyze.py '{"path":"expenses.json"}'` — or
load it straight into pandas with `pd.read_json`.
