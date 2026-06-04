// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdCache implements `agt cache [--since <dur>] [--tenant <id>] [--json]` (M294)
// — the prompt-cache savings view: tokens served from / written to the provider
// cache and the microcents that saved versus paying the full input rate. The CLI
// counterpart of the Web UI Cache panel; both proxy CmdCacheStats.
func cmdCache(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	sinceMS := int64(0)
	sinceLabel := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s cache: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s cache: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s cache: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
			sinceLabel = d.String()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s cache [--since <dur>] [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "prompt-cache savings: tokens served from cache + microcents saved vs the full input rate\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s cache: unexpected arg %q (expected --since, --tenant, or --json)\n", brand.CLI, a)
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
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	res, err := c.Call(ctx, controlplane.CmdCacheStats, withTenant(tenant, callArgs))
	if err != nil {
		fmt.Fprintf(stderr, "%s cache: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}

	calls := intOfStatus(res["calls"])
	windowSuffix := ""
	if sinceLabel != "" {
		windowSuffix = " in the last " + sinceLabel
	}
	if calls == 0 {
		fmt.Fprintf(stdout, "no priced calls%s.\n", windowSuffix)
		return 0
	}
	saved := intOfStatus(res["saved_microcents"])
	reads := intOfStatus(res["cached_input_tokens"])
	writes := intOfStatus(res["cache_write_input_tokens"])
	fmt.Fprintf(stdout, "prompt cache (over %d priced call(s)%s):\n\n", calls, windowSuffix)
	fmt.Fprintf(stdout, "  saved        : %s\n", fmtUSD(saved))
	fmt.Fprintf(stdout, "  cache reads  : %d tok\n", reads)
	fmt.Fprintf(stdout, "  cache writes : %d tok\n", writes)
	return 0
}
