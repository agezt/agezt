// SPDX-License-Identifier: MIT
//
// cmd/agt `workboard` lifecycle subcommands (claim, heartbeat, comment, block, fail).
// Extracted from workboard_mutate.go during Day 211 god-file refactor (#96).
// Public API unchanged.
package main

import (
	"fmt"
	"io"

	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

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
