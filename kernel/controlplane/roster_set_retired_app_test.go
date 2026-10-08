// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestAgentRetireReviveNativeTypedRegistry(t *testing.T) {
	for cmd, path := range map[string]string{CmdAgentRetire: "/api/agents/retire", CmdAgentRevive: "/api/agents/revive"} {
		found := 0
		for _, operation := range registeredAppOperations() {
			spec := operation.Spec()
			if spec.Name != cmd {
				continue
			}
			found++
			if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.HTTP != (opapi.HTTP{Method: "POST", Path: path}) {
				t.Fatalf("%s metadata=%+v", cmd, spec)
			}
		}
		wire, exists := commandRegistry[cmd]
		if found != 1 || !exists || !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire found=%d metadata=%+v", cmd, found, wire)
		}
	}
}

// The native ports pause the agent's triggers on retire, preview the impact
// first, count what a revive leaves paused, and journal both actions.
func TestAgentRetireReviveNativePorts(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	if _, err := k.AddProfile(roster.Profile{Slug: "lead", Soul: "s"}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.AddStanding(standing.Order{Name: "watch", Triggers: []standing.Trigger{{Type: standing.TriggerEvent, Subject: "x.y"}}, Agent: "lead", Plan: "do"}); err != nil {
		t.Fatal(err)
	}
	// Distinct counts (one standing order, two schedules) keep the ports apart.
	for _, intent := range []string{"refresh", "rotate"} {
		job, err := k.Schedules().Add(intent, time.Hour, "", "test", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := k.Schedules().SetAgent(job.ID, "lead"); err != nil {
			t.Fatal(err)
		}
	}
	s := NewServer(k, dir)
	s.token = "primary"
	call := func(cmd string, args map[string]any) string {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "life", Cmd: cmd, Token: "primary", Args: args})
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
	retire := call(CmdAgentRetire, map[string]any{"ref": "lead", "reason": "done"})
	for _, want := range []string{`"impact":["watch`, `"schedule_count":2`, `"schedules":["refresh (sched-`, `"schedules_paused":2,"skill_count":0`, `"standing_paused":1`, `"retired":true`, `"retired_reason":"done"`} {
		if !strings.Contains(retire, want) {
			t.Fatalf("retire missing %s: %s", want, retire)
		}
	}
	revive := call(CmdAgentRevive, map[string]any{"ref": "lead"})
	if !strings.Contains(revive, `"schedules_paused":2`) || !strings.Contains(revive, `"standing_paused":1`) || strings.Contains(revive, `"retired":true`) || strings.Contains(revive, "impact") {
		t.Fatalf("revive: %s", revive)
	}
	subjects := map[string]map[string]any{}
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Subject == "agent.retire" || e.Subject == "agent.revive" {
			var pl map[string]any
			_ = json.Unmarshal(e.Payload, &pl)
			subjects[e.Subject] = pl
		}
		return nil
	})
	summary, _ := subjects["agent.retire"]["impact_summary"].(map[string]any)
	if subjects["agent.retire"]["reason"] != "done" || summary["standing_paused"] != float64(1) || summary["schedules_paused"] != float64(2) || summary["standing_count"] != float64(1) || subjects["agent.revive"]["standing_paused"] != float64(1) || subjects["agent.revive"]["schedules_paused"] != float64(2) {
		t.Fatalf("journal: %+v", subjects)
	}
}
