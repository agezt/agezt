// SPDX-License-Identifier: MIT
//
// cmd/agt doctor daemon/service checks: checkSandbox + checkProvider +
// checkCatalog + checkApprovals + checkWebhooks + checkAgentHealth +
// checkGuardianNoise (with all of its helpers) + checkSchedules +
// checkStanding + int64Of. Split from doctor.go during Day 211 god-file
// refactor (#40). Public API unchanged.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func checkSandbox(ctx context.Context, client *controlplane.Client) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdWardenStats, nil)
	if err != nil {
		return ok("sandbox (warden)", "warden stats unavailable (—)")
	}
	return sandboxCheckFromStats(res)
}

// sandboxCheckFromStats is the pure verdict from a warden-stats response — split
// out so the downgrade/limit logic is testable without a live daemon (M98).
func sandboxCheckFromStats(res map[string]any) doctorCheck {
	const name = "sandbox (warden)"
	total := intOfStatus(res["executions"])
	if total == 0 {
		return ok(name, "no sandboxed executions yet")
	}
	downgraded := intOfStatus(res["downgraded"])
	rate, _ := res["downgrade_rate"].(float64)
	if downgraded > 0 {
		return warn(name,
			fmt.Sprintf("%d/%d execution(s) ran with downgraded isolation (%.0f%%)", downgraded, total, rate*100),
			"the host lacks the requested sandbox backend; on Linux build with full-namespace support, or accept the reduced isolation knowingly")
	}
	if breaches := intOfStatus(res["limit_breaches"]); breaches > 0 {
		return warn(name, fmt.Sprintf("%d execution(s), %d limit breach(es)", total, breaches),
			"a tool hit a warden resource cap; check `agt warden log --issues`")
	}
	return ok(name, fmt.Sprintf("%d execution(s), full requested isolation", total))
}

// checkProvider warns when the daemon has been silently falling back from its
// primary model provider to a secondary (M99) — a high fallback rate means the
// primary keeps failing (bad key, outage, rate limit) and the agent is running
// degraded without anyone noticing. Same shape as the sandbox-downgrade check:
// a real operational gap that belongs in the go-to diagnostic, not only in
// `agt provider stats`. Best-effort: no routing yet, or the call failing, is OK.
func checkProvider(ctx context.Context, client *controlplane.Client) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdProviderStats, nil)
	if err != nil {
		return ok("provider routing", "provider stats unavailable (—)")
	}
	return providerCheckFromStats(res)
}

// providerCheckFromStats is the pure verdict from a provider-stats response —
// split out so the fallback logic is testable without a live daemon (M99).
func providerCheckFromStats(res map[string]any) doctorCheck {
	const name = "provider routing"
	routed := intOfStatus(res["routed"])
	if routed == 0 {
		return ok(name, "no provider routing yet")
	}
	fallbacks := intOfStatus(res["fallbacks"])
	if fallbacks == 0 {
		return ok(name, fmt.Sprintf("%d routed call(s), no fallbacks", routed))
	}
	rate, _ := res["fallback_rate"].(float64)
	detail := fmt.Sprintf("%d/%d routed call(s) fell back to a secondary provider (%.0f%%)", fallbacks, routed, rate*100)
	hint := "the primary provider is failing (bad key, outage, or rate limit); check `agt provider stats`"
	if worst := topFailingProvider(res["fallbacks_by_primary"]); worst != "" {
		hint = fmt.Sprintf("%q is failing most often (bad key, outage, or rate limit); check `agt provider stats`", worst)
	}
	return warn(name, detail, hint)
}

// topFailingProvider returns the provider name with the most fallbacks from a
// fallbacks_by_primary map (ties broken by name for determinism), or "".
func topFailingProvider(raw any) string {
	m, _ := raw.(map[string]any)
	worst := ""
	worstN := int64(0)
	for name, v := range m {
		n := intOfStatus(v)
		if n > worstN || (n == worstN && name < worst) {
			worst, worstN = name, n
		}
	}
	return worst
}

// catalogStaleAfter is how old an API catalog sync can get before doctor warns:
// pricing drifts, so cost/budget decisions made on a stale catalog can be wrong.
const catalogStaleAfter = 21 * 24 * time.Hour

// checkCatalog warns when the API model catalog hasn't been synced in a while
// (M110) — stale pricing silently skews spend tracking and budget enforcement.
// Best-effort: a never-synced catalog (offline/mock) or an unreachable call is an
// informational OK, never a FAIL.
func checkCatalog(ctx context.Context, client *controlplane.Client) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdCatalogList, nil)
	if err != nil {
		return ok("catalog freshness", "catalog info unavailable (—)")
	}
	syncedAt, _ := res["api_synced_at"].(string)
	return catalogCheckFromSync(syncedAt, time.Now())
}

// catalogCheckFromSync is the pure verdict from an api_synced_at timestamp — split
// out so the staleness logic is testable without a live daemon (M110).
func catalogCheckFromSync(apiSyncedAt string, now time.Time) doctorCheck {
	const name = "catalog freshness"
	if apiSyncedAt == "" {
		return ok(name, "no API catalog synced (offline/mock, or pre-sync)")
	}
	t, err := time.Parse(time.RFC3339, apiSyncedAt)
	if err != nil || t.Year() <= 1 {
		return ok(name, "no API catalog synced (offline/mock, or pre-sync)")
	}
	age := now.Sub(t)
	days := int(age.Hours() / 24)
	if age > catalogStaleAfter {
		return warn(name,
			fmt.Sprintf("API catalog last synced %d day(s) ago — model pricing may be stale", days),
			"refresh with `agt catalog sync` so cost estimates and budget enforcement use current prices")
	}
	return ok(name, fmt.Sprintf("API catalog synced %d day(s) ago", days))
}

// checkApprovals warns when HITL approvals have been timing out (M100) — in
// prompt mode an approval that expires with no operator answer auto-denies, so
// the run silently stalls or dies. A nonzero timeout count means the operator is
// not answering or the AGEZT_APPROVAL_TIMEOUT window is too short for the
// deployment; either way it belongs in the go-to diagnostic, not only in
// `agt approvals stats`. Best-effort: no approvals yet, or the call failing, is OK.
func checkApprovals(ctx context.Context, client *controlplane.Client) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdApprovalsStats, nil)
	if err != nil {
		return ok("approvals (HITL)", "approvals stats unavailable (—)")
	}
	return approvalsCheckFromStats(res)
}

// approvalsCheckFromStats is the pure verdict from an approvals-stats response —
// split out so the timeout logic is testable without a live daemon (M100).
func approvalsCheckFromStats(res map[string]any) doctorCheck {
	const name = "approvals (HITL)"
	total := intOfStatus(res["total"])
	if total == 0 {
		return ok(name, "no approvals requested yet")
	}
	timeouts := intOfStatus(res["timeout"])
	if timeouts > 0 {
		return warn(name,
			fmt.Sprintf("%d/%d approval(s) expired with no operator response", timeouts, total),
			"HITL requests are going unanswered — runs auto-deny and stall; respond promptly, lengthen "+brand.EnvPrefix+"APPROVAL_TIMEOUT, or change "+brand.EnvPrefix+"APPROVAL_MODE")
	}
	if pending := intOfStatus(res["pending"]); pending > 0 {
		return ok(name, fmt.Sprintf("%d resolved, %d awaiting operator", total-pending, pending))
	}
	return ok(name, fmt.Sprintf("%d approval(s), none timed out", total))
}

// checkWebhooks warns when outbound webhook deliveries have been failing (M121)
// — an operator wires a webhook sink precisely so they get notified; a sink that
// silently 5xx's or times out is the classic "I never got paged" outage, and it
// is invisible unless someone thinks to run `agt webhook stats`. Folding it into
// the go-to diagnostic surfaces broken notifications proactively. Same shape as
// the sandbox/provider/approvals checks. Best-effort: no deliveries yet, or the
// call failing, is an informational OK, never a FAIL.
func checkWebhooks(ctx context.Context, client *controlplane.Client) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdWebhookStats, nil)
	if err != nil {
		return ok("webhooks", "webhook stats unavailable (—)")
	}
	return webhookCheckFromStats(res)
}

// webhookCheckFromStats is the pure verdict from a webhook-stats response — split
// out so the failure logic is testable without a live daemon (M121).
func webhookCheckFromStats(res map[string]any) doctorCheck {
	const name = "webhooks"
	total := intOfStatus(res["total"])
	if total == 0 {
		return ok(name, "no webhook deliveries yet")
	}
	failed := intOfStatus(res["failed"])
	if failed == 0 {
		return ok(name, fmt.Sprintf("%d delivery(ies), all delivered", total))
	}
	rate, _ := res["failure_rate"].(float64)
	detail := fmt.Sprintf("%d/%d webhook delivery(ies) failed (%.0f%%)", failed, total, rate*100)
	hint := "a notification sink is unreachable or erroring; check `agt webhook log --failed`"
	if worst := topFailingWebhook(res["by_url"]); worst != "" {
		hint = fmt.Sprintf("%q is failing; check `agt webhook log --failed`", worst)
	}
	return warn(name, detail, hint)
}

// topFailingWebhook returns the sink URL with the most failed deliveries from a
// by_url map (url → {delivered, failed}), ties broken by URL for determinism, or "".
func topFailingWebhook(raw any) string {
	m, _ := raw.(map[string]any)
	worst := ""
	worstN := int64(0)
	for url, v := range m {
		entry, _ := v.(map[string]any)
		n := intOfStatus(entry["failed"])
		if n > worstN || (n == worstN && n > 0 && url < worst) {
			worst, worstN = url, n
		}
	}
	if worstN == 0 {
		return ""
	}
	return worst
}

func checkAgentHealth(ctx context.Context, client *controlplane.Client) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdReaperScan, nil)
	if err != nil {
		return ok("agents", "agent health scan unavailable (—)")
	}
	return agentHealthCheckFromScan(res)
}

func agentHealthCheckFromScan(res map[string]any) doctorCheck {
	const name = "agents"
	rows, _ := res["degraded_agents"].([]any)
	misconfigured, _ := res["misconfigured_agents"].([]any)
	if len(rows) == 0 && len(misconfigured) == 0 {
		return ok(name, "no degraded agents")
	}
	if len(rows) == 0 {
		first, _ := misconfigured[0].(map[string]any)
		slug, _ := first["slug"].(string)
		if slug == "" {
			slug = "agent"
		}
		detail := fmt.Sprintf("%d misconfigured agent(s); %s needs attention", len(misconfigured), slug)
		if issues, ok := first["issues"].([]any); ok && len(issues) > 0 {
			if issue, _ := issues[0].(string); issue != "" {
				detail += ": " + issue
			}
		}
		hint := fmt.Sprintf("inspect with `%s agent show %s` and repair owner/parent/config settings", brand.CLI, slug)
		if doctor, _ := first["doctor_agent"].(string); doctor != "" {
			hint += fmt.Sprintf("; doctor agent: %s", doctor)
		}
		if repair, _ := first["self_repair_enabled"].(bool); repair {
			hint += "; self-repair enabled"
		}
		return warn(name, detail, hint)
	}
	first, _ := rows[0].(map[string]any)
	slug, _ := first["slug"].(string)
	if slug == "" {
		slug = "agent"
	}
	failures := intOfStatus(first["failures"])
	threshold := intOfStatus(first["threshold"])
	doctor, _ := first["doctor_agent"].(string)
	repair, _ := first["self_repair_enabled"].(bool)
	detail := fmt.Sprintf("%d degraded agent(s); %s has %d failure(s)", len(rows), slug, failures)
	if threshold > 0 {
		detail += fmt.Sprintf(" (threshold %d)", threshold)
	}
	hint := fmt.Sprintf("inspect with `%s agent activity %s`", brand.CLI, slug)
	if doctor != "" {
		hint += fmt.Sprintf("; doctor agent: %s", doctor)
	}
	if repair {
		hint += "; self-repair enabled"
	}
	return warn(name, detail, hint)
}

const (
	doctorGuardianMaxCostMc         = 50_000_000
	doctorGuardianMaxDailyMc        = 50_000_000
	doctorGuardianNotifyCooldownSec = 8 * 3600
)

func checkGuardianNoise(ctx context.Context, client *controlplane.Client, repair bool) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		return ok("guardian noise", "agent roster unavailable (—)")
	}
	check := guardianNoiseCheckFromList(res)
	if !repair || check.Status != statusWarn {
		return check
	}
	profiles, _ := res["profiles"].([]any)
	targets := noisyGuardianProfiles(profiles)
	repaired := 0
	for _, p := range targets {
		slug := str(p["slug"])
		if slug == "" {
			continue
		}
		if _, err := client.Call(ctx, controlplane.CmdAgentCapabilities, guardianQuietPatch(p)); err != nil {
			return warn("guardian noise",
				fmt.Sprintf("%s; repair quieted %d/%d guardian profile(s)", check.Detail, repaired, len(targets)),
				fmt.Sprintf("failed to quiet %s: %v; inspect with `%s agent show %s`", slug, err, brand.CLI, slug))
		}
		repaired++
	}
	return ok("guardian noise", fmt.Sprintf("quieted %d noisy system guardian profile(s)", repaired))
}

func guardianNoiseCheckFromList(res map[string]any) doctorCheck {
	const name = "guardian noise"
	profiles, _ := res["profiles"].([]any)
	active := 0
	targets := noisyGuardianProfiles(profiles)
	for _, raw := range profiles {
		p, _ := raw.(map[string]any)
		if isActiveSystemGuardian(p) {
			active++
		}
	}
	if active == 0 {
		return ok(name, "no active system guardians")
	}
	if len(targets) == 0 {
		return ok(name, fmt.Sprintf("%d system guardian profile(s) quiet", active))
	}
	first := targets[0]
	slug := str(first["slug"])
	if slug == "" {
		slug = "guardian"
	}
	issues := guardianNoiseIssues(first)
	detail := fmt.Sprintf("%d/%d system guardian profile(s) need quieting; %s: %s", len(targets), active, slug, strings.Join(issues, ", "))
	hint := fmt.Sprintf("run `%s doctor --repair` to enforce memory off, notify >= warning, cooldown >=8h, caps, isolated memory scope, and trust <= L2", brand.CLI)
	return warn(name, detail, hint)
}

func noisyGuardianProfiles(profiles []any) []map[string]any {
	var out []map[string]any
	for _, raw := range profiles {
		p, _ := raw.(map[string]any)
		if !isActiveSystemGuardian(p) {
			continue
		}
		if len(guardianNoiseIssues(p)) > 0 {
			out = append(out, p)
		}
	}
	return out
}

func isActiveSystemGuardian(p map[string]any) bool {
	if p == nil {
		return false
	}
	system, _ := p["system"].(bool)
	retired, _ := p["retired"].(bool)
	return system && !retired
}

func guardianNoiseIssues(p map[string]any) []string {
	if p == nil {
		return nil
	}
	slug := str(p["slug"])
	noise, _ := p["noise_policy"].(map[string]any)
	var issues []string
	if noise == nil {
		issues = append(issues, "no noise policy")
	} else {
		silent, _ := noise["silent_on_success"].(bool)
		disableMemory, _ := noise["disable_memory_writes"].(bool)
		if !silent {
			issues = append(issues, "success notifications enabled")
		}
		if !disableMemory && !guardianToolDenyHasMemory(p) {
			issues = append(issues, "memory writes enabled")
		}
		if notifySeverityRank(str(noise["min_notify_severity"])) < notifySeverityRank("warning") {
			issues = append(issues, "notify below warning")
		}
		if intNumber(noise["min_notify_interval_sec"]) < doctorGuardianNotifyCooldownSec {
			issues = append(issues, "notify cooldown <8h")
		}
	}
	if intNumber(p["max_cost_mc"]) <= 0 || intNumber(p["max_cost_mc"]) > doctorGuardianMaxCostMc {
		issues = append(issues, "run cap missing/high")
	}
	if intNumber(p["max_daily_mc"]) <= 0 || intNumber(p["max_daily_mc"]) > doctorGuardianMaxDailyMc {
		issues = append(issues, "daily cap missing/high")
	}
	if guardianTrustRank(str(p["trust_ceiling"])) > guardianTrustRank("L2") {
		issues = append(issues, "trust above L2")
	}
	if slug != "" && str(p["memory_scope"]) != "system/"+slug {
		issues = append(issues, "memory scope not isolated")
	}
	return issues
}

func guardianToolDenyHasMemory(p map[string]any) bool {
	for _, raw := range stringsAny(p["tool_deny"]) {
		if strings.EqualFold(strings.TrimSpace(str(raw)), "memory") {
			return true
		}
	}
	return false
}

func guardianTrustRank(level string) int {
	level = strings.TrimSpace(strings.ToUpper(level))
	if len(level) == 2 && level[0] == 'L' && level[1] >= '0' && level[1] <= '4' {
		return int(level[1] - '0')
	}
	return 4
}

func notifySeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 3
	case "warning", "warn":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func guardianQuietPatch(p map[string]any) map[string]any {
	slug := str(p["slug"])
	policy, _ := p["noise_policy"].(map[string]any)
	notifySeverity := str(policy["min_notify_severity"])
	if notifySeverityRank(notifySeverity) < notifySeverityRank("warning") {
		notifySeverity = "warning"
	}
	cooldown := intNumber(policy["min_notify_interval_sec"])
	if cooldown < doctorGuardianNotifyCooldownSec {
		cooldown = doctorGuardianNotifyCooldownSec
	}
	toolDeny := stringsAny(p["tool_deny"])
	hasMemory := false
	for _, raw := range toolDeny {
		if strings.EqualFold(strings.TrimSpace(str(raw)), "memory") {
			hasMemory = true
			break
		}
	}
	if !hasMemory {
		toolDeny = append(toolDeny, "memory")
	}
	toolAllow := make([]any, 0, len(stringsAny(p["tool_allow"])))
	for _, raw := range stringsAny(p["tool_allow"]) {
		if strings.EqualFold(strings.TrimSpace(str(raw)), "memory") {
			continue
		}
		toolAllow = append(toolAllow, raw)
	}
	return map[string]any{
		"ref":           slug,
		"memory_scope":  "system/" + slug,
		"max_cost_mc":   doctorGuardianMaxCostMc,
		"max_daily_mc":  doctorGuardianMaxDailyMc,
		"trust_ceiling": "L2",
		"tool_allow":    toolAllow,
		"tool_deny":     toolDeny,
		"noise_policy": map[string]any{
			"silent_on_success":       true,
			"disable_memory_writes":   true,
			"min_notify_severity":     notifySeverity,
			"min_notify_interval_sec": cooldown,
		},
	}
}

// checkSchedules warns when an enabled schedule's most recent firing failed
// (M162). Scheduled runs are the autonomy axis: they fire unattended, so a run
// that errors leaves no one watching — the failure sits silently in the journal
// until someone thinks to run `agt schedule list`. Folding the last-firing
// outcome into the go-to diagnostic surfaces broken automation proactively. Same
// shape as the webhooks check. Best-effort: no schedules, or the call failing, is
// an informational OK, never a FAIL.
func checkSchedules(ctx context.Context, client *controlplane.Client, status map[string]any, repair bool) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		return ok("schedules", "schedule list unavailable (—)")
	}
	check := schedulesCheckFromList(res, status)
	if !repair || check.Status != statusWarn {
		return check
	}
	ids := scheduleAttentionIDs(res)
	if len(ids) == 0 {
		return check
	}
	paused := 0
	for _, id := range ids {
		out, err := client.Call(ctx, controlplane.CmdScheduleEnable, map[string]any{"id": id, "enabled": false})
		if err != nil {
			return warn("schedules",
				fmt.Sprintf("%s; repair paused %d/%d attention schedule(s)", check.Detail, paused, len(ids)),
				fmt.Sprintf("failed to pause %s: %v; inspect with `%s schedule list`", id, err, brand.CLI))
		}
		if updated, _ := out["updated"].(bool); updated {
			paused++
		}
	}
	return ok("schedules", fmt.Sprintf("paused %d attention schedule(s) with unsafe cadence or blocked targets", paused))
}

// schedulesCheckFromList is the pure verdict from a schedule-list response — split
// out so the failure logic is testable without a live daemon (M162). Only ENABLED
// schedules count: a disabled one the operator turned off shouldn't raise an
// alarm. A schedule that hasn't fired yet (no last_status) is healthy-by-default.
func schedulesCheckFromList(res map[string]any, status map[string]any) doctorCheck {
	const name = "schedules"
	rows, _ := res["schedules"].([]any)
	if len(rows) == 0 {
		return ok(name, "no schedules configured")
	}
	enabled := 0
	var failedIDs []string
	var noisyIDs []string
	var blockedIDs []string
	worst := "" // id of the most recently-failed schedule, for the hint
	worstMS := int64(-1)
	for _, r := range rows {
		row, _ := r.(map[string]any)
		if row == nil {
			continue
		}
		if on, _ := row["enabled"].(bool); !on {
			continue
		}
		enabled++
		if warning, _ := row["frequency_warning"].(string); strings.TrimSpace(warning) != "" {
			if id, _ := row["id"].(string); strings.TrimSpace(id) != "" {
				noisyIDs = append(noisyIDs, id)
			}
		}
		if scheduleTargetBlocked(row) {
			if id, _ := row["id"].(string); strings.TrimSpace(id) != "" {
				blockedIDs = append(blockedIDs, id)
			}
		}
		switch s, _ := row["last_status"].(string); s {
		case "failed", "abandoned":
			id, _ := row["id"].(string)
			failedIDs = append(failedIDs, id)
			if ms := int64Of(row["last_fired_unix_ms"]); ms >= worstMS {
				worst, worstMS = id, ms
			}
		}
	}
	if enabled > 0 && !scheduleResident(status) {
		return warn(name,
			fmt.Sprintf("%d enabled schedule(s), but the cadence resident is not attached", enabled),
			fmt.Sprintf("restart the daemon or inspect `%s status --json` schedules.resident", brand.CLI))
	}
	if len(blockedIDs) > 0 {
		id := blockedIDs[0]
		return warn(name,
			fmt.Sprintf("%d/%d enabled schedule(s) have blocked targets", len(blockedIDs), enabled),
			fmt.Sprintf("inspect `%s schedule list`; pause with `%s schedule enable %s false` or fix the target", brand.CLI, brand.CLI, id))
	}
	if len(failedIDs) == 0 {
		if enabled == 0 {
			return ok(name, fmt.Sprintf("%d schedule(s), none enabled", len(rows)))
		}
		if len(noisyIDs) > 0 {
			id := noisyIDs[0]
			return warn(name,
				fmt.Sprintf("%d/%d enabled schedule(s) are more frequent than their quiet cadence", len(noisyIDs), enabled),
				fmt.Sprintf("inspect `%s schedule list`; pause with `%s schedule enable %s false` or increase its interval", brand.CLI, brand.CLI, id))
		}
		return ok(name, fmt.Sprintf("%d enabled schedule(s), recent firings healthy", enabled))
	}
	detail := fmt.Sprintf("%d/%d enabled schedule(s) last firing failed", len(failedIDs), enabled)
	hint := fmt.Sprintf("inspect with `agt schedule fires --id %s` (or `agt runs`)", worst)
	return warn(name, detail, hint)
}

func scheduleResident(status map[string]any) bool {
	sched, _ := status["schedules"].(map[string]any)
	if sched == nil {
		return true
	}
	resident, ok := sched["resident"].(bool)
	if !ok {
		return true
	}
	return resident
}

func scheduleAttentionIDs(res map[string]any) []string {
	rows, _ := res["schedules"].([]any)
	out := []string{}
	for _, r := range rows {
		row, _ := r.(map[string]any)
		if row == nil {
			continue
		}
		if on, _ := row["enabled"].(bool); !on {
			continue
		}
		warning, _ := row["frequency_warning"].(string)
		if strings.TrimSpace(warning) == "" && !scheduleTargetBlocked(row) {
			continue
		}
		id, _ := row["id"].(string)
		if id = strings.TrimSpace(id); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func scheduleTargetBlocked(row map[string]any) bool {
	if status, _ := row["target_status"].(string); strings.EqualFold(strings.TrimSpace(status), "blocked") {
		return true
	}
	err, _ := row["target_error"].(string)
	return strings.TrimSpace(err) != ""
}

func checkStanding(ctx context.Context, client *controlplane.Client, repair bool) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdStandingList, nil)
	if err != nil {
		return ok("standing", "standing order list unavailable (—)")
	}
	check := standingCheckFromList(res)
	if !repair || check.Status != statusWarn {
		return check
	}
	ids := standingAttentionIDs(res)
	if len(ids) == 0 {
		return check
	}
	paused := 0
	for _, id := range ids {
		out, err := client.Call(ctx, controlplane.CmdStandingSetEnabled, map[string]any{"id": id, "enabled": false})
		if err != nil {
			return warn("standing",
				fmt.Sprintf("%s; repair paused %d/%d attention standing order(s)", check.Detail, paused, len(ids)),
				fmt.Sprintf("failed to pause %s: %v; inspect with `%s standing list`", id, err, brand.CLI))
		}
		if _, ok := out["order"].(map[string]any); ok {
			paused++
		}
	}
	return ok("standing", fmt.Sprintf("paused %d attention standing order(s) with unsafe cadence or blocked targets", paused))
}

func standingCheckFromList(res map[string]any) doctorCheck {
	const name = "standing"
	rows, _ := res["orders"].([]any)
	if len(rows) == 0 {
		return ok(name, "no standing wake rules configured")
	}
	enabled := 0
	var noisyIDs []string
	var blockedIDs []string
	for _, r := range rows {
		row, _ := r.(map[string]any)
		if row == nil {
			continue
		}
		if on, _ := row["enabled"].(bool); !on {
			continue
		}
		enabled++
		if warning, _ := row["frequency_warning"].(string); strings.TrimSpace(warning) != "" {
			if id, _ := row["id"].(string); strings.TrimSpace(id) != "" {
				noisyIDs = append(noisyIDs, id)
			}
		}
		if scheduleTargetBlocked(row) {
			if id, _ := row["id"].(string); strings.TrimSpace(id) != "" {
				blockedIDs = append(blockedIDs, id)
			}
		}
	}
	if enabled == 0 {
		return ok(name, fmt.Sprintf("%d standing wake rule(s), none enabled", len(rows)))
	}
	if len(blockedIDs) > 0 {
		id := blockedIDs[0]
		return warn(name,
			fmt.Sprintf("%d/%d enabled standing wake rule(s) have blocked targets", len(blockedIDs), enabled),
			fmt.Sprintf("inspect `%s standing list`; pause with `%s standing pause %s` or fix the target agent", brand.CLI, brand.CLI, id))
	}
	if len(noisyIDs) > 0 {
		id := noisyIDs[0]
		return warn(name,
			fmt.Sprintf("%d/%d enabled standing wake rule(s) are more frequent than their quiet cadence", len(noisyIDs), enabled),
			fmt.Sprintf("inspect `%s standing list`; pause with `%s standing pause %s` or increase its interval/cooldown", brand.CLI, brand.CLI, id))
	}
	return ok(name, fmt.Sprintf("%d enabled standing wake rule(s)", enabled))
}

func standingAttentionIDs(res map[string]any) []string {
	rows, _ := res["orders"].([]any)
	out := []string{}
	for _, r := range rows {
		row, _ := r.(map[string]any)
		if row == nil {
			continue
		}
		if on, _ := row["enabled"].(bool); !on {
			continue
		}
		warning, _ := row["frequency_warning"].(string)
		if strings.TrimSpace(warning) == "" && !scheduleTargetBlocked(row) {
			continue
		}
		id, _ := row["id"].(string)
		if id = strings.TrimSpace(id); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// int64Of coerces a decoded-JSON number (float64) to int64. Returns -1 for a
// missing/non-numeric value so it sorts below any real timestamp.
func int64Of(v any) int64 {
	if f, ok := v.(float64); ok {
		return int64(f)
	}
	return -1
}

// doctorRecentWindowMS is the look-back window for the log-folding security checks
// (netguard M163, ratelimit M164). These events have no rate denominator (unlike
// webhooks), so an all-time count would WARN forever after a single reviewed event.
// A 24h window keeps the checks actionable and self-clearing: yesterday's egress
// block or throttle is worth a look now; one from last week that was already
// handled is not.
const doctorRecentWindowMS = int64(24 * 60 * 60 * 1000)

// checkNetguard warns when the egress guard has refused connections recently
// (M163). A netguard.blocked event means a tool (http/browser) tried to reach an
// internal/metadata address (e.g. 169.254.169.254) and was stopped — a strong
// SSRF / prompt-injection / exfiltration signal, OR a legitimate host that needs
// allowlisting. Either way the operator should look. This is a WARN, not a FAIL:
// the guard did its job; the run was protected. Best-effort: the call failing, or
// no blocks, is an informational OK. Same shape as the webhooks/schedules checks.
