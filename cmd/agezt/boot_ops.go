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

// updateDrainer is the slice of the kernel an update needs: stop new work and
// unwind in-flight runs, and undo that when the update does not happen.
type updateDrainer interface {
	DrainAndHalt(timeout time.Duration) (bool, int)
	ResumeWith(reason string)
}

// updateApplyFunc is update.Service.Apply.
type updateApplyFunc func(ctx context.Context, info *update.UpdateInfo, drain func(context.Context, time.Duration) update.DrainResult) error

// applyAvailableUpdate installs info and reports whether the daemon must now
// restart into the new binary.
//
// The kernel is halted only inside Apply's drain step — AFTER the binary was
// downloaded and its checksum and signature verified — and resumed on every
// failure after that point. It used to be the other way round: the checker
// drained and halted FIRST and passed Apply a no-op drain, so a failed
// download or a refused signature (the common case: no release key is
// configured yet) left the daemon running but halted — no agent could run
// until an operator noticed and resumed it by hand, while the log claimed
// "daemon stays running". A drain timeout left it halted the same way.
func applyAvailableUpdate(ctx context.Context, k updateDrainer, apply updateApplyFunc, info *update.UpdateInfo,
	publish func(event.Spec), stdout, stderr io.Writer) bool {
	halted := false
	err := apply(ctx, info, func(_ context.Context, timeout time.Duration) update.DrainResult {
		fmt.Fprintf(stdout, "%s: auto-update: %s verified, draining daemon\n", brand.Binary, info.Version)
		halted = true
		_, active := k.DrainAndHalt(timeout)
		return update.DrainResult{Timeout: active > 0, ActiveRuns: active}
	})
	if err != nil {
		if halted {
			k.ResumeWith("auto-update to " + info.Version + " aborted: " + err.Error())
		}
		publish(event.Spec{
			Subject: "update.failed", Kind: event.KindAnomalyDetected, Actor: "update-checker",
			Payload: map[string]any{"version": info.Version, "error": err.Error()},
		})
		fmt.Fprintf(stderr, "%s: auto-update failed: %v (daemon stays running)\n", brand.Binary, err)
		return false
	}
	if !halted {
		// No drain configured (DrainTimeout 0): Apply swapped without one, so
		// stop new work before the restart, as the checker always did.
		k.DrainAndHalt(0)
	}
	publish(event.Spec{
		Subject: "update.applied", Kind: event.KindInfo, Actor: "update-checker",
		Payload: map[string]any{"version": info.Version},
	})
	fmt.Fprintf(stdout, "%s: auto-update: %s applied, restarting\n", brand.Binary, info.Version)
	return true
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
		publish := func(s event.Spec) { _, _ = k.Bus().Publish(s) }
		if applyAvailableUpdate(ctx, k, svc.Apply, info, publish, stdout, stderr) {
			os.Exit(0)
		}
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
