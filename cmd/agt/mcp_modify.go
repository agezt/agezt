// SPDX-License-Identifier: MIT
//
// cmd/agt `mcp` mutation subcommands (cmdMCPAdd, cmdMCPRefAction, cmdMCPSetEnabled).
// Extracted from mcp.go during Day 211 god-file refactor (#77).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdMCPAdd(args []string, stdout, stderr io.Writer) int {
	name, command, desc, url := "", "", "", ""
	lazy := false
	var srvArgs []string
	headers := map[string]any{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		need := func() bool {
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s mcp add: %s needs a value\n", brand.CLI, a)
				return false
			}
			return true
		}
		switch a {
		case "--cmd", "--command":
			if !need() {
				return 2
			}
			i++
			command = args[i]
		case "--url":
			if !need() {
				return 2
			}
			i++
			url = args[i]
		case "--arg":
			if !need() {
				return 2
			}
			i++
			srvArgs = append(srvArgs, args[i])
		case "--header":
			// --header "Authorization: Bearer ..." (remote servers, M904).
			if !need() {
				return 2
			}
			i++
			k, v, ok := strings.Cut(args[i], ":")
			if !ok || strings.TrimSpace(k) == "" {
				fmt.Fprintf(stderr, "%s mcp add: --header wants \"Name: value\"\n", brand.CLI)
				return 2
			}
			headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
		case "--lazy":
			// Collapse this server's tools into one mcp_<name> dispatcher (M906).
			lazy = true
		case "--desc", "--description":
			if !need() {
				return 2
			}
			i++
			desc = args[i]
		default:
			if !strings.HasPrefix(a, "--") && name == "" {
				name = a
			}
		}
	}
	if name == "" || (command == "" && url == "") {
		fmt.Fprintf(stderr, "usage: %s mcp add <name> (--cmd EXE [--arg A ...] | --url URL [--header \"K: V\" ...]) [--desc TEXT]\n", brand.CLI)
		return 2
	}
	server := map[string]any{"name": name, "description": desc}
	if lazy {
		server["lazy"] = true
	}
	if url != "" {
		server["url"] = url
		if len(headers) > 0 {
			server["headers"] = headers
		}
	} else {
		server["command"] = command
	}
	if len(srvArgs) > 0 {
		list := make([]any, len(srvArgs))
		for i, a := range srvArgs {
			list[i] = a
		}
		server["args"] = list
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Call(ctx, controlplane.CmdMCPAdd, map[string]any{"server": server}); err != nil {
		fmt.Fprintf(stderr, "%s mcp add: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "registered %s — attach it now with `%s mcp attach %s` (auto-attaches on daemon start)\n", name, brand.CLI, name)
	return 0
}
func cmdMCPRefAction(args []string, stdout, stderr io.Writer, action string) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s mcp %s <name|id>\n", brand.CLI, action)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	timeout := 5 * time.Second
	cmd := controlplane.CmdMCPDetach
	switch action {
	case "attach":
		cmd, timeout = controlplane.CmdMCPAttach, 2*time.Minute
	case "remove":
		cmd = controlplane.CmdMCPRemove
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	res, err := c.Call(ctx, cmd, map[string]any{"ref": args[0]})
	if err != nil {
		fmt.Fprintf(stderr, "%s mcp %s: %v\n", brand.CLI, action, err)
		return 1
	}
	switch action {
	case "attach":
		tools, _ := res["tools"].([]any)
		fmt.Fprintf(stdout, "attached %s — %d tool(s) now callable:\n", args[0], len(tools))
		for _, t := range tools {
			fmt.Fprintf(stdout, "  %s\n", str(t))
		}
	case "detach":
		fmt.Fprintf(stdout, "detached %s — its tools are gone from the next run\n", args[0])
	case "remove":
		if ok, _ := res["removed"].(bool); !ok {
			fmt.Fprintf(stderr, "%s mcp remove: unknown server %q\n", brand.CLI, args[0])
			return 1
		}
		fmt.Fprintf(stdout, "removed %s\n", args[0])
	}
	return 0
}
func cmdMCPSetEnabled(args []string, stdout, stderr io.Writer, enabled bool) int {
	verb := "enable"
	if !enabled {
		verb = "disable"
	}
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s mcp %s <name|id>\n", brand.CLI, verb)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Call(ctx, controlplane.CmdMCPSetEnabled, map[string]any{"ref": args[0], "enabled": enabled}); err != nil {
		fmt.Fprintf(stderr, "%s mcp %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	state := "will auto-attach on daemon start"
	if !enabled {
		state = "will NOT auto-attach on daemon start"
	}
	fmt.Fprintf(stdout, "%s %s\n", args[0], state)
	return 0
}
