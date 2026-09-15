// SPDX-License-Identifier: MIT
//
// cmd/agt agent show sub-command + approval summary. Split from agent.go
// during Day 211 god-file refactor (#41). Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

func cmdAgentShow(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	ref := ""
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else if !strings.HasPrefix(a, "--") && ref == "" {
			ref = a
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s agent show <slug|id> [--json]\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent show: %v\n", brand.CLI, err)
		return 1
	}
	profiles, _ := res["profiles"].([]any)
	for _, raw := range profiles {
		p, _ := raw.(map[string]any)
		if p == nil || (str(p["slug"]) != ref && str(p["id"]) != ref) {
			continue
		}
		if asJSON {
			return jsonout.Write(stdout, p)
		}
		fmt.Fprintf(stdout, "slug:         %s\n", str(p["slug"]))
		fmt.Fprintf(stdout, "id:           %s\n", str(p["id"]))
		fmt.Fprintf(stdout, "name:         %s\n", str(p["name"]))
		state := "enabled"
		if en, _ := p["enabled"].(bool); !en {
			state = "PAUSED"
		}
		fmt.Fprintf(stdout, "state:        %s\n", state)
		if v := str(p["model"]); v != "" {
			fmt.Fprintf(stdout, "model:        %s\n", v)
		}
		if fb, _ := p["fallbacks"].([]any); len(fb) > 0 {
			parts := make([]string, 0, len(fb))
			for _, m := range fb {
				parts = append(parts, str(m))
			}
			fmt.Fprintf(stdout, "fallbacks:    %s\n", strings.Join(parts, " → "))
		}
		if v := str(p["task_type"]); v != "" {
			fmt.Fprintf(stdout, "task type:    %s\n", v)
		}
		if mc, ok := p["max_cost_mc"].(float64); ok && mc > 0 {
			fmt.Fprintf(stdout, "max cost/run: %s\n", fmtUSD(int64(mc)))
		}
		if mc, ok := p["max_daily_mc"].(float64); ok && mc > 0 {
			fmt.Fprintf(stdout, "max cost/day: %s\n", fmtUSD(int64(mc)))
		}
		if v := str(p["memory_scope"]); v != "" {
			fmt.Fprintf(stdout, "memory scope: %s\n", v)
		}
		if v := str(p["workdir"]); v != "" {
			fmt.Fprintf(stdout, "workdir:      %s\n", v)
		}
		if v := str(p["description"]); v != "" {
			fmt.Fprintf(stdout, "description:  %s\n", v)
		}
		if v := str(p["kind"]); v != "" {
			fmt.Fprintf(stdout, "kind:         %s\n", v)
		}
		if v := str(p["owner_agent"]); v != "" {
			fmt.Fprintf(stdout, "owner agent:  %s\n", v)
		}
		if v := str(p["parent_agent"]); v != "" {
			fmt.Fprintf(stdout, "parent agent: %s\n", v)
		}
		if v, ok := p["direct_callable"].(bool); ok {
			fmt.Fprintf(stdout, "direct call:  %v\n", v)
		}
		if v := str(p["trust_ceiling"]); v != "" {
			fmt.Fprintf(stdout, "trust ceiling:%s\n", padValue(v))
		}
		if v := str(p["execution_profile"]); v != "" {
			fmt.Fprintf(stdout, "exec profile: %s\n", v)
		}
		if tools, _ := p["tool_allow"].([]any); len(tools) > 0 {
			fmt.Fprintf(stdout, "tool allow:   %s\n", joinAnyStrings(tools, ", "))
		}
		if tools, _ := p["tool_deny"].([]any); len(tools) > 0 {
			fmt.Fprintf(stdout, "tool deny:    %s\n", joinAnyStrings(tools, ", "))
		}
		if cfg, ok := p["config_overrides"].(map[string]any); ok && len(cfg) > 0 {
			fmt.Fprintf(stdout, "config:       %d override(s)\n", len(cfg))
		}
		if st, ok := p["status"].(map[string]any); ok {
			if state := str(st["operational_state"]); state != "" {
				fmt.Fprintf(stdout, "state:        %s", state)
				if label := str(st["operational_label"]); label != "" && label != state {
					fmt.Fprintf(stdout, " (%s)", label)
				}
				fmt.Fprintln(stdout)
			}
			if active := intNumber(st["active_run_count"]); active > 0 {
				fmt.Fprintf(stdout, "live:         %d running", active)
				if phase := str(st["active_phase"]); phase != "" {
					fmt.Fprintf(stdout, " phase=%q", phase)
				}
				if intent := str(st["active_intent"]); intent != "" {
					fmt.Fprintf(stdout, " intent=%q", intent)
				}
				if detail := str(st["active_detail"]); detail != "" {
					fmt.Fprintf(stdout, " detail=%q", detail)
				}
				if tool := str(st["active_tool"]); tool != "" {
					fmt.Fprintf(stdout, " tool=%s", tool)
				}
				if model := str(st["active_model"]); model != "" {
					fmt.Fprintf(stdout, " model=%s", model)
				}
				if source := str(st["active_wake_source"]); source != "" {
					fmt.Fprintf(stdout, " source=%s", source)
				}
				if sched := str(st["active_schedule_id"]); sched != "" {
					fmt.Fprintf(stdout, " schedule=%s", sched)
				}
				if standing := str(st["active_standing_name"]); standing != "" {
					fmt.Fprintf(stdout, " standing=%q", standing)
				} else if standing := str(st["active_standing_id"]); standing != "" {
					fmt.Fprintf(stdout, " standing=%s", standing)
				}
				if trigger := str(st["active_trigger_subject"]); trigger != "" {
					fmt.Fprintf(stdout, " trigger=%q", trigger)
				}
				if parent := str(st["active_parent_correlation"]); parent != "" {
					fmt.Fprintf(stdout, " parent=%s", parent)
				}
				if corr := str(st["active_correlation_id"]); corr != "" {
					fmt.Fprintf(stdout, " corr=%s", corr)
				}
				fmt.Fprintln(stdout)
			}
			if last := intNumber(st["last_activity_ms"]); last > 0 {
				fmt.Fprintf(stdout, "last active:  %s", time.UnixMilli(int64(last)).Format(time.RFC3339))
				if summary := str(st["last_activity_summary"]); summary != "" {
					fmt.Fprintf(stdout, " %s", summary)
				}
				fmt.Fprintln(stdout)
			}
			if schedules, standings := intNumber(st["wake_schedule_count"]), intNumber(st["wake_standing_count"]); schedules+standings > 0 {
				fmt.Fprintf(stdout, "wake:         %d schedule, %d standing", schedules, standings)
				if label := str(st["next_wake_label"]); label != "" {
					fmt.Fprintf(stdout, " next=%s", label)
				}
				fmt.Fprintln(stdout)
			}
			// Latest autonomy wake contract — which trigger family last woke this
			// agent and under what trigger/route/recovery/sleep posture (M-runbook).
			if rb, ok := st["last_autonomy_runbook"].(map[string]any); ok && len(rb) > 0 {
				var parts []string
				for _, key := range []string{"trigger_contract", "route_contract", "recovery_contract", "sleep_contract"} {
					if v := str(rb[key]); v != "" {
						parts = append(parts, v)
					}
				}
				if len(parts) > 0 {
					fmt.Fprintf(stdout, "wake contract:%s", " "+strings.Join(parts, "/"))
					if src := str(rb["source"]); src != "" {
						fmt.Fprintf(stdout, " via %s", src)
					}
					if phase := str(rb["phase"]); phase != "" {
						fmt.Fprintf(stdout, " (%s)", phase)
					}
					fmt.Fprintln(stdout)
				}
			}
			// Runtime enforcement audit: tool calls policy actually refused.
			if denied := intNumber(st["policy_denied_count"]); denied > 0 {
				fmt.Fprintf(stdout, "tool denials: %d", denied)
				if tool := str(st["policy_denied_last_tool"]); tool != "" {
					fmt.Fprintf(stdout, " last=%s", tool)
				}
				if hard, _ := st["policy_denied_last_hard"].(bool); hard {
					fmt.Fprintf(stdout, " [hard]")
				}
				if reason := str(st["policy_denied_last_reason"]); reason != "" {
					fmt.Fprintf(stdout, " (%s)", reason)
				}
				fmt.Fprintln(stdout)
			}
		}
		printAgentLifecycle(stdout, p["lifecycle"])
		printAgentTaskSummary(stdout, p["tasklist"])
		printAgentPolicy(stdout, "retry", p["retry_policy"])
		printAgentPolicy(stdout, "health", p["health_policy"])
		printAgentPolicy(stdout, "self repair", p["self_repair"])
		printAgentPolicy(stdout, "noise", p["noise_policy"])
		if ins, _ := p["instructions"].([]any); len(ins) > 0 {
			fmt.Fprintf(stdout, "instructions:\n")
			for _, line := range ins {
				if s := str(line); s != "" {
					fmt.Fprintf(stdout, "  - %s\n", s)
				}
			}
		}
		if v := str(p["soul"]); v != "" {
			fmt.Fprintf(stdout, "soul:\n  %s\n", strings.ReplaceAll(v, "\n", "\n  "))
		}
		if sum, ok := agentApprovalSummary(ctx, c, str(p["slug"])); ok {
			fmt.Fprintf(stdout, "approvals:    %d total", sum.Total)
			if sum.Pending > 0 {
				fmt.Fprintf(stdout, " %d pending", sum.Pending)
			}
			if sum.Granted > 0 {
				fmt.Fprintf(stdout, " %d granted", sum.Granted)
			}
			if sum.Denied > 0 {
				fmt.Fprintf(stdout, " %d denied", sum.Denied)
			}
			if sum.Timeout > 0 {
				fmt.Fprintf(stdout, " %d timeout", sum.Timeout)
			}
			if sum.LastStatus != "" {
				fmt.Fprintf(stdout, " last=%s", sum.LastStatus)
				if sum.LastTool != "" {
					fmt.Fprintf(stdout, ":%s", sum.LastTool)
				}
			}
			fmt.Fprintln(stdout)
		}
		return 0
	}
	fmt.Fprintf(stderr, "%s agent show: unknown agent %q\n", brand.CLI, ref)
	return 1
}

type agentApprovalStats struct {
	Total      int
	Granted    int
	Denied     int
	Timeout    int
	Pending    int
	LastStatus string
	LastTool   string
}

func agentApprovalSummary(ctx context.Context, c *controlplane.Client, slug string) (agentApprovalStats, bool) {
	if slug == "" || c == nil {
		return agentApprovalStats{}, false
	}
	res, err := c.Call(ctx, controlplane.CmdApprovalsLog, map[string]any{"limit": 200})
	if err != nil {
		return agentApprovalStats{}, false
	}
	rows, _ := res["approvals"].([]any)
	var out agentApprovalStats
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		if row == nil || str(row["actor"]) != slug {
			continue
		}
		out.Total++
		status := str(row["status"])
		switch status {
		case "granted":
			out.Granted++
		case "denied":
			out.Denied++
		case "timeout":
			out.Timeout++
		default:
			out.Pending++
		}
		if out.LastStatus == "" {
			out.LastStatus = status
			out.LastTool = str(row["tool"])
		}
	}
	return out, out.Total > 0
}
