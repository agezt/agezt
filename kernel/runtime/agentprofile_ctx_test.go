// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// TestWithAgentProfile_AppliesIdentityToRun: the one-call profile application
// (M790 — used by the standing runner) carries the whole identity into the
// run: soul → system, model + fallbacks → model chain, memory scope → private
// notes in the injected context.
func TestWithAgentProfile_AppliesIdentityToRun(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	var req agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { req = r }
	k, err := runtime.Open(runtime.Config{
		BaseDir:      t.TempDir(),
		Provider:     prov,
		Model:        "default-model",
		Tools:        map[string]agent.Tool{"shell": shell.New()},
		MemoryInject: true,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Memory().Remember("", memory.RememberSpec{
		Subject: "target notes", Content: "researcher-private-fact",
		Tags: map[string]string{"scope": "researcher"},
	}); err != nil {
		t.Fatalf("remember: %v", err)
	}

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug: "researcher", Soul: "You are Researcher.",
		Model: "agent-model", Fallbacks: []string{"agent-model", "backup-1"},
	})
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "what about the target?"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}

	if req.Model != "agent-model" {
		t.Errorf("model = %q, want agent-model", req.Model)
	}
	if len(req.ModelChain) != 2 || req.ModelChain[0] != "agent-model" || req.ModelChain[1] != "backup-1" {
		t.Errorf("chain = %v, want [agent-model backup-1] (dupe skipped)", req.ModelChain)
	}
	if !strings.Contains(req.System, "You are Researcher.") {
		t.Errorf("soul missing from system:\n%s", req.System)
	}
	if !strings.Contains(req.System, "researcher-private-fact") {
		t.Errorf("memory scope not applied — private note missing:\n%s", req.System)
	}
}
