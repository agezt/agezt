// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdTenant dispatches `agt tenant <subcommand>` — the operator's management
// path into the daemon's multi-tenant registry (ROADMAP P6-MULTI). Each tenant
// is fully isolated: its own journal, state, vault, memory, and schedules under
// its own base dir. Requires the daemon started with AGEZT_MULTITENANT=on.
func cmdTenant(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s tenant: subcommand required (create|list|release|rm)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "create", "add":
		return cmdTenantByID(args[1:], stdout, stderr, "create", controlplane.CmdTenantCreate)
	case "list", "ls":
		return cmdTenantList(args[1:], stdout, stderr)
	case "release", "close":
		return cmdTenantByID(args[1:], stdout, stderr, "release", controlplane.CmdTenantRelease)
	case "rm", "remove", "delete":
		return cmdTenantByID(args[1:], stdout, stderr, "rm", controlplane.CmdTenantRemove)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s tenant <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  create <id> [--json]    create / open an isolated tenant\n")
		fmt.Fprintf(stdout, "  list [--json]           list tenants (id, open state, base dir)\n")
		fmt.Fprintf(stdout, "  release <id> [--json]   close a tenant's kernel, keep its state on disk\n")
		fmt.Fprintf(stdout, "  rm <id> [--json]        delete a tenant and ALL its state (destructive)\n")
		fmt.Fprintf(stdout, "  <id> is [a-z0-9_-], 1-64 chars. Requires the daemon started with %sMULTITENANT=on.\n", brand.EnvPrefix)
		return 0
	default:
		fmt.Fprintf(stderr, "%s tenant: unknown subcommand %q (create|list|release|rm)\n", brand.CLI, args[0])
		return 2
	}
}

func cmdTenantList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s tenant list [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s tenant list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdTenantList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s tenant list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	list, _ := res["tenants"].([]any)
	if len(list) == 0 {
		fmt.Fprintf(stdout, "no tenants. Create one with `%s tenant create <id>`.\n", brand.CLI)
		return 0
	}
	for _, item := range list {
		m, _ := item.(map[string]any)
		id, _ := m["id"].(string)
		baseDir, _ := m["base_dir"].(string)
		open, _ := m["open"].(bool)
		state := "closed"
		if open {
			state = "open"
		}
		fmt.Fprintf(stdout, "  %-24s [%s]  %s\n", id, state, baseDir)
	}
	return 0
}

// cmdTenantByID factors create/release/rm: each takes a single id and reports a
// boolean-ish result. The reporting differs per verb, so it switches on cmd.
func cmdTenantByID(args []string, stdout, stderr io.Writer, verb, cmd string) int {
	asJSON := false
	var id string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s tenant %s <id> [--json]\n", brand.CLI, verb)
			return 0
		default:
			if id == "" {
				id = a
			}
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s tenant %s: an id is required\n", brand.CLI, verb)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s tenant %s: %v\n", brand.CLI, verb, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	switch cmd {
	case controlplane.CmdTenantCreate:
		baseDir, _ := res["base_dir"].(string)
		if created, _ := res["created"].(bool); created {
			fmt.Fprintf(stdout, "created %s (%s)\n", id, baseDir)
		} else {
			fmt.Fprintf(stdout, "opened %s (already existed; %s)\n", id, baseDir)
		}
	case controlplane.CmdTenantRelease:
		if released, _ := res["released"].(bool); !released {
			fmt.Fprintf(stderr, "%s tenant release: %s was not open\n", brand.CLI, id)
			return 3
		}
		fmt.Fprintf(stdout, "%s released (state kept on disk)\n", id)
	case controlplane.CmdTenantRemove:
		if removed, _ := res["removed"].(bool); !removed {
			fmt.Fprintf(stderr, "%s tenant rm: %s not found\n", brand.CLI, id)
			return 3
		}
		fmt.Fprintf(stdout, "%s removed (all state deleted)\n", id)
	}
	return 0
}
