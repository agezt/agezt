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

// cmdChangelog implements `agt changelog [N] [--since <dur>] [--json]` (SPEC-08
// §4.2, M133) — the system timeline: a curated, tamper-evident fold of the
// journal showing the material changes to this system (halt/resume, policy
// changes, skill lifecycle, reflection, catalog sync, pulse pause/resume),
// newest-first, each with its event id so `agt why <id>` can prove and explain
// it. Distinct from `journal tail` (raw, every kind). `--system` is accepted as a
// spec-compatible alias (the component-version aggregation of §4.1 needs the
// plugin/update infrastructure that doesn't exist yet).
func cmdChangelog(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	limit := 0
	sinceMS := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--system":
			// Accepted alias — the system timeline is the default view.
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s changelog: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s changelog: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s changelog: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s changelog [N] [--since <dur>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "the system timeline: material changes (halt/resume, policy, skills, reflection,\n")
			fmt.Fprintf(stdout, "catalog sync, pulse) folded from the hash-chained journal — `%s why <id>` explains any\n", brand.CLI)
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s changelog: unexpected arg %q (expected N, --since, --json)\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	callArgs := map[string]any{}
	if limit > 0 {
		callArgs["limit"] = limit
	}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdChangelog, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s changelog: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}

	entries, _ := res["entries"].([]any)
	if len(entries) == 0 {
		fmt.Fprintln(stdout, "no system changes journaled yet (halt/resume, policy, skills, catalog, …).")
		return 0
	}
	for _, raw := range entries {
		m, _ := raw.(map[string]any)
		ts := int64(0)
		if f, ok := m["ts_unix_ms"].(float64); ok {
			ts = int64(f)
		}
		when := "—"
		if ts > 0 {
			when = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
		label, _ := m["label"].(string)
		detail, _ := m["detail"].(string)
		id, _ := m["event_id"].(string)
		line := fmt.Sprintf("  %s  %-26s", when, label)
		if detail != "" {
			line += "  " + detail
		}
		if id != "" {
			line += "  (" + id + ")" // full event id so `agt why <id>` works
		}
		fmt.Fprintln(stdout, line)
	}
	return 0
}
