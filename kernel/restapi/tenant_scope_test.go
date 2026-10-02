// SPDX-License-Identifier: MIT

package restapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agezt/agezt/kernel/bus"
)

// A per-tenant token reached /metrics, /api/v1/health and /api/v1/models, and
// all three answered from the PRIMARY kernel: a tenant read the daemon-wide
// spend and activity counters and the operator's model set.
//   - /metrics is daemon-wide by nature → admin token only (like the mailbox
//     and update routes, V-011).
//   - health/models answer from the engine the request is bound to.
func TestTenantToken_ScopedToItsOwnTenant(t *testing.T) {
	primary := &fakeEngine{model: "operator-model", models: []string{"operator-model", "operator-secret-model"}}
	s := newServer(t, primary, "admin-tok")
	alpha := &fakeEngine{model: "alpha-model", models: []string{"alpha-model"}}
	s.SetTenantResolver(func(id string) (Engine, *bus.Bus, error) { return alpha, nil, nil })
	s.SetTenantAuthorizer(func(id, presented string) bool { return id == "alpha" && presented == "alpha-tok" })
	s.SetMetrics(func() []Metric { return []Metric{{Name: "spend_microcents_today", Value: 42}} })

	get := func(path, token, tenant string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		if tenant != "" {
			r.Header.Set("X-Agezt-Tenant", tenant)
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, r)
		return rec
	}

	if rec := get("/metrics", "alpha-tok", "alpha"); rec.Code != http.StatusUnauthorized {
		t.Errorf("/metrics with a tenant token: status=%d body=%q, want 401", rec.Code, rec.Body.String())
	}
	if rec := get("/metrics", "admin-tok", ""); rec.Code != http.StatusOK {
		t.Errorf("/metrics with the admin token: status=%d, want 200", rec.Code)
	}

	var models struct {
		Default string   `json:"default"`
		Models  []string `json:"models"`
	}
	rec := get("/api/v1/models", "alpha-tok", "alpha")
	_ = json.Unmarshal(rec.Body.Bytes(), &models)
	if rec.Code != http.StatusOK || models.Default != "alpha-model" || len(models.Models) != 1 {
		t.Errorf("/api/v1/models for tenant alpha: status=%d default=%q models=%v, want alpha's own set",
			rec.Code, models.Default, models.Models)
	}

	var health map[string]any
	rec = get("/api/v1/health", "alpha-tok", "alpha")
	_ = json.Unmarshal(rec.Body.Bytes(), &health)
	if health["default_model"] != "alpha-model" {
		t.Errorf("/api/v1/health for tenant alpha: default_model=%v, want alpha-model", health["default_model"])
	}

	// The admin token without a tenant header still sees the primary.
	rec = get("/api/v1/models", "admin-tok", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &models)
	if models.Default != "operator-model" {
		t.Errorf("primary /api/v1/models default=%q, want operator-model", models.Default)
	}
}
