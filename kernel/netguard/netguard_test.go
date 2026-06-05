// SPDX-License-Identifier: MIT

package netguard_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/netguard"
)

func TestAllowed_DefaultBlocksInternal(t *testing.T) {
	g := netguard.New()
	blocked := []string{
		"127.0.0.1", "127.5.5.5", // loopback
		"10.0.0.1", "172.16.3.4", "192.168.1.1", // RFC1918
		"169.254.169.254",         // cloud metadata (link-local)
		"::1",                     // IPv6 loopback
		"fe80::1",                 // IPv6 link-local
		"fc00::1", "fd12:3456::1", // IPv6 ULA (private)
		"0.0.0.0", "::", // unspecified
		"::ffff:127.0.0.1", // IPv4-mapped loopback
	}
	for _, s := range blocked {
		ip := net.ParseIP(s)
		if ok, reason := g.Allowed(ip); ok {
			t.Errorf("Allowed(%s) = true, want blocked", s)
		} else if reason == "" {
			t.Errorf("Allowed(%s) blocked but no reason given", s)
		}
	}

	allowed := []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:2800:220:1::1"}
	for _, s := range allowed {
		ip := net.ParseIP(s)
		if ok, _ := g.Allowed(ip); !ok {
			t.Errorf("Allowed(%s) = false, want allowed (public)", s)
		}
	}
}

// TestAllowed_SSRFBypassVectors — the encoding/range bypasses closed in M171.
// Each MUST block by default; the NAT64/IPv4-compatible cases are the credential-
// theft path (they wrap 169.254.169.254 = a9fe:a9fe in an IPv6 literal).
func TestAllowed_SSRFBypassVectors(t *testing.T) {
	g := netguard.New()
	blocked := map[string]string{
		"64:ff9b::a9fe:a9fe": "NAT64-wrapped metadata 169.254.169.254",
		"::a9fe:a9fe":        "IPv4-compatible metadata 169.254.169.254",
		"64:ff9b::7f00:1":    "NAT64-wrapped loopback 127.0.0.1",
		"64:ff9b::a00:1":     "NAT64-wrapped private 10.0.0.1",
		"100.64.0.1":         "CGNAT 100.64.0.0/10",
		"100.127.255.255":    "CGNAT upper bound",
		"0.0.0.1":            "0.0.0.0/8 reserved",
		"255.255.255.255":    "limited broadcast",
		"239.1.2.3":          "multicast",
	}
	for s, desc := range blocked {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Errorf("%s (%s) did not parse", s, desc)
			continue
		}
		if ok, _ := g.Allowed(ip); ok {
			t.Errorf("Allowed(%s) = true, want BLOCKED — %s", s, desc)
		}
	}
	// A legit public CGNAT-adjacent and public v6 stay allowed (no over-block).
	for _, s := range []string{"100.63.255.255" /* just below CGNAT */, "101.0.0.1", "2606:2800:220:1::1"} {
		if ok, _ := g.Allowed(net.ParseIP(s)); !ok {
			t.Errorf("Allowed(%s) = false, want allowed (public)", s)
		}
	}
}

func TestAllowed_OptIns(t *testing.T) {
	if ok, _ := netguard.New(netguard.AllowLoopback()).Allowed(net.ParseIP("127.0.0.1")); !ok {
		t.Error("AllowLoopback should permit 127.0.0.1")
	}
	if ok, _ := netguard.New(netguard.AllowPrivate()).Allowed(net.ParseIP("10.1.2.3")); !ok {
		t.Error("AllowPrivate should permit 10.1.2.3")
	}
	if ok, _ := netguard.New(netguard.AllowLinkLocal()).Allowed(net.ParseIP("169.254.169.254")); !ok {
		t.Error("AllowLinkLocal should permit the metadata endpoint")
	}
	// Opt-ins are independent: AllowPrivate must NOT unblock the metadata endpoint.
	if ok, _ := netguard.New(netguard.AllowPrivate()).Allowed(net.ParseIP("169.254.169.254")); ok {
		t.Error("AllowPrivate must not unblock link-local metadata")
	}
}

func TestControl_RejectsBlockedAddress(t *testing.T) {
	g := netguard.New()
	if err := g.Control("tcp", "169.254.169.254:80", nil); err == nil {
		t.Error("Control should reject the metadata endpoint")
	} else if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("unexpected error: %v", err)
	}
	if err := g.Control("tcp", "8.8.8.8:443", nil); err != nil {
		t.Errorf("Control should allow a public address: %v", err)
	}
	// A non-literal (unresolved) address fails closed.
	if err := g.Control("tcp", "example.com:80", nil); err == nil {
		t.Error("Control should refuse a non-IP address (fail closed)")
	}
}

// End-to-end: the guarded client must refuse to reach a loopback server by
// default (httptest binds 127.0.0.1), and reach it once loopback is allowed.
func TestHTTPClient_BlocksLoopbackByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	blocked := netguard.New().HTTPClient(5 * time.Second)
	if _, err := blocked.Get(srv.URL); err == nil {
		t.Error("default guard should block the loopback test server")
	} else if !strings.Contains(err.Error(), "netguard") {
		t.Errorf("expected a netguard error, got: %v", err)
	}

	allowed := netguard.New(netguard.AllowLoopback()).HTTPClient(5 * time.Second)
	resp, err := allowed.Get(srv.URL)
	if err != nil {
		t.Fatalf("AllowLoopback client should reach the server: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// A redirect to an internal address must be blocked at the dial of the redirect
// hop, even though the first hop was an allowed (loopback-permitted) host.
func TestHTTPClient_BlocksRedirectToInternal(t *testing.T) {
	// Target the metadata endpoint via redirect. Loopback is allowed so the
	// FIRST hop (the test server) connects; the redirect hop must be blocked.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redir" {
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := netguard.New(netguard.AllowLoopback()).HTTPClient(5 * time.Second)
	_, err := client.Get(srv.URL + "/redir")
	if err == nil {
		t.Fatal("redirect to the metadata endpoint should be blocked")
	}
	if !strings.Contains(err.Error(), "netguard") {
		t.Errorf("expected a netguard rejection on the redirect hop, got: %v", err)
	}
}

func TestOnBlock_FiresOnRefusal(t *testing.T) {
	var gotIP, gotReason string
	calls := 0
	g := netguard.New(netguard.OnBlock(func(ip, reason string) {
		calls++
		gotIP, gotReason = ip, reason
	}))

	// A blocked dial (metadata IP) must invoke the callback with ip + reason.
	err := g.Control("tcp", "169.254.169.254:80", nil)
	if err == nil {
		t.Fatalf("expected Control to block the metadata IP")
	}
	if calls != 1 {
		t.Fatalf("OnBlock called %d times, want 1", calls)
	}
	if gotIP != "169.254.169.254" || gotReason == "" {
		t.Errorf("OnBlock got ip=%q reason=%q", gotIP, gotReason)
	}

	// An allowed dial must NOT invoke the callback.
	calls = 0
	if err := g.Control("tcp", "8.8.8.8:443", nil); err != nil {
		t.Fatalf("public IP should be allowed: %v", err)
	}
	if calls != 0 {
		t.Errorf("OnBlock fired %d times for an allowed dial, want 0", calls)
	}
}
