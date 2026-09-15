// SPDX-License-Identifier: MIT
//
// cmd/agt schedule add sub-command. Split from schedule.go during Day 211
// god-file refactor (#37). Public API unchanged.
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

func cmdScheduleAdd(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	once := false
	var every, continuous, at, in, between, days, tz, model, agentRef, workflowRef, systemTask, toolRef, payloadSpec string
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--once":
			once = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s schedule add [\"<agent-task|label>\"] (--every <dur> [--between <HH:MM-HH:MM> [--days <spec>]] | --continuous <dur> | --at <HH:MM> [--days <spec>] | --once --at <HH:MM> | --in <dur>) [--tz <IANA>] [--agent <slug>] [--workflow <ref> [--payload JSON] | --system-task %s | --tool <name> [--payload JSON]] [--model <id>] [--json]\n", brand.CLI, scheduleSystemTaskUsage())
			return 0
		case "--tz":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --tz needs an IANA zone (e.g. America/New_York)\n", brand.CLI)
				return 2
			}
			i++
			tz = args[i]
		case "--between":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --between needs a HH:MM-HH:MM window\n", brand.CLI)
				return 2
			}
			i++
			between = args[i]
		case "--every":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --every needs a duration\n", brand.CLI)
				return 2
			}
			i++
			every = args[i]
		case "--continuous":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --continuous needs a cooldown duration\n", brand.CLI)
				return 2
			}
			i++
			continuous = args[i]
		case "--at":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --at needs a HH:MM time\n", brand.CLI)
				return 2
			}
			i++
			at = args[i]
		case "--in":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --in needs a duration\n", brand.CLI)
				return 2
			}
			i++
			in = args[i]
		case "--days":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --days needs a spec (e.g. mon-fri, weekends)\n", brand.CLI)
				return 2
			}
			i++
			days = args[i]
		case "--model":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --model needs a value\n", brand.CLI)
				return 2
			}
			i++
			model = args[i]
		case "--agent":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --agent needs a roster slug\n", brand.CLI)
				return 2
			}
			i++
			agentRef = args[i]
		case "--workflow":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --workflow needs a workflow name or id\n", brand.CLI)
				return 2
			}
			i++
			workflowRef = args[i]
		case "--system-task":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --system-task needs a task name\n", brand.CLI)
				return 2
			}
			i++
			systemTask = args[i]
		case "--tool":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --tool needs a registered tool name\n", brand.CLI)
				return 2
			}
			i++
			toolRef = args[i]
		case "--payload":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --payload needs a JSON value\n", brand.CLI)
				return 2
			}
			i++
			payloadSpec = args[i]
		default:
			positional = append(positional, a)
		}
	}
	intent := strings.TrimSpace(strings.Join(positional, " "))
	if intent == "" && strings.TrimSpace(workflowRef) == "" && strings.TrimSpace(systemTask) == "" && strings.TrimSpace(toolRef) == "" {
		fmt.Fprintf(stderr, "%s schedule add: an agent task or typed target is required\n", brand.CLI)
		return 2
	}
	targets := 0
	for _, s := range []string{workflowRef, systemTask, toolRef} {
		if strings.TrimSpace(s) != "" {
			targets++
		}
	}
	if targets > 1 {
		fmt.Fprintf(stderr, "%s schedule add: choose only one of --workflow, --system-task, or --tool\n", brand.CLI)
		return 2
	}
	if strings.TrimSpace(agentRef) != "" && strings.TrimSpace(systemTask) != "" {
		fmt.Fprintf(stderr, "%s schedule add: --agent can run workflow/tool schedules, not system tasks\n", brand.CLI)
		return 2
	}
	if payloadSpec != "" && strings.TrimSpace(workflowRef) == "" && strings.TrimSpace(toolRef) == "" {
		fmt.Fprintf(stderr, "%s schedule add: --payload applies only to --workflow or --tool\n", brand.CLI)
		return 2
	}
	// Exactly one cadence source: --every (interval/window), --continuous
	// (completion-anchored loop), --at (daily/one-shot), or --in (one-shot
	// relative).
	sources := 0
	for _, s := range []string{every, continuous, at, in} {
		if s != "" {
			sources++
		}
	}
	if sources != 1 {
		fmt.Fprintf(stderr, "%s schedule add: pass exactly one of --every <dur>, --continuous <dur>, --at <HH:MM>, or --in <dur>\n", brand.CLI)
		return 2
	}
	if continuous != "" && (between != "" || days != "" || tz != "" || once) {
		fmt.Fprintf(stderr, "%s schedule add: --continuous cannot combine with --between, --days, --tz, or --once\n", brand.CLI)
		return 2
	}
	if days != "" && at == "" && between == "" {
		fmt.Fprintf(stderr, "%s schedule add: --days applies to --at (daily) or --every+--between (windowed) schedules\n", brand.CLI)
		return 2
	}
	if between != "" && every == "" {
		fmt.Fprintf(stderr, "%s schedule add: --between requires --every <dur> (a windowed interval)\n", brand.CLI)
		return 2
	}
	if tz != "" && !((at != "" && !once) || between != "") {
		fmt.Fprintf(stderr, "%s schedule add: --tz applies to --at (daily) or --every+--between (windowed) schedules\n", brand.CLI)
		return 2
	}
	if once && at == "" {
		fmt.Fprintf(stderr, "%s schedule add: --once requires --at <HH:MM> (use --in for a relative one-shot)\n", brand.CLI)
		return 2
	}
	if once && days != "" {
		fmt.Fprintf(stderr, "%s schedule add: --days cannot combine with --once (a one-shot has no recurrence)\n", brand.CLI)
		return 2
	}

	callArgs := map[string]any{}
	if intent != "" {
		callArgs["intent"] = intent
	}
	var human string
	switch {
	case in != "":
		d, err := time.ParseDuration(in)
		if err != nil {
			fmt.Fprintf(stderr, "%s schedule add: bad --in duration %q: %v\n", brand.CLI, in, err)
			return 2
		}
		if d < time.Second {
			fmt.Fprintf(stderr, "%s schedule add: --in must be at least 1s\n", brand.CLI)
			return 2
		}
		fireAt := time.Now().Add(d)
		callArgs["once_at_unix"] = fireAt.Unix()
		human = "once at " + fireAt.Format("2006-01-02 15:04")
	case at != "" && once:
		mins, err := parseHHMM(at)
		if err != nil {
			fmt.Fprintf(stderr, "%s schedule add: bad --at %q: %v\n", brand.CLI, at, err)
			return 2
		}
		fireAt := nextWallclock(time.Now(), mins)
		callArgs["once_at_unix"] = fireAt.Unix()
		human = "once at " + fireAt.Format("2006-01-02 15:04")
	case at != "":
		mins, err := parseHHMM(at)
		if err != nil {
			fmt.Fprintf(stderr, "%s schedule add: bad --at %q: %v\n", brand.CLI, at, err)
			return 2
		}
		callArgs["at_minutes"] = mins
		human = "daily at " + at
		if days != "" {
			mask, err := cadence.ParseDays(days)
			if err != nil {
				fmt.Fprintf(stderr, "%s schedule add: bad --days %q: %v\n", brand.CLI, days, err)
				return 2
			}
			callArgs["days"] = mask
			if label := cadence.FormatDays(mask); label != "" {
				human = label + " at " + at
			}
		}
		if tz != "" {
			callArgs["tz"] = tz
			human += " " + tz
		}
	case continuous != "":
		d, err := time.ParseDuration(continuous)
		if err != nil {
			fmt.Fprintf(stderr, "%s schedule add: bad --continuous duration %q: %v\n", brand.CLI, continuous, err)
			return 2
		}
		if d < time.Second {
			fmt.Fprintf(stderr, "%s schedule add: --continuous cooldown must be at least 1s\n", brand.CLI)
			return 2
		}
		callArgs["cooldown_sec"] = int64(d / time.Second)
		human = "continuous · " + d.String() + " cooldown"
	default: // every (plain interval, or windowed when --between is set)
		d, err := time.ParseDuration(every)
		if err != nil {
			fmt.Fprintf(stderr, "%s schedule add: bad --every duration %q: %v\n", brand.CLI, every, err)
			return 2
		}
		if d < time.Second {
			fmt.Fprintf(stderr, "%s schedule add: interval must be at least 1s\n", brand.CLI)
			return 2
		}
		callArgs["interval_sec"] = int64(d / time.Second)
		if between != "" {
			start, end, err := parseWindow(between)
			if err != nil {
				fmt.Fprintf(stderr, "%s schedule add: bad --between %q: %v\n", brand.CLI, between, err)
				return 2
			}
			callArgs["window_start"] = start
			callArgs["window_end"] = end
			human = fmt.Sprintf("every %s %s", d, between)
			if days != "" {
				mask, err := cadence.ParseDays(days)
				if err != nil {
					fmt.Fprintf(stderr, "%s schedule add: bad --days %q: %v\n", brand.CLI, days, err)
					return 2
				}
				callArgs["days"] = mask
				if label := cadence.FormatDays(mask); label != "" {
					human += " " + label
				}
			}
			if tz != "" {
				callArgs["tz"] = tz
				human += " " + tz
			}
		} else {
			human = "every " + d.String()
		}
	}
	if model != "" {
		callArgs["model"] = model
	}
	if strings.TrimSpace(agentRef) != "" {
		callArgs["agent"] = strings.TrimSpace(agentRef)
	}
	if strings.TrimSpace(workflowRef) != "" {
		callArgs["target"] = "workflow"
		callArgs["workflow"] = strings.TrimSpace(workflowRef)
		if payloadSpec != "" {
			var payload any
			if err := json.Unmarshal([]byte(payloadSpec), &payload); err != nil {
				fmt.Fprintf(stderr, "%s schedule add: bad --payload JSON: %v\n", brand.CLI, err)
				return 2
			}
			callArgs["payload"] = payload
		}
	}
	if strings.TrimSpace(systemTask) != "" {
		callArgs["target"] = cadence.TargetSystemTask
		callArgs["system_task"] = strings.TrimSpace(systemTask)
	}
	if strings.TrimSpace(toolRef) != "" {
		callArgs["target"] = cadence.TargetTool
		callArgs["tool"] = strings.TrimSpace(toolRef)
		if payloadSpec != "" {
			var payload any
			if err := json.Unmarshal([]byte(payloadSpec), &payload); err != nil {
				fmt.Fprintf(stderr, "%s schedule add: bad --payload JSON: %v\n", brand.CLI, err)
				return 2
			}
			callArgs["payload"] = payload
		}
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdScheduleAdd, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule add: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	id, _ := res["id"].(string)
	fmt.Fprintf(stdout, "scheduled %s (%s)\n", id, human)
	return 0
}

// cmdScheduleEdit changes an existing schedule in place: any of --intent,
// --agent, one target flag (--workflow/--system-task/--tool), --model, and at
// most one new cadence (--every | --continuous | --at [--days] | --in |
// --once --at). The id is preserved; the cadence change recomputes the next run.
