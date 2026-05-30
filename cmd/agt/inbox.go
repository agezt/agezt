// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdInbox implements `agt inbox [N] [--json]` — the Unified Inbox
// (SPEC-07 §4): channel conversations grouped by correlation, newest first.
func cmdInbox(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	limit := 0
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s inbox [N] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "list the last N channel conversation threads (default 20)\n")
			return 0
		default:
			if n, err := strconv.Atoi(a); err == nil && n > 0 {
				limit = n
				continue
			}
			fmt.Fprintf(stderr, "%s inbox: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	callArgs := map[string]any{}
	if limit > 0 {
		callArgs["limit"] = limit
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdInbox, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s inbox: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}

	threads, _ := res["threads"].([]any)
	if len(threads) == 0 {
		fmt.Fprintln(stdout, "inbox empty (no channel messages yet)")
		return 0
	}
	fmt.Fprintf(stdout, "%d thread(s):\n", len(threads))
	for _, raw := range threads {
		th, _ := raw.(map[string]any)
		kind, _ := th["channel_kind"].(string)
		chID, _ := th["channel_id"].(string)
		fmt.Fprintf(stdout, "── %s/%s\n", kind, chID)
		msgs, _ := th["messages"].([]any)
		for _, mraw := range msgs {
			m, _ := mraw.(map[string]any)
			dir, _ := m["direction"].(string)
			text, _ := m["text"].(string)
			arrow := "←"
			if dir == "out" {
				arrow = "→"
			}
			fmt.Fprintf(stdout, "   %s %s\n", arrow, text)
		}
	}
	return 0
}
