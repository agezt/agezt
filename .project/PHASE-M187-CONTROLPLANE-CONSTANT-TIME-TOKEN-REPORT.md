# M187 — Constant-time primary-token comparison (control plane)

## Why
The control plane is the daemon's network surface (localhost TCP, token-authed). Every
request carries a token; the **primary (admin) token** is the most privileged
credential — it authorizes every command on every tenant. The auth gate compared it
with a plain Go string comparison:

```go
if req.Token != s.Token() {   // server.go (auth gate)
...
if req.Token == s.Token() {   // server.go handleWhoami
```

Go's `==`/`!=` on strings returns as soon as it finds the first differing byte. The
time taken therefore depends on how many leading bytes match, which leaks the token to
an attacker who can measure response latency — the classic timing side-channel that
`crypto/subtle.ConstantTimeCompare` exists to defeat. Notably the **tenant** auth path
was *already* hardened (`tenant.Registry.Authorize` uses `ConstantTimeCompare`), so the
less-privileged credential was protected while the more-privileged one was not — an
inconsistency worth closing.

## What
- Added `Server.tokenIsPrimary(presented string) bool` — reads the primary token and
  compares with `subtle.ConstantTimeCompare(...) == 1`. It also short-circuits a blank
  presented or unset server token to `false` (defense in depth, mirroring the tenant
  path): `ConstantTimeCompare("", "")` returns 1, which would otherwise let an empty
  token match an as-yet-unset server token. Emptiness is not token-content, so the
  short-circuit leaks nothing secret.
- Both comparison sites now route through it: the auth gate
  (`if !s.tokenIsPrimary(req.Token)`) and `handleWhoami`
  (`if s.tokenIsPrimary(req.Token)`).

Behavior is otherwise identical — the exact token still authorizes, every other token
is still rejected — only the comparison is now timing-safe.

## Tests
`kernel/controlplane/auth_test.go` (white-box):
- `TestTokenIsPrimary_OnlyExactMatches` — the exact token matches; a prefix
  (one byte short), a one-byte-longer string, a last-byte-differs variant (which a
  plain `==` would reject only after scanning the whole prefix), an empty string, and
  an unrelated string are all rejected.
- `TestTokenIsPrimary_EmptyServerTokenRejectsEmpty` — an empty presented token does not
  authorize against an unset server token (the blank-token guard).

Timing itself isn't unit-testable, but `crypto/subtle` provides the constant-time
guarantee; these tests lock in the correctness it must preserve. Existing
`controlplane_test`/`tenant_auth_test` continue to pass (no behavioral regression).

## Verification
- `go test ./...` — 1589 passing, 0 failing.
- `go vet ./kernel/controlplane/` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged (`crypto/subtle` is stdlib).
- Local commit only (no push); standard trailer.

## Files
- `kernel/controlplane/server.go` — `crypto/subtle` import, `tokenIsPrimary` helper,
  both comparison sites routed through it.
- `kernel/controlplane/auth_test.go` — new.

## Follow-up (same surface, queued)
The control plane's per-request read (`handleConn`: `reader.ReadBytes('\n')`,
server.go) is unbounded — a client that sends bytes without a newline within the 10-min
read deadline can grow the buffer without limit (the plugin-host/mcpbridge OOM class,
M177/M185, on the localhost control socket). A bounded request read is the natural next
control-plane hardening milestone.
