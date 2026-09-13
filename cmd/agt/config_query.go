// SPDX-License-Identifier: MIT

// Package main: `agt config ls` + `agt config get` read-only queries.
// Extracted from config.go during the Day-211 god-file split. Public API
// unchanged.
package main


import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)
func cmdConfigLs(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s config ls [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s config ls: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	if asJSON {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, err := c.Call(ctx, controlplane.CmdConfigValues, nil)
		if err != nil {
			fmt.Fprintf(stderr, "%s config ls: %v\n", brand.CLI, err)
			return 1
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	fields, ok := configValues(c, stderr, "config ls")
	if !ok {
		return 1
	}
	fmt.Fprintf(stdout, "%s config (%d settings):\n", brand.CLI, len(fields))
	for _, fi := range fields {
		m, _ := fi.(map[string]any)
		env, _ := m["env"].(string)
		secret, _ := m["secret"].(bool)
		set, _ := m["set"].(bool)
		pinned, _ := m["env_pinned"].(bool)
		state := "unset"
		if secret {
			if set {
				state = "set (secret)"
			} else {
				state = "unset (secret)"
			}
		} else if val, _ := m["value"].(string); val != "" {
			state = val
		}
		tag := ""
		if pinned {
			tag = " [env-pinned]"
		}
		fmt.Fprintf(stdout, "  %-34s %s%s\n", env, state, tag)
	}
	return 0
}
func cmdConfigGet(args []string, stdout, stderr io.Writer) int {
	var env string
	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s config get <ENV>\n", brand.CLI)
			return 0
		default:
			if env != "" {
				fmt.Fprintf(stderr, "%s config get: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
			env = a
		}
	}
	if env == "" {
		fmt.Fprintf(stderr, "%s config get: ENV required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	fields, ok := configValues(c, stderr, "config get")
	if !ok {
		return 1
	}
	for _, fi := range fields {
		m, _ := fi.(map[string]any)
		if m["env"] != env {
			continue
		}
		secret, _ := m["secret"].(bool)
		set, _ := m["set"].(bool)
		if secret {
			if set {
				fmt.Fprintf(stdout, "%s: set (secret, value not shown)\n", env)
			} else {
				fmt.Fprintf(stdout, "%s: not set\n", env)
			}
		} else {
			val, _ := m["value"].(string)
			fmt.Fprintf(stdout, "%s=%s\n", env, val)
		}
		return 0
	}
	fmt.Fprintf(stderr, "%s config get: unknown setting %q\n", brand.CLI, env)
	return 1
}
