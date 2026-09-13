// SPDX-License-Identifier: MIT

package main

// agt configcenter SET subcommand: cmdConfigCenterSet. Carved out
// of configcenter.go during the Day 194 god-file split so the main
// file can stay focused on the dispatcher + Help + Get + Delete
// and the list file can stay focused on the list subcommand.
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)

func cmdConfigCenterSet(args []string, stdout, stderr io.Writer) int {
	var key, value, rating, description, allowAgents, denyAgents string

	i := 0
	for i < len(args) {
		switch args[i] {
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s configcenter set <key> <value> [--rating <rating>] [--description <desc>] [--allow-agent csv] [--deny-agent csv]\n", brand.CLI)
			return 0
		case "--rating":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s configcenter set: --rating requires a value\n", brand.CLI)
				return 2
			}
			i++
			rating = args[i]
		case "--description":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s configcenter set: --description requires a value\n", brand.CLI)
				return 2
			}
			i++
			description = args[i]
		case "--allow-agent", "--allow-agents":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s configcenter set: %s requires a value\n", brand.CLI, args[i])
				return 2
			}
			i++
			allowAgents = args[i]
		case "--deny-agent", "--deny-agents":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s configcenter set: %s requires a value\n", brand.CLI, args[i])
				return 2
			}
			i++
			denyAgents = args[i]
		default:
			if key == "" {
				key = args[i]
			} else if value == "" {
				value = args[i]
			} else {
				fmt.Fprintf(stderr, "%s configcenter set: unexpected arg %q\n", brand.CLI, args[i])
				return 2
			}
		}
		i++
	}

	if key == "" {
		fmt.Fprintf(stderr, "%s configcenter set: key required\n", brand.CLI)
		return 2
	}
	if value == "" {
		fmt.Fprintf(stderr, "%s configcenter set: value required\n", brand.CLI)
		return 2
	}

	// Determine rating
	r := configcenter.RatingInternal
	if rating != "" {
		switch strings.ToLower(rating) {
		case "public":
			r = configcenter.RatingPublic
		case "internal":
			r = configcenter.RatingInternal
		case "restricted":
			r = configcenter.RatingRestricted
		case "secret":
			r = configcenter.RatingSecret
		default:
			fmt.Fprintf(stderr, "%s configcenter set: invalid rating %q (public, internal, restricted, secret)\n", brand.CLI, rating)
			return 2
		}
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	params := map[string]any{
		"key":    key,
		"value":  value,
		"rating": string(r),
	}
	if description != "" {
		params["description"] = description
	}
	if allowAgents != "" {
		params["allowed_agents"] = splitList(allowAgents)
	}
	if denyAgents != "" {
		params["excluded_agents"] = splitList(denyAgents)
	}

	res, err := c.Call(ctx, controlplane.CmdConfigCenterSet, params)
	if err != nil {
		fmt.Fprintf(stderr, "%s configcenter set: %v\n", brand.CLI, err)
		return 1
	}

	entry, _ := res["entry"].(map[string]any)
	if entry != nil {
		fmt.Fprintf(stdout, "%s: ", key)
		if val, _ := entry["value"].(string); val != "" {
			// Truncate long values
			if len(val) > 60 {
				fmt.Fprintf(stdout, "%s...\n", val[:60])
			} else {
				fmt.Fprintf(stdout, "%s\n", val)
			}
		}
		fmt.Fprintf(stdout, "  rating: %s\n", entry["rating"])
		if desc, _ := entry["description"].(string); desc != "" {
			fmt.Fprintf(stdout, "  description: %s\n", desc)
		}
		if allowed, _ := entry["allowed_agents"].([]any); len(allowed) > 0 {
			fmt.Fprintf(stdout, "  allow agents: %s\n", joinAnyStrings(allowed, ", "))
		}
		if denied, _ := entry["excluded_agents"].([]any); len(denied) > 0 {
			fmt.Fprintf(stdout, "  deny agents: %s\n", joinAnyStrings(denied, ", "))
		}
	} else {
		fmt.Fprintf(stdout, "%s set\n", key)
	}

	return 0
}

