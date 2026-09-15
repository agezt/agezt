// SPDX-License-Identifier: MIT
//
// cmd/agt schedule edit sub-command. Split from schedule.go during
// Day 211 god-file refactor (#37). Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

func cmdScheduleEdit(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	once := false
	var id, intent, model, agentRef, workflowRef, systemTask, toolRef, payloadSpec, every, continuous, at, in, between, days, tz string
	var setIntent, setModel, setAgent, setWorkflow, setSystemTask, setTool bool
	for i := 0; i < len(args); i++ {
		a := args[i]
		needVal := func(flag string) (string, bool) {
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule edit: %s needs a value\n", brand.CLI, flag)
				return "", false
			}
			i++
			return args[i], true
		}
		switch a {
		case "--json":
			asJSON = true
		case "--once":
			once = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s schedule edit <id> [--intent <s>] [--agent <slug>] [--workflow <ref> [--payload JSON] | --system-task %s | --tool <name> [--payload JSON]] [--model <id>] [--every <dur> | --continuous <dur> | --at <HH:MM> [--days <spec>] | --in <dur> | --once --at <HH:MM>] [--json]\n", brand.CLI, scheduleSystemTaskUsage())
			return 0
		case "--intent":
			v, ok := needVal("--intent")
			if !ok {
				return 2
			}
			intent, setIntent = v, true
		case "--model":
			v, ok := needVal("--model")
			if !ok {
				return 2
			}
			model, setModel = v, true
		case "--agent":
			v, ok := needVal("--agent")
			if !ok {
				return 2
			}
			agentRef, setAgent = v, true
		case "--workflow":
			v, ok := needVal("--workflow")
			if !ok {
				return 2
			}
			workflowRef, setWorkflow = v, true
		case "--system-task":
			v, ok := needVal("--system-task")
			if !ok {
				return 2
			}
			systemTask, setSystemTask = v, true
		case "--tool":
			v, ok := needVal("--tool")
			if !ok {
				return 2
			}
			toolRef, setTool = v, true
		case "--payload":
			v, ok := needVal("--payload")
			if !ok {
				return 2
			}
			payloadSpec = v
		case "--every":
			v, ok := needVal("--every")
			if !ok {
				return 2
			}
			every = v
		case "--continuous":
			v, ok := needVal("--continuous")
			if !ok {
				return 2
			}
			continuous = v
		case "--at":
			v, ok := needVal("--at")
			if !ok {
				return 2
			}
			at = v
		case "--in":
			v, ok := needVal("--in")
			if !ok {
				return 2
			}
			in = v
		case "--between":
			v, ok := needVal("--between")
			if !ok {
				return 2
			}
			between = v
		case "--tz":
			v, ok := needVal("--tz")
			if !ok {
				return 2
			}
			tz = v
		case "--days":
			v, ok := needVal("--days")
			if !ok {
				return 2
			}
			days = v
		default:
			if id == "" && !strings.HasPrefix(a, "-") {
				id = a
			} else {
				fmt.Fprintf(stderr, "%s schedule edit: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s schedule edit: an id is required\n", brand.CLI)
		return 2
	}
	sources := 0
	for _, s := range []string{every, continuous, at, in} {
		if s != "" {
			sources++
		}
	}
	if sources > 1 {
		fmt.Fprintf(stderr, "%s schedule edit: pass at most one of --every, --continuous, --at, or --in\n", brand.CLI)
		return 2
	}
	if continuous != "" && (between != "" || days != "" || tz != "" || once) {
		fmt.Fprintf(stderr, "%s schedule edit: --continuous cannot combine with --between, --days, --tz, or --once\n", brand.CLI)
		return 2
	}
	if days != "" && at == "" && between == "" {
		fmt.Fprintf(stderr, "%s schedule edit: --days applies to --at (daily) or --every+--between (windowed) schedules\n", brand.CLI)
		return 2
	}
	if between != "" && every == "" {
		fmt.Fprintf(stderr, "%s schedule edit: --between requires --every <dur>\n", brand.CLI)
		return 2
	}
	if tz != "" && !((at != "" && !once) || between != "") {
		fmt.Fprintf(stderr, "%s schedule edit: --tz applies to --at (daily) or --every+--between (windowed) schedules\n", brand.CLI)
		return 2
	}
	if once && at == "" {
		fmt.Fprintf(stderr, "%s schedule edit: --once requires --at <HH:MM>\n", brand.CLI)
		return 2
	}
	if once && days != "" {
		fmt.Fprintf(stderr, "%s schedule edit: --days cannot combine with --once\n", brand.CLI)
		return 2
	}
	targets := 0
	for _, set := range []bool{setWorkflow, setSystemTask, setTool} {
		if set {
			targets++
		}
	}
	if targets > 1 {
		fmt.Fprintf(stderr, "%s schedule edit: choose only one of --workflow, --system-task, or --tool\n", brand.CLI)
		return 2
	}
	if setAgent && setSystemTask && strings.TrimSpace(agentRef) != "" {
		fmt.Fprintf(stderr, "%s schedule edit: --agent can run workflow/tool schedules, not system tasks\n", brand.CLI)
		return 2
	}
	if payloadSpec != "" && !setWorkflow && !setTool {
		fmt.Fprintf(stderr, "%s schedule edit: --payload applies only to --workflow or --tool\n", brand.CLI)
		return 2
	}
	if !setIntent && !setModel && !setAgent && targets == 0 && sources == 0 {
		fmt.Fprintf(stderr, "%s schedule edit: nothing to change (pass --intent <task/label>, a target flag, --model, or a cadence flag)\n", brand.CLI)
		return 2
	}

	callArgs := map[string]any{"id": id}
	if setIntent {
		if strings.TrimSpace(intent) == "" {
			fmt.Fprintf(stderr, "%s schedule edit: --intent task/label cannot be empty\n", brand.CLI)
			return 2
		}
		callArgs["intent"] = intent
	}
	if setModel {
		callArgs["model"] = model
	}
	if setAgent {
		callArgs["agent"] = strings.TrimSpace(agentRef)
		if !setWorkflow && !setTool {
			callArgs["target"] = ""
		}
	}
	if setWorkflow {
		callArgs["target"] = cadence.TargetWorkflow
		callArgs["workflow"] = strings.TrimSpace(workflowRef)
		if payloadSpec != "" {
			var payload any
			if err := json.Unmarshal([]byte(payloadSpec), &payload); err != nil {
				fmt.Fprintf(stderr, "%s schedule edit: bad --payload JSON: %v\n", brand.CLI, err)
				return 2
			}
			callArgs["payload"] = payload
		}
	}
	if setSystemTask {
		callArgs["target"] = cadence.TargetSystemTask
		callArgs["system_task"] = strings.TrimSpace(systemTask)
	}
	if setTool {
		callArgs["target"] = cadence.TargetTool
		callArgs["tool"] = strings.TrimSpace(toolRef)
		if payloadSpec != "" {
			var payload any
			if err := json.Unmarshal([]byte(payloadSpec), &payload); err != nil {
				fmt.Fprintf(stderr, "%s schedule edit: bad --payload JSON: %v\n", brand.CLI, err)
				return 2
			}
			callArgs["payload"] = payload
		}
	}
	switch {
	case in != "":
		d, err := time.ParseDuration(in)
		if err != nil || d < time.Second {
			fmt.Fprintf(stderr, "%s schedule edit: bad --in duration %q\n", brand.CLI, in)
			return 2
		}
		callArgs["once_at_unix"] = time.Now().Add(d).Unix()
	case continuous != "":
		d, err := time.ParseDuration(continuous)
		if err != nil || d < time.Second {
			fmt.Fprintf(stderr, "%s schedule edit: bad --continuous duration %q\n", brand.CLI, continuous)
			return 2
		}
		callArgs["cooldown_sec"] = int64(d / time.Second)
	case at != "" && once:
		mins, err := parseHHMM(at)
		if err != nil {
			fmt.Fprintf(stderr, "%s schedule edit: bad --at %q: %v\n", brand.CLI, at, err)
			return 2
		}
		callArgs["once_at_unix"] = nextWallclock(time.Now(), mins).Unix()
	case at != "":
		mins, err := parseHHMM(at)
		if err != nil {
			fmt.Fprintf(stderr, "%s schedule edit: bad --at %q: %v\n", brand.CLI, at, err)
			return 2
		}
		callArgs["at_minutes"] = mins
		if days != "" {
			mask, err := cadence.ParseDays(days)
			if err != nil {
				fmt.Fprintf(stderr, "%s schedule edit: bad --days %q: %v\n", brand.CLI, days, err)
				return 2
			}
			callArgs["days"] = mask
		}
		if tz != "" {
			callArgs["tz"] = tz
		}
	case every != "":
		d, err := time.ParseDuration(every)
		if err != nil || d < time.Second {
			fmt.Fprintf(stderr, "%s schedule edit: bad --every duration %q\n", brand.CLI, every)
			return 2
		}
		callArgs["interval_sec"] = int64(d / time.Second)
		if between != "" {
			start, end, err := parseWindow(between)
			if err != nil {
				fmt.Fprintf(stderr, "%s schedule edit: bad --between %q: %v\n", brand.CLI, between, err)
				return 2
			}
			callArgs["window_start"] = start
			callArgs["window_end"] = end
			if days != "" {
				mask, err := cadence.ParseDays(days)
				if err != nil {
					fmt.Fprintf(stderr, "%s schedule edit: bad --days %q: %v\n", brand.CLI, days, err)
					return 2
				}
				callArgs["days"] = mask
			}
			if tz != "" {
				callArgs["tz"] = tz
			}
		}
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdScheduleEdit, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule edit: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	if updated, _ := res["updated"].(bool); !updated {
		fmt.Fprintf(stderr, "%s schedule edit: not found (%s)\n", brand.CLI, id)
		return 3
	}
	cad, _ := res["cadence"].(string)
	fmt.Fprintf(stdout, "%s updated (%s)\n", id, cad)
	return 0
}

// cmdScheduleEnable backs `schedule pause` (enabled=false) and `resume`
// (enabled=true).
