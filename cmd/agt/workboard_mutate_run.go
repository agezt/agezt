// SPDX-License-Identifier: MIT
//
// cmd/agt workboard mutation sub-commands (sweep/dispatch/watch). Split
// from workboard_mutate.go during Day 211 god-file refactor (#30).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

func cmdWorkboardSweep(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	callArgs := map[string]any{"stale_after_ms": int((10 * time.Minute).Milliseconds()), "limit": 100}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--actor":
			val, ok := workboardFlagValue(args, &i, a, stderr, "sweep")
			if !ok {
				return 2
			}
			callArgs["actor"] = val
		case "--stale-after":
			val, ok := workboardFlagValue(args, &i, a, stderr, "sweep")
			if !ok {
				return 2
			}
			d, err := time.ParseDuration(val)
			if err != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s workboard sweep: --stale-after needs a duration like 10m\n", brand.CLI)
				return 2
			}
			callArgs["stale_after_ms"] = int(d.Milliseconds())
		case "--limit":
			val, ok := workboardFlagValue(args, &i, a, stderr, "sweep")
			if !ok {
				return 2
			}
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				fmt.Fprintf(stderr, "%s workboard sweep: --limit needs a positive integer\n", brand.CLI)
				return 2
			}
			callArgs["limit"] = n
		default:
			fmt.Fprintf(stderr, "%s workboard sweep: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	res, code := callWorkboard(controlplane.CmdWorkboardSweep, callArgs, stderr)
	if code != 0 {
		return code
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	tasks, _ := res["tasks"].([]any)
	if len(tasks) == 0 {
		fmt.Fprintln(stdout, "no stale workboard claims reclaimed")
		return 0
	}
	for _, raw := range tasks {
		renderWorkboardTaskLine(stdout, mapAny(raw))
	}
	fmt.Fprintf(stdout, "%v stale claim(s) reclaimed\n", res["reclaimed_count"])
	return 0
}

func cmdWorkboardDispatch(args []string, stdout, stderr io.Writer) int {
	id := ""
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--agent", "--intent", "--reason":
			val, ok := workboardFlagValue(args, &i, a, stderr, "dispatch")
			if !ok {
				return 2
			}
			callArgs[strings.TrimPrefix(a, "--")] = val
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s workboard dispatch: unexpected flag %q\n", brand.CLI, a)
				return 2
			}
			if id == "" {
				id = a
				continue
			}
			fmt.Fprintf(stderr, "%s workboard dispatch: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "usage: %s workboard dispatch <id> [--agent A] [--intent TEXT] [--reason R]\n", brand.CLI)
		return 2
	}
	callArgs["id"] = id
	res, code := callWorkboard(controlplane.CmdWorkboardDispatch, callArgs, stderr)
	if code != 0 {
		return code
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	fmt.Fprintf(stdout, "dispatched %s to %s", shortID(id), str(res["agent"]))
	if corr := str(res["correlation_id"]); corr != "" {
		fmt.Fprintf(stdout, " corr=%s", corr)
	}
	fmt.Fprintln(stdout)
	return 0
}

func cmdWorkboardWatch(args []string, stdout, stderr io.Writer) int {
	id := ""
	asJSON := false
	follow := false
	interval := 2 * time.Second
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--follow":
			follow = true
		case "--run", "--limit", "--interval":
			val, ok := workboardFlagValue(args, &i, a, stderr, "watch")
			if !ok {
				return 2
			}
			switch a {
			case "--run":
				callArgs["run_id"] = val
			case "--limit":
				n, err := strconv.Atoi(val)
				if err != nil || n < 1 {
					fmt.Fprintf(stderr, "%s workboard watch: --limit needs a positive integer\n", brand.CLI)
					return 2
				}
				callArgs["limit"] = n
			case "--interval":
				d, err := time.ParseDuration(val)
				if err != nil || d <= 0 {
					fmt.Fprintf(stderr, "%s workboard watch: --interval needs a duration like 2s\n", brand.CLI)
					return 2
				}
				interval = d
			}
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s workboard watch: unexpected flag %q\n", brand.CLI, a)
				return 2
			}
			if id == "" {
				id = a
				continue
			}
			fmt.Fprintf(stderr, "%s workboard watch: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "usage: %s workboard watch <id> [--run R] [--limit N] [--follow] [--json]\n", brand.CLI)
		return 2
	}
	callArgs["id"] = id
	for {
		res, code := callWorkboard(controlplane.CmdWorkboardWatch, callArgs, stderr)
		if code != 0 {
			return code
		}
		if asJSON {
			if rc := jsonout.Write(stdout, res); rc != 0 {
				return rc
			}
		} else {
			renderWorkboardWatch(stdout, res)
		}
		if !follow || workboardWatchTerminal(res) {
			return 0
		}
		time.Sleep(interval)
		if !asJSON {
			fmt.Fprintln(stdout, "---")
		}
	}
}
