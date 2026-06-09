// SPDX-License-Identifier: MIT

package webui

import (
	"bufio"
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// fakeCaller records the commands the proxy issues and returns canned results.
type fakeCaller struct {
	calls    []string
	lastArgs map[string]any
	result   map[string]any
	err      error
}

func (f *fakeCaller) Call(_ context.Context, cmd string, args map[string]any) (map[string]any, error) {
	f.calls = append(f.calls, cmd)
	f.lastArgs = args
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

// Stream records the streamed command like Call but routes through the Stream
// path (used by Flow Studio's plan run). onEvent is exercised once so a
// regression that drops event relaying is observable.
func (f *fakeCaller) Stream(_ context.Context, cmd string, args map[string]any, onEvent func(*event.Event)) (map[string]any, error) {
	f.calls = append(f.calls, cmd)
	f.lastArgs = args
	if f.err != nil {
		return nil, f.err
	}
	if onEvent != nil {
		onEvent(&event.Event{Kind: event.KindNodeStarted})
	}
	return f.result, nil
}

func newServer(t *testing.T, client Caller, token string) (*Server, *bus.Bus) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	return New(b, client, token), b
}

// The embedded React SPA is served at "/": index.html with a #root mount and a
// reference to the hashed JS bundle under /assets/.
func TestSPAIndexServedAtRoot(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	req := httptest.NewRequest(http.MethodGet, "/?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content-type = %q want text/html", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="root"`) {
		t.Error("index.html missing the React #root mount")
	}
	if !strings.Contains(body, "/assets/") {
		t.Error("index.html missing a hashed /assets/ bundle reference")
	}
}

// index.html must be no-cache so a daemon upgrade (new asset hashes) isn't masked
// by a stale shell pointing at assets that no longer exist.
func TestIndexHTMLNoLongCache(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	req := httptest.NewRequest(http.MethodGet, "/?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("index.html Cache-Control = %q want no-cache", cc)
	}
}

// A client-side deep link (e.g. /runs after navigation + refresh) must serve the
// SPA shell, not 404 — the app routes client-side.
func TestSPADeepLinkServesIndex(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	req := httptest.NewRequest(http.MethodGet, "/runs?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("deep link status = %d want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `id="root"`) {
		t.Error("deep link did not serve the SPA shell")
	}
}

// The hashed bundle is served under /assets/ with a long immutable cache and an
// explicit, OS-independent Content-Type.
func TestSPAAssetsServed(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	name := firstAsset(t, s)
	req := httptest.NewRequest(http.MethodGet, "/assets/"+name+"?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("asset status = %d want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("asset Cache-Control = %q want immutable", cc)
	}
	// .js must be served as JS (not text/plain) or the browser refuses it under
	// nosniff — the Windows mime-registry bug this code guards against.
	if strings.HasSuffix(name, ".js") {
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
			t.Errorf("js asset Content-Type = %q want javascript", ct)
		}
	}
}

// The bundle is PUBLIC: the browser loads it as a subresource of index.html and
// cannot attach the token, so /assets/* must serve without one. (The data
// surfaces — /events, /api/* — stay token-gated; see TestAuthRequired.)
func TestSPAAssetsArePublic(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	name := firstAsset(t, s)
	req := httptest.NewRequest(http.MethodGet, "/assets/"+name, nil) // no token
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("no-token asset status = %d want 200 (assets are public)", rec.Code)
	}
	// Security headers still apply to public assets.
	if csp := rec.Header().Get("Content-Security-Policy"); csp == "" {
		t.Error("public asset missing Content-Security-Policy header")
	}
}

// The CSP is now static (no per-request nonce): it admits the external bundle via
// script-src 'self' and carries no nonce.
func TestSPACSPIsStaticNoNonce(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	req := httptest.NewRequest(http.MethodGet, "/?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("CSP missing script-src 'self': %q", csp)
	}
	if !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("CSP missing default-src 'none': %q", csp)
	}
	if strings.Contains(csp, "nonce-") || strings.Contains(csp, "__CSP_NONCE__") {
		t.Errorf("CSP unexpectedly carries a nonce: %q", csp)
	}
}

// The embedded bundle must actually be present (index.html + ≥1 asset) — the
// cross-OS safety net that doesn't depend on byte-reproducibility of the build.
func TestEmbeddedDistPresent(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")
	if _, err := fs.Stat(s.dist, "index.html"); err != nil {
		t.Fatalf("embedded dist missing index.html: %v", err)
	}
	if firstAsset(t, s) == "" {
		t.Fatal("embedded dist has no assets/* file")
	}
}

// firstAsset returns the name of one file under dist/assets/ in the embed.FS.
func firstAsset(t *testing.T, s *Server) string {
	t.Helper()
	entries, err := fs.ReadDir(s.dist, "assets")
	if err != nil {
		t.Fatalf("read embedded assets/: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			return e.Name()
		}
	}
	return ""
}

func TestStatsRouteProxiesRunsStats(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"total": 0}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/stats?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "runs_stats" {
		t.Errorf("expected one runs_stats call, got %v", fc.calls)
	}
}

func TestJournalRouteForwardsCorrelationOnly(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"events": []any{}}}
	s, _ := newServer(t, fc, "secret")
	// correlation_id is allowlisted; a stray param must be dropped.
	req := httptest.NewRequest(http.MethodGet,
		"/api/journal?token=secret&correlation_id=run-1&evil=rm", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "journal_grep" {
		t.Fatalf("expected one journal_grep call, got %v", fc.calls)
	}
	if fc.lastArgs["correlation_id"] != "run-1" {
		t.Errorf("correlation_id not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Errorf("non-allowlisted arg leaked through: %v", fc.lastArgs)
	}
}

func TestBudgetRouteProxiesBudget(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"spent_mc": 0}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/budget?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "budget" {
		t.Errorf("expected one budget call, got %v", fc.calls)
	}
}

func TestCacheRouteProxiesCacheStats(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"saved_microcents": 0}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/cache?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "cache_stats" {
		t.Errorf("expected one cache_stats call, got %v", fc.calls)
	}
}

func TestProvidersRouteProxiesProviderStats(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"routed": 0}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/providers?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "provider_stats" {
		t.Errorf("expected one provider_stats call, got %v", fc.calls)
	}
}

func TestCatalogRouteProxiesCatalogList(t *testing.T) {
	// The Chat model picker reads the full provider/model catalog via /api/catalog.
	fc := &fakeCaller{result: map[string]any{"providers": []any{}}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/catalog?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "catalog_list" {
		t.Errorf("expected one catalog_list call, got %v", fc.calls)
	}
}

func TestToolsRouteProxiesToolStats(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"total": 0}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/tools?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "tool_stats" {
		t.Errorf("expected one tool_stats call, got %v", fc.calls)
	}
}

func TestConfigRouteProxiesConfig(t *testing.T) {
	// The config inspector surfaces the daemon's resolved config (paths, model,
	// env PRESENCE only). It is a read-only proxy of the `config` command.
	fc := &fakeCaller{result: map[string]any{
		"model":      "mock",
		"tool_count": 3,
		"env":        map[string]any{"AGEZT_PROVIDER": true},
	}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/config?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "config" {
		t.Errorf("expected one config call, got %v", fc.calls)
	}
}

func TestPolicyRouteProxiesEdictStats(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"total": 0}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/policy?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "edict_stats" {
		t.Errorf("expected one edict_stats call, got %v", fc.calls)
	}
}

func TestPolicyLogRouteForwardsLimit(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"decisions": []any{}}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet,
		"/api/policy_log?token=secret&limit=40&evil=rm", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "edict_log" {
		t.Fatalf("expected one edict_log call, got %v", fc.calls)
	}
	if fc.lastArgs["limit"] != "40" {
		t.Errorf("limit not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Errorf("non-allowlisted arg leaked through: %v", fc.lastArgs)
	}
}

func TestToolLogRouteForwardsLimit(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"invocations": []any{}}}
	s, _ := newServer(t, fc, "secret")
	// The tool-log modal queries tool_log with limit; the limit arg is
	// allowlisted and must reach tool_log, while a stray param is dropped.
	req := httptest.NewRequest(http.MethodGet,
		"/api/tool_log?token=secret&limit=40&evil=rm", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "tool_log" {
		t.Fatalf("expected one tool_log call, got %v", fc.calls)
	}
	if fc.lastArgs["limit"] != "40" {
		t.Errorf("limit not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Errorf("non-allowlisted arg leaked through: %v", fc.lastArgs)
	}
}

func TestProviderLogRouteForwardsLimit(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"events": []any{}}}
	s, _ := newServer(t, fc, "secret")
	// The routing-log modal queries provider_log with limit; the limit arg is
	// allowlisted and must reach provider_log, while a stray param is dropped.
	req := httptest.NewRequest(http.MethodGet,
		"/api/provider_log?token=secret&limit=40&evil=rm", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "provider_log" {
		t.Fatalf("expected one provider_log call, got %v", fc.calls)
	}
	if fc.lastArgs["limit"] != "40" {
		t.Errorf("limit not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Errorf("non-allowlisted arg leaked through: %v", fc.lastArgs)
	}
}

// The Search view (M618): /api/journal_search forwards the full grep filter set
// (free-text pattern + kind/subject/actor/correlation/limit) and drops anything
// else.
func TestJournalSearchForwardsFullFilterSet(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"events": []any{}}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet,
		"/api/journal_search?token=secret&pattern=denied&kind=policy.decision&actor=agent-1&subject=governor&correlation_id=run-9&limit=50&evil=rm", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "journal_grep" {
		t.Fatalf("expected one journal_grep call, got %v", fc.calls)
	}
	for k, want := range map[string]string{
		"pattern": "denied", "kind": "policy.decision", "actor": "agent-1",
		"subject": "governor", "correlation_id": "run-9", "limit": "50",
	} {
		if fc.lastArgs[k] != want {
			t.Errorf("%s not forwarded: got %v want %q", k, fc.lastArgs[k], want)
		}
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Error("non-allowlisted arg leaked through journal_search")
	}
}

func TestJournalRouteForwardsKind(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"events": []any{}}}
	s, _ := newServer(t, fc, "secret")
	// The fallback-detail modal queries the journal by kind; the kind arg is
	// allowlisted and must reach journal_grep, while a stray param is dropped.
	req := httptest.NewRequest(http.MethodGet,
		"/api/journal?token=secret&kind=provider.fallback&limit=30&evil=rm", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "journal_grep" {
		t.Fatalf("expected one journal_grep call, got %v", fc.calls)
	}
	if fc.lastArgs["kind"] != "provider.fallback" {
		t.Errorf("kind not forwarded: %v", fc.lastArgs)
	}
	if fc.lastArgs["limit"] != "30" {
		t.Errorf("limit not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Errorf("non-allowlisted arg leaked through: %v", fc.lastArgs)
	}
}

func TestSchedulesRouteProxiesScheduleList(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"schedules": []any{}}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/schedules?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "schedule_list" {
		t.Errorf("expected one schedule_list call, got %v", fc.calls)
	}
}

func TestRunsRouteProxiesRunsList(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"runs": []any{}}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/runs?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "runs_list" {
		t.Errorf("expected one runs_list call, got %v", fc.calls)
	}
}

func TestAuthRequired(t *testing.T) {
	s, _ := newServer(t, &fakeCaller{}, "secret")

	cases := []struct {
		name, target string
		header       string
		want         int
	}{
		{"no token", "/api/status", "", http.StatusUnauthorized},
		{"wrong token", "/api/status?token=nope", "", http.StatusUnauthorized},
		{"query token", "/api/status?token=secret", "", http.StatusOK},
		{"bearer token", "/api/status", "Bearer secret", http.StatusOK},
		{"wrong bearer", "/api/status", "Bearer nope", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("status = %d want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestEmptyTokenNeverAuthorizes(t *testing.T) {
	// A server with no token must reject everything (fail closed).
	s, _ := newServer(t, &fakeCaller{}, "")
	req := httptest.NewRequest(http.MethodGet, "/?token=", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("empty-token server returned %d, want 401", rec.Code)
	}
}

func TestAPIProxiesControlPlane(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"agents": 3, "world_entities": 7}}
	s, _ := newServer(t, fc, "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/status?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "status" {
		t.Errorf("expected one CmdStatus call, got %v", fc.calls)
	}
	if !strings.Contains(rec.Body.String(), "world_entities") {
		t.Errorf("proxied body missing result: %s", rec.Body.String())
	}
}

func TestAPIReadOnly(t *testing.T) {
	// Every GET /api route must map to a read-only command — assert the proxy
	// never issues anything outside the known read set.
	readOnly := map[string]bool{
		"status": true, "config": true, "runs_list": true, "runs_stats": true, "budget": true, "cache_stats": true, "provider_stats": true, "tool_stats": true, "edict_stats": true, "schedule_list": true, "memory_list": true, "world_list": true,
		"skill_list": true, "standing_list": true, "inbox": true, "reflect_show": true, "approvals": true,
		"plan_stats": true, "edict_show": true, "tool_list": true, "board_read": true, "autonomy_feed": true,
		"catalog_list": true, "sandbox_list": true,
		"config_schema": true, "config_values": true, "routing_get": true, "persona_get": true,
	}
	for path := range apiRoutes {
		fc := &fakeCaller{result: map[string]any{"ok": true}}
		s, _ := newServer(t, fc, "secret")
		req := httptest.NewRequest(http.MethodGet, path+"?token=secret", nil)
		s.Handler().ServeHTTP(httptest.NewRecorder(), req)
		if len(fc.calls) != 1 {
			t.Fatalf("%s issued %d calls", path, len(fc.calls))
		}
		if !readOnly[fc.calls[0]] {
			t.Errorf("%s issued non-read command %q", path, fc.calls[0])
		}
	}
}

func TestWriteRequiresPOST(t *testing.T) {
	// A GET to a write route must NOT issue the mutating command (405).
	for path := range writeRoutes {
		fc := &fakeCaller{result: map[string]any{"ok": true}}
		s, _ := newServer(t, fc, "secret")
		req := httptest.NewRequest(http.MethodGet, path+"?token=secret", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s = %d want 405", path, rec.Code)
		}
		if len(fc.calls) != 0 {
			t.Errorf("GET %s must not issue a command, issued %v", path, fc.calls)
		}
	}
}

func TestWriteRequiresToken(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/halt", nil) // no token
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /api/halt without token = %d want 401", rec.Code)
	}
	if len(fc.calls) != 0 {
		t.Errorf("unauthorized write must not issue a command, issued %v", fc.calls)
	}
}

func TestHaltPOSTIssuesCommand(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"halted": true}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/halt?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "halt" {
		t.Errorf("expected one CmdHalt call, got %v", fc.calls)
	}
}

// The Activity monitor's per-run Cancel button: POST /api/cancel_run forwards
// the allowlisted correlation and nothing else.
func TestCancelRunForwardsCorrelation(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"correlation": "run-9", "cancelled": true}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodPost,
		"/api/cancel_run?token=secret&correlation=run-9&evil=x", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "cancel_run" {
		t.Fatalf("expected one CmdCancelRun call, got %v", fc.calls)
	}
	if fc.lastArgs["correlation"] != "run-9" {
		t.Errorf("correlation not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Error("non-allowlisted arg leaked into the cancel call")
	}
}

// The Budget view's runtime ceiling control (M607): POST /api/budget_set
// forwards the allowlisted ceiling_mc and nothing else.
func TestBudgetSetForwardsCeiling(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ceiling_mc": 2_000_000_000}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodPost,
		"/api/budget_set?token=secret&ceiling_mc=2000000000&evil=x", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "budget_set" {
		t.Fatalf("expected one CmdBudgetSet call, got %v", fc.calls)
	}
	if fc.lastArgs["ceiling_mc"] != "2000000000" {
		t.Errorf("ceiling_mc not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Error("non-allowlisted arg leaked into the budget_set call")
	}
}

// Policy control center (M610): the edict mutation routes forward only their
// allowlisted args and map to the right command, so an operator can grant/deny
// capabilities from the cockpit.
func TestEdictControlRoutes(t *testing.T) {
	for _, tc := range []struct {
		path, cmd string
		query     string
		wantArgs  map[string]string
	}{
		{"/api/edict/set_level", "edict_set_level", "capability=browser.read&level=L4&evil=x",
			map[string]string{"capability": "browser.read", "level": "L4"}},
		{"/api/edict/set_mode", "edict_set_mode", "mode=allow", map[string]string{"mode": "allow"}},
		{"/api/edict/deny_add", "edict_deny_add", "rule=secret", map[string]string{"rule": "secret"}},
		{"/api/edict/deny_rm", "edict_deny_rm", "name=runtime%5B0%5D", map[string]string{"name": "runtime[0]"}},
	} {
		fc := &fakeCaller{result: map[string]any{"ok": true}}
		s, _ := newServer(t, fc, "secret")
		req := httptest.NewRequest(http.MethodPost, tc.path+"?token=secret&"+tc.query, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s status = %d want 200", tc.path, rec.Code)
		}
		if len(fc.calls) != 1 || fc.calls[0] != tc.cmd {
			t.Errorf("%s issued %v want [%s]", tc.path, fc.calls, tc.cmd)
		}
		for k, v := range tc.wantArgs {
			if fc.lastArgs[k] != v {
				t.Errorf("%s arg %s = %v want %q", tc.path, k, fc.lastArgs[k], v)
			}
		}
		if _, leaked := fc.lastArgs["evil"]; leaked {
			t.Errorf("%s leaked a non-allowlisted arg", tc.path)
		}
	}
}

// The edict_show read route proxies the parameterless show command (GET).
func TestEdictShowRoute(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"levels": map[string]any{}, "ask_policy": "allow"}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/edict_show?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "edict_show" {
		t.Errorf("expected one edict_show call, got %v", fc.calls)
	}
}

// Live steering (M608): POST /api/run/steer forwards the allowlisted
// correlation + directive and drops anything else.
func TestRunSteerForwardsArgs(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"correlation": "run-7", "accepted": true}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodPost,
		"/api/run/steer?token=secret&correlation=run-7&directive=focus+on+X&evil=1", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "run_steer" {
		t.Fatalf("expected one CmdRunSteer call, got %v", fc.calls)
	}
	if fc.lastArgs["correlation"] != "run-7" || fc.lastArgs["directive"] != "focus on X" {
		t.Errorf("args not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Error("non-allowlisted arg leaked into the steer call")
	}
}

// The pause/resume/step routes each forward only the correlation and map to the
// right command.
func TestRunControlRoutesMapToCommands(t *testing.T) {
	for _, tc := range []struct{ path, cmd string }{
		{"/api/run/pause", "run_pause"},
		{"/api/run/resume", "run_resume"},
		{"/api/run/step", "run_step"},
	} {
		fc := &fakeCaller{result: map[string]any{"ok": true}}
		s, _ := newServer(t, fc, "secret")
		req := httptest.NewRequest(http.MethodPost, tc.path+"?token=secret&correlation=run-3", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s status = %d want 200", tc.path, rec.Code)
		}
		if len(fc.calls) != 1 || fc.calls[0] != tc.cmd {
			t.Errorf("%s issued %v want [%s]", tc.path, fc.calls, tc.cmd)
		}
		if fc.lastArgs["correlation"] != "run-3" {
			t.Errorf("%s did not forward correlation: %v", tc.path, fc.lastArgs)
		}
	}
}

// Cancel is a mutation — GET must be refused so a prefetch/crawler can't cancel
// a run.
func TestCancelRunRejectsGet(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/cancel_run?token=secret&correlation=r1", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d want 405", rec.Code)
	}
	if len(fc.calls) != 0 {
		t.Errorf("a GET must not cancel a run, got %v", fc.calls)
	}
}

func TestDecidePassesAllowlistedArgs(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret")
	// id + decision are allowlisted; a stray param must be dropped.
	req := httptest.NewRequest(http.MethodPost,
		"/api/decide?token=secret&id=ap1&decision=grant&evil=rm-rf", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "decide" {
		t.Fatalf("expected CmdDecide, got %v", fc.calls)
	}
	if _, ok := fc.lastArgs["evil"]; ok {
		t.Error("non-allowlisted arg leaked into the call")
	}
	if fc.lastArgs["id"] != "ap1" || fc.lastArgs["decision"] != "grant" {
		t.Errorf("allowlisted args not forwarded: %v", fc.lastArgs)
	}
}

func TestAPIErrorIsBadGateway(t *testing.T) {
	fc := &fakeCaller{err: errors.New("daemon down")}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/world?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("upstream error → status %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "daemon down") {
		t.Errorf("error body missing cause: %s", rec.Body.String())
	}
}

func TestEventsStreamsPublishedEvent(t *testing.T) {
	s, b := newServer(t, &fakeCaller{}, "secret")
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events?token=secret", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q want text/event-stream", ct)
	}

	reader := bufio.NewReader(resp.Body)
	// Drain the opening ": connected" comment, then publish and read the frame.
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("read open frame: %v", err)
	}

	if _, err := b.Publish(event.Spec{Subject: "demo.subject", Kind: event.KindTaskReceived, Actor: "tester"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	line, err := readDataLine(reader)
	if err != nil {
		t.Fatalf("read data line: %v", err)
	}
	if !strings.Contains(line, "demo.subject") {
		t.Errorf("streamed frame missing subject: %q", line)
	}
	if !strings.Contains(line, string(event.KindTaskReceived)) {
		t.Errorf("streamed frame missing kind: %q", line)
	}
}

// readDataLine reads SSE frames until it finds a "data:" line (skipping
// comment/heartbeat lines), or errors.
func readDataLine(r *bufio.Reader) (string, error) {
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		if strings.HasPrefix(line, "data:") {
			return line, nil
		}
		if line == "" {
			return "", io.EOF
		}
	}
}

func TestTokenMatch_ConstantTimeAcceptReject(t *testing.T) {
	// Lock in the constant-time token gate: exact match accepts; any wrong value,
	// a length-differing value, a prefix (the shape a timing attack probes), and
	// the empty string all reject. (The comparison itself runs in constant time
	// via crypto/subtle so the reject path can't be byte-by-byte guessed.)
	s, _ := newServer(t, &fakeCaller{}, "s3cret-token")
	if !s.tokenMatch("s3cret-token") {
		t.Error("exact token must match")
	}
	for _, bad := range []string{"", "s3cret-toke", "s3cret-token-extra", "S3CRET-TOKEN", "wrong", "s"} {
		if s.tokenMatch(bad) {
			t.Errorf("tokenMatch(%q) accepted, want reject", bad)
		}
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	// The web UI is a control surface (mutating buttons) whose URL carries the
	// auth token in ?token=. Defensive headers must be present on both authorized
	// and unauthorized responses.
	s, _ := newServer(t, &fakeCaller{result: map[string]any{"ok": true}}, "secret")
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for _, tc := range []struct {
		name, url string
	}{
		{"authorized", "/api/status?token=secret"},
		{"unauthorized", "/api/status?token=wrong"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))
			for k, v := range want {
				if got := rec.Header().Get(k); got != v {
					t.Errorf("header %s = %q, want %q", k, got, v)
				}
			}
		})
	}
}

// ---- Flow Studio ----

// Generate forwards intent + model from the JSON body and drops anything else.
func TestFlowGenerateForwardsBodyKeys(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"plan_json": "{}", "node_count": 0}}
	s, _ := newServer(t, fc, "secret")
	body := `{"intent":"ship the release","model":"sonnet","evil":"rm -rf"}`
	req := httptest.NewRequest(http.MethodPost, "/api/plan/generate?token=secret",
		strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "plan_generate" {
		t.Fatalf("expected one plan_generate call, got %v", fc.calls)
	}
	if fc.lastArgs["intent"] != "ship the release" || fc.lastArgs["model"] != "sonnet" {
		t.Errorf("body keys not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Errorf("non-allowlisted body key leaked through: %v", fc.lastArgs)
	}
}

// Refine forwards plan_json + feedback (the large-body case that motivates a
// JSON body over query args).
func TestFlowRefineForwardsBodyKeys(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"plan_json": "{}", "node_count": 1}}
	s, _ := newServer(t, fc, "secret")
	body := `{"plan_json":"{\"nodes\":[]}","feedback":"add a gate"}`
	req := httptest.NewRequest(http.MethodPost, "/api/plan/refine?token=secret",
		strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "plan_refine" {
		t.Fatalf("expected one plan_refine call, got %v", fc.calls)
	}
	if fc.lastArgs["plan_json"] != `{"nodes":[]}` || fc.lastArgs["feedback"] != "add a gate" {
		t.Errorf("body keys not forwarded: %v", fc.lastArgs)
	}
}

// The JSON-body routes are POST-only: a GET (prefetch, <img>) must never invoke
// the LLM.
func TestFlowGenerateRejectsGet(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/plan/generate?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d want 405", rec.Code)
	}
	if len(fc.calls) != 0 {
		t.Errorf("a GET must not reach the control plane, got %v", fc.calls)
	}
}

// A malformed body is rejected before the control plane is touched.
func TestFlowGenerateRejectsBadBody(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/plan/generate?token=secret",
		strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400", rec.Code)
	}
	if len(fc.calls) != 0 {
		t.Errorf("a bad body must not reach the control plane, got %v", fc.calls)
	}
}

// An over-cap body is refused (the MaxBytesReader guard), not buffered whole.
func TestFlowGenerateRejectsOversizedBody(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	huge := `{"intent":"` + strings.Repeat("a", jsonBodyMax+1) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/plan/generate?token=secret",
		strings.NewReader(huge))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400 (body over cap)", rec.Code)
	}
	if len(fc.calls) != 0 {
		t.Errorf("an oversized body must not reach the control plane, got %v", fc.calls)
	}
}

// Run drives CmdPlan through Stream (not Call) and forwards only plan_json.
func TestFlowRunStreamsPlan(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"plan_id": "plan-1", "node_outputs": map[string]any{}}}
	s, _ := newServer(t, fc, "secret")
	body := `{"plan_json":"{\"nodes\":[{\"id\":\"a\",\"kind\":\"loop\"}]}","evil":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/plan/run?token=secret",
		strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "plan" {
		t.Fatalf("expected one plan (stream) call, got %v", fc.calls)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Errorf("non-allowlisted body key leaked through: %v", fc.lastArgs)
	}
	if !strings.Contains(rec.Body.String(), "plan_id") {
		t.Errorf("terminal result not relayed: %s", rec.Body.String())
	}
}

func TestFlowRunRejectsGet(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/plan/run?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d want 405", rec.Code)
	}
	if len(fc.calls) != 0 {
		t.Errorf("a GET must not start a plan run, got %v", fc.calls)
	}
}

// Plan reads proxy through the read-arg / parameterless routes.
func TestFlowHistoryForwardsLimit(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"plans": []any{}}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet,
		"/api/plan_history?token=secret&limit=5&status=failed&evil=x", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "plan_history" {
		t.Fatalf("expected one plan_history call, got %v", fc.calls)
	}
	if fc.lastArgs["limit"] != "5" || fc.lastArgs["status"] != "failed" {
		t.Errorf("args not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Errorf("non-allowlisted arg leaked through: %v", fc.lastArgs)
	}
}

// The Chat view's send button: POST /api/run must stream the agent's events
// inline as SSE — an `open` frame, every event the loop emits forwarded verbatim,
// then a terminal `done` frame carrying the result — so the conversation renders
// live in the browser.
func TestRunStreamForwardsEventsInline(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"answer": "hi there", "iters": float64(1)}}
	s, _ := newServer(t, fc, "secret")
	body := `{"intent":"say hi","model":"demo","evil":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/run?token=secret", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type = %q want text/event-stream", ct)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "run" {
		t.Fatalf("expected one run (stream) call, got %v", fc.calls)
	}
	if fc.lastArgs["intent"] != "say hi" || fc.lastArgs["model"] != "demo" {
		t.Errorf("intent/model not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Errorf("non-allowlisted body key leaked through: %v", fc.lastArgs)
	}
	out := rec.Body.String()
	// open frame, the inline-forwarded loop event, and the terminal result.
	if !strings.Contains(out, `"kind":"open"`) {
		t.Errorf("missing open frame: %s", out)
	}
	if !strings.Contains(out, string(event.KindNodeStarted)) {
		t.Errorf("loop event not forwarded inline: %s", out)
	}
	if !strings.Contains(out, `"kind":"done"`) || !strings.Contains(out, "hi there") {
		t.Errorf("terminal result not relayed: %s", out)
	}
}

// Multi-turn continuity: when the Chat view sends prior `history`, the proxy
// folds it with the new turn into one transcript intent (the same convo mapping
// the OpenAI API uses) and the control plane only ever sees the resolved intent
// — never the raw history.
func TestRunStreamFoldsHistoryIntoTranscript(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"answer": "it was 7"}}
	s, _ := newServer(t, fc, "secret")
	body := `{"intent":"what was it?","history":[` +
		`{"role":"user","text":"remember 7"},` +
		`{"role":"assistant","text":"Got it, 7."}` +
		`]}`
	req := httptest.NewRequest(http.MethodPost, "/api/run?token=secret", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "run" {
		t.Fatalf("expected one run call, got %v", fc.calls)
	}
	gotIntent, _ := fc.lastArgs["intent"].(string)
	want := "User: remember 7\nAssistant: Got it, 7.\nUser: what was it?"
	if gotIntent != want {
		t.Errorf("folded intent =\n%q\nwant\n%q", gotIntent, want)
	}
	if _, leaked := fc.lastArgs["history"]; leaked {
		t.Errorf("history must not reach the control plane: %v", fc.lastArgs)
	}
}

// With no history, the intent passes through verbatim (the single-shot path is
// unchanged — the first message in a thread behaves exactly as before).
func TestRunStreamNoHistoryIsVerbatim(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"answer": "hi"}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/run?token=secret",
		strings.NewReader(`{"intent":"just hello"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if got, _ := fc.lastArgs["intent"].(string); got != "just hello" {
		t.Errorf("intent = %q, want verbatim", got)
	}
}

// An empty intent is a no-op the UI should reject before spending a run.
func TestRunStreamRequiresIntent(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/run?token=secret",
		strings.NewReader(`{"intent":"   "}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400", rec.Code)
	}
	if len(fc.calls) != 0 {
		t.Errorf("a blank intent must not start a run, got %v", fc.calls)
	}
}

// When the loop itself errors, the failure must reach the browser as an inline
// `error` frame on the open stream (headers are already flushed), not a 500.
func TestRunStreamRelaysLoopError(t *testing.T) {
	fc := &fakeCaller{err: errors.New("budget exhausted")}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/run?token=secret",
		strings.NewReader(`{"intent":"go"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200 (stream already open)", rec.Code)
	}
	out := rec.Body.String()
	if !strings.Contains(out, `"kind":"error"`) || !strings.Contains(out, "budget exhausted") {
		t.Errorf("loop error not relayed inline: %s", out)
	}
}

func TestRunStreamRejectsGet(t *testing.T) {
	fc := &fakeCaller{}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/run?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d want 405", rec.Code)
	}
	if len(fc.calls) != 0 {
		t.Errorf("a GET must not start a run, got %v", fc.calls)
	}
}

func TestFlowStatsProxiesPlanStats(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"total": 0}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/plan_stats?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "plan_stats" {
		t.Errorf("expected one plan_stats call, got %v", fc.calls)
	}
}
