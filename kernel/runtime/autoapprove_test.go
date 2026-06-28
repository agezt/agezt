// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type autoApproveProbeTool struct{ invoked *int32 }

func autoApproveToolUse(callID, toolName string, input any) agent.CompletionResponse {
	raw, err := json.Marshal(input)
	if err != nil {
		panic("autoApproveToolUse: marshal input: " + err.Error())
	}
	return agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{{ID: callID, Name: toolName, Input: raw}},
		},
		StopReason: agent.StopToolUse,
	}
}

func (t autoApproveProbeTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:        "approvalprobe",
		Description: "approval probe",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Effect: agent.ToolEffect{
			Class:             agent.EffectCompensable,
			PredictedEffects:  []string{"probe action"},
			AffectedResources: []string{"resource:probe"},
			RollbackNotes:     "test only",
			Confidence:        0.9,
		},
	}
}

func (t autoApproveProbeTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	atomic.AddInt32(t.invoked, 1)
	return agent.Result{Output: "ok"}, nil
}

func TestAutoApproveCapabilitiesContext(t *testing.T) {
	base := context.Background()

	// No grant → nothing auto-approves.
	if autoApproveCap(base, "tool.forge") {
		t.Fatal("empty context must not auto-approve")
	}

	// Empty set is a no-op (context unchanged, no auto-approve).
	if got := WithAutoApproveCapabilities(base, nil); got != base {
		t.Fatal("nil caps should leave the context unchanged")
	}
	if got := WithAutoApproveCapabilities(base, map[string]bool{}); got != base {
		t.Fatal("empty caps should leave the context unchanged")
	}

	// A grant covers exactly its listed capabilities and rides the context (so it
	// reaches every sub-agent the run spawns, which inherit context values).
	ctx := WithAutoApproveCapabilities(base, map[string]bool{"tool.forge": true, "code.exec": true})
	if !autoApproveCap(ctx, "tool.forge") {
		t.Fatal("granted tool.forge should auto-approve")
	}
	if !autoApproveCap(ctx, "code.exec") {
		t.Fatal("granted code.exec should auto-approve")
	}
	if autoApproveCap(ctx, "shell") {
		t.Fatal("ungranted capability must still require approval")
	}
}

func TestRunWith_ConfigAutoApproveCapabilitiesSatisfiesPromptMode(t *testing.T) {
	var invoked int32
	reg := approval.New(approval.Config{Timeout: time.Second})
	k, err := Open(Config{
		BaseDir: t.TempDir(),
		Provider: mock.New(
			autoApproveToolUse("probe-1", "approvalprobe", map[string]any{}),
			mock.FinalText("done"),
		),
		Tools: map[string]agent.Tool{"approvalprobe": autoApproveProbeTool{invoked: &invoked}},
		Edict: edict.New(edict.Options{
			Levels:    map[edict.Capability]edict.TrustLevel{"approvalprobe": edict.LevelAsk},
			AskPolicy: edict.AskPrompt,
		}),
		Approvals:               reg,
		AutoApproveCapabilities: map[string]bool{"approvalprobe": true},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Run(context.Background(), "probe"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt32(&invoked); got != 1 {
		t.Fatalf("probe invoked %d times, want 1", got)
	}
	if pending := reg.Pending(); len(pending) != 0 {
		t.Fatalf("auto-approved run left %d pending approval(s)", len(pending))
	}
}

func TestParsePromptInjectionModeDefaultsToWarn(t *testing.T) {
	for _, raw := range []string{"", "nonsense", "warn", "audit"} {
		if got := ParsePromptInjectionMode(raw); got != PromptInjectionWarn {
			t.Fatalf("ParsePromptInjectionMode(%q) = %v, want warn", raw, got)
		}
	}
	for _, raw := range []string{"on", "block", "prompt", "1"} {
		if got := ParsePromptInjectionMode(raw); got != PromptInjectionOn {
			t.Fatalf("ParsePromptInjectionMode(%q) = %v, want on", raw, got)
		}
	}
	if got := ParsePromptInjectionMode("off"); got != PromptInjectionOff {
		t.Fatalf("off = %v, want off", got)
	}
}
