// SPDX-License-Identifier: MIT
//
// cmd/agt agent wake/repair async-action handlers + payload helpers
// (cmdAgentWake, cmdAgentRepair, cmdAgentRepairStatus, callAgentAsyncAction,
// buildAgentWakePayload, buildAgentRepairPayload, applyAgentActionFlags).
// Extracted from agent_crud.go during Day 211 god-file refactor (#64).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)

func cmdAgentWake(args []string, stdout, stderr io.Writer) int {
	payload, ok := buildAgentWakePayload(args, stderr)
	if !ok {
		return 2
	}
	return callAgentAsyncAction(controlplane.CmdAgentWake, "wake", payload, stdout, stderr)
}
func cmdAgentRepair(args []string, stdout, stderr io.Writer) int {
	payload, ok := buildAgentRepairPayload(args, stderr)
	if !ok {
		return 2
	}
	return callAgentAsyncAction(controlplane.CmdAgentRepair, "repair", payload, stdout, stderr)
}
func cmdAgentRepairStatus(args []string, stdout, stderr io.Writer) int {
	ref := ""
	limit := 20
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--limit":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s agent repair-status: --limit needs a value\n", brand.CLI)
				return 2
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "%s agent repair-status: invalid --limit %q\n", brand.CLI, args[i])
				return 2
			}
			limit = n
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s agent repair-status <slug|id> [--limit N]\n", brand.CLI)
			return 0
		default:
			if strings.HasPrefix(args[i], "--") || ref != "" {
				fmt.Fprintf(stderr, "usage: %s agent repair-status <slug|id> [--limit N]\n", brand.CLI)
				return 2
			}
			ref = args[i]
		}
	}
	if ref == "" {
		fmt.Fprintf(stderr, "usage: %s agent repair-status <slug|id> [--limit N]\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentRepairStatus, map[string]any{"ref": ref, "limit": limit})
	if err != nil {
		fmt.Fprintf(stderr, "%s agent repair-status: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "agent %s repair: %v history, %v inflight", str(res["slug"]), res["count"], res["inflight_count"])
	if next := intNumber(res["next_eligible_ms"]); next > 0 {
		fmt.Fprintf(stdout, " next_eligible_ms=%d", next)
	}
	fmt.Fprintln(stdout)
	if latest, _ := res["latest"].(map[string]any); latest != nil {
		fmt.Fprintf(stdout, "latest: %s %s\n", str(latest["phase"]), str(latest["reason"]))
	}
	return 0
}
func callAgentAsyncAction(cmd string, label string, payload map[string]any, stdout, stderr io.Writer) int {
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent %s: %v\n", brand.CLI, label, err)
		return 1
	}
	fmt.Fprintf(stdout, "agent %s %s accepted", str(res["agent"]), label)
	if corr := str(res["correlation_id"]); corr != "" {
		fmt.Fprintf(stdout, " corr=%s", corr)
	}
	fmt.Fprintln(stdout)
	return 0
}
func buildAgentWakePayload(args []string, stderr io.Writer) (map[string]any, bool) {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: %s agent wake <slug|id> [intent text] [--reason TEXT] [--incident ID] [--root ID] [--parent ID]\n", brand.CLI)
		return nil, false
	}
	payload := map[string]any{"ref": args[0]}
	var intent []string
	if !applyAgentActionFlags(payload, args[1:], &intent, stderr, "wake") {
		return nil, false
	}
	if len(intent) > 0 {
		payload["intent"] = strings.Join(intent, " ")
	}
	return payload, true
}
func buildAgentRepairPayload(args []string, stderr io.Writer) (map[string]any, bool) {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: %s agent repair <slug|id> [--reason TEXT] [--incident ID] [--root ID] [--parent ID]\n", brand.CLI)
		return nil, false
	}
	payload := map[string]any{"ref": args[0]}
	var extras []string
	if !applyAgentActionFlags(payload, args[1:], &extras, stderr, "repair") {
		return nil, false
	}
	if len(extras) > 0 && str(payload["reason"]) == "" {
		payload["reason"] = strings.Join(extras, " ")
	}
	return payload, true
}
func applyAgentActionFlags(payload map[string]any, args []string, positional *[]string, stderr io.Writer, cmd string) bool {
	need := func(i int, flag string) bool {
		if i+1 >= len(args) {
			fmt.Fprintf(stderr, "%s agent %s: %s needs a value\n", brand.CLI, cmd, flag)
			return false
		}
		return true
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--reason":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["reason"] = args[i]
		case "--incident", "--incident-id":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["incident_id"] = args[i]
		case "--root", "--root-incident", "--root-incident-id":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["root_incident_id"] = args[i]
		case "--parent", "--parent-incident", "--parent-incident-id":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["parent_incident_id"] = args[i]
		default:
			if strings.HasPrefix(args[i], "--") {
				fmt.Fprintf(stderr, "%s agent %s: unknown flag %s\n", brand.CLI, cmd, args[i])
				return false
			}
			*positional = append(*positional, args[i])
		}
	}
	return true
}
