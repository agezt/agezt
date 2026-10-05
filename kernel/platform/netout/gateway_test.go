// SPDX-License-Identifier: MIT

package netout

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGatewayGETPreservesHeadersStatusTypeAndBodyBound(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/models" || r.URL.Query().Get("format") != "fixture" || r.Header.Get("X-Fixture-Key") != "fixture-value" {
			t.Errorf("request lost method/path/query/header: %s %s %v", r.Method, r.URL, r.Header)
		}
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("abcdefgh"))
	}))
	defer server.Close()
	body, status, ctype, err := GatewayGET(server.URL+"/models?format=fixture", "X-Fixture-Key", "fixture-value", 4)
	if err != nil || string(body) != "abcd" || status != http.StatusTeapot || ctype != "image/png" || calls.Load() != 1 {
		t.Fatalf("response = %q %d %q, %v; calls=%d", body, status, ctype, err, calls.Load())
	}
}

func TestGatewayGETPreservesEmptyKeyAndDirectTransport(t *testing.T) {
	var endpointCalls, proxyCalls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpointCalls.Add(1)
		if _, exists := r.Header["X-Fixture-Key"]; exists {
			t.Error("empty key added a header")
		}
		_, _ = w.Write([]byte("direct"))
	}))
	defer server.Close()
	body, status, _, err := GatewayGET(server.URL, "X-Fixture-Key", "", 100)
	if err != nil || status != http.StatusOK || string(body) != "direct" || endpointCalls.Load() != 1 || proxyCalls.Load() != 0 {
		t.Fatalf("transport changed: %q %d %v, endpoint=%d proxy=%d", body, status, err, endpointCalls.Load(), proxyCalls.Load())
	}
}

func TestGatewayGETPreservesRedirectsAndTheirBound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/final", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		default:
			if r.Header.Get("Authorization") != "Bearer fixture" {
				t.Error("redirect lost same-host authorization header")
			}
			_, _ = w.Write([]byte("redirected"))
		}
	}))
	defer server.Close()
	body, status, _, err := GatewayGET(server.URL+"/start", "Authorization", "Bearer fixture", 100)
	if err != nil || status != http.StatusOK || string(body) != "redirected" {
		t.Fatalf("redirect response = %q %d %v", body, status, err)
	}
	if _, _, _, err := GatewayGET(server.URL+"/loop", "", "", 100); err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Fatalf("redirect bound changed: %v", err)
	}
}

func TestGatewayGETPreservesParseErrorIdentity(t *testing.T) {
	for _, input := range []string{"ftp://fixture.invalid", "http:///missing-host", "http://[invalid"} {
		body, status, ctype, err := GatewayGET(input, "", "", 100)
		var parsed *url.Error
		if !errors.As(err, &parsed) || parsed.Op != "parse" || parsed.URL != input || body != nil || status != 0 || ctype != "" {
			t.Fatalf("invalid URL %q = %q %d %q %v", input, body, status, ctype, err)
		}
	}
}

func TestGatewayGETPreservesBestEffortPartialRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("partial"))
	}))
	defer server.Close()
	body, status, _, err := GatewayGET(server.URL, "", "", 100)
	if err != nil || status != http.StatusOK || string(body) != "partial" {
		t.Fatalf("legacy partial read changed: %q %d %v", body, status, err)
	}
}
