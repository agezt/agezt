// SPDX-License-Identifier: MIT

package main

// agt config-center metadata subcommands: cmdConfigCenterRating +
// cmdConfigCenterAccessLog + cmdConfigCenterAudit +
// cmdConfigCenterHealth. Carved out of configcenter.go during the
// Day 169 god-file split so the main file can stay focused on the
// dispatcher + Help + Set + Get + List + Delete CRUD surface.
// Public API unchanged.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)
func cmdConfigCenterRating(args []string, stdout, stderr io.Writer) int {
	var key, rating string
	setMode := false

	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s configcenter rating <key> [--rating <rating>]\n", brand.CLI)
			return 0
		case "--rating":
			setMode = true
		default:
			if key == "" {
				key = a
			} else if rating == "" && setMode {
				rating = a
			} else if rating == "" {
				rating = a
				setMode = true
			} else {
				fmt.Fprintf(stderr, "%s configcenter rating: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
		}
	}

	if key == "" {
		fmt.Fprintf(stderr, "%s configcenter rating: key required\n", brand.CLI)
		return 2
	}

	if rating != "" {
		// Set mode
		validRating := false
		for _, r := range []string{"public", "internal", "restricted", "secret"} {
			if rating == r {
				validRating = true
				break
			}
		}
		if !validRating {
			fmt.Fprintf(stderr, "%s configcenter rating: invalid rating %q\n", brand.CLI, rating)
			return 2
		}

		c := dialpkg.New(stderr)
		if c == nil {
			return 1
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		res, err := c.Call(ctx, controlplane.CmdConfigCenterSetRating, map[string]any{
			"key":    key,
			"rating": rating,
		})
		if err != nil {
			fmt.Fprintf(stderr, "%s configcenter rating: %v\n", brand.CLI, err)
			return 1
		}

		fmt.Fprintf(stdout, "%s rating set to %s\n", key, rating)
		if override, _ := res["override"].(bool); override {
			fmt.Fprintf(stdout, "  (manual override)\n")
		}
	} else {
		// Get mode
		c := dialpkg.New(stderr)
		if c == nil {
			return 1
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		res, err := c.Call(ctx, controlplane.CmdConfigCenterGet, map[string]any{"key": key})
		if err != nil {
			fmt.Fprintf(stderr, "%s configcenter rating: %v\n", brand.CLI, err)
			return 1
		}

		entry, _ := res["entry"].(map[string]any)
		if entry == nil {
			fmt.Fprintf(stderr, "%s configcenter rating: key %q not found\n", brand.CLI, key)
			return 1
		}

		autoRating := ""
		if ar, ok := entry["auto_rating"]; ok && ar != entry["rating"] {
			autoRating = fmt.Sprintf(" (auto: %s)", ar)
		}

		fmt.Fprintf(stdout, "%s: %s%s\n", key, entry["rating"], autoRating)
	}

	return 0
}

// cmdConfigCenterAccessLog views config access history
func cmdConfigCenterAccessLog(args []string, stdout, stderr io.Writer) int {
	var key, agentID, since string
	asJSON := false

	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s configcenter access-log [--key <key>] [--agent <agent>] [--since <duration>] [--json]\n", brand.CLI)
			return 0
		case "--key":
			// Will be processed
		case "--agent":
			// Will be processed
		case "--since":
			// Will be processed
		case "--json":
			asJSON = true
		default:
			fmt.Fprintf(stderr, "%s configcenter access-log: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	// Parse flags
	i := 0
	for i < len(args) {
		if args[i] == "--key" {
			i++
			if i < len(args) {
				key = args[i]
			}
		} else if args[i] == "--agent" {
			i++
			if i < len(args) {
				agentID = args[i]
			}
		} else if args[i] == "--since" {
			i++
			if i < len(args) {
				since = args[i]
			}
		} else if args[i] != "--json" {
			fmt.Fprintf(stderr, "%s configcenter access-log: unexpected arg %q\n", brand.CLI, args[i])
			return 2
		}
		i++
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	params := map[string]any{}
	if key != "" {
		params["key"] = key
	}
	if agentID != "" {
		params["agent_id"] = agentID
	}
	if since != "" {
		params["since"] = since
	}

	res, err := c.Call(ctx, controlplane.CmdConfigCenterAccessLog, params)
	if err != nil {
		fmt.Fprintf(stderr, "%s configcenter access-log: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	logs, _ := res["logs"].([]any)
	if len(logs) == 0 {
		fmt.Fprintf(stdout, "No access logs found\n")
		return 0
	}

	fmt.Fprintf(stdout, "%s configcenter access-log: %d entries\n\n", brand.CLI, len(logs))

	for _, l := range logs {
		lm := l.(map[string]any)
		ts := time.UnixMilli(int64(lm["timestamp"].(float64)))

		agent := lm["agent_id"].(string)
		logKey := lm["key"].(string)
		rating := lm["rating"].(string)
		decision := lm["decision"].(string)
		reason := lm["reason"].(string)

		decisionIcon := "✅"
		if decision == "denied" {
			decisionIcon = "❌"
		}

		fmt.Fprintf(stdout, "%s  %s  %s  %s\n", ts.Format("06-01-02 15:04:05"), decisionIcon, agent, logKey)
		fmt.Fprintf(stdout, "    rating=%s reason=%s\n", rating, reason)
	}

	return 0
}

// cmdConfigCenterAudit views audit log
func cmdConfigCenterAudit(args []string, stdout, stderr io.Writer) int {
	var since string
	asJSON := false

	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s configcenter audit [--since <duration>] [--json]\n", brand.CLI)
			return 0
		case "--since":
			// Will be processed
		case "--json":
			asJSON = true
		default:
			if strings.HasPrefix(a, "--") {
				fmt.Fprintf(stderr, "%s configcenter audit: unexpected flag %q\n", brand.CLI, a)
			} else {
				fmt.Fprintf(stderr, "%s configcenter audit: unexpected arg %q\n", brand.CLI, a)
			}
			return 2
		}
	}

	// Parse flags
	i := 0
	for i < len(args) {
		if args[i] == "--since" {
			i++
			if i < len(args) {
				since = args[i]
			}
		}
		i++
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	params := map[string]any{}
	if since != "" {
		params["since"] = since
	}

	res, err := c.Call(ctx, controlplane.CmdConfigCenterAudit, params)
	if err != nil {
		fmt.Fprintf(stderr, "%s configcenter audit: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	entries, _ := res["entries"].([]any)
	if len(entries) == 0 {
		fmt.Fprintf(stdout, "No audit entries found\n")
		return 0
	}

	fmt.Fprintf(stdout, "%s configcenter audit: %d entries\n\n", brand.CLI, len(entries))

	for _, e := range entries {
		em := e.(map[string]any)
		ts := time.UnixMilli(int64(em["timestamp"].(float64)))
		event := em["event"].(string)
		key := em["key"].(string)
		actor := em["actor"].(string)
		action := em["action"].(string)

		fmt.Fprintf(stdout, "%s  %s  %s  %s\n", ts.Format("06-01-02 15:04:05"), event, key, actor)
		fmt.Fprintf(stdout, "    action=%s\n", action)
	}

	return 0
}

// cmdConfigCenterHealth checks Config Center health
func cmdConfigCenterHealth(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Fprintf(stdout, "usage: %s configcenter health\n", brand.CLI)
			return 0
		}
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.Call(ctx, controlplane.CmdConfigCenterHealth, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s configcenter health: %v\n", brand.CLI, err)
		return 1
	}

	status, _ := res["status"].(string)
	fmt.Fprintf(stdout, "Status: %s\n", status)

	if checks, ok := res["checks"].(map[string]any); ok {
		for k, v := range checks {
			icon := "✅"
			if v != "ok" && v != "connected" && v != "healthy" {
				icon = "❌"
			}
			fmt.Fprintf(stdout, "  %s %s: %s\n", icon, k, v)
		}
	}

	stats, _ := res["stats"].(map[string]any)
	if stats != nil {
		fmt.Fprintf(stdout, "\nStats:\n")
		for k, v := range stats {
			fmt.Fprintf(stdout, "  %s: %v\n", k, v)
		}
	}

	return 0
}
