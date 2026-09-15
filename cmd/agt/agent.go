// SPDX-License-Identifier: MIT
//
// cmd/agt agent sub-command top-level handlers + small helpers
// (cmdAgent, agentUsage, agentListStateLabel, agentListStatusSuffix).
// Extracted from agent.go during Day 211 god-file refactor (#41, #60).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

func cmdAgent(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return agentUsage(stderr)
	}
	switch args[0] {
	case "list":
		return cmdAgentList(args[1:], stdout, stderr)
	case "add", "create":
		return cmdAgentAdd(args[1:], stdout, stderr)
	case "show":
		return cmdAgentShow(args[1:], stdout, stderr)
	case "authority":
		return cmdAgentAuthority(args[1:], stdout, stderr)
	case "impact":
		return cmdAgentImpact(args[1:], stdout, stderr)
	case "tombstone":
		return cmdAgentTombstone(args[1:], stdout, stderr)
	case "graveyard":
		return cmdAgentGraveyard(args[1:], stdout, stderr)
	case "set", "edit":
		return cmdAgentSet(args[1:], stdout, stderr)
	case "task", "tasks":
		return cmdAgentTask(args[1:], stdout, stderr)
	case "wake":
		return cmdAgentWake(args[1:], stdout, stderr)
	case "repair":
		return cmdAgentRepair(args[1:], stdout, stderr)
	case "repair-status":
		return cmdAgentRepairStatus(args[1:], stdout, stderr)
	case "pause":
		return cmdAgentSetEnabled(args[1:], stdout, stderr, false)
	case "resume":
		return cmdAgentSetEnabled(args[1:], stdout, stderr, true)
	case "retire":
		return cmdAgentRetire(args[1:], stdout, stderr)
	case "revive":
		return cmdAgentRevive(args[1:], stdout, stderr)
	case "remove", "rm":
		return cmdAgentRemove(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		return agentUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s agent: unknown subcommand %q\n", brand.CLI, args[0])
		return agentUsage(stderr)
	}
}
func agentUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s agent <list|add|show|impact|tombstone|graveyard|set|task|wake|repair|repair-status|pause|resume|retire|revive|remove>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--json]                                  show the agent roster\n")
	fmt.Fprintf(w, "  add <slug> [--name N] [--soul TEXT] [--model M] [--fallbacks m1,m2]\n")
	fmt.Fprintf(w, "      [--task TYPE] [--max-cost USD] [--max-daily USD] [--memory-scope S] [--workdir DIR] [--desc TEXT]\n")
	fmt.Fprintf(w, "      [--owner-agent SLUG] [--parent-agent SLUG] [--direct-callable true|false]\n")
	fmt.Fprintf(w, "      [--instructions TEXT] [--tool-allow csv] [--tool-deny csv] [--trust-ceiling L0..L4] [--exec PROFILE] [--config KEY=VALUE,...]\n")
	fmt.Fprintf(w, "      [--lifecycle persistent|cycle|retire_on_complete] [--max-cycles N] [--cycle-task TEXT] [--total-task TEXT]\n")
	fmt.Fprintf(w, "      [--retry-attempts N] [--retry-backoff fixed|exponential] [--retry-base-sec N] [--retry-max-sec N] [--retry-on csv]\n")
	fmt.Fprintf(w, "      [--doctor-agent SLUG] [--health-stale-sec N] [--health-window N] [--health-threshold N]\n")
	fmt.Fprintf(w, "      [--self-repair true|false] [--self-repair-attempts N] [--self-repair-escalate SLUG]\n")
	fmt.Fprintf(w, "      [--silent-on-success true|false] [--disable-memory-writes true|false] [--notify-min-severity info|warning|critical]\n")
	fmt.Fprintf(w, "      [--notify-cooldown-sec N]\n")
	fmt.Fprintf(w, "  show <slug|id> [--json]                        one agent's full profile\n")
	fmt.Fprintf(w, "  authority <slug|id> [--json]                   effective runtime authority: tools, trust ceiling, memory, policy overlay\n")
	fmt.Fprintf(w, "  impact <slug|id>                               show lifecycle dependencies before retire/remove\n")
	fmt.Fprintf(w, "  tombstone <slug|id> [--json]                   read-only death certificate: identity, retirement, resource footprint\n")
	fmt.Fprintf(w, "  graveyard [--older-than DAYS] [--json]         list retired agents by age (retention-eligibility view; reports only)\n")
	fmt.Fprintf(w, "  set <slug|id> [same flags as add]              edit an agent (slug is immutable)\n")
	fmt.Fprintf(w, "  task add <slug|id> <title> [--id ID] [--scope cycle|total] [--status todo|doing|done|blocked|retired] [--desc TEXT]\n")
	fmt.Fprintf(w, "  task set <slug|id> <task-id> [--title TEXT] [--scope cycle|total] [--status ...] [--desc TEXT]\n")
	fmt.Fprintf(w, "  task <done|doing|todo|blocked|retired|remove> <slug|id> <task-id>\n")
	fmt.Fprintf(w, "  wake <slug|id> [intent text] [--reason TEXT] [--incident ID] [--root ID] [--parent ID]\n")
	fmt.Fprintf(w, "  repair <slug|id> [--reason TEXT] [--incident ID] [--root ID] [--parent ID]\n")
	fmt.Fprintf(w, "  repair-status <slug|id> [--limit N]          show repair history and inflight work\n")
	fmt.Fprintf(w, "  pause <slug|id>                                disable an agent (runs refused)\n")
	fmt.Fprintf(w, "  resume <slug|id>                               re-enable an agent\n")
	fmt.Fprintf(w, "  retire <slug|id> [reason]                      move a dead agent to the graveyard (paused; delegation refused)\n")
	fmt.Fprintf(w, "  revive <slug|id>                               bring a retired agent back from the graveyard\n")
	fmt.Fprintf(w, "  remove <slug|id> [--with-all|--with-schedules|--with-memory|--with-authored-memory|--with-skills|--with-config|--with-workspace|--with-standing|--with-subagents]\n")
	fmt.Fprintf(w, "                                                   delete an agent and optionally clean related state\n")
	fmt.Fprintf(w, "notes:\n")
	fmt.Fprintf(w, "  managed sub-agents (kind=subagent or --direct-callable false) are woken/repaired through their parent/owner agent\n")
	fmt.Fprintf(w, "  --with-memory cleans private memory; --with-authored-memory cleans shared memory records written by the agent\n")
	fmt.Fprintf(w, "  --with-subagents retires dependent sub-agents before parent removal; it does not hard-delete child profiles\n")
	fmt.Fprintf(w, "run as an agent:  %s run --agent <slug> \"intent\"\n", brand.CLI)
	return 0
}
func agentListStateLabel(p map[string]any) string {
	if retired, _ := p["retired"].(bool); retired {
		return "RETIRED"
	}
	if en, _ := p["enabled"].(bool); !en {
		return "PAUSED"
	}
	return "enabled"
}
func agentListStatusSuffix(p map[string]any) string {
	st, _ := p["status"].(map[string]any)
	if st == nil {
		return ""
	}
	parts := []string{}
	if active := intNumber(st["active_run_count"]); active > 0 {
		live := fmt.Sprintf("live=%d", active)
		if phase := str(st["active_phase"]); phase != "" {
			live += ":" + phase
		}
		if tool := str(st["active_tool"]); tool != "" {
			live += "[" + tool + "]"
		}
		if model := str(st["active_model"]); model != "" {
			live += "#" + model
		}
		if source := str(st["active_wake_source"]); source != "" {
			live += "@" + source
		}
		if standing := str(st["active_standing_name"]); standing != "" {
			live += "(" + standing + ")"
		} else if schedule := str(st["active_schedule_id"]); schedule != "" {
			live += "(" + schedule + ")"
		}
		parts = append(parts, live)
	}
	if active := intNumber(st["active_run_count"]); active == 0 {
		if schedules, standings := intNumber(st["wake_schedule_count"]), intNumber(st["wake_standing_count"]); schedules+standings > 0 {
			wake := fmt.Sprintf("wake=%d schedule/%d standing", schedules, standings)
			if label := str(st["next_wake_label"]); label != "" {
				wake += "(" + label + ")"
			}
			parts = append(parts, wake)
		}
	}
	// Compact runtime-enforcement flag so a fleet scan shows which agents hit a
	// policy wall (full detail in `agt agent show`).
	if denied := intNumber(st["policy_denied_count"]); denied > 0 {
		parts = append(parts, fmt.Sprintf("denied=%d", denied))
	}
	return strings.Join(parts, " ")
}
