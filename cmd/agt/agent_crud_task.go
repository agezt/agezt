// SPDX-License-Identifier: MIT
//
// cmd/agt agent task sub-command + helpers (cmdAgentTask, buildAgentTaskPayload,
// applyTaskFlags, printAgentTaskUsage). Extracted from agent_crud.go during
// Day 211 god-file refactor (#64). Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)

func cmdAgentTask(args []string, stdout, stderr io.Writer) int {
	payload, label, ok := buildAgentTaskPayload(args, stderr)
	if !ok {
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdAgentTaskUpdate, payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s agent task: %v\n", brand.CLI, err)
		return 1
	}
	task, _ := res["task"].(map[string]any)
	if label == "" {
		label = str(task["status"])
	}
	fmt.Fprintf(stdout, "agent %s task %s", str(payload["ref"]), label)
	if id := str(task["id"]); id != "" {
		fmt.Fprintf(stdout, " %s", id)
	}
	if title := str(task["title"]); title != "" {
		fmt.Fprintf(stdout, " (%s)", title)
	}
	fmt.Fprintln(stdout)
	return 0
}
func buildAgentTaskPayload(args []string, stderr io.Writer) (map[string]any, string, bool) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printAgentTaskUsage(stderr)
		return nil, "", false
	}
	cmd := strings.ToLower(strings.TrimSpace(args[0]))
	switch cmd {
	case "add":
		if len(args) < 3 {
			printAgentTaskUsage(stderr)
			return nil, "", false
		}
		payload := map[string]any{"op": "add", "ref": args[1], "title": args[2], "scope": "total", "status": "todo"}
		if !applyTaskFlags(payload, args[3:], stderr) {
			return nil, "", false
		}
		return payload, "added", true
	case "set", "update", "edit":
		if len(args) < 3 {
			printAgentTaskUsage(stderr)
			return nil, "", false
		}
		payload := map[string]any{"op": "update", "ref": args[1], "id": args[2]}
		if !applyTaskFlags(payload, args[3:], stderr) {
			return nil, "", false
		}
		if len(payload) <= 3 {
			fmt.Fprintf(stderr, "%s agent task set: at least one field flag is required\n", brand.CLI)
			return nil, "", false
		}
		return payload, "updated", true
	case "remove", "delete", "rm":
		if len(args) != 3 {
			printAgentTaskUsage(stderr)
			return nil, "", false
		}
		return map[string]any{"op": "remove", "ref": args[1], "id": args[2]}, "removed", true
	case "todo", "doing", "done", "blocked", "retired":
		if len(args) != 3 {
			printAgentTaskUsage(stderr)
			return nil, "", false
		}
		return map[string]any{"op": "update", "ref": args[1], "id": args[2], "status": cmd}, cmd, true
	default:
		fmt.Fprintf(stderr, "%s agent task: unknown task command %q\n", brand.CLI, args[0])
		printAgentTaskUsage(stderr)
		return nil, "", false
	}
}
func applyTaskFlags(payload map[string]any, args []string, stderr io.Writer) bool {
	need := func(i int, flag string) bool {
		if i+1 >= len(args) {
			fmt.Fprintf(stderr, "%s agent task: %s needs a value\n", brand.CLI, flag)
			return false
		}
		return true
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--title":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["title"] = args[i]
		case "--desc", "--description":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["description"] = args[i]
		case "--scope":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["scope"] = args[i]
		case "--status":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["status"] = args[i]
		case "--id":
			if !need(i, args[i]) {
				return false
			}
			i++
			payload["id"] = args[i]
		default:
			fmt.Fprintf(stderr, "%s agent task: unknown flag %s\n", brand.CLI, args[i])
			return false
		}
	}
	return true
}
func printAgentTaskUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: %s agent task add <slug|id> <title> [--id ID] [--scope cycle|total] [--status todo|doing|done|blocked|retired] [--desc TEXT]\n", brand.CLI)
	fmt.Fprintf(w, "       %s agent task set <slug|id> <task-id> [--title TEXT] [--scope cycle|total] [--status ...] [--desc TEXT]\n", brand.CLI)
	fmt.Fprintf(w, "       %s agent task <done|doing|todo|blocked|retired|remove> <slug|id> <task-id>\n", brand.CLI)
}
