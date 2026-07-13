// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/update"
)

// runUpdate implements `agezt update [--apply]`.
func runUpdate(stdout, stderr io.Writer) int {
	baseDir, err := paths.BaseDir()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.Binary, err)
		return 1
	}
	args := os.Args[2:]
	apply := len(args) > 0 && args[0] == "--apply"
	cl, err := controlplane.NewClient(baseDir)
	if err != nil {
		fmt.Fprintf(stderr, "%s: controlplane: %v\n", brand.Binary, err)
		return 1
	}
	defer cl.Close()

	if apply {
		check, err := cl.UpdateCheck(context.Background())
		if err != nil {
			fmt.Fprintf(stderr, "%s: update check: %v\n", brand.Binary, err)
			return 1
		}
		if check.Update == nil {
			fmt.Fprintf(stdout, "%s: already up to date (%s)\n", brand.Binary, check.Current)
			return 0
		}
		fmt.Fprintf(stdout, "%s: applying update %s (from %s)\n", brand.Binary, check.Update.Version, check.Current)
		result, err := cl.UpdateApply(context.Background(), check.Update.Version, check.Update.SHA256, check.Update.URL, check.Update.Notes)
		if err != nil {
			fmt.Fprintf(stderr, "%s: update apply: %v\n", brand.Binary, err)
			return 1
		}
		if result.Error != "" {
			fmt.Fprintf(stderr, "%s: update failed: %s\n", brand.Binary, result.Error)
			return 1
		}
		fmt.Fprintf(stdout, "%s: update applied, daemon will restart shortly\n", brand.Binary)
		return 0
	}

	check, err := cl.UpdateCheck(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "%s: update check: %v\n", brand.Binary, err)
		return 1
	}
	if check.Update == nil {
		fmt.Fprintf(stdout, "%s: up to date (%s)\n", brand.Binary, check.Current)
	} else {
		fmt.Fprintf(stdout, "%s: update available: %s (current: %s)\n", brand.Binary, check.Update.Version, check.Current)
		if check.Update.Notes != "" {
			fmt.Fprintf(stdout, "\n%s\n", check.Update.Notes)
		}
	}
	return 0
}

// startUpdateChecker runs the background auto-update check loop.
func startUpdateChecker(ctx context.Context, k *kernelruntime.Kernel, svc *update.Service, stdout, stderr io.Writer) {
	ticker := time.NewTicker(svc.CheckInterval())
	defer ticker.Stop()

	check := func() {
		result, err := svc.Check(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "%s: auto-update check: %v\n", brand.Binary, err)
			return
		}
		if result.Update == nil {
			return
		}
		info := result.Update
		fmt.Fprintf(stdout, "%s: auto-update: %s available (current: %s)\n", brand.Binary, info.Version, result.Current)
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "update.available", Kind: event.KindInfo, Actor: "update-checker",
			Payload: map[string]any{"current_version": result.Current, "new_version": info.Version, "url": info.URL},
		})
		fmt.Fprintf(stdout, "%s: auto-update: draining daemon for %s\n", brand.Binary, info.Version)
		_, activeRuns := k.DrainAndHalt(svc.DrainTimeout())
		if activeRuns > 0 {
			fmt.Fprintf(stderr, "%s: auto-update: drain timeout (%d runs still active)\n", brand.Binary, activeRuns)
			return
		}
		err = svc.Apply(ctx, info, func(context.Context, time.Duration) update.DrainResult {
			return update.DrainResult{}
		})
		if err != nil {
			_, _ = k.Bus().Publish(event.Spec{
				Subject: "update.failed", Kind: event.KindAnomalyDetected, Actor: "update-checker",
				Payload: map[string]any{"version": info.Version, "error": err.Error()},
			})
			fmt.Fprintf(stderr, "%s: auto-update failed: %v (daemon stays running)\n", brand.Binary, err)
			return
		}
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "update.applied", Kind: event.KindInfo, Actor: "update-checker",
			Payload: map[string]any{"version": info.Version},
		})
		fmt.Fprintf(stdout, "%s: auto-update: %s applied, restarting\n", brand.Binary, info.Version)
		os.Exit(0)
	}

	check()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}
