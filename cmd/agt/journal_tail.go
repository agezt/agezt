// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdJournalTail implements `agt journal tail [N] [--json]`.
//
// Snapshot read of the last N journaled events — different from
// `agt pulse` which is a live subscription, and different from
// `agt why` which is correlation-scoped. Tail is the right tool
// for "what just happened across every run?" — smoke tests,
// postmortems, scrolling back after spotting trouble in pulse.
//
//	agt journal tail              # last 20 events
//	agt journal tail 100          # last 100 events
//	agt journal tail --json       # full JSON dump, jq-friendly
func cmdJournalTail(args []string, stdout, stderr io.Writer) int {
	n := 20
	asJSON := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s journal tail [N] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "snapshot of the last N journaled events (default 20)\n")
			return 0
		default:
			// Single bare positional = N. Reject extras to catch typos.
			v, err := strconv.Atoi(a)
			if err != nil {
				fmt.Fprintf(stderr, "%s journal tail: unexpected arg %q (expected N or --json)\n", brand.CLI, a)
				return 2
			}
			if v < 1 {
				fmt.Fprintf(stderr, "%s journal tail: N must be >= 1 (got %d)\n", brand.CLI, v)
				return 2
			}
			n = v
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdJournalTail, map[string]any{"n": n})
	if err != nil {
		fmt.Fprintf(stderr, "%s journal tail: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	events, _ := res["events"].([]any)
	head := intOfStatus(res["head"])
	if len(events) == 0 {
		fmt.Fprintf(stdout, "no events in journal (head seq=%d)\n", head)
		return 0
	}
	fmt.Fprintf(stdout, "last %d event(s) (journal head seq=%d):\n", len(events), head)
	for _, raw := range events {
		m, _ := raw.(map[string]any)
		fmt.Fprintf(stdout, "  seq=%-5v %-22v subject=%v actor=%v\n",
			m["seq"], m["kind"], m["subject"], m["actor"])
	}
	return 0
}
