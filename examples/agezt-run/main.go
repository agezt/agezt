// SPDX-License-Identifier: MIT

// Command agezt-run is a minimal example of embedding Agezt with the Go SDK
// (github.com/agezt/agezt/sdk). It connects to the local daemon, streams a run
// live (printing the answer as it generates), prints the run's cost and
// correlation id, then lists the last few runs.
//
// Usage:
//
//	agezt-run "summarise this repository"
//	agezt-run -model claude-opus-4-8 -timeout 2m "what changed recently?"
//
// It expects a running daemon (`agezt` / `agt`) under $AGEZT_HOME (or ~/.agezt).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/sdk"
)

func main() {
	model := flag.String("model", "", "model override (empty = the daemon's default)")
	timeout := flag.Duration("timeout", 5*time.Minute, "per-run wall-clock timeout")
	flag.Parse()

	intent := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if intent == "" {
		fmt.Fprintln(os.Stderr, "usage: agezt-run [-model M] [-timeout D] \"<intent>\"")
		os.Exit(2)
	}

	client, err := sdk.Dial("") // "" → $AGEZT_HOME or ~/.agezt
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	opts := []sdk.Option{sdk.WithTimeout(*timeout)}
	if *model != "" {
		opts = append(opts, sdk.WithModel(*model))
	}

	// Stream the run, printing the answer as it generates and noting each tool
	// the agent reaches for.
	res, err := client.RunStream(ctx, intent, func(ev *sdk.Event) {
		if txt, ok := sdk.TokenText(ev); ok {
			fmt.Print(txt)
		} else if name, ok := sdk.ToolCall(ev); ok {
			fmt.Fprintf(os.Stderr, "\n[tool: %s]\n", name)
		}
	}, opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nrun: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n\n--- done ---\n")
	fmt.Printf("correlation: %s\n", res.CorrelationID)
	if res.Model != "" {
		fmt.Printf("model: %s · %d iteration(s) · $%.4f\n", res.Model, res.Iterations, res.CostUSD)
	}

	// Show the last few runs from the journal.
	runs, err := client.Runs(ctx, 5)
	if err == nil && len(runs) > 0 {
		fmt.Printf("\nrecent runs:\n")
		for _, r := range runs {
			fmt.Printf("  %-12s %-10s %s\n", r.CorrelationID, r.Status, r.Intent)
		}
	}
}
