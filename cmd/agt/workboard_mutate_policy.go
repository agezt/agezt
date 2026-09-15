// SPDX-License-Identifier: MIT
//
// cmd/agt workboard mutation sub-commands (policy/depend/reclaim). Split
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
)

func cmdWorkboardPolicy(args []string, stdout, stderr io.Writer) int {
	id := ""
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--clear":
			callArgs["clear"] = true
		case "--actor", "--max-attempts", "--escalate-to":
			val, ok := workboardFlagValue(args, &i, a, stderr, "policy")
			if !ok {
				return 2
			}
			switch a {
			case "--actor":
				callArgs["actor"] = val
			case "--escalate-to":
				callArgs["escalate_to"] = val
			case "--max-attempts":
				n, err := strconv.Atoi(val)
				if err != nil || n < 1 {
					fmt.Fprintf(stderr, "%s workboard policy: --max-attempts needs a positive integer\n", brand.CLI)
					return 2
				}
				callArgs["max_attempts"] = n
			}
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s workboard policy: unexpected flag %q\n", brand.CLI, a)
				return 2
			}
			if id == "" {
				id = a
				continue
			}
			fmt.Fprintf(stderr, "%s workboard policy: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" || (!truthy(callArgs["clear"]) && intNumber(callArgs["max_attempts"]) == 0) {
		fmt.Fprintf(stderr, "usage: %s workboard policy <id> --max-attempts N [--escalate-to A] [--actor A] [--clear] [--json]\n", brand.CLI)
		return 2
	}
	callArgs["id"] = id
	res, code := callWorkboard(controlplane.CmdWorkboardPolicy, callArgs, stderr)
	return renderWorkboardMutation(res, code, asJSON, stdout)
}

func cmdWorkboardDepend(args []string, stdout, stderr io.Writer) int {
	id := ""
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--on", "--depends-on":
			val, ok := workboardFlagValue(args, &i, a, stderr, "depend")
			if !ok {
				return 2
			}
			callArgs["depends_on"] = val
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s workboard depend: unexpected flag %q\n", brand.CLI, a)
				return 2
			}
			if id == "" {
				id = a
				continue
			}
			fmt.Fprintf(stderr, "%s workboard depend: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" || str(callArgs["depends_on"]) == "" {
		fmt.Fprintf(stderr, "usage: %s workboard depend <id> --on TASK [--json]\n", brand.CLI)
		return 2
	}
	callArgs["id"] = id
	res, code := callWorkboard(controlplane.CmdWorkboardDepend, callArgs, stderr)
	return renderWorkboardMutation(res, code, asJSON, stdout)
}

func cmdWorkboardReclaim(args []string, stdout, stderr io.Writer) int {
	id := ""
	asJSON := false
	callArgs := map[string]any{"stale_after_ms": int((10 * time.Minute).Milliseconds())}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--actor":
			val, ok := workboardFlagValue(args, &i, a, stderr, "reclaim")
			if !ok {
				return 2
			}
			callArgs["actor"] = val
		case "--stale-after":
			val, ok := workboardFlagValue(args, &i, a, stderr, "reclaim")
			if !ok {
				return 2
			}
			d, err := time.ParseDuration(val)
			if err != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s workboard reclaim: --stale-after needs a duration like 10m\n", brand.CLI)
				return 2
			}
			callArgs["stale_after_ms"] = int(d.Milliseconds())
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s workboard reclaim: unexpected flag %q\n", brand.CLI, a)
				return 2
			}
			if id == "" {
				id = a
				continue
			}
			fmt.Fprintf(stderr, "%s workboard reclaim: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "usage: %s workboard reclaim <id> [--actor A] [--stale-after 10m] [--json]\n", brand.CLI)
		return 2
	}
	callArgs["id"] = id
	res, code := callWorkboard(controlplane.CmdWorkboardReclaim, callArgs, stderr)
	return renderWorkboardMutation(res, code, asJSON, stdout)
}
