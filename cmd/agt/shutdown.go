// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdShutdown implements `agt shutdown` and `agt shutdown --json`.
// Asks the daemon to exit gracefully — the same teardown path as
// SIGTERM, but reachable from CI / scripted contexts that don't
// have a shell on the daemon's host. Auth is the control-plane
// token like every other command; no extra `--force` or `--yes`
// gate because the operator already proved they have the token
// just by being able to call the daemon.
//
// Exit codes:
//
//	0 — daemon ACKed shutdown
//	1 — couldn't reach the daemon (already down, perhaps?)
func cmdShutdown(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s shutdown [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "ask the daemon to exit gracefully (same path as SIGTERM)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s shutdown: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdShutdown, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s shutdown: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	fmt.Fprintf(stdout, "%s: shutdown requested; daemon will exit shortly.\n", brand.CLI)
	return 0
}
