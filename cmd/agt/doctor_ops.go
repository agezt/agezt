// SPDX-License-Identifier: MIT
//
// cmd/agt doctor ops/state checks: checkNetguard + checkRateLimit +
// checkDisk + checkExposure + checkBudget + checkCredentials +
// checkChannels + checkModelReadiness + checkBaseDir + checkVersionSkew +
// checkJournal + checkTools + checkHalt + humanBytes helper.
// Split from doctor.go during Day 211 god-file refactor (#40).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/cmd/agt/format"
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

// netguardCheckFromLog is the pure verdict from a netguard-log response — split out
// so the logic is testable without a live daemon (M163). Blocks arrive newest-first
// (the handler sorts by ts desc), so blocks[0] is the most recent — surfaced in the
// hint so the operator knows what to look at.
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

// checkRateLimit warns when callers have been throttled recently (M164). A
// rate.limited event means a tenant exceeded its per-minute request cap (M14
// quotas) and was refused — persistent throttling means a caller is undersized for
// its workload, or something is hammering the daemon. Surfacing it lets the
// operator raise the cap or pace the caller before it manifests as mysterious
// failed runs. Best-effort: the call failing, or no throttling, is an
// informational OK. Same shape as the netguard check.
func checkRateLimit(ctx context.Context, client *controlplane.Client) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdRateLimitStats, map[string]any{
		"since_ms": float64(doctorRecentWindowMS),
	})
	if err != nil {
		return ok("ratelimit", "throttle stats unavailable (—)")
	}
	return rateLimitCheckFromStats(res)
}

// rateLimitCheckFromStats is the pure verdict from a ratelimit-stats response —
// split out so the logic is testable without a live daemon (M164).
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

// checkDisk warns when the filesystem holding the journal is running low (M131)
// — the journal is append-only and grows forever, so on a small host a full disk
// is the classic silent outage: writes start failing and the daemon can no
// longer record what it does. Surfacing it in the go-to diagnostic catches it
// before that. Best-effort: a daemon without the disk probe wired, or the call
// failing, is an informational OK.
func checkDisk(ctx context.Context, client *controlplane.Client) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdDiskStats, nil)
	if err != nil {
		return ok("disk", "disk usage unavailable (—)")
	}
	return diskCheckFromStats(res)
}

// diskCheckFromStats is the pure verdict from a disk-stats response — split out
// so the threshold logic is testable without a live daemon (M131).
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

// humanBytes is a shim → format.Bytes (Day 8 extraction; the body used to
// live here verbatim from M131).
func humanBytes(n int64) string { return format.Bytes(n) }

// checkExposure warns when a network-exposed HTTP server (web UI / REST / OpenAI
// API) is bound beyond loopback (M137). Those surfaces drive the full agent loop
// — shell/file/http tools — gated only by a bearer token, so a non-loopback bind
// puts the agent on the network. The per-server boot banner warns once; this
// makes it a persistent line in the go-to diagnostic. Reads the status snapshot,
// so no extra round-trip. An all-loopback (or no-HTTP) daemon is an OK.
func checkExposure(status map[string]any) doctorCheck {
	const name = "network exposure"
	servers, _ := status["http_servers"].([]any)
	if len(servers) == 0 {
		return ok(name, "no HTTP servers exposed (control plane is loopback-only)")
	}
	var exposed []string
	for _, raw := range servers {
		m, _ := raw.(map[string]any)
		if lb, _ := m["loopback"].(bool); !lb {
			n, _ := m["name"].(string)
			a, _ := m["addr"].(string)
			exposed = append(exposed, fmt.Sprintf("%s (%s)", n, a))
		}
	}
	if len(exposed) > 0 {
		return warn(name,
			fmt.Sprintf("%d HTTP server(s) reachable beyond localhost: %s", len(exposed), strings.Join(exposed, ", ")),
			"the agent (shell/file/http tools) is exposed to the network, gated only by a token — bind to 127.0.0.1 and front it with a TLS reverse proxy, or restrict with a firewall")
	}
	return ok(name, fmt.Sprintf("%d HTTP server(s), all loopback-bound", len(servers)))
}

// budgetWarnPct is the daily-spend fraction (%) at which the doctor starts
// warning — close enough to the ceiling that runs will soon be blocked.
const budgetWarnPct = 90.0

// checkBudget warns as the day's spend approaches (or reaches) the global daily
// ceiling — runs fail terminally (no fallback) once the cap is hit, so an operator
// wants warning before that, not a confusing "all providers failed" mid-run. Pure
// logic in budgetCheckFromBudget; a failed/absent budget call is an informational
// OK (never a false alarm).
func checkBudget(ctx context.Context, client *controlplane.Client) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdBudget, nil)
	if err != nil {
		return ok("budget", "unavailable ("+err.Error()+")")
	}
	return budgetCheckFromBudget(res)
}

func budgetCheckFromBudget(res map[string]any) doctorCheck {
	const name = "budget"
	spent := mcFromAny(res["spent_mc"])
	ceiling := mcFromAny(res["ceiling_mc"])
	if ceiling <= 0 {
		return ok(name, fmt.Sprintf("%s spent today (no daily ceiling)", fmtUSD(spent)))
	}
	used := float64(spent) / float64(ceiling) * 100
	detail := fmt.Sprintf("%s / %s today (%.0f%%)", fmtUSD(spent), fmtUSD(ceiling), used)
	switch {
	case spent >= ceiling:
		return warn(name, detail+" — daily ceiling reached",
			"new runs are blocked until the daily spend window resets at UTC midnight")
	case used >= budgetWarnPct:
		return warn(name, detail+" — near the daily ceiling",
			"runs are blocked once the ceiling is hit (resets at UTC midnight); reduce usage to avoid mid-run failures")
	default:
		return ok(name, detail)
	}
}

// checkChannels warns when a messaging channel (M141 status surface) is
// half-configured: it has a listen addr but inbound is DISABLED — i.e. the
// operator exposed an endpoint that will reject every event because the inbound
// secret / public key is missing (a Slack/Discord webhook channel set up with a
// token + addr but no AGEZT_*_SIGNING_SECRET / _PUBLIC_KEY). The boot banner shows
// this once and `agt status` renders it as "outbound-only", but neither nags;
// this makes it a persistent WARN in the go-to diagnostic. All-good / no-channels
// is an OK. Pure function of the status snapshot (no extra round-trip).
// checkCredentials surfaces the resolved AWS credential chain (M308) in the
// preflight pane — the same description `agt status` shows, so an operator
// deploying to EKS/cloud can confirm which keyless/ambient layer engaged
// (IRSA/web-identity, SSO, assume-role) before running a workload. Always
// informational OK: the chain always resolves to at least vault→env→file/IMDS,
// and whether those actually hold credentials is a runtime fact the separate
// provider check already covers. A keyless layer is called out explicitly
// because that's the bit operators most want confirmed.
func checkCredentials(status map[string]any) doctorCheck {
	const name = "aws creds"
	chain, _ := status["cred_chain"].(string)
	if chain == "" {
		return ok(name, "default chain (vault → env → ~/.aws → IMDS)")
	}
	for _, layer := range []string{"web_identity", "assume_role", "sso"} {
		if strings.Contains(chain, layer+"=") {
			return ok(name, chain+"  [keyless: "+layer+"]")
		}
	}
	return ok(name, chain)
}

func checkChannels(status map[string]any) doctorCheck {
	const name = "channels"
	chans, _ := status["channels"].([]any)
	if len(chans) == 0 {
		return ok(name, "no messaging channels configured")
	}
	var halfConfigured []string
	inbound := 0
	for _, raw := range chans {
		m, _ := raw.(map[string]any)
		isIn, _ := m["inbound"].(bool)
		addr, _ := m["addr"].(string)
		if isIn {
			inbound++
		}
		// A listen addr with inbound disabled = the endpoint is up but rejects
		// everything (missing secret/key). An addr-less outbound-only channel is a
		// deliberate, fine choice — not flagged.
		if addr != "" && !isIn {
			kind, _ := m["kind"].(string)
			halfConfigured = append(halfConfigured, fmt.Sprintf("%s (%s)", kind, addr))
		}
	}
	if len(halfConfigured) > 0 {
		return warn(name,
			fmt.Sprintf("%d channel(s) listening but inbound DISABLED: %s", len(halfConfigured), strings.Join(halfConfigured, ", ")),
			"set the channel's signing secret / public key (AGEZT_SLACK_SIGNING_SECRET / AGEZT_DISCORD_PUBLIC_KEY) so inbound messages are accepted, or unset the addr to run outbound-only")
	}
	return ok(name, fmt.Sprintf("%d configured, %d can receive commands", len(chans), inbound))
}

// checkModelReadiness reports whether the daemon's configured model is fit
// for the tool-driven agent loop, surfacing the same catalog.Model.AgentWarnings
// as `agt provider check --caps` / the boot advisory (M23–M25), now inside the
// operator's go-to diagnostic. Conservative: WARN only on a known capability
// gap; an offline/mock model or a model the catalog doesn't list is an
// informational OK (capabilities unknown), never a FAIL.
func checkModelReadiness(status map[string]any, cat *catalog.Catalog) doctorCheck {
	const name = "model readiness"
	model, _ := status["model"].(string)
	if model == "" || model == "mock" {
		return ok(name, "offline/mock model (no catalog capabilities to assess)")
	}
	if cat == nil {
		return ok(name, fmt.Sprintf("%s (catalog not synced — capabilities unknown)", model))
	}
	_, m := cat.FindModel(model)
	if m == nil {
		return ok(name, fmt.Sprintf("%s (not in catalog — capabilities unknown)", model))
	}
	if w := m.AgentWarnings(); len(w) > 0 {
		return warn(name, fmt.Sprintf("%s — %s", model, strings.Join(w, "; ")),
			"pick a tool-capable model (AGEZT_MODEL) or set AGEZT_MODEL_STRICT=on to fail fast")
	}
	return ok(name, model+" (agent-ready: advertises tool-use)")
}

func checkBaseDir(base string, baseErr error) doctorCheck {
	const name = "base directory"
	if baseErr != nil {
		return fail(name, baseErr.Error(), "set AGEZT_HOME or check filesystem permissions")
	}
	info, err := os.Stat(base)
	if os.IsNotExist(err) {
		return warn(name, base+" (not created yet)",
			fmt.Sprintf("run `%s` once to initialise it", brand.Binary))
	}
	if err != nil {
		return fail(name, err.Error(), "check filesystem permissions")
	}
	if !info.IsDir() {
		return fail(name, base+" exists but is not a directory", "remove the file or set AGEZT_HOME")
	}
	// Prove writability rather than guessing from mode bits.
	probe := filepath.Join(base, ".doctor-probe")
	if werr := os.WriteFile(probe, []byte("ok"), 0o600); werr != nil {
		return fail(name, base+" (not writable: "+werr.Error()+")", "fix ownership/permissions on the base dir")
	}
	_ = os.Remove(probe)
	return ok(name, base+" (writable)")
}

func checkVersionSkew(status map[string]any) doctorCheck {
	const name = "version skew"
	daemonVer, _ := status["daemon"].(string)
	daemonProto := intOfStatus(status["protocol"])
	if daemonVer == brand.Version && daemonProto == int64(brand.ProtocolVersion) {
		return ok(name, fmt.Sprintf("client and daemon aligned (%s, protocol v%d)", brand.Version, brand.ProtocolVersion))
	}
	return warn(name,
		fmt.Sprintf("client %s/v%d vs daemon %s/v%d", brand.Version, brand.ProtocolVersion, daemonVer, daemonProto),
		fmt.Sprintf("restart the daemon to align (`%s shutdown` then `%s`)", brand.CLI, brand.Binary))
}

func checkJournal(ctx context.Context, client *controlplane.Client, status map[string]any) doctorCheck {
	const name = "journal"
	if _, err := client.Call(ctx, controlplane.CmdJournalVerify, nil); err != nil {
		return fail(name, "hash chain verification failed: "+err.Error(),
			"the audit log may be tampered or truncated — investigate before trusting it")
	}
	head := intOfStatus(status["journal_head"])
	return ok(name, fmt.Sprintf("BLAKE3 hash chain verified (head seq=%d)", head))
}

func checkTools(status map[string]any) doctorCheck {
	const name = "tools"
	n := intOfStatus(status["tools"])
	if n == 0 {
		return warn(name, "0 registered", "no capabilities available — check tool plugins / AGEZT_TOOLS")
	}
	return ok(name, fmt.Sprintf("%d registered", n))
}

func checkHalt(status map[string]any) doctorCheck {
	const name = "halt state"
	if halted, _ := status["halted"].(bool); halted {
		return warn(name, "system is HALTED", fmt.Sprintf("resume work with `%s resume`", brand.CLI))
	}
	return ok(name, "running")
}
