# M543 — Mutation testing browser: pin the one-level wildcard SSRF deny

## Context
`plugins/tools/browser` fetches arbitrary web pages on the agent's behalf, so its
host allowlist is an SSRF boundary (re-checked on every redirect hop, M254). Its
wildcard match is **stricter** than the sibling http tool's: `*.example.com`
matches exactly one label deep, enforced by a dot-count guard. `GOMAXPROCS=3`.

## The genuine gap (closed)
```go
if strings.HasPrefix(a, "*.") {
	suffix := a[1:] // ".example.com"
	if strings.HasSuffix(host, suffix) && strings.Count(host, ".") == strings.Count(a, ".") {
		return true
	}
}
```

`TestInvoke_WildcardHostMatch` covered the bare apex (denied) and a one-level
subdomain (allowed), but **not** a multi-level subdomain. So the dot-count guard
— the only thing distinguishing this from http's "any depth" wildcard — was
untested: mutating `Count(host,".") == Count(a,".")` to a constant `true` left
every test green while `a.b.example.com` would now match `*.example.com`. That
silently widens the operator's one-level allowlist to arbitrary depth — an SSRF
allowlist bypass for any attacker who can stand up (or redirect to) a deeper
subdomain under the allowed apex.

Note this guard is genuinely browser-specific: http's wildcard
(`HasSuffix(host, suffix) && host != suffix[1:]`, M536) excludes only the apex and
*intentionally* allows any depth, so http's behavior is fully tested and correct
as-is. Only browser restricts to one level, and only browser left it unpinned.

## Fix
Extended `TestInvoke_WildcardHostMatch`: `a.b.example.com` against `*.example.com`
must be denied ("host not in allowlist"), alongside the existing apex-denied and
one-level-allowed assertions.

## Negative control (manual, CPU-capped)
`Count(host,".") == Count(a,".") → true` (guard removed): FAIL — `a.b.example.com`
is admitted and dialed instead of denied. Restored byte-for-byte
(`git diff --ignore-all-space` on browser.go empty); passes again.

## Honestly assessed, left unpinned (no padding)
The two truncation caps in the same file — `len(rawBody) > MaxFetchBytes` and
`len(text) > maxChars` — are the same inclusive-max idiom but purely cosmetic
(a one-byte "…[truncated]" marker on output that exactly fills the cap); the
rune-boundary test already exercises the truncation path with text far over the
cap. The redirect chain cap (`len(via) >= maxRedirects`) and per-hop allowlist
re-check are covered by `redirect_test.go` (off-allowlist redirect blocked,
same-host followed). The SSRF deny was the one security-meaningful survivor.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.
