// SPDX-License-Identifier: MIT

// Pulse periodic-ticker starters + onOff helper (startReflectTicker,
// startWorkboardSweepTicker, startBrainDistillTicker, startProfileDistillTicker,
// onOff). Extracted from main_pulse.go during Day 211 god-file refactor (#57).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/ulid"
)

func startReflectTicker(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
	raw := os.Getenv(brand.EnvPrefix + "REFLECT_EVERY")
	if raw == "" {
		return ""
	}
	every, err := time.ParseDuration(raw)
	if err != nil || every <= 0 {
		fmt.Fprintf(stdout, "  reflection       : invalid AGEZT_REFLECT_EVERY %q (%v) — on-demand only\n", raw, err)
		return ""
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				corr := "reflect-" + ulid.New()
				if _, err := k.Reflect().Reflect(ctx, corr); err != nil {
					fmt.Fprintf(stdout, "reflection pass failed: %v\n", err)
				}
			}
		}
	}()
	return "every " + every.String()
}
func startWorkboardSweepTicker(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
	raw := os.Getenv(brand.EnvPrefix + "WORKBOARD_SWEEP_EVERY")
	if raw == "" {
		return ""
	}
	every, err := time.ParseDuration(raw)
	if err != nil || every <= 0 {
		fmt.Fprintf(stdout, "  workboard sweep  : invalid AGEZT_WORKBOARD_SWEEP_EVERY %q (%v) - on-demand only\n", raw, err)
		return ""
	}
	staleAfter := 10 * time.Minute
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WORKBOARD_STALE_AFTER")); spec != "" {
		if parsed, perr := time.ParseDuration(spec); perr == nil && parsed > 0 {
			staleAfter = parsed
		} else {
			fmt.Fprintf(stdout, "  workboard sweep  : invalid AGEZT_WORKBOARD_STALE_AFTER %q (%v) - using %s\n", spec, perr, staleAfter)
		}
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				corr := "workboard-sweep-" + ulid.New()
				tasks, err := k.SweepStaleWorkboardClaims(corr, "workboard-sweeper", staleAfter, 100)
				if err != nil {
					fmt.Fprintf(stdout, "workboard sweep failed: %v\n", err)
					continue
				}
				if len(tasks) > 0 {
					fmt.Fprintf(stdout, "workboard sweep reclaimed %d stale claim(s)\n", len(tasks))
				}
			}
		}
	}()
	return "every " + every.String() + " (stale after " + staleAfter.String() + ")"
}
func startBrainDistillTicker(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
	raw := os.Getenv(brand.EnvPrefix + "BRAIN_DISTILL_EVERY")
	if raw == "" {
		return ""
	}
	every, err := time.ParseDuration(raw)
	if err != nil || every <= 0 {
		fmt.Fprintf(stdout, "  brain distill    : invalid AGEZT_BRAIN_DISTILL_EVERY %q (%v) — on-demand only\n", raw, err)
		return ""
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				corr := "brain-distill-" + ulid.New()
				if _, err := k.DistillBrain(ctx, corr); err != nil {
					fmt.Fprintf(stdout, "brain-distill pass failed: %v\n", err)
				}
			}
		}
	}()
	return "every " + every.String()
}
func startProfileDistillTicker(ctx context.Context, k *kernelruntime.Kernel, on bool, stdout io.Writer) string {
	if !on {
		return ""
	}
	every := 24 * time.Hour
	if raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "USER_PROFILE_EVERY")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			every = d
		} else {
			fmt.Fprintf(stdout, "  user profile     : invalid AGEZT_USER_PROFILE_EVERY %q (%v) — using 24h\n", raw, err)
		}
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				corr := "profile-distill-" + ulid.New()
				if _, err := k.DistillProfile(ctx, corr); err != nil {
					fmt.Fprintf(stdout, "profile-distill pass failed: %v\n", err)
				}
			}
		}
	}()
	return "every " + every.String()
}
func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
