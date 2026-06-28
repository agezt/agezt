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
	// Collapse IPv6 forms that embed an IPv4 address — NAT64 (64:ff9b::/96) and the
	// deprecated IPv4-compatible (::a.b.c.d) — down to that IPv4 and classify it.
	// Otherwise a blocked v4 (above all the metadata IP 169.254.169.254) is
	// smuggled past the v4 checks as an IPv6 literal, e.g.
	// http://[64:ff9b::a9fe:a9fe]/ which a NAT64 gateway routes to the metadata
	// service (M171). (IPv4-mapped ::ffff:a.b.c.d is already classified directly:
	// net.IP's methods use To4 internally.)
	if v4 := embeddedV4(ip); v4 != nil {
		ip = v4
	}
	switch {
	case ip.IsUnspecified():
		return false, "unspecified address"
	case isZeroBlock(ip):
		return false, "reserved (0.0.0.0/8)"
	case ip.IsLoopback():
		if g.allowLoopback {
			return true, ""
		}
		return false, "loopback"
	case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
		return false, "link-local (cloud metadata / autoconf)"
	case ip.IsPrivate() || isCGNAT(ip):
		if g.allowPrivate {
			return true, ""
		}
		return false, "private network"
	case ip.IsMulticast() || isV4Broadcast(ip):
		return false, "multicast/broadcast"
	}
	return true, ""
}

// embeddedV4 returns the IPv4 address embedded in an IPv6 literal for the forms
// that actually route to it — NAT64 (64:ff9b::/96) and IPv4-compatible (::/96) —
// or nil when ip carries no such embedding. A plain IPv4 (or IPv4-mapped
// ::ffff:a.b.c.d, which net.IP classifies directly via To4) returns nil. :: and
// ::1 are excluded so they keep their own (unspecified/loopback) reasons.
func embeddedV4(ip net.IP) net.IP {
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

func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

// isZeroBlock matches the whole 0.0.0.0/8 "this host on this network" range
// (Linux routes it to local interfaces; 0.0.0.1 etc. are loopback-pivot aliases).
// net.IP.IsUnspecified only matches the exact 0.0.0.0.
func isZeroBlock(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 0
}

// isCGNAT matches carrier-grade-NAT space 100.64.0.0/10, which net.IP.IsPrivate
// does not cover but several cloud/overlay fabrics use internally.
func isCGNAT(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127
}

// isV4Broadcast matches the limited broadcast 255.255.255.255.
func isV4Broadcast(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 255 && v4[1] == 255 && v4[2] == 255 && v4[3] == 255
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
