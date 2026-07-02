// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdSeat(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "add", "create":
			return cmdSeatAdd(args[1:], stdout, stderr)
		case "remove", "rm", "delete":
			return cmdSeatRemove(args[1:], stdout, stderr)
		case "list", "ls", "-h", "--help", "help", "--json":
			// fall through to the listing
		default:
			fmt.Fprintf(stderr, "%s seats: unexpected arg %q\n", brand.CLI, args[0])
			fmt.Fprintf(stderr, "usage: %s seats [list] [--json] | seats add <id> [--exec P] [--name N] [--desc D] [--tool X] | seats remove <id>\n", brand.CLI)
			return 2
		}
	}
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help", "help", "list", "ls":
			// list is the only action; help falls through to the listing
		default:
			fmt.Fprintf(stderr, "%s seats: unexpected arg %q\n", brand.CLI, a)
			fmt.Fprintf(stderr, "usage: %s seats [list] [--json] | seats add <id> ... | seats remove <id>\n", brand.CLI)
			return 2
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdSeatList, map[string]any{})
	if err != nil {
		fmt.Fprintf(stderr, "%s seats: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	seats, _ := res["seats"].([]any)
	if len(seats) == 0 {
		fmt.Fprintln(stdout, "no seats")
		return 0
	}
	for _, raw := range seats {
		st := mapAny(raw)
		iso := str(st["execution_profile"])
		if iso == "" {
			iso = "-"
		}
		kind := "custom "
		if b, _ := st["builtin"].(bool); b {
			kind = "builtin"
		}
		fmt.Fprintf(stdout, "%-12s %s iso=%-10s %s\n", str(st["id"]), kind, iso, str(st["description"]))
	}
	return 0
}

func cmdSeatAdd(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	callArgs := map[string]any{}
	var tools []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--exec", "--execution-profile", "--name", "--desc", "--description", "--tool":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s seats add: %s needs a value\n", brand.CLI, a)
				return 2
			}
			i++
			switch a {
			case "--exec", "--execution-profile":
				callArgs["execution_profile"] = args[i]
			case "--name":
				callArgs["name"] = args[i]
			case "--desc", "--description":
				callArgs["description"] = args[i]
			case "--tool":
				tools = append(tools, args[i])
			}
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s seats add: unexpected flag %q\n", brand.CLI, a)
				return 2
			}
			if _, ok := callArgs["id"]; !ok {
				callArgs["id"] = a
			}
		}
	}
	if len(tools) > 0 {
		callArgs["tools"] = tools
	}
	if str(callArgs["id"]) == "" {
		fmt.Fprintf(stderr, "usage: %s seats add <id> [--exec local|warden|container] [--name N] [--desc D] [--tool X ...]\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdSeatCreate, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s seats add: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	fmt.Fprintf(stdout, "added seat %s\n", str(mapAny(res["seat"])["id"]))
	return 0
}

func cmdSeatRemove(args []string, stdout, stderr io.Writer) int {
	id, asJSON := "", false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else if !strings.HasPrefix(a, "-") && id == "" {
			id = a
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "usage: %s seats remove <id>\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdSeatDelete, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s seats remove: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	fmt.Fprintf(stdout, "removed seat %s\n", str(res["deleted"]))
	return 0
}
