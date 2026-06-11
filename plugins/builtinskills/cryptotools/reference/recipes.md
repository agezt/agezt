# crypto-tools recipes

The helper (`scripts/crypto.py`) covers hash/verify/hmac/base64/token with stdlib
only. For encryption, key derivation, or JWT, use `cryptography`/`PyJWT` directly.

## Verify a downloaded file's checksum

```sh
python scripts/crypto.py '{"op":"verify","path":"app.tar.gz","algo":"sha256",
  "expected":"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"}'
```
Returns `{"match": true|false}` (constant-time compare).

## Verify a webhook HMAC signature

Many APIs sign the request body with a shared secret (e.g. GitHub's
`X-Hub-Signature-256`):
```sh
export SECRET=...   # the shared webhook secret
python scripts/crypto.py '{"op":"hmac_verify","text":"<raw-body>","key":"'"$SECRET"'",
  "algo":"sha256","expected":"<sig-from-header>"}'
```

## Sign an outgoing request

```sh
python scripts/crypto.py '{"op":"hmac","text":"<body>","key":"'"$SECRET"'","algo":"sha256"}'
# → put the digest in your signature header via http-api-client
```

## base64 for an API payload

```sh
python scripts/crypto.py '{"op":"base64","mode":"encode","text":"user:pass"}'   # Basic auth value
python scripts/crypto.py '{"op":"base64","mode":"decode","text":"dXNlcjpwYXNz"}'
```
Use `"urlsafe": true` for URL-safe base64 (JWT segments, tokens in URLs).

## Mint a secure token / API key

```sh
python scripts/crypto.py '{"op":"token","bytes":32,"format":"urlsafe"}'   # session/api key
python scripts/crypto.py '{"op":"token","bytes":16,"format":"hex"}'       # idempotency key
```
Uses `secrets` (CSPRNG) — safe for credentials, unlike `random`.

## Symmetric file encryption (helper doesn't cover it)

```python
# pip install cryptography
from cryptography.fernet import Fernet
key = Fernet.generate_key()           # store this safely; losing it = losing the data
f = Fernet(key)
token = f.encrypt(open("secret.txt","rb").read())
open("secret.enc","wb").write(token)
# decrypt: f.decrypt(open("secret.enc","rb").read())
```

## Which algorithm?
- **Checksums / dedup:** sha256 (md5/sha1 OK for non-security integrity only).
- **Signatures / HMAC / anything trust-bearing:** sha256 or stronger — never md5/sha1.
- **Random secrets:** always `token` (CSPRNG), never `random`.
