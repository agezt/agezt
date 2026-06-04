// SPDX-License-Identifier: MIT

package openaiapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// retrieveModel issues an authed GET /v1/models/<path> and returns the recorder.
func retrieveModel(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// TestRetrieveModel_KnownReturnsModelObject: GET /v1/models/{id} for a routable
// id (default model or a catalog id, incl. a provider-prefixed id with a slash)
// returns the OpenAI model object. This is what client.models.retrieve(id) calls.
func TestRetrieveModel_KnownReturnsModelObject(t *testing.T) {
	eng := &fakeEngine{model: "MiniMax-M2.7", models: []string{"gpt-4o", "anthropic/claude-3-5"}}
	s := newAPIServer(t, eng, "secret")

	for _, id := range []string{"MiniMax-M2.7", "gpt-4o", "anthropic/claude-3-5"} {
		rec := retrieveModel(t, s, "/v1/models/"+id)
		if rec.Code != http.StatusOK {
			t.Fatalf("id %q: status=%d body=%s", id, rec.Code, rec.Body.String())
		}
		var out struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("id %q: %v", id, err)
		}
		if out.ID != id || out.Object != "model" || out.OwnedBy != "agezt" {
			t.Errorf("id %q: got %+v", id, out)
		}
	}
}

// TestRetrieveModel_UnknownIs404OpenAIShape: an id the engine can't route returns
// 404 with an OpenAI-shaped {error:{message,type}} body — so an SDK distinguishes
// "unknown model" from "endpoint missing".
func TestRetrieveModel_UnknownIs404OpenAIShape(t *testing.T) {
	eng := &fakeEngine{model: "MiniMax-M2.7", models: []string{"gpt-4o"}}
	s := newAPIServer(t, eng, "secret")

	rec := retrieveModel(t, s, "/v1/models/no-such-model")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d (want 404)", rec.Code)
	}
	var out struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Error.Type != "invalid_request_error" || out.Error.Message == "" {
		t.Errorf("error body = %+v", out.Error)
	}
}

// TestRetrieveModel_EmptyIdIs404: GET /v1/models/ (trailing slash, no id) is not
// a valid retrieve and must 404 rather than fall through to the list or panic.
func TestRetrieveModel_EmptyIdIs404(t *testing.T) {
	eng := &fakeEngine{model: "MiniMax-M2.7", models: []string{"gpt-4o"}}
	s := newAPIServer(t, eng, "secret")

	rec := retrieveModel(t, s, "/v1/models/")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status=%d (want 404)", rec.Code)
	}
}

// TestRetrieveModel_ListRouteStillExactMatch: the subtree handler must not shadow
// the list — GET /v1/models (no trailing slash) still returns the list object.
func TestRetrieveModel_ListRouteStillExactMatch(t *testing.T) {
	eng := &fakeEngine{model: "MiniMax-M2.7", models: []string{"gpt-4o"}}
	s := newAPIServer(t, eng, "secret")

	rec := retrieveModel(t, s, "/v1/models")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var out struct {
		Object string `json:"object"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Object != "list" {
		t.Errorf("object=%q (want list)", out.Object)
	}
}

// TestRetrieveModel_RejectsNonGET: a non-GET to the retrieve route is 405 with
// Allow: GET (parity with the rest of the OpenAI surface).
func TestRetrieveModel_RejectsNonGET(t *testing.T) {
	eng := &fakeEngine{model: "MiniMax-M2.7", models: []string{"gpt-4o"}}
	s := newAPIServer(t, eng, "secret")

	req := httptest.NewRequest(http.MethodPost, "/v1/models/gpt-4o", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d (want 405)", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Errorf("Allow=%q", got)
	}
}

// TestRetrieveModel_RequiresAuth: the retrieve route is behind the same auth as
// the rest of the surface — an unauthenticated GET is 401, never a model object.
func TestRetrieveModel_RequiresAuth(t *testing.T) {
	eng := &fakeEngine{model: "MiniMax-M2.7", models: []string{"gpt-4o"}}
	s := newAPIServer(t, eng, "secret")

	req := httptest.NewRequest(http.MethodGet, "/v1/models/gpt-4o", nil)
	// no Authorization header
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d (want 401)", rec.Code)
	}
}
