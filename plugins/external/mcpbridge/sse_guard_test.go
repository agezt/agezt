// SPDX-License-Identifier: MIT

package main

// Tests for the SSE endpoint SSRF gate (VULN mcp-sse-ssrf-pivot). These
// pin down the four invariants the guard exists for:
//
//  1. Same-origin enforcement: an announced postURL whose host, port, or
//     scheme differs from the SSE URL is rejected (cross-origin pivot).
//  2. Blocked-range enforcement: an announced postURL that resolves into
//     loopback / private / link-local / metadata IP space is rejected
//     unless the matching opt-in env var is set.
//  3. Allowed-range pass-through: an announced postURL whose host is
//     same-origin AND whose resolved IP is permitted reaches send().
//  4. Malformed inputs: bad URLs, bad schemes, empty data — all rejected.
//
// Tests that need to override the opt-in env vars use t.Setenv so the
// overrides are scoped to a single test (no global TestMain state — the
// TestMain in sse_transport_test.go sets loopback+private ON; the tests
// here intentionally set them OFF to exercise the deny path, then back ON
// per case as needed).

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/netguard"
)

func TestSSEResolveEndpoint_SameOriginRelative(t *testing.T) {
	t.Setenv("MCPBRIDGE_ALLOW_LOOPBACK", "1")
	t.Setenv("MCPBRIDGE_ALLOW_PRIVATE", "1")
	// Use an RFC5737 documentation IP (192.0.2.x) — guaranteed never to
	// resolve via DNS in any environment, so the test is hermetic.
	policy, err := buildSSEEndpointPolicy("https://192.0.2.1/sse")
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}
	got, err := resolveEndpoint("https://192.0.2.1/sse", "/messages", policy)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := "https://192.0.2.1/messages"
	if got != want {
		t.Errorf("resolve(%q) = %q, want %q", "/messages", got, want)
	}
}

func TestSSEResolveEndpoint_SameOriginAbsolute(t *testing.T) {
	t.Setenv("MCPBRIDGE_ALLOW_LOOPBACK", "1")
	t.Setenv("MCPBRIDGE_ALLOW_PRIVATE", "1")
	policy, err := buildSSEEndpointPolicy("http://192.0.2.1:9000/sse")
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}
	got, err := resolveEndpoint("http://192.0.2.1:9000/sse", "http://192.0.2.1:9000/messages", policy)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "http://192.0.2.1:9000/messages" {
		t.Errorf("got %q, want same", got)
	}
}

func TestSSEResolveEndpoint_RejectsCrossOriginHost(t *testing.T) {
	policy, err := buildSSEEndpointPolicy("https://mcp.example.com/sse")
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}
	// Operator trusted mcp.example.com; server pivots to attacker.com.
	_, err = resolveEndpoint("https://mcp.example.com/sse", "https://attacker.com/messages", policy)
	if err == nil {
		t.Fatal("cross-origin host accepted; expected rejection")
	}
	if !strings.Contains(err.Error(), "host") {
		t.Errorf("rejection reason %q should mention host pivot", err.Error())
	}
}

func TestSSEResolveEndpoint_RejectsCrossOriginPort(t *testing.T) {
	policy, err := buildSSEEndpointPolicy("https://mcp.example.com/sse")
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}
	// Same host, different port — classic cross-service pivot.
	_, err = resolveEndpoint("https://mcp.example.com/sse", "https://mcp.example.com:9001/messages", policy)
	if err == nil {
		t.Fatal("cross-origin port accepted; expected rejection")
	}
	if !strings.Contains(err.Error(), "port") {
		t.Errorf("rejection reason %q should mention port pivot", err.Error())
	}
}

func TestSSEResolveEndpoint_RejectsCrossOriginScheme(t *testing.T) {
	policy, err := buildSSEEndpointPolicy("https://mcp.example.com/sse")
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}
	// https→http downgrade attempt.
	_, err = resolveEndpoint("https://mcp.example.com/sse", "http://mcp.example.com/messages", policy)
	if err == nil {
		t.Fatal("cross-origin scheme accepted; expected rejection")
	}
	if !strings.Contains(err.Error(), "scheme") {
		t.Errorf("rejection reason %q should mention scheme pivot", err.Error())
	}
}

func TestSSEResolveEndpoint_RejectsLoopbackByDefault(t *testing.T) {
	t.Setenv("MCPBRIDGE_ALLOW_LOOPBACK", "")
	t.Setenv("MCPBRIDGE_ALLOW_PRIVATE", "")
	policy, err := buildSSEEndpointPolicy("http://127.0.0.1:9000/sse")
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}
	// IP literal same-origin but the SSE URL itself is loopback — denied
	// unless MCPBRIDGE_ALLOW_LOOPBACK is set. (Operator who genuinely
	// runs a local MCP server opts in.)
	_, err = resolveEndpoint("http://127.0.0.1:9000/sse", "http://127.0.0.1:9000/messages", policy)
	if err == nil {
		t.Fatal("loopback accepted without opt-in; expected rejection")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Errorf("rejection reason %q should mention loopback", err.Error())
	}
}

func TestSSEResolveEndpoint_RejectsLinkLocalMetadata(t *testing.T) {
	t.Setenv("MCPBRIDGE_ALLOW_LOOPBACK", "1") // would NOT help here
	t.Setenv("MCPBRIDGE_ALLOW_PRIVATE", "1")
	policy, err := buildSSEEndpointPolicy("http://169.254.169.254/sse")
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}
	// Cloud metadata endpoint is link-local — denied even when loopback
	// and private are opted in. There is no opt-in for metadata; that
	// range must stay blocked.
	_, err = resolveEndpoint("http://169.254.169.254/sse", "http://169.254.169.254/latest/meta-data/", policy)
	if err == nil {
		t.Fatal("link-local metadata IP accepted; expected rejection")
	}
	if !strings.Contains(err.Error(), "link-local") {
		t.Errorf("rejection reason %q should mention link-local", err.Error())
	}
}

func TestSSEResolveEndpoint_RejectsPrivateByDefault(t *testing.T) {
	t.Setenv("MCPBRIDGE_ALLOW_LOOPBACK", "1")
	t.Setenv("MCPBRIDGE_ALLOW_PRIVATE", "")
	policy, err := buildSSEEndpointPolicy("http://10.0.0.5:9000/sse")
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}
	_, err = resolveEndpoint("http://10.0.0.5:9000/sse", "http://10.0.0.5:9000/messages", policy)
	if err == nil {
		t.Fatal("private IP accepted without MCPBRIDGE_ALLOW_PRIVATE; expected rejection")
	}
	if !strings.Contains(err.Error(), "private") {
		t.Errorf("rejection reason %q should mention private", err.Error())
	}
}

func TestSSEResolveEndpoint_AllowsLoopbackWhenOptedIn(t *testing.T) {
	t.Setenv("MCPBRIDGE_ALLOW_LOOPBACK", "1")
	t.Setenv("MCPBRIDGE_ALLOW_PRIVATE", "1")
	policy, err := buildSSEEndpointPolicy("http://127.0.0.1:9000/sse")
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}
	got, err := resolveEndpoint("http://127.0.0.1:9000/sse", "http://127.0.0.1:9000/messages", policy)
	if err != nil {
		t.Fatalf("opt-in loopback rejected: %v", err)
	}
	if got != "http://127.0.0.1:9000/messages" {
		t.Errorf("got %q, want same", got)
	}
}

func TestSSEResolveEndpoint_RejectsMalformed(t *testing.T) {
	policy, err := buildSSEEndpointPolicy("https://mcp.example.com/sse")
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "empty endpoint URL"},
		{"ftp scheme", "ftp://mcp.example.com/x", "scheme"},
		{"javascript scheme", "javascript:alert(1)", "scheme"},
		{"bare garbage", "://broken", "parse"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveEndpoint("https://mcp.example.com/sse", tc.in, policy)
			if err == nil {
				t.Fatalf("accepted malformed endpoint %q", tc.in)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestSSEResolveEndpoint_RejectsBuildPolicyBadScheme(t *testing.T) {
	if _, err := buildSSEEndpointPolicy("ftp://mcp.example.com/sse"); err == nil {
		t.Error("build policy accepted non-http(s) scheme")
	}
	if _, err := buildSSEEndpointPolicy("not a url"); err == nil {
		t.Error("build policy accepted garbage")
	}
}

func TestIPPolicyReason_Classifies(t *testing.T) {
	cases := []struct {
		ip    string
		want  []string // any of these substrings in the rejection reason is acceptable
		allow bool     // true = expected to be permitted
	}{
		{"127.0.0.1", []string{"loopback"}, false},
		{"::1", []string{"loopback"}, false},
		{"10.0.0.1", []string{"private"}, false},
		{"172.16.0.1", []string{"private"}, false},
		{"192.168.1.1", []string{"private"}, false},
		{"100.64.0.1", []string{"private"}, false}, // CGNAT
		{"169.254.169.254", []string{"link-local"}, false},
		{"fe80::1", []string{"link-local"}, false},
		{"0.0.0.0", []string{"unspecified"}, false},
		{"::", []string{"unspecified"}, false},
		// 224.0.0.0/24 is BOTH link-local-multicast AND multicast per the
		// IP spec; the implementation's first match wins ("link-local"),
		// but either reason correctly signals "denied", so the test
		// accepts both wordings.
		{"224.0.0.1", []string{"multicast", "link-local"}, false},
		// NAT64 trick: blocked v4 smuggled as v6 literal.
		{"64:ff9b::a9fe:a9fe", []string{"link-local"}, false}, // 169.254.169.254 embedded
		// Public addresses should be allowed.
		{"8.8.8.8", nil, true},
		{"1.1.1.1", nil, true},
	}
	policy := sseEndpointPolicy{} // all opt-ins off
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			got := ipPolicyReason(parseIP(t, tc.ip), policy)
			if tc.allow {
				if got != "" {
					t.Errorf("ipPolicyReason(%s) = %q, want allowed", tc.ip, got)
				}
				return
			}
			matched := false
			for _, w := range tc.want {
				if strings.Contains(got, w) {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("ipPolicyReason(%s) = %q, want one of %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestIPPolicyReason_OptInsLift(t *testing.T) {
	policy := sseEndpointPolicy{allowLoopback: true, allowPrivate: true}
	if got := ipPolicyReason(parseIP(t, "127.0.0.1"), policy); got != "" {
		t.Errorf("loopback still blocked with opt-in: %q", got)
	}
	if got := ipPolicyReason(parseIP(t, "10.0.0.1"), policy); got != "" {
		t.Errorf("private still blocked with opt-in: %q", got)
	}
	// Link-local stays blocked regardless — no opt-in.
	if got := ipPolicyReason(parseIP(t, "169.254.169.254"), policy); got == "" {
		t.Error("link-local allowed; expected block (no opt-in for metadata)")
	}
}

// TestIPPolicyReason_EmbeddedV4NotSmuggled replaces the old
// TestCollapseEmbeddedV4. That test drove a private helper this package no
// longer owns — ipPolicyReason now delegates to kernel/netguard, which
// implements the collapsing itself.
//
// The INVARIANT it protected is real and is kept here, asserted one level up at
// the boundary that actually decides: an IPv4 metadata address wrapped in an
// IPv6 form must not reach the allow path. Deleting the helper's test without
// re-homing the invariant would have quietly dropped that coverage.
func TestIPPolicyReason_EmbeddedV4NotSmuggled(t *testing.T) {
	// Opt in to everything an operator can opt in to; link-local has no opt-in,
	// so the metadata address must stay refused however it is spelled.
	policy := sseEndpointPolicy{allowLoopback: true, allowPrivate: true}
	for _, spelling := range []string{
		"169.254.169.254",        // plain v4
		"64:ff9b::a9fe:a9fe",     // NAT64 well-known prefix
		"::ffff:169.254.169.254", // v4-mapped
	} {
		t.Run(spelling, func(t *testing.T) {
			if got := ipPolicyReason(parseIP(t, spelling), policy); got == "" {
				t.Errorf("metadata address spelled %s was allowed; want refused", spelling)
			}
		})
	}

	// Sanity: an ordinary public address is still permitted, so the test above
	// is not passing because everything is refused.
	if got := ipPolicyReason(parseIP(t, "93.184.216.34"), policy); got != "" {
		t.Errorf("public address refused: %q", got)
	}
}

// parseIP is a tiny helper that fails the test on parse error so the
// per-case logic above stays readable.
func parseIP(t *testing.T, s string) net.IP {
	t.Helper()
	if ip := net.ParseIP(s); ip != nil {
		return ip
	}
	t.Fatalf("net.ParseIP(%q) = nil", s)
	return nil
}

// TestNetguardClient_BlocksRedirectHop is the SSRF-002 regression test.
//
// resolveEndpoint's comment promised the announced endpoint was "enforced again
// per-POST via the dialer — see dialerGuard below". No dialerGuard existed
// anywhere in the repo; the transport used a bare &http.Client{}. The endpoint
// check was therefore ONE-SHOT, at the moment the endpoint event arrived, and a
// 307 walked straight past it into whatever the redirect named.
//
// This drives the exact bypass: loopback is opted in (so the first hop is
// allowed and reaches a real server), private ranges are NOT, and the server
// redirects into 10.0.0.0/8. A one-shot check passes the first hop and never
// sees the second; a dialer-level guard refuses it.
func TestNetguardClient_BlocksRedirectHop(t *testing.T) {
	redirected := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected = true
		http.Redirect(w, r, "http://10.0.0.1/internal", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	// Loopback allowed (the trusted first hop), private refused (the pivot).
	policy := sseEndpointPolicy{allowLoopback: true, allowPrivate: false}
	client := netguard.New(netguardOptsFor(policy)...).HTTPClient(5 * time.Second)

	resp, err := client.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("a redirect into 10.0.0.0/8 must be refused at the dialer; the request succeeded")
	}
	if !redirected {
		t.Fatal("precondition: the trusted first hop should have been reached and issued the redirect")
	}

	// And the guard must not be blocking indiscriminately — the same client
	// still reaches the allowed loopback origin when nothing redirects it.
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer plain.Close()
	ok, err := client.Get(plain.URL)
	if err != nil {
		t.Fatalf("allowed loopback origin must still be reachable: %v", err)
	}
	ok.Body.Close()
}
