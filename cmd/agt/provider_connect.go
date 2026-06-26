// SPDX-License-Identifier: MIT

package main

// `agt provider connect` — the CLI twin of the Web UI "Quick Connect" gallery.
// Registers a provider in the catalog's custom layer (pinning its base URL),
// optionally stores its API key, and optionally makes it the default brain — in
// one command, applied live (no restart). Keyless local runtimes (Ollama, …)
// connect with no --env/--key at all.

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func connectUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: %s provider connect <id> --url <base> --model <model> [flags]\n\n", brand.CLI)
	fmt.Fprintf(w, "  --url <base>     OpenAI/Anthropic-compatible base URL (required), e.g. https://api.deepseek.com\n")
	fmt.Fprintf(w, "  --model <id>     default model id (required), e.g. deepseek-chat\n")
	fmt.Fprintf(w, "  --name <name>    display name (default: the id)\n")
	fmt.Fprintf(w, "  --npm <hint>     wire family: @ai-sdk/openai-compatible (default) or @ai-sdk/anthropic\n")
	fmt.Fprintf(w, "  --env <ENV>      key env var, e.g. DEEPSEEK_API_KEY (omit for a keyless local runtime)\n")
	fmt.Fprintf(w, "  --key <value>    API key to store (needs --env)\n")
	fmt.Fprintf(w, "  --default        also make this the daemon's default provider + model\n\n")
	fmt.Fprintf(w, "examples:\n")
	fmt.Fprintf(w, "  %s provider connect deepseek --url https://api.deepseek.com --model deepseek-chat --env DEEPSEEK_API_KEY --key sk-… --default\n", brand.CLI)
	fmt.Fprintf(w, "  %s provider connect ollama --url http://localhost:11434/v1 --model llama3.2\n", brand.CLI)
}

func cmdProviderConnect(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		connectUsage(stdout)
		return map[bool]int{true: 0, false: 2}[len(args) > 0]
	}
	var id, url, model, name, npm, env, key string
	var setDefault bool
	var pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--default":
			setDefault = true
		case a == "-h" || a == "--help":
			connectUsage(stdout)
			return 0
		case strings.HasPrefix(a, "--"):
			flag, val, hasEq := strings.Cut(a[2:], "=")
			if !hasEq {
				if i+1 >= len(args) {
					fmt.Fprintf(stderr, "%s provider connect: --%s needs a value\n", brand.CLI, flag)
					return 2
				}
				i++
				val = args[i]
			}
			switch flag {
			case "url":
				url = val
			case "model":
				model = val
			case "name":
				name = val
			case "npm":
				npm = val
			case "env":
				env = val
			case "key":
				key = val
			default:
				fmt.Fprintf(stderr, "%s provider connect: unknown flag --%s\n", brand.CLI, flag)
				return 2
			}
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) != 1 {
		connectUsage(stderr)
		return 2
	}
	id = pos[0]
	if strings.TrimSpace(url) == "" || strings.TrimSpace(model) == "" {
		fmt.Fprintf(stderr, "%s provider connect: --url and --model are required\n", brand.CLI)
		return 2
	}
	if key != "" && env == "" {
		fmt.Fprintf(stderr, "%s provider connect: --key needs --env <ENV> to store it under\n", brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Register the provider (custom.json) + reload.
	res, err := c.Call(ctx, controlplane.CmdProviderConnect, map[string]any{
		"id": id, "name": name, "npm": npm, "api": url, "env": env, "model": model,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s provider connect: %v\n", brand.CLI, err)
		return 1
	}
	if added, _ := res["added"].(bool); added {
		fmt.Fprintf(stdout, "registered provider %q (%s, model %s)\n", id, url, model)
	} else {
		fmt.Fprintf(stdout, "updated provider %q\n", id)
	}

	// 2. Store the key (secret path), if given.
	if key != "" {
		if _, err := c.Call(ctx, controlplane.CmdProviderKeyAdd, map[string]any{"provider": id, "env": env, "label": "default", "value": key, "active": true}); err != nil {
			fmt.Fprintf(stderr, "%s: provider saved, but storing the key failed: %v\n", brand.CLI, err)
			return 1
		}
		fmt.Fprintf(stdout, "stored key in %s (active)\n", env)
	} else if env != "" {
		fmt.Fprintf(stdout, "add a key with `%s provider keys add --provider %s %s default <value> --active`\n", brand.CLI, id, env)
	}

	// 3. Make it the default brain, if asked.
	if setDefault {
		for _, kv := range []struct{ name, value string }{
			{brand.EnvPrefix + "PROVIDER", id},
			{brand.EnvPrefix + "MODEL", model},
		} {
			if _, err := c.Call(ctx, controlplane.CmdConfigSet, map[string]any{"name": kv.name, "value": kv.value}); err != nil {
				fmt.Fprintf(stderr, "%s: provider saved, but setting %s failed: %v\n", brand.CLI, kv.name, err)
				return 1
			}
		}
		_, _ = c.Call(ctx, controlplane.CmdProviderReload, nil)
		fmt.Fprintf(stdout, "set as the default brain (provider=%s, model=%s)\n", id, model)
	}
	return 0
}
