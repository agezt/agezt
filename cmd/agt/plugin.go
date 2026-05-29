// SPDX-License-Identifier: MIT

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
	"github.com/agezt/agezt/kernel/plugin"
)

// cmdPlugin dispatches `agt plugin <subcommand>`. M1.ff adds one
// subcommand:
//
//	agt plugin hash <path>   — print the BLAKE3-256 hex digest of the
//	                          file at <path>, suitable for use as a
//	                          AGEZT_PLUGIN_PINS entry.
//
// Operators run this once per plugin binary to record the pin, then
// drop the hash into their daemon-startup env:
//
//	AGEZT_PLUGINS="search=/opt/agezt/search"
//	AGEZT_PLUGIN_PINS="search=$(agt plugin hash /opt/agezt/search)"
//	agezt
//
// Subsequent runs verify the binary still matches; any drift (apt
// upgrade replaced it, supply-chain attack swapped it) becomes a
// hard "plugin pin mismatch" error rather than silent execution.
func cmdPlugin(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s plugin: subcommand required (hash|list)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "hash":
		return cmdPluginHash(args[1:], stdout, stderr)
	case "list":
		return cmdPluginList(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s plugin <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "\n")
		fmt.Fprintf(stdout, "  hash <path>      print the BLAKE3-256 hex digest of a plugin binary\n")
		fmt.Fprintf(stdout, "                   (use the output in AGEZT_PLUGIN_PINS=\"<prefix>=<hash>\")\n")
		fmt.Fprintf(stdout, "  list [--json]    list the external plugins the daemon spawned at startup\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s plugin: unknown subcommand %q (hash|list)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdPluginList implements `agt plugin list [--json]`. Operator
// debugging "I configured plugin X but its tools aren't showing"
// — confirms the spawn actually happened (vs failed silently and
// got logged to daemon stderr the operator never sees).
func cmdPluginList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s plugin list [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "list the external plugins the daemon spawned at startup\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s plugin list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdPluginList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s plugin list: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	rows, _ := res["plugins"].([]any)
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "no external plugins loaded (set AGEZT_PLUGINS to configure)")
		return 0
	}
	fmt.Fprintf(stdout, "%d plugin(s):\n", len(rows))
	for _, raw := range rows {
		r, _ := raw.(map[string]any)
		prefix, _ := r["prefix"].(string)
		path, _ := r["path"].(string)
		toolCount := intOfStatus(r["tool_count"])
		hashPinned, _ := r["hash_pinned"].(bool)
		pinBadge := ""
		if hashPinned {
			pinBadge = " [pinned]"
		}
		fmt.Fprintf(stdout, "\n  %s%s\n", prefix, pinBadge)
		fmt.Fprintf(stdout, "    path  : %s\n", path)
		if argsArr, _ := r["args"].([]any); len(argsArr) > 0 {
			parts := make([]string, 0, len(argsArr))
			for _, a := range argsArr {
				if s, ok := a.(string); ok {
					parts = append(parts, s)
				}
			}
			fmt.Fprintf(stdout, "    args  : %s\n", strings.Join(parts, " "))
		}
		fmt.Fprintf(stdout, "    tools : %d registered\n", toolCount)
		// allowed_tools is JSON null when no allowlist applies —
		// surface "unrestricted" so operators don't confuse "no
		// allowlist set" with "empty allowlist locking it down".
		allowed, isArr := r["allowed_tools"].([]any)
		if !isArr || allowed == nil {
			fmt.Fprintln(stdout, "    allow : unrestricted")
		} else {
			parts := make([]string, 0, len(allowed))
			for _, a := range allowed {
				if s, ok := a.(string); ok {
					parts = append(parts, s)
				}
			}
			fmt.Fprintf(stdout, "    allow : %s\n", strings.Join(parts, ", "))
		}
	}
	return 0
}

func cmdPluginHash(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "%s plugin hash: exactly one path argument required\n", brand.CLI)
		return 2
	}
	sum, err := plugin.HashFile(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "%s plugin hash: %v\n", brand.CLI, err)
		return 1
	}
	// Print just the digest on stdout (no path prefix) so the output
	// is directly substitutable in `AGEZT_PLUGIN_PINS=name=$(...)`.
	// Most operators run this in a $() — the cleaner the stdout, the
	// less they have to post-process.
	fmt.Fprintln(stdout, sum)
	return 0
}
