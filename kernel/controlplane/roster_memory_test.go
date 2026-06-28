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
	"github.com/agezt/agezt/kernel/warden"
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
		Tools:        map[string]agent.Tool{"shell": shell.NewWithWarden(warden.New(nil))},
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

// TestRun_AsAgent_ModelChain proves the M787 chain end to end over the wire:
// a run AS a named agent with fallbacks carries [primary, fallbacks...] as the
// request's ModelChain; an explicit --model heads the chain; a plain run
// carries none.
func TestRun_AsAgent_ModelChain(t *testing.T) {
	prov := mock.New(
		mock.FinalText("run1"),
		mock.FinalText("run2"),
		mock.FinalText("run3"),
	)
	var chains [][]string
	prov.OnRequest = func(req agent.CompletionRequest) { chains = append(chains, req.ModelChain) }
	_, _, c, _ := startPair(t, prov)
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug": "researcher", "model": "agent-model",
			"fallbacks": []any{"backup-1", "backup-2"},
		},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}

	for _, args := range []map[string]any{
		{"intent": "x", "agent": "researcher"},
		{"intent": "x", "agent": "researcher", "model": "explicit-model"},
		{"intent": "x"},
	} {
		if _, err := c.Stream(ctx, controlplane.CmdRun, args, nil); err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	if len(chains) < 3 {
		t.Fatalf("saw %d requests, want 3", len(chains))
	}
	want := []string{"agent-model", "backup-1", "backup-2"}
	if !equalStrings(chains[0], want) {
		t.Errorf("agent run chain = %v, want %v", chains[0], want)
	}
	wantExplicit := []string{"explicit-model", "backup-1", "backup-2"}
	if !equalStrings(chains[1], wantExplicit) {
		t.Errorf("explicit-model chain = %v, want %v (explicit heads the chain)", chains[1], wantExplicit)
	}
	if len(chains[2]) != 0 {
		t.Errorf("plain run carries a chain: %v", chains[2])
	}
}

// TestStanding_AgentFieldRoundTrips: a standing order created with an agent
// (M790) keeps it through add → list → edit (set + clear) over the wire.
func TestStanding_AgentFieldRoundTrips(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	for _, slug := range []string{"researcher", "ops"} {
		if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
			"profile": map[string]any{"slug": slug, "soul": "Run standing work.", "model": "m"},
		}); err != nil {
			t.Fatalf("agent add %s: %v", slug, err)
		}
	}

	add, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{
		"order": map[string]any{
			"name":     "inbox answerer",
			"triggers": []any{map[string]any{"type": "event", "subject": "board.dm.researcher"}},
			"plan":     "read your inbox and reply",
			"agent":    "researcher",
		},
	})
	if err != nil {
		t.Fatalf("standing add: %v", err)
	}
	o, _ := add["order"].(map[string]any)
	if o["agent"] != "researcher" {
		t.Fatalf("add lost the agent: %v", o)
	}
	id, _ := o["id"].(string)

	edit, err := c.Call(ctx, controlplane.CmdStandingEdit, map[string]any{"id": id, "agent": "ops"})
	if err != nil {
		t.Fatalf("standing edit: %v", err)
	}
	if eo, _ := edit["order"].(map[string]any); eo["agent"] != "ops" {
		t.Fatalf("edit didn't set the agent: %v", edit)
	}
	clear, err := c.Call(ctx, controlplane.CmdStandingEdit, map[string]any{"id": id, "agent": ""})
	if err != nil {
		t.Fatalf("standing clear: %v", err)
	}
	if co, _ := clear["order"].(map[string]any); co["agent"] != nil {
		t.Fatalf("empty agent should clear (omitempty): %v", clear)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
