#!/usr/bin/env python3
"""web-research helper — fetch one or more URLs and extract title + main text.

Usage:  python extract.py '<json-spec>'   (or pipe the JSON on stdin)
Spec:   { "urls": "https://..." | ["https://...", ...],
          "max_chars": 6000, "timeout": 20 }
Output: one JSON object on stdout:
        { "ok": true, "results": [ {url,status,title,text,chars} ],
          "errors": [ {url,error} ] }

Uses trafilatura for clean main-text extraction when installed, else falls back
to BeautifulSoup (drop script/style/nav, collapse whitespace). A fast start, not
a cage: for JS-heavy or gated pages, use the browser-use skill instead.
"""
import json
import re
import sys

UA = "Mozilla/5.0 (compatible; agezt-web-research/1.0; +https://github.com/agezt/agezt)"


def read_spec():
    if len(sys.argv) > 1 and sys.argv[1].strip():
        return json.loads(sys.argv[1])
    data = sys.stdin.read()
    if data.strip():
        return json.loads(data)
    raise ValueError("no spec: pass a JSON spec as argv[1] or on stdin")


def soup_extract(html):
    from bs4 import BeautifulSoup

    soup = BeautifulSoup(html, "html.parser")
    title = soup.title.get_text(strip=True) if soup.title else ""
    for tag in soup(["script", "style", "noscript", "nav", "header", "footer", "form"]):
        tag.decompose()
    text = soup.get_text(separator="\n")
    text = re.sub(r"\n\s*\n+", "\n\n", text)
    text = re.sub(r"[ \t]+", " ", text).strip()
    return title, text


def extract(url, html):
    try:
        import trafilatura

        text = trafilatura.extract(html, include_comments=False, include_tables=True)
        if text:
            md = trafilatura.extract_metadata(html)
            title = (md.title if md and md.title else "") or ""
            if not title:
                title, _ = soup_extract(html)
            return title, text
    except Exception:  # noqa: BLE001 — trafilatura missing or failed; fall back
        pass
    return soup_extract(html)


def fetch_one(url, timeout, max_chars):
    import requests

    resp = requests.get(url, headers={"User-Agent": UA}, timeout=timeout)
    title, text = extract(url, resp.text)
    if max_chars and len(text) > max_chars:
        text = text[:max_chars].rstrip() + " …"
    return {
        "url": url,
        "status": resp.status_code,
        "title": title,
        "text": text,
        "chars": len(text),
    }


def run(spec):
    urls = spec.get("urls")
    if not urls:
        raise ValueError("spec.urls is required")
    if isinstance(urls, str):
        urls = [urls]
    timeout = float(spec.get("timeout", 20))
    max_chars = int(spec.get("max_chars", 6000))

    results, errors = [], []
    for url in urls:
        try:
            results.append(fetch_one(url, timeout, max_chars))
        except Exception as e:  # noqa: BLE001 — one bad URL must not sink the batch
            errors.append({"url": url, "error": str(e)})
    return {"ok": True, "results": results, "errors": errors}


def main():
    try:
        print(json.dumps(run(read_spec()), default=str))
    except Exception as e:  # noqa: BLE001
        print(json.dumps({"ok": False, "error": str(e)}))
        sys.exit(1)


if __name__ == "__main__":
    main()
