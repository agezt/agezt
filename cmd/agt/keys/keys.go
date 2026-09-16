// SPDX-License-Identifier: MIT
//
// cmd/agt provider-keys top-level dispatch (Run) + list subcommand (listCmd).
// Extracted from keys.go during Day 211 god-file refactor (#73).
// Public API unchanged.
package keys

import (
	"context"
	"fmt"
	"io"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s provider keys: subcommand required (list, add, activate, rm)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "list", "ls":
		return listCmd(args[1:], stdout, stderr)
	case "add", "set":
		return addCmd(args[1:], stdout, stderr)
	case "activate", "use":
		return activateCmd(args[1:], stdout, stderr)
	case "rm", "remove", "del", "delete":
		return removeCmd(args[1:], stdout, stderr)
	case "-h", "--help":
		fmt.Fprintf(stdout, "usage: %s provider keys <subcommand>\n\n", brand.CLI)
		fmt.Fprintf(stdout, "  list [--provider <id>] <ENV>                       list keys (label + active + last-4)\n")
		fmt.Fprintf(stdout, "  add [--provider <id>] <ENV> <label> [value] [--active]   add a key (prompts if value omitted)\n")
		fmt.Fprintf(stdout, "  activate [--provider <id>] <ENV> <label>           make a key active (reloads the provider)\n")
		fmt.Fprintf(stdout, "  rm [--provider <id>] <ENV> <label>                 remove a key\n\n")
		fmt.Fprintf(stdout, "ENV is the provider's key env var. --provider scopes storage to one catalog provider; without it, the legacy global env keyring is used.\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s provider keys: unknown subcommand %q (list, add, activate, rm)\n", brand.CLI, args[0])
		return 2
	}
}
func listCmd(args []string, stdout, stderr io.Writer) int {
	provider, args, ok := providerFlag(args, stderr)
	if !ok {
		return 2
	}
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s provider keys list [--provider <id>] <ENV>\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdProviderKeyList, keyRequest(provider, args[0]))
	if err != nil {
		fmt.Fprintf(stderr, "%s provider keys list: %v\n", brand.CLI, err)
		return 1
	}
	stored, _ := res["keys"].([]any)
	target := targetLabel(provider, args[0])
	if len(stored) == 0 {
		fmt.Fprintf(stdout, "no keys stored for %s\n", target)
		fmt.Fprintf(stdout, "add one with `%s provider keys add %s%s <label> <value>`\n", brand.CLI, flagSnippet(provider), args[0])
		return 0
	}
	fmt.Fprintf(stdout, "keys for %s:\n", target)
	for _, ki := range stored {
		m, _ := ki.(map[string]any)
		label, _ := m["label"].(string)
		last4, _ := m["last4"].(string)
		active, _ := m["active"].(bool)
		marker := "  "
		if active {
			marker = "* "
		}
		suffix := ""
		if active {
			suffix = "  (active)"
		}
		fmt.Fprintf(stdout, "%s%-16s %s%s\n", marker, label, last4, suffix)
	}
	return 0
}
