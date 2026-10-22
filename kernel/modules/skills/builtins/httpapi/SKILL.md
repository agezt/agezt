---
name: http-api-client
description: Call REST / JSON HTTP APIs — any method (GET/POST/PUT/PATCH/DELETE), with headers, query params, JSON or form bodies, and bearer/basic auth — and get the status, headers, and parsed body back, when a task needs to integrate with a web service or API rather than just read a web page
triggers: [api, rest, http, endpoint, webhook, request, post, put, delete, bearer, oauth, json, integration]
tools: [code_exec, shell, fetch]
---

# http-api-client — talk to REST / JSON APIs

When a task needs to *call* a web service — create a record via its API, POST to
a webhook, page through a JSON endpoint, hit something behind a bearer token —
use this. It's the write-capable complement to the `fetch` tool and the
**web-research** skill, which are for *reading* web pages. Runs through
`code_exec` (python).

## fetch vs. this skill

- **Reading a web page** (article, docs, HTML) → the `fetch` tool / web-research.
- **Calling a JSON/REST API** (auth headers, POST/PUT/DELETE, JSON bodies,
  pagination) → this skill.

## One-time setup

Run `scripts/setup.sh` once (via code_exec or shell): installs `requests`. Use
`skill op=files http-api-client` to find the bundle directory.

## The helper

`scripts/api.py` takes a JSON spec and prints the response as JSON:

```sh
# GET with query params + a bearer token:
python scripts/api.py '{"method":"GET","url":"https://api.example.com/v1/items",
  "params":{"limit":50},"auth":{"type":"bearer","token":"$TOKEN"}}'

# POST a JSON body:
python scripts/api.py '{"method":"POST","url":"https://api.example.com/v1/items",
  "json":{"name":"widget","qty":3},"headers":{"X-Idempotency-Key":"abc"}}'

# Basic auth + form body:
python scripts/api.py '{"method":"PUT","url":"https://api.example.com/login",
  "data":{"user":"a"},"auth":{"type":"basic","user":"me","pass":"pw"}}'
```

### Spec fields
- `method` (default GET), `url` (required).
- `headers` (object), `params` (query object).
- `json` (parsed JSON body) **or** `data` (form/raw body) — not both.
- `auth` — `{"type":"bearer","token":"..."}` or `{"type":"basic","user":"..","pass":".."}`.
- `timeout` (seconds, default 30), `max_chars` (truncate the returned body text,
  default 8000).

### Output (JSON on stdout)
```
{ "ok": true, "status": 200, "elapsed_ms": 142,
  "headers": {...}, "json": {...} }      # or "text": "..." when not JSON
```
`ok` is the HTTP success flag (2xx). A 4xx/5xx still returns a JSON object with
the status and body so you can read the error — it doesn't throw.

## Secrets

Pass tokens via the spec; don't paste long-lived secrets into chat. Prefer
referencing an environment variable you set first (`export TOKEN=...` then use
`$TOKEN` — expand it in the shell before the call). The helper does **not** echo
the request's auth header back in its output.

## Going further

The helper is a fast start, not a cage — for OAuth flows, multipart uploads,
streaming downloads, retries with backoff, or cursor pagination, write `requests`
directly in `code_exec`. Pair with **data-analysis** (page an API into rows, then
`pd.DataFrame`) and **sql-db** (persist results). See `reference/recipes.md`.
