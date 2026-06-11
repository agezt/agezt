#!/usr/bin/env python3
"""sql-db helper — run SQL against SQLite/PostgreSQL/MySQL via SQLAlchemy.

Usage:  python db.py '<json-spec>'   (or pipe the JSON on stdin)
Spec:   { "url": "<sqlalchemy-url>", "op": "tables|schema|query|exec",
          "sql": "...", "params": {...}, "table": "...", "limit": 200 }
Output: one JSON object on stdout.

Always bind values with :name + params — never string-format input into SQL.
A fast start, not a cage: for transactions/bulk loads, use SQLAlchemy directly.
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


def engine_for(url):
    from sqlalchemy import create_engine

    if not url:
        raise ValueError("spec.url is required (a SQLAlchemy connection string)")
    return create_engine(url)


def op_tables(eng, spec):
    from sqlalchemy import inspect

    return {"tables": sorted(inspect(eng).get_table_names())}


def op_schema(eng, spec):
    from sqlalchemy import inspect

    table = spec.get("table")
    if not table:
        raise ValueError("schema needs spec.table")
    cols = inspect(eng).get_columns(table)
    return {
        "table": table,
        "columns": [
            {"name": c["name"], "type": str(c.get("type")), "nullable": bool(c.get("nullable", True))}
            for c in cols
        ],
    }


def op_query(eng, spec):
    from sqlalchemy import text

    sql = spec.get("sql")
    if not sql:
        raise ValueError("query needs spec.sql")
    limit = int(spec.get("limit", 200))
    with eng.connect() as conn:
        res = conn.execute(text(sql), spec.get("params") or {})
        cols = list(res.keys())
        rows = []
        for i, row in enumerate(res):
            if i >= limit:
                break
            rows.append({c: row[j] for j, c in enumerate(cols)})
    return {"rows": rows, "count": len(rows), "truncated": len(rows) >= limit}


def op_write(eng, spec):
    """INSERT/UPDATE/DDL — the JSON op name is 'exec'."""
    from sqlalchemy import text

    sql = spec.get("sql")
    if not sql:
        raise ValueError("exec needs spec.sql")
    with eng.begin() as conn:  # begin() commits on success
        res = conn.execute(text(sql), spec.get("params") or {})
        return {"rowcount": res.rowcount}


OPS = {"tables": op_tables, "schema": op_schema, "query": op_query, "exec": op_write}


def run(spec):
    op = spec.get("op")
    if op not in OPS:
        raise ValueError("spec.op must be one of: " + ", ".join(OPS))
    eng = engine_for(spec.get("url"))
    try:
        handler = OPS[op]
        result = handler(eng, spec)
    finally:
        eng.dispose()
    result.update({"ok": True, "op": op})
    return result


def main():
    try:
        print(json.dumps(run(read_spec()), default=str))
    except Exception as e:  # noqa: BLE001
        print(json.dumps({"ok": False, "error": str(e)}))
        sys.exit(1)


if __name__ == "__main__":
    main()
