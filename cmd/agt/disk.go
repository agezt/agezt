// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdDisk implements `agt disk [--json]` (M131) — the journal's size on disk and
// the free space of the filesystem it lives on. The journal is append-only and
// never shrinks, so a full disk is the classic silent outage on a small host;
// this is the direct-inspection counterpart to the `agt doctor` disk check. Exit
// 0 normally, 1 on a daemon/transport error (CI can branch on it).
func cmdDisk(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s disk [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "show the journal's on-disk size and free space on its filesystem\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s disk: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdDiskStats, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s disk: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}

	base, _ := res["base_dir"].(string)
	journal := intOfStatus(res["journal_bytes"])
	fmt.Fprintf(stdout, "base dir : %s\n", base)
	fmt.Fprintf(stdout, "journal  : %s\n", humanBytes(journal))
	if avail, _ := res["disk_available"].(bool); avail {
		free := intOfStatus(res["disk_free_bytes"])
		total := intOfStatus(res["disk_total_bytes"])
		pct, _ := res["disk_free_pct"].(float64)
		fmt.Fprintf(stdout, "disk     : %s free of %s (%.1f%%)\n", humanBytes(free), humanBytes(total), pct)
		if pct < diskWarnPct {
			fmt.Fprintf(stdout, "  ⚠ low free space — the journal grows append-only; archive with `%s journal export` and free space\n", brand.CLI)
		}
	} else {
		fmt.Fprintf(stdout, "disk     : free space unavailable\n")
	}
	return 0
}
