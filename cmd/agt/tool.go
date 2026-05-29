// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/ersinkoc/agezt/internal/brand"
	"github.com/ersinkoc/agezt/kernel/controlplane"
)

// cmdTool dispatches `agt tool <subcommand>`. Currently the only
// subcommand is `list`; left as a dispatcher (vs flattening into
// `agt tool-list`) so future `agt tool invoke <name>` /
// `agt tool describe <name>` slot in without renaming.
func cmdTool(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s tool: subcommand required (list)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "list":
		return cmdToolList(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s tool: unknown subcommand %q\n", brand.CLI, args[0])
		return 2
	}
}

// cmdToolList implements `agt tool list` and `agt tool list --json`.
// Shows the in-process tools the daemon will advertise to the model.
// First place to look when a model isn't calling a tool the operator
// expected — confirms whether the tool is even registered before
// chasing prompt/schema bugs.
func cmdToolList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s tool list [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "list the in-process tools the daemon advertises to the model\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s tool list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdToolList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s tool list: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	rows, _ := res["tools"].([]any)
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "no tools registered")
		return 0
	}
	fmt.Fprintf(stdout, "%d tool(s):\n", len(rows))
	for _, raw := range rows {
		r, _ := raw.(map[string]any)
		name, _ := r["name"].(string)
		desc, _ := r["description"].(string)
		if desc == "" {
			fmt.Fprintf(stdout, "  %s\n", name)
		} else {
			fmt.Fprintf(stdout, "  %-20s %s\n", name, desc)
		}
	}
	return 0
}
