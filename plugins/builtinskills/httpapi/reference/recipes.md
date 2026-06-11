# http-api-client recipes

The helper (`scripts/api.py`) covers single requests with auth, params, and JSON/
form bodies. For OAuth, multipart, streaming, or pagination, use `requests`
directly in `code_exec`. Patterns:

## GET with a token

```sh
export TOKEN=...   # set the secret in the shell first
python scripts/api.py '{"method":"GET","url":"https://api.example.com/me",
  "auth":{"type":"bearer","token":"'"$TOKEN"'"}}'
```

## POST JSON, read the created record

```sh
python scripts/api.py '{"method":"POST","url":"https://api.example.com/items",
  "json":{"name":"widget","qty":3}}'
```
A 4xx/5xx still returns `{ok:false,status,json|text}` — read the error body
instead of guessing.

## Paginate a cursor/offset API (write it directly)

```python
import requests
out, url = [], "https://api.example.com/items?limit=100"
h = {"Authorization": "Bearer " + TOKEN}
while url:
    r = requests.get(url, headers=h, timeout=30); r.raise_for_status()
    body = r.json(); out += body["data"]
    url = body.get("next")     # or build the next offset
print(len(out), "rows")
```

## Retry with backoff

```python
import requests, time
for attempt in range(5):
    r = requests.get(url, timeout=30)
    if r.status_code < 500:
        break
    time.sleep(2 ** attempt)   # 1,2,4,8,16s
```

## Multipart upload

```python
import requests
with open("photo.jpg", "rb") as f:
    r = requests.post(url, files={"file": f}, headers={"Authorization": "Bearer "+TOKEN})
```

## The integration pipeline

http-api-client composes with the data bundles:
- Page an API → rows → hand to **data-analysis** (`pd.DataFrame(rows)`).
- Persist results to a DB with **sql-db**.
- POST a generated **pdf-tools**/**image-tools** artifact to an upload endpoint.

## Secrets
Reference an env var (`export TOKEN=...`) and expand it in the shell before the
call rather than pasting long-lived secrets into chat. The helper never echoes
the request's `Authorization` header back in its output.
