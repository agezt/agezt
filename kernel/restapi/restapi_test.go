// SPDX-License-Identifier: MIT

package restapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// fakeEngine implements Engine. RunModel publishes its tokens on the bus under
// the run subject (exercising the real SSE path) then returns its answer.
type fakeEngine struct {
	b         *bus.Bus
	answer    string
	tokens    []string
	model     string
	models    []string
	events    []*event.Event
	ranIntent string
	ranModel  string
}

func (f *fakeEngine) NewCorrelation() string        { return "run-test" }
func (f *fakeEngine) SubjectForRun(c string) string { return "agent.agent-" + c + ".llm" }
func (f *fakeEngine) DefaultModel() string          { return f.model }
func (f *fakeEngine) ModelIDs() []string            { return f.models }
func (f *fakeEngine) RunModel(_ context.Context, corr, intent, model string, _ []string, _ bool) (string, error) {
	f.ranIntent = intent
	f.ranModel = model
	for _, tok := range f.tokens {
		_, _ = f.b.PublishStreaming(event.Spec{
			Subject:       f.SubjectForRun(corr),
			Kind:          event.KindLLMToken,
			Actor:         "agent-" + corr,
			CorrelationID: corr,
			Payload:       map[string]any{"text": tok},
		})
	}
	return f.answer, nil
}
func (f *fakeEngine) EventsForCorrelation(corr string) ([]*event.Event, error) {
	return f.events, nil
}

func newServer(t *testing.T, eng *fakeEngine, token string) *Server {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	eng.b = b
	return New(eng, b, token, "9.9.9")
}

func do(t *testing.T, s *Server, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)
	return rec
}

func TestRun_TenantRouting(t *testing.T) {
	// Primary engine + a separate tenant engine with its own bus.
	primary := &fakeEngine{answer: "primary-answer", model: "m"}
	s := newServer(t, primary, "secret")

	tj, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	tbus := bus.New(tj)
	t.Cleanup(func() { tbus.Close(); tj.Close() })
	alpha := &fakeEngine{answer: "alpha-answer", model: "m", b: tbus}

	s.SetTenantResolver(func(id string) (Engine, *bus.Bus, error) {
		if id == "alpha" {
			return alpha, tbus, nil
		}
		return nil, nil, errors.New("unknown tenant " + id)
	})

	// No header → primary engine.
	rec := do(t, s, http.MethodPost, "/api/v1/runs", `{"intent":"hello"}`, "secret")
	if rec.Code != http.StatusOK || primary.ranIntent != "hello" {
		t.Fatalf("primary route: code=%d ran=%q", rec.Code, primary.ranIntent)
	}
	if alpha.ranIntent != "" {
		t.Error("tenant engine should not have run for a header-less request")
	}

	// X-Agezt-Tenant: alpha → tenant engine.
	r := httptest.NewRequest(http.MethodPost, "/api/v1/runs", strings.NewReader(`{"intent":"for-alpha"}`))
	r.Header.Set("Authorization", "Bearer secret")
	r.Header.Set("X-Agezt-Tenant", "alpha")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("tenant route status=%d", rec.Code)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["answer"] != "alpha-answer" {
		t.Errorf("answer = %v, want alpha-answer", out["answer"])
	}
	if alpha.ranIntent != "for-alpha" {
		t.Errorf("tenant engine ran %q, want for-alpha", alpha.ranIntent)
	}
	if primary.ranIntent != "hello" {
		t.Error("primary engine must not have run the tenant request")
	}

	// Unknown tenant → 400 from the resolver error.
	r = httptest.NewRequest(http.MethodPost, "/api/v1/runs", strings.NewReader(`{"intent":"x"}`))
	r.Header.Set("Authorization", "Bearer secret")
	r.Header.Set("X-Agezt-Tenant", "ghost")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown tenant status = %d, want 400", rec.Code)
	}
}

// A per-tenant token authorizes ONLY its own tenant; the admin token authorizes
// anything; a tenant token is useless without (or with the wrong) tenant header.
func TestTenantAuth(t *testing.T) {
	eng := &fakeEngine{model: "m"}
	s := newServer(t, eng, "admin-tok")
	s.SetTenantAuthorizer(func(id, presented string) bool {
		return id == "alpha" && presented == "alpha-tok"
	})

	// req builds a GET /health with optional bearer token + tenant header.
	req := func(token, tenant string) int {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if tenant != "" {
			r.Header.Set("X-Agezt-Tenant", tenant)
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, r)
		return rec.Code
	}

	cases := []struct {
		name          string
		token, tenant string
		want          int
	}{
		{"admin no tenant", "admin-tok", "", http.StatusOK},
		{"admin any tenant", "admin-tok", "alpha", http.StatusOK},
		{"tenant token own tenant", "alpha-tok", "alpha", http.StatusOK},
		{"tenant token wrong tenant", "alpha-tok", "beta", http.StatusUnauthorized},
		{"tenant token no header", "alpha-tok", "", http.StatusUnauthorized},
		{"unknown token with header", "nope", "alpha", http.StatusUnauthorized},
		{"no token", "", "alpha", http.StatusUnauthorized},
	}
	for _, c := range cases {
		if got := req(c.token, c.tenant); got != c.want {
			t.Errorf("%s: status = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestHealth(t *testing.T) {
	eng := &fakeEngine{model: "m", models: []string{"a", "b"}}
	s := newServer(t, eng, "secret")
	rec := do(t, s, http.MethodGet, "/api/v1/health", "", "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["status"] != "ok" || out["version"] != "9.9.9" || out["default_model"] != "m" {
		t.Errorf("health = %v", out)
	}
}

func TestModels(t *testing.T) {
	eng := &fakeEngine{model: "m", models: []string{"m", "x"}} // m duplicated
	s := newServer(t, eng, "secret")
	rec := do(t, s, http.MethodGet, "/api/v1/models", "", "secret")
	var out struct {
		Default string   `json:"default"`
		Models  []string `json:"models"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Default != "m" {
		t.Errorf("default=%q", out.Default)
	}
	if len(out.Models) != 2 { // m, x — no dup
		t.Errorf("models=%v", out.Models)
	}
}

func TestAuthRequired(t *testing.T) {
	eng := &fakeEngine{model: "m"}
	s := newServer(t, eng, "secret")
	if rec := do(t, s, http.MethodGet, "/api/v1/health", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: %d", rec.Code)
	}
	if rec := do(t, s, http.MethodGet, "/api/v1/health", "", "wrong"); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: %d", rec.Code)
	}
}

func TestEmptyTokenFailsClosed(t *testing.T) {
	eng := &fakeEngine{model: "m"}
	s := newServer(t, eng, "") // no token configured
	r := httptest.NewRequest(http.MethodGet, "/api/v1/health?token=", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("empty token must fail closed, got %d", rec.Code)
	}
}

func TestSubmitRun_NonStreaming(t *testing.T) {
	eng := &fakeEngine{model: "default-m", answer: "the answer"}
	s := newServer(t, eng, "secret")
	rec := do(t, s, http.MethodPost, "/api/v1/runs", `{"intent":"do a thing","model":"gpt-4o"}`, "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Corr   string `json:"correlation_id"`
		Model  string `json:"model"`
		Status string `json:"status"`
		Answer string `json:"answer"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Status != "completed" || out.Answer != "the answer" || out.Corr == "" {
		t.Errorf("run result = %+v", out)
	}
	if out.Model != "gpt-4o" || eng.ranModel != "gpt-4o" {
		t.Errorf("per-request model not honoured: resp=%q ran=%q", out.Model, eng.ranModel)
	}
	if eng.ranIntent != "do a thing" {
		t.Errorf("intent=%q", eng.ranIntent)
	}
}

func TestSubmitRun_DefaultModel(t *testing.T) {
	eng := &fakeEngine{model: "default-m", answer: "ok"}
	s := newServer(t, eng, "secret")
	do(t, s, http.MethodPost, "/api/v1/runs", `{"intent":"hi"}`, "secret")
	if eng.ranModel != "default-m" {
		t.Errorf("ranModel=%q want default", eng.ranModel)
	}
}

func TestSubmitRun_EmptyIntent(t *testing.T) {
	eng := &fakeEngine{model: "m"}
	s := newServer(t, eng, "secret")
	rec := do(t, s, http.MethodPost, "/api/v1/runs", `{"intent":"  "}`, "secret")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty intent should be 400, got %d", rec.Code)
	}
}

func TestSubmitRun_Streaming(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "hello world", tokens: []string{"hello", " world"}}
	s := newServer(t, eng, "secret")
	rec := do(t, s, http.MethodPost, "/api/v1/runs", `{"intent":"hi","stream":true}`, "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type=%q", ct)
	}
	out := rec.Body.String()
	for _, want := range []string{
		"event: start", "event: token", `"text":"hello"`, `"text":" world"`,
		"event: done", `"answer":"hello world"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stream missing %q in:\n%s", want, out)
		}
	}
}

func TestInspectRun_ReturnsEvents(t *testing.T) {
	eng := &fakeEngine{
		model: "m",
		events: []*event.Event{
			{ID: "e1", Kind: event.KindTaskReceived, CorrelationID: "run-xyz", Subject: "agent.x.task"},
			{ID: "e2", Kind: event.KindTaskCompleted, CorrelationID: "run-xyz", Subject: "agent.x.task"},
		},
	}
	s := newServer(t, eng, "secret")
	rec := do(t, s, http.MethodGet, "/api/v1/runs/run-xyz", "", "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Corr   string `json:"correlation_id"`
		Count  int    `json:"count"`
		Events []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"events"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Corr != "run-xyz" || out.Count != 2 || len(out.Events) != 2 {
		t.Fatalf("inspect = %+v", out)
	}
	if out.Events[0].Kind != "task.received" || out.Events[1].Kind != "task.completed" {
		t.Errorf("events = %+v", out.Events)
	}
}

func TestInspectRun_NotFound(t *testing.T) {
	eng := &fakeEngine{model: "m", events: nil} // unknown correlation
	s := newServer(t, eng, "secret")
	rec := do(t, s, http.MethodGet, "/api/v1/runs/nope", "", "secret")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown run should 404, got %d", rec.Code)
	}
}

func TestRunsRoot_RejectsGET(t *testing.T) {
	eng := &fakeEngine{model: "m"}
	s := newServer(t, eng, "secret")
	rec := do(t, s, http.MethodGet, "/api/v1/runs", "", "secret")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET on /runs should be 405, got %d", rec.Code)
	}
}
