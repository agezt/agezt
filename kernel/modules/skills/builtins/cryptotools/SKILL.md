---
name: crypto-tools
description: Hash and verify data, compute and check HMAC signatures, encode/decode base64, and generate secure random tokens — when a task needs a checksum, a webhook signature, base64 for an API, or a random secret, using fast standard-library primitives
triggers: [hash, checksum, sha256, md5, hmac, signature, base64, encode, token, secret, verify, digest]
tools: [code_exec, shell]
---

# crypto-tools — hashes, signatures, encoding, tokens

When a task needs cryptographic *primitives* — verify a download's checksum, sign
or check a webhook's HMAC, base64-encode a payload for an API, or mint a secure
random token — use this. It's fast, exact, and uses only the Python **standard
library** (`hashlib`/`hmac`/`base64`/`secrets`) — no install.

> Scope: this is hashing/encoding/signing, **not** file encryption. For symmetric
> encryption (protect a file with a password), write `cryptography.fernet`
> directly in `code_exec`.

## No setup needed

`hashlib`, `hmac`, `base64`, `secrets` ship with Python. Use `skill op=files
crypto-tools` to find the bundle directory.

## The helper

`scripts/crypto.py` takes a JSON spec with an `op` and prints JSON. Ops:

```sh
# Hash a file (verify a download) or a string:
python scripts/crypto.py '{"op":"hash","path":"app.tar.gz","algo":"sha256"}'
python scripts/crypto.py '{"op":"hash","text":"hello","algo":"sha256"}'

# Verify a file against an expected digest:
python scripts/crypto.py '{"op":"verify","path":"app.tar.gz","algo":"sha256","expected":"abc123…"}'

# HMAC sign / verify (webhook signatures):
python scripts/crypto.py '{"op":"hmac","text":"payload","key":"$SECRET","algo":"sha256"}'
python scripts/crypto.py '{"op":"hmac_verify","text":"payload","key":"$SECRET","algo":"sha256","expected":"…"}'

# base64 encode / decode:
python scripts/crypto.py '{"op":"base64","mode":"encode","text":"hello"}'
python scripts/crypto.py '{"op":"base64","mode":"decode","text":"aGVsbG8="}'

# Generate a secure random token:
python scripts/crypto.py '{"op":"token","bytes":32,"format":"hex"}'
```

### Spec fields
- `op` — `hash` | `verify` | `hmac` | `hmac_verify` | `base64` | `token`.
- `path` **or** `text` (hash/verify/hmac). `algo` (default `sha256`; any
  `hashlib` name — sha256/sha1/md5/sha512…).
- `expected` (verify/hmac_verify — compared in constant time).
- `key` (hmac), `mode` (`encode`/`decode`, base64), `urlsafe` (base64, default
  false), `bytes` (token, default 32), `format` (token: `hex`|`urlsafe`).

### Output (JSON on stdout)
```
{ "ok": true, "op": "hash", "algo": "sha256", "digest": "…" }
{ "ok": true, "op": "verify", "match": true }
{ "ok": true, "op": "token", "token": "…" }
```

## Notes

- **Verify/hmac_verify compare in constant time** (`hmac.compare_digest`) — use
  them rather than `==` so signature checks aren't timing-leaky.
- Pass an HMAC `key` / secret via an env var you set first (`export SECRET=…` →
  `$SECRET`); the helper never echoes the key back.
- MD5/SHA1 are fine for **checksums** but not for security — use SHA-256+ for
  signatures and anything trust-bearing.

## Going further

The helper is a fast start, not a cage — for symmetric encryption, key
derivation (PBKDF2/scrypt), JWT, or asymmetric signing, use `cryptography`/`PyJWT`
directly. Pairs with **http-api-client** (sign a request / verify a webhook),
**archive-tools** (checksum a bundle), and **ssh-remote** (verify a file you
transferred). See `reference/recipes.md`.
