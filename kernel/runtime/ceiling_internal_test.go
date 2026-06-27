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

// TestWithTrustCeiling_TightenOnly is the regression guard for VULN-001: a
// ceiling may only ever tighten down a delegation tree. Re-applying a LOOSER
// ceiling (as WithAgentProfile does when a delegated sub-agent's profile declares
// a higher TrustCeiling) must NOT loosen an existing tighter cap, or a capped run
// could delegate its way to escaped autonomy (CWE-269).
func TestWithTrustCeiling_TightenOnly(t *testing.T) {
	read := func(ctx context.Context) (edict.TrustLevel, bool) { return trustCeilingFromCtx(ctx) }

	// Tight parent (L1) then a looser child ceiling (L3): the tight one survives.
	ctx := WithTrustCeiling(context.Background(), edict.LevelAsk) // L1
	ctx = WithTrustCeiling(ctx, edict.LevelAskScoped)             // L3 (looser) — must be ignored
	if lvl, ok := read(ctx); !ok || lvl != edict.LevelAsk {
		t.Errorf("looser child ceiling escaped the parent: got (%v,%v), want (L1,true)", lvl, ok)
	}

	// A tighter child ceiling DOES narrow further.
	ctx = WithTrustCeiling(ctx, edict.LevelDeny) // L0 (tighter) — applies
	if lvl, ok := read(ctx); !ok || lvl != edict.LevelDeny {
		t.Errorf("tighter child ceiling should narrow: got (%v,%v), want (L0,true)", lvl, ok)
	}

	// Applying "no clamp" (LevelAllow) must not erase an inherited tighter ceiling.
	ctx2 := WithTrustCeiling(context.Background(), edict.LevelAskFirst) // L2
	ctx2 = WithTrustCeiling(ctx2, edict.LevelAllow)                     // L4 no-op
	if lvl, ok := read(ctx2); !ok || lvl != edict.LevelAskFirst {
		t.Errorf("LevelAllow erased an inherited ceiling: got (%v,%v), want (L2,true)", lvl, ok)
	}
}
