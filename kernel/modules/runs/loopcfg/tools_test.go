// SPDX-License-Identifier: MIT

package loopcfg

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
)

// stubTool is a minimal toolapi.Tool for exercising filterTools by name.
type stubTool struct{ name string }

func (s stubTool) Definition() toolapi.ToolDef { return toolapi.ToolDef{Name: s.name} }
func (s stubTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{Output: s.name}, nil
}

func toolSet(names ...string) map[string]toolapi.Tool {
	m := make(map[string]toolapi.Tool, len(names))
	for _, n := range names {
		m[n] = stubTool{name: n}
	}
	return m
}

func sortedKeys(m map[string]toolapi.Tool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestFilterTools(t *testing.T) {
	all := toolSet("read", "write", "shell", "notify")

	cases := []struct {
		name  string
		allow []string
		want  []string
	}{
		{"subset", []string{"read", "notify"}, []string{"notify", "read"}},
		{"single", []string{"read"}, []string{"read"}},
		{"empty allow = no tools", []string{}, []string{}},
		{"nil allow = no tools", nil, []string{}},
		{"unknown name ignored", []string{"read", "ghost"}, []string{"read"}},
		{"all names", []string{"read", "write", "shell", "notify"}, []string{"notify", "read", "shell", "write"}},
		{"duplicate names dedup to one", []string{"read", "read"}, []string{"read"}},
		{"names are trimmed and case-folded", []string{"  READ  "}, []string{"read"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sortedKeys(filterTools(all, tc.allow))
			if len(got) != len(tc.want) {
				t.Fatalf("filterTools(%v) = %v, want %v", tc.allow, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("filterTools(%v) = %v, want %v", tc.allow, got, tc.want)
				}
			}
		})
	}

	// filterTools must not mutate the source map.
	if len(all) != 4 {
		t.Fatalf("source toolset mutated: got %d tools, want 4", len(all))
	}
}

func TestWithTools_RoundTrip(t *testing.T) {
	// No value set: ok=false means "unrestricted".
	if v, ok := runctx.ToolsFromCtx(context.Background()); ok {
		t.Fatalf("bare context: ok=true (%v), want ok=false", v)
	}

	// Explicit non-empty allowlist round-trips.
	ctx := runctx.WithTools(context.Background(), []string{"read", "notify"})
	v, ok := runctx.ToolsFromCtx(ctx)
	if !ok {
		t.Fatal("runctx.WithTools(non-empty): ok=false, want true")
	}
	if len(v) != 2 || v[0] != "read" || v[1] != "notify" {
		t.Fatalf("runctx.ToolsFromCtx = %v, want [read notify]", v)
	}

	// Explicit empty allowlist: ok=true (restriction present) with zero names —
	// distinct from "unset". This is the --no-tools case.
	emptyCtx := runctx.WithTools(context.Background(), []string{})
	ev, eok := runctx.ToolsFromCtx(emptyCtx)
	if !eok {
		t.Fatal("runctx.WithTools(empty): ok=false, want true (restriction is set)")
	}
	if len(ev) != 0 {
		t.Fatalf("runctx.WithTools(empty): got %v, want empty", ev)
	}
	// And filtering by it yields no tools.
	if got := filterTools(toolSet("read", "write"), ev); len(got) != 0 {
		t.Fatalf("filterTools(empty allow) = %v, want no tools", sortedKeys(got))
	}
}

// TestAgentTools_WithoutProfileReturnsTheSetUnchanged pins the no-profile
// contract: without a named agent the tool set is the daemon's own.
func TestAgentTools_WithoutProfileReturnsTheSetUnchanged(t *testing.T) {
	all := toolSet("read", "write")
	got := AgentTools(context.Background(), all)
	if len(got) != 2 {
		t.Fatalf("AgentTools(no profile) = %v, want both tools", sortedKeys(got))
	}
}

// TestAgentTools_AppliesTheProfileAllowlist covers the allow half: the
// profile's tool_allow narrows the surface to what the agent may use.
func TestAgentTools_AppliesTheProfileAllowlist(t *testing.T) {
	ctx := runctx.WithAgentProfile(context.Background(), agentapi.Profile{
		ToolAllow: []string{"read"},
	})
	got := sortedKeys(AgentTools(ctx, toolSet("read", "write", "shell")))
	if len(got) != 1 || got[0] != "read" {
		t.Fatalf("AgentTools(allow=[read]) = %v, want [read]", got)
	}
}

// TestAgentTools_AppliesTheProfileDenylist covers the deny half: the profile's
// tool_deny removes a tool the daemon offers.
func TestAgentTools_AppliesTheProfileDenylist(t *testing.T) {
	ctx := runctx.WithAgentProfile(context.Background(), agentapi.Profile{
		ToolDeny: []string{"shell"},
	})
	got := sortedKeys(AgentTools(ctx, toolSet("read", "shell", "notify")))
	if len(got) != 2 || got[0] != "notify" || got[1] != "read" {
		t.Fatalf("AgentTools(deny=[shell]) = %v, want [notify read]", got)
	}
}

// TestAgentTools_DenyWinsOverAllow pins the precedence: a tool that is both
// allowed and denied is denied — the deny list is the stronger promise.
func TestAgentTools_DenyWinsOverAllow(t *testing.T) {
	ctx := runctx.WithAgentProfile(context.Background(), agentapi.Profile{
		ToolAllow: []string{"read", "shell"},
		ToolDeny:  []string{"shell"},
	})
	got := sortedKeys(AgentTools(ctx, toolSet("read", "shell")))
	if len(got) != 1 || got[0] != "read" {
		t.Fatalf("AgentTools(allow+deny) = %v, want [read]", got)
	}
}

// TestAgentToolPolicyDenial covers the daemon's policy hook question: why is
// this tool refused by the agent's tool policy?
func TestAgentToolPolicyDenial(t *testing.T) {
	cases := []struct {
		name       string
		pol        runctx.AgentToolPolicy
		tool       string
		wantReason string
		wantDenied bool
	}{
		{"denied by denylist", runctx.AgentToolPolicy{Deny: []string{"shell"}}, "shell", "agent tool denylist", true},
		{"not in allowlist", runctx.AgentToolPolicy{Allow: []string{"read"}}, "shell", "not in agent tool allowlist", true},
		{"in allowlist", runctx.AgentToolPolicy{Allow: []string{"read"}}, "read", "", false},
		{"no policy at all", runctx.AgentToolPolicy{}, "shell", "", false},
		{"empty tool name", runctx.AgentToolPolicy{Deny: []string{"shell"}}, "  ", "", false},
		{"case folded", runctx.AgentToolPolicy{Deny: []string{"SHELL"}}, "shell", "agent tool denylist", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason, denied := AgentToolPolicyDenial(tc.pol, tc.tool)
			if denied != tc.wantDenied {
				t.Fatalf("AgentToolPolicyDenial(%q) denied=%v, want %v", tc.tool, denied, tc.wantDenied)
			}
			if reason != tc.wantReason {
				t.Fatalf("AgentToolPolicyDenial(%q) reason=%q, want %q", tc.tool, reason, tc.wantReason)
			}
		})
	}
}

// TestApplyAgentNoisePolicyToPromptTools_PrunesTheMemoryTool covers the noise
// policy's half of the tool-shaping rule: an agent that may not write memory
// does not see the memory tool at all.
func TestApplyAgentNoisePolicyToPromptTools_PrunesTheMemoryTool(t *testing.T) {
	ctx := runctx.WithAgentProfile(context.Background(), agentapi.Profile{
		NoisePolicy: &agentapi.NoisePolicy{DisableMemoryWrites: true},
	})
	got := sortedKeys(applyAgentNoisePolicyToPromptTools(toolSet("memory", "read"), ctx))
	if len(got) != 1 || got[0] != "read" {
		t.Fatalf("noise policy tool set = %v, want [read]", got)
	}
}

// TestApplyAgentNoisePolicyToPromptTools_KeepsTheToolWithoutThePolicy is the
// no-policy case: the tool set is returned unchanged.
func TestApplyAgentNoisePolicyToPromptTools_KeepsTheToolWithoutThePolicy(t *testing.T) {
	got := sortedKeys(applyAgentNoisePolicyToPromptTools(toolSet("memory", "read"), context.Background()))
	if len(got) != 2 {
		t.Fatalf("no noise policy tool set = %v, want both tools", got)
	}
}
