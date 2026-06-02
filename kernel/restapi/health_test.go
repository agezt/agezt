// SPDX-License-Identifier: MIT

package restapi

import (
	"strings"
	"testing"
)

// TestHealthz_UnauthenticatedLiveness — /healthz needs no token (deployment
// probes can't carry one) and reports liveness without leaking version/model
// (M134).
func TestHealthz_Unauthenticated(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m", models: []string{"m"}}, "secret")

	// No token → still 200.
	rec := do(t, s, "GET", "/healthz", "", "")
	if rec.Code != 200 {
		t.Fatalf("/healthz no-token = %d want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"status"`) || !strings.Contains(body, "ok") {
		t.Errorf("/healthz body = %q want status ok", body)
	}
	// Must NOT leak version/model on the unauth endpoint.
	if strings.Contains(body, "9.9.9") || strings.Contains(body, `"model`) {
		t.Errorf("/healthz leaked version/model: %q", body)
	}
	// HEAD is accepted (monitors use it).
	if rec := do(t, s, "HEAD", "/healthz", "", ""); rec.Code != 200 {
		t.Errorf("HEAD /healthz = %d want 200", rec.Code)
	}
}

// TestReadyz_ReflectsReadiness — /readyz is unauthenticated and reports 200 when
// ready, 503 when the injected probe says not-ready (e.g. halted) (M134).
func TestReadyz_ReflectsReadiness(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")

	// No readiness probe set → ready (the server answering proves liveness).
	if rec := do(t, s, "GET", "/readyz", "", ""); rec.Code != 200 {
		t.Fatalf("/readyz default = %d want 200", rec.Code)
	}

	// Halted → 503 with a reason.
	halted := false
	s.SetReadiness(func() (bool, string) {
		if halted {
			return false, "halted"
		}
		return true, ""
	})
	if rec := do(t, s, "GET", "/readyz", "", ""); rec.Code != 200 {
		t.Errorf("/readyz ready = %d want 200", rec.Code)
	}
	halted = true
	rec := do(t, s, "GET", "/readyz", "", "")
	if rec.Code != 503 {
		t.Errorf("/readyz halted = %d want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "halted") {
		t.Errorf("/readyz halted body = %q want reason 'halted'", rec.Body.String())
	}
}

// TestApiV1Health_StillRequiresToken — the authed /api/v1/health (which DOES
// expose version/model) is unchanged: no token → 401 (M134 regression guard).
func TestApiV1Health_StillRequiresToken(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m", models: []string{"m"}}, "secret")
	if rec := do(t, s, "GET", "/api/v1/health", "", ""); rec.Code != 401 {
		t.Errorf("/api/v1/health no-token = %d want 401", rec.Code)
	}
	if rec := do(t, s, "GET", "/api/v1/health", "", "secret"); rec.Code != 200 {
		t.Errorf("/api/v1/health with-token = %d want 200", rec.Code)
	}
}
