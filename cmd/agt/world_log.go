// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdWorldLog implements `agt world log [N] [--kind entity|relation]
// [--since <dur>] [--json]` (M86) — a timeline of world-model operations. `world
// list` shows the current graph; this shows how it formed: what entities and
// relations the agent observed, reinforced, and forgot. The world-model analogue
// of `agt memory log`.
func cmdWorldLog(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args) // M129: inspect a tenant's own world model
	asJSON := false
	limit := 0
	kind := ""
	sinceMS := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--kind":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s world log: --kind needs a value\n", brand.CLI)
				return 2
			}
			i++
			kind = args[i]
		case strings.HasPrefix(a, "--kind="):
			kind = strings.TrimPrefix(a, "--kind=")
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s world log: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s world log: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s world log: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s world log [N] [--kind entity|relation] [--since <dur>] [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show the world-model operation timeline (entities/relations observed, reinforced, forgotten)\n")
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s world log: unexpected arg %q (expected N, --kind, --since, or --json)\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	callArgs := map[string]any{}
	if limit > 0 {
		callArgs["limit"] = limit
	}
	if kind != "" {
		callArgs["kind"] = kind
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdWorldLog, withTenant(tenant, callArgs))
	if err != nil {
		fmt.Fprintf(stderr, "%s world log: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	ops, _ := res["ops"].([]any)
	if len(ops) == 0 {
		fmt.Fprintln(stdout, "no world-model operations journaled yet.")
		return 0
	}
	for _, raw := range ops {
		m, _ := raw.(map[string]any)
		opName, _ := m["op"].(string)
		what, _ := m["what"].(string)
		label, _ := m["label"].(string)
		ts := int64(0)
		if f, ok := m["ts_unix_ms"].(float64); ok {
			ts = int64(f)
		}
		whenStr := "—"
		if ts > 0 {
			whenStr = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
		line := fmt.Sprintf("  %s  %-9s %-8s", whenStr, opName, what)
		if label != "" {
			line += "  " + label
		}
		fmt.Fprintln(stdout, line)
	}
	return 0
}
