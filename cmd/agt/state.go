// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdState dispatches `agt state <subcommand>`. State subsystem
// visibility — agents and scheduler write here, operators
// previously had no read path other than shelling into the data
// dir. Subcommands: list (namespaces or keys), get (single value).
func cmdState(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s state: subcommand required (list|get)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "list":
		return cmdStateList(args[1:], stdout, stderr)
	case "get":
		return cmdStateGet(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s state <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  list [<namespace>] [--json]   enumerate namespaces, or keys within one\n")
		fmt.Fprintf(stdout, "  get <namespace> <key> [--json]   read a single value (raw JSON)\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s state: unknown subcommand %q (list|get)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdStateList implements `agt state list [<namespace>] [--json]`.
// No namespace → list all namespaces. Namespace given → list keys
// in it.
func cmdStateList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var ns string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s state list [<namespace>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "without arg: list all namespaces; with arg: list keys in that namespace\n")
			return 0
		default:
			if ns == "" {
				ns = a
				continue
			}
			fmt.Fprintf(stderr, "%s state list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdStateList, map[string]any{"namespace": ns})
	if err != nil {
		fmt.Fprintf(stderr, "%s state list: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	if ns == "" {
		rows, _ := res["namespaces"].([]any)
		if len(rows) == 0 {
			fmt.Fprintln(stdout, "no namespaces (state store is empty)")
			return 0
		}
		fmt.Fprintf(stdout, "%d namespace(s):\n", len(rows))
		for _, raw := range rows {
			if s, ok := raw.(string); ok {
				fmt.Fprintf(stdout, "  %s\n", s)
			}
		}
		return 0
	}
	rows, _ := res["keys"].([]any)
	if len(rows) == 0 {
		fmt.Fprintf(stdout, "namespace %q has no keys\n", ns)
		return 0
	}
	fmt.Fprintf(stdout, "%d key(s) in %q:\n", len(rows), ns)
	for _, raw := range rows {
		if s, ok := raw.(string); ok {
			fmt.Fprintf(stdout, "  %s\n", s)
		}
	}
	return 0
}

// cmdStateGet implements `agt state get <namespace> <key> [--json]`.
// Exit 0 if key found, exit 3 if absent (distinct from exit 1 for
// daemon/network errors — lets CI scripts test for key presence
// without parsing output).
func cmdStateGet(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var ns, key string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s state get <namespace> <key> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "exit 0 = found, 3 = absent, 1 = error\n")
			return 0
		default:
			if ns == "" {
				ns = a
				continue
			}
			if key == "" {
				key = a
				continue
			}
			fmt.Fprintf(stderr, "%s state get: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if ns == "" || key == "" {
		fmt.Fprintf(stderr, "%s state get: namespace and key required\n", brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdStateGet, map[string]any{"namespace": ns, "key": key})
	if err != nil {
		fmt.Fprintf(stderr, "%s state get: %v\n", brand.CLI, err)
		return 1
	}

	found, _ := res["found"].(bool)
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		if !found {
			return 3
		}
		return 0
	}

	if !found {
		fmt.Fprintf(stderr, "%s state get: %s/%s not found\n", brand.CLI, ns, key)
		return 3
	}
	// Pretty-print just the value (operators usually want the data,
	// not the wrapping metadata). For raw protocol shape, use --json.
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res["value"])
	return 0
}
