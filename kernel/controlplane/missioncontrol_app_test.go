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

	apppulse "github.com/agezt/agezt/kernel/app/pulse"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestMissionControlNativeTypedRegistry(t *testing.T) {
	for _, cmd := range []string{CmdSpendToday, CmdAttention} {
		found := 0
		for _, operation := range registeredAppOperations() {
			spec := operation.Spec()
			if spec.Name != cmd {
				continue
			}
			found++
			if spec.Input == nil || spec.Output == nil || len(spec.OutputSchema) == 0 || !spec.ReadOnly || !spec.AllowUnknownInput || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
				t.Fatalf("%s metadata=%+v", cmd, spec)
			}
		}
		wire, exists := commandRegistry[cmd]
		if found != 1 || !exists || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire found=%d metadata=%+v", cmd, found, wire)
		}
	}
}

type askPulse struct {
	apppulse.Controller
	asks []map[string]any
}

func (p askPulse) PendingAsks() []map[string]any { return p.asks }

// The tile reads the governor's spend for today, and the feed merges the live
// approval registry with the injected pulse's asks.
func TestMissionControlNativeBindings(t *testing.T) {
	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{Name: "mock", Provider: mock.New(llm.CompletionResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}, StopReason: llm.StopEndTurn, Usage: llm.Usage{InputTokens: 1000, OutputTokens: 500, Model: "claude-sonnet-4-6"}}), AuthMode: governor.AuthAPIKey}); err != nil {
		t.Fatal(err)
	}
	gov, err := governor.New(governor.Config{Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gov.Complete(context.Background(), llm.CompletionRequest{Model: "claude-sonnet-4-6"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: gov})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	stop, cancel := context.WithCancel(context.Background())
	defer cancel()
	go k.Approvals().Submit(stop, approval.SubmitSpec{Capability: "shell", ToolName: "shell", Reason: "risky", CorrelationID: "c1"})
	for k.Approvals().PendingCount() != 1 {
		time.Sleep(time.Millisecond)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	call := func(cmd string, args map[string]any) string {
		a, b := net.Pipe()
		a.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), b) }()
		raw, _ := json.Marshal(Request{ID: "m", Cmd: cmd, Token: "primary", Args: args})
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
	if line := call(CmdSpendToday, nil); !strings.Contains(line, `"result":{"total":1050000}`) {
		t.Fatal("the tile reads the governor's spend", line)
	}
	if line := call(CmdAttention, nil); !strings.Contains(line, `"count":1`) || !strings.Contains(line, `"summary":"shell — risky"`) {
		t.Fatal("without a pulse the feed holds the pending approval", line)
	}
	s.SetPulse(askPulse{asks: []map[string]any{{"issue_key": "disk", "summary": "disk filling", "ts_unix_ms": time.Now().UnixMilli()}}})
	if line := call(CmdAttention, map[string]any{"limit": 1}); !strings.Contains(line, `"count":1`) || !strings.Contains(line, `"href":"/jarvis#ask-disk"`) {
		t.Fatal("the newest item, a fresh pulse ask, leads the feed", line)
	}
}
