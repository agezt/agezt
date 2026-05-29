// SPDX-License-Identifier: MIT

package main

// `agt journal grep <pattern> [filters]` — server-side filter
// instead of the older `agt journal tail 10000 --json | jq ...`
// pipeline. The pattern matches case-insensitively across the
// event's kind/subject/actor/correlation/payload. Additional
// flags AND together for tighter queries.

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

func cmdJournalGrep(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	limit := 100
	pattern := ""
	kind := ""
	subject := ""
	actor := ""
	corr := ""

	// Manual flag parse so we can keep the "pattern is the single
	// bare positional" rule consistent with the rest of the suite.
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--kind":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s journal grep: --kind needs a value\n", brand.CLI)
				return 2
			}
			kind = args[i]
		case "--subject":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s journal grep: --subject needs a value\n", brand.CLI)
				return 2
			}
			subject = args[i]
		case "--actor":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s journal grep: --actor needs a value\n", brand.CLI)
				return 2
			}
			actor = args[i]
		case "--correlation":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s journal grep: --correlation needs a value\n", brand.CLI)
				return 2
			}
			corr = args[i]
		case "--limit":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s journal grep: --limit needs a value\n", brand.CLI)
				return 2
			}
			v, err := strconv.Atoi(args[i])
			if err != nil || v < 1 {
				fmt.Fprintf(stderr, "%s journal grep: --limit must be a positive integer (got %q)\n", brand.CLI, args[i])
				return 2
			}
			limit = v
		case "-h", "--help":
			printJournalGrepHelp(stdout)
			return 0
		default:
			if pattern == "" {
				pattern = a
			} else {
				fmt.Fprintf(stderr, "%s journal grep: unexpected arg %q (only one pattern; quote it if it contains spaces)\n", brand.CLI, a)
				return 2
			}
		}
	}
	// At least one filter (pattern OR a typed filter) is required —
	// "grep with no constraint" is what `journal tail` does, and
	// silently aliasing here would be a footgun.
	if pattern == "" && kind == "" && subject == "" && actor == "" && corr == "" {
		fmt.Fprintf(stderr, "%s journal grep: provide a pattern or at least one filter (--kind/--subject/--actor/--correlation)\n", brand.CLI)
		fmt.Fprintf(stderr, "  (use `%s journal tail` for an unfiltered recent window)\n", brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	callArgs := map[string]any{"limit": limit}
	if pattern != "" {
		callArgs["pattern"] = pattern
	}
	if kind != "" {
		callArgs["kind"] = kind
	}
	if subject != "" {
		callArgs["subject"] = subject
	}
	if actor != "" {
		callArgs["actor"] = actor
	}
	if corr != "" {
		callArgs["correlation_id"] = corr
	}
	res, err := c.Call(ctx, controlplane.CmdJournalGrep, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s journal grep: %v\n", brand.CLI, err)
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
		fmt.Fprintf(stdout, "no matching events (journal head seq=%d)\n", head)
		return 0
	}
	fmt.Fprintf(stdout, "%d match(es) (journal head seq=%d, limit=%d):\n", len(events), head, limit)
	for _, raw := range events {
		m, _ := raw.(map[string]any)
		fmt.Fprintf(stdout, "  seq=%-5v %-22v subject=%v actor=%v\n",
			m["seq"], m["kind"], m["subject"], m["actor"])
	}
	return 0
}

func printJournalGrepHelp(w io.Writer) {
	fmt.Fprintf(w, "usage: %s journal grep <pattern> [filters] [--json]\n", brand.CLI)
	fmt.Fprintf(w, "server-side filter over journal events; cheaper than `tail | jq`\n")
	fmt.Fprintf(w, "Filters (all AND together):\n")
	fmt.Fprintf(w, "  <pattern>             case-insensitive substring; matches kind/subject/actor/corr/payload\n")
	fmt.Fprintf(w, "  --kind <k>            exact-match Event.Kind (e.g. tool.invoked)\n")
	fmt.Fprintf(w, "  --subject <s>         exact-match Event.Subject\n")
	fmt.Fprintf(w, "  --actor <a>           exact-match Event.Actor\n")
	fmt.Fprintf(w, "  --correlation <c>     exact-match Event.CorrelationID\n")
	fmt.Fprintf(w, "  --limit N             max matches (default 100, max 10000)\n")
	fmt.Fprintf(w, "  --json                emit full match array (pipe to jq)\n")
}
