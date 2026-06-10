// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// TestRun_AsAgent_MemoryScope proves the M786 chain end to end over the wire:
// a run AS a named agent recalls that agent's PRIVATE notes into its injected
// context (plus shared memory), while a plain run sees shared memory only —
// "shared brain, private notes" (M652) wired to the M783 identity.
func TestRun_AsAgent_MemoryScope(t *testing.T) {
	prov := mock.New(
		mock.FinalText("run1"), // the agent run
		mock.FinalText("run2"), // the plain run
	)
	var systems []string
	prov.OnRequest = func(req agent.CompletionRequest) { systems = append(systems, req.System) }
	k, _, c, _ := startPairWithConfig(t, runtime.Config{
		Provider:     prov,
		Tools:        map[string]agent.Tool{"shell": shell.New()},
		MemoryInject: true,
	})
	ctx := context.Background()

	// Shared fact + a note private to the researcher scope.
	if _, _, err := k.Memory().Remember("", memory.RememberSpec{
		Subject: "deploy target", Content: "the shared deploy target is prod-eu",
	}); err != nil {
		t.Fatalf("remember shared: %v", err)
	}
	if _, _, err := k.Memory().Remember("", memory.RememberSpec{
		Subject: "deploy target notes", Content: "researcher-private: prefer the staging mirror",
		Tags: map[string]string{"scope": "researcher"},
	}); err != nil {
		t.Fatalf("remember private: %v", err)
	}

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "researcher", "soul": "You research."},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}

	// Run AS the agent: both the shared fact and the private note inject.
	if _, err := c.Stream(ctx, controlplane.CmdRun,
		map[string]any{"intent": "what is the deploy target?", "agent": "researcher"}, nil); err != nil {
		t.Fatalf("run as agent: %v", err)
	}
	// Plain run: shared only.
	if _, err := c.Stream(ctx, controlplane.CmdRun,
		map[string]any{"intent": "what is the deploy target?"}, nil); err != nil {
		t.Fatalf("plain run: %v", err)
	}

	if len(systems) < 2 {
		t.Fatalf("saw %d provider requests, want 2", len(systems))
	}
	asAgent, plain := systems[0], systems[1]
	if !strings.Contains(asAgent, "researcher-private") {
		t.Errorf("agent run's injected context missing its private note:\n%s", asAgent)
	}
	if !strings.Contains(asAgent, "prod-eu") {
		t.Errorf("agent run's injected context missing shared memory:\n%s", asAgent)
	}
	if strings.Contains(plain, "researcher-private") {
		t.Errorf("plain run leaked a private note:\n%s", plain)
	}
	if !strings.Contains(plain, "prod-eu") {
		t.Errorf("plain run's injected context missing shared memory:\n%s", plain)
	}
}
