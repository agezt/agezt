// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)


func cmdConfigCenter(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s configcenter: subcommand required (set, get, list, delete, rating, access-log, audit, health)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "set":
		return cmdConfigCenterSet(args[1:], stdout, stderr)
	case "get":
		return cmdConfigCenterGet(args[1:], stdout, stderr)
	case "list":
		return cmdConfigCenterList(args[1:], stdout, stderr)
	case "delete":
		return cmdConfigCenterDelete(args[1:], stdout, stderr)
	case "rating":
		return cmdConfigCenterRating(args[1:], stdout, stderr)
	case "access-log":
		return cmdConfigCenterAccessLog(args[1:], stdout, stderr)
	case "audit":
		return cmdConfigCenterAudit(args[1:], stdout, stderr)
	case "health":
		return cmdConfigCenterHealth(args[1:], stdout, stderr)
	case "-h", "--help":
		return cmdConfigCenterHelp(stdout)
	default:
		fmt.Fprintf(stderr, "%s configcenter: unknown subcommand %q\n", brand.CLI, args[0])
		return 2
	}
}

func cmdConfigCenterHelp(stdout io.Writer) int {
	fmt.Fprintf(stdout, "usage: %s configcenter <subcommand>\n\n", brand.CLI)
	fmt.Fprintf(stdout, "  set <key> <value> [--rating <rating>] [--description <desc>] [--allow-agent csv] [--deny-agent csv]\n")
	fmt.Fprintf(stdout, "                            Set a config value (auto-rates secret patterns)\n")
	fmt.Fprintf(stdout, "  get <key>                 Get a config value (admin only)\n")
	fmt.Fprintf(stdout, "  list [--rating <rating>] [--json]\n")
	fmt.Fprintf(stdout, "                            List all config entries\n")
	fmt.Fprintf(stdout, "  delete <key>              Delete a config entry\n")
	fmt.Fprintf(stdout, "  rating <key> [--rating <rating>]\n")
	fmt.Fprintf(stdout, "                            Get or set rating for a key\n")
	fmt.Fprintf(stdout, "  access-log [--key <key>] [--agent <agent>] [--since <duration>] [--json]\n")
	fmt.Fprintf(stdout, "                            View config access history\n")
	fmt.Fprintf(stdout, "  audit [--since <duration>] [--json]\n")
	fmt.Fprintf(stdout, "                            View audit log\n")
	fmt.Fprintf(stdout, "  health                    Check Config Center health\n")
	fmt.Fprintf(stdout, "\nRatings: public, internal, restricted, secret\n")
	return 0
}

func cmdConfigCenterGet(args []string, stdout, stderr io.Writer) int {
	var key string
	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s configcenter get <key>\n", brand.CLI)
			return 0
		default:
			if key != "" {
				fmt.Fprintf(stderr, "%s configcenter get: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
			key = a
		}
	}

	if key == "" {
		fmt.Fprintf(stderr, "%s configcenter get: key required\n", brand.CLI)
		return 2
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.Call(ctx, controlplane.CmdConfigCenterGet, map[string]any{"key": key})
	if err != nil {
		fmt.Fprintf(stderr, "%s configcenter get: %v\n", brand.CLI, err)
		return 1
	}

	entry, _ := res["entry"].(map[string]any)
	if entry == nil {
		fmt.Fprintf(stderr, "%s configcenter get: key %q not found\n", brand.CLI, key)
		return 1
	}

	fmt.Fprintf(stdout, "key:     %s\n", entry["key"])
	fmt.Fprintf(stdout, "value:   %s\n", entry["value"])
	fmt.Fprintf(stdout, "rating:  %s\n", entry["rating"])
	if desc, _ := entry["description"].(string); desc != "" {
		fmt.Fprintf(stdout, "desc:    %s\n", desc)
	}
	fmt.Fprintf(stdout, "updated: %s\n", time.UnixMilli(int64(entry["updated_at"].(float64))).Format(time.RFC3339))

	return 0
}

func cmdConfigCenterDelete(args []string, stdout, stderr io.Writer) int {
	var key string
	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s configcenter delete <key>\n", brand.CLI)
			return 0
		default:
			if key != "" {
				fmt.Fprintf(stderr, "%s configcenter delete: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
			key = a
		}
	}

	if key == "" {
		fmt.Fprintf(stderr, "%s configcenter delete: key required\n", brand.CLI)
		return 2
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.Call(ctx, controlplane.CmdConfigCenterDelete, map[string]any{"key": key})
	if err != nil {
		fmt.Fprintf(stderr, "%s configcenter delete: %v\n", brand.CLI, err)
		return 1
	}

	if deleted, _ := res["deleted"].(bool); deleted {
		fmt.Fprintf(stdout, "%s deleted\n", key)
	} else {
		fmt.Fprintf(stdout, "%s not found\n", key)
	}

	return 0
}

// cmdConfigCenterRating gets or sets rating for a key

