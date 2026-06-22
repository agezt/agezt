// SPDX-License-Identifier: MIT

package restapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestUpdateCheck_DisabledServiceReturnsStatus(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	rec := do(t, s, http.MethodGet, "/api/v1/update", "", "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/update = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"up_to_date":true`) || !strings.Contains(body, "update is disabled") {
		t.Fatalf("GET /api/v1/update body = %s", body)
	}
}

func TestUpdateCheck_RequiresAuth(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	if rec := do(t, s, http.MethodGet, "/api/v1/update", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/v1/update without token = %d, want 401", rec.Code)
	}
}

func TestUpdateCheck_MethodGuard(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	if rec := do(t, s, http.MethodPost, "/api/v1/update", "{}", "secret"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/v1/update = %d, want 405", rec.Code)
	}
}

func TestUpdateApply_DisabledServiceIsBadRequest(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	rec := do(t, s, http.MethodPost, "/api/v1/update/apply", `{"version":"1.2.3","sha256":"abc","url":"https://example.com/agezt"}`, "secret")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/v1/update/apply = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "update_disabled") {
		t.Fatalf("POST /api/v1/update/apply body = %s", rec.Body.String())
	}
}

func TestUpdateApply_RequiresAuth(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	if rec := do(t, s, http.MethodPost, "/api/v1/update/apply", `{}`, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST /api/v1/update/apply without token = %d, want 401", rec.Code)
	}
}
