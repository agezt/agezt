// SPDX-License-Identifier: MIT

package webui

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
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

func TestDashboardServedAtRoot(t *testing.T) {
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
	if !strings.Contains(rec.Body.String(), "live monitor") {
		t.Error("dashboard body missing expected marker")
	}
	// The world panel ships a node-link graph renderer; guard against a refactor
	// silently dropping it (the backend feeds it via the /api/world `edges` key).
	body := rec.Body.String()
	if !strings.Contains(body, "function worldGraph") {
		t.Error("dashboard missing the world graph renderer")
	}
	// The Runs panel renders the live run list; guard its wiring (panel id +
	// renderer) against a refactor dropping it.
	if !strings.Contains(body, `data-panel="runs"`) || !strings.Contains(body, "runs:") {
		t.Error("dashboard missing the Runs panel")
	}
	if !strings.Contains(body, `data-panel="schedules"`) || !strings.Contains(body, "schedules:") {
		t.Error("dashboard missing the Schedules panel")
	}
	// The Schedules panel shows each entry's next-fire time.
	if !strings.Contains(body, "next_run_unix") || !strings.Contains(body, "function fmtDateTime") {
		t.Error("dashboard missing the schedule next-fire wiring")
	}
	// Clicking a run opens a detail modal that fetches its event arc.
	if !strings.Contains(body, "function openRun") || !strings.Contains(body, "/api/journal") {
		t.Error("dashboard missing the run-detail modal wiring")
	}
	if !strings.Contains(body, `data-panel="stats"`) || !strings.Contains(body, "stats:") {
		t.Error("dashboard missing the Stats panel")
	}
	// The Budget panel renders the governor spend snapshot.
	if !strings.Contains(body, `data-panel="budget"`) || !strings.Contains(body, "budget:") {
		t.Error("dashboard missing the Budget panel")
	}
	// The Cache panel renders the prompt-cache savings aggregate.
	if !strings.Contains(body, `data-panel="cache"`) || !strings.Contains(body, "cache:") {
		t.Error("dashboard missing the Cache panel")
	}
	// A non-zero provider-fallback count drives a header warning badge.
	if !strings.Contains(body, `id="fbBadge"`) || !strings.Contains(body, "function updateFallbackBadge") {
		t.Error("dashboard missing the provider-fallback badge wiring")
	}
	// Clicking the badge opens a modal listing recent provider.fallback events.
	if !strings.Contains(body, "function openFallbacks") || !strings.Contains(body, "provider.fallback") {
		t.Error("dashboard missing the fallback-detail modal wiring")
	}
	// The event feed can be filtered by kind (client-side row toggling).
	if !strings.Contains(body, `id="feedFilter"`) || !strings.Contains(body, "function applyFeedFilter") {
		t.Error("dashboard missing the feed kind-filter wiring")
	}
	// The Providers panel renders the provider-routing aggregate.
	if !strings.Contains(body, `data-panel="providers"`) || !strings.Contains(body, "providers:") {
		t.Error("dashboard missing the Providers panel")
	}
	// The Providers panel drills into the per-call routing timeline.
	if !strings.Contains(body, "function openProviderLog") || !strings.Contains(body, "/api/provider_log") {
		t.Error("dashboard missing the provider routing-log modal wiring")
	}
	// The Tools panel renders the tool-execution aggregate.
	if !strings.Contains(body, `data-panel="tools"`) || !strings.Contains(body, "tools:") {
		t.Error("dashboard missing the Tools panel")
	}
	// The Tools panel drills into the per-call invocation log.
	if !strings.Contains(body, "function openToolLog") || !strings.Contains(body, "/api/tool_log") {
		t.Error("dashboard missing the tool-log modal wiring")
	}
	// The Policy panel renders the edict-decision aggregate + drills into the log.
	if !strings.Contains(body, `data-panel="policy"`) || !strings.Contains(body, "policy:") {
		t.Error("dashboard missing the Policy panel")
	}
	if !strings.Contains(body, "function openPolicyLog") || !strings.Contains(body, "/api/policy_log") {
		t.Error("dashboard missing the policy-log modal wiring")
	}
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
		"skill_list": true, "inbox": true, "reflect_show": true, "approvals": true,
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

func TestDashboard_NoUnsafeDOMSinks(t *testing.T) {
	// The dashboard renders server-supplied text (tool output, intents, fallback
	// reasons) into the DOM. It is XSS-safe BY CONSTRUCTION — textContent /
	// createElement only, never an HTML-injection sink. This test locks that
	// invariant in: a future edit that introduces innerHTML-with-data (or another
	// sink) fails here instead of silently shipping a stored-XSS vector.
	src := string(dashboardHTML)
	for _, sink := range []string{"insertAdjacentHTML", "document.write", "outerHTML"} {
		if strings.Contains(src, sink) {
			t.Errorf("dashboard uses unsafe DOM sink %q — render via textContent/el() instead", sink)
		}
	}
	// innerHTML is permitted ONLY to clear a node (= "" / = ''). Any other
	// assignment is an HTML-injection risk.
	for _, m := range regexp.MustCompile(`innerHTML\s*=\s*([^;\n]*)`).FindAllStringSubmatch(src, -1) {
		if v := strings.TrimSpace(m[1]); v != `""` && v != `''` {
			t.Errorf("dashboard assigns innerHTML to %q — only clearing (= \"\") is allowed", v)
		}
	}
}

// TestDashboard_RendersContextInspector locks in the SPEC-10 §3.5 / SPEC-07
// context-inspector surface (M373): the run-detail arc must read the
// llm.request event's context_chars + context_by_role and render the compact
// "ctx chars" summary plus the expandable per-role breakdown. A refactor that
// drops this rendering fails here. (The end-to-end behaviour is Playwright-
// verified against a live daemon; this guards the wiring in the embedded HTML.)
func TestDashboard_RendersContextInspector(t *testing.T) {
	src := string(dashboardHTML)
	for _, marker := range []string{"context_chars", "context_by_role", "ctx chars", "llm.request"} {
		if !strings.Contains(src, marker) {
			t.Errorf("dashboard context inspector missing %q — the SPEC-10 §3.5 surface regressed", marker)
		}
	}
}

// TestDashboard_RendersIsolationCard locks in the SPEC-12 §4 / SPEC-07 tool-call
// debug "isolation" view: the run-detail arc must render warden.executed events
// (which carry the effective/requested profile + downgrade flag from the journal)
// rather than dropping them to a bare kind line. The payload keys it reads are
// the warden.executed contract — if either side renames a field this trips.
func TestDashboard_RendersIsolationCard(t *testing.T) {
	src := string(dashboardHTML)
	for _, marker := range []string{
		"warden.executed",   // the arcDetail/arcFull case exists
		"profile_effective", // the effective isolation profile is read
		"profile_requested", // the requested profile (for the downgrade delta)
		"downgraded",        // the security-relevant downgrade flag is surfaced
		"isolation",         // the human label
	} {
		if !strings.Contains(src, marker) {
			t.Errorf("dashboard isolation card missing %q — the SPEC-12 §4 tool-call isolation surface regressed", marker)
		}
	}
}
