// SPDX-License-Identifier: MIT

// Package netguard is the network-egress guard (SPEC-06 security defaults): it
// stops outbound tool traffic from reaching the host's own internal network.
// An autonomous agent — especially one steered by a prompt-injected instruction
// — must not be able to read the cloud metadata endpoint (169.254.169.254),
// probe `127.0.0.1`, or pivot into RFC1918 space to exfiltrate credentials.
//
// The guard works at the DIALER, not the URL string. A hostname allowlist alone
// is not enough: an allowed host can resolve to an internal IP (DNS rebinding),
// and an allowed host can 30x-redirect to `http://169.254.169.254/…`. By
// validating the *resolved* IP on every connection attempt — initial dial AND
// every redirect hop — via net.Dialer.Control, the guard sees the concrete
// address actually being connected to and refuses the blocked ones before the
// socket connects.
//
// Default policy (secure-by-default, DECISIONS F2): block loopback, private
// (RFC1918 + ULA), link-local (169.254/16 incl. cloud metadata, fe80::/10), and
// the unspecified address. Opt back in per range for local development.
package netguard

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// Guard decides whether an outbound connection's resolved IP is permitted.
// The zero value blocks every internal range (the secure default); use the
// Allow* options to relax it.
type Guard struct {
	allowLoopback bool
	allowPrivate  bool // RFC1918 + ULA
	allowLinkqual bool // link-local (169.254/16, fe80::/10) — includes cloud metadata
	onBlock       func(ip, reason string)
}

// Option configures a Guard.
type Option func(*Guard)

// AllowLoopback permits connections to 127.0.0.0/8 and ::1. Use only when a tool
// is meant to reach a local service (development, a sidecar).
func AllowLoopback() Option { return func(g *Guard) { g.allowLoopback = true } }

// AllowPrivate permits RFC1918 (10/8, 172.16/12, 192.168/16) and IPv6 ULA
// (fc00::/7). Use for tools meant to reach the local network.
func AllowPrivate() Option { return func(g *Guard) { g.allowPrivate = true } }

// AllowLinkLocal permits link-local addresses, INCLUDING the cloud metadata
// endpoint (169.254.169.254). Dangerous; only for explicit, trusted use.
func AllowLinkLocal() Option { return func(g *Guard) { g.allowLinkqual = true } }

// OnBlock installs a callback invoked (with the resolved IP and reason) every
// time the guard refuses a dial (M109). The daemon uses it to journal a
// netguard.blocked event so an operator can audit egress that was stopped —
// notably a tool trying to reach the cloud-metadata endpoint, a sign of prompt
// injection or exfiltration. The callback runs on the dial path, so it must be
// cheap and non-blocking.
func OnBlock(fn func(ip, reason string)) Option { return func(g *Guard) { g.onBlock = fn } }

// New builds a Guard with the secure defaults, relaxed by any options.
func New(opts ...Option) *Guard {
	g := &Guard{}
	for _, o := range opts {
		o(g)
	}
	return g
}

// Allowed reports whether ip may be connected to. The second return is a short
// reason when blocked (empty when allowed).
func (g *Guard) Allowed(ip net.IP) (bool, string) {
	if ip == nil {
		return false, "unparseable address"
	}
	switch {
	case ip.IsUnspecified():
		return false, "unspecified address"
	case ip.IsLoopback():
		if g.allowLoopback {
			return true, ""
		}
		return false, "loopback"
	case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
		if g.allowLinkqual {
			return true, ""
		}
		return false, "link-local (cloud metadata / autoconf)"
	case ip.IsPrivate():
		if g.allowPrivate {
			return true, ""
		}
		return false, "private network"
	}
	return true, ""
}

// Control is a net.Dialer.Control hook. The dialer calls it after resolving the
// host, with address as the concrete IP:port about to be connected — so it sees
// past DNS rebinding and applies on every redirect hop. It returns a non-nil
// error to refuse the connection.
func (g *Guard) Control(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("netguard: bad dial address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// The dialer should always hand us a literal IP here; if not, fail
		// closed rather than connect to something we can't classify.
		return fmt.Errorf("netguard: refusing unresolved dial address %q", address)
	}
	if ok, reason := g.Allowed(ip); !ok {
		if g.onBlock != nil {
			g.onBlock(ip.String(), reason)
		}
		return fmt.Errorf("netguard: blocked %s (%s)", ip, reason)
	}
	return nil
}

// Dialer returns a *net.Dialer that refuses blocked addresses, with the given
// connect timeout (0 = no timeout).
func (g *Guard) Dialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{Timeout: timeout, Control: g.Control}
}

// HTTPClient returns an *http.Client whose every connection — initial and each
// redirect hop — is validated by the guard. timeout bounds the whole request.
// The transport is a fresh, non-shared one so the guard can't leak into other
// clients' connection pools.
func (g *Guard) HTTPClient(timeout time.Duration) *http.Client {
	d := g.Dialer(timeout)
	tr := &http.Transport{
		DialContext:           d.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{Timeout: timeout, Transport: tr}
}
