// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdSchedule dispatches `agt schedule <subcommand>` — the operator's
// management path into the persistent scheduled-intents store (autonomy). The
// cadence resident fires due schedules through the same governed loop as
// `agt run`; these commands add/list/remove/trigger them.
func cmdSchedule(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s schedule: subcommand required (add|list|rm|run)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "add":
		return cmdScheduleAdd(args[1:], stdout, stderr)
	case "list", "ls":
		return cmdScheduleList(args[1:], stdout, stderr)
	case "rm", "remove":
		return cmdScheduleRemove(args[1:], stdout, stderr)
	case "run", "trigger":
		return cmdScheduleRun(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s schedule <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  add \"<intent>\" --every <dur> [--model <id>] [--json]   schedule a recurring intent\n")
		fmt.Fprintf(stdout, "  list [--json]                                          list all schedules\n")
		fmt.Fprintf(stdout, "  rm <id> [--json]                                       delete a schedule\n")
		fmt.Fprintf(stdout, "  run <id> [--json]                                      fire a schedule now (next tick)\n")
		fmt.Fprintf(stdout, "  <dur> is a Go duration: 30m, 1h, 24h, …\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s schedule: unknown subcommand %q (add|list|rm|run)\n", brand.CLI, args[0])
		return 2
	}
}

func cmdScheduleAdd(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var every, model string
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s schedule add \"<intent>\" --every <dur> [--model <id>] [--json]\n", brand.CLI)
			return 0
		case "--every":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --every needs a duration\n", brand.CLI)
				return 2
			}
			i++
			every = args[i]
		case "--model":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule add: --model needs a value\n", brand.CLI)
				return 2
			}
			i++
			model = args[i]
		default:
			positional = append(positional, a)
		}
	}
	intent := strings.TrimSpace(strings.Join(positional, " "))
	if intent == "" {
		fmt.Fprintf(stderr, "%s schedule add: an intent is required\n", brand.CLI)
		return 2
	}
	if every == "" {
		fmt.Fprintf(stderr, "%s schedule add: --every <dur> is required (e.g. 1h)\n", brand.CLI)
		return 2
	}
	d, err := time.ParseDuration(every)
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule add: bad --every duration %q: %v\n", brand.CLI, every, err)
		return 2
	}
	if d < time.Second {
		fmt.Fprintf(stderr, "%s schedule add: interval must be at least 1s\n", brand.CLI)
		return 2
	}

	callArgs := map[string]any{"intent": intent, "interval_sec": int64(d / time.Second)}
	if model != "" {
		callArgs["model"] = model
	}
	c := dial(stderr)
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
		return encodeJSON(stdout, res)
	}
	id, _ := res["id"].(string)
	fmt.Fprintf(stdout, "scheduled %s (every %s)\n", id, d)
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
	c := dial(stderr)
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
		return encodeJSON(stdout, res)
	}
	list, _ := res["schedules"].([]any)
	if len(list) == 0 {
		fmt.Fprintf(stdout, "no schedules. Add one with `%s schedule add \"<intent>\" --every 1h`.\n", brand.CLI)
		return 0
	}
	for _, item := range list {
		m, _ := item.(map[string]any)
		id, _ := m["id"].(string)
		intent, _ := m["intent"].(string)
		sec, _ := m["interval_sec"].(float64)
		source, _ := m["source"].(string)
		enabled, _ := m["enabled"].(bool)
		next, _ := m["next_run_unix"].(float64)
		state := "enabled"
		if !enabled {
			state = "disabled"
		}
		nextStr := "—"
		if next > 0 {
			nextStr = time.Unix(int64(next), 0).Format("2006-01-02 15:04")
		}
		fmt.Fprintf(stdout, "  %-22s every %-8s [%s,%s] next %s  %q\n",
			id, (time.Duration(sec) * time.Second).String(), source, state, nextStr, intent)
	}
	return 0
}

func cmdScheduleRemove(args []string, stdout, stderr io.Writer) int {
	return scheduleByID(args, stdout, stderr, "rm", controlplane.CmdScheduleRemove, "removed", "removed", "not found")
}

func cmdScheduleRun(args []string, stdout, stderr io.Writer) int {
	return scheduleByID(args, stdout, stderr, "run", controlplane.CmdScheduleRun, "triggered", "triggered (fires on the next tick)", "not found")
}

// scheduleByID factors the rm/run commands: both take a single id, call a
// control-plane command, and report a boolean result key.
func scheduleByID(args []string, stdout, stderr io.Writer, verb, cmd, resultKey, okMsg, missMsg string) int {
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
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	ok, _ := res[resultKey].(bool)
	if !ok {
		fmt.Fprintf(stderr, "%s schedule %s: %s (%s)\n", brand.CLI, verb, missMsg, id)
		return 3
	}
	fmt.Fprintf(stdout, "%s %s\n", id, okMsg)
	return 0
}
