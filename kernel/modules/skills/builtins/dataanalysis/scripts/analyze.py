#!/usr/bin/env python3
"""data-analysis helper — load a data file and summarize it with pandas.

Usage:  python analyze.py '<json-spec>'   (or pipe the JSON on stdin)
Spec:   { "path": "data.csv", "describe": true, "head": 10,
          "group_by": {"by":"col","agg":"sum","column":"num"},
          "chart": {"kind":"bar","x":"col","y":"num","out":"chart.png"} }
Output: one JSON object on stdout (shape, dtypes, describe, head, group_by?, chart?).

A fast start, not a cage: for anything beyond this, write your own pandas in code_exec.
Install once with scripts/setup.sh (pandas + matplotlib + openpyxl + pyarrow).
"""
import json
import sys


def read_spec():
    if len(sys.argv) > 1 and sys.argv[1].strip():
        return json.loads(sys.argv[1])
    data = sys.stdin.read()
    if data.strip():
        return json.loads(data)
    raise ValueError("no spec: pass a JSON spec as argv[1] or on stdin")


def load(path):
    import pandas as pd

    low = path.lower()
    if low.endswith((".xlsx", ".xls")):
        return pd.read_excel(path)
    if low.endswith(".json"):
        return pd.read_json(path)
    if low.endswith(".parquet"):
        return pd.read_parquet(path)
    if low.endswith(".tsv"):
        return pd.read_csv(path, sep="\t")
    return pd.read_csv(path)


def run(spec):
    if not spec.get("path"):
        raise ValueError("spec.path is required")
    df = load(spec["path"])

    out = {
        "ok": True,
        "rows": int(df.shape[0]),
        "cols": [str(c) for c in df.columns],
        "dtypes": {str(c): str(t) for c, t in df.dtypes.items()},
    }

    if spec.get("describe", True):
        try:
            out["describe"] = json.loads(df.describe(include="all").to_json())
        except Exception:  # noqa: BLE001
            out["describe"] = json.loads(df.describe().to_json())

    head_n = int(spec.get("head", 10))
    out["head"] = json.loads(df.head(head_n).to_json(orient="records"))

    gb = spec.get("group_by")
    if gb and gb.get("by"):
        agg = gb.get("agg", "sum")
        col = gb.get("column")
        grouped = df.groupby(gb["by"])
        series = getattr(grouped[col] if col else grouped, agg)()
        out["group_by"] = json.loads(series.to_json())

    ch = spec.get("chart")
    if ch and ch.get("x"):
        import matplotlib

        matplotlib.use("Agg")
        import matplotlib.pyplot as plt

        kind = ch.get("kind", "bar")
        x, y = ch["x"], ch.get("y")
        fig, ax = plt.subplots(figsize=(8, 4.5))
        plot_df = df.set_index(x)[y] if y else df.set_index(x)
        plot_df.plot(kind=kind, ax=ax)
        fig.tight_layout()
        out_path = ch.get("out", "chart.png")
        fig.savefig(out_path)
        out["chart"] = out_path

    return out


def main():
    try:
        print(json.dumps(run(read_spec()), default=str))
    except Exception as e:  # noqa: BLE001
        print(json.dumps({"ok": False, "error": str(e)}))
        sys.exit(1)


if __name__ == "__main__":
    main()
