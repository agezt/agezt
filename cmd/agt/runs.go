// SPDX-License-Identifier: MIT
//
// cmd/agt runs sub-command top-level handlers (cmdRuns, cmdRunsSteerVerb,
// cmdRunsSteer). Extracted from runs.go during Day 211 god-file refactor
// (#36, #59). Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)

func cmdRuns(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s runs: subcommand required (list|show)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "list":
		return cmdRunsList(args[1:], stdout, stderr)
	case "show":
		return cmdRunsShow(args[1:], stdout, stderr)
	case "last":
		return cmdRunsLast(args[1:], stdout, stderr)
	case "stats":
		return cmdRunsStats(args[1:], stdout, stderr)
	case "cancel":
		return cmdRunsCancel(args[1:], stdout, stderr)
	case "pause":
		return cmdRunsSteerVerb(controlplane.CmdRunPause, "pause", args[1:], stdout, stderr)
	case "resume":
		return cmdRunsSteerVerb(controlplane.CmdRunResume, "resume", args[1:], stdout, stderr)
	case "step":
		return cmdRunsSteerVerb(controlplane.CmdRunStep, "step", args[1:], stdout, stderr)
	case "steer":
		return cmdRunsSteer(args[1:], stdout, stderr)
	case "intervene":
		return cmdRunsIntervene(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s runs <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  list [N] [--json]            show the last N agent runs (default 20)\n")
		fmt.Fprintf(stdout, "  show <correlation> [--json]  render one run as a task arc\n")
		fmt.Fprintf(stdout, "  last [--json]                shorthand for show <newest correlation>\n")
		fmt.Fprintf(stdout, "  stats [--json]               aggregate run health (counts, success rate, durations)\n")
		fmt.Fprintf(stdout, "  cancel <correlation>         cancel one in-flight run (→ failed/canceled)\n")
		fmt.Fprintf(stdout, "  pause <correlation>          park a live run at its next iteration boundary\n")
		fmt.Fprintf(stdout, "  resume <correlation>         let a paused run continue\n")
		fmt.Fprintf(stdout, "  step <correlation>           advance a run one iteration, then re-pause\n")
		fmt.Fprintf(stdout, "  steer <correlation> <text>   inject an operator directive into a live run\n")
		fmt.Fprintf(stdout, "  intervene <primitive> <correlation> [text] [--lease <dur>] [--key <id>]\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s runs: unknown subcommand %q (list|show|last|stats|cancel|pause|resume|step|steer|intervene)\n", brand.CLI, args[0])
		return 2
	}
}
func cmdRunsSteerVerb(cmd, verb string, args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var corr string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s runs %s <correlation> [--json]\n", brand.CLI, verb)
			return 0
		default:
			if corr == "" {
				corr = a
				continue
			}
			fmt.Fprintf(stderr, "%s runs %s: unexpected arg %q\n", brand.CLI, verb, a)
			return 2
		}
	}
	if corr == "" {
		fmt.Fprintf(stderr, "%s runs %s: correlation id required\n", brand.CLI, verb)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, map[string]any{"correlation": corr})
	if err != nil {
		fmt.Fprintf(stderr, "%s runs %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	if ok, _ := res["ok"].(bool); ok {
		fmt.Fprintf(stdout, "%s: %s\n", corr, verb+"d")
		return 0
	}
	fmt.Fprintf(stderr, "no in-flight run with correlation %q (already finished or unknown)\n", corr)
	return 1
}
func cmdRunsSteer(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var corr string
	var parts []string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s runs steer <correlation> <directive...> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "inject a directive into a running agent (folded into its next prompt)\n")
			return 0
		case corr == "":
			corr = a
		default:
			parts = append(parts, a)
		}
	}
	if corr == "" || len(parts) == 0 {
		fmt.Fprintf(stderr, "%s runs steer: correlation id and a directive are required\n", brand.CLI)
		return 2
	}
	directive := strings.Join(parts, " ")
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdRunSteer, map[string]any{"correlation": corr, "directive": directive})
	if err != nil {
		fmt.Fprintf(stderr, "%s runs steer: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	if acc, _ := res["accepted"].(bool); acc {
		fmt.Fprintf(stdout, "%s: steering directive accepted\n", corr)
		return 0
	}
	fmt.Fprintf(stderr, "no in-flight run with correlation %q (already finished or unknown)\n", corr)
	return 1
}
