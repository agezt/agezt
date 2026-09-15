// SPDX-License-Identifier: MIT
//
// cmd/agt runs intervene/cancel handlers (cmdRunsIntervene, cmdRunsCancel).
// Extracted from runs.go during Day 211 god-file refactor (#59).
// Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)

func cmdRunsIntervene(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var primitive, corr, key string
	var lease time.Duration
	var parts []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--lease":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s runs intervene: --lease needs a duration\n", brand.CLI)
				return 2
			}
			d, err := time.ParseDuration(args[i+1])
			if err != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s runs intervene: bad --lease %q\n", brand.CLI, args[i+1])
				return 2
			}
			lease = d
			i++
		case a == "--key":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s runs intervene: --key needs an idempotency key\n", brand.CLI)
				return 2
			}
			key = args[i+1]
			i++
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s runs intervene <halt|abort|redirect|adjust|query> <correlation> [directive...] [--lease <dur>] [--key <id>] [--json]\n", brand.CLI)
			return 0
		case primitive == "":
			primitive = a
		case corr == "":
			corr = a
		default:
			parts = append(parts, a)
		}
	}
	if primitive == "" || corr == "" {
		fmt.Fprintf(stderr, "%s runs intervene: primitive and correlation are required\n", brand.CLI)
		return 2
	}
	directive := strings.Join(parts, " ")
	callArgs := map[string]any{"primitive": primitive, "correlation": corr}
	if directive != "" {
		callArgs["directive"] = directive
	}
	if lease > 0 {
		callArgs["lease_ms"] = float64(lease / time.Millisecond)
	}
	if key != "" {
		callArgs["idempotency_key"] = key
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdRunIntervene, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s runs intervene: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	if acc, _ := res["accepted"].(bool); acc {
		fmt.Fprintf(stdout, "%s: %s %v\n", corr, primitive, res["state"])
		return 0
	}
	fmt.Fprintf(stderr, "intervention not accepted for correlation %q: %v\n", corr, res["reason"])
	return 1
}
func cmdRunsCancel(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var corr string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s runs cancel <correlation> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "cancel one in-flight run by correlation id (leaves the daemon running)\n")
			return 0
		default:
			if corr == "" {
				corr = a
				continue
			}
			fmt.Fprintf(stderr, "%s runs cancel: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if corr == "" {
		fmt.Fprintf(stderr, "%s runs cancel: correlation id required\n", brand.CLI)
		return 2
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdCancelRun, map[string]any{"correlation": corr})
	if err != nil {
		fmt.Fprintf(stderr, "%s runs cancel: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
	}
	cancelled, _ := res["cancelled"].(bool)
	if !cancelled {
		if !asJSON {
			fmt.Fprintf(stderr, "no in-flight run with correlation %q (already finished or unknown)\n", corr)
		}
		return 1
	}
	if !asJSON {
		fmt.Fprintf(stdout, "cancelled run %s (it will terminate as failed/canceled)\n", corr)
	}
	return 0
}
