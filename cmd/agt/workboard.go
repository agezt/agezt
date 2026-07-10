// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdWorkboard(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return workboardUsage(stderr)
	}
	switch args[0] {
	case "list", "ls":
		return cmdWorkboardList(args[1:], stdout, stderr)
	case "lanes":
		return cmdWorkboardLanes(args[1:], stdout, stderr)
	case "show", "get":
		return cmdWorkboardShow(args[1:], stdout, stderr)
	case "create", "add":
		return cmdWorkboardCreate(args[1:], stdout, stderr)
	case "claim":
		return cmdWorkboardClaim(args[1:], stdout, stderr)
	case "heartbeat", "beat":
		return cmdWorkboardHeartbeat(args[1:], stdout, stderr)
	case "comment":
		return cmdWorkboardComment(args[1:], stdout, stderr)
	case "block":
		return cmdWorkboardBlock(args[1:], stdout, stderr)
	case "fail":
		return cmdWorkboardFail(args[1:], stdout, stderr)
	case "unblock":
		return cmdWorkboardActor(args[1:], stdout, stderr, "unblock", controlplane.CmdWorkboardUnblock)
	case "complete", "done":
		return cmdWorkboardActor(args[1:], stdout, stderr, "complete", controlplane.CmdWorkboardComplete)
	case "prove":
		return cmdWorkboardActor(args[1:], stdout, stderr, "prove", controlplane.CmdWorkboardProve)
	case "seat":
		return cmdWorkboardSeat(args[1:], stdout, stderr)
	case "archive":
		return cmdWorkboardActor(args[1:], stdout, stderr, "archive", controlplane.CmdWorkboardArchive)
	case "link":
		return cmdWorkboardLink(args[1:], stdout, stderr)
	case "policy":
		return cmdWorkboardPolicy(args[1:], stdout, stderr)
	case "depend":
		return cmdWorkboardDepend(args[1:], stdout, stderr)
	case "reclaim":
		return cmdWorkboardReclaim(args[1:], stdout, stderr)
	case "sweep":
		return cmdWorkboardSweep(args[1:], stdout, stderr)
	case "dispatch":
		return cmdWorkboardDispatch(args[1:], stdout, stderr)
	case "watch":
		return cmdWorkboardWatch(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		return workboardUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s workboard: unknown subcommand %q\n", brand.CLI, args[0])
		return workboardUsage(stderr)
	}
}

func workboardUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s workboard <list|lanes|show|create|claim|heartbeat|comment|block|fail|unblock|complete|prove|seat|archive|link|policy|depend|reclaim|sweep|dispatch|watch>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--status S] [--assignee A] [--tenant T] [--limit N] [--archived] [--json]\n")
	fmt.Fprintf(w, "  lanes [--status S] [--tenant T] [--limit N] [--archived] [--json]\n")
	fmt.Fprintf(w, "  create --title T [--desc D] [--assignee A] [--priority N] [--tenant T] [--status S] [--idempotency-key K] [--max-attempts N] [--escalate-to A] [--tag X] [--criterion \"C\"] [--seat S] [--json]\n")
	fmt.Fprintf(w, "  show <id> [--json]\n")
	fmt.Fprintf(w, "  claim <id> --agent A [--run R] [--json]\n")
	fmt.Fprintf(w, "  heartbeat <id> --agent A [--run R] [--json]\n")
	fmt.Fprintf(w, "  comment <id> --body B [--author A] [--json]\n")
	fmt.Fprintf(w, "  block <id> --reason R [--actor A] [--json]\n")
	fmt.Fprintf(w, "  fail <id> --reason R [--actor A] [--json]\n")
	fmt.Fprintf(w, "  unblock|complete|archive <id> [--actor A] [--json]\n")
	fmt.Fprintf(w, "  prove <id> [--actor A] [--json]   (judge acceptance criteria; done if satisfied, else review)\n")
	fmt.Fprintf(w, "  seat <id> <seat>   (set execution seat: default|reader|builder|isolated; see `%s seats`)\n", brand.CLI)
	fmt.Fprintf(w, "  link <id> --type TYPE --target TARGET [--json]\n")
	fmt.Fprintf(w, "  policy <id> --max-attempts N [--escalate-to A] [--actor A] [--clear] [--json]\n")
	fmt.Fprintf(w, "  depend <id> --on TASK [--json]\n")
	fmt.Fprintf(w, "  reclaim <id> [--actor A] [--stale-after 10m] [--json]\n")
	fmt.Fprintf(w, "  sweep [--actor A] [--stale-after 10m] [--limit N] [--json]\n")
	fmt.Fprintf(w, "  dispatch <id> [--agent A] [--intent TEXT] [--reason R] [--json]\n")
	fmt.Fprintf(w, "  watch <id> [--run R] [--limit N] [--follow] [--json]\n")
	return 0
}

func cmdWorkboardList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--archived":
			callArgs["include_archived"] = true
		case "--status", "--assignee", "--tenant", "--limit":
			val, ok := workboardFlagValue(args, &i, args[i], stderr, "list")
			if !ok {
				return 2
			}
			switch args[i-1] {
			case "--status":
				callArgs["status"] = val
			case "--assignee":
				callArgs["assignee"] = val
			case "--tenant":
				callArgs["tenant"] = val
			case "--limit":
				n, err := strconv.Atoi(val)
				if err != nil || n < 1 {
					fmt.Fprintf(stderr, "%s workboard list: --limit needs a positive integer\n", brand.CLI)
					return 2
				}
				callArgs["limit"] = n
			}
		default:
			fmt.Fprintf(stderr, "%s workboard list: unexpected arg %q\n", brand.CLI, args[i])
			return 2
		}
	}
	res, code := callWorkboard(controlplane.CmdWorkboardList, callArgs, stderr)
	if code != 0 {
		return code
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	tasks, _ := res["tasks"].([]any)
	if len(tasks) == 0 {
		fmt.Fprintln(stdout, "workboard empty")
		return 0
	}
	for _, raw := range tasks {
		renderWorkboardTaskLine(stdout, mapAny(raw))
	}
	fmt.Fprintf(stdout, "%v task(s)\n", res["count"])
	return 0
}

func cmdWorkboardLanes(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--archived":
			callArgs["include_archived"] = true
		case "--status", "--tenant", "--limit":
			val, ok := workboardFlagValue(args, &i, args[i], stderr, "lanes")
			if !ok {
				return 2
			}
			switch args[i-1] {
			case "--status":
				callArgs["status"] = val
			case "--tenant":
				callArgs["tenant"] = val
			case "--limit":
				n, err := strconv.Atoi(val)
				if err != nil || n < 1 {
					fmt.Fprintf(stderr, "%s workboard lanes: --limit needs a positive integer\n", brand.CLI)
					return 2
				}
				callArgs["limit"] = n
			}
		default:
			fmt.Fprintf(stderr, "%s workboard lanes: unexpected arg %q\n", brand.CLI, args[i])
			return 2
		}
	}
	res, code := callWorkboard(controlplane.CmdWorkboardLanes, callArgs, stderr)
	if code != 0 {
		return code
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	lanes, _ := res["lanes"].([]any)
	if len(lanes) == 0 {
		fmt.Fprintln(stdout, "workboard lanes empty")
		return 0
	}
	for _, raw := range lanes {
		lane := mapAny(raw)
		fmt.Fprintf(stdout, "%s (%d)\n", str(lane["label"]), intNumber(lane["count"]))
		tasks, _ := lane["tasks"].([]any)
		for _, taskRaw := range tasks {
			fmt.Fprint(stdout, "  ")
			renderWorkboardTaskLine(stdout, mapAny(taskRaw))
		}
	}
	fmt.Fprintf(stdout, "%v lane(s), %v task(s)\n", res["count"], res["task_count"])
	return 0
}

func cmdWorkboardShow(args []string, stdout, stderr io.Writer) int {
	id, asJSON, ok := parseWorkboardIDJSON(args, "show", stderr)
	if !ok {
		return 2
	}
	res, code := callWorkboard(controlplane.CmdWorkboardShow, map[string]any{"id": id}, stderr)
	if code != 0 {
		return code
	}
	task := mapAny(res["task"])
	if asJSON {
		return encodeJSON(stdout, task)
	}
	renderWorkboardTask(stdout, task)
	return 0
}

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
		return encodeJSON(stdout, res)
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
		return encodeJSON(stdout, res)
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
		return encodeJSON(stdout, res)
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
		return encodeJSON(stdout, res)
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
			if rc := encodeJSON(stdout, res); rc != 0 {
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

func parseWorkboardIDJSON(args []string, name string, stderr io.Writer) (string, bool, bool) {
	id := ""
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s workboard %s: unexpected flag %q\n", brand.CLI, name, a)
				return "", false, false
			}
			if id != "" {
				fmt.Fprintf(stderr, "%s workboard %s: unexpected arg %q\n", brand.CLI, name, a)
				return "", false, false
			}
			id = a
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s workboard %s: task id required\n", brand.CLI, name)
		return "", false, false
	}
	return id, asJSON, true
}

func parseWorkboardAgentArgs(args []string, name string, stderr io.Writer) (string, string, string, bool, bool) {
	id := ""
	agent := ""
	runID := ""
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--agent", "--run":
			val, ok := workboardFlagValue(args, &i, args[i], stderr, name)
			if !ok {
				return "", "", "", false, false
			}
			if args[i-1] == "--agent" {
				agent = val
			} else {
				runID = val
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(stderr, "%s workboard %s: unexpected flag %q\n", brand.CLI, name, args[i])
				return "", "", "", false, false
			}
			if id == "" {
				id = args[i]
				continue
			}
			fmt.Fprintf(stderr, "%s workboard %s: unexpected arg %q\n", brand.CLI, name, args[i])
			return "", "", "", false, false
		}
	}
	if id == "" || agent == "" {
		fmt.Fprintf(stderr, "usage: %s workboard %s <id> --agent A [--run R]\n", brand.CLI, name)
		return "", "", "", false, false
	}
	return id, agent, runID, asJSON, true
}

func parseWorkboardIDActorArgs(args []string, name, actorFlag string, stderr io.Writer) (string, bool, map[string]any, bool) {
	id := ""
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--actor", "--author", "--body", "--reason", "--type", "--target":
			val, ok := workboardFlagValue(args, &i, a, stderr, name)
			if !ok {
				return "", false, nil, false
			}
			switch a {
			case "--actor", "--author":
				callArgs[actorFlag] = val
			default:
				callArgs[strings.TrimPrefix(a, "--")] = val
			}
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s workboard %s: unexpected flag %q\n", brand.CLI, name, a)
				return "", false, nil, false
			}
			if id == "" {
				id = a
				continue
			}
			fmt.Fprintf(stderr, "%s workboard %s: unexpected arg %q\n", brand.CLI, name, a)
			return "", false, nil, false
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s workboard %s: task id required\n", brand.CLI, name)
		return "", false, nil, false
	}
	return id, asJSON, callArgs, true
}

func workboardFlagValue(args []string, i *int, flag string, stderr io.Writer, cmd string) (string, bool) {
	if *i+1 >= len(args) {
		fmt.Fprintf(stderr, "%s workboard %s: %s needs a value\n", brand.CLI, cmd, flag)
		return "", false
	}
	*i = *i + 1
	return args[*i], true
}

func callWorkboard(cmd string, args map[string]any, stderr io.Writer) (map[string]any, int) {
	c := dial(stderr)
	if c == nil {
		return nil, 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, args)
	if err != nil {
		fmt.Fprintf(stderr, "%s workboard: %v\n", brand.CLI, err)
		return nil, 1
	}
	return res, 0
}

func renderWorkboardMutation(res map[string]any, code int, asJSON bool, stdout io.Writer) int {
	if code != 0 {
		return code
	}
	task := mapAny(res["task"])
	if asJSON {
		return encodeJSON(stdout, task)
	}
	renderWorkboardTaskLine(stdout, task)
	return 0
}

func renderWorkboardTaskLine(w io.Writer, task map[string]any) {
	assignee := str(task["assignee"])
	if assignee == "" {
		assignee = "-"
	}
	fmt.Fprintf(w, "%-26s %-8s p=%-3d %-16s %s\n",
		shortID(str(task["id"])), str(task["status"]), intNumber(task["priority"]), assignee, str(task["title"]))
}

func renderWorkboardTask(w io.Writer, task map[string]any) {
	fmt.Fprintf(w, "id:       %s\n", str(task["id"]))
	fmt.Fprintf(w, "title:    %s\n", str(task["title"]))
	fmt.Fprintf(w, "status:   %s\n", str(task["status"]))
	if d := str(task["description"]); d != "" {
		fmt.Fprintf(w, "desc:     %s\n", d)
	}
	if assignee := str(task["assignee"]); assignee != "" {
		fmt.Fprintf(w, "assignee: %s\n", assignee)
	}
	if priority := intNumber(task["priority"]); priority != 0 {
		fmt.Fprintf(w, "priority: %d\n", priority)
	}
	if s := str(task["seat"]); s != "" {
		fmt.Fprintf(w, "seat:     %s\n", s)
	}
	if reason := str(task["block_reason"]); reason != "" {
		fmt.Fprintf(w, "blocked:  %s\n", reason)
	}
	if policy := mapAny(task["retry_policy"]); len(policy) > 0 {
		fmt.Fprintf(w, "retry:    max_attempts=%d", intNumber(policy["max_attempts"]))
		if esc := str(policy["escalate_to"]); esc != "" {
			fmt.Fprintf(w, " escalate_to=%s", esc)
		}
		if failures := intNumber(task["failed_attempt_count"]); failures > 0 {
			fmt.Fprintf(w, " failures=%d", failures)
		}
		fmt.Fprintln(w)
	}
	if criteria, _ := task["criteria"].([]any); len(criteria) > 0 {
		met := intNumber(task["criteria_met"])
		verdict := "unproven"
		if proven, _ := task["proven"].(bool); proven {
			verdict = "PROVEN"
		}
		fmt.Fprintf(w, "proof:    %s (%d/%d criteria met)\n", verdict, met, len(criteria))
		for _, raw := range criteria {
			c := mapAny(raw)
			mark := "✗"
			if ok, _ := c["met"].(bool); ok {
				mark = "✓"
			}
			line := fmt.Sprintf("  %s %s", mark, str(c["text"]))
			if note := str(c["note"]); note != "" {
				line += " — " + note
			}
			fmt.Fprintln(w, line)
		}
		if pf := mapAny(task["proof"]); len(pf) > 0 {
			if v := mapAny(pf["verdict"]); len(v) > 0 {
				if gap := str(v["gap"]); gap != "" {
					fmt.Fprintf(w, "  gap: %s\n", gap)
				}
			}
			if ev := mapAny(pf["evidence"]); len(ev) > 0 {
				arts, _ := ev["artifacts"].([]any)
				fmt.Fprintf(w, "  evidence: %d artifact(s), journal #%d–#%d\n",
					len(arts), intNumber(ev["journal_from"]), intNumber(ev["journal_to"]))
			}
		}
	}
	if comments, _ := task["comments"].([]any); len(comments) > 0 {
		fmt.Fprintln(w, "comments:")
		for _, raw := range comments {
			c := mapAny(raw)
			fmt.Fprintf(w, "  - %s: %s\n", str(c["author"]), str(c["body"]))
		}
	}
	if deps, _ := task["dependencies"].([]any); len(deps) > 0 {
		fmt.Fprintln(w, "depends:")
		for _, raw := range deps {
			d := mapAny(raw)
			fmt.Fprintf(w, "  - %s\n", str(d["id"]))
		}
	}
	if links, _ := task["links"].([]any); len(links) > 0 {
		fmt.Fprintln(w, "links:")
		for _, raw := range links {
			l := mapAny(raw)
			fmt.Fprintf(w, "  - %s %s\n", str(l["type"]), str(l["target"]))
		}
	}
}

func renderWorkboardWatch(w io.Writer, res map[string]any) {
	task := mapAny(res["task"])
	renderWorkboardTask(w, task)
	if runID := str(res["run_id"]); runID != "" {
		fmt.Fprintf(w, "run:      %s\n", runID)
	}
	events, _ := res["events"].([]any)
	if len(events) == 0 {
		fmt.Fprintln(w, "events:   none")
	} else {
		fmt.Fprintln(w, "events:")
		for _, raw := range events {
			e := mapAny(raw)
			payload := mapAny(e["payload"])
			detail := firstNonEmptyCLI(str(payload["phase"]), str(payload["action"]), str(payload["status"]), str(payload["error"]))
			if detail != "" {
				detail = " " + detail
			}
			fmt.Fprintf(w, "  #%d %s corr=%s%s\n", intNumber(e["seq"]), str(e["kind"]), shortID(str(e["correlation_id"])), detail)
		}
	}
	if blocked, _ := res["blocked_dependencies"].([]any); len(blocked) > 0 {
		fmt.Fprintln(w, "blocked dependencies:")
		for _, raw := range blocked {
			d := mapAny(raw)
			title := str(d["title"])
			if title != "" {
				title = " " + title
			}
			fmt.Fprintf(w, "  - %s%s (%s)\n", str(d["id"]), title, str(d["status"]))
		}
	}
}

func workboardWatchTerminal(res map[string]any) bool {
	task := mapAny(res["task"])
	switch str(task["status"]) {
	case "blocked", "review", "done", "archived":
		return true
	default:
		return false
	}
}

func firstNonEmptyCLI(items ...string) string { return strutil.FirstNonEmpty(items...) }

func mapAny(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func truthy(v any) bool {
	b, _ := v.(bool)
	return b
}
