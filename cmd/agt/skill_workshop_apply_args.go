// SPDX-License-Identifier: MIT

package main

// agt skill workshop arg-parsing + data helpers: parseWorkshopIDJSON +
// parseWorkshopReasonArgs + workshopFetchSkill + workshopProposals +
// workshopCanReject. Carved out of skill_workshop_apply.go during the
// Day 184 god-file split so the apply/main file can stay focused on
// the 5 cmd entry points + transition dispatch.
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func parseWorkshopIDJSON(args []string, cmd string, stdout, stderr io.Writer) (string, bool, bool) {
	asJSON := false
	id := ""
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill workshop %s <id> [--json]\n", brand.CLI, cmd)
			return "", false, false
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s skill workshop %s: unexpected flag %q\n", brand.CLI, cmd, a)
			return "", false, false
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s skill workshop %s: unexpected arg %q\n", brand.CLI, cmd, a)
			return "", false, false
		}
	}
	return id, asJSON, true
}

func parseWorkshopReasonArgs(args []string, cmd, defaultReason string, stdout, stderr io.Writer) (string, string, bool, bool) {
	asJSON := false
	reason := defaultReason
	id := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill workshop %s <id> [--reason R] [--json]\n", brand.CLI, cmd)
			return "", "", false, false
		case a == "--reason":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s skill workshop %s: --reason needs a value\n", brand.CLI, cmd)
				return "", "", false, false
			}
			i++
			reason = args[i]
		case strings.HasPrefix(a, "--reason="):
			reason = strings.TrimPrefix(a, "--reason=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s skill workshop %s: unexpected flag %q\n", brand.CLI, cmd, a)
			return "", "", false, false
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s skill workshop %s: unexpected arg %q\n", brand.CLI, cmd, a)
			return "", "", false, false
		}
	}
	return id, reason, asJSON, true
}

func workshopFetchSkill(ctx context.Context, c *controlplane.Client, id string) (map[string]any, bool, error) {
	res, err := c.Call(ctx, controlplane.CmdSkillGet, map[string]any{"id": id})
	if err != nil {
		return nil, false, err
	}
	found, _ := res["found"].(bool)
	if !found {
		return nil, false, nil
	}
	sk, _ := res["skill"].(map[string]any)
	return sk, sk != nil, nil
}

func workshopProposals(raw any) []map[string]any {
	var out []map[string]any
	items, _ := raw.([]any)
	for _, item := range items {
		sk, _ := item.(map[string]any)
		if sk == nil {
			continue
		}
		switch str(sk["status"]) {
		case "draft", "shadow":
			out = append(out, sk)
		}
	}
	return out
}

func workshopCanReject(status string) bool {
	return status == "draft" || status == "shadow"
}


