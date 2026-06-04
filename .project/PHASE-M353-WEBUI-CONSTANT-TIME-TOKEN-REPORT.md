# M353 — Constant-time web UI token comparison (timing side-channel fix)

## Why
Priority-A security fix, found by a constant-time-comparison audit (grep for
secret/token `==`/`!=` comparisons not going through `crypto/subtle` /
`hmac.Equal`). The web UI's `authorized` check compared the presented auth token
to the configured one with a plain string `==`:

```go
if tok := r.URL.Query().Get("token"); tok == s.token { return true }
...
return strings.TrimPrefix(h, "Bearer ") == s.token
```

`==` on strings short-circuits at the first differing byte, so its run time leaks
how many leading bytes matched — a classic timing side-channel an attacker who can
reach the web UI (same host, or via SSRF/an XSS'd browser) could use to recover the
token one byte at a time. The control-plane already gates its token with
`subtle.ConstantTimeCompare` (server.go); the web UI was the outlier.

## What
Production security fix + lock-in test.
- **`kernel/webui/webui.go`** — both token comparisons now route through a new
  `(*Server).tokenMatch(presented)` helper that uses
  `subtle.ConstantTimeCompare([]byte(presented), []byte(s.token)) == 1`. Behaviour
  (accept exact, reject everything else, for both the `?token=` query param and the
  `Authorization: Bearer` header) is unchanged; only the comparison is now
  constant-time. Added `crypto/subtle` import.
- **`kernel/webui/webui_test.go`** — `TestTokenMatch_ConstantTimeAcceptReject`:
  exact token accepts; empty, a one-byte-short prefix (the shape a timing attack
  probes), an over-long value, a case variant, and unrelated values all reject.

## Verification
- `go test ./kernel/webui -run 'AuthRequired|EmptyTokenNeverAuthorizes|WriteRequiresToken|TokenMatch_ConstantTime' -v`
  — all pass: the pre-existing auth tests (no/wrong token, bearer, wrong bearer,
  empty-token-never-authorizes, write-requires-token) confirm behaviour is
  preserved, and the new test pins the helper.
- `gofmt -l` clean; `go vet ./kernel/webui/` clean; `GOOS=linux go build ./...`
  exit 0. Full suite **2085** passing (was 2084; +1), `go test ./...` exit 0.
  `go.mod` / `go.sum` unchanged. CHANGELOG updated (Security).

## Scope notes
- The web UI binds to loopback by default and refuses to serve without a token, so
  real-world exposure was limited — but a loopback-reachable or SSRF-capable
  attacker made this exploitable, and constant-time auth comparison is the standard
  practice the rest of the daemon already follows.
- Audit result: this was the **only** non-constant-time secret comparison among the
  daemon's auth/signature paths. The channel inbound signature checks already use
  `hmac.Equal` / `ed25519.Verify`, the control-plane and tenant registry use
  `subtle.ConstantTimeCompare`, and the remaining `== ""` hits are presence checks
  (not value comparisons).
