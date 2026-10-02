// SPDX-License-Identifier: MIT

package toolapi

import (
	"context"
	"testing"
)

// TestWithWorkdirRefusesEscapes pins the defense-in-depth half of the per-agent
// workdir: whatever the roster validated, a tool must never be handed an
// absolute or parent-escaping directory through the invocation context.
func TestWithWorkdirRefusesEscapes(t *testing.T) {
	ctx := context.Background()
	for _, bad := range []string{"", "  ", "/etc", "..", "../x", "a/../../b", "a/.."} {
		if got := WorkdirFromContext(WithWorkdir(ctx, bad)); got != "" {
			t.Errorf("WithWorkdir(%q) carried %q, want refused", bad, got)
		}
	}
	for in, want := range map[string]string{"agents/researcher": "agents/researcher", ` notes `: "notes"} {
		if got := WorkdirFromContext(WithWorkdir(ctx, in)); got != want {
			t.Errorf("WithWorkdir(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestInvocationContextRoundTrip: empty values leave the context untouched, set
// values read back, and a bare context reads as "no run / default identity".
func TestInvocationContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	if WithCorrelation(ctx, "") != ctx || WithAgent(ctx, "") != ctx {
		t.Fatal("empty correlation/agent must leave the context unchanged")
	}
	if CorrelationFromContext(ctx) != "" || AgentFromContext(ctx) != "" || WorkdirFromContext(ctx) != "" {
		t.Fatal("bare context must carry nothing")
	}
	ctx = WithAgent(WithCorrelation(ctx, "corr-1"), "researcher")
	if CorrelationFromContext(ctx) != "corr-1" || AgentFromContext(ctx) != "researcher" {
		t.Fatalf("round trip lost a value: corr=%q agent=%q", CorrelationFromContext(ctx), AgentFromContext(ctx))
	}
}
