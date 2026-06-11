#!/usr/bin/env python3
"""pdf-tools helper — extract text/tables, read info, merge/split PDFs.

Usage:  python pdf.py '<json-spec>'   (or pipe the JSON on stdin)
Ops:
  info   {path}                          -> {pages, metadata}
  text   {path, pages?, max_chars?}      -> {pages, text, chars}
  tables {path, pages?}                  -> {tables:[{page, rows:[[...]]}]}
  merge  {inputs:[...], out}             -> {out, pages}
  split  {path, pages, out}              -> {out, pages}

`pages` is a 1-based selection: "1-3", "2", or "1,4,6"; omit for all.
A fast start, not a cage: for form-fill/redaction/render, use pypdf/pdfplumber
directly in code_exec.
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


def parse_pages(expr, total):
    """1-based 'pages' expr -> sorted 0-based indices within [0, total)."""
    if not expr:
        return list(range(total))
    idx = set()
    for part in str(expr).split(","):
        part = part.strip()
        if not part:
            continue
        if "-" in part:
            a, b = part.split("-", 1)
            for n in range(int(a), int(b) + 1):
                idx.add(n - 1)
        else:
            idx.add(int(part) - 1)
    return sorted(i for i in idx if 0 <= i < total)


def op_info(spec):
    from pypdf import PdfReader

    r = PdfReader(spec["path"])
    meta = {k.lstrip("/"): str(v) for k, v in (r.metadata or {}).items()}
    return {"pages": len(r.pages), "metadata": meta}


def op_text(spec):
    import pdfplumber

    out = []
    with pdfplumber.open(spec["path"]) as pdf:
        total = len(pdf.pages)
        for i in parse_pages(spec.get("pages"), total):
            out.append(pdf.pages[i].extract_text() or "")
    text = "\n\n".join(out).strip()
    mc = spec.get("max_chars")
    if mc and len(text) > int(mc):
        text = text[: int(mc)].rstrip() + " …"
    return {"pages": total, "text": text, "chars": len(text)}


def op_tables(spec):
    import pdfplumber

    tables = []
    with pdfplumber.open(spec["path"]) as pdf:
        total = len(pdf.pages)
        for i in parse_pages(spec.get("pages"), total):
            for t in pdf.pages[i].extract_tables() or []:
                tables.append({"page": i + 1, "rows": t})
    return {"tables": tables, "count": len(tables)}


def op_merge(spec):
    from pypdf import PdfWriter

    inputs = spec.get("inputs") or []
    if not inputs:
        raise ValueError("merge needs inputs[]")
    out = spec.get("out", "merged.pdf")
    w = PdfWriter()
    for p in inputs:
        w.append(p)
    with open(out, "wb") as fh:
        w.write(fh)
    return {"out": out, "pages": len(w.pages)}


def op_split(spec):
    from pypdf import PdfReader, PdfWriter

    r = PdfReader(spec["path"])
    out = spec.get("out", "slice.pdf")
    idx = parse_pages(spec.get("pages"), len(r.pages))
    if not idx:
        raise ValueError("split needs a non-empty 'pages' range")
    w = PdfWriter()
    for i in idx:
        w.add_page(r.pages[i])
    with open(out, "wb") as fh:
        w.write(fh)
    return {"out": out, "pages": len(idx)}


OPS = {"info": op_info, "text": op_text, "tables": op_tables, "merge": op_merge, "split": op_split}


def run(spec):
    op = spec.get("op")
    if op not in OPS:
        raise ValueError("spec.op must be one of: " + ", ".join(OPS))
    result = OPS[op](spec)
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
