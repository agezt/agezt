// SPDX-License-Identifier: MIT

package main

// agt memory lifecycle / maintenance subcommands: cmdMemoryForget,
// cmdMemoryPromote, cmdMemoryAudit, cmdMemoryClean, cmdMemoryConsolidate,
// cmdMemoryProfile, cmdMemoryPrune, cmdMemoryBulkForget,
// cmdMemoryFindRelated, plus the small helpers (num, lenSlice,
// renderRecordLine). Carved out of memory.go during the Day 158 god-file
// split so the main file can focus on the dispatcher + new-memory write +
// read-side subcommands.
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)
func cmdMemoryForget(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var id string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s memory forget <id> [--json]\n", brand.CLI)
			return 0
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s memory forget: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s memory forget: id required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMemoryForget, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s memory forget: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	if ok, _ := res["forgotten"].(bool); ok {
		fmt.Fprintf(stdout, "forgot %s\n", id)
	} else {
		fmt.Fprintf(stdout, "no such record %s\n", id)
	}
	return 0
}

// cmdMemoryPromote implements `agt memory promote <id> [--json]` (M915):
// share a private (agent-scoped) record with every agent — the selective-
// sharing valve over per-agent memory.
func cmdMemoryPromote(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var id string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s memory promote <id> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "clears the record's private scope so every agent recalls it\n")
			return 0
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s memory promote: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s memory promote: id required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMemoryPromote, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s memory promote: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	if ok, _ := res["promoted"].(bool); ok {
		fmt.Fprintf(stdout, "promoted %s to shared memory\n", id)
	} else {
		fmt.Fprintf(stdout, "no such record %s\n", id)
	}
	return 0
}

func cmdMemoryAudit(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s memory audit [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "report memory records excluded by expiration/suspension and same-topic competing claims\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s memory audit: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMemoryAudit, withTenant(tenant, nil))
	if err != nil {
		fmt.Fprintf(stderr, "%s memory audit: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	fmt.Fprintf(stdout, "memory audit:\n")
	fmt.Fprintf(stdout, "  usable       : %d\n", int(num(res["usable"])))
	fmt.Fprintf(stdout, "  expired      : %d\n", int(num(res["expired"])))
	fmt.Fprintf(stdout, "  suspended    : %d\n", int(num(res["suspended"])))
	fmt.Fprintf(stdout, "  conflict load: %d\n", int(num(res["contradiction_load"])))
	if groups, _ := res["contradictions"].([]any); len(groups) > 0 {
		fmt.Fprintf(stdout, "  contradictions:\n")
		for _, raw := range groups {
			g, _ := raw.(map[string]any)
			fmt.Fprintf(stdout, "    - %s (%d records)\n", str(g["key"]), lenSlice(g["ids"]))
		}
	}
	return 0
}

func cmdMemoryClean(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	dryRun := true
	asJSON := false
	for _, a := range args {
		switch a {
		case "--execute", "--confirm", "-y":
			dryRun = false
		case "--dry-run":
			dryRun = true
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s memory clean [--tenant <id>] [--execute|--dry-run] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "hard-delete records that look like logs, transient notes, or automatic low-value memories\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s memory clean: unknown flag %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMemoryClean, withTenant(tenant, map[string]any{"dry_run": dryRun}))
	if err != nil {
		fmt.Fprintf(stderr, "%s memory clean: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	scanned := int(num(res["scanned"]))
	rejected := int(num(res["rejected"]))
	removed := int(num(res["removed"]))
	if dryRun {
		fmt.Fprintf(stdout, "dry-run — scanned %d usable record(s), %d look low-value.\n", scanned, rejected)
		if rejected > 0 {
			fmt.Fprintf(stdout, "re-run with --execute to permanently delete them.\n")
		}
		return 0
	}
	fmt.Fprintf(stdout, "cleaned memory: scanned %d, permanently deleted %d low-value record(s).\n", scanned, removed)
	return 0
}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}

func lenSlice(v any) int {
	if xs, ok := v.([]any); ok {
		return len(xs)
	}
	return 0
}

// renderRecordLine formats a record map (as returned over the wire) into a
// single human-readable line: "<id12> [TYPE] subject: content".
func renderRecordLine(r map[string]any) string {
	id, _ := r["id"].(string)
	if len(id) > 12 {
		id = id[:12]
	}
	typ, _ := r["type"].(string)
	subject, _ := r["subject"].(string)
	content, _ := r["content"].(string)
	prefix := id
	if subject != "" {
		return fmt.Sprintf("  %s [%s] %s: %s", prefix, typ, subject, content)
	}
	return fmt.Sprintf("  %s [%s] %s", prefix, typ, content)
}

// cmdMemoryConsolidate implements `agt memory consolidate [--json]` (M804):
// one synchronous brain-distillation pass on the daemon.
