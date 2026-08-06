// SPDX-License-Identifier: MIT

// Tool-set shaping for a run: the per-run allowlist filter, the roster
// profile allow/deny policy, and the noise-policy prompt-tool pruning.
// Split from runtime.go (Phase 3.1 A2).

package runtime

import (
	"context"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)

func applyAgentNoisePolicyToPromptTools(tools map[string]agent.Tool, ctx context.Context) map[string]agent.Tool {
	policy, ok := agentNoisePolicyFromCtx(ctx)
	if !ok || !policy.disableMemoryWrites {
		return tools
	}
	if _, ok := tools["memory"]; !ok {
		return tools
	}
	out := make(map[string]agent.Tool, len(tools)-1)
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
func filterTools(tools map[string]agent.Tool, allow []string) map[string]agent.Tool {
	keep := make(map[string]struct{}, len(allow))
	for _, n := range allow {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			keep[n] = struct{}{}
		}
	}
	out := make(map[string]agent.Tool, len(keep))
	for name, tool := range tools {
		if _, ok := keep[strings.ToLower(name)]; ok {
			out[name] = tool
		}
	}
	return out
}

func applyAgentToolPolicy(tools map[string]agent.Tool, pol agentToolPolicy) map[string]agent.Tool {
	out := tools
	if len(pol.allow) > 0 {
		out = filterTools(out, pol.allow)
	}
	if len(pol.deny) == 0 {
		return out
	}
	deny := make(map[string]struct{}, len(pol.deny))
	for _, name := range pol.deny {
		if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
			deny[name] = struct{}{}
		}
	}
	next := make(map[string]agent.Tool, len(out))
	for name, tool := range out {
		if _, blocked := deny[strings.ToLower(name)]; blocked {
			continue
		}
		next[name] = tool
	}
	return next
}

func agentToolPolicyDenial(pol agentToolPolicy, toolName string) (string, bool) {
	name := strings.ToLower(strings.TrimSpace(toolName))
	if name == "" {
		return "", false
	}
	for _, denied := range pol.deny {
		if strings.ToLower(strings.TrimSpace(denied)) == name {
			return "agent tool denylist", true
		}
	}
	if len(pol.allow) == 0 {
		return "", false
	}
	for _, allowed := range pol.allow {
		if strings.ToLower(strings.TrimSpace(allowed)) == name {
			return "", false
		}
	}
	return "not in agent tool allowlist", true
}
