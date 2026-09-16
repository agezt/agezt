// SPDX-License-Identifier: MIT
//
// cmd/agt `run` mode runners (runJSONMode, runDryRunMode).
// Extracted from main_run_modes.go during Day 211 god-file refactor (#95).
// Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
)

func runJSONMode(ctx context.Context, c *controlplane.Client, runArgs map[string]any, stdout, stderr io.Writer) int {
	enc := json.NewEncoder(stdout)
	// Compact one-object-per-line is the convention for ndjson;
	// jq -c handles it natively. Using NewEncoder also flushes
	// after every Encode call, so each line streams as it arrives.
	result, err := c.Stream(ctx, controlplane.CmdRun, runArgs, func(ev *event.Event) {
		_ = enc.Encode(map[string]any{"type": "event", "event": ev})
	})
	if err != nil {
		// Final line: error envelope. Exit 1 so CI scripts
		// distinguish failure from success.
		_ = enc.Encode(map[string]any{"type": "error", "error": err.Error()})
		return 1
	}
	_ = enc.Encode(map[string]any{"type": "result", "result": result})
	return 0
}
func runDryRunMode(ctx context.Context, c *controlplane.Client, runArgs map[string]any, asJSON bool, stdout, stderr io.Writer) int {
	plan, err := c.Call(ctx, controlplane.CmdRun, runArgs)
	if err != nil {
		if asJSON {
			_ = json.NewEncoder(stdout).Encode(map[string]any{"type": "error", "error": err.Error()})
		} else {
			fmt.Fprintf(stderr, "%s run --dry-run: %v\n", brand.CLI, err)
		}
		return 1
	}
	if asJSON {
		enc, _ := json.Marshal(plan)
		fmt.Fprintf(stdout, "%s\n", enc)
		return 0
	}

	str := func(k string) string { s, _ := plan[k].(string); return s }
	fmt.Fprintf(stdout, "dry-run — this run would execute as:\n")
	fmt.Fprintf(stdout, "  intent        : %s\n", str("intent"))
	fmt.Fprintf(stdout, "  tenant        : %s\n", str("tenant"))
	model := str("model")
	if known, _ := plan["model_known"].(bool); known {
		caps := []string{}
		if v, _ := plan["supports_vision"].(bool); v {
			caps = append(caps, "vision")
		}
		if v, _ := plan["supports_tools"].(bool); v {
			caps = append(caps, "tool_call")
		}
		capStr := "no advertised caps"
		if len(caps) > 0 {
			capStr = strings.Join(caps, "+")
		}
		fmt.Fprintf(stdout, "  model         : %s (%s) [catalog: %s]\n", model, str("model_source"), capStr)
	} else {
		fmt.Fprintf(stdout, "  model         : %s (%s) [not in catalog]\n", model, str("model_source"))
	}
	fmt.Fprintf(stdout, "  system prompt : %s\n", str("system_source"))
	fmt.Fprintf(stdout, "  timeout       : %s\n", str("timeout"))
	fmt.Fprintf(stdout, "  cost cap      : %s\n", str("cost_cap"))
	fmt.Fprintf(stdout, "  execution     : %s (%s), warden=%s\n", str("execution_profile"), str("execution_profile_source"), str("warden_profile"))
	if peer := str("remote_peer"); peer != "" {
		fmt.Fprintf(stdout, "  remote peer   : %s\n", peer)
	}

	tools := toStringSlice(plan["tools"])
	switch str("tools_mode") {
	case "all":
		fmt.Fprintf(stdout, "  tools         : all (%d): %s\n", len(tools), strings.Join(tools, ", "))
	case "restricted":
		fmt.Fprintf(stdout, "  tools         : restricted (%d): %s\n", len(tools), strings.Join(tools, ", "))
	default: // "none (--no-tools)"
		fmt.Fprintf(stdout, "  tools         : none (--no-tools)\n")
	}
	if dropped := toStringSlice(plan["tools_dropped"]); len(dropped) > 0 {
		fmt.Fprintf(stdout, "  tools dropped : %s (requested but not registered)\n", strings.Join(dropped, ", "))
	}
	if warns := toStringSlice(plan["warnings"]); len(warns) > 0 {
		fmt.Fprintf(stdout, "\nwarnings:\n")
		for _, w := range warns {
			fmt.Fprintf(stdout, "  ! %s\n", w)
		}
	}
	fmt.Fprintf(stdout, "\n(no run started, no tokens spent — drop --dry-run to execute)\n")
	return 0
}
