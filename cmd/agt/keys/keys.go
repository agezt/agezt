// SPDX-License-Identifier: MIT

// Package keys is documented in doc.go; this file holds the
// command implementation, lifted from cmd/agt/provider_keys.go
// at the Day-6 refactor. The dispatch (the switch on
// list/add/activate/rm) is now in Run, the public entry. The
// handlers and helpers below are private to this package.
package keys

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// Run is `agt provider keys <subcommand>` — manage several API
// keys per provider and pick the active one (M700). It
// dispatches to one of the four subcommand handlers below and
// is the single entry the main `cmdProvider` switch calls.
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

func addCmd(args []string, stdout, stderr io.Writer) int {
	makeActive := false
	provider := ""
	pos := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--active" {
			makeActive = true
			continue
		}
		if a == "--provider" {
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s provider keys add: --provider needs a value\n", brand.CLI)
				return 2
			}
			i++
			provider = args[i]
			continue
		}
		if strings.HasPrefix(a, "--provider=") {
			provider = strings.TrimPrefix(a, "--provider=")
			continue
		}
		pos = append(pos, a)
	}
	if len(pos) < 2 {
		fmt.Fprintf(stderr, "usage: %s provider keys add [--provider <id>] <ENV> <label> [value] [--active]\n", brand.CLI)
		return 2
	}
	env, label := pos[0], pos[1]
	var value string
	if len(pos) >= 3 {
		value = strings.Join(pos[2:], " ")
	} else {
		fmt.Fprintf(stdout, "value for %s/%s: ", env, label)
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && err != io.EOF {
			fmt.Fprintf(stderr, "%s: read stdin: %v\n", brand.CLI, err)
			return 1
		}
		value = strings.TrimRight(line, "\r\n")
	}
	if strings.TrimSpace(value) == "" {
		fmt.Fprintf(stderr, "%s: value is empty\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	payload := keyRequest(provider, env)
	payload["label"] = label
	payload["value"] = value
	payload["active"] = makeActive
	res, err := c.Call(ctx, controlplane.CmdProviderKeyAdd, payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s provider keys add: %v\n", brand.CLI, err)
		return 1
	}
	if ac, _ := res["active_changed"].(bool); ac {
		fmt.Fprintf(stdout, "added %s/%s and made it active (provider reloaded)\n", targetLabel(provider, env), label)
	} else {
		fmt.Fprintf(stdout, "added %s/%s (activate it with `%s provider keys activate %s%s %s`)\n", targetLabel(provider, env), label, brand.CLI, flagSnippet(provider), env, label)
	}
	return 0
}

func activateCmd(args []string, stdout, stderr io.Writer) int {
	provider, args, ok := providerFlag(args, stderr)
	if !ok {
		return 2
	}
	if len(args) != 2 {
		fmt.Fprintf(stderr, "usage: %s provider keys activate [--provider <id>] <ENV> <label>\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	payload := keyRequest(provider, args[0])
	payload["label"] = args[1]
	res, err := c.Call(ctx, controlplane.CmdProviderKeyActivate, payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s provider keys activate: %v\n", brand.CLI, err)
		return 1
	}
	if re, _ := res["reload_error"].(string); re != "" {
		fmt.Fprintf(stdout, "activated %s/%s, but reload failed: %s\n", targetLabel(provider, args[0]), args[1], re)
		return 0
	}
	fmt.Fprintf(stdout, "activated %s/%s (provider reloaded)\n", targetLabel(provider, args[0]), args[1])
	return 0
}

func removeCmd(args []string, stdout, stderr io.Writer) int {
	provider, args, ok := providerFlag(args, stderr)
	if !ok {
		return 2
	}
	if len(args) != 2 {
		fmt.Fprintf(stderr, "usage: %s provider keys rm [--provider <id>] <ENV> <label>\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	payload := keyRequest(provider, args[0])
	payload["label"] = args[1]
	res, err := c.Call(ctx, controlplane.CmdProviderKeyRemove, payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s provider keys rm: %v\n", brand.CLI, err)
		return 1
	}
	if removed, _ := res["removed"].(bool); !removed {
		fmt.Fprintf(stdout, "%s/%s was not stored\n", targetLabel(provider, args[0]), args[1])
		return 0
	}
	if wa, _ := res["was_active"].(bool); wa {
		fmt.Fprintf(stdout, "removed %s/%s — it was active, so %s is now uncredentialed until you activate another key\n", targetLabel(provider, args[0]), args[1], targetLabel(provider, args[0]))
	} else {
		fmt.Fprintf(stdout, "removed %s/%s\n", targetLabel(provider, args[0]), args[1])
	}
	return 0
}

func providerFlag(args []string, stderr io.Writer) (string, []string, bool) {
	provider := ""
	pos := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--provider" {
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s provider keys: --provider needs a value\n", brand.CLI)
				return "", nil, false
			}
			i++
			provider = args[i]
			continue
		}
		if strings.HasPrefix(a, "--provider=") {
			provider = strings.TrimPrefix(a, "--provider=")
			continue
		}
		pos = append(pos, a)
	}
	return strings.TrimSpace(provider), pos, true
}

func keyRequest(provider, env string) map[string]any {
	out := map[string]any{"env": env}
	if strings.TrimSpace(provider) != "" {
		out["provider"] = strings.TrimSpace(provider)
	}
	return out
}

func targetLabel(provider, env string) string {
	if strings.TrimSpace(provider) == "" {
		return env
	}
	return strings.TrimSpace(provider) + "/" + env
}

func flagSnippet(provider string) string {
	if strings.TrimSpace(provider) == "" {
		return ""
	}
	return "--provider " + strings.TrimSpace(provider) + " "
}
