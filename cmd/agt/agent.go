// SPDX-License-Identifier: MIT
//
// cmd/agt agent entry + flags + list sub-command. Split from agent.go
// during Day 211 god-file refactor (#41). Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
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

// agentFlags parses the shared add/set flag set. Returns the profile fields
// that were explicitly provided (set tracks which, for partial edits).
type agentFlags struct {
	name, soul, model, task, memScope, workdir, desc, ownerAgent, parentAgent string
	retryBackoff, retryOn, doctorAgent, selfRepairEscalate                    string
	notifyMinSeverity                                                         string
	instructions, toolAllow, toolDeny, trustCeiling, configOverrides          string
	executionProfile                                                          string
	lifecycleMode                                                             string
	fallbacks                                                                 []string
	cycleTasks, totalTasks                                                    []string
	maxCostMc, maxDailyMc                                                     int64
	lifecycleMaxCycles                                                        int
	retryAttempts, retryBaseSec, retryMaxSec                                  int
	healthStaleSec, healthWindow, healthThreshold                             int
	selfRepairAttempts                                                        int
	notifyCooldownSec                                                         int
	directCallable                                                            bool
	selfRepairEnabled                                                         bool
	silentOnSuccess, disableMemoryWrites                                      bool
	set                                                                       map[string]bool
}

func parseAgentFlags(args []string, stderr io.Writer, cmd string) (agentFlags, []string, bool) {
	f := agentFlags{set: map[string]bool{}}
	var rest []string
	need := func(i int, flag string) bool {
		if i+1 >= len(args) {
			fmt.Fprintf(stderr, "%s agent %s: %s needs a value\n", brand.CLI, cmd, flag)
			return false
		}
		return true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--name":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.name, f.set["name"] = args[i], true
		case "--soul":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.soul, f.set["soul"] = args[i], true
		case "--model":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.model, f.set["model"] = args[i], true
		case "--fallbacks":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			for m := range strings.SplitSeq(args[i], ",") {
				if m = strings.TrimSpace(m); m != "" {
					f.fallbacks = append(f.fallbacks, m)
				}
			}
			f.set["fallbacks"] = true
		case "--task":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.task, f.set["task"] = args[i], true
		case "--max-cost":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			mc, err := parseUSDToMicrocents(args[i])
			if err != nil {
				fmt.Fprintf(stderr, "%s agent %s: invalid --max-cost %q (want a dollar amount like 0.50)\n", brand.CLI, cmd, args[i])
				return f, nil, false
			}
			f.maxCostMc, f.set["max_cost"] = mc, true
		case "--max-daily":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			mc, err := parseUSDToMicrocents(args[i])
			if err != nil {
				fmt.Fprintf(stderr, "%s agent %s: invalid --max-daily %q (want a dollar amount like 5.00)\n", brand.CLI, cmd, args[i])
				return f, nil, false
			}
			f.maxDailyMc, f.set["max_daily"] = mc, true
		case "--memory-scope":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.memScope, f.set["memory_scope"] = args[i], true
		case "--workdir":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.workdir, f.set["workdir"] = args[i], true
		case "--desc":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.desc, f.set["desc"] = args[i], true
		case "--owner-agent":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.ownerAgent, f.set["owner_agent"] = args[i], true
		case "--parent-agent":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.parentAgent, f.set["parent_agent"] = args[i], true
		case "--direct-callable":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			v, err := strconv.ParseBool(args[i])
			if err != nil {
				fmt.Fprintf(stderr, "%s agent %s: invalid --direct-callable %q (want true or false)\n", brand.CLI, cmd, args[i])
				return f, nil, false
			}
			f.directCallable, f.set["direct_callable"] = v, true
		case "--instructions":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.instructions, f.set["instructions"] = args[i], true
		case "--tool-allow":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.toolAllow, f.set["tool_allow"] = args[i], true
		case "--tool-deny":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.toolDeny, f.set["tool_deny"] = args[i], true
		case "--trust-ceiling":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.trustCeiling, f.set["trust_ceiling"] = args[i], true
		case "--execution-profile", "--exec":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.executionProfile, f.set["execution_profile"] = args[i], true
		case "--config":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.configOverrides, f.set["config_overrides"] = args[i], true
		case "--lifecycle":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.lifecycleMode, f.set["lifecycle"] = args[i], true
		case "--max-cycles":
			n, ok := parseNonNegativeFlag(args, &i, a, stderr, cmd)
			if !ok {
				return f, nil, false
			}
			f.lifecycleMaxCycles, f.set["max_cycles"] = n, true
		case "--cycle-task":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.cycleTasks = append(f.cycleTasks, splitList(args[i])...)
			f.set["cycle_tasks"] = true
		case "--total-task":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.totalTasks = append(f.totalTasks, splitList(args[i])...)
			f.set["total_tasks"] = true
		case "--retry-attempts":
			n, ok := parseNonNegativeFlag(args, &i, a, stderr, cmd)
			if !ok {
				return f, nil, false
			}
			f.retryAttempts, f.set["retry_attempts"] = n, true
		case "--retry-backoff":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.retryBackoff, f.set["retry_backoff"] = args[i], true
		case "--retry-base-sec":
			n, ok := parseNonNegativeFlag(args, &i, a, stderr, cmd)
			if !ok {
				return f, nil, false
			}
			f.retryBaseSec, f.set["retry_base_sec"] = n, true
		case "--retry-max-sec":
			n, ok := parseNonNegativeFlag(args, &i, a, stderr, cmd)
			if !ok {
				return f, nil, false
			}
			f.retryMaxSec, f.set["retry_max_sec"] = n, true
		case "--retry-on":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.retryOn, f.set["retry_on"] = args[i], true
		case "--doctor-agent":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.doctorAgent, f.set["doctor_agent"] = args[i], true
		case "--health-stale-sec":
			n, ok := parseNonNegativeFlag(args, &i, a, stderr, cmd)
			if !ok {
				return f, nil, false
			}
			f.healthStaleSec, f.set["health_stale_sec"] = n, true
		case "--health-window":
			n, ok := parseNonNegativeFlag(args, &i, a, stderr, cmd)
			if !ok {
				return f, nil, false
			}
			f.healthWindow, f.set["health_window"] = n, true
		case "--health-threshold":
			n, ok := parseNonNegativeFlag(args, &i, a, stderr, cmd)
			if !ok {
				return f, nil, false
			}
			f.healthThreshold, f.set["health_threshold"] = n, true
		case "--self-repair":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			v, err := strconv.ParseBool(args[i])
			if err != nil {
				fmt.Fprintf(stderr, "%s agent %s: invalid --self-repair %q (want true or false)\n", brand.CLI, cmd, args[i])
				return f, nil, false
			}
			f.selfRepairEnabled, f.set["self_repair"] = v, true
		case "--self-repair-attempts":
			n, ok := parseNonNegativeFlag(args, &i, a, stderr, cmd)
			if !ok {
				return f, nil, false
			}
			f.selfRepairAttempts, f.set["self_repair_attempts"] = n, true
		case "--self-repair-escalate":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.selfRepairEscalate, f.set["self_repair_escalate"] = args[i], true
		case "--silent-on-success":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			v, err := strconv.ParseBool(args[i])
			if err != nil {
				fmt.Fprintf(stderr, "%s agent %s: invalid --silent-on-success %q (want true or false)\n", brand.CLI, cmd, args[i])
				return f, nil, false
			}
			f.silentOnSuccess, f.set["silent_on_success"] = v, true
		case "--disable-memory-writes":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			v, err := strconv.ParseBool(args[i])
			if err != nil {
				fmt.Fprintf(stderr, "%s agent %s: invalid --disable-memory-writes %q (want true or false)\n", brand.CLI, cmd, args[i])
				return f, nil, false
			}
			f.disableMemoryWrites, f.set["disable_memory_writes"] = v, true
		case "--notify-min-severity":
			if !need(i, a) {
				return f, nil, false
			}
			i++
			f.notifyMinSeverity, f.set["notify_min_severity"] = args[i], true
		case "--notify-cooldown-sec":
			n, ok := parseNonNegativeFlag(args, &i, a, stderr, cmd)
			if !ok {
				return f, nil, false
			}
			f.notifyCooldownSec, f.set["notify_cooldown_sec"] = n, true
		default:
			rest = append(rest, a)
		}
	}
	return f, rest, true
}

func cmdAgentList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	profiles, _ := res["profiles"].([]any)
	if len(profiles) == 0 {
		fmt.Fprintf(stdout, "no agents yet — create one with `%s agent add <slug> --soul \"...\"`\n", brand.CLI)
		return 0
	}
	for _, raw := range profiles {
		p, _ := raw.(map[string]any)
		if p == nil {
			continue
		}
		state := agentListStateLabel(p)
		model, _ := p["model"].(string)
		if model == "" {
			model = "(default)"
		}
		kind := str(p["kind"])
		if kind == "" {
			kind = "custom"
		}
		fmt.Fprintf(stdout, "%-20s %-9s %-8s model=%s", str(p["slug"]), state, kind, model)
		if tt, _ := p["task_type"].(string); tt != "" {
			fmt.Fprintf(stdout, " task=%s", tt)
		}
		if parent := str(p["parent_agent"]); parent != "" {
			fmt.Fprintf(stdout, " parent=%s", parent)
		} else if owner := str(p["owner_agent"]); owner != "" {
			fmt.Fprintf(stdout, " owner=%s", owner)
		}
		if mc, ok := p["max_cost_mc"].(float64); ok && mc > 0 {
			fmt.Fprintf(stdout, " max-cost=%s", fmtUSD(int64(mc)))
		}
		if suffix := agentListStatusSuffix(p); suffix != "" {
			fmt.Fprintf(stdout, " %s", suffix)
		}
		fmt.Fprintln(stdout)
	}
	fmt.Fprintf(stdout, "%v agent(s)\n", res["count"])
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
