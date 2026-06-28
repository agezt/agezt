// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/agent"

	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// TestRunWith_ExplicitModelBeatsTaskChain (M931): an operator's explicit
// per-run model pick — the Chat picker, `agt run --model`, the OpenAI-compat
// `model` field — must actually serve the run. The governor's per-task chain
// ("chat") supersedes req.Model, so before the fix the pick was silently
// replaced by the chain's models; now the pick travels as the per-request
// ModelChain, which wins over the task chain (M787 precedence).
func TestRunWith_ExplicitModelBeatsTaskChain(t *testing.T) {
	var served []string
	prov := &mock.Provider{Responder: func(agent.CompletionRequest) agent.CompletionResponse {
		return mock.FinalText("ok")
	}}
	prov.OnRequest = func(r agent.CompletionRequest) { served = append(served, r.Model) }

	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{
		Name: "mock", Provider: prov, AuthMode: governor.AuthLocal,
	}); err != nil {
		t.Fatal(err)
	}
	gov, err := governor.New(governor.Config{
		Registry: reg,
		// The trap: chat has a configured chain that used to clobber req.Model.
		TaskModelChains: governor.TaskModelChains{"chat": {"chain-primary", "chain-fallback"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: gov,
		Model:    "default-model",
		Tools:    map[string]agent.Tool{"shell": shell.NewWithWarden(warden.New(nil))},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	// No override → the chat chain serves (pre-M931 behaviour, unchanged).
	if _, err := k.RunWith(context.Background(), k.NewCorrelation(), "hi"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if len(served) == 0 || served[0] != "chain-primary" {
		t.Fatalf("without an override the chat chain must serve: got %v, want chain-primary first", served)
	}

	// Explicit pick → exactly that model, not the chain.
	served = nil
	ctx := runtime.WithModel(context.Background(), "my-explicit-pick")
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "hi again"); err != nil {
		t.Fatalf("RunWith with override: %v", err)
	}
	if len(served) == 0 {
		t.Fatal("provider never called")
	}
	for _, m := range served {
		if m != "my-explicit-pick" {
			t.Errorf("served model %q, want my-explicit-pick (the chat chain must not clobber an explicit pick)", m)
		}
	}
}
