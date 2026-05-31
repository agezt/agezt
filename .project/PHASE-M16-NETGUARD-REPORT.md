# Phase Report — Milestone M16 (Network egress guard: no SSRF, no metadata theft)

> Status: **Phase 1 shipped** · Date: 2026-05-31
> SPEC-06 security defaults. Phase 1: the egress-guard substrate
> (`kernel/netguard`) — a dialer-level block on internal/metadata addresses that
> defeats DNS rebinding and redirect-based SSRF. Phase 2 wires the HTTP tool (and
> other outbound tools) to use it by default.

## Why this milestone

This completes the containment triad: M14 isolated tenants (storage + identity +
cost), M15 kept secrets out of the permanent record (redaction), and M16 stops
the agent's outbound traffic from reaching the host's own internal network.

The threat is concrete and severe. An autonomous agent runs tools that make
outbound HTTP. A prompt-injected instruction ("fetch this URL") — or a compromised
page the agent is asked to read — can point a request at:

- `http://169.254.169.254/latest/meta-data/iam/security-credentials/…` — the
  cloud **metadata endpoint**, which hands out the host's IAM credentials;
- `http://127.0.0.1:<port>/…` — a co-located admin service, the control plane,
  another tenant's loopback surface;
- RFC1918 hosts — anything on the private network the daemon can route to.

The HTTP tool already had a **hostname allowlist** (default-deny), but a hostname
allowlist is not an egress guard:

1. **DNS rebinding** — an allowed host `evil.example.com` can resolve to
   `169.254.169.254`. The name passes; the IP is internal.
2. **Redirects** — `http.Client` follows 30x by default. An allowed first hop can
   `Location:` you straight to `http://169.254.169.254/…`. The allowlist only saw
   the first URL.

Both bypasses share a root cause: the check was on the *URL string*, but the
danger is the *IP actually connected to*.

## What shipped — `kernel/netguard`

A `Guard` that validates the **resolved IP** on every connection attempt, at the
dialer, via `net.Dialer.Control`. The dialer calls `Control(network, address)`
after DNS resolution with the concrete `IP:port` about to be connected — once per
candidate address, on the initial dial **and on every redirect hop**. So the
guard sees past the hostname to the real target, and a redirect to an internal
address is refused at the moment its socket would open.

- **Secure default (DECISIONS F2).** The zero-value guard blocks loopback
  (`127/8`, `::1`), private (RFC1918 + IPv6 ULA), link-local (`169.254/16`
  including cloud metadata, `fe80::/10`), and the unspecified address — using
  `net.IP`'s own classifiers (so IPv4-mapped IPv6 forms like `::ffff:127.0.0.1`
  are caught too). Public addresses pass.
- **Per-range opt-in.** `AllowLoopback`, `AllowPrivate`, `AllowLinkLocal` relax
  exactly one range each for legitimate uses (a local sidecar, a LAN service).
  They are independent: `AllowPrivate` does **not** unblock the metadata endpoint.
- **Drop-in client.** `Guard.HTTPClient(timeout)` returns an `*http.Client` with a
  fresh guarded transport (the guard can't leak into other clients' pools);
  `Guard.Dialer` / `Guard.Control` are exposed for tools that build their own.
- **Fail closed.** If `Control` is ever handed a non-literal address it can't
  classify, it refuses rather than connect.

Stdlib only (`net`, `net/http`, `syscall`, `time`, `fmt`).

## Proven

- **Classification:** a table of internal addresses (loopback, RFC1918, ULA,
  link-local/metadata, unspecified, IPv4-mapped loopback) all blocked with a
  reason; public v4/v6 allowed; the three opt-ins each unblock exactly their
  range and `AllowPrivate` leaves the metadata endpoint blocked.
- **Control hook:** rejects `169.254.169.254:80`, allows `8.8.8.8:443`, fails
  closed on a non-IP address.
- **End-to-end (httptest):** the default client refuses the loopback test server
  with a `netguard` error; with `AllowLoopback` it reaches it (200). **The SSRF
  proof:** a loopback-allowed client following a redirect to
  `http://169.254.169.254/…` is **blocked at the redirect hop** — the exact attack
  a hostname allowlist misses.

5 new tests; suite **1144** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Deferred — Phase 2+ (named)

- **Phase 2 — wire the HTTP tool.** Build the tool's client from a `netguard`
  guard (default-deny internal), keeping the hostname allowlist as a second layer.
  An `AGEZT_HTTP_ALLOW_LOOPBACK` / `_PRIVATE` escape hatch for trusted local use.
- **Other outbound tools** (`browser`, `peer`, MCP bridge, webhook sinks) routed
  through the same guard.
- **Per-call egress as an Edict capability** so the trust ladder governs which
  hosts/ranges a given run may reach.
