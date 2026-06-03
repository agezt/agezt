// SPDX-License-Identifier: MIT

package restapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/meshctx"
)

// runsPost drives POST /api/v1/runs with an optional mesh-hop header.
func runsPost(s *Server, token, body string, hop string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/runs", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	if hop != "" {
		r.Header.Set(meshctx.HopHeader, hop)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)
	return rec
}

// TestMeshHop_OverLimitRefused: a run arriving with a hop past MaxHops is rejected
// with 508 Loop Detected, so a cross-node delegation chain can't recurse forever.
func TestMeshHop_OverLimitRefused(t *testing.T) {
	eng := &fakeEngine{answer: "ok"}
	s := newServer(t, eng, "secret")

	rec := runsPost(s, "secret", `{"intent":"hi"}`, strconv.Itoa(meshctx.MaxHops+1))
	if rec.Code != http.StatusLoopDetected {
		t.Fatalf("over-limit hop should be %d, got %d", http.StatusLoopDetected, rec.Code)
	}
	if eng.ranIntent != "" {
		t.Error("an over-limit run must not execute")
	}
}

// TestMeshHop_AtLimitStillRuns: a hop exactly at MaxHops is the last allowed hop —
// the run still executes (the NEXT delegation, hop+1 > MaxHops, would be refused).
func TestMeshHop_AtLimitStillRuns(t *testing.T) {
	eng := &fakeEngine{answer: "ok"}
	s := newServer(t, eng, "secret")

	rec := runsPost(s, "secret", `{"intent":"hi"}`, strconv.Itoa(meshctx.MaxHops))
	if rec.Code != http.StatusOK {
		t.Fatalf("at-limit hop should run (200), got %d", rec.Code)
	}
	if eng.ranIntent != "hi" {
		t.Errorf("run should have executed, ranIntent=%q", eng.ranIntent)
	}
}

// TestMeshHop_NoHeaderRuns: a normal (non-delegated) run has no header and runs.
func TestMeshHop_NoHeaderRuns(t *testing.T) {
	eng := &fakeEngine{answer: "ok"}
	s := newServer(t, eng, "secret")

	rec := runsPost(s, "secret", `{"intent":"hi"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("no-header run should be 200, got %d", rec.Code)
	}
}

// TestMeshHop_RefusalIsAudited: a refused federation loop publishes a
// mesh.loop_refused event so it is visible in the journal / `agt pulse` (M210).
func TestMeshHop_RefusalIsAudited(t *testing.T) {
	eng := &fakeEngine{answer: "ok"}
	s := newServer(t, eng, "secret")

	sub, err := s.bus.Subscribe("mesh.>", 8)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()

	rec := runsPost(s, "secret", `{"intent":"hi"}`, strconv.Itoa(meshctx.MaxHops+1))
	if rec.Code != http.StatusLoopDetected {
		t.Fatalf("want 508, got %d", rec.Code)
	}

	select {
	case ev := <-sub.C:
		if ev.Kind != event.KindMeshLoopRefused {
			t.Errorf("event kind = %q, want %q", ev.Kind, event.KindMeshLoopRefused)
		}
	case <-time.After(time.Second):
		t.Fatal("expected a mesh.loop_refused event, got none")
	}
}
