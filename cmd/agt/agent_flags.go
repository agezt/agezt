// SPDX-License-Identifier: MIT
//
// cmd/agt agent sub-command flag parser (parseAgentFlags).
// Extracted from agent.go during Day 211 god-file refactor (#60).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

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
