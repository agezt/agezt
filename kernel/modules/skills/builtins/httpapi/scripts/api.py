#!/usr/bin/env python3
"""http-api-client helper — call a REST/JSON API and print the response as JSON.

Usage:  python api.py '<json-spec>'   (or pipe the JSON on stdin)
Spec:   { "method":"GET", "url":"https://...", "headers":{}, "params":{},
          "json":{...} | "data":{...}, "auth":{"type":"bearer","token":"..."} |
          {"type":"basic","user":"..","pass":".."}, "timeout":30, "max_chars":8000 }
Output: { ok, status, elapsed_ms, headers, json|text }

A non-2xx still returns the object (with the error body) rather than throwing.
The request's auth header is never echoed back. A fast start, not a cage: for
OAuth/multipart/streaming/retries, use requests directly.
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


def build_auth(spec):
    import requests.auth

    a = spec.get("auth") or {}
    kind = a.get("type")
    if kind == "basic":
        return requests.auth.HTTPBasicAuth(a.get("user", ""), a.get("pass", ""))
    return None


def build_headers(spec):
    headers = dict(spec.get("headers") or {})
    a = spec.get("auth") or {}
    if a.get("type") == "bearer" and a.get("token"):
        headers["Authorization"] = "Bearer " + str(a["token"])
    return headers


def run(spec):
    import requests

    url = spec.get("url")
    if not url:
        raise ValueError("spec.url is required")
    method = str(spec.get("method", "GET")).upper()
    if "json" in spec and "data" in spec:
        raise ValueError("pass json OR data, not both")

    resp = requests.request(
        method,
        url,
        headers=build_headers(spec),
        params=spec.get("params") or None,
        json=spec.get("json"),
        data=spec.get("data"),
        auth=build_auth(spec),
        timeout=float(spec.get("timeout", 30)),
    )

    out = {
        "ok": resp.ok,
        "status": resp.status_code,
        "elapsed_ms": int(resp.elapsed.total_seconds() * 1000),
        "headers": dict(resp.headers),
    }
    ctype = resp.headers.get("Content-Type", "")
    if "application/json" in ctype:
        try:
            out["json"] = resp.json()
        except ValueError:
            out["text"] = resp.text[: int(spec.get("max_chars", 8000))]
    else:
        text = resp.text
        mc = int(spec.get("max_chars", 8000))
        out["text"] = text[:mc] + (" …" if len(text) > mc else "")
    return out


def main():
    try:
        print(json.dumps(run(read_spec()), default=str))
    except Exception as e:  # noqa: BLE001
        print(json.dumps({"ok": False, "error": str(e)}))
        sys.exit(1)


if __name__ == "__main__":
    main()
