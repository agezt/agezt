// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdAgent dispatches `agt agent <subcommand>` — the management surface for the
// durable agent roster (M783): named agent identities with their own soul
// (identity core), model, per-run cost ceiling, memory scope, and workdir.
// `agt run --agent <slug>` runs AS one. Every mutation is journaled (roster.*).
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
	fmt.Fprintf(w, "      [--instructions TEXT] [--tool-allow csv] [--tool-deny csv] [--trust-ceiling L0..L4] [--config KEY=VALUE,...]\n")
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
	c := dial(stderr)
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
		return encodeJSON(stdout, res)
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
	c := dial(stderr)
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
			return encodeJSON(stdout, p)
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

func cmdAgentAdd(args []string, stdout, stderr io.Writer) int {
	f, rest, ok := parseAgentFlags(args, stderr, "add")
	if !ok {
		return 2
	}
	if len(rest) != 1 {
		fmt.Fprintf(stderr, "usage: %s agent add <slug> [--soul TEXT] [--model M] ...\n", brand.CLI)
		return 2
	}
	profile := map[string]any{
		"slug": rest[0], "name": f.name, "soul": f.soul, "model": f.model,
		"task_type": f.task, "max_cost_mc": f.maxCostMc, "max_daily_mc": f.maxDailyMc,
		"memory_scope": f.memScope, "workdir": f.workdir, "description": f.desc,
	}
	if f.set["owner_agent"] {
		profile["owner_agent"] = f.ownerAgent
	}
	if f.set["parent_agent"] {
		profile["parent_agent"] = f.parentAgent
	}
	if f.set["direct_callable"] {
		profile["direct_callable"] = f.directCallable
	}
	if err := applyAgentAdvancedFlags(profile, f); err != nil {
		fmt.Fprintf(stderr, "%s agent add: %v\n", brand.CLI, err)
		return 2
	}
	applyAgentPolicyFlags(profile, f)
	if len(f.fallbacks) > 0 {
		fb := make([]any, len(f.fallbacks))
		for i, m := range f.fallbacks {
			fb[i] = m
		}
		profile["fallbacks"] = fb
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{"profile": profile})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent add: %v\n", brand.CLI, err)
		return 1
	}
	p, _ := res["profile"].(map[string]any)
	fmt.Fprintf(stdout, "agent %s created (run it: %s run --agent %s \"...\")\n", str(p["slug"]), brand.CLI, str(p["slug"]))
	return 0
}

func cmdAgentSet(args []string, stdout, stderr io.Writer) int {
	f, rest, ok := parseAgentFlags(args, stderr, "set")
	if !ok {
		return 2
	}
	if len(rest) != 1 || len(f.set) == 0 {
		fmt.Fprintf(stderr, "usage: %s agent set <slug|id> --soul TEXT|--model M|... (at least one flag)\n", brand.CLI)
		return 2
	}
	ref := rest[0]
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// agent_edit applies the mutable fields WHOLESALE, so a partial edit must
	// start from the current profile and overlay only the provided flags.
	cur, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent set: %v\n", brand.CLI, err)
		return 1
	}
	var base map[string]any
	if profiles, _ := cur["profiles"].([]any); profiles != nil {
		for _, raw := range profiles {
			p, _ := raw.(map[string]any)
			if p != nil && (str(p["slug"]) == ref || str(p["id"]) == ref) {
				base = p
				break
			}
		}
	}
	if base == nil {
		fmt.Fprintf(stderr, "%s agent set: unknown agent %q\n", brand.CLI, ref)
		return 1
	}
	overlay := func(key, flag, val string) {
		if f.set[flag] {
			base[key] = val
		}
	}
	overlay("name", "name", f.name)
	overlay("soul", "soul", f.soul)
	overlay("model", "model", f.model)
	overlay("task_type", "task", f.task)
	overlay("memory_scope", "memory_scope", f.memScope)
	overlay("workdir", "workdir", f.workdir)
	overlay("description", "desc", f.desc)
	overlay("owner_agent", "owner_agent", f.ownerAgent)
	overlay("parent_agent", "parent_agent", f.parentAgent)
	if f.set["max_cost"] {
		base["max_cost_mc"] = f.maxCostMc
	}
	if f.set["max_daily"] {
		base["max_daily_mc"] = f.maxDailyMc
	}
	if f.set["direct_callable"] {
		base["direct_callable"] = f.directCallable
	}
	if err := applyAgentAdvancedFlags(base, f); err != nil {
		fmt.Fprintf(stderr, "%s agent set: %v\n", brand.CLI, err)
		return 2
	}
	applyAgentPolicyFlags(base, f)
	if f.set["fallbacks"] {
		fb := make([]any, len(f.fallbacks))
		for i, m := range f.fallbacks {
			fb[i] = m
		}
		base["fallbacks"] = fb
	}
	res, err := c.Call(ctx, controlplane.CmdAgentEdit, map[string]any{"ref": ref, "profile": base})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent set: %v\n", brand.CLI, err)
		return 1
	}
	p, _ := res["profile"].(map[string]any)
	fmt.Fprintf(stdout, "agent %s updated\n", str(p["slug"]))
	return 0
}

func cmdAgentTask(args []string, stdout, stderr io.Writer) int {
	payload, label, ok := buildAgentTaskPayload(args, stderr)
	if !ok {
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentTaskUpdate, payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent task: %v\n", brand.CLI, err)
		return 1
	}
	task, _ := res["task"].(map[string]any)
	if label == "" {
		label = str(task["status"])
	}
	fmt.Fprintf(stdout, "agent %s task %s", str(payload["ref"]), label)
	if id := str(task["id"]); id != "" {
		fmt.Fprintf(stdout, " %s", id)
	}
	if title := str(task["title"]); title != "" {
		fmt.Fprintf(stdout, " (%s)", title)
	}
	fmt.Fprintln(stdout)
	return 0
}

func buildAgentTaskPayload(args []string, stderr io.Writer) (map[string]any, string, bool) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printAgentTaskUsage(stderr)
		return nil, "", false
	}
	cmd := strings.ToLower(strings.TrimSpace(args[0]))
	switch cmd {
	case "add":
		if len(args) < 3 {
			printAgentTaskUsage(stderr)
			return nil, "", false
		}
		payload := map[string]any{"op": "add", "ref": args[1], "title": args[2], "scope": "total", "status": "todo"}
		if !applyTaskFlags(payload, args[3:], stderr) {
			return nil, "", false
		}
		return payload, "added", true
	case "set", "update", "edit":
		if len(args) < 3 {
			printAgentTaskUsage(stderr)
			return nil, "", false
		}
		payload := map[string]any{"op": "update", "ref": args[1], "id": args[2]}
		if !applyTaskFlags(payload, args[3:], stderr) {
			return nil, "", false
		}
		if len(payload) <= 3 {
			fmt.Fprintf(stderr, "%s agent task set: at least one field flag is required\n", brand.CLI)
			return nil, "", false
		}
		return payload, "updated", true
	case "remove", "delete", "rm":
		if len(args) != 3 {
			printAgentTaskUsage(stderr)
			return nil, "", false
		}
		return map[string]any{"op": "remove", "ref": args[1], "id": args[2]}, "removed", true
	case "todo", "doing", "done", "blocked", "retired":
		if len(args) != 3 {
			printAgentTaskUsage(stderr)
			return nil, "", false
		}
		return map[string]any{"op": "update", "ref": args[1], "id": args[2], "status": cmd}, cmd, true
	default:
		fmt.Fprintf(stderr, "%s agent task: unknown task command %q\n", brand.CLI, args[0])
		printAgentTaskUsage(stderr)
		return nil, "", false
	}
}

func applyTaskFlags(payload map[string]any, args []string, stderr io.Writer) bool {
	need := func(i int, flag string) bool {
		if i+1 >= len(args) {
			fmt.Fprintf(stderr, "%s agent task: %s needs a value\n", brand.CLI, flag)
			return false
		}
		return true
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--title":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["title"] = args[i]
		case "--desc", "--description":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["description"] = args[i]
		case "--scope":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["scope"] = args[i]
		case "--status":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["status"] = args[i]
		case "--id":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["id"] = args[i]
		default:
			fmt.Fprintf(stderr, "%s agent task: unknown flag %s\n", brand.CLI, args[i])
			return false
		}
	}
	return true
}

func printAgentTaskUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: %s agent task add <slug|id> <title> [--id ID] [--scope cycle|total] [--status todo|doing|done|blocked|retired] [--desc TEXT]\n", brand.CLI)
	fmt.Fprintf(w, "       %s agent task set <slug|id> <task-id> [--title TEXT] [--scope cycle|total] [--status ...] [--desc TEXT]\n", brand.CLI)
	fmt.Fprintf(w, "       %s agent task <done|doing|todo|blocked|retired|remove> <slug|id> <task-id>\n", brand.CLI)
}

func cmdAgentWake(args []string, stdout, stderr io.Writer) int {
	payload, ok := buildAgentWakePayload(args, stderr)
	if !ok {
		return 2
	}
	return callAgentAsyncAction(controlplane.CmdAgentWake, "wake", payload, stdout, stderr)
}

func cmdAgentRepair(args []string, stdout, stderr io.Writer) int {
	payload, ok := buildAgentRepairPayload(args, stderr)
	if !ok {
		return 2
	}
	return callAgentAsyncAction(controlplane.CmdAgentRepair, "repair", payload, stdout, stderr)
}

func cmdAgentRepairStatus(args []string, stdout, stderr io.Writer) int {
	ref := ""
	limit := 20
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--limit":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s agent repair-status: --limit needs a value\n", brand.CLI)
				return 2
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "%s agent repair-status: invalid --limit %q\n", brand.CLI, args[i])
				return 2
			}
			limit = n
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s agent repair-status <slug|id> [--limit N]\n", brand.CLI)
			return 0
		default:
			if strings.HasPrefix(args[i], "--") || ref != "" {
				fmt.Fprintf(stderr, "usage: %s agent repair-status <slug|id> [--limit N]\n", brand.CLI)
				return 2
			}
			ref = args[i]
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s agent repair-status <slug|id> [--limit N]\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentRepairStatus, map[string]any{"ref": ref, "limit": limit})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent repair-status: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "agent %s repair: %v history, %v inflight", str(res["slug"]), res["count"], res["inflight_count"])
	if next := intNumber(res["next_eligible_ms"]); next > 0 {
		fmt.Fprintf(stdout, " next_eligible_ms=%d", next)
	}
	fmt.Fprintln(stdout)
	if latest, _ := res["latest"].(map[string]any); latest != nil {
		fmt.Fprintf(stdout, "latest: %s %s\n", str(latest["phase"]), str(latest["reason"]))
	}
	return 0
}

func callAgentAsyncAction(cmd string, label string, payload map[string]any, stdout, stderr io.Writer) int {
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent %s: %v\n", brand.CLI, label, err)
		return 1
	}
	fmt.Fprintf(stdout, "agent %s %s accepted", str(res["agent"]), label)
	if corr := str(res["correlation_id"]); corr != "" {
		fmt.Fprintf(stdout, " corr=%s", corr)
	}
	fmt.Fprintln(stdout)
	return 0
}

func buildAgentWakePayload(args []string, stderr io.Writer) (map[string]any, bool) {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: %s agent wake <slug|id> [intent text] [--reason TEXT] [--incident ID] [--root ID] [--parent ID]\n", brand.CLI)
		return nil, false
	}
	payload := map[string]any{"ref": args[0]}
	var intent []string
	if !applyAgentActionFlags(payload, args[1:], &intent, stderr, "wake") {
		return nil, false
	}
	if len(intent) > 0 {
		payload["intent"] = strings.Join(intent, " ")
	}
	return payload, true
}

func buildAgentRepairPayload(args []string, stderr io.Writer) (map[string]any, bool) {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: %s agent repair <slug|id> [--reason TEXT] [--incident ID] [--root ID] [--parent ID]\n", brand.CLI)
		return nil, false
	}
	payload := map[string]any{"ref": args[0]}
	var extras []string
	if !applyAgentActionFlags(payload, args[1:], &extras, stderr, "repair") {
		return nil, false
	}
	if len(extras) > 0 && str(payload["reason"]) == "" {
		payload["reason"] = strings.Join(extras, " ")
	}
	return payload, true
}

func applyAgentActionFlags(payload map[string]any, args []string, positional *[]string, stderr io.Writer, cmd string) bool {
	need := func(i int, flag string) bool {
		if i+1 >= len(args) {
			fmt.Fprintf(stderr, "%s agent %s: %s needs a value\n", brand.CLI, cmd, flag)
			return false
		}
		return true
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--reason":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["reason"] = args[i]
		case "--incident", "--incident-id":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["incident_id"] = args[i]
		case "--root", "--root-incident", "--root-incident-id":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["root_incident_id"] = args[i]
		case "--parent", "--parent-incident", "--parent-incident-id":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["parent_incident_id"] = args[i]
		default:
			if strings.HasPrefix(args[i], "--") {
				fmt.Fprintf(stderr, "%s agent %s: unknown flag %s\n", brand.CLI, cmd, args[i])
				return false
			}
			*positional = append(*positional, args[i])
		}
	}
	return true
}

func cmdAgentSetEnabled(args []string, stdout, stderr io.Writer, enabled bool) int {
	verb := "pause"
	if enabled {
		verb = "resume"
	}
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s agent %s <slug|id>\n", brand.CLI, verb)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentSetEnabled, map[string]any{"ref": args[0], "enabled": enabled})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	p, _ := res["profile"].(map[string]any)
	fmt.Fprintf(stdout, "agent %s %sd\n", str(p["slug"]), verb)
	return 0
}

// cmdAgentAuthority implements `agt agent authority <slug|id> [--json]`.
//
// It is the effective-policy proof surface (see docs/COMPARISON.md): it merges
// the agent's own authority fields (tool allow/deny, trust ceiling, memory
// scope, workdir, config) with the live Edict policy snapshot (capability
// levels, hard-deny rules, approval mode) into a single view of what this
// agent can actually do at runtime. Read-only; no side effects.
//
// This is deliberately client-side: it fetches CmdAgentList + CmdEdictShow and
// joins them, so no new control-plane command is needed. If the daemon's
// authority model grows (config-center access per agent, schedule/channel wake
// permissions), this command should extend to surface those too.
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

	c := dial(stderr)
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
		return encodeJSON(stdout, authority)
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
	Name      string   `json:"name"`
	Substring string   `json:"substring"`
	Scope     string   `json:"scope"`
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
	c := dial(stderr)
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

func cmdAgentTombstone(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var ref string
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else if !strings.HasPrefix(a, "-") && ref == "" {
			ref = a
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s agent tombstone <slug|id> [--json]\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentTombstone, map[string]any{"ref": ref})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent tombstone: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	t, _ := res["tombstone"].(map[string]any)
	if t == nil {
		fmt.Fprintln(stderr, "tombstone: empty response")
		return 1
	}
	fmt.Fprintf(stdout, "⚰ tombstone — %s", str(t["slug"]))
	if name := str(t["name"]); name != "" && name != str(t["slug"]) {
		fmt.Fprintf(stdout, " (%s)", name)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "kind:         %s\n", str(t["kind"]))
	if mgr := str(t["manager"]); mgr != "" {
		fmt.Fprintf(stdout, "manager:      %s\n", mgr)
	}
	if retired, _ := t["retired"].(bool); retired {
		line := "status:       retired (graveyard)"
		if ms := intNumber(t["retired_ms"]); ms > 0 {
			line += " since " + time.UnixMilli(int64(ms)).Format(time.RFC3339)
		}
		fmt.Fprintln(stdout, line)
		if reason := str(t["retired_reason"]); reason != "" {
			fmt.Fprintf(stdout, "reason:       %s\n", reason)
		}
	} else {
		fmt.Fprintln(stdout, "status:       active (snapshot)")
	}
	mode := str(t["lifecycle_mode"])
	if mode == "" {
		mode = "persistent"
	}
	fmt.Fprintf(stdout, "lifecycle:    %s", mode)
	if mx := intNumber(t["max_cycles"]); mx > 0 {
		fmt.Fprintf(stdout, " %d/%d cycles", intNumber(t["completed_cycles"]), mx)
	} else if done := intNumber(t["completed_cycles"]); done > 0 {
		fmt.Fprintf(stdout, " %d cycles", done)
	}
	fmt.Fprintln(stdout)
	if fp, ok := t["footprint"].(map[string]any); ok {
		var parts []string
		for _, f := range []struct{ key, label string }{
			{"standing_orders", "standing"}, {"schedules", "schedules"},
			{"memories", "memories"}, {"skills", "skills"}, {"configs", "configs"},
			{"workspaces", "workspaces"}, {"workflow_refs", "workflow refs"},
			{"mailbox_messages", "mailbox msgs"}, {"subagents", "sub-agents"},
		} {
			if n := intNumber(fp[f.key]); n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n, f.label))
			}
		}
		if len(parts) > 0 {
			fmt.Fprintf(stdout, "footprint:    %s\n", strings.Join(parts, ", "))
		} else {
			fmt.Fprintln(stdout, "footprint:    none")
		}
	}
	return 0
}

func cmdAgentGraveyard(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	olderThan := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--older-than":
			if i+1 < len(args) {
				olderThan = args[i+1]
				i++
			}
		default:
			if strings.HasPrefix(args[i], "--older-than=") {
				olderThan = strings.TrimPrefix(args[i], "--older-than=")
			}
		}
	}
	reqArgs := map[string]any{}
	if olderThan != "" {
		if d, err := strconv.ParseFloat(olderThan, 64); err == nil {
			reqArgs["older_than_days"] = d
		} else {
			fmt.Fprintf(stderr, "%s agent graveyard: --older-than expects a number of days\n", brand.CLI)
			return 2
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentGraveyard, reqArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent graveyard: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	rows, _ := res["graveyard"].([]any)
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "graveyard is empty (no retired agents match)")
		return 0
	}
	for _, raw := range rows {
		r, _ := raw.(map[string]any)
		if r == nil {
			continue
		}
		fmt.Fprintf(stdout, "%-20s retired %dd ago", str(r["slug"]), intNumber(r["age_days"]))
		if kind := str(r["kind"]); kind != "" {
			fmt.Fprintf(stdout, " [%s]", kind)
		}
		if reason := str(r["retired_reason"]); reason != "" {
			fmt.Fprintf(stdout, " — %s", reason)
		}
		fmt.Fprintln(stdout)
	}
	fmt.Fprintf(stdout, "%v retired agent(s)\n", res["count"])
	return 0
}

// cmdAgentRetire moves a dead agent to the graveyard (M846): it shows what
// depends on the agent first, then retires. The agent is paused and any
// delegation to it is refused until revived. The roster keeps it (recoverable),
// distinct from `remove` which deletes.
func cmdAgentRetire(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintf(stderr, "usage: %s agent retire <slug|id> [reason]\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := map[string]any{"ref": args[0]}
	if len(args) > 1 {
		req["reason"] = strings.Join(args[1:], " ")
	}
	res, err := c.Call(ctx, controlplane.CmdAgentRetire, req)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent retire: %v\n", brand.CLI, err)
		return 1
	}
	p, _ := res["profile"].(map[string]any)
	fmt.Fprintf(stdout, "agent %s retired to the graveyard (revive it: %s agent revive %s)\n", str(p["slug"]), brand.CLI, str(p["slug"]))
	if summary, _ := res["impact_summary"].(map[string]any); len(summary) > 0 {
		printAgentImpactSummary(stdout, summary)
	} else if impact, _ := res["impact"].([]any); len(impact) > 0 {
		printImpactItems(stdout, "standing orders", impact)
	}
	return 0
}

// cmdAgentRevive brings a retired agent back from the graveyard (M846). It is
// restored paused so the operator decides when to resume it.
func cmdAgentRevive(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s agent revive <slug|id>\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentRevive, map[string]any{"ref": args[0]})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent revive: %v\n", brand.CLI, err)
		return 1
	}
	p, _ := res["profile"].(map[string]any)
	fmt.Fprintf(stdout, "agent %s revived (paused — resume it: %s agent resume %s)\n", str(p["slug"]), brand.CLI, str(p["slug"]))
	return 0
}

func cmdAgentRemove(args []string, stdout, stderr io.Writer) int {
	ref, cascade, help, ok := buildAgentRemovePayload(args, stderr)
	if help {
		printAgentRemoveUsage(stdout)
		return 0
	}
	if !ok {
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	payload := map[string]any{"ref": ref}
	if len(cascade) > 0 {
		payload["cascade"] = cascade
	}
	if impact, err := c.Call(ctx, controlplane.CmdAgentImpact, map[string]any{"ref": ref}); err == nil {
		printAgentImpactSummary(stdout, impact)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentRemove, payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent remove: %v\n", brand.CLI, err)
		return 1
	}
	if removed, _ := res["removed"].(bool); !removed {
		fmt.Fprintf(stderr, "%s agent remove: unknown agent %q\n", brand.CLI, ref)
		return 1
	}
	fmt.Fprintf(stdout, "agent %s removed", ref)
	if summary := agentRemoveResultSummary(res); summary != "" {
		fmt.Fprintf(stdout, " (%s)", summary)
	}
	fmt.Fprintln(stdout)
	return 0
}

func agentRemoveResultSummary(res map[string]any) string {
	parts := []string{}
	add := func(key, label string) {
		n := intNumber(res[key])
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, label))
		}
	}
	add("standing_removed", "standing")
	add("schedules_removed", "schedule")
	add("memories_forgotten", "private memory")
	add("authored_memories_forgotten", "authored shared memory")
	add("skills_archived", "skill archived")
	add("configs_deleted", "config deleted")
	add("workspaces_deleted", "workspace deleted")
	add("subagents_retired", "subagent retired")
	add("mailbox_messages_retained", "mailbox/audit retained")
	return strings.Join(parts, ", ")
}

func buildAgentRemovePayload(args []string, stderr io.Writer) (string, map[string]any, bool, bool) {
	ref := ""
	cascade := map[string]any{}
	for _, a := range args {
		switch a {
		case "--with-all":
			cascade["standing"] = true
			cascade["schedules"] = true
			cascade["memory"] = true
			cascade["authored_memory"] = true
			cascade["skills"] = true
			cascade["config"] = true
			cascade["workspace"] = true
			cascade["subagents"] = true
		case "--with-standing":
			cascade["standing"] = true
		case "--with-schedules":
			cascade["schedules"] = true
		case "--with-memory":
			cascade["memory"] = true
		case "--with-authored-memory", "--with-authored-shared-memory":
			cascade["authored_memory"] = true
		case "--with-skills":
			cascade["skills"] = true
		case "--with-config":
			cascade["config"] = true
		case "--with-workspace", "--with-workdir":
			cascade["workspace"] = true
		case "--with-subagents":
			cascade["subagents"] = true
		case "-h", "--help":
			return "", nil, true, false
		default:
			if strings.HasPrefix(a, "--") {
				fmt.Fprintf(stderr, "%s agent remove: unknown flag %s\n", brand.CLI, a)
				return "", nil, false, false
			}
			if ref != "" {
				printAgentRemoveUsage(stderr)
				return "", nil, false, false
			}
			ref = a
		}
	}
	if ref == "" {
		printAgentRemoveUsage(stderr)
		return "", nil, false, false
	}
	return ref, cascade, false, true
}

func printAgentRemoveUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: %s agent remove <slug|id> [--with-all|--with-schedules|--with-memory|--with-authored-memory|--with-skills|--with-config|--with-workspace|--with-standing|--with-subagents]\n", brand.CLI)
	fmt.Fprintf(w, "  --with-memory cleans private memory; --with-authored-memory cleans shared memory records written by the agent\n")
}

func printAgentImpactSummary(w io.Writer, summary map[string]any) bool {
	sections := []struct {
		key   string
		label string
	}{
		{"standing_orders", "standing orders"},
		{"schedules", "schedules"},
		{"memories", "private memory"},
		{"authored_shared_memories", "authored shared memory"},
		{"skills", "private skills"},
		{"configs", "agent config"},
		{"workspaces", "workspace"},
		{"subagents", "dependent sub-agents"},
		{"subagent_standing_orders", "sub-agent standing orders"},
		{"subagent_schedules", "sub-agent schedules"},
		{"subagent_memories", "sub-agent private memory"},
		{"subagent_authored_shared_memories", "sub-agent authored shared memory"},
		{"subagent_skills", "sub-agent skills"},
		{"subagent_configs", "sub-agent config"},
		{"subagent_workspaces", "sub-agent workspace"},
	}
	printed := false
	for _, sec := range sections {
		items := stringsAny(summary[sec.key])
		if len(items) == 0 {
			continue
		}
		if !printed {
			fmt.Fprintln(w, "impact:")
			printed = true
		}
		printImpactItems(w, sec.label, items)
	}
	return printed
}

func printImpactItems(w io.Writer, label string, items []any) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(w, "  %s (%d):\n", label, len(items))
	for _, item := range items {
		fmt.Fprintf(w, "    - %s\n", str(item))
	}
}

func stringsAny(v any) []any {
	switch xs := v.(type) {
	case []any:
		return xs
	case []string:
		out := make([]any, 0, len(xs))
		for _, x := range xs {
			out = append(out, x)
		}
		return out
	default:
		return nil
	}
}

// str renders any JSON value as its string form ("" for nil/non-strings).
func str(v any) string {
	s, _ := v.(string)
	return s
}

func intNumber(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func parseNonNegativeFlag(args []string, i *int, flag string, stderr io.Writer, cmd string) (int, bool) {
	if *i+1 >= len(args) {
		fmt.Fprintf(stderr, "%s agent %s: %s needs a value\n", brand.CLI, cmd, flag)
		return 0, false
	}
	*i = *i + 1
	n, err := strconv.Atoi(args[*i])
	if err != nil || n < 0 {
		fmt.Fprintf(stderr, "%s agent %s: invalid %s %q (want a non-negative integer)\n", brand.CLI, cmd, flag, args[*i])
		return 0, false
	}
	return n, true
}

func applyAgentPolicyFlags(profile map[string]any, f agentFlags) {
	if f.set["retry_attempts"] || f.set["retry_backoff"] || f.set["retry_base_sec"] || f.set["retry_max_sec"] || f.set["retry_on"] {
		pol := objectMap(profile["retry_policy"])
		if f.set["retry_attempts"] {
			pol["max_attempts"] = f.retryAttempts
		}
		if f.set["retry_backoff"] {
			pol["backoff"] = f.retryBackoff
		}
		if f.set["retry_base_sec"] {
			pol["base_delay_sec"] = f.retryBaseSec
		}
		if f.set["retry_max_sec"] {
			pol["max_delay_sec"] = f.retryMaxSec
		}
		if f.set["retry_on"] {
			pol["retry_on"] = stringList(f.retryOn)
		}
		profile["retry_policy"] = pol
	}
	if f.set["doctor_agent"] || f.set["health_stale_sec"] || f.set["health_window"] || f.set["health_threshold"] {
		pol := objectMap(profile["health_policy"])
		if f.set["doctor_agent"] {
			pol["doctor_agent"] = f.doctorAgent
		}
		if f.set["health_stale_sec"] {
			pol["stale_after_sec"] = f.healthStaleSec
		}
		if f.set["health_window"] {
			pol["failure_window"] = f.healthWindow
		}
		if f.set["health_threshold"] {
			pol["failure_threshold"] = f.healthThreshold
		}
		profile["health_policy"] = pol
	}
	if f.set["self_repair"] || f.set["self_repair_attempts"] || f.set["self_repair_escalate"] {
		pol := objectMap(profile["self_repair"])
		if f.set["self_repair"] {
			pol["enabled"] = f.selfRepairEnabled
		}
		if f.set["self_repair_attempts"] {
			pol["max_attempts"] = f.selfRepairAttempts
		}
		if f.set["self_repair_escalate"] {
			pol["escalate_to"] = f.selfRepairEscalate
		}
		profile["self_repair"] = pol
	}
	if f.set["silent_on_success"] || f.set["disable_memory_writes"] || f.set["notify_min_severity"] || f.set["notify_cooldown_sec"] {
		pol := objectMap(profile["noise_policy"])
		if f.set["silent_on_success"] {
			pol["silent_on_success"] = f.silentOnSuccess
		}
		if f.set["disable_memory_writes"] {
			pol["disable_memory_writes"] = f.disableMemoryWrites
		}
		if f.set["notify_min_severity"] {
			pol["min_notify_severity"] = strings.TrimSpace(f.notifyMinSeverity)
		}
		if f.set["notify_cooldown_sec"] {
			pol["min_notify_interval_sec"] = f.notifyCooldownSec
		}
		profile["noise_policy"] = pol
	}
}

func applyAgentAdvancedFlags(profile map[string]any, f agentFlags) error {
	if f.set["instructions"] {
		profile["instructions"] = stringList(f.instructions)
	}
	if f.set["tool_allow"] {
		profile["tool_allow"] = stringList(f.toolAllow)
	}
	if f.set["tool_deny"] {
		profile["tool_deny"] = stringList(f.toolDeny)
	}
	if f.set["trust_ceiling"] {
		profile["trust_ceiling"] = strings.TrimSpace(f.trustCeiling)
	}
	if f.set["config_overrides"] {
		cfg, err := parseConfigOverrides(f.configOverrides)
		if err != nil {
			return err
		}
		profile["config_overrides"] = cfg
	}
	if f.set["lifecycle"] || f.set["max_cycles"] {
		life := objectMap(profile["lifecycle"])
		if f.set["lifecycle"] {
			mode := strings.TrimSpace(f.lifecycleMode)
			life["mode"] = mode
			life["retire_on_complete"] = mode == "retire_on_complete"
		}
		if f.set["max_cycles"] {
			life["max_cycles"] = f.lifecycleMaxCycles
		}
		profile["lifecycle"] = life
	}
	if f.set["cycle_tasks"] || f.set["total_tasks"] {
		var tasks []any
		if existing, ok := profile["tasklist"].([]any); ok {
			for _, rawTask := range existing {
				scope := taskScope(rawTask)
				if scope == "cycle" && f.set["cycle_tasks"] {
					continue
				}
				if scope == "total" && f.set["total_tasks"] {
					continue
				}
				tasks = append(tasks, rawTask)
			}
		}
		if f.set["cycle_tasks"] {
			for _, title := range f.cycleTasks {
				tasks = append(tasks, map[string]any{"title": title, "scope": "cycle", "status": "todo"})
			}
		}
		if f.set["total_tasks"] {
			for _, title := range f.totalTasks {
				tasks = append(tasks, map[string]any{"title": title, "scope": "total", "status": "todo"})
			}
		}
		profile["tasklist"] = tasks
	}
	return nil
}

func taskScope(raw any) string {
	task, _ := raw.(map[string]any)
	if strings.TrimSpace(str(task["scope"])) == "cycle" {
		return "cycle"
	}
	return "total"
}

func parseConfigOverrides(s string) (map[string]any, error) {
	out := map[string]any{}
	for _, part := range splitList(s) {
		eq := strings.Index(part, "=")
		if eq <= 0 {
			return nil, fmt.Errorf("invalid --config entry %q (want KEY=VALUE)", part)
		}
		key := strings.TrimSpace(part[:eq])
		val := strings.TrimSpace(part[eq+1:])
		if key == "" {
			return nil, fmt.Errorf("invalid --config entry %q (empty key)", part)
		}
		out[key] = val
	}
	return out, nil
}

func objectMap(v any) map[string]any {
	out := map[string]any{}
	if m, ok := v.(map[string]any); ok {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func stringList(csv string) []any {
	var out []any
	for _, s := range splitList(csv) {
		out = append(out, s)
	}
	return out
}

func splitList(s string) []string {
	var out []string
	for _, item := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	}) {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func printAgentPolicy(w io.Writer, label string, raw any) {
	pol, ok := raw.(map[string]any)
	if !ok || len(pol) == 0 {
		return
	}
	var parts []string
	for _, key := range []string{
		"max_attempts", "backoff", "base_delay_sec", "max_delay_sec", "retry_on",
		"doctor_agent", "stale_after_sec", "failure_window", "failure_threshold",
		"enabled", "escalate_to",
	} {
		v, ok := pol[key]
		if !ok || emptyJSONValue(v) {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%v", key, v))
	}
	if len(parts) > 0 {
		fmt.Fprintf(w, "%-13s %s\n", label+":", strings.Join(parts, " "))
	}
}

func emptyJSONValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case float64:
		return x == 0
	case int:
		return x == 0
	case bool:
		return false
	case []any:
		return len(x) == 0
	default:
		return false
	}
}

func joinAnyStrings(values []any, sep string) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		if s := str(v); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, sep)
}

func padValue(s string) string {
	if s == "" {
		return ""
	}
	return " " + s
}

func printAgentLifecycle(w io.Writer, raw any) {
	life, ok := raw.(map[string]any)
	if !ok || len(life) == 0 {
		return
	}
	var parts []string
	for _, key := range []string{"mode", "retire_on_complete", "max_cycles", "completed_cycles"} {
		if v, ok := life[key]; ok && !emptyJSONValue(v) {
			parts = append(parts, fmt.Sprintf("%s=%v", key, v))
		}
	}
	if len(parts) > 0 {
		fmt.Fprintf(w, "lifecycle:    %s\n", strings.Join(parts, " "))
	}
}

func printAgentTaskSummary(w io.Writer, raw any) {
	tasks, ok := raw.([]any)
	if !ok || len(tasks) == 0 {
		return
	}
	cycle, total := 0, 0
	for _, rawTask := range tasks {
		task, _ := rawTask.(map[string]any)
		if str(task["scope"]) == "cycle" {
			cycle++
		} else {
			total++
		}
	}
	fmt.Fprintf(w, "tasklist:     %d cycle, %d total\n", cycle, total)
}
