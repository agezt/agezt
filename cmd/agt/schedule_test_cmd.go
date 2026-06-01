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

// cmdScheduleTest implements `agt schedule test <id> [--count N] [--json]` (M120)
// — a read-only dry-run that previews a schedule's next N fire times, so an
// operator can confirm a daily/windowed/interval cadence does what they expect
// before relying on it. Exit 3 when the schedule is absent.
func cmdScheduleTest(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	count := 0
	var id string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--count":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s schedule test: --count needs a number\n", brand.CLI)
				return 2
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 {
				fmt.Fprintf(stderr, "%s schedule test: bad --count %q\n", brand.CLI, args[i])
				return 2
			}
			count = n
		case strings.HasPrefix(a, "--count="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--count="))
			if err != nil || n < 1 {
				fmt.Fprintf(stderr, "%s schedule test: bad --count\n", brand.CLI)
				return 2
			}
			count = n
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s schedule test <id> [--count N] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "preview the next N fire times of a schedule (dry-run; default 5)\n")
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s schedule test: unexpected flag %q\n", brand.CLI, a)
			return 2
		default:
			if id != "" {
				fmt.Fprintf(stderr, "%s schedule test: one schedule id\n", brand.CLI)
				return 2
			}
			id = a
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s schedule test: a schedule id is required\n", brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	callArgs := map[string]any{"id": id}
	if count > 0 {
		callArgs["count"] = count
	}
	res, err := c.Call(ctx, controlplane.CmdScheduleTest, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s schedule test: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		code := encodeJSON(stdout, res)
		if found, _ := res["found"].(bool); !found {
			return 3
		}
		return code
	}
	if found, _ := res["found"].(bool); !found {
		fmt.Fprintf(stderr, "%s schedule test: %s not found\n", brand.CLI, id)
		return 3
	}
	cadence, _ := res["cadence"].(string)
	fmt.Fprintf(stdout, "%s — %s\n", id, cadence)
	if enabled, ok := res["enabled"].(bool); ok && !enabled {
		fmt.Fprintln(stdout, "  (paused — these fires won't actually run until resumed)")
	}
	fires, _ := res["forecasts"].([]any)
	if len(fires) == 0 {
		fmt.Fprintln(stdout, "  no upcoming fires (a one-shot that already passed?)")
		return 0
	}
	for _, raw := range fires {
		m, _ := raw.(map[string]any)
		u := int64(0)
		if f, ok := m["unix"].(float64); ok {
			u = int64(f)
		}
		t := time.Unix(u, 0)
		fmt.Fprintf(stdout, "  %s (%s)\n", t.Format("2006-01-02 15:04"), strings.ToLower(t.Format("Mon")))
	}
	return 0
}
