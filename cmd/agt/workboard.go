// SPDX-License-Identifier: MIT
//
// cmd/agt workboard command: the entry dispatcher + the read-only
// sub-commands (List + Lanes + Show).
// The mutation sub-commands live in workboard_mutate.go; the
// parse / render / dispatch helpers live in workboard_render.go.
// Extracted from workboard.go during the Day-209 god-file split.
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strconv"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/cmd/agt/jsonout"
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
		return jsonout.Write(stdout, res)
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
		return jsonout.Write(stdout, res)
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
		return jsonout.Write(stdout, task)
	}
	renderWorkboardTask(stdout, task)
	return 0
}
