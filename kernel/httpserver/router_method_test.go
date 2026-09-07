package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	kernelauth "github.com/agezt/agezt/kernel/auth"
)

// Handle validates, normalizes, and records the route's declared method
// policy — so it must also enforce it. A route declared Method: POST,
// Mutation: true that accepts GET lets every state-changing handler be
// reached by navigation/CSRF-class GET requests, and Routes() then reports a
// tighter policy than the server actually applies.
func TestHandle_EnforcesDeclaredMethod(t *testing.T) {
	rt := NewRouter(Authenticator{}, nil)
	called := false
	rt.Handle("/api/mutate", RouteOpts{Tier: kernelauth.TierPublic, Method: http.MethodPost, Mutation: true}, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	get := httptest.NewRequest(http.MethodGet, "/api/mutate", nil)
	grec := httptest.NewRecorder()
	rt.ServeHTTP(grec, get)
	if called {
		t.Fatalf("GET reached a POST-declared mutation handler: the router validates, normalizes and records the method policy but never enforces it — state-changing handlers are callable via any method and Routes() misrepresents the live policy")
	}
	if grec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET to POST-declared route: got %d, want 405", grec.Code)
	}
	if allow := grec.Header().Get("Allow"); allow != "POST" {
		t.Fatalf("Allow header: got %q, want %q", allow, "POST")
	}

	post := httptest.NewRequest(http.MethodPost, "/api/mutate", nil)
	prec := httptest.NewRecorder()
	rt.ServeHTTP(prec, post)
	if !called || prec.Code != http.StatusOK {
		t.Fatalf("declared method blocked: called=%v code=%d", called, prec.Code)
	}
}

// Comma-separated method lists (the console's "GET,OPTIONS" SSE-token shape)
// allow each listed method and refuse the rest.
func TestHandle_MultiMethodList(t *testing.T) {
	rt := NewRouter(Authenticator{}, nil)
	rt.Handle("/multi", RouteOpts{Tier: kernelauth.TierPublic, Method: "GET,OPTIONS"}, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	for _, m := range []string{http.MethodGet, http.MethodOptions} {
		rec := httptest.NewRecorder()
		rt.ServeHTTP(rec, httptest.NewRequest(m, "/multi", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s on GET,OPTIONS route: got %d, want 200", m, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/multi", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE on GET,OPTIONS route: got %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET, OPTIONS" {
		t.Fatalf("Allow header: got %q, want %q", allow, "GET, OPTIONS")
	}
}

// The empty/`*` method default is the documented migration escape hatch: it
// must keep accepting every method.
func TestHandle_WildcardMethodStillAcceptsAnything(t *testing.T) {
	rt := NewRouter(Authenticator{}, nil)
	rt.Handle("/wild", RouteOpts{Tier: kernelauth.TierPublic}, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		rec := httptest.NewRecorder()
		rt.ServeHTTP(rec, httptest.NewRequest(m, "/wild", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s on *-method route: got %d, want 200 — the migration default must stay open", m, rec.Code)
		}
	}
}

// The auth wrapper stays outermost: an unauthenticated request to a protected
// route keeps its 401 even with the wrong method (no behavior change for
// unauthenticated traffic).
func TestHandle_MethodCheckAfterAuth(t *testing.T) {
	reject := func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
	rt := NewRouter(Authenticator{}, reject)
	rt.Handle("/p", RouteOpts{Tier: kernelauth.TierUser, Method: http.MethodGet}, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/p", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated DELETE on protected route: got %d, want 401 (auth stays outermost)", rec.Code)
	}
}
