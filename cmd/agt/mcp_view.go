// SPDX-License-Identifier: MIT
//
// cmd/agt `mcp` read-only subcommands (mcpUsage, cmdMCPList).
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
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func mcpUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s mcp <list|add|attach|detach|enable|disable|remove>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--json]                              registrations + live attachment status\n")
	fmt.Fprintf(w, "  add <name> --cmd EXE [--arg A ...] [--desc TEXT]\n")
	fmt.Fprintf(w, "      register a stdio MCP server, e.g. %s mcp add everything --cmd npx --arg -y --arg @modelcontextprotocol/server-everything\n", brand.CLI)
	fmt.Fprintf(w, "  add <name> --url URL [--header \"K: V\" ...] [--desc TEXT]\n")
	fmt.Fprintf(w, "      register a remote (Streamable HTTP) MCP server, e.g. %s mcp add github --url https://api.example.com/mcp --header \"Authorization: Bearer ghp_...\"\n", brand.CLI)
	fmt.Fprintf(w, "      add --lazy to collapse a chatty server's tools into one mcp_<name> dispatcher (context-efficient)\n")
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
	c := dialpkg.New(stderr)
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
		return jsonout.Write(stdout, res)
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
		if lz, _ := srv["lazy"].(bool); lz {
			auto += " lazy"
		}
		argv := str(srv["command"])
		if u := str(srv["url"]); u != "" {
			argv = "http " + u // remote (Streamable HTTP) server, M904
		}
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
