// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestAgentWakeNativeTypedRegistry(t *testing.T) {
	found := 0
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if spec.Name != CmdAgentWake {
			continue
		}
		found++
		if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.HTTP != (opapi.HTTP{Method: "POST", Path: "/api/agents/wake"}) {
			t.Fatalf("agent_wake metadata=%+v", spec)
		}
	}
	wire, exists := commandRegistry[CmdAgentWake]
	if found != 1 || !exists || !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
		t.Fatalf("agent_wake native wire found=%d metadata=%+v", found, wire)
	}
}

// The native launch adapter runs the wake asynchronously under its correlation
// and carries the whole incident lineage into the run's terminal event.
func TestAgentWakeNativeLaunchCarriesLineage(t *testing.T) {
	dir := t.TempDir()
	prov := mock.New(mock.FinalText("woken"))
	// The run must not start before the response: a synchronous launch would
	// hold the reply until this request returns and the read times out.
	release := make(chan struct{})
	prov.OnRequest = func(llm.CompletionRequest) { <-release }
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: prov})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	if _, err := k.AddProfile(roster.Profile{Slug: "ops", Soul: "s"}); err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	a, b := net.Pipe()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(context.Background(), b) }()
	raw, _ := json.Marshal(Request{ID: "wake", Cmd: CmdAgentWake, Token: "primary", Args: map[string]any{"ref": "ops", "reason": "why", "incident_id": "i", "root_incident_id": "r", "parent_incident_id": "p"}})
	if _, err := a.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(a).ReadBytes(10)
	close(release)
	a.Close()
	<-done
	var resp struct {
		Result struct {
			Accepted      bool   `json:"accepted"`
			Agent         string `json:"agent"`
			CorrelationID string `json:"correlation_id"`
		} `json:"result"`
	}
	if err != nil || json.Unmarshal(line, &resp) != nil || !resp.Result.Accepted || resp.Result.Agent != "ops" || resp.Result.CorrelationID == "" {
		t.Fatal(string(line), err)
	}
	var phases []string
	var completed map[string]any
	for deadline := time.Now().Add(3 * time.Second); completed == nil && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		phases = nil
		_ = k.Journal().Range(func(e *event.Event) error {
			if e.Subject != "agent.wake" || e.CorrelationID != resp.Result.CorrelationID {
				return nil
			}
			var pl map[string]any
			_ = json.Unmarshal(e.Payload, &pl)
			phase, _ := pl["phase"].(string)
			phases = append(phases, phase)
			if phase == "completed" {
				completed = pl
			}
			return nil
		})
	}
	if len(phases) != 2 || phases[0] != "requested" || completed == nil || completed["answer"] != "woken" || completed["reason"] != "why" || completed["incident_id"] != "i" || completed["root_incident_id"] != "r" || completed["parent_incident_id"] != "p" || prov.CallCount() != 1 {
		t.Fatalf("wake phases %v completed %+v calls %d", phases, completed, prov.CallCount())
	}
}
