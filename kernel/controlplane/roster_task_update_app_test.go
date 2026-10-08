// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"regexp"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestAgentTaskUpdateNativeTypedRegistry(t *testing.T) {
	found := 0
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if spec.Name != CmdAgentTaskUpdate {
			continue
		}
		found++
		if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.HTTP != (opapi.HTTP{Method: "POST", Path: "/api/agents/task"}) {
			t.Fatalf("agent_task_update metadata=%+v", spec)
		}
	}
	wire, exists := commandRegistry[CmdAgentTaskUpdate]
	if found != 1 || !exists || !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
		t.Fatalf("agent_task_update native wire found=%d metadata=%+v", found, wire)
	}
}

func TestAgentTaskUpdateNativeWire(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	if _, err := k.AddProfile(roster.Profile{Slug: "ops", Soul: "s"}); err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	call := func(args map[string]any) string {
		t.Helper()
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "task", Cmd: CmdAgentTaskUpdate, Token: "primary", Args: args})
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
	line := call(map[string]any{"ref": "ops", "op": "add", "title": "check", "description": "d"})
	// The legacy handler embedded roster.AgentTask, so its declared member order
	// survives the otherwise sorted object envelope.
	if !regexp.MustCompile(`"task":\{"id":"[0-9A-Z]+","title":"check","description":"d","scope":"total","status":"todo","created_ms":\d+,"updated_ms":\d+\}`).MatchString(line) || !regexp.MustCompile(`"updated":true`).MatchString(line) {
		t.Fatal("task wire", line)
	}
	edits := func() (n int) {
		_ = k.Journal().Range(func(e *event.Event) error {
			if e.Kind == event.KindRosterUpdated && e.Subject == "roster.ops" {
				n++
			}
			return nil
		})
		return n
	}
	before := edits()
	if line := call(map[string]any{"ref": "ops", "id": "missing", "status": "done"}); !regexp.MustCompile(`"error":"unknown agent task: missing"`).MatchString(line) {
		t.Fatal("missing task", line)
	}
	// A missed task still runs the journaled profile update, as before.
	if after := edits(); after != before+1 {
		t.Fatalf("roster edits %d -> %d", before, after)
	}
}
