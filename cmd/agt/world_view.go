// SPDX-License-Identifier: MIT

// agt world view helpers: renderEntityLine + cmdWorldAudit (audit subcommand + entity-line formatter).
// Code extracted from world.go during the Day-98 god-file split.
// Public API unchanged.
package main


import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func renderEntityLine(e map[string]any) string {
	id, _ := e["id"].(string)
	if len(id) > 12 {
		id = id[:12]
	}
	kind, _ := e["kind"].(string)
	name, _ := e["name"].(string)
	line := fmt.Sprintf("  %s [%s] %s", id, kind, name)
	if aliases, ok := e["aliases"].([]any); ok && len(aliases) > 0 {
		parts := make([]string, 0, len(aliases))
		for _, a := range aliases {
			if sv, ok := a.(string); ok {
				parts = append(parts, sv)
			}
		}
		line += " (aka " + strings.Join(parts, ", ") + ")"
	}
	return line
}

func cmdWorldAudit(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "--tenant":
			// handled by extractTenantFlag at the caller, skip
		default:
			if !strings.HasPrefix(a, "--") {
				fmt.Fprintf(stderr, "%s world audit: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdWorldList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s world audit: %v\n", brand.CLI, err)
		return 1
	}
	entities, _ := res["entities"].([]any)
	relations, _ := res["relations"].([]any)

	var total, untyped, decayed, superseded int
	kindCounts := map[string]int{}
	now := time.Now().UnixMilli()
	staleThreshold := int64(30 * 24 * 3600 * 1000) // 30 days in ms

	for _, raw := range entities {
		e, _ := raw.(map[string]any)
		if e == nil {
			continue
		}
		total++
		kind := str(e["kind"])
		if kind == "" || kind == "unknown" {
			untyped++
		} else {
			kindCounts[kind]++
		}
		if sup := str(e["superseded_by"]); sup != "" {
			superseded++
		}
		lastSeen := intNumber(e["last_seen_ms"])
		if lastSeen > 0 && now-int64(lastSeen) > staleThreshold {
			decayed++
		}
	}

	audit := map[string]any{
		"entities":          total,
		"relations":         len(relations),
		"untyped":           untyped,
		"decayed_30d":       decayed,
		"superseded":        superseded,
		"kind_distribution": kindCounts,
	}

	if asJSON {
		return jsonout.Write(stdout, audit)
	}

	fmt.Fprintf(stdout, "world audit:\n")
	fmt.Fprintf(stdout, "  entities:      %d\n", total)
	fmt.Fprintf(stdout, "  relations:     %d\n", len(relations))
	fmt.Fprintf(stdout, "  untyped:       %d\n", untyped)
	fmt.Fprintf(stdout, "  decayed (30d): %d\n", decayed)
	fmt.Fprintf(stdout, "  superseded:    %d\n", superseded)
	if len(kindCounts) > 0 {
		fmt.Fprintf(stdout, "\nkind distribution:\n")
		for k, n := range kindCounts {
			fmt.Fprintf(stdout, "  %-15s %d\n", k, n)
		}
	}
	if total == 0 {
		fmt.Fprintf(stdout, "\nworld model is empty — no entities tracked yet.\n")
	} else if untyped == 0 && decayed == 0 {
		fmt.Fprintf(stdout, "\nworld model looks healthy — all entities are typed and recently seen.\n")
	}
	return 0
}
