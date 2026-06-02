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

// cmdMemoryLog implements `agt memory log [N] [--op <o>] [--since <dur>]
// [--json]` (M85) — a timeline of memory operations (written/forgotten/
// superseded). `memory list` shows the current records; this shows the history
// of how they came to be: what the agent learned, forgot, and replaced.
func cmdMemoryLog(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args) // M129: inspect a tenant's own memory
	asJSON := false
	limit := 0
	op := ""
	sinceMS := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--op":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory log: --op needs a value\n", brand.CLI)
				return 2
			}
			i++
			op = args[i]
		case strings.HasPrefix(a, "--op="):
			op = strings.TrimPrefix(a, "--op=")
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory log: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s memory log: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s memory log: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s memory log [N] [--op written|forgotten|superseded] [--since <dur>] [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show the memory-operation timeline (what the agent learned, forgot, replaced)\n")
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s memory log: unexpected arg %q (expected N, --op, --since, or --json)\n", brand.CLI, a)
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
	if op != "" {
		callArgs["op"] = op
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdMemoryLog, withTenant(tenant, callArgs))
	if err != nil {
		fmt.Fprintf(stderr, "%s memory log: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	ops, _ := res["ops"].([]any)
	if len(ops) == 0 {
		fmt.Fprintln(stdout, "no memory operations journaled yet.")
		return 0
	}
	for _, raw := range ops {
		m, _ := raw.(map[string]any)
		opName, _ := m["op"].(string)
		mtyp, _ := m["type"].(string)
		subject, _ := m["subject"].(string)
		id, _ := m["id"].(string)
		ts := int64(0)
		if f, ok := m["ts_unix_ms"].(float64); ok {
			ts = int64(f)
		}
		whenStr := "—"
		if ts > 0 {
			whenStr = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
		line := fmt.Sprintf("  %s  %-9s", whenStr, opName)
		if mtyp != "" {
			line += fmt.Sprintf(" %-12s", mtyp)
		}
		if subject != "" {
			line += "  " + subject
		}
		if id != "" {
			line += "  (" + id + ")"
		}
		fmt.Fprintln(stdout, line)
	}
	return 0
}
