// SPDX-License-Identifier: MIT

package webui

import (
	"bufio"
	"context"
	"errors"
	"io"
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
	calls  []string
	result map[string]any
	err    error
}

func (f *fakeCaller) Call(_ context.Context, cmd string, _ map[string]any) (map[string]any, error) {
	f.calls = append(f.calls, cmd)
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
	// Every advertised /api route must map to a read-only command — assert the
	// proxy never issues anything outside the known read set.
	readOnly := map[string]bool{
		"status": true, "memory_list": true, "world_list": true,
		"skill_list": true, "inbox": true, "reflect_show": true,
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
