// SPDX-License-Identifier: MIT

package main

// `agt provider keys` — manage several API keys per provider and pick the active
// one (M700). Everything goes through the daemon's control plane so adding /
// activating / removing a key reloads the provider in place. Values are never
// printed back; list shows a last-4 fingerprint only.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdProviderKeys(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s provider keys: subcommand required (list, add, activate, rm)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "list", "ls":
		return cmdProviderKeysList(args[1:], stdout, stderr)
	case "add", "set":
		return cmdProviderKeysAdd(args[1:], stdout, stderr)
	case "activate", "use":
		return cmdProviderKeysActivate(args[1:], stdout, stderr)
	case "rm", "remove", "del", "delete":
		return cmdProviderKeysRemove(args[1:], stdout, stderr)
	case "-h", "--help":
		fmt.Fprintf(stdout, "usage: %s provider keys <subcommand>\n\n", brand.CLI)
		fmt.Fprintf(stdout, "  list <ENV>                       list keys for a provider (label + active + last-4)\n")
		fmt.Fprintf(stdout, "  add <ENV> <label> [value] [--active]   add a key (prompts if value omitted)\n")
		fmt.Fprintf(stdout, "  activate <ENV> <label>           make a key the active one (reloads the provider)\n")
		fmt.Fprintf(stdout, "  rm <ENV> <label>                 remove a key\n\n")
		fmt.Fprintf(stdout, "ENV is the provider's key env var, e.g. OPENAI_API_KEY, ANTHROPIC_API_KEY.\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s provider keys: unknown subcommand %q (list, add, activate, rm)\n", brand.CLI, args[0])
		return 2
	}
}

func cmdProviderKeysList(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: %s provider keys list <ENV>\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdProviderKeyList, map[string]any{"env": args[0]})
	if err != nil {
		fmt.Fprintf(stderr, "%s provider keys list: %v\n", brand.CLI, err)
		return 1
	}
	keys, _ := res["keys"].([]any)
	if len(keys) == 0 {
		fmt.Fprintf(stdout, "no keys stored for %s\n", args[0])
		fmt.Fprintf(stdout, "add one with `%s provider keys add %s <label> <value>`\n", brand.CLI, args[0])
		return 0
	}
	fmt.Fprintf(stdout, "keys for %s:\n", args[0])
	for _, ki := range keys {
		m, _ := ki.(map[string]any)
		label, _ := m["label"].(string)
		last4, _ := m["last4"].(string)
		active, _ := m["active"].(bool)
		marker := "  "
		if active {
			marker = "* "
		}
		fmt.Fprintf(stdout, "%s%-16s %s%s\n", marker, label, last4, map[bool]string{true: "  (active)", false: ""}[active])
	}
	return 0
}

func cmdProviderKeysAdd(args []string, stdout, stderr io.Writer) int {
	makeActive := false
	pos := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--active" {
			makeActive = true
			continue
		}
		pos = append(pos, a)
	}
	if len(pos) < 2 {
		fmt.Fprintf(stderr, "usage: %s provider keys add <ENV> <label> [value] [--active]\n", brand.CLI)
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
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdProviderKeyAdd, map[string]any{"env": env, "label": label, "value": value, "active": makeActive})
	if err != nil {
		fmt.Fprintf(stderr, "%s provider keys add: %v\n", brand.CLI, err)
		return 1
	}
	if ac, _ := res["active_changed"].(bool); ac {
		fmt.Fprintf(stdout, "added %s/%s and made it active (provider reloaded)\n", env, label)
	} else {
		fmt.Fprintf(stdout, "added %s/%s (activate it with `%s provider keys activate %s %s`)\n", env, label, brand.CLI, env, label)
	}
	return 0
}

func cmdProviderKeysActivate(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintf(stderr, "usage: %s provider keys activate <ENV> <label>\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdProviderKeyActivate, map[string]any{"env": args[0], "label": args[1]})
	if err != nil {
		fmt.Fprintf(stderr, "%s provider keys activate: %v\n", brand.CLI, err)
		return 1
	}
	if re, _ := res["reload_error"].(string); re != "" {
		fmt.Fprintf(stdout, "activated %s/%s, but reload failed: %s\n", args[0], args[1], re)
		return 0
	}
	fmt.Fprintf(stdout, "activated %s/%s (provider reloaded)\n", args[0], args[1])
	return 0
}

func cmdProviderKeysRemove(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintf(stderr, "usage: %s provider keys rm <ENV> <label>\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdProviderKeyRemove, map[string]any{"env": args[0], "label": args[1]})
	if err != nil {
		fmt.Fprintf(stderr, "%s provider keys rm: %v\n", brand.CLI, err)
		return 1
	}
	if removed, _ := res["removed"].(bool); !removed {
		fmt.Fprintf(stdout, "%s/%s was not stored\n", args[0], args[1])
		return 0
	}
	if wa, _ := res["was_active"].(bool); wa {
		fmt.Fprintf(stdout, "removed %s/%s — it was active, so %s is now uncredentialed until you activate another key\n", args[0], args[1], args[0])
	} else {
		fmt.Fprintf(stdout, "removed %s/%s\n", args[0], args[1])
	}
	return 0
}
