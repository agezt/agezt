// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestAgentResolveNativeTypedRegistry(t *testing.T) {
	found := 0
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if spec.Name != CmdAgentResolve {
			continue
		}
		found++
		if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.HTTP != (opapi.HTTP{Method: "POST", Path: "/api/agents/resolve"}) {
			t.Fatalf("agent_resolve metadata=%+v", spec)
		}
	}
	wire, exists := commandRegistry[CmdAgentResolve]
	if found != 1 || !exists || !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
		t.Fatalf("agent_resolve native wire found=%d metadata=%+v", found, wire)
	}
}

type resolveRoutingProvider struct {
	llm.Provider
	mu     sync.Mutex
	chains map[string][]string
}

func (p *resolveRoutingProvider) TaskModelChainsView() map[string][]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string][]string, len(p.chains))
	for k, v := range p.chains {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func (p *resolveRoutingProvider) SetTaskModelChains(chains map[string][]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.chains = make(map[string][]string, len(chains))
	for k, v := range chains {
		p.chains[k] = append([]string(nil), v...)
	}
}

func resolveCall(t *testing.T, s *Server, args map[string]any) string {
	t.Helper()
	a, b := net.Pipe()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(context.Background(), b) }()
	raw, _ := json.Marshal(Request{ID: "resolve", Cmd: CmdAgentResolve, Token: "primary", Args: args})
	if _, err := a.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(a).ReadBytes(10)
	a.Close()
	<-done
	if err != nil {
		t.Fatal(err)
	}
	return string(line)
}

// The native ports read the prior force generation and the doctor's exhausted
// chain (matched by any lineage id), apply the chain through the overseer, and
// post delegations on the shared board.
func TestAgentResolveNativePorts(t *testing.T) {
	dir := t.TempDir()
	prov := &resolveRoutingProvider{Provider: mock.New(), chains: map[string][]string{"code": {"gpt-5", "gpt-4.1"}}}
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: prov})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	for _, p := range []roster.Profile{{Slug: "ops", Soul: "s"}, {Slug: "peer", Soul: "s"}} {
		if _, err := k.AddProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	for _, payload := range []map[string]any{
		{"phase": "completed", "agent": "ops", "resolution": "force_chain", "routing_task_type": "code", "routing_force_generation": 4},
		{"phase": "routing_force_exhausted_detected", "agent": "ops", "routing_task_type": "code", "routing_task_model_chain": []string{"a", "b"}, "root_incident_id": "chain-root"},
	} {
		subject := "agent.resolve"
		if payload["phase"] != "completed" {
			subject = "doctor.auto_repair"
		}
		if _, err := k.Bus().Publish(event.Spec{Subject: subject, Kind: event.KindInfo, Actor: "test", Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	s := NewServer(k, dir)
	s.token = "primary"
	if line := resolveCall(t, s, map[string]any{"ref": "ops", "resolution": "force_chain", "task_type": "code", "task_model_chain": []any{"A", " b "}, "parent_incident_id": "chain-root"}); !strings.Contains(line, `"error":"force_chain resolution must choose a new chain for exhausted routing policy"`) {
		t.Fatal("exhausted chain via parent lineage", line)
	}
	line := resolveCall(t, s, map[string]any{"ref": "ops", "resolution": "force_chain", "task_type": "code", "task_model_chain": []any{"gpt-4.1", "c"}, "summary": "why"})
	if !strings.Contains(line, `"applied":true`) || strings.Join(prov.TaskModelChainsView()["code"], ",") != "gpt-4.1,c" {
		t.Fatal("force chain", line, prov.TaskModelChainsView())
	}
	var completed map[string]any
	_ = k.Journal().Range(func(e *event.Event) error {
		var pl map[string]any
		if e.Subject == "agent.resolve" && json.Unmarshal(e.Payload, &pl) == nil && pl["phase"] == "completed" && pl["resolution_summary"] == "why" {
			completed = pl
		}
		return nil
	})
	if completed["routing_force_generation"] != float64(5) || completed["previous_routing_force_generation"] != float64(4) || completed["routing_task_type"] != "code" {
		t.Fatalf("force generation from the journal: %+v", completed)
	}
	if line := resolveCall(t, s, map[string]any{"ref": "ops", "resolution": "delegated", "delegate_to": "peer"}); !strings.Contains(line, `"error":"the board is not available on this daemon"`) {
		t.Fatal("no board", line)
	}
	st, err := board.Open(filepath.Join(dir, "board"))
	if err != nil {
		t.Fatal(err)
	}
	var notified []string
	s.SetBoard(st, func(m board.Message, corr string) { notified = append(notified, m.To+"|"+m.Text+"|"+corr) })
	if line := resolveCall(t, s, map[string]any{"ref": "ops", "resolution": "delegated", "delegate_to": "peer", "summary": "own this"}); !strings.Contains(line, `"applied":true`) || len(notified) != 1 || notified[0] != "peer|own this|" {
		t.Fatal("delegation", line, notified)
	}
	if line := resolveCall(t, s, map[string]any{"ref": "peer", "resolution": "paused"}); !strings.Contains(line, `"applied":true`) {
		t.Fatal("pause", line)
	}
	if p, _ := k.Roster().Get("peer"); p.Enabled {
		t.Fatal("pause port did not pause")
	}
	if line := resolveCall(t, s, map[string]any{"ref": "peer", "resolution": "retired"}); !strings.Contains(line, `"applied":true`) {
		t.Fatal("retire", line)
	}
	if p, _ := k.Roster().Get("peer"); !p.Retired || p.RetiredReason != "retired by operator incident resolution" {
		t.Fatalf("retire port: %+v", p)
	}
}
