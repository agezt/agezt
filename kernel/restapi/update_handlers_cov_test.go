// SPDX-License-Identifier: MIT

package restapi

import (
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/update"
)

// ghRT serves a canned GitHub-API response so the wired update.Service can run
// its Check without touching the network.
type ghRT struct {
	status int
	body   string
}

func (rt ghRT) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: rt.status,
		Body:       io.NopCloser(strings.NewReader(rt.body)),
		Header:     make(http.Header),
	}, nil
}

func wireUpdate(s *Server, rt http.RoundTripper) {
	svc := update.New(update.Config{
		Source:      update.SourceGitHub,
		GitHubOwner: "agezt",
		GitHubRepo:  "agezt",
		HTTPClient:  &http.Client{Transport: rt},
	})
	s.SetUpdateService(svc) // exercises SetUpdateService (was 0%)
}

func TestUpdateCheck_ServiceWired_UpToDate(t *testing.T) {
	orig := update.CurrentVersion
	update.CurrentVersion = "9.9.9"
	defer func() { update.CurrentVersion = orig }()

	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	// tag matches current version → up to date.
	wireUpdate(s, ghRT{status: http.StatusOK, body: `{"tag_name":"v9.9.9"}`})

	rec := do(t, s, http.MethodGet, "/api/v1/update", "", "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/update = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"up_to_date":true`) {
		t.Fatalf("expected up_to_date true, got %s", rec.Body.String())
	}
}

func TestUpdateCheck_ServiceWired_UpdateAvailable(t *testing.T) {
	orig := update.CurrentVersion
	update.CurrentVersion = "1.0.0"
	defer func() { update.CurrentVersion = orig }()

	osName := runtime.GOOS
	if osName == "darwin" {
		osName = "macos"
	}
	asset := "agezt-" + osName + "_" + runtime.GOARCH
	body := `{"tag_name":"v2.0.0","body":"notes","assets":[{"name":"` + asset +
		`","browser_download_url":"https://example.com/` + asset + `"}]}`

	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	wireUpdate(s, ghRT{status: http.StatusOK, body: body})

	rec := do(t, s, http.MethodGet, "/api/v1/update", "", "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/update = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	got := rec.Body.String()
	if !strings.Contains(got, `"up_to_date":false`) || !strings.Contains(got, `"2.0.0"`) {
		t.Fatalf("expected update to 2.0.0, got %s", got)
	}
}

func TestUpdateCheck_ServiceWired_CheckError(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	// A 500 from GitHub makes Check return an error → 500 from the handler.
	wireUpdate(s, ghRT{status: http.StatusInternalServerError, body: "boom"})

	rec := do(t, s, http.MethodGet, "/api/v1/update", "", "secret")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("GET /api/v1/update on Check error = %d, want 500; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "update_check_failed") {
		t.Fatalf("expected update_check_failed, got %s", rec.Body.String())
	}
}

func TestUpdateApply_ServiceWired_MethodGuard(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	wireUpdate(s, ghRT{status: http.StatusOK, body: `{"tag_name":"v9.9.9"}`})
	// GET on the apply endpoint must be rejected.
	if rec := do(t, s, http.MethodGet, "/api/v1/update/apply", "", "secret"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/v1/update/apply = %d, want 405", rec.Code)
	}
}

func TestUpdateApply_ServiceWired_BadJSON(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "secret")
	wireUpdate(s, ghRT{status: http.StatusOK, body: `{"tag_name":"v9.9.9"}`})
	// Malformed JSON body → 400.
	rec := do(t, s, http.MethodPost, "/api/v1/update/apply", "{not json", "secret")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/v1/update/apply bad JSON = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}
