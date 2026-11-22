// SPDX-License-Identifier: MIT

package main

// Provenance: Package main: `agt config` dispatcher + sortedKeys helper +
//             cmdConfigSet (the write op). Snapshot display (renderRoutingTable +
//             cmdConfigShow + configValues) moved to config_show.go; read-only
//             queries (cmdConfigLs + cmdConfigGet) moved to config_query.go. Day-211
//             god-file split. Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func cmdConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s config: subcommand required %s\n", brand.CLI, cliConfig.listNames())
		return 2
	}
	if sub, ok := cliConfig.lookup(args[0]); ok {
		return sub.run(nil, args, stdout, stderr)
	}
	fmt.Fprintf(stderr, "%s config: unknown subcommand %q %s\n", brand.CLI, args[0], cliConfig.listNames())
	return 2
}
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func cmdConfigSet(args []string, stdout, stderr io.Writer) int {
	if len(args) >= 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintf(stdout, "usage: %s config set <ENV> [value]\n", brand.CLI)
		fmt.Fprintf(stdout, "  omit value (or pass \"\") to clear the setting\n")
		return 0
	}
	if len(args) < 1 {
		fmt.Fprintf(stderr, "%s config set: ENV required (usage: config set <ENV> [value])\n", brand.CLI)
		return 2
	}
	env := args[0]
	value := strings.Join(args[1:], " ")
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, cerr := saveConfigSettingRollbackCheckpoint(ctx, c, "config.set", env); cerr != nil {
		fmt.Fprintf(stderr, "%s config set: checkpoint: %v\n", brand.CLI, cerr)
		return 1
	}
	res, err := c.Call(ctx, opapi.ConfigSet, map[string]any{"name": env, "value": value})
	if err != nil {
		fmt.Fprintf(stderr, "%s config set: %v\n", brand.CLI, err)
		return 1
	}
	if pinned, _ := res["env_pinned"].(bool); pinned {
		fmt.Fprintf(stdout, "%s saved, but pinned by the environment — unset it in .env to apply\n", env)
		return 0
	}
	switch applied, _ := res["applied"].(string); applied {
	case "live":
		fmt.Fprintf(stdout, "%s applied live\n", env)
	default:
		fmt.Fprintf(stdout, "%s saved — restart to apply\n", env)
	}
	return 0
}
