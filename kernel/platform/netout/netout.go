// SPDX-License-Identifier: MIT

// Package netout is the outbound-HTTP platform (architecture/20-target-
// architecture.md §5, layer L2): the one place a client that dials out is built.
//
// An Egress names a posture — which hosts may be named, which address ranges may
// be reached — and Client turns it into an *http.Client whose dialer checks the
// RESOLVED address of every connection (kernel/netguard) and whose redirect
// policy re-checks the host allowlist on every hop. Those two checks used to be
// re-implemented per tool, and drifted: two copies of the allowlist matcher
// disagreed on what "*.example.com" means, and the `fetch` tool, governed as the
// same `http.get` capability as the `http` tool, ignored the allowlist that
// restricts it.
//
// Postures:
//
//   - An agent-driven tool (http, fetch, browser.read) uses an Egress built from
//     the operator's settings: any public host by default (owner's default-allow
//     posture), an allowlist when the operator pins one, loopback/private only
//     when explicitly opted in.
//   - An operator-configured endpoint (a provider base URL, a channel server, a
//     webhook) uses OperatorClient: loopback and private networks are legitimate
//     destinations there, the link-local / cloud-metadata range never is.
//   - The two credential paths whose whole job is to ask the metadata service
//     (AWS IMDS, the GCE metadata server) use MetadataClient — the one named,
//     greppable exception.
//
// Outside MetadataClient the link-local range (169.254.0.0/16 including the
// cloud metadata endpoint, fe80::/10) and the unspecified address are refused.
package netout

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/netguard"
)

// MaxRedirects caps a redirect chain. It matches Go's default and is explicit
// because installing CheckRedirect replaces that default.
const MaxRedirects = 10

// ErrHostDenied is returned when a request or a redirect hop names a host the
// Egress does not allow.
var ErrHostDenied = errors.New("host not in allowlist")

// Egress is an outbound posture.
type Egress struct {
	// AnyHost allows every host name; the IP guard still applies. When false,
	// only AllowedHosts may be named — an empty list then allows nothing.
	AnyHost bool
	// AllowedHosts are bare host names, case-insensitive, no scheme or port.
	// "*.example.com" matches exactly one subdomain level ("api.example.com",
	// not "example.com" and not "a.b.example.com").
	AllowedHosts []string
	// AllowLoopback / AllowPrivate let the dialer reach 127.0.0.0/8 + ::1 and
	// RFC1918 + ULA respectively. Neither unblocks link-local or metadata.
	AllowLoopback bool
	AllowPrivate  bool
	// OnBlock is told (resolved IP, reason) whenever the dialer refuses an
	// address — the daemon journals it as netguard.blocked.
	OnBlock func(ip, reason string)
}

// HostAllowed reports whether host (a bare host name, as url.URL.Hostname
// returns it) may be named under this posture.
func (e Egress) HostAllowed(host string) bool {
	if e.AnyHost {
		return true
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return false
	}
	for _, pat := range e.AllowedHosts {
		pat = strings.ToLower(strings.TrimSpace(pat))
		switch {
		case pat == "":
		case strings.HasPrefix(pat, "*."):
			parent := pat[2:]
			if label, ok := strings.CutSuffix(host, "."+parent); ok && label != "" && !strings.Contains(label, ".") {
				return true
			}
		case pat == host:
			return true
		}
	}
	return false
}

// Guard returns the dial-time IP guard for this posture.
func (e Egress) Guard() *netguard.Guard {
	var opts []netguard.Option
	if e.AllowLoopback {
		opts = append(opts, netguard.AllowLoopback())
	}
	if e.AllowPrivate {
		opts = append(opts, netguard.AllowPrivate())
	}
	if e.OnBlock != nil {
		opts = append(opts, netguard.OnBlock(e.OnBlock))
	}
	return netguard.New(opts...)
}

// Client returns an HTTP client for this posture: the IP guard on every dial
// (initial and each redirect), the host allowlist on every redirect hop, at
// most MaxRedirects hops, and the given overall timeout.
//
// The initial request's host is the caller's to check (HostAllowed) — callers
// want to refuse it with their own message before any network traffic.
func (e Egress) Client(timeout time.Duration) *http.Client {
	c := e.Guard().HTTPClient(timeout)
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= MaxRedirects {
			return fmt.Errorf("stopped after %d redirects", MaxRedirects)
		}
		if !e.HostAllowed(req.URL.Hostname()) {
			return fmt.Errorf("%w: %s (redirect target)", ErrHostDenied, req.URL.Hostname())
		}
		return nil
	}
	return c
}
