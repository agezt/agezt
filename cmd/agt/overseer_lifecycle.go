// SPDX-License-Identifier: MIT

// Package main: `agt overseer` lifecycle CRUD ops (impact, retire, revive, get, delete).
// Extracted from overseer.go during the Day-211 god-file split.
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
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)
func overseerAgentImpact(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintf(stderr, "%s overseer impact: requires an agent slug\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(context.TODO(), controlplane.CmdAgentImpact, map[string]any{"ref": strings.TrimSpace(args[0])})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintln(stdout, string(b))
	return 0
}

func overseerRetire(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintf(stderr, "%s overseer retire: requires an agent slug\n", brand.CLI)
		return 2
	}
	ref := strings.TrimSpace(args[0])
	reason := strings.Join(args[1:], " ")
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(context.TODO(), controlplane.CmdAgentRetire, map[string]any{"ref": ref, "reason": reason})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	if slug, _ := res["slug"].(string); slug != "" {
		fmt.Fprintf(stdout, "agent %s retired\n", slug)
	}
	return 0
}

func overseerRevive(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintf(stderr, "%s overseer revive: requires an agent slug\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(context.TODO(), controlplane.CmdAgentRevive, map[string]any{"ref": strings.TrimSpace(args[0])})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	if slug, _ := res["slug"].(string); slug != "" {
		fmt.Fprintf(stdout, "agent %s revived\n", slug)
	}
	return 0
}

func overseerGet(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintf(stderr, "%s overseer get: requires an agent slug or id\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(context.TODO(), controlplane.CmdAgentList, map[string]any{"ref": strings.TrimSpace(args[0])})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintln(stdout, string(b))
	return 0
}

func overseerDelete(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintf(stderr, "%s overseer delete: requires an agent slug\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	res, err := c.Call(context.TODO(), controlplane.CmdAgentRemove, map[string]any{"ref": strings.TrimSpace(args[0])})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	if removed, _ := res["removed"].(bool); removed {
		fmt.Fprintf(stdout, "agent %s removed\n", args[0])
	} else {
		fmt.Fprintf(stderr, "%s overseer delete: unknown agent %q\n", brand.CLI, args[0])
		return 1
	}
	return 0
}
