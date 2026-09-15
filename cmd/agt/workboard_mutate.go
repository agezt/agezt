// SPDX-License-Identifier: MIT
//
// cmd/agt workboard mutation sub-commands (create/claim/heartbeat/comment/
// block/fail/seat/actor/link). Split from workboard_mutate.go during
// Day 211 god-file refactor (#30).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

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
