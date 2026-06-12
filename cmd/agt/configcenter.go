// SPDX-License-Identifier: MIT

package main

// `agt configcenter` — Operator CLI for the Config Center system.
// This manages config entries with ratings, access policies, and audit logs.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/controlplane"
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
	fmt.Fprintf(stdout, "  set <key> <value> [--rating <rating>] [--description <desc>]\n")
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

// cmdConfigCenterSet sets a config value
func cmdConfigCenterSet(args []string, stdout, stderr io.Writer) int {
	var key, value, rating, description string

	i := 0
	for i < len(args) {
		switch args[i] {
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s configcenter set <key> <value> [--rating <rating>] [--description <desc>]\n", brand.CLI)
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

	c := dial(stderr)
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
	} else {
		fmt.Fprintf(stdout, "%s set\n", key)
	}

	return 0
}

// cmdConfigCenterGet gets a config value (admin only - bypasses access control)
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

	c := dial(stderr)
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

// cmdConfigCenterList lists all config entries
func cmdConfigCenterList(args []string, stdout, stderr io.Writer) int {
	var rating string
	asJSON := false

	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s configcenter list [--rating <rating>] [--json]\n", brand.CLI)
			return 0
		case "--rating":
			// Will be processed below
		case "--json":
			asJSON = true
		default:
			if rating == "" && !strings.HasPrefix(a, "--rating") {
				rating = a
			} else {
				fmt.Fprintf(stderr, "%s configcenter list: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	params := map[string]any{}
	if rating != "" {
		params["rating"] = rating
	}

	res, err := c.Call(ctx, controlplane.CmdConfigCenterList, params)
	if err != nil {
		fmt.Fprintf(stderr, "%s configcenter list: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	entries, _ := res["entries"].([]any)
	fmt.Fprintf(stdout, "%s configcenter: %d entries\n", brand.CLI, len(entries))

	// Sort by rating (secret first, then restricted, internal, public)
	type entryInfo struct {
		key       string
		rating    string
		value     string
		updatedAt int64
	}
	byRating := make(map[configcenter.Rating][]entryInfo)

	for _, e := range entries {
		em := e.(map[string]any)
		ei := entryInfo{
			key:       em["key"].(string),
			rating:    em["rating"].(string),
			value:     em["value"].(string),
			updatedAt: int64(em["updated_at"].(float64)),
		}
		byRating[configcenter.Rating(em["rating"].(string))] = append(byRating[configcenter.Rating(em["rating"].(string))], ei)
	}

	// Print by rating order
	for _, r := range []configcenter.Rating{configcenter.RatingSecret, configcenter.RatingRestricted, configcenter.RatingInternal, configcenter.RatingPublic} {
		entries := byRating[r]
		if len(entries) == 0 {
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })

		ratingLabel := string(r)
		if r == configcenter.RatingSecret {
			fmt.Fprintf(stdout, "\n🔴 %s (%d):\n", ratingLabel, len(entries))
		} else if r == configcenter.RatingRestricted {
			fmt.Fprintf(stdout, "\n🟡 %s (%d):\n", ratingLabel, len(entries))
		} else if r == configcenter.RatingInternal {
			fmt.Fprintf(stdout, "\n🔵 %s (%d):\n", ratingLabel, len(entries))
		} else {
			fmt.Fprintf(stdout, "\n🟢 %s (%d):\n", ratingLabel, len(entries))
		}

		for _, e := range entries {
			val := e.value
			if len(val) > 40 {
				val = val[:40] + "..."
			}
			if r == configcenter.RatingSecret {
				val = "********"
			}
			fmt.Fprintf(stdout, "  %-40s = %s\n", e.key, val)
		}
	}

	return 0
}

// cmdConfigCenterDelete deletes a config entry
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

	c := dial(stderr)
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

		c := dial(stderr)
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
		c := dial(stderr)
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

	c := dial(stderr)
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

	c := dial(stderr)
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

	c := dial(stderr)
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
