// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestRun_AsAgent_AppliesWholeProfile: a direct `agt run --agent X` (and the
// console) is bound by the same profile as every other way of running X —
// standing orders, cadence, the workboard. The handler used to copy a subset
// of the profile by hand, so a direct run offered the model tools the agent
// denies and dropped its standing instructions (W2).
func TestRun_AsAgent_AppliesWholeProfile(t *testing.T) {
	var (
		mu  sync.Mutex
		got []llm.CompletionRequest
	)
	prov := mock.New()
	prov.Responder = func(req llm.CompletionRequest) llm.CompletionResponse {
		mu.Lock()
		got = append(got, req)
		mu.Unlock()
		return mock.FinalText("ok")
	}
	_, _, c, _ := startPair(t, prov)
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":         "scribe",
			"soul":         "You write.",
			"instructions": []any{"Never touch the shell."},
			"tool_deny":    []any{"shell"},
		},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	// The dry-run plan is built from the same context, so it agrees.
	plan, err := c.Call(ctx, controlplane.CmdRun,
		map[string]any{"intent": "write a note", "agent": "scribe", "dry_run": true})
	if err != nil {
		t.Fatalf("dry-run as agent: %v", err)
	}
	tools, _ := plan["tools"].([]any)
	for _, name := range tools {
		if name == "shell" {
			t.Errorf("dry-run plan lists %q, which the agent's tool_deny forbids: %v", name, tools)
		}
	}

	if _, err := c.Stream(ctx, controlplane.CmdRun,
		map[string]any{"intent": "write a note", "agent": "scribe"},
		func(*event.Event) {}); err != nil {
		t.Fatalf("run as agent: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 {
		t.Fatal("the provider was never called")
	}
	req := got[0]
	for _, td := range req.Tools {
		if td.Name == "shell" {
			t.Errorf("the model was offered %q, which the agent's tool_deny forbids", td.Name)
		}
	}
	if !strings.Contains(req.System, "You write.") {
		t.Errorf("system prompt lost the agent's soul:\n%s", req.System)
	}
	if !strings.Contains(req.System, "Never touch the shell.") {
		t.Errorf("system prompt lost the agent's standing instructions:\n%s", req.System)
	}
}
