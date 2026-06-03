// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenantctx"
)

// toolThenFinalProvider emits one tool call to "probe" on its first turn, then a final
// answer — enough to drive the agent loop through a single tool invocation.
type toolThenFinalProvider struct{ calls int }

func (p *toolThenFinalProvider) Name() string { return "ttf" }
func (p *toolThenFinalProvider) Complete(_ context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	p.calls++
	if p.calls == 1 {
		return &agent.CompletionResponse{
			Message: agent.Message{
				Role:      agent.RoleAssistant,
				ToolCalls: []agent.ToolCall{{ID: "1", Name: "file", Input: json.RawMessage(`{"op":"read"}`)}},
			},
			StopReason: agent.StopToolUse,
		}, nil
	}
	return &agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, Content: "done"},
		StopReason: agent.StopEndTurn,
	}, nil
}

// recordingTool captures the tenant id carried by the context it is invoked with.
type recordingTool struct {
	mu      sync.Mutex
	seen    string
	invoked int
}

func (r *recordingTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "file", Description: "records the run tenant", InputSchema: json.RawMessage(`{"type":"object"}`)}
}
func (r *recordingTool) Invoke(ctx context.Context, _ json.RawMessage) (agent.Result, error) {
	r.mu.Lock()
	r.seen = tenantctx.Tenant(ctx)
	r.invoked++
	r.mu.Unlock()
	return agent.Result{Output: "ok"}, nil
}

// TestKernel_StampsTenantOnRunContext proves the kernel injects its Config.TenantID into
// the run context so tools can read it (M219): a tenant kernel stamps its id, and the
// primary kernel (empty TenantID) leaves it blank.
func TestKernel_StampsTenantOnRunContext(t *testing.T) {
	for _, tc := range []struct{ id, want string }{
		{"alpha", "alpha"},
		{"", ""},
	} {
		rec := &recordingTool{}
		k, err := runtime.Open(runtime.Config{
			BaseDir:  t.TempDir(),
			Provider: &toolThenFinalProvider{},
			System:   "base prompt",
			TenantID: tc.id,
			Tools:    map[string]agent.Tool{"file": rec},
		})
		if err != nil {
			t.Fatalf("TenantID=%q: Open: %v", tc.id, err)
		}
		if _, _, err := k.Run(context.Background(), "go"); err != nil {
			k.Close()
			t.Fatalf("TenantID=%q: Run: %v", tc.id, err)
		}
		k.Close()

		rec.mu.Lock()
		seen := rec.seen
		invoked := rec.invoked
		rec.mu.Unlock()
		if invoked == 0 {
			t.Fatalf("TenantID=%q: probe tool was never invoked (policy?) — cannot verify injection", tc.id)
		}
		if seen != tc.want {
			t.Errorf("TenantID=%q: tool saw tenant %q, want %q", tc.id, seen, tc.want)
		}
	}
}
