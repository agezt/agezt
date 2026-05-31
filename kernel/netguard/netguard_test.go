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
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
