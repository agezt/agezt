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
	case "stats":
		return cmdTenantStats(args[1:], stdout, stderr)
	case "token":
		return cmdTenantByID(args[1:], stdout, stderr, "token", controlplane.CmdTenantToken)
	case "release", "close":
		return cmdTenantByID(args[1:], stdout, stderr, "release", controlplane.CmdTenantRelease)
	case "rm", "remove", "delete":
		return cmdTenantByID(args[1:], stdout, stderr, "rm", controlplane.CmdTenantRemove)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s tenant <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  create <id> [--json]    create / open an isolated tenant (prints its token)\n")
		fmt.Fprintf(stdout, "  list [--json]           list tenants (id, open state, base dir)\n")
		fmt.Fprintf(stdout, "  stats [--json]          per-tenant run count / spend / last activity (primary token)\n")
		fmt.Fprintf(stdout, "  token <id> [--json]     reveal a tenant's per-tenant credential\n")
		fmt.Fprintf(stdout, "  release <id> [--json]   close a tenant's kernel, keep its state on disk\n")
		fmt.Fprintf(stdout, "  rm <id> [--json]        delete a tenant and ALL its state (destructive)\n")
		fmt.Fprintf(stdout, "  <id> is [a-z0-9_-], 1-64 chars. Requires the daemon started with %sMULTITENANT=on.\n", brand.EnvPrefix)
		fmt.Fprintf(stdout, "  Route to a tenant: HTTP `X-Agezt-Tenant: <id>` + its token, or `%s run --tenant <id>`.\n", brand.CLI)
		return 0
	default:
		fmt.Fprintf(stderr, "%s tenant: unknown subcommand %q (create|list|token|release|rm)\n", brand.CLI, args[0])
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

// cmdTenantStats implements `agt tenant stats [--json]` (M126) — the cross-tenant
// usage view: per-tenant run count / completed / failed / active / spend / last
// activity, plus grand totals, so the primary operator can see which tenant is
// busy, spending, or failing. Requires the primary token (a tenant sees only its
// own runs via `agt runs stats --tenant <id>`).
func cmdTenantStats(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s tenant stats [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "per-tenant run count, spend, and last activity across all tenants (primary token)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s tenant stats: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdTenantStats, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s tenant stats: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	rows, _ := res["tenants"].([]any)
	if len(rows) == 0 {
		fmt.Fprintf(stdout, "no tenants. Create one with `%s tenant create <id>`.\n", brand.CLI)
		return 0
	}
	for _, raw := range rows {
		m, _ := raw.(map[string]any)
		id, _ := m["id"].(string)
		if errMsg, _ := m["error"].(string); errMsg != "" {
			fmt.Fprintf(stdout, "  %-24s [error: %s]\n", id, errMsg)
			continue
		}
		runs := intOfStatus(m["runs"])
		completed := intOfStatus(m["completed"])
		failed := intOfStatus(m["failed"])
		active := intOfStatus(m["active"])
		spent := mcFromAny(m["spent_microcents"])
		last := "—"
		if ms := intOfStatus(m["last_activity_unix_ms"]); ms > 0 {
			last = time.UnixMilli(int64(ms)).Format("2006-01-02 15:04")
		}
		line := fmt.Sprintf("  %-24s %d run(s)  (%d ok, %d failed, %d active)  last: %s",
			id, runs, completed, failed, active, last)
		if spent > 0 {
			line += "  spend: " + fmtUSD(spent)
		}
		fmt.Fprintln(stdout, line)
	}
	totalRuns := intOfStatus(res["total_runs"])
	totalSpent := mcFromAny(res["total_spent_microcents"])
	footer := fmt.Sprintf("\n%d tenant(s), %d run(s) total", len(rows), totalRuns)
	if totalSpent > 0 {
		footer += ", " + fmtUSD(totalSpent) + " spent"
	}
	fmt.Fprintln(stdout, footer)
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
		token, _ := res["token"].(string)
		if created, _ := res["created"].(bool); created {
			fmt.Fprintf(stdout, "created %s (%s)\n", id, baseDir)
		} else {
			fmt.Fprintf(stdout, "opened %s (already existed; %s)\n", id, baseDir)
		}
		if token != "" {
			fmt.Fprintf(stdout, "  token: %s\n", token)
			fmt.Fprintf(stdout, "  use:   HTTP header `X-Agezt-Tenant: %s` with `Authorization: Bearer %s`\n", id, token)
		}
	case controlplane.CmdTenantToken:
		token, _ := res["token"].(string)
		fmt.Fprintf(stdout, "%s\n", token)
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
