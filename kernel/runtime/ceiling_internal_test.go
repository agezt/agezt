// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestPolicyHook_TrustCeiling: a context carrying WithTrustCeiling clamps the
// policy decision (SPEC-16 §4 / M408). White-box so we can call policyHook
// directly with a shell call that the engine would normally auto-allow.
func TestPolicyHook_TrustCeiling(t *testing.T) {
	eng := edict.New(edict.Options{
		Levels:    map[edict.Capability]edict.TrustLevel{edict.CapShell: edict.LevelAllow},
		AskPolicy: edict.AskAllow,
	})
	k, err := Open(Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
		Edict:    eng,
		Tools:    map[string]agent.Tool{},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	call := agent.ToolCall{Name: "shell", Input: []byte(`{"command":"echo hi"}`)}

	// No ceiling → the L4 capability is auto-allowed.
	if v := k.policyHook(context.Background(), call); !v.Allow {
		t.Fatalf("without a ceiling the L4 shell call should be allowed, got %+v", v)
	}

	// Ceiling L0 → denied, with a reason naming the clamp.
	ctx := WithTrustCeiling(context.Background(), edict.LevelDeny)
	v := k.policyHook(ctx, call)
	if v.Allow {
		t.Errorf("ceiling L0 should deny the shell call, got allow")
	}
	if !strings.Contains(v.Reason, "ceiling") {
		t.Errorf("denied reason should mention the ceiling, got %q", v.Reason)
	}
}
