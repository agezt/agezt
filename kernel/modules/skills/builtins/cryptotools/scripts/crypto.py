#!/usr/bin/env python3
"""crypto-tools helper — hashes, HMAC, base64, secure tokens. Standard library only.

Usage:  python crypto.py '<json-spec>'   (or pipe the JSON on stdin)
Ops:
  hash        {path|text, algo=sha256}                  -> {algo, digest}
  verify      {path|text, algo, expected}               -> {match}        (constant-time)
  hmac        {text, key, algo=sha256}                  -> {algo, digest}
  hmac_verify {text, key, algo, expected}               -> {match}        (constant-time)
  base64      {mode:"encode|decode", text, urlsafe?}    -> {result}
  token       {bytes=32, format:"hex|urlsafe"}          -> {token}

Verify/hmac_verify use hmac.compare_digest (not ==) so checks aren't timing-leaky.
Keys/secrets passed in are never echoed back.
"""
import base64 as b64
import hashlib
import hmac
import json
import secrets
import sys


def read_spec():
    if len(sys.argv) > 1 and sys.argv[1].strip():
        return json.loads(sys.argv[1])
    data = sys.stdin.read()
    if data.strip():
        return json.loads(data)
    raise ValueError("no spec: pass a JSON spec as argv[1] or on stdin")


def _data_bytes(spec):
    if spec.get("path"):
        with open(spec["path"], "rb") as fh:
            return fh.read()
    if "text" in spec:
        return str(spec["text"]).encode("utf-8")
    raise ValueError("needs path or text")


def _algo(spec):
    name = spec.get("algo", "sha256")
    if name not in hashlib.algorithms_available:
        raise ValueError(f"unknown algo: {name}")
    return name


def op_hash(spec):
    algo = _algo(spec)
    h = hashlib.new(algo)
    h.update(_data_bytes(spec))
    return {"algo": algo, "digest": h.hexdigest()}


def op_verify(spec):
    expected = spec.get("expected")
    if not expected:
        raise ValueError("verify needs expected")
    got = op_hash(spec)["digest"]
    return {"match": hmac.compare_digest(got, str(expected).strip().lower())}


def op_hmac(spec):
    key = spec.get("key")
    if key is None:
        raise ValueError("hmac needs key")
    if "text" not in spec and not spec.get("path"):
        raise ValueError("hmac needs text or path")
    algo = _algo(spec)
    mac = hmac.new(str(key).encode("utf-8"), _data_bytes(spec), algo)
    return {"algo": algo, "digest": mac.hexdigest()}


def op_hmac_verify(spec):
    expected = spec.get("expected")
    if not expected:
        raise ValueError("hmac_verify needs expected")
    got = op_hmac(spec)["digest"]
    return {"match": hmac.compare_digest(got, str(expected).strip().lower())}


def op_base64(spec):
    mode = spec.get("mode", "encode")
    text = spec.get("text", "")
    urlsafe = bool(spec.get("urlsafe", False))
    if mode == "encode":
        raw = str(text).encode("utf-8")
        enc = b64.urlsafe_b64encode(raw) if urlsafe else b64.b64encode(raw)
        return {"result": enc.decode("ascii")}
    if mode == "decode":
        raw = str(text).encode("ascii")
        dec = b64.urlsafe_b64decode(raw) if urlsafe else b64.b64decode(raw)
        return {"result": dec.decode("utf-8", errors="replace")}
    raise ValueError("base64 mode must be encode or decode")


def op_token(spec):
    n = int(spec.get("bytes", 32))
    fmt = spec.get("format", "hex")
    if fmt == "urlsafe":
        return {"token": secrets.token_urlsafe(n)}
    return {"token": secrets.token_hex(n)}


OPS = {
    "hash": op_hash,
    "verify": op_verify,
    "hmac": op_hmac,
    "hmac_verify": op_hmac_verify,
    "base64": op_base64,
    "token": op_token,
}


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
