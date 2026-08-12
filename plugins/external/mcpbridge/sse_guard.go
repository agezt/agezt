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
// bridge should refuse to talk to by default.
//
// It DELEGATES that classification to `kernel/netguard` (2026-08-12). This
// comment used to say the bridge was "a deliberately kernel-free binary" and
// that "duplicating the small set of IP-classification helpers here is cheaper
// than importing the kernel package". The duplicate was not cheaper: it drifted,
// losing the zero-block and v4-broadcast cases the kernel guard had grown
// (SSRF-003). The bridge is a separate binary, not a separate module, so the
// import costs nothing at runtime and buys one implementation instead of two.
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

	"github.com/agezt/agezt/kernel/netguard"
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

// netguardPolicyOpts maps the operator opt-ins onto netguard options. Split out
// from netguardOptsFor so the one-shot endpoint classification can share the
// exact policy without also installing the dialer's stderr logger.
func netguardPolicyOpts(policy sseEndpointPolicy) []netguard.Option {
	var opts []netguard.Option
	if policy.allowLoopback {
		opts = append(opts, netguard.AllowLoopback())
	}
	if policy.allowPrivate {
		opts = append(opts, netguard.AllowPrivate())
	}
	return opts
}

// netguardOptsFor translates this package's endpoint policy into kernel/netguard
// options, so the per-dial guard honours exactly the same operator opt-ins as
// the one-shot endpoint check above. Keeping one classifier is the point: this
// package used to carry its own copy, and the copy had drifted (SSRF-003).
func netguardOptsFor(policy sseEndpointPolicy) []netguard.Option {
	opts := netguardPolicyOpts(policy)
	// Surface refusals on stderr. A blocked dial that logs nothing is
	// indistinguishable from a request that was never made — the same
	// invisibility that made browser.action's bypass hard to notice.
	opts = append(opts, netguard.OnBlock(func(ip, reason string) {
		fmt.Fprintf(os.Stderr, "mcpbridge: netguard blocked dial to %s: %s\n", ip, reason)
	}))
	return opts
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
// and is enforced again on every dial by the netguard-backed client the
// transport builds — see netguardOptsFor in sse_transport.go.
//
// That second half used to say "see dialerGuard below". There was no
// dialerGuard, anywhere in the repo: the transport used a bare
// &http.Client{}, so this one-shot check was the ONLY check and a 307 or a
// DNS rebind walked past it (SSRF-002, fixed 2026-08-12). A guard that
// exists only in the comment describing it is worse than no comment.
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

// ipPolicyReason returns a non-empty reason when ip is in a range the bridge
// refuses to dial given the policy, or "" when the address is permitted.
//
// It DELEGATES to kernel/netguard rather than restating its category list. This
// function used to carry its own switch, described as "intentionally mirroring"
// netguard's defaults — and mirroring by intention is what drifts. The copy had
// already fallen behind on the zero-block and v4-broadcast cases (SSRF-003).
// Delegation cannot drift, and it means the one-shot endpoint check and the
// per-dial guard now decide with one implementation instead of two.
func ipPolicyReason(ip net.IP, policy sseEndpointPolicy) string {
	if ip == nil {
		return "unparseable address"
	}
	if ok, reason := netguard.New(netguardPolicyOpts(policy)...).Allowed(ip); !ok {
		return reason
	}
	return ""
}

// The IP classification helpers that used to live here — collapseEmbeddedV4,
// allZero and isCGNAT — were deleted on 2026-08-12 when ipPolicyReason began
// delegating to kernel/netguard, which implements all three cases and several
// this copy had never grown (SSRF-003).
