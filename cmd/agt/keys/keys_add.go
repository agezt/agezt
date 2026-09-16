// SPDX-License-Identifier: MIT
//
// cmd/agt provider-keys CRUD handlers (addCmd, activateCmd, removeCmd).
// Extracted from keys.go during Day 211 god-file refactor (#73).
// Public API unchanged.
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
