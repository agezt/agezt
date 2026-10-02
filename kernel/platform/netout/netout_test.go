// SPDX-License-Identifier: MIT

package netout

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestHostAllowed pins the one allowlist grammar every tool now shares. The
// http tool's private copy let "*.x.com" match "a.b.x.com"; the browser's did
// not. One level, as both tools' docs always said.
func TestHostAllowed(t *testing.T) {
	e := Egress{AllowedHosts: []string{"example.com", "*.x.com", " API.Corp.io "}}
	for host, want := range map[string]bool{
		"example.com":     true,
		"EXAMPLE.com":     true,
		"example.com.":    true,
		"sub.x.com":       true,
		"x.com":           false, // apex does not match *.x.com
		"a.b.x.com":       false, // one level only
		"evilx.com":       false,
		"sub.evil.com":    false,
		"api.corp.io":     true,
		"":                false,
		"notexample.com":  false,
		"sub.example.com": false,
	} {
		if got := e.HostAllowed(host); got != want {
			t.Errorf("HostAllowed(%q) = %v, want %v", host, got, want)
		}
	}
	if !(Egress{AnyHost: true}).HostAllowed("anything.example") {
		t.Error("AnyHost must allow every host")
	}
	if (Egress{}).HostAllowed("example.com") {
		t.Error("an empty allowlist without AnyHost must allow nothing")
	}
}

// TestClientRechecksAllowlistOnRedirect: an allowed host that redirects to a
// host outside the allowlist must not be followed.
func TestClientRechecksAllowlistOnRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://elsewhere.invalid/", http.StatusFound)
	}))
	defer srv.Close()
	e := Egress{AllowedHosts: []string{"127.0.0.1"}, AllowLoopback: true}
	_, err := e.Client(5 * time.Second).Get(srv.URL)
	if !errors.Is(err, ErrHostDenied) || !strings.Contains(err.Error(), "elsewhere.invalid") {
		t.Fatalf("redirect to a non-allowlisted host: err = %v, want ErrHostDenied", err)
	}
}

// TestClientCapsRedirects: a redirect loop stops at MaxRedirects.
func TestClientCapsRedirects(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/again", http.StatusFound)
	}))
	defer srv.Close()
	_, err := Egress{AnyHost: true, AllowLoopback: true}.Client(5 * time.Second).Get(srv.URL)
	if err == nil || !strings.Contains(err.Error(), "stopped after") {
		t.Fatalf("err = %v, want redirect cap", err)
	}
}

// TestPostures: loopback is refused unless opted in, and the metadata range is
// refused in every posture, the most permissive included.
func TestPostures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	var blocked string
	strict := Egress{AnyHost: true, OnBlock: func(ip, reason string) { blocked = ip }}
	if _, err := strict.Client(5 * time.Second).Get(srv.URL); err == nil || blocked == "" {
		t.Fatalf("strict posture reached loopback (err=%v, blocked=%q)", err, blocked)
	}
	resp, err := Egress{AnyHost: true, AllowLoopback: true}.Client(5 * time.Second).Get(srv.URL)
	if err != nil {
		t.Fatalf("loopback opt-in refused loopback: %v", err)
	}
	resp.Body.Close()

	for _, e := range []Egress{strict, {AnyHost: true, AllowLoopback: true, AllowPrivate: true}} {
		if ok, _ := e.Guard().Allowed([]byte{169, 254, 169, 254}); ok {
			t.Fatalf("posture %+v allows the cloud metadata address", e)
		}
	}
}

// TestOperatorClient: loopback/private reachable (local model servers, LAN
// channels), the metadata range refused, the environment proxy honoured as
// http.DefaultTransport does, and one transport shared by every client.
func TestOperatorClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	resp, err := OperatorClient(5 * time.Second).Get(srv.URL)
	if err != nil {
		t.Fatalf("operator client refused loopback: %v", err)
	}
	resp.Body.Close()

	_, err = OperatorClient(2 * time.Second).Get("http://169.254.169.254/latest/meta-data/")
	if err == nil || !strings.Contains(err.Error(), "netguard: blocked") {
		t.Fatalf("operator client dialled the metadata address: err = %v", err)
	}

	tr := OperatorTransport().(*http.Transport)
	if tr.Proxy == nil {
		t.Fatal("operator transport ignores HTTP(S)_PROXY; http.DefaultTransport honours it")
	}
	if OperatorClient(time.Second).Transport != OperatorClient(time.Minute).Transport {
		t.Fatal("operator clients must share one transport (connection reuse)")
	}
}
