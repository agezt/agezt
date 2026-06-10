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

// cmdMCP dispatches `agt mcp <subcommand>` — the operator surface of MCP
// self-install (M796): register an MCP server, attach it at runtime (its
// tools become callable as mcp_<name>_<tool> without a restart), detach
// (kill switch), enable/disable auto-attach at daemon start, remove. Every
// mutation is journaled (mcp.*).
func cmdMCP(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return mcpUsage(stderr)
	}
	switch args[0] {
	case "list":
		return cmdMCPList(args[1:], stdout, stderr)
	case "add", "register":
		return cmdMCPAdd(args[1:], stdout, stderr)
	case "attach":
		return cmdMCPRefAction(args[1:], stdout, stderr, "attach")
	case "detach":
		return cmdMCPRefAction(args[1:], stdout, stderr, "detach")
	case "enable":
		return cmdMCPSetEnabled(args[1:], stdout, stderr, true)
	case "disable":
		return cmdMCPSetEnabled(args[1:], stdout, stderr, false)
	case "remove", "rm":
		return cmdMCPRefAction(args[1:], stdout, stderr, "remove")
	case "-h", "--help", "help":
		return mcpUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s mcp: unknown subcommand %q\n", brand.CLI, args[0])
		return mcpUsage(stderr)
	}
}

func mcpUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s mcp <list|add|attach|detach|enable|disable|remove>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--json]                              registrations + live attachment status\n")
	fmt.Fprintf(w, "  add <name> --cmd EXE [--arg A ...] [--desc TEXT]\n")
	fmt.Fprintf(w, "      register a stdio MCP server, e.g. %s mcp add everything --cmd npx --arg -y --arg @modelcontextprotocol/server-everything\n", brand.CLI)
	fmt.Fprintf(w, "  attach <name|id>                           spawn + handshake NOW; its tools become callable as mcp_<name>_<tool>\n")
	fmt.Fprintf(w, "  detach <name|id>                           stop it (kill switch); its tools vanish from the next run\n")
	fmt.Fprintf(w, "  enable|disable <name|id>                   auto-attach at daemon start on/off\n")
	fmt.Fprintf(w, "  remove <name|id>                           delete the registration (detaches first)\n")
	return 0
}

func cmdMCPList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMCPList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s mcp list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	servers, _ := res["servers"].([]any)
	if len(servers) == 0 {
		fmt.Fprintf(stdout, "no mcp servers yet — register one with `%s mcp add <name> --cmd npx --arg -y --arg <package>`\n", brand.CLI)
		return 0
	}
	for _, raw := range servers {
		srv, _ := raw.(map[string]any)
		if srv == nil {
			continue
		}
		state := "registered"
		if att, _ := srv["attached"].(bool); att {
			n, _ := srv["tool_count"].(float64)
			state = fmt.Sprintf("ATTACHED (%d tools)", int(n))
		}
		auto := ""
		if en, _ := srv["enabled"].(bool); en {
			auto = " auto-attach"
		}
		argv := str(srv["command"])
		if list, _ := srv["args"].([]any); len(list) > 0 {
			parts := make([]string, 0, len(list))
			for _, a := range list {
				parts = append(parts, str(a))
			}
			argv += " " + strings.Join(parts, " ")
		}
		fmt.Fprintf(stdout, "%-16s %-20s%s  %s\n", str(srv["name"]), state, auto, argv)
	}
	fmt.Fprintf(stdout, "%v server(s), %v attached\n", res["count"], res["attached_count"])
	return 0
}

func cmdMCPAdd(args []string, stdout, stderr io.Writer) int {
	name, command, desc := "", "", ""
	var srvArgs []string
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
		case "--arg":
			if !need() {
				return 2
			}
			i++
			srvArgs = append(srvArgs, args[i])
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
	if name == "" || command == "" {
		fmt.Fprintf(stderr, "usage: %s mcp add <name> --cmd EXE [--arg A ...] [--desc TEXT]\n", brand.CLI)
		return 2
	}
	server := map[string]any{"name": name, "command": command, "description": desc}
	if len(srvArgs) > 0 {
		list := make([]any, len(srvArgs))
		for i, a := range srvArgs {
			list[i] = a
		}
		server["args"] = list
	}
	c := dial(stderr)
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

// cmdMCPRefAction handles the three single-ref subcommands: attach (long
// timeout — npx may download the server on first run), detach, remove.
func cmdMCPRefAction(args []string, stdout, stderr io.Writer, action string) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s mcp %s <name|id>\n", brand.CLI, action)
		return 2
	}
	c := dial(stderr)
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
	c := dial(stderr)
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
