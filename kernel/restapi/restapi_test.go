// SPDX-License-Identifier: MIT

package restapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/meshctx"
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
	ranHop    int // mesh hop observed in the Run context (M339)
}

func (f *fakeEngine) NewCorrelation() string        { return "run-test" }
func (f *fakeEngine) SubjectForRun(c string) string { return "agent.agent-" + c + ".llm" }
func (f *fakeEngine) DefaultModel() string          { return f.model }
func (f *fakeEngine) ModelIDs() []string            { return f.models }
func (f *fakeEngine) RunModel(ctx context.Context, corr, intent, model string, _ []string, _ bool) (string, error) {
	f.ranIntent = intent
	f.ranModel = model
	f.ranHop = meshctx.Hop(ctx)
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

type fakeArtifactEngine struct {
	*fakeEngine
	artifacts             []ArtifactEntry
	gotKind, gotSource    string
	gotCorrelationID      string
	artifactLookupErr     error
	artifactLookupInvoked bool
	artifactBytes         map[string][]byte
}

func (f *fakeArtifactEngine) ArtifactEntries(kind, source, corr string) ([]ArtifactEntry, error) {
	f.artifactLookupInvoked = true
	f.gotKind = kind
	f.gotSource = source
	f.gotCorrelationID = corr
	if f.artifactLookupErr != nil {
		return nil, f.artifactLookupErr
	}
	return f.artifacts, nil
}
func (f *fakeArtifactEngine) ArtifactBytes(id string, maxBytes int64) ([]byte, ArtifactEntry, error) {
	for _, a := range f.artifacts {
		if a.ID != id {
			continue
		}
		if maxBytes > 0 && a.Size > maxBytes {
			return nil, a, ErrArtifactTooLarge
		}
		data, ok := f.artifactBytes[id]
		if !ok {
			return nil, ArtifactEntry{}, ErrArtifactNotFound
		}
		return data, a, nil
	}
	return nil, ArtifactEntry{}, ErrArtifactNotFound
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

// V-011: the shared mailbox/board and the host-global self-update endpoints are
// daemon-global and not tenant-partitioned, so a per-tenant token must NOT reach
// them (it could otherwise read/spoof across tenants on the one shared board).
// The admin token still authorizes them; tenant tokens get 401. Per-tenant runs
// remain allowed (those are tenant-bound via the resolver).
func TestAdminOnlyRoutes_RejectTenantToken(t *testing.T) {
	eng := &fakeEngine{model: "m"}
	s := newServer(t, eng, "admin-tok")
	st, err := board.Open(t.TempDir())
	if err != nil {
		t.Fatalf("board.Open: %v", err)
	}
	s.SetMailbox(st, func(board.Message, string) {})
	s.SetTenantAuthorizer(func(id, presented string) bool {
		return id == "alpha" && presented == "alpha-tok"
	})

	req := func(method, path, token, tenant string) int {
		r := httptest.NewRequest(method, path, nil)
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

	adminOnly := []string{
		"/api/v1/mailbox/inbox?name=x",
		"/api/v1/mailbox/topics",
		"/api/v1/update",
	}
	for _, path := range adminOnly {
		// A valid tenant token targeting its own tenant is still rejected here.
		if got := req(http.MethodGet, path, "alpha-tok", "alpha"); got != http.StatusUnauthorized {
			t.Errorf("%s with tenant token: status = %d, want 401", path, got)
		}
		// The admin token is accepted (it reaches the handler — not a 401).
		if got := req(http.MethodGet, path, "admin-tok", ""); got == http.StatusUnauthorized {
			t.Errorf("%s with admin token: got 401, want it to pass auth", path)
		}
	}

	// A tenant-scoped route (health) still accepts the tenant token — the gate is
	// specific to the daemon-global surfaces, not a blanket tenant lockout.
	if got := req(http.MethodGet, "/api/v1/health", "alpha-tok", "alpha"); got != http.StatusOK {
		t.Errorf("tenant token on /health: status = %d, want 200", got)
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
	r := httptest.NewRequest(http.MethodGet, "/api/v1/health?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("query token must not authorize, got %d", rec.Code)
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

func TestArtifacts_MetadataOnlyWithFiltersAndLimit(t *testing.T) {
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	eng := &fakeArtifactEngine{
		fakeEngine: &fakeEngine{model: "m", b: b},
		artifacts: []ArtifactEntry{
			{ID: "art-2", Ref: strings.Repeat("b", 64), Name: "second.png", Mime: "image/png", Kind: "image", Source: "run", Corr: "run-abc", Size: 20, CreatedMs: 2000},
			{ID: "art-1", Ref: strings.Repeat("a", 64), Name: "first.txt", Mime: "text/plain", Kind: "file", Source: "run", Corr: "run-abc", Size: 10, CreatedMs: 1000, Caption: "note"},
		},
	}
	s := New(eng, b, "secret", "9.9.9")

	rec := do(t, s, http.MethodGet, "/api/v1/artifacts?kind=image&source=run&corr=run-abc&limit=1", "", "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !eng.artifactLookupInvoked || eng.gotKind != "image" || eng.gotSource != "run" || eng.gotCorrelationID != "run-abc" {
		t.Fatalf("artifact filters not passed through: kind=%q source=%q corr=%q invoked=%v",
			eng.gotKind, eng.gotSource, eng.gotCorrelationID, eng.artifactLookupInvoked)
	}
	var out struct {
		Count      int             `json:"count"`
		TotalCount int             `json:"total_count"`
		Truncated  bool            `json:"truncated"`
		Entries    []ArtifactEntry `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Count != 1 || out.TotalCount != 2 || !out.Truncated || len(out.Entries) != 1 {
		t.Fatalf("artifact list = %+v, want one truncated entry out of two", out)
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "data") || strings.Contains(raw, "secret") {
		t.Fatalf("artifact metadata endpoint leaked bytes/token-like fields: %s", raw)
	}
}

func TestArtifacts_UnavailableWhenEngineDoesNotList(t *testing.T) {
	eng := &fakeEngine{model: "m"}
	s := newServer(t, eng, "secret")
	rec := do(t, s, http.MethodGet, "/api/v1/artifacts", "", "secret")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s, want 503", rec.Code, rec.Body.String())
	}
}

func TestArtifactBytes_DefaultDenyThenAllow(t *testing.T) {
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	entry := ArtifactEntry{ID: "art-1", Ref: strings.Repeat("a", 64), Name: "result.txt", Mime: "text/plain", Kind: "tool-output", Corr: "run-abc", Size: 11, CreatedMs: 1000}
	eng := &fakeArtifactEngine{
		fakeEngine:    &fakeEngine{model: "m", b: b},
		artifacts:     []ArtifactEntry{entry},
		artifactBytes: map[string][]byte{"art-1": []byte("hello world")},
	}
	s := New(eng, b, "secret", "9.9.9")

	t.Setenv("AGEZT_REMOTE_ARTIFACT_BYTES", "")
	rec := do(t, s, http.MethodGet, "/api/v1/artifacts/art-1/bytes", "", "secret")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("default artifact bytes status=%d body=%s, want 403", rec.Code, rec.Body.String())
	}

	t.Setenv("AGEZT_REMOTE_ARTIFACT_BYTES", "allow")
	rec = do(t, s, http.MethodGet, "/api/v1/artifacts/art-1/bytes", "", "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("allowed artifact bytes status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Entry ArtifactEntry `json:"entry"`
		Size  int           `json:"size"`
		Data  string        `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, err := base64.StdEncoding.DecodeString(out.Data)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if out.Entry.ID != "art-1" || out.Size != len("hello world") || string(data) != "hello world" {
		t.Fatalf("artifact bytes response = %+v data=%q", out, data)
	}
}

// TestSubmitRun_MeshHopLimit pins the federation loop guard (M209): a run arriving
// with a mesh hop count above this node's limit is refused with 508 Loop Detected,
// while a run at EXACTLY the limit is still accepted (the bound is inclusive) and its
// hop threads into the run context. The hop limit is the classic off-by-one surface
// and had no REST-layer test at all, so mutation testing (M513) left both
// `hopIn > maxHops → >= maxHops` (would refuse a run at the limit) and `→ < maxHops`
// (would stop refusing the loop entirely — a federation could recurse forever) alive.
func TestSubmitRun_MeshHopLimit(t *testing.T) {
	t.Setenv(meshctx.EnvMaxHops, "2") // deterministic limit, independent of the default
	if got := meshctx.MaxHopsFromEnv(); got != 2 {
		t.Fatalf("setup: maxHops = %d, want 2", got)
	}
	submit := func(hop string) (int, *fakeEngine) {
		eng := &fakeEngine{model: "m", answer: "ok"}
		s := newServer(t, eng, "secret")
		r := httptest.NewRequest(http.MethodPost, "/api/v1/runs", strings.NewReader(`{"intent":"hi"}`))
		r.Header.Set("Authorization", "Bearer secret")
		r.Header.Set(meshctx.HopHeader, hop)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, r)
		return rec.Code, eng
	}

	// One past the limit (3 > 2) → refused with 508.
	if code, _ := submit("3"); code != http.StatusLoopDetected {
		t.Errorf("hop 3 (limit 2): status = %d, want %d (508 loop detected)", code, http.StatusLoopDetected)
	}
	// Exactly at the limit (2) → accepted, and the hop threads into the run context.
	if code, eng := submit("2"); code != http.StatusOK {
		t.Errorf("hop 2 (limit 2, inclusive): status = %d, want 200", code)
	} else if eng.ranHop != 2 {
		t.Errorf("hop not threaded into the run context: ranHop = %d, want 2", eng.ranHop)
	}
}
