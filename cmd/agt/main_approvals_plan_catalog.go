// SPDX-License-Identifier: MIT
//
// cmd/agt catalog sub-commands (cmdCatalog, cmdCatalogList).
// Extracted from main_approvals_plan.go during Day 211 god-file refactor (#67).
// Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)

func cmdCatalog(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s catalog: subcommand required: sync|list|discover\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "sync":
		return cmdCatalogSync(args[1:], stdout, stderr)
	case "discover":
		callArgs := map[string]any{}
		if len(args) > 1 {
			callArgs["endpoint"] = args[1]
		}
		return cmdSimple(controlplane.CmdCatalogDiscover, callArgs, stdout, stderr)
	case "list":
		return cmdCatalogList(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s catalog: unknown subcommand %q\n", brand.CLI, args[0])
		return 2
	}
}
func cmdCatalogList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s catalog list [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "list synced providers + models + pricing\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s catalog list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdCatalogList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s catalog list: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	providers, _ := res["providers"].([]any)
	syncedAt, _ := res["api_synced_at"].(string)
	source, _ := res["api_source_url"].(string)

	fmt.Fprintf(stdout, "%d providers (synced %s from %s)\n",
		len(providers), formatTime(syncedAt), source)

	for _, raw := range providers {
		p, _ := raw.(map[string]any)
		credentialed := p["credentialed"] == true
		credBadge := "[no creds]"
		if credentialed {
			credBadge = "[creds OK]"
		}
		fmt.Fprintf(stdout, "\n  %s  (%s, family=%s)  %s\n",
			p["id"], p["name"], p["family"], credBadge)
		if api, _ := p["api"].(string); api != "" {
			fmt.Fprintf(stdout, "    api  : %s\n", api)
		}
		if env, ok := p["env"].([]any); ok && len(env) > 0 {
			fmt.Fprintf(stdout, "    env  : ")
			for i, e := range env {
				if i > 0 {
					fmt.Fprint(stdout, ", ")
				}
				fmt.Fprint(stdout, e)
			}
			fmt.Fprintln(stdout)
		}
		models, _ := p["models"].([]any)
		if len(models) == 0 {
			fmt.Fprintf(stdout, "    (no models)\n")
			continue
		}
		fmt.Fprintf(stdout, "    %d model(s):\n", len(models))
		for _, mraw := range models {
			m, _ := mraw.(map[string]any)
			cost := "free"
			if in, ok := m["cost_input_usd_per_mtok"].(float64); ok {
				out, _ := m["cost_output_usd_per_mtok"].(float64)
				cost = fmt.Sprintf("$%.2f / $%.2f per MTok", in, out)
			}
			fmt.Fprintf(stdout, "      %-40s  %s\n", m["id"], cost)
		}
	}
	return 0
}
