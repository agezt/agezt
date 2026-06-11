---
name: sql-db
description: Query SQL databases — SQLite, PostgreSQL, MySQL — run SELECTs and statements, list and describe tables, and pull results out as JSON, when a task needs to read or write a real database instead of flat files
triggers: [sql, database, db, query, postgres, postgresql, mysql, sqlite, select, table, schema, join]
tools: [code_exec, shell, artifacts]
---

# sql-db — talk to SQL databases

When a task needs a real database — read rows from Postgres, query the SQLite
file an app left behind, write to the MySQL the docker-services skill spun up —
connect with a driver and run SQL. This skill runs through `code_exec` (python).

## One-time setup

Run `scripts/setup.sh` once (via code_exec or shell): it installs SQLAlchemy plus
the Postgres (`psycopg2-binary`) and MySQL (`PyMySQL`) drivers. SQLite needs no
driver (it's in the Python stdlib). Use `skill op=files sql-db` to find the bundle
directory.

## The helper

`scripts/db.py` takes a JSON spec with a `url` (SQLAlchemy connection string) and
an `op`, and prints JSON. Ops:

```sh
# List tables:
python scripts/db.py '{"url":"sqlite:///app.db","op":"tables"}'

# Describe a table (columns + types):
python scripts/db.py '{"url":"sqlite:///app.db","op":"schema","table":"users"}'

# Run a SELECT (rows come back as JSON, capped by `limit`):
python scripts/db.py '{"url":"postgresql://u:p@localhost:5432/db","op":"query",
  "sql":"select id,email from users where active = :a","params":{"a":true},"limit":200}'

# Run a statement (INSERT/UPDATE/DDL) — returns affected row count:
python scripts/db.py '{"url":"sqlite:///app.db","op":"exec",
  "sql":"update users set seen = 1 where id = :id","params":{"id":7}}'
```

### Connection strings
- SQLite: `sqlite:////absolute/path.db` or `sqlite:///relative.db`
- PostgreSQL: `postgresql://user:pass@host:5432/dbname`
- MySQL: `mysql+pymysql://user:pass@host:3306/dbname`

### Spec fields
- `url` (required), `op` (`tables` | `schema` | `query` | `exec`), `sql`,
  `params` (named, bound via `:name` — **always** use these, never string-format
  user input into SQL), `table` (schema op), `limit` (query op, default 200).

### Output (JSON on stdout)
```
{ "ok": true, "op": "query", "rows": [ {...} ], "count": 12 }
{ "ok": true, "op": "exec", "rowcount": 1 }
{ "ok": true, "op": "tables", "tables": ["users","orders"] }
```

## Safety

- **Always bind parameters** (`:name` + `params`) — never concatenate values into
  SQL. This is the one hard rule; it prevents both injection and quoting bugs.
- `query` is read-shaped (returns rows); `exec` is for writes/DDL. Pick the right
  one so a typo'd SELECT doesn't silently no-op.

## Going further

The helper is a fast start, not a cage — for transactions, bulk loads, or ORMs,
use SQLAlchemy directly in `code_exec`. Pairs with **docker-services** (spin up
the Postgres/MySQL, then point `url` at `localhost`) and **data-analysis** (load
a query's rows straight into pandas: `pd.read_sql(sql, engine)`). See
`reference/recipes.md`.
