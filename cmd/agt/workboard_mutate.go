// SPDX-License-Identifier: MIT
//
// cmd/agt `workboard create` subcommand (cmdWorkboardCreate).
// Extracted from workboard_mutate.go during Day 211 god-file refactor (#96).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
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
