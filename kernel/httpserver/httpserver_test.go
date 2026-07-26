// SPDX-License-Identifier: MIT

package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kernelauth "github.com/agezt/agezt/kernel/auth"
)

func request(token, tenant string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if token != "" {
		r.Header.Set("Authorization", token)
	}
	if tenant != "" {
		r.Header.Set("X-Agezt-Tenant", tenant)
	}
	return r
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"Bearer secret", "secret"},
		{"Bearer ", ""},
		{"bearer secret", ""},
		{"Basic secret", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := BearerToken(request(tt.header, "")); got != tt.want {
			t.Errorf("BearerToken(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
	if got := BearerToken(nil); got != "" {
		t.Errorf("BearerToken(nil) = %q, want empty", got)
	}
}

func TestAuthenticatorTiersAndTenantScope(t *testing.T) {
	var tenantCalls int
	a := Authenticator{
		Verifier: kernelauth.NewStaticVerifier("admin"),
		TenantAuthorize: func(tenant, presented string) bool {
			tenantCalls++
			return tenant == "acme" && presented == "tenant-token"
		},
	}
	tests := []struct {
		name     string
		header   string
		tenant   string
		required kernelauth.Tier
		want     bool
	}{
		{"public", "", "", kernelauth.TierPublic, true},
		{"admin on admin", "Bearer admin", "", kernelauth.TierAdmin, true},
		{"admin on user", "Bearer admin", "", kernelauth.TierUser, true},
		{"tenant on user", "Bearer tenant-token", "acme", kernelauth.TierUser, true},
		{"tenant cannot admin", "Bearer tenant-token", "acme", kernelauth.TierAdmin, false},
		{"tenant needs scope", "Bearer tenant-token", "", kernelauth.TierUser, false},
		{"wrong tenant", "Bearer tenant-token", "other", kernelauth.TierUser, false},
		{"blank token", "", "acme", kernelauth.TierUser, false},
		{"unknown tier", "Bearer admin", "", kernelauth.Tier(99), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := a.Authorized(request(tt.header, tt.tenant), tt.required); got != tt.want {
				t.Fatalf("Authorized = %v, want %v", got, tt.want)
			}
		})
	}
	if tenantCalls != 2 {
		t.Fatalf("tenant authorizer calls = %d, want 2 user-tier scoped checks", tenantCalls)
	}
}

func TestAuthenticatorCustomTenantHeader(t *testing.T) {
	a := Authenticator{
		Verifier:     kernelauth.NewStaticVerifier("admin"),
		TenantHeader: "X-Custom-Tenant",
		TenantAuthorize: func(tenant, presented string) bool {
			return tenant == "custom" && presented == "scoped"
		},
	}
	r := request("Bearer scoped", "")
	r.Header.Set("X-Custom-Tenant", " custom ")
	if !a.Authorized(r, kernelauth.TierUser) {
		t.Fatal("custom tenant header did not authorize")
	}
}

func TestAuthenticatorRequestAuthorizerOwnsProtectedRoutes(t *testing.T) {
	var calls int
	a := Authenticator{
		Verifier: kernelauth.NewStaticVerifier("must-not-be-used"),
		RequestAuthorize: func(r *http.Request, tier kernelauth.Tier) bool {
			calls++
			return tier == kernelauth.TierUser && r.Header.Get("X-Session") == "valid"
		},
	}

	if !a.Authorized(request("", ""), kernelauth.TierPublic) {
		t.Fatal("public route was not authorized")
	}
	if calls != 0 {
		t.Fatalf("request authorizer called %d times for public route, want 0", calls)
	}

	protected := request("Bearer must-not-be-used", "")
	if a.Authorized(protected, kernelauth.TierUser) {
		t.Fatal("fallback verifier bypassed the request authorizer")
	}
	protected.Header.Set("X-Session", "valid")
	if !a.Authorized(protected, kernelauth.TierUser) {
		t.Fatal("request authorizer did not authorize valid session")
	}
	if calls != 2 {
		t.Fatalf("request authorizer calls = %d, want 2", calls)
	}
}

func TestMiddlewarePreservesSurfaceError(t *testing.T) {
	var handled bool
	a := Authenticator{Verifier: kernelauth.NewStaticVerifier("admin")}
	mw := a.Middleware(kernelauth.TierAdmin, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"surface-shaped"}`))
	})
	h := mw(func(w http.ResponseWriter, _ *http.Request) {
		handled = true
		w.WriteHeader(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	h(rec, request("", ""))
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "surface-shaped") {
		t.Fatalf("unauthorized response = %d %q", rec.Code, rec.Body.String())
	}
	if handled {
		t.Fatal("unauthorized request reached handler")
	}

	rec = httptest.NewRecorder()
	h(rec, request("Bearer admin", ""))
	if rec.Code != http.StatusNoContent || !handled {
		t.Fatalf("authorized response = %d handled=%v", rec.Code, handled)
	}
}

func TestRouterAppliesAuthBeforeBodyLimit(t *testing.T) {
	a := Authenticator{Verifier: kernelauth.NewStaticVerifier("admin")}
	rt := NewRouter(a, nil)
	var decoded bool
	rt.Handle("/limited", RouteOpts{
		Tier:    kernelauth.TierAdmin,
		BodyMax: 8,
	}, func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		err := json.NewDecoder(r.Body).Decode(&v)
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}
		decoded = true
		w.WriteHeader(http.StatusNoContent)
	})

	unauthorized := httptest.NewRequest(http.MethodPost, "/limited", strings.NewReader(strings.Repeat("x", 64)))
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, unauthorized)
	if rec.Code != http.StatusUnauthorized || decoded {
		t.Fatalf("unauthorized oversized request = %d decoded=%v", rec.Code, decoded)
	}

	oversized := httptest.NewRequest(http.MethodPost, "/limited", strings.NewReader(`{"value":"too large"}`))
	oversized.Header.Set("Authorization", "Bearer admin")
	rec = httptest.NewRecorder()
	rt.ServeHTTP(rec, oversized)
	if rec.Code != http.StatusRequestEntityTooLarge || decoded {
		t.Fatalf("authorized oversized request = %d decoded=%v", rec.Code, decoded)
	}
}

func TestRouterMetadataIsCopied(t *testing.T) {
	rt := NewRouter(Authenticator{}, nil)
	rt.Handle("/public", RouteOpts{Tier: kernelauth.TierPublic}, func(http.ResponseWriter, *http.Request) {})
	got := rt.Routes()
	if len(got) != 1 || got[0].Pattern != "/public" || got[0].Tier != kernelauth.TierPublic {
		t.Fatalf("routes = %#v", got)
	}
	got[0].Pattern = "/mutated"
	if rt.Routes()[0].Pattern != "/public" {
		t.Fatal("Routes exposed mutable internal storage")
	}
}

func TestRouterRejectsInvalidPolicy(t *testing.T) {
	tests := []struct {
		name string
		opts RouteOpts
	}{
		{"invalid tier", RouteOpts{Tier: kernelauth.Tier(99)}},
		{"negative body limit", RouteOpts{Tier: kernelauth.TierPublic, BodyMax: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("Handle accepted invalid route policy")
				}
			}()
			NewRouter(Authenticator{}, nil).Handle("/", tt.opts, func(http.ResponseWriter, *http.Request) {})
		})
	}
}
