// SPDX-License-Identifier: MIT

// Anomaly auto-halt + alert-notify + standing-trust-ceiling builders
// (buildAnomaly, buildAlertNotify, standingTrustCeiling). Extracted from
// main_overlay.go during Day 211 god-file refactor (#56).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/alerter"
	"github.com/agezt/agezt/kernel/anomaly"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/pulse"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
)

func buildAnomaly(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
	max := 120 // ~12 tool calls/sec sustained — only a tight loop hits this
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ANOMALY_MAX_TOOLCALLS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			max = n
		}
	}
	window := 10 * time.Second
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ANOMALY_WINDOW")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			window = d
		}
	}
	if max <= 0 {
		return "disabled (AGEZT_ANOMALY_MAX_TOOLCALLS=0)"
	}
	started := anomaly.Start(ctx, k.Bus(), anomaly.Config{MaxToolCalls: max, Window: window}, func(reason string) {
		fmt.Fprintf(stdout, "  ⚠ anomaly auto-halt engaged: %s\n", reason)
		k.HaltWith(reason)
	})
	if !started {
		return "disabled"
	}
	return fmt.Sprintf("on (halt if >%d tool calls / %s; set AGEZT_ANOMALY_MAX_TOOLCALLS=0 to disable)", max, window)
}
func buildAlertNotify(ctx context.Context, k *kernelruntime.Kernel, sink pulse.BriefSink) string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY"))) {
	case "1", "on", "true", "yes":
	default:
		return "disabled (set AGEZT_ALERT_NOTIFY=1 to push warning/critical alerts to channels)"
	}
	if sink == nil {
		return "enabled but NO channel configured — configure Telegram/Slack/… first"
	}
	cfg := alerter.Config{MinLevel: alerter.ParseLevel(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_LEVEL"))}
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_COOLDOWN")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Cooldown = d
		}
	}
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_MAX")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxPerWindow = n
		}
	}
	// Mute window (M815): hold warnings during a daily quiet window (criticals
	// always break through). Reuses Pulse's "START-END" 24h form, e.g. "0-7".
	cfg.Mute = pulse.ParseQuietHours(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_MUTE"))
	// Per-source routing (M815): drop noisy categories (run/egress/budget/
	// provider/kernel) outright while keeping the rest.
	cfg.MuteSources = alerter.ParseMuteSources(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_MUTE_SOURCES"))
	if !alerter.Start(ctx, k.Bus(), sink, cfg) {
		return "disabled"
	}
	extra := ""
	if cfg.Mute.Enabled {
		extra += fmt.Sprintf("; muted %s (criticals still break through)", cfg.Mute.Spec())
	}
	if len(cfg.MuteSources) > 0 {
		srcs := make([]string, 0, len(cfg.MuteSources))
		for s := range cfg.MuteSources {
			srcs = append(srcs, s)
		}
		sort.Strings(srcs)
		extra += "; sources muted: " + strings.Join(srcs, ",")
	}
	return fmt.Sprintf("on (level≥%s → channels; repeats suppressed; flood-capped%s)", cfg.MinLevel, extra)
}
func standingTrustCeiling(in standing.Initiative) (edict.TrustLevel, bool) {
	ceil := edict.LevelAllow
	have := false
	if lvl, err := edict.ParseTrustLevel(in.MaxTrust); err == nil {
		ceil, have = lvl, true
	}
	if modeLvl, capped := in.Mode.MaxAutonomyTrust(); capped {
		if lvl, err := edict.ParseTrustLevel(modeLvl); err == nil {
			if !have || lvl < ceil {
				ceil = lvl
			}
			have = true
		}
	}
	if !have && in.Mode == standing.InitiativeActOrAsk {
		ceil, have = edict.LevelAskFirst, true
	}
	return ceil, have
}
