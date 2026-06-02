# M171 — Egress-guard SSRF range hardening (security review)

## Why
An adversarial security review was run on `kernel/netguard` — the egress guard that
stops the http/browser tools (driven by a potentially prompt-injected LLM) from
reaching internal/private/metadata addresses, above all the cloud metadata endpoint
`169.254.169.254` (steals IAM/instance credentials). A bypass here = credential
theft.

## Confirmed sound (architecture)
The review confirmed the hard parts are correct and left them unchanged:
- The guard checks the **actually-dialed IP** via `net.Dialer.Control` (runs after
  resolution with the concrete `IP:port`), so **DNS rebinding** is caught at connect
  time — there is no separate pre-resolution hostname check to diverge from.
- **Redirects** are followed through the same guarded transport, so every hop
  re-invokes `Control` (a 302 → `http://169.254.169.254` is blocked at the redirect
  hop). The browser cookie-jar shim reuses the same transport.
- Parsing is **fail-closed**: a non-literal/unparseable dial address errors rather
  than connecting; octal/decimal/hex/short IPv4 forms are normalized by Go's
  resolver to a canonical IP before the guard sees them.
- Opt-in flags are correctly scoped (`AllowPrivate` does **not** unblock
  link-local/metadata).

## The bugs (range completeness in `Allowed`)
`Allowed` enumerated blocked ranges via `net.IP`'s classification methods, several
of which have no matching method:

- **Critical — NAT64-wrapped metadata.** `net.IP.To4()` is non-nil only for the
  IPv4-mapped `::ffff:` form. A NAT64 address `64:ff9b::a9fe:a9fe`
  (= `169.254.169.254`) has `To4()==nil` and matches none of
  `IsLoopback/IsLinkLocalUnicast/IsPrivate/IsUnspecified`, so it fell through to
  `return true` (allowed). On any host with a NAT64 gateway — increasingly the
  default on IPv6-only cloud subnets — `64:ff9b::/96` is routed and translated to
  the v4 metadata service. A prompt-injected model only had to emit
  `http://[64:ff9b::a9fe:a9fe]/latest/meta-data/…`.
- **High — IPv4-compatible `::a.b.c.d`** (`::a9fe:a9fe`): same fall-through.
- **High — CGNAT `100.64.0.0/10`**: `IsPrivate` doesn't cover it → allowed.
- **Medium — `0.0.0.0/8`**: `IsUnspecified` matches only the exact `0.0.0.0`, so
  `0.0.0.1` (a loopback-pivot alias on Linux) was allowed.
- **Low — multicast `224.0.0.0/4` / broadcast `255.255.255.255`**: not checked.

## Fix
`Allowed` now, before classifying, **collapses IPv6 forms that embed a routable
IPv4** to that IPv4 via a new `embeddedV4` helper — NAT64 `64:ff9b::/96` and
IPv4-compatible `::/96` (excluding `::`/`::1`, which keep their own reasons;
IPv4-mapped `::ffff:` is already classified directly by `net.IP`'s `To4`-based
methods). So a blocked v4 can't be smuggled through an IPv6 literal. Added explicit
cases: `isZeroBlock` (`0.0.0.0/8`), `isCGNAT` (`100.64.0.0/10`, gated by
`allowPrivate`), and `IsMulticast()||isV4Broadcast()`.

## Tests (+1, all passing)
`TestAllowed_SSRFBypassVectors` — every new vector blocks by default:
`64:ff9b::a9fe:a9fe`, `::a9fe:a9fe`, NAT64-wrapped loopback/private,
`100.64.0.1`/`100.127.255.255` (CGNAT), `0.0.0.1`, `255.255.255.255`, `239.1.2.3`;
and `100.63.255.255` (just below CGNAT), `101.0.0.1`, a public v6 stay **allowed**
(no over-block). The existing default-block, opt-in-scoping, Control-fail-closed,
loopback-e2e, redirect-block, and OnBlock tests still pass.

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command, env var, or event kind.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on both touched files.
- `go test ./... -count=1` — **FAIL 0**, **1549 tests** (was 1548; +1), 61 packages.

## Result
The egress guard's most catastrophic bypass — reaching cloud metadata credentials
via a NAT64/IPv4-compatible IPv6 literal — is closed, along with CGNAT, `0.0.0.0/8`,
and multicast/broadcast gaps. No embedded-IPv4 representation can now smuggle a
blocked address past the guard.
