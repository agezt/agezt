# sql-db recipes

The helper (`scripts/db.py`) covers tables/schema/query/exec. For transactions,
bulk loads, or an ORM, use SQLAlchemy directly in `code_exec`. Patterns:

## Explore an unknown database

```sh
python scripts/db.py '{"url":"sqlite:///app.db","op":"tables"}'
python scripts/db.py '{"url":"sqlite:///app.db","op":"schema","table":"orders"}'
```

## Parameterised read (always bind values)

```sh
python scripts/db.py '{"url":"postgresql://u:p@localhost:5432/db","op":"query",
  "sql":"select * from orders where status = :s and total > :min","params":{"s":"paid","min":100}}'
```
Never string-format input into SQL — bind it with `:name` + `params`. This is the
one hard rule.

## Write / DDL

```sh
python scripts/db.py '{"url":"sqlite:///app.db","op":"exec",
  "sql":"create table notes (id integer primary key, body text)"}'
python scripts/db.py '{"url":"sqlite:///app.db","op":"exec",
  "sql":"insert into notes (body) values (:b)","params":{"b":"hello"}}'
```

## Spin up a database, then query it (with docker-services)

```sh
# 1) start Postgres (docker-services skill)
scripts/../../dockerservices/scripts/svc.sh up pg postgres:16 -e POSTGRES_PASSWORD=secret -p 5432:5432
until docker exec agezt-pg pg_isready -U postgres; do sleep 1; done
# 2) query it
python scripts/db.py '{"url":"postgresql://postgres:secret@localhost:5432/postgres","op":"tables"}'
```

## Analyze query results (with data-analysis)

Load straight into pandas — skip the JSON round-trip:
```python
import pandas as pd
from sqlalchemy import create_engine
eng = create_engine("postgresql://postgres:secret@localhost:5432/postgres")
df = pd.read_sql("select category, sum(amount) amt from sales group by 1", eng)
print(df.to_string())
```

## Export a query to a file / artifact

```python
import csv
from sqlalchemy import create_engine, text
eng = create_engine("sqlite:///app.db")
with eng.connect() as c, open("out.csv", "w", newline="") as f:
    res = c.execute(text("select * from users"))
    w = csv.writer(f); w.writerow(res.keys()); w.writerows(res)
```
Then register `out.csv` with the `artifacts` tool so it shows in Files.

## Tips
- Connection strings: `sqlite:///file.db`, `postgresql://u:p@host:5432/db`,
  `mysql+pymysql://u:p@host:3306/db`.
- `query` returns rows (capped by `limit`, default 200; `truncated` flags a cap);
  `exec` returns `rowcount`. Use the one that matches your intent.
