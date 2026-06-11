# PHASE M894 — Built-in crypto-tools skill bundle

**Status:** shipped
**Milestone:** M894 (session range M889–M899; branched from `origin/main`,
concurrent local-main arc untouched).
**Theme:** Backlog **#34** — a fourteenth built-in skill bundle: cryptographic
primitives (hash/verify, HMAC, base64, secure tokens), stdlib-only.

## What shipped

A built-in `crypto-tools` bundle, seeded active at startup by the existing
`plugins/builtinskills` seeder (`builtinBundles` + `go:embed` only). **Zero pip
deps** — stdlib `hashlib`/`hmac`/`base64`/`secrets`, so there's no `setup.sh`:

- `SKILL.md` — the ops, the constant-time-compare note, key-via-env guidance, the
  md5/sha1-for-checksums-not-security caveat, and the explicit scope line (this is
  hashing/encoding/signing, not file encryption).
- `scripts/crypto.py` — one JSON-spec helper, six ops: `hash` (file or text, any
  `hashlib` algo), `verify` (constant-time digest check), `hmac` / `hmac_verify`
  (sign / constant-time-verify, for webhook signatures), `base64`
  (encode/decode, std or urlsafe), `token` (CSPRNG hex/urlsafe secrets). Keys are
  never echoed back.
- `reference/recipes.md` — verify a download, verify/sign a webhook HMAC, base64
  for an API, mint a token, the (out-of-scope) Fernet encryption pointer, and an
  algorithm-choice guide.

## Why this milestone is conflict-free

A new bundle touches **only** `plugins/builtinskills/`. The seeder auto-loads it.
It tests in isolation: `go test ./plugins/builtinskills/`. Branched from
`origin/main` (my M862–M893), leaving the concurrent session's local-main arc
untouched.

## Verification

- **Isolated gate:** `go vet` clean; linux cross-build of `./plugins/builtinskills/`
  clean; `gofmt -l` empty. Package suite green — `TestSeedAll_InstallsCryptoTools`
  asserts the bundle seeds **active** and materializes `crypto.py` / `recipes.md`;
  bundle-count assertions now cover fourteen bundles.
- **Functional smoke (stdlib, ran locally):** `sha256("hello")` →
  `2cf24dba…938b9824` (matches the known digest); `base64("user:pass")` →
  `dXNlcjpwYXNz`; an HMAC sign→verify round-trip returns `match:true`.
- No new Go dep; no new env. `.gitattributes` already forces LF on
  `plugins/builtinskills/**`. Full `go build ./...` deliberately skipped.

## Notes
- Fourteen seeded bundles now ship. crypto-tools pairs with http-api-client
  (sign/verify webhooks), archive-tools (checksum a bundle), and ssh-remote
  (verify a transferred file). Verify ops use `hmac.compare_digest` so signature
  checks aren't timing-leaky.
