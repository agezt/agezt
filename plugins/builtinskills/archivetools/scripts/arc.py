#!/usr/bin/env python3
"""archive-tools helper — pack/unpack zip and tar(.gz). Standard library only.

Usage:  python arc.py '<json-spec>'   (or pipe the JSON on stdin)
Ops:
  list    {path}                       -> {entries:[...], count}
  extract {path, dest}                 -> {dest, files}   (path-traversal-guarded)
  zip     {inputs:[...], out}          -> {out, files}
  tar     {inputs:[...], out}          -> {out, files}    (.tar.gz if out ends .gz/.tgz)

Format is inferred from the extension (.zip, .tar, .tar.gz/.tgz). A fast start,
not a cage: for password-protected or streamed archives, use zipfile/tarfile.
"""
import json
import os
import sys
import tarfile
import zipfile


def read_spec():
    if len(sys.argv) > 1 and sys.argv[1].strip():
        return json.loads(sys.argv[1])
    data = sys.stdin.read()
    if data.strip():
        return json.loads(data)
    raise ValueError("no spec: pass a JSON spec as argv[1] or on stdin")


def is_tar(path):
    low = path.lower()
    return low.endswith((".tar", ".tar.gz", ".tgz"))


def _within(dest, target):
    """True if `target` resolves to a path inside `dest` (zip-slip guard)."""
    dest_abs = os.path.realpath(dest)
    target_abs = os.path.realpath(target)
    return target_abs == dest_abs or target_abs.startswith(dest_abs + os.sep)


def op_list(spec):
    path = spec["path"]
    if is_tar(path):
        with tarfile.open(path, "r:*") as t:
            names = [m.name for m in t.getmembers() if m.isfile() or m.isdir()]
    else:
        with zipfile.ZipFile(path) as z:
            names = z.namelist()
    return {"entries": names, "count": len(names)}


def op_extract(spec):
    path = spec["path"]
    dest = spec.get("dest", "extracted")
    os.makedirs(dest, exist_ok=True)
    count = 0
    if is_tar(path):
        with tarfile.open(path, "r:*") as t:
            for m in t.getmembers():
                if not _within(dest, os.path.join(dest, m.name)):
                    raise ValueError(f"unsafe path in archive (zip slip): {m.name}")
            t.extractall(dest)
            count = sum(1 for m in t.getmembers() if m.isfile())
    else:
        with zipfile.ZipFile(path) as z:
            for name in z.namelist():
                if not _within(dest, os.path.join(dest, name)):
                    raise ValueError(f"unsafe path in archive (zip slip): {name}")
            z.extractall(dest)
            count = sum(1 for n in z.namelist() if not n.endswith("/"))
    return {"dest": dest, "files": count}


def _walk_inputs(inputs):
    """Yield (abspath, arcname) for each file under the given files/dirs.

    Archive names keep the input folder itself (so `inputs:["src/"]` stores
    `src/a.txt`, not `a.txt`) and always use forward slashes. Strips both
    separators so a trailing slash never collapses the prefix on any platform.
    """
    for item in inputs:
        item = item.rstrip("/\\")
        if os.path.isdir(item):
            parent = os.path.dirname(item)  # "" when the dir is in the cwd
            for root, _dirs, files in os.walk(item):
                for fn in files:
                    full = os.path.join(root, fn)
                    arc = os.path.relpath(full, parent) if parent else full
                    yield full, arc.replace(os.sep, "/")
        elif os.path.isfile(item):
            yield item, os.path.basename(item)
        else:
            raise ValueError(f"input not found: {item}")


def op_zip(spec):
    inputs = spec.get("inputs") or []
    if not inputs:
        raise ValueError("zip needs inputs[]")
    out = spec.get("out", "archive.zip")
    n = 0
    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
        for full, arc in _walk_inputs(inputs):
            z.write(full, arc)
            n += 1
    return {"out": out, "files": n}


def op_tar(spec):
    inputs = spec.get("inputs") or []
    if not inputs:
        raise ValueError("tar needs inputs[]")
    out = spec.get("out", "archive.tar.gz")
    mode = "w:gz" if out.lower().endswith((".gz", ".tgz")) else "w"
    n = 0
    with tarfile.open(out, mode) as t:
        for full, arc in _walk_inputs(inputs):
            t.add(full, arcname=arc)
            n += 1
    return {"out": out, "files": n}


OPS = {"list": op_list, "extract": op_extract, "zip": op_zip, "tar": op_tar}


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
