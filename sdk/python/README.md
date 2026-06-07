# Agezt Python SDK

The official Python client for the [Agezt](https://github.com/agezt/agezt)
agentic OS. It talks to a running Agezt daemon's native REST API (`/api/v1`) —
the same governed kernel loop `agt run` uses (Edict policy, the journal, cost
governance) — over plain HTTP with a bearer token.

**Standard library only — no runtime dependencies.**

## Install

```bash
pip install agezt          # from PyPI (when published)
# or, from this repo:
pip install ./sdk/python
```

The client itself is pure stdlib, so you can also just vendor the `agezt/`
package directly.

## Quick start

Start a daemon with the REST API enabled and note the token it prints:

```bash
AGEZT_REST_ADDR=127.0.0.1:8800 agezt
#   rest api : http://127.0.0.1:8800/api/v1  (Authorization: Bearer <token>)
```

```python
from agezt import Client

c = Client("http://127.0.0.1:8800", token="<token>")

print(c.health())          # {'status': 'ok', 'version': '1.0.0', ...}
print(c.models()["default"])

# Blocking run — returns the final answer.
result = c.run("summarise the latest commits")
print(result.answer)

# Streaming run — tokens as the agent produces them.
for ev in c.run_stream("write a haiku about Go"):
    if ev.event == "token":
        print(ev.data.get("text", ""), end="", flush=True)

# The journaled event arc of a past run.
arc = c.get_run(result.correlation_id)
print(arc["count"], "events")
```

## API

| Method | REST endpoint | Returns |
|---|---|---|
| `health()` | `GET /api/v1/health` | `dict` (status, version, default_model, model_count) |
| `models()` | `GET /api/v1/models` | `dict` (`default`, `models`) |
| `run(intent, model=None)` | `POST /api/v1/runs` | `RunResult` (correlation_id, model, status, answer) |
| `run_stream(intent, model=None)` | `POST /api/v1/runs` (SSE) | iterator of `StreamEvent` (`start`/`token`/`done`/`error`) |
| `get_run(correlation_id)` | `GET /api/v1/runs/{id}` | `dict` (correlation_id, count, events) |

`Client(base_url, token, timeout=30, tenant=None)` — pass `tenant` to target an
isolated tenant (sent as the `X-Agezt-Tenant` header) on a multi-tenant daemon.

Non-2xx responses raise `agezt.APIError` (`.status`, `.type`, `.message`).

## Tests

Standard-library `unittest`, no third-party deps:

```bash
cd sdk/python
python -m unittest discover -s tests
```
