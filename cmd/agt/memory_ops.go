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
	"strconv"
	"strings"
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
func cmdMemoryConsolidate(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	// Up to a handful of provider calls; generous but bounded.
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMemoryConsolidate, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s memory consolidate: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	found, _ := res["clusters_found"].(float64)
	merged, _ := res["clusters_merged"].(float64)
	superseded, _ := res["records_superseded"].(float64)
	before, _ := res["active_before"].(float64)
	after, _ := res["active_after"].(float64)
	if found == 0 {
		fmt.Fprintf(stdout, "nothing to consolidate — %d active record(s), no cluster of related records found\n", int(before))
		return 0
	}
	fmt.Fprintf(stdout, "consolidated %d of %d cluster(s): %d record(s) merged away (%d → ~%d active) — correlation %s\n",
		int(merged), int(found), int(superseded), int(before), int(after), str(res["correlation_id"]))
	return 0
}

// cmdMemoryProfile implements `agt memory profile [--json]` (M1000): one
// synchronous operator-profile distillation pass on the daemon.
func cmdMemoryProfile(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdProfileRebuild, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s memory profile: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	input, _ := res["input_records"].(float64)
	written, _ := res["facets_written"].(float64)
	if input == 0 {
		fmt.Fprintf(stdout, "nothing to learn from yet — no accumulated memory to build a profile\n")
		return 0
	}
	fmt.Fprintf(stdout, "operator profile rebuilt: %d facet(s) from %d memory record(s) — correlation %s\n",
		int(written), int(input), str(res["correlation_id"]))
	return 0
}

// cmdMemoryPrune implements `agt memory prune [--days N] [--execute].
// Defaults to a dry-run that reports hygiene + prunable count.
// --execute (or --confirm) performs the actual hard-delete of soft-deleted records.
func cmdMemoryPrune(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	days := 0
	dryRun := true
	asJSON := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--days":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory prune: --days needs a value\n", brand.CLI)
				return 2
			}
			i++
			d, err := strconv.Atoi(args[i])
			if err != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s memory prune: --days must be a positive integer\n", brand.CLI)
				return 2
			}
			days = d
		case a == "--execute" || a == "--confirm" || a == "-y":
			dryRun = false
		case a == "--dry-run":
			dryRun = true
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s memory prune [--days N] [--execute|--dry-run] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "hard-delete soft-deleted (tombstoned/superseded) records older than --days.\n")
			fmt.Fprintf(stdout, "defaults: --days=30, --dry-run (safe; use --execute to actually prune)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s memory prune: unknown flag %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	callArgs := map[string]any{"dry_run": dryRun}
	if days > 0 {
		callArgs["older_than_days"] = days
	}
	res, err := c.Call(ctx, controlplane.CmdMemoryPrune, withTenant(tenant, callArgs))
	if err != nil {
		fmt.Fprintf(stderr, "%s memory prune: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	if dryRun {
		stats, _ := res["stats"].(map[string]any)
		prunable, _ := res["prunable"].(float64)
		cutoffMs, _ := res["cutoff_ms"].(float64)
		cutoffDays, _ := res["older_than_days"].(float64)
		fmt.Fprintf(stdout, "dry-run — nothing deleted yet.\n")
		if stats != nil {
			fmt.Fprintf(stdout, "  total records : %d\n", int(stats["total"].(float64)))
			fmt.Fprintf(stdout, "  active        : %d\n", int(stats["active"].(float64)))
			fmt.Fprintf(stdout, "  tombstoned    : %d\n", int(stats["tombstoned"].(float64)))
			fmt.Fprintf(stdout, "  superseded    : %d\n", int(stats["superseded"].(float64)))
		}
		fmt.Fprintf(stdout, "  prunable (>%.0f days old): %.0f\n", cutoffDays, prunable)
		if cutoffMs > 0 {
			t := time.UnixMilli(int64(cutoffMs))
			fmt.Fprintf(stdout, "  cutoff        : %s\n", t.Format(time.RFC3339))
		}
		fmt.Fprintf(stdout, "re-run with --execute to permanently remove %d record(s).\n", int(prunable))
	} else {
		pruned, _ := res["pruned"].(float64)
		cutoffDays, _ := res["older_than_days"].(float64)
		fmt.Fprintf(stdout, "pruned %.0f record(s) older than %.0f days.\n", pruned, cutoffDays)
	}
	return 0
}

// cmdMemoryBulkForget implements `agt memory bulk-forget <id>...`.
// Accepts one or more record IDs and soft-deletes them in a single call.
// Returns: how many were forgotten vs not_found.
func cmdMemoryBulkForget(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	var ids []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s memory bulk-forget <id>... [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "soft-delete multiple memory records in one call.\n")
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s memory bulk-forget: unknown flag %q\n", brand.CLI, a)
			return 2
		default:
			ids = append(ids, a)
		}
	}
	if len(ids) == 0 {
		fmt.Fprintf(stderr, "%s memory bulk-forget: at least one <id> required\n", brand.CLI)
		return 2
	}
	if len(ids) > 500 {
		fmt.Fprintf(stderr, "%s memory bulk-forget: at most 500 IDs per call (got %d)\n", brand.CLI, len(ids))
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	idAnys := make([]any, len(ids))
	for i, id := range ids {
		idAnys[i] = id
	}
	res, err := c.Call(ctx, controlplane.CmdMemoryBulkForget, withTenant(tenant, map[string]any{"ids": idAnys}))
	if err != nil {
		fmt.Fprintf(stderr, "%s memory bulk-forget: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	forgotten, _ := res["forgotten"].(float64)
	notFound, _ := res["not_found"].(float64)
	fmt.Fprintf(stdout, "forgotten: %.0f  not_found: %.0f\n", forgotten, notFound)
	if notFound > 0 && forgotten == 0 {
		return 3
	}
	return 0
}

// cmdMemoryFindRelated implements `agt memory find-related --id <id> [--limit N] [--json]`.
// Uses hybrid (keyword + embedding) search to find records semantically similar to
// the seed record identified by --id. The seed itself is excluded from results.
func cmdMemoryFindRelated(args []string, stdout, stderr io.Writer) int {
	tenant, args := extractTenantFlag(args)
	asJSON := false
	seedID := ""
	limit := 10
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--id":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory find-related: --id needs a value\n", brand.CLI)
				return 2
			}
			i++
			seedID = args[i]
		case a == "--limit":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory find-related: --limit needs a value\n", brand.CLI)
				return 2
			}
			i++
			l, err := strconv.Atoi(args[i])
			if err != nil || l <= 0 {
				fmt.Fprintf(stderr, "%s memory find-related: --limit must be a positive integer\n", brand.CLI)
				return 2
			}
			limit = l
			if limit > 100 {
				limit = 100
			}
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s memory find-related --id <id> [--limit N] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "find records semantically related to the seed record (--id).\n")
			fmt.Fprintf(stdout, "uses hybrid search (keyword + embedding similarity).\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s memory find-related: unknown arg %q (use --id and --limit flags)\n", brand.CLI, a)
			return 2
		}
	}
	if seedID == "" {
		fmt.Fprintf(stderr, "%s memory find-related: --id is required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMemoryFindRelated, withTenant(tenant, map[string]any{"id": seedID, "limit": limit}))
	if err != nil {
		fmt.Fprintf(stderr, "%s memory find-related: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	results, _ := res["results"].([]any)
	count, _ := res["count"].(float64)
	if len(results) == 0 {
		fmt.Fprintf(stdout, "no related records found for id %q\n", seedID)
		return 0
	}
	fmt.Fprintf(stdout, "%.0f related record(s) for seed %q:\n", count, seedID)
	for _, raw := range results {
		r, _ := raw.(map[string]any)
		rec, _ := r["record"].(map[string]any)
		score, _ := r["score"].(float64)
		id, _ := rec["id"].(string)
		subject, _ := rec["subject"].(string)
		fmt.Fprintf(stdout, "  [%.3f] %s  (%s)\n", score, subject, id)
	}
	return 0
}

// encodeJSON was the local pretty-JSON helper. It was replaced
// by cmd/agt/jsonout.Write (Day 5) and removed in Day 7's bulk
// rewrite. New code outside cmd/agt/ that needs pretty JSON
// uses cmd/agt/jsonout.Write directly.
