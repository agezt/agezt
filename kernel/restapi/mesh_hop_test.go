// SPDX-License-Identifier: MIT

package restapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
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

// TestMeshHop_EnvOverrideTightens: with AGEZT_MESH_MAX_HOPS lowered, a hop that would
// pass the default limit is refused at the configured one (M211).
func TestMeshHop_EnvOverrideTightens(t *testing.T) {
	t.Setenv(meshctx.EnvMaxHops, "2")
	eng := &fakeEngine{answer: "ok"}
	s := newServer(t, eng, "secret")

	// hop 3 > configured 2 (but < default 8) → refused.
	rec := runsPost(s, "secret", `{"intent":"hi"}`, "3")
	if rec.Code != http.StatusLoopDetected {
		t.Fatalf("hop 3 with limit 2 should be %d, got %d", http.StatusLoopDetected, rec.Code)
	}
	if eng.ranIntent != "" {
		t.Error("an over-limit run must not execute")
	}
	// hop 2 == configured limit → still runs.
	eng2 := &fakeEngine{answer: "ok"}
	s2 := newServer(t, eng2, "secret")
	rec2 := runsPost(s2, "secret", `{"intent":"hi"}`, "2")
	if rec2.Code != http.StatusOK {
		t.Fatalf("hop 2 at limit 2 should run, got %d", rec2.Code)
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

// TestMeshHop_RefusalAuditedToTenantBus: a tenant-targeted run that is loop-refused
// publishes its audit event to that TENANT's bus, not the primary one (M212), so a
// tenant sees its own mesh refusals.
func TestMeshHop_RefusalAuditedToTenantBus(t *testing.T) {
	primary := &fakeEngine{answer: "ok"}
	s := newServer(t, primary, "secret")

	tj, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	tbus := bus.New(tj)
	t.Cleanup(func() { tbus.Close(); tj.Close() })
	alpha := &fakeEngine{answer: "alpha", b: tbus}
	s.SetTenantResolver(func(id string) (Engine, *bus.Bus, error) {
		if id == "alpha" {
			return alpha, tbus, nil
		}
		return nil, nil, errors.New("unknown tenant " + id)
	})

	// Subscribe on BOTH buses; only the tenant bus should receive the event.
	tenantSub, err := tbus.Subscribe("mesh.>", 8)
	if err != nil {
		t.Fatalf("tenant subscribe: %v", err)
	}
	defer tenantSub.Cancel()
	primarySub, err := s.bus.Subscribe("mesh.>", 8)
	if err != nil {
		t.Fatalf("primary subscribe: %v", err)
	}
	defer primarySub.Cancel()

	r := httptest.NewRequest(http.MethodPost, "/api/v1/runs", strings.NewReader(`{"intent":"hi"}`))
	r.Header.Set("Authorization", "Bearer secret")
	r.Header.Set("X-Agezt-Tenant", "alpha")
	r.Header.Set(meshctx.HopHeader, strconv.Itoa(meshctx.MaxHops+1))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)
	if rec.Code != http.StatusLoopDetected {
		t.Fatalf("want 508, got %d", rec.Code)
	}

	select {
	case ev := <-tenantSub.C:
		if ev.Kind != event.KindMeshLoopRefused {
			t.Errorf("tenant event kind = %q", ev.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("tenant bus should have received the refusal event")
	}
	// The primary bus must NOT have received it.
	select {
	case ev := <-primarySub.C:
		t.Errorf("primary bus must not receive a tenant's refusal, got %q", ev.Kind)
	case <-time.After(50 * time.Millisecond):
		// expected: nothing on the primary bus
	}
}
