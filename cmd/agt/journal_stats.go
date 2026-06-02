// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdJournalStats implements `agt journal stats [--json]` (M132) — the journal's
// size and shape: total events, segments, bytes, the time span it covers, and a
// per-event-kind breakdown so an operator can see WHAT is filling it. The journal
// is append-only and full-retention (projections rebuild from it on boot, so it
// is not pruned in place); when disk is tight the remedy is to archive it
// (`agt backup` / `agt journal export`) and/or grow the disk. Walks the whole
// journal, so it scans every segment — fine for an operator-invoked command.
func cmdJournalStats(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s journal stats [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show journal size/shape: events, segments, bytes, time span, by-kind breakdown\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s journal stats: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	// A large journal can take a while to fold; give it room.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdJournalStats, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s journal stats: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}

	events := intOfStatus(res["events"])
	segments := intOfStatus(res["segments"])
	bytes := intOfStatus(res["bytes"])
	fmt.Fprintf(stdout, "events   : %d\n", events)
	fmt.Fprintf(stdout, "segments : %d\n", segments)
	fmt.Fprintf(stdout, "size     : %s  (append-only, full retention)\n", humanBytes(bytes))

	oldest := intOfStatus(res["oldest_unix_ms"])
	newest := intOfStatus(res["newest_unix_ms"])
	if oldest > 0 && newest > 0 {
		span := time.Duration(newest-oldest) * time.Millisecond
		fmt.Fprintf(stdout, "span     : %s → %s (%s)\n",
			time.UnixMilli(oldest).Format("2006-01-02"),
			time.UnixMilli(newest).Format("2006-01-02"),
			fmtDuration(span.Milliseconds()))
	}

	if byKind, _ := res["by_kind"].(map[string]any); len(byKind) > 0 {
		type kc struct {
			kind  string
			count int64
		}
		rows := make([]kc, 0, len(byKind))
		for k, v := range byKind {
			rows = append(rows, kc{kind: k, count: intOfStatus(v)})
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].count != rows[j].count {
				return rows[i].count > rows[j].count
			}
			return rows[i].kind < rows[j].kind
		})
		fmt.Fprintf(stdout, "\nby kind (top events):\n")
		shown := rows
		if len(shown) > 12 {
			shown = shown[:12]
		}
		for _, r := range shown {
			pct := 0.0
			if events > 0 {
				pct = float64(r.count) / float64(events) * 100
			}
			fmt.Fprintf(stdout, "  %-28s %8d  (%.1f%%)\n", r.kind, r.count, pct)
		}
		if len(rows) > len(shown) {
			fmt.Fprintf(stdout, "  … and %d more kind(s)\n", len(rows)-len(shown))
		}
	}
	if bytes > 0 {
		fmt.Fprintf(stdout, "\nto reclaim space: archive with `%s backup` or `%s journal export`, then move to a larger disk\n", brand.CLI, brand.CLI)
	}
	return 0
}
