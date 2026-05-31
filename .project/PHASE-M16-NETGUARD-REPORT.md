# Phase Report — Milestone M16 (Network egress guard: no SSRF, no metadata theft)

> Status: **Phases 1–3 shipped** · Date: 2026-05-31
> SPEC-06 security defaults. Phase 1: the egress-guard substrate
> (`kernel/netguard`) — a dialer-level block on internal/metadata addresses that
> defeats DNS rebinding and redirect-based SSRF. Phase 2: the HTTP tool is guarded
> by default. Phase 3: `browser.read` is guarded by default too. Both outbound
> URL-fetching tools now refuse internal addresses even with the host check
> bypassed.

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

## Phase 2 — the HTTP tool is guarded by default

`http.Tool` now builds its request client from a `netguard` guard whenever no
client is injected — which is the production path (the daemon never injects). The
guard is a **second, IP-level layer beneath the hostname allowlist**:

- **Default-deny internal.** Even an allowlisted host — or `AGEZT_HTTP_ALLOW_ALL=1`,
  which only ever loosened the *hostname* check — can no longer reach loopback,
  the metadata endpoint, or RFC1918. The previously-dangerous `AllowAll` is now
  safe against the worst case (AllowAll + `http://169.254.169.254/…`).
- **Per-range opt-in.** `AllowLoopback` / `AllowPrivate` fields (env
  `AGEZT_HTTP_ALLOW_LOOPBACK=1` / `AGEZT_HTTP_ALLOW_PRIVATE=1`) relax exactly one
  range for a local sidecar or LAN service; the private opt-in logs a warning.
  Neither unblocks the metadata endpoint. The tool banner reports the egress
  posture (`http(hosts=N, egress=guarded)`).
- **Injection still bypasses** (an explicit caller choice — used by tests).

**Proven:** a new tool test (`TestSSRFGuard_BlocksLoopbackEvenWithAllowAll`)
sets `AllowAll=true` and points the tool at a loopback server: the request is
refused with a `netguard` error and the server's body never returns; flipping
`AllowLoopback` then lets it through. The four existing tests that dial loopback
test servers opt in explicitly. Live: the daemon banner shows
`http(allow_all=true, egress=guarded)`.

## Phase 3 — `browser.read` is guarded too

The browser tool fetches arbitrary URLs and follows redirects — the same SSRF
surface as the http tool — so it gets the same treatment. `browser.Tool` builds
its fetch client from a `netguard` guard when none is injected, with
`AllowLoopback`/`AllowPrivate` fields and `AGEZT_BROWSER_ALLOW_LOOPBACK` /
`_PRIVATE` env opt-ins (private logs a warning). The per-Invoke cookie-jar shim
still works — it shallow-copies the guarded client and sets the jar, preserving
the guarded transport.

**Proven:** `TestSSRFGuard_BlocksLoopbackEvenWithAllowAll` — `AllowAll=true`
pointed at a loopback page is refused with a `netguard` error; `AllowLoopback`
lets it through. The two cookie tests that exercise the default client opt into
loopback; every other browser test injects `srv.Client()` (an explicit bypass)
and is unaffected.

## Deferred — later phases (named)

- **Remaining outbound paths** (`peer` tool, MCP bridge, webhook sinks) routed
  through the same guard — each builds its own client today; the two
  agent-driven URL fetchers (http, browser) — the highest-risk surface — are done.
- **Per-call egress as an Edict capability** so the trust ladder governs which
  hosts/ranges a given run may reach (not just a global tool flag).
