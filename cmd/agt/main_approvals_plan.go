// SPDX-License-Identifier: MIT
//
// cmd/agt approvals + decide sub-commands (cmdApprovals, cmdDecide).
// Extracted from main_approvals_plan.go during Day 211 god-file refactor (#42, #67).
// Public API unchanged.
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

func cmdApprovals(args []string, stdout, stderr io.Writer) int {
	// `agt approvals log` — the resolved/pending audit timeline (M87); plain
	// `agt approvals` stays the pending-only list.
	if len(args) > 0 && args[0] == "log" {
		return cmdApprovalsLog(args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "stats" {
		return cmdApprovalsStats(args[1:], stdout, stderr)
	}
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s approvals [log] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "list pending HITL approval requests (or `approvals log` for the resolved audit)\n")
			fmt.Fprintf(stdout, "  --json   emit the full pending array (CI/automation pipelines)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s approvals: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdApprovals, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s approvals: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		// Pass the full response shape through — exits 0 even when
		// pending is empty (the empty array is the correct, valid
		// machine answer; jq pipelines should not need to special-
		// case "no approvals" via stderr scraping).
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	pending, _ := res["pending"].([]any)
	if len(pending) == 0 {
		fmt.Fprintf(stdout, "no pending approvals\n")
		return 0
	}
	fmt.Fprintf(stdout, "%d pending approval(s):\n", len(pending))
	for _, raw := range pending {
		m, _ := raw.(map[string]any)
		fmt.Fprintf(stdout, "\n  id         : %v\n", m["id"])
		fmt.Fprintf(stdout, "  capability : %v\n", m["capability"])
		fmt.Fprintf(stdout, "  tool       : %v\n", m["tool_name"])
		fmt.Fprintf(stdout, "  reason     : %v\n", m["reason"])
		if v := strings.TrimSpace(fmt.Sprint(m["canonical_intent"])); v != "" && v != "<nil>" {
			fmt.Fprintf(stdout, "  intent     : %v\n", v)
		}
		if v := strings.TrimSpace(fmt.Sprint(m["harmful_interpretation"])); v != "" && v != "<nil>" {
			fmt.Fprintf(stdout, "  harmful    : %v\n", v)
		}
		if v := strings.TrimSpace(fmt.Sprint(m["confirmation_prompt"])); v != "" && v != "<nil>" {
			fmt.Fprintf(stdout, "  confirm    : %v\n", v)
		}
		if v := strings.TrimSpace(fmt.Sprint(m["effect_class"])); v != "" && v != "<nil>" {
			fmt.Fprintf(stdout, "  effect     : %v\n", v)
		}
		if resources := toStringSlice(m["affected_resources"]); len(resources) > 0 {
			fmt.Fprintf(stdout, "  resources  : %s\n", strings.Join(resources, ", "))
		}
		if axes, ok := m["regret_axes"].(map[string]any); ok && len(axes) > 0 {
			fmt.Fprintf(stdout, "  regret     : physical=%v informational=%v social=%v identity=%v\n",
				axes["physical"], axes["informational"], axes["social"], axes["identity"])
		}
		fmt.Fprintf(stdout, "  actor      : %v\n", m["actor"])
		fmt.Fprintf(stdout, "  input      : %v\n", m["input"])
		fmt.Fprintf(stdout, "  timeout    : unix %v\n", m["timeout_unix"])
	}
	fmt.Fprintf(stdout, "\nResolve with: %s approve <id> [reason]  |  %s deny <id> [reason]\n", brand.CLI, brand.CLI)
	return 0
}
func cmdDecide(decision string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s %s: id required\n", brand.CLI, decision)
		return 2
	}
	id := args[0]
	reason := strings.Join(args[1:], " ")
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdDecide, map[string]any{
		"id": id, "decision": decision, "reason": reason,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s %s: %v\n", brand.CLI, decision, err)
		return 1
	}
	enc, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintf(stdout, "%s\n", enc)
	return 0
}
