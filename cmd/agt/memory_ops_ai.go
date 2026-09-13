// SPDX-License-Identifier: MIT

package main

// agt memory advanced-maintenance subcommands: cmdMemoryConsolidate +
// cmdMemoryProfile + cmdMemoryPrune + cmdMemoryBulkForget +
// cmdMemoryFindRelated. Carved out of memory_ops.go during the Day 176
// god-file split so the main file can stay focused on Forget/Promote/
// Audit/Clean + small render helpers.
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
