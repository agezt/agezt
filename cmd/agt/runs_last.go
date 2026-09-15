// SPDX-License-Identifier: MIT
//
// cmd/agt runs last sub-command handler + small helpers (cmdRunsLast,
// failedByReasonStr, arcPreview, synthesizePlanSummary, fmtDuration).
// Extracted from runs_last.go during Day 211 god-file refactor (#36, #63).
// Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/format"
)

func cmdRunsLast(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s runs last [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "render the most-recent run as a task arc (shorthand for `runs show`)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s runs last: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Limit=1 because CmdRunsList sorts newest-first — the head of
	// the slice is exactly what `last` wants.
	listRes, err := c.Call(ctx, controlplane.CmdRunsList, map[string]any{"limit": 1})
	if err != nil {
		fmt.Fprintf(stderr, "%s runs last: %v\n", brand.CLI, err)
		return 1
	}
	runs, _ := listRes["runs"].([]any)
	if len(runs) == 0 {
		fmt.Fprintln(stderr, "no runs yet (journal has no task.received events)")
		return 1
	}
	row, _ := runs[0].(map[string]any)
	corr, _ := row["correlation_id"].(string)
	if corr == "" {
		fmt.Fprintln(stderr, "runs last: most-recent run has no correlation_id (corrupt journal?)")
		return 1
	}

	// Delegate to runs show so the rendering path is identical.
	// Pass the JSON flag through verbatim.
	delegateArgs := []string{corr}
	if asJSON {
		delegateArgs = append(delegateArgs, "--json")
	}
	return cmdRunsShow(delegateArgs, stdout, stderr)
}
func failedByReasonStr(raw any) string {
	m, _ := raw.(map[string]any)
	if len(m) == 0 {
		return ""
	}
	var parts []string
	seen := map[string]bool{}
	for _, reason := range []string{"error", "timeout", "max_iters", "canceled", "unknown"} {
		if n := intOfStatus(m[reason]); n > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", reason, n))
			seen[reason] = true
		}
	}
	// Any reason tags we didn't anticipate — append in sorted order so the
	// output stays deterministic.
	var extras []string
	for reason := range m {
		if !seen[reason] {
			extras = append(extras, reason)
		}
	}
	sort.Strings(extras)
	for _, reason := range extras {
		if n := intOfStatus(m[reason]); n > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", reason, n))
		}
	}
	return strings.Join(parts, ", ")
}
// arcPreviewRunes bounds a tool input/output excerpt on a task-arc line (M68) —
// long enough to read a short command or error, short enough to keep one event
// per line. Mirrors the server-side toolOutputPreviewRunes for `agt tool log`.
const arcPreviewRunes = 80

// arcPreview renders a journaled tool input/output value as a compact one-line
func arcPreview(v any) string {
	if v == nil {
		return ""
	}
	var s string
	switch t := v.(type) {
	case string:
		s = t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		s = string(b)
	}
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) > arcPreviewRunes {
		return string(r[:arcPreviewRunes]) + "…"
	}
	return s
}
func synthesizePlanSummary(chain []map[string]any) map[string]any {
	summary := map[string]any{"status": "running"}
	for _, e := range chain {
		kind, _ := e["kind"].(string)
		payload, _ := e["payload"].(map[string]any)
		switch kind {
		case "plan.started":
			if name, _ := payload["plan_name"].(string); name != "" {
				summary["intent"] = "plan: " + name
			}
		case "plan.completed":
			summary["status"] = "completed"
		case "plan.failed":
			summary["status"] = "failed"
			if r, _ := payload["reason"].(string); r != "" {
				summary["reason"] = r
			}
		}
	}
	return summary
}
func fmtDuration(ms int64) string { return format.Duration(ms) }
