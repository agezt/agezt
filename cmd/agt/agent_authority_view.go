// SPDX-License-Identifier: MIT
//
// cmd/agt `agent authority` view layer (buildAgentAuthority, renderAgentAuthority).
// Extracted from agent_authority.go during Day 211 god-file refactor (#85).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// agentAuthorityView is the merged effective-authority structure.
type agentAuthorityView struct {
	Slug         string            `json:"slug"`
	TrustCeiling string            `json:"trust_ceiling,omitempty"`
	ToolAllow    []string          `json:"tool_allow,omitempty"`
	ToolDeny     []string          `json:"tool_deny,omitempty"`
	MemoryScope  string            `json:"memory_scope,omitempty"`
	Workdir      string            `json:"workdir,omitempty"`
	DirectCall   bool              `json:"direct_callable"`
	ConfigCount  int               `json:"config_overrides,omitempty"`
	ApprovalMode string            `json:"approval_mode,omitempty"`
	CapLevels    map[string]string `json:"capability_levels,omitempty"`
	HardDeny     []agentHardDeny   `json:"hard_deny,omitempty"`
}

type agentHardDeny struct {
	Name      string `json:"name"`
	Substring string `json:"substring"`
	Scope     string `json:"scope"`
}

func buildAgentAuthority(profile, edict map[string]any) agentAuthorityView {
	v := agentAuthorityView{
		Slug:         str(profile["slug"]),
		TrustCeiling: str(profile["trust_ceiling"]),
		MemoryScope:  str(profile["memory_scope"]),
		Workdir:      str(profile["workdir"]),
		ApprovalMode: str(edict["ask_policy"]),
	}
	if dc, ok := profile["direct_callable"].(bool); ok {
		v.DirectCall = dc
	}
	v.ToolAllow = anyToStringSlice(profile["tool_allow"])
	v.ToolDeny = anyToStringSlice(profile["tool_deny"])
	if cfg, ok := profile["config_overrides"].(map[string]any); ok {
		v.ConfigCount = len(cfg)
	}
	if levels, ok := edict["levels"].(map[string]any); ok {
		v.CapLevels = make(map[string]string)
		for cap, lvl := range levels {
			v.CapLevels[str(cap)] = str(lvl)
		}
	}
	if rules, ok := edict["hard_deny"].([]any); ok {
		for _, raw := range rules {
			r, _ := raw.(map[string]any)
			if r == nil {
				continue
			}
			hd := agentHardDeny{
				Name:      str(r["name"]),
				Substring: str(r["substring"]),
			}
			if caps := anyToStringSlice(r["applies_to"]); len(caps) > 0 {
				hd.Scope = strings.Join(caps, ", ")
			} else {
				hd.Scope = "all capabilities"
			}
			v.HardDeny = append(v.HardDeny, hd)
		}
	}
	return v
}
func renderAgentAuthority(w io.Writer, v agentAuthorityView) {
	fmt.Fprintf(w, "agent:          %s\n", v.Slug)
	fmt.Fprintf(w, "trust ceiling: %s\n", orDash(v.TrustCeiling))
	fmt.Fprintf(w, "direct call:   %v\n", v.DirectCall)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "tool allow:    %s\n", orDash(strings.Join(v.ToolAllow, ", ")))
	fmt.Fprintf(w, "tool deny:     %s\n", orDash(strings.Join(v.ToolDeny, ", ")))
	if v.MemoryScope != "" {
		fmt.Fprintf(w, "memory scope:  %s\n", v.MemoryScope)
	}
	if v.Workdir != "" {
		fmt.Fprintf(w, "workdir:       %s\n", v.Workdir)
	}
	if v.ConfigCount > 0 {
		fmt.Fprintf(w, "config:        %d override(s)\n", v.ConfigCount)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "approval mode: %s\n", orDash(v.ApprovalMode))
	if len(v.CapLevels) > 0 {
		fmt.Fprintf(w, "capability levels:\n")
		caps := make([]string, 0, len(v.CapLevels))
		for c := range v.CapLevels {
			caps = append(caps, c)
		}
		sort.Strings(caps)
		for _, c := range caps {
			fmt.Fprintf(w, "  %-18s %s", c, v.CapLevels[c])
			// If the agent has a trust ceiling and the capability level exceeds
			// it, note the effective cap.
			if v.TrustCeiling != "" && levelExceeds(v.CapLevels[c], v.TrustCeiling) {
				fmt.Fprintf(w, "  (capped to %s)", v.TrustCeiling)
			}
			fmt.Fprintln(w)
		}
	}
	if len(v.HardDeny) > 0 {
		fmt.Fprintf(w, "\nhard-deny floor (%d rules):\n", len(v.HardDeny))
		for _, hd := range v.HardDeny {
			fmt.Fprintf(w, "  %-22s  match=%q  (%s)\n", hd.Name, hd.Substring, hd.Scope)
		}
	}
}
