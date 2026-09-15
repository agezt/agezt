// SPDX-License-Identifier: MIT
//
// cmd/agt doctor network/disk checks: checkNetguard + checkRateLimit +
// checkDisk + their from-Xxx helpers + humanBytes helper + diskWarnPct/diskCritPct.
// Split from doctor.go during Day 211 god-file refactor (#40, #54).
// Public API unchanged.
package main

import (
	"context"
	"fmt"

	"github.com/agezt/agezt/cmd/agt/format"
	"github.com/agezt/agezt/kernel/controlplane"
)

func checkNetguard(ctx context.Context, client *controlplane.Client) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdNetguardLog, map[string]any{
		"since_ms": float64(doctorRecentWindowMS),
		"limit":    float64(200),
	})
	if err != nil {
		return ok("netguard", "egress log unavailable (—)")
	}
	return netguardCheckFromLog(res)
}
func netguardCheckFromLog(res map[string]any) doctorCheck {
	const name = "netguard"
	blocks, _ := res["blocks"].([]any)
	n := len(blocks)
	if n == 0 {
		return ok(name, "no egress blocked in the last 24h")
	}
	target := ""
	if first, _ := blocks[0].(map[string]any); first != nil {
		ip, _ := first["ip"].(string)
		tool, _ := first["tool"].(string)
		switch {
		case ip != "" && tool != "":
			target = tool + "→" + ip
		case ip != "":
			target = ip
		}
	}
	detail := fmt.Sprintf("%d egress connection(s) blocked in the last 24h", n)
	hint := "the guard prevented them — review `agt netguard log` (a host to allowlist, or an SSRF/injection attempt)"
	if target != "" {
		hint = fmt.Sprintf("most recent: %s — review `agt netguard log` (allowlist a host, or an SSRF/injection attempt)", target)
	}
	return warn(name, detail, hint)
}
func checkRateLimit(ctx context.Context, client *controlplane.Client) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdRateLimitStats, map[string]any{
		"since_ms": float64(doctorRecentWindowMS),
	})
	if err != nil {
		return ok("ratelimit", "throttle stats unavailable (—)")
	}
	return rateLimitCheckFromStats(res)
}
func rateLimitCheckFromStats(res map[string]any) doctorCheck {
	const name = "ratelimit"
	throttled := intOfStatus(res["throttled"])
	if throttled <= 0 {
		return ok(name, "no requests throttled in the last 24h")
	}
	limit := intOfStatus(res["limit_per_min"])
	worst := intOfStatus(res["worst_used"])
	detail := fmt.Sprintf("%d request(s) throttled in the last 24h", throttled)
	if limit > 0 {
		detail = fmt.Sprintf("%d request(s) throttled in the last 24h (cap %d/min, peak %d)", throttled, limit, worst)
	}
	hint := "a caller is exceeding its per-minute rate cap; raise the limit or pace the caller (`agt ratelimit log`)"
	return warn(name, detail, hint)
}
// diskWarnPct / diskCritPct are the free-space thresholds for the disk check
// (M131). Below crit the journal is in imminent danger of failing to write
// (append-only, never shrinks); below warn it's worth acting before that.
// diskWarnPct is sourced from the format package so the threshold lives
// next to its unit test; diskCritPct is doctor-local because nothing else
// cares about it.
const (
	diskWarnPct = format.DiskWarnPct
	diskCritPct = 3.0
)
func checkDisk(ctx context.Context, client *controlplane.Client) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdDiskStats, nil)
	if err != nil {
		return ok("disk", "disk usage unavailable (—)")
	}
	return diskCheckFromStats(res)
}
func diskCheckFromStats(res map[string]any) doctorCheck {
	const name = "disk"
	journal := intOfStatus(res["journal_bytes"])
	avail, _ := res["disk_available"].(bool)
	if !avail {
		return ok(name, fmt.Sprintf("journal %s (free space unknown)", humanBytes(journal)))
	}
	free := intOfStatus(res["disk_free_bytes"])
	pct, _ := res["disk_free_pct"].(float64)
	detail := fmt.Sprintf("journal %s; disk %.0f%% free (%s)", humanBytes(journal), pct, humanBytes(free))
	if pct < diskCritPct {
		return fail(name, detail,
			"disk almost full — the append-only journal will soon fail to write; archive with `agt backup` and move to a larger disk (the journal is full-retention; see `agt journal stats`)")
	}
	if pct < diskWarnPct {
		return warn(name, detail,
			"disk low and the journal only grows (full retention) — archive with `agt backup` and plan a larger disk; `agt journal stats` shows what's filling it")
	}
	return ok(name, detail)
}
func humanBytes(n int64) string { return format.Bytes(n) }
