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

// TestUpdateApply_TheDoorAndTheEndpointsOwnRefusals holds the self-update
// endpoint's two voices together: the door's (the operation registry's
// sentence, for a body that does not fit the contract's row) and the
// endpoint's own (for an empty body, a missing field, a disabled service).
func TestUpdateApply_TheDoorAndTheEndpointsOwnRefusals(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	s.SetUpdateService(doorUpdateBackend{})

	cases := []struct {
		name string
		body string
		want string
		// inBody looks the refusal up in the whole answer (the endpoint's
		// own refusals name their kind in the envelope's "type"); otherwise
		// it is the message the door wrote.
		inBody bool
	}{
		{"a mistyped field", `{"version":42,"sha256":"abc","url":"u"}`,
			"operation update_apply input: $.version has wrong type: got number, want string", false},
		{"a null field", `{"version":"1.2.3","sha256":"abc","url":"u","notes":null}`,
			"operation update_apply input: $.notes has wrong type: got null, want string", false},
		{"a body that is not an object", `[]`,
			"operation update_apply input: $ has wrong type: got array, want object", false},
		{"an empty body", ``, "empty_body", true},
		{"a missing field", `{"version":"1.2.3"}`, "missing_fields", true},
	}
	for _, c := range cases {
		rec := do(t, s, http.MethodPost, "/api/v1/update/apply", c.body, "secret")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s answered %d %s, want 400", c.name, rec.Code, rec.Body.String())
			continue
		}
		got := doorErrorMessage(t, rec)
		if c.inBody {
			got = rec.Body.String()
		}
		if !strings.Contains(got, c.want) {
			t.Errorf("%s said %q, want it to contain %q", c.name, got, c.want)
		}
	}

	// A body that fits the contract still reaches the handler.
	rec := do(t, s, http.MethodPost, "/api/v1/update/apply",
		`{"version":"1.2.3","sha256":"abc","url":"u","notes":"n"}`, "secret")
	if rec.Code != http.StatusOK {
		t.Errorf("a valid body answered %d %s, want 200", rec.Code, rec.Body.String())
	}
}

// TestUpdateApply_TheDisabledServiceStillAnswersFirst: the endpoint's early
// answer is still early — a daemon with the self-update disabled says so
// without reading a body.
func TestUpdateApply_TheDisabledServiceStillAnswersFirst(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	rec := do(t, s, http.MethodPost, "/api/v1/update/apply", `{"version":42,"sha256":"abc","url":"u"}`, "secret")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a mistyped body answered %d %s, want 400", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "update_disabled") {
		t.Fatalf("a mistyped body answered %s, want the disabled answer", rec.Body.String())
	}
}
