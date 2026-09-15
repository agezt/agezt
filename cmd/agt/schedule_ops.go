// SPDX-License-Identifier: MIT
//
// cmd/agt schedule sub-command top-level handlers (cmdScheduleEnable,
// cmdScheduleList). Extracted from schedule_ops.go during Day 211
// god-file refactor (#37, #58). Public API unchanged.
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

func cmdScheduleEnable(args []string, stdout, stderr io.Writer, enabled bool) int {
	verb := "resume"
	if !enabled {
		verb = "pause"
	}
	asJSON := false
	var id string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s schedule %s <id> [--json]\n", brand.CLI, verb)
			return 0
		default:
			if id == "" {
				id = a
			}
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s schedule %s: an id is required\n", brand.CLI, verb)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdScheduleEnable, map[string]any{"id": id, "enabled": enabled})
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	if updated, _ := res["updated"].(bool); !updated {
		fmt.Fprintf(stderr, "%s schedule %s: not found (%s)\n", brand.CLI, verb, id)
		return 3
	}
	fmt.Fprintf(stdout, "%s %sd\n", id, verb)
	return 0
}
func cmdScheduleList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s schedule list [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s schedule list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	list, _ := res["schedules"].([]any)
	if len(list) == 0 {
		fmt.Fprintf(stdout, "no schedules. Add one with `%s schedule add \"<task>\" --every 1h`.\n", brand.CLI)
		return 0
	}
	for _, item := range list {
		m, _ := item.(map[string]any)
		id, _ := m["id"].(string)
		intent, _ := m["intent"].(string)
		cadence, _ := m["cadence"].(string)
		source, _ := m["source"].(string)
		agent, _ := m["agent"].(string)
		target, _ := m["target"].(string)
		workflowRef, _ := m["workflow"].(string)
		systemTask, _ := m["system_task"].(string)
		toolRef, _ := m["tool"].(string)
		enabled, _ := m["enabled"].(bool)
		next, _ := m["next_run_unix"].(float64)
		state := "enabled"
		if !enabled {
			state = "paused"
		}
		nextStr := "—"
		if next > 0 {
			nextStr = time.Unix(int64(next), 0).Format("2006-01-02 15:04")
		}
		action := scheduleActionText(map[string]string{
			"id":          id,
			"intent":      intent,
			"target":      target,
			"agent":       agent,
			"workflow":    workflowRef,
			"system_task": systemTask,
			"tool":        toolRef,
		})
		if contract, _ := m["execution_contract"].(string); contract != "" {
			action = contract
		}
		fmt.Fprintf(stdout, "  %-22s %-16s [%s,%s] next %s  %s",
			id, cadence, source, state, nextStr, action)
		executor, _ := m["executor"].(string)
		usesLLM, hasUsesLLM := m["uses_llm"].(bool)
		if exec := scheduleExecutorText(executor, usesLLM, hasUsesLLM); exec != "" {
			fmt.Fprintf(stdout, "  executor:%s", exec)
		}
		if targetStatus := scheduleTargetStatusText(m); targetStatus != "" {
			fmt.Fprintf(stdout, "  target:%s", targetStatus)
		}
		if intent != "" && action != intent && !strings.Contains(action, intent) {
			fmt.Fprintf(stdout, "  label:%q", intent)
		}
		if agent != "" {
			fmt.Fprintf(stdout, "  agent:%s", agent)
		}
		if workflowRef != "" {
			fmt.Fprintf(stdout, "  workflow:%s", workflowRef)
		}
		if systemTask != "" {
			fmt.Fprintf(stdout, "  system_task:%s", systemTask)
		}
		if toolRef != "" {
			fmt.Fprintf(stdout, "  tool:%s", toolRef)
		}
		// Last-firing outcome (M56) — how the schedule last went, when known.
		lastStatus, _ := m["last_status"].(string)
		if lastStatus != "" {
			lastReason, _ := m["last_reason"].(string)
			if lastStatus == "failed" && lastReason != "" {
				lastStatus = "failed (" + lastReason + ")"
			}
			lastWhen := ""
			if lf, ok := m["last_fired_unix_ms"].(float64); ok && lf > 0 {
				lastWhen = " " + time.UnixMilli(int64(lf)).Format("01-02 15:04")
			}
			fmt.Fprintf(stdout, "  last: %s%s", lastStatus, lastWhen)
		}
		fmt.Fprintln(stdout)
	}
	return 0
}
