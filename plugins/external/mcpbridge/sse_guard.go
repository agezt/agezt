// SPDX-License-Identifier: MIT

package main

// SSRF gate for the MCP SSE endpoint event (VULN mcp-sse-ssrf-pivot).
//
// The MCP HTTP+SSE transport opens two HTTP connections to the same remote
// server:
//
//   - GET <sseURL>   — long-lived text/event-stream
//   - POST <postURL> — one per JSON-RPC request
//
// The `postURL` is NOT supplied by the operator — the remote server tells
// us, via an SSE `event: endpoint\ndata: <postURL>` line. A malicious MCP
// server (or one whose SSE stream is hijacked / MITMed) can announce a
// `postURL` that:
//
//   - points at a different host entirely (cross-origin pivot),
//   - resolves to a metadata/internal IP (169.254.169.254, RFC1918, …),
//   - points at the loopback bridge in front of an admin UI the operator
//     didn't intend to expose to MCP tool calls.
//
// This file gates the announced `postURL` against the *trusted* origin
// (the operator-supplied `sseURL`) and against the network ranges the
// bridge should refuse to talk to by default. It is the SSE equivalent of
// the kernel netguard policy (`kernel/netguard`); kept in this package
// because the bridge is a deliberately kernel-free binary — duplicating
// the small set of IP-classification helpers here is cheaper than
// importing the kernel package.
//
// Two operator opt-ins are supported:
//
//   - MCPBRIDGE_ALLOW_LOOPBACK=1 — permit `postURL` to resolve to 127/8 ::1
//   - MCPBRIDGE_ALLOW_PRIVATE=1  — permit RFC1918 + IPv6 ULA + CGNAT
//
// Both default off. The `getenv` indirection is wrapped through a package
// variable so tests can swap a stub without leaking state between cases.

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// envOrEmpty returns the trimmed value of name, or "" if unset. Kept local
// to this file because the bridge's other env vars are read directly via
// os.Getenv at start time and don't need a helper.
func envOrEmpty(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

// sseEndpointPolicy captures the trusted origin and the network ranges the
// bridge is willing to POST to. It is built ONCE per transport at construct
// time (the SSE URL is operator-supplied and constant for the life of the
// session) and applied to every announced `postURL`.
type sseEndpointPolicy struct {
	// trustedOrigin is the (scheme, host, port) of the SSE URL. The
	// announced `postURL` must match these byte-for-byte — any drift
	// (different host, different port, http vs https) is a pivot attempt.
	trustedScheme string
	trustedHost   string // lower-cased hostname (or IP literal) of sseURL
	trustedPort   string // explicit port string from sseURL.Host (handles ":0")

	// IP-range opt-ins, mirroring kernel/netguard defaults. Both default
	// false (block) — only an operator who *deliberately* runs an MCP
	// server on localhost / behind a private IP should opt in.
	allowLoopback bool
	allowPrivate  bool
}

// buildSSEEndpointPolicy parses sseURL and merges in the env-var opt-ins.
// A malformed sseURL is an error: the constructor that owns this transport
// has already passed url.Parse on it, but the origin tuple is what we gate
// against, so a second parse + explicit error is cheap insurance.
func buildSSEEndpointPolicy(sseURL string) (sseEndpointPolicy, error) {
	u, err := url.Parse(sseURL)
	if err != nil {
		return sseEndpointPolicy{}, fmt.Errorf("sse mcp guard: parse sseURL %q: %w", sseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return sseEndpointPolicy{}, fmt.Errorf("sse mcp guard: sseURL scheme %q (need http or https)", u.Scheme)
	}
	if u.Host == "" {
		return sseEndpointPolicy{}, fmt.Errorf("sse mcp guard: sseURL %q has no host", sseURL)
	}
	host := u.Hostname()
	port := u.Port() // "" when implicit (80/443); comparison tolerates ""
	return sseEndpointPolicy{
		trustedScheme: u.Scheme,
		trustedHost:   strings.ToLower(host),
		trustedPort:   port,
		allowLoopback: envOrEmpty("MCPBRIDGE_ALLOW_LOOPBACK") == "1",
		allowPrivate:  envOrEmpty("MCPBRIDGE_ALLOW_PRIVATE") == "1",
	}, nil
}

// resolveEndpoint validates an announced postURL against the policy and
// returns the canonical postURL the transport should dial. Three checks:
//
//  1. Parse — must be a valid http(s) URL.
//  2. Same-origin — Scheme + Host + Port must match the SSE origin.
//  3. IP-range — the resolved IPs of the postURL host must not land in a
//     blocked range (loopback / RFC1918 / CGNAT / link-local incl. cloud
//     metadata / multicast / unspecified) unless the corresponding
//     opt-in is set.
//
// Step 3 closes the "operator pinned the SSE URL to a public host that
// also resolves to an internal IP via split-horizon DNS" pivot as well
// as the simpler "announced http://127.0.0.1/..." pivot. Resolution
// happens here at the time the endpoint event arrives (cheap, one-shot)
// and is enforced again per-POST via the dialer — see dialerGuard below.
func resolveEndpoint(sseURL, announced string, policy sseEndpointPolicy) (string, error) {
	postURL := strings.TrimSpace(announced)
	if postURL == "" {
		return "", fmt.Errorf("empty endpoint URL")
	}
	raw := postURL
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		// Relative — resolve against sseURL origin. This is the common
		// case for legitimate servers.
		base, err := url.Parse(sseURL)
		if err != nil {
			return "", fmt.Errorf("resolve relative endpoint: parse sseURL: %w", err)
		}
		rel, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("resolve relative endpoint %q: %w", raw, err)
		}
		postURL = base.ResolveReference(rel).String()
	}
	u, err := url.Parse(postURL)
	if err != nil {
		return "", fmt.Errorf("parse announced endpoint %q: %w", postURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("endpoint scheme %q rejected (need http or https)", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("endpoint %q has no host", postURL)
	}

	// Same-origin: scheme + host + port must match the SSE origin.
	if u.Scheme != policy.trustedScheme {
		return "", fmt.Errorf("endpoint scheme %q does not match sseURL scheme %q (cross-origin pivot?)", u.Scheme, policy.trustedScheme)
	}
	if port := u.Port(); port != policy.trustedPort {
		// Tolerate "" vs explicit 80/443 — the trusted side may have omitted
		// the default port, but a non-default port on the endpoint when the
		// SSE side uses the default is a pivot to a different service.
		if !(port == "" && (policy.trustedPort == "80" || policy.trustedPort == "443")) &&
			!(policy.trustedPort == "" && (port == "80" || port == "443")) {
			return "", fmt.Errorf("endpoint port %q does not match sseURL port %q (cross-origin pivot?)", port, policy.trustedPort)
		}
	}
	if host := strings.ToLower(u.Hostname()); host != policy.trustedHost {
		return "", fmt.Errorf("endpoint host %q does not match sseURL host %q (cross-origin pivot?)", host, policy.trustedHost)
	}

	// IP-range gate: resolve the host and refuse blocked ranges. Even when
	// the URL passes the same-origin check, split-horizon DNS can map a
	// public hostname to an internal IP at request time.
	if err := classifyHost(u.Hostname(), policy); err != nil {
		return "", err
	}
	return postURL, nil
}

// classifyHost resolves host (a hostname or IP literal) and rejects any
// resolved address that falls in a range the bridge is not permitted to
// talk to. A single hostname that resolves to one blocked + one allowed
// address is rejected (fail-closed — the attacker controls DNS so the
// safe choice is to require ALL resolved addresses to be allowed).
func classifyHost(host string, policy sseEndpointPolicy) error {
	// IP literal fast path: skip DNS, classify the address directly.
	if ip := net.ParseIP(host); ip != nil {
		if reason := ipPolicyReason(ip, policy); reason != "" {
			return fmt.Errorf("endpoint IP %s blocked: %s (set MCPBRIDGE_ALLOW_LOOPBACK=1 or MCPBRIDGE_ALLOW_PRIVATE=1 to opt in)", host, reason)
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		// DNS failure on the announced host is fatal: we cannot tell whether
		// the resolved IP is safe. Refuse to dial.
		return fmt.Errorf("endpoint host %q DNS lookup failed: %w (refusing to dial)", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("endpoint host %q has no A/AAAA records", host)
	}
	for _, ip := range ips {
		if reason := ipPolicyReason(ip, policy); reason != "" {
			return fmt.Errorf("endpoint host %q resolves to blocked IP %s: %s", host, ip.String(), reason)
		}
	}
	return nil
}

// ipPolicyReason returns a non-empty reason when ip is in a range the
// bridge refuses to dial given the policy, or "" when the address is
// permitted. The list intentionally mirrors the categories
// `kernel/netguard` blocks by default — a hostile MCP server is the same
// threat model as a hostile model-issued HTTP call.
func ipPolicyReason(ip net.IP, policy sseEndpointPolicy) string {
	if ip == nil {
		return "unparseable address"
	}
	// Collapse IPv6 forms that embed an IPv4 address (NAT64 / IPv4-compat)
	// so a v4 metadata IP isn't smuggled past the v4 checks as a v6 literal.
	if v4 := collapseEmbeddedV4(ip); v4 != nil {
		ip = v4
	}
	switch {
	case ip.IsUnspecified():
		return "unspecified address"
	case ip.IsLoopback():
		if policy.allowLoopback {
			return ""
		}
		return "loopback"
	case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
		return "link-local (cloud metadata / autoconf)"
	case ip.IsMulticast():
		return "multicast"
	case ip.IsPrivate() || isCGNAT(ip):
		if policy.allowPrivate {
			return ""
		}
		return "private network (RFC1918 / ULA / CGNAT)"
	}
	return ""
}

// collapseEmbeddedV4 returns the IPv4 address embedded in an IPv6 literal
// for the forms that actually route to it (NAT64 well-known prefix
// 64:ff9b::/96, and IPv4-compatible ::/96), or nil when ip carries no
// such embedding. :: and ::1 are excluded so they keep their own
// (unspecified / loopback) reasons.
func collapseEmbeddedV4(ip net.IP) net.IP {
	if ip.To4() != nil {
		return nil
	}
	v6 := ip.To16()
	if v6 == nil {
		return nil
	}
	// NAT64 well-known prefix 64:ff9b::/96.
	if v6[0] == 0x00 && v6[1] == 0x64 && v6[2] == 0xff && v6[3] == 0x9b && allZero(v6[4:12]) {
		return net.IPv4(v6[12], v6[13], v6[14], v6[15])
	}
	// IPv4-compatible ::/96 (first 96 bits zero), excluding :: and ::1.
	if allZero(v6[:12]) {
		v4 := net.IPv4(v6[12], v6[13], v6[14], v6[15])
		if v4.Equal(net.IPv4zero) || v4.Equal(net.IPv4(0, 0, 0, 1)) {
			return nil
		}
		return v4
	}
	return nil
}

// allZero reports whether every byte of b is zero.
func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

// isCGNAT matches carrier-grade-NAT space 100.64.0.0/10, which
// net.IP.IsPrivate does not cover but several cloud / overlay fabrics use
// internally. Refused by default (RFC1918-class).
func isCGNAT(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127
}
