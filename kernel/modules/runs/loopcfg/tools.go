// SPDX-License-Identifier: MIT

package loopcfg

import (
	"context"
	"strings"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
)

// Tool-set shaping for a run: the per-run allowlist filter, the roster
// profile allow/deny policy, and the noise-policy prompt-tool pruning.
//
// These rules moved here from the daemon in W3.76g: they ARE the loop's tool
// surface, and the loop's tool surface is what this module owns. The
// application layer's dry-run plan shapes its tool set with the same rule
// (AgentTools), so the plan cannot list a tool the run would not offer.

func applyAgentNoisePolicyToPromptTools(tools map[string]toolapi.Tool, ctx context.Context) map[string]toolapi.Tool {
	policy, ok := runctx.AgentNoisePolicyFromCtx(ctx)
	if !ok || !policy.DisableMemoryWrites {
		return tools
	}
	if _, ok := tools["memory"]; !ok {
		return tools
	}
	out := make(map[string]toolapi.Tool, len(tools)-1)
	for name, tool := range tools {
		if name != "memory" {
			out[name] = tool
		}
	}
	return out
}

// filterTools returns the subset of tools whose names are in allow (a registered
// name not present is dropped; an allow name with no matching tool is ignored).
// An empty/nil allow yields an empty map — no tools.
func filterTools(tools map[string]toolapi.Tool, allow []string) map[string]toolapi.Tool {
	keep := make(map[string]struct{}, len(allow))
	for _, n := range allow {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			keep[n] = struct{}{}
		}
	}
	out := make(map[string]toolapi.Tool, len(keep))
	for name, tool := range tools {
		if _, ok := keep[strings.ToLower(name)]; ok {
			out[name] = tool
		}
	}
	return out
}

// AgentTools narrows tools to what the run's agent profile allows (its
// tool_allow / tool_deny, applied by runctx.WithAgentProfile). Without a profile the
// set is returned unchanged. The run loop and the dry-run plan both shape the
// tool set with it, so the plan cannot list a tool the run would not offer.
func AgentTools(ctx context.Context, tools map[string]toolapi.Tool) map[string]toolapi.Tool {
	return applyAgentToolPolicy(tools, runctx.AgentToolPolicyFromCtx(ctx))
}

func applyAgentToolPolicy(tools map[string]toolapi.Tool, pol runctx.AgentToolPolicy) map[string]toolapi.Tool {
	out := tools
	if len(pol.Allow) > 0 {
		out = filterTools(out, pol.Allow)
	}
	if len(pol.Deny) == 0 {
		return out
	}
	deny := make(map[string]struct{}, len(pol.Deny))
	for _, name := range pol.Deny {
		if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
			deny[name] = struct{}{}
		}
	}
	next := make(map[string]toolapi.Tool, len(out))
	for name, tool := range out {
		if _, blocked := deny[strings.ToLower(name)]; blocked {
			continue
		}
		next[name] = tool
	}
	return next
}

// AgentToolPolicyDenial explains why the agent's tool policy refuses a tool,
// or reports that it does not. The daemon's policy hook asks it.
func AgentToolPolicyDenial(pol runctx.AgentToolPolicy, toolName string) (string, bool) {
	name := strings.ToLower(strings.TrimSpace(toolName))
	if name == "" {
		return "", false
	}
	for _, denied := range pol.Deny {
		if strings.ToLower(strings.TrimSpace(denied)) == name {
			return "agent tool denylist", true
		}
	}
	if len(pol.Allow) == 0 {
		return "", false
	}
	for _, allowed := range pol.Allow {
		if strings.ToLower(strings.TrimSpace(allowed)) == name {
			return "", false
		}
	}
	return "not in agent tool allowlist", true
}
