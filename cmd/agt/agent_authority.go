// SPDX-License-Identifier: MIT
//
// cmd/agt agent authority + impact sub-commands. Split from agent.go
// during Day 211 god-file refactor (#41). Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

func cmdAgentAuthority(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	ref := ""
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "--explain":
			// Accepted for documentation parity with docs/COMPARISON.md; the
			// text render IS the explain form, so this is a no-op alias.
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s agent authority <slug|id> [--json] [--explain]\n", brand.CLI)
			fmt.Fprintf(stdout, "show the effective runtime authority for an agent (merged profile + policy)\n")
			return 0
		default:
			if !strings.HasPrefix(a, "--") && ref == "" {
				ref = a
			}
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s agent authority <slug|id> [--json] [--explain]\n", brand.CLI)
		return 2
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Fetch the agent profile.
	listRes, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent authority: %v\n", brand.CLI, err)
		return 1
	}
	var profile map[string]any
	if profiles, _ := listRes["profiles"].([]any); profiles != nil {
		for _, raw := range profiles {
			p, _ := raw.(map[string]any)
			if p != nil && (str(p["slug"]) == ref || str(p["id"]) == ref) {
				profile = p
				break
			}
		}
	}
	if profile == nil {
		fmt.Fprintf(stderr, "%s agent authority: unknown agent %q\n", brand.CLI, ref)
		return 1
	}

	// Fetch the live Edict policy snapshot (levels + hard-deny + approval mode).
	edictRes, err := c.Call(ctx, controlplane.CmdEdictShow, nil)
	if err != nil {
		// The daemon might be an older version without CmdEdictShow; degrade
		// gracefully and note the policy overlay is unavailable.
		edictRes = map[string]any{}
	}

	// Build the merged authority view.
	authority := buildAgentAuthority(profile, edictRes)

	if asJSON {
		return jsonout.Write(stdout, authority)
	}
	renderAgentAuthority(stdout, authority)
	return 0
}

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

func anyToStringSlice(v any) []string {
	switch xs := v.(type) {
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			if s := str(x); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return xs
	default:
		return nil
	}
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

// levelExceeds reports whether level is stronger than ceiling. Trust levels are
// ordered L0 < L1 < L2 < L3 < L4, with named aliases deny=L0, ask=L1,
// askfirst=L2, askscoped=L3, allow=L4. This is a lexical comparison of the
// numeric form only; named aliases are normalized first.
func levelExceeds(level, ceiling string) bool {
	return levelRank(level) > levelRank(ceiling)
}

func levelRank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "l0", "deny", "":
		return 0
	case "l1", "ask":
		return 1
	case "l2", "askfirst":
		return 2
	case "l3", "askscoped":
		return 3
	case "l4", "allow":
		return 4
	default:
		return 0
	}
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func cmdAgentImpact(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s agent impact <slug|id>\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentImpact, map[string]any{"ref": args[0]})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent impact: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "agent %s lifecycle impact\n", str(res["slug"]))
	if !printAgentImpactSummary(stdout, res) {
		fmt.Fprintln(stdout, "impact: none")
	}
	return 0
}
