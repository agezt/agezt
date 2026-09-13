// SPDX-License-Identifier: MIT

// Package main: `agt overseer bulk` batch operations + jsonOrString helper.
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
func overseerBulk(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintf(stderr, "%s overseer bulk: requires an action (pause|unpause|retire|revive|delete) and comma-separated slugs\n", brand.CLI)
		return 2
	}
	action := strings.TrimSpace(args[0])
	slugs := strings.Split(strings.TrimSpace(args[1]), ",")
	if len(slugs) == 0 || (len(slugs) == 1 && slugs[0] == "") {
		fmt.Fprintf(stderr, "%s overseer bulk: requires at least one slug\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	var cpCmd string
	var payload map[string]any
	switch action {
	case "pause":
		cpCmd = controlplane.CmdAgentSetEnabled
		payload = map[string]any{"refs": slugs, "enabled": false}
	case "unpause":
		cpCmd = controlplane.CmdAgentSetEnabled
		payload = map[string]any{"refs": slugs, "enabled": true}
	case "retire":
		cpCmd = controlplane.CmdAgentRetire
		payload = map[string]any{"refs": slugs}
	case "revive":
		cpCmd = controlplane.CmdAgentRevive
		payload = map[string]any{"refs": slugs}
	case "delete", "rm":
		cpCmd = controlplane.CmdAgentRemove
		payload = map[string]any{"refs": slugs}
	default:
		fmt.Fprintf(stderr, "%s overseer bulk: unknown action %q (pause|unpause|retire|revive|delete)\n", brand.CLI, action)
		return 2
	}
	res, err := c.Call(context.TODO(), cpCmd, payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintln(stdout, string(b))
	return 0
}

func jsonOrString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
