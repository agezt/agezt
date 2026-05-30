// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdCatalogSync implements `agt catalog sync [url] [--local] [--json]`.
//
// Two paths, picked automatically so it "just works" whether or not the
// daemon is up:
//
//   - **Daemon path (default when reachable):** the daemon fetches, persists,
//     and hot-reloads its in-memory catalog in place — no restart needed.
//   - **Local path (offline):** when the daemon isn't running (or `--local` is
//     given), the CLI fetches models.dev itself and writes the catalog store
//     directly. A later daemon start picks it up; a running one needs
//     `agt provider reload`.
//
// This is the "easy offline sync" surface: you can populate the catalog before
// ever starting the daemon, then add keys with `agt provider setup`.
func cmdCatalogSync(args []string, stdout, stderr io.Writer) int {
	var url string
	local, asJSON := false, false
	for _, a := range args {
		switch {
		case a == "--local" || a == "--offline":
			local = true
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s catalog sync [url] [--local] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "sync the provider/model catalog from models.dev\n")
			fmt.Fprintf(stdout, "  --local   fetch + write client-side without the daemon (works offline)\n")
			fmt.Fprintf(stdout, "  (without --local, uses the daemon when running, else falls back to local)\n")
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s catalog sync: unknown flag %q\n", brand.CLI, a)
			return 2
		default:
			if url != "" {
				fmt.Fprintf(stderr, "%s catalog sync: unexpected extra arg %q\n", brand.CLI, a)
				return 2
			}
			url = a
		}
	}

	base, err := paths.BaseDir()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}

	// Try the daemon first unless forced local. If it's reachable, it syncs
	// and hot-reloads in place — the ideal outcome.
	if !local {
		if c, derr := controlplane.NewClient(base); derr == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			callArgs := map[string]any{}
			if url != "" {
				callArgs["url"] = url
			}
			res, cerr := c.Call(ctx, controlplane.CmdCatalogSync, callArgs)
			cancel()
			if cerr == nil {
				return renderSyncResult(res, "daemon (reloaded in place)", asJSON, stdout)
			}
			// Daemon present but the call failed (not running, network, …).
			// Fall through to a local sync so the operator still gets a result.
			fmt.Fprintf(stderr, "note: daemon sync failed (%v); falling back to local sync\n", cerr)
		}
	}

	// Local path: fetch + write the store directly.
	store := catalog.NewStore(base + "/catalog")
	syncer := catalog.NewSyncer()
	if url == "" {
		url = envOr(brand.EnvPrefix+"CATALOG_URL", catalog.DefaultSyncURL)
	}
	syncer.URL = url

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, _, res, serr := syncer.Sync(ctx)
	if serr != nil {
		fmt.Fprintf(stderr, "%s catalog sync: %v\n", brand.CLI, serr)
		return 1
	}
	if werr := store.SaveAPI(raw, url); werr != nil {
		fmt.Fprintf(stderr, "%s catalog sync: save: %v\n", brand.CLI, werr)
		return 1
	}

	out := map[string]any{
		"url":            url,
		"bytes":          res.Bytes,
		"provider_count": res.ProviderCount,
		"model_count":    res.ModelCount,
		"duration_ms":    res.Duration.Milliseconds(),
		"mode":           "local",
	}
	// If a daemon is running, its in-memory catalog is now stale — nudge.
	if _, derr := controlplane.NewClient(base); derr == nil && !local {
		out["note"] = "a daemon is running; run `agt provider reload` to pick this up"
	}
	return renderSyncResult(out, "local (offline)", asJSON, stdout)
}

// renderSyncResult prints a sync result map (from either path) consistently.
func renderSyncResult(res map[string]any, mode string, asJSON bool, stdout io.Writer) int {
	if asJSON {
		if _, ok := res["mode"]; !ok {
			res["mode"] = mode
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	fmt.Fprintf(stdout, "catalog synced [%s]\n", mode)
	fmt.Fprintf(stdout, "  source   : %v\n", res["url"])
	fmt.Fprintf(stdout, "  providers: %v\n", intOfStatus(res["provider_count"]))
	fmt.Fprintf(stdout, "  models   : %v\n", intOfStatus(res["model_count"]))
	if ms := intOfStatus(res["duration_ms"]); ms > 0 {
		fmt.Fprintf(stdout, "  took     : %dms\n", ms)
	}
	if note, _ := res["note"].(string); note != "" {
		fmt.Fprintf(stdout, "  note     : %s\n", note)
	}
	fmt.Fprintf(stdout, "next: `%s provider setup` to add API keys for the providers you'll use\n", brand.CLI)
	return 0
}

// envOr returns the env var value or a fallback. Local sibling of the daemon's
// envOrDefault (which lives in the controlplane package).
func envOr(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
