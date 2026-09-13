// SPDX-License-Identifier: MIT
//
// cmd/agt workboard mutation sub-commands: cmdWorkboardCreate +
// cmdWorkboardClaim + cmdWorkboardHeartbeat + cmdWorkboardComment +
// cmdWorkboardBlock + cmdWorkboardFail + cmdWorkboardSeat +
// cmdWorkboardActor + cmdWorkboardLink + cmdWorkboardPolicy +
// cmdWorkboardDepend + cmdWorkboardReclaim + cmdWorkboardSweep +
// cmdWorkboardDispatch + cmdWorkboardWatch.
// Extracted from workboard.go during the Day-209 god-file split.
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

func cmdWorkboardCreate(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	callArgs := map[string]any{}
	var tags, artifacts, criteria []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--title", "--desc", "--description", "--assignee", "--priority", "--tenant", "--status", "--owner", "--idempotency-key", "--max-attempts", "--escalate-to", "--tag", "--artifact", "--criterion", "--seat":
			val, ok := workboardFlagValue(args, &i, a, stderr, "create")
			if !ok {
				return 2
			}
			switch a {
			case "--title":
				callArgs["title"] = val
			case "--desc", "--description":
				callArgs["description"] = val
			case "--assignee":
				callArgs["assignee"] = val
			case "--tenant":
				callArgs["tenant"] = val
			case "--status":
				callArgs["status"] = val
			case "--owner":
				callArgs["owner"] = val
			case "--idempotency-key":
				callArgs["idempotency_key"] = val
			case "--escalate-to":
				callArgs["escalate_to"] = val
			case "--priority":
				n, err := strconv.Atoi(val)
				if err != nil {
					fmt.Fprintf(stderr, "%s workboard create: --priority needs an integer\n", brand.CLI)
					return 2
				}
				callArgs["priority"] = n
			case "--max-attempts":
				n, err := strconv.Atoi(val)
				if err != nil || n < 1 {
					fmt.Fprintf(stderr, "%s workboard create: --max-attempts needs a positive integer\n", brand.CLI)
					return 2
				}
				callArgs["max_attempts"] = n
			case "--tag":
				tags = append(tags, val)
			case "--artifact":
				artifacts = append(artifacts, val)
			case "--criterion":
				criteria = append(criteria, val)
			case "--seat":
				callArgs["seat"] = val
			}
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s workboard create: unexpected flag %q\n", brand.CLI, a)
				return 2
			}
			if _, exists := callArgs["title"]; !exists {
				callArgs["title"] = a
				continue
			}
			fmt.Fprintf(stderr, "%s workboard create: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if len(tags) > 0 {
		callArgs["tags"] = tags
	}
	if len(artifacts) > 0 {
		callArgs["artifacts"] = artifacts
	}
	if len(criteria) > 0 {
		callArgs["criteria"] = criteria
	}
	if str(callArgs["title"]) == "" {
		fmt.Fprintf(stderr, "usage: %s workboard create --title T [--desc D]\n", brand.CLI)
		return 2
	}
	res, code := callWorkboard(controlplane.CmdWorkboardCreate, callArgs, stderr)
	if code != 0 {
		return code
	}
	task := mapAny(res["task"])
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	verb := "updated"
	if created, _ := res["created"].(bool); created {
		verb = "created"
	}
	fmt.Fprintf(stdout, "%s ", verb)
	renderWorkboardTaskLine(stdout, task)
	return 0
}

func cmdWorkboardClaim(args []string, stdout, stderr io.Writer) int {
	id, agent, runID, asJSON, ok := parseWorkboardAgentArgs(args, "claim", stderr)
	if !ok {
		return 2
	}
	res, code := callWorkboard(controlplane.CmdWorkboardClaim, map[string]any{"id": id, "agent": agent, "run_id": runID}, stderr)
	return renderWorkboardMutation(res, code, asJSON, stdout)
}

func cmdWorkboardHeartbeat(args []string, stdout, stderr io.Writer) int {
	id, agent, runID, asJSON, ok := parseWorkboardAgentArgs(args, "heartbeat", stderr)
	if !ok {
		return 2
	}
	res, code := callWorkboard(controlplane.CmdWorkboardHeartbeat, map[string]any{"id": id, "agent": agent, "run_id": runID}, stderr)
	return renderWorkboardMutation(res, code, asJSON, stdout)
}

func cmdWorkboardComment(args []string, stdout, stderr io.Writer) int {
	id, asJSON, callArgs, ok := parseWorkboardIDActorArgs(args, "comment", "author", stderr)
	if !ok {
		return 2
	}
	callArgs["id"] = id
	if str(callArgs["body"]) == "" {
		fmt.Fprintf(stderr, "%s workboard comment: --body required\n", brand.CLI)
		return 2
	}
	res, code := callWorkboard(controlplane.CmdWorkboardComment, callArgs, stderr)
	return renderWorkboardMutation(res, code, asJSON, stdout)
}

func cmdWorkboardBlock(args []string, stdout, stderr io.Writer) int {
	id, asJSON, callArgs, ok := parseWorkboardIDActorArgs(args, "block", "actor", stderr)
	if !ok {
		return 2
	}
	callArgs["id"] = id
	if str(callArgs["reason"]) == "" {
		fmt.Fprintf(stderr, "%s workboard block: --reason required\n", brand.CLI)
		return 2
	}
	res, code := callWorkboard(controlplane.CmdWorkboardBlock, callArgs, stderr)
	return renderWorkboardMutation(res, code, asJSON, stdout)
}

func cmdWorkboardFail(args []string, stdout, stderr io.Writer) int {
	id, asJSON, callArgs, ok := parseWorkboardIDActorArgs(args, "fail", "actor", stderr)
	if !ok {
		return 2
	}
	callArgs["id"] = id
	if str(callArgs["reason"]) == "" {
		fmt.Fprintf(stderr, "%s workboard fail: --reason required\n", brand.CLI)
		return 2
	}
	res, code := callWorkboard(controlplane.CmdWorkboardFail, callArgs, stderr)
	if code != 0 {
		return code
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	task := mapAny(res["task"])
	renderWorkboardTaskLine(stdout, task)
	if decision := mapAny(res["decision"]); len(decision) > 0 {
		fmt.Fprintf(stdout, "policy: action=%s failures=%d/%d", str(decision["action"]), intNumber(decision["failure_count"]), intNumber(decision["max_attempts"]))
		if next := intNumber(decision["next_attempt"]); next > 0 {
			fmt.Fprintf(stdout, " next=%d", next)
		}
		if esc := str(decision["escalate_to"]); esc != "" {
			fmt.Fprintf(stdout, " escalate_to=%s", esc)
		}
		fmt.Fprintln(stdout)
	}
	return 0
}

func cmdWorkboardSeat(args []string, stdout, stderr io.Writer) int {
	id, seatID, asJSON := "", "", false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s workboard seat: unexpected flag %q\n", brand.CLI, a)
				return 2
			}
			if id == "" {
				id = a
			} else if seatID == "" {
				seatID = a
			}
		}
	}
	if id == "" || seatID == "" {
		fmt.Fprintf(stderr, "usage: %s workboard seat <id> <seat>   (seat: default|reader|builder|isolated; \"default\" clears)\n", brand.CLI)
		return 2
	}
	if seatID == "default" || seatID == "none" || seatID == "clear" {
		seatID = ""
	}
	res, code := callWorkboard(controlplane.CmdWorkboardSeat, map[string]any{"id": id, "seat": seatID}, stderr)
	return renderWorkboardMutation(res, code, asJSON, stdout)
}

func cmdWorkboardActor(args []string, stdout, stderr io.Writer, name, cmd string) int {
	id, asJSON, callArgs, ok := parseWorkboardIDActorArgs(args, name, "actor", stderr)
	if !ok {
		return 2
	}
	callArgs["id"] = id
	res, code := callWorkboard(cmd, callArgs, stderr)
	return renderWorkboardMutation(res, code, asJSON, stdout)
}

func cmdWorkboardLink(args []string, stdout, stderr io.Writer) int {
	id, asJSON, callArgs, ok := parseWorkboardIDActorArgs(args, "link", "actor", stderr)
	if !ok {
		return 2
	}
	callArgs["id"] = id
	if str(callArgs["type"]) == "" || str(callArgs["target"]) == "" {
		fmt.Fprintf(stderr, "%s workboard link: --type and --target required\n", brand.CLI)
		return 2
	}
	res, code := callWorkboard(controlplane.CmdWorkboardLink, callArgs, stderr)
	return renderWorkboardMutation(res, code, asJSON, stdout)
}

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

