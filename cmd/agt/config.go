// SPDX-License-Identifier: MIT

package main

// `agt config show` — the operator's dashboard for "what is this
// daemon ACTUALLY running with?" The handler returns resolved
// paths, model, system-prompt presence, tool/plugin counts,
// ask-policy, and which AGEZT_* env vars are set. Values are
// never returned (presence only) so the JSON view is safe to
// paste into bug reports.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s config: subcommand required (show)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "show":
		return cmdConfigShow(args[1:], stdout, stderr)
	case "-h", "--help":
		fmt.Fprintf(stdout, "usage: %s config show [--json]\n", brand.CLI)
		fmt.Fprintf(stdout, "show the daemon's resolved config (paths, model, env presence)\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s config: unknown subcommand %q (show)\n", brand.CLI, args[0])
		return 2
	}
}

// renderRoutingTable prints a "task → [providers]" table for routes / requires.
func renderRoutingTable(stdout io.Writer, label string, raw any) {
	m, ok := raw.(map[string]any)
	if !ok || len(m) == 0 {
		return
	}
	fmt.Fprintf(stdout, "    %s:\n", label)
	for _, k := range sortedKeys(m) {
		provs, _ := m[k].([]any)
		names := make([]string, 0, len(provs))
		for _, p := range provs {
			if s, ok := p.(string); ok {
				names = append(names, s)
			}
		}
		fmt.Fprintf(stdout, "      %-12s → %v\n", k, names)
	}
}

// sortedKeys returns the keys of a map[string]any sorted for stable output.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func cmdConfigShow(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s config show [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "snapshot of the daemon's effective config — what AGEZT_* env vars,\n")
			fmt.Fprintf(stdout, "model, system prompt (presence only), and on-disk paths it's using.\n")
			fmt.Fprintf(stdout, "  --json   emit the full snapshot (pipe to jq / CI parsers)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s config show: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdConfig, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s config show: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	paths, _ := res["paths"].(map[string]any)
	model, _ := res["model"].(string)
	sysSet, _ := res["system_prompt_set"].(bool)
	askPolicy, _ := res["ask_policy"].(string)
	toolCount := intOfStatus(res["tool_count"])
	pluginCount := intOfStatus(res["plugin_count"])
	envMap, _ := res["env"].(map[string]any)

	fmt.Fprintf(stdout, "%s config:\n", brand.CLI)
	fmt.Fprintf(stdout, "  paths:\n")
	// Stable path key order so consecutive runs look identical.
	for _, k := range []string{"base", "journal", "state", "runtime", "catalog", "vault"} {
		if v, ok := paths[k].(string); ok {
			fmt.Fprintf(stdout, "    %-8s : %s\n", k, v)
		}
	}
	if model == "" {
		fmt.Fprintf(stdout, "  model           : (provider default)\n")
	} else {
		fmt.Fprintf(stdout, "  model           : %s\n", model)
	}
	if sysSet {
		fmt.Fprintf(stdout, "  system prompt   : set (content not shown)\n")
	} else {
		fmt.Fprintf(stdout, "  system prompt   : unset\n")
	}
	fmt.Fprintf(stdout, "  ask_policy      : %s\n", askPolicy)
	fmt.Fprintf(stdout, "  tools           : %d registered\n", toolCount)
	fmt.Fprintf(stdout, "  plugins         : %d spawned\n", pluginCount)

	// Effective routing tables (M108) — only present when configured, so the
	// common no-routing daemon stays compact.
	if routing, ok := res["routing"].(map[string]any); ok && len(routing) > 0 {
		fmt.Fprintf(stdout, "  routing (effective):\n")
		renderRoutingTable(stdout, "routes", routing["routes"])
		renderRoutingTable(stdout, "requires", routing["requires"])
		if ov, ok := routing["model_overrides"].(map[string]any); ok && len(ov) > 0 {
			fmt.Fprintf(stdout, "    model_overrides:\n")
			for _, k := range sortedKeys(ov) {
				if m, _ := ov[k].(string); m != "" {
					fmt.Fprintf(stdout, "      %-12s → %s\n", k, m)
				}
			}
		}
	}

	if len(envMap) == 0 {
		fmt.Fprintf(stdout, "  env (AGEZT_*)   : none set\n")
		return 0
	}
	keys := make([]string, 0, len(envMap))
	for k := range envMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(stdout, "  env (AGEZT_*)   : %d set (values not shown)\n", len(keys))
	for _, k := range keys {
		fmt.Fprintf(stdout, "    %s\n", k)
	}
	return 0
}
