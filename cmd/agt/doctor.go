// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/meshctx"
	"github.com/agezt/agezt/kernel/plugin"
	"github.com/agezt/agezt/plugins/tools/peer"
)

// cmdDoctor implements `agt doctor` — the zero-config-first-run preflight
// (ROADMAP §2.1 "always-on essential"; SPEC-08 §3.3 version-skew check).
//
// It runs a checklist and prints each result as OK / WARN / FAIL with a
// remediation hint. Local checks (base dir) always run; daemon checks
// (reachability, version skew, journal integrity, tools, halt state) run when
// the daemon is up, and collapse to a single FAIL on "daemon" when it isn't —
// so `agt doctor` is the first thing to run when something feels wrong, and it
// degrades honestly rather than erroring out.
//
// Exit: 0 when nothing FAILed (warnings don't fail — they're advisories); 1
// when any check FAILed; 2 on bad args. With `--strict`, warnings also exit 1 —
// so monitoring/CI can alert on advisory-level signals (a failing schedule, an
// egress block, throttling). `--json` emits the machine form for CI.
//
// Reuses existing surfaces only: paths.BaseDir, controlplane.NewClient/Call,
// CmdStatus, CmdJournalVerify. No new control-plane command, no new event kind.
func cmdDoctor(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	strict := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "--strict":
			strict = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s doctor [--json] [--strict]\n", brand.CLI)
			fmt.Fprintf(stdout, "preflight health check: base dir, daemon, version skew, journal, tools\n")
			fmt.Fprintf(stdout, "  --strict  exit non-zero on warnings too (not just failures)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s doctor: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}

	checks := runDoctorChecks()

	if asJSON {
		return renderDoctorJSON(checks, strict, stdout)
	}
	return renderDoctorText(checks, strict, stdout)
}

// doctorExitCode maps the worst check status to a process exit code. A FAIL is
// always non-zero; a WARN is non-zero only under --strict (warnings are advisories
// by default, but a monitoring/CI caller can opt into treating them as actionable).
func doctorExitCode(worst checkStatus, strict bool) int {
	if worst == statusFail || (strict && worst == statusWarn) {
		return 1
	}
	return 0
}

// checkStatus is a tri-state result. Order matters: worst wins in a summary.
type checkStatus int

const (
	statusOK checkStatus = iota
	statusWarn
	statusFail
)

func (s checkStatus) label() string {
	switch s {
	case statusWarn:
		return "WARN"
	case statusFail:
		return "FAIL"
	default:
		return "OK"
	}
}

// doctorCheck is one line of the report.
type doctorCheck struct {
	Name   string      `json:"name"`
	Status checkStatus `json:"-"`
	State  string      `json:"status"` // string form for JSON ("OK"/"WARN"/"FAIL")
	Detail string      `json:"detail"`
	Hint   string      `json:"hint,omitempty"`
}

func ok(name, detail string) doctorCheck {
	return doctorCheck{Name: name, Status: statusOK, State: "OK", Detail: detail}
}
func warn(name, detail, hint string) doctorCheck {
	return doctorCheck{Name: name, Status: statusWarn, State: "WARN", Detail: detail, Hint: hint}
}
func fail(name, detail, hint string) doctorCheck {
	return doctorCheck{Name: name, Status: statusFail, State: "FAIL", Detail: detail, Hint: hint}
}

// runDoctorChecks performs the diagnostics and returns them in display order.
func runDoctorChecks() []doctorCheck {
	var checks []doctorCheck

	base, baseErr := paths.BaseDir()
	checks = append(checks, checkBaseDir(base, baseErr))

	// Daemon-dependent checks need a client. If we can't build one (no
	// addr/token files), the daemon isn't running — report that one FAIL and
	// skip the rest (they'd all just say "daemon unreachable").
	if baseErr != nil {
		checks = append(checks, fail("daemon", "cannot resolve base dir", "fix the base dir error above"))
		return checks
	}
	// Probe the recorded control-plane address. This surfaces *which* daemon
	// the CLI reaches (so a stray second instance is visible) and tells a
	// stale socket (recorded, dead) apart from no socket at all.
	addr, alive := controlplane.ProbeExisting(base)
	switch {
	case addr == "":
		checks = append(checks, fail("daemon", "not running (no control-plane socket recorded)",
			fmt.Sprintf("start it: %s", brand.Binary)))
		return checks
	case !alive:
		checks = append(checks, fail("daemon", "recorded at "+addr+" but not responding (stale socket)",
			fmt.Sprintf("a daemon crashed or was killed; start a fresh one: %s", brand.Binary)))
		return checks
	}

	client, err := controlplane.NewClient(base)
	if err != nil {
		checks = append(checks, fail("daemon", "socket recorded but client build failed: "+err.Error(),
			fmt.Sprintf("start it: %s", brand.Binary)))
		return checks
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	status, err := client.Call(ctx, controlplane.CmdStatus, nil)
	if err != nil {
		checks = append(checks, fail("daemon", "control-plane call failed: "+err.Error(),
			fmt.Sprintf("is the daemon healthy? try `%s status`", brand.CLI)))
		return checks
	}
	checks = append(checks, ok("daemon", "running at "+addr))
	checks = append(checks, checkVersionSkew(status))
	checks = append(checks, checkJournal(ctx, client, status))
	checks = append(checks, checkTools(status))
	// Model readiness (M26): is the running model fit for the tool-driven
	// agent loop? Best-effort — the catalog is read from disk (the same one
	// the daemon loaded); a missing catalog or unlisted model yields an
	// informational OK, never a false alarm.
	cat, _ := loadCatalogIfAny(io.Discard)
	checks = append(checks, checkModelReadiness(status, cat))
	checks = append(checks, checkSandbox(ctx, client))
	checks = append(checks, checkProvider(ctx, client))
	checks = append(checks, checkApprovals(ctx, client))
	checks = append(checks, checkBudget(ctx, client))
	checks = append(checks, checkCatalog(ctx, client))
	checks = append(checks, checkWebhooks(ctx, client))
	checks = append(checks, checkSchedules(ctx, client))
	checks = append(checks, checkDisk(ctx, client))
	checks = append(checks, checkExposure(status))
	checks = append(checks, checkNetguard(ctx, client))
	checks = append(checks, checkRateLimit(ctx, client))
	checks = append(checks, checkChannels(status))
	checks = append(checks, checkPlugins())
	checks = append(checks, checkMesh())
	// Mesh auth posture (M214): only when peers are configured. Flags a peer reached
	// without a token — an unauthenticated cross-node delegation.
	if peers, err := peer.ParsePeers(os.Getenv(brand.EnvPrefix + "PEERS")); err == nil && len(peers) > 0 {
		checks = append(checks, checkMeshAuth(peers))
	}
	// Mesh hop-limit config (M213): only surfaced when AGEZT_MESH_MAX_HOPS is set, so
	// single-node operators see no noise. Flags a typo that would silently fall back.
	if _, raw, _ := meshctx.MaxHopsConfig(); raw != "" {
		checks = append(checks, checkMeshHopLimit())
	}
	// Mesh loop-guard activity (M226): surfaced only when the local node has
	// actually refused a delegation loop, so healthy/single-node output stays
	// quiet. A non-zero count is a real signal worth investigating.
	if c, show := checkMeshLoops(ctx, client); show {
		checks = append(checks, c)
	}
	// Per-tenant peer overrides (M227): only when AGEZT_TENANT_PEERS is set —
	// validates the spec the daemon hard-fails on, so a typo is caught here
	// rather than by a daemon that won't restart.
	if c, show := checkTenantPeers(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TENANT_PEERS"))); show {
		checks = append(checks, c)
	}
	checks = append(checks, checkHalt(status))

	return checks
}

// checkMeshAuth flags peers configured without a Bearer token (M214). A token-less
// peer means `remote_run` delegates to that node unauthenticated — at odds with the
// "loopback + token only" posture. WARN (not FAIL): a peer on a trusted private
// network may legitimately need no token, so this is a posture nudge, not a hard stop.
func checkMeshAuth(peers map[string]peer.Peer) doctorCheck {
	var tokenless []string
	for name, p := range peers {
		if p.Token == "" {
			tokenless = append(tokenless, name)
		}
	}
	if len(tokenless) == 0 {
		return ok("mesh-auth", fmt.Sprintf("all %d peer(s) authenticate with a token", len(peers)))
	}
	sort.Strings(tokenless)
	return warn("mesh-auth",
		fmt.Sprintf("%d/%d peer(s) have no token — unauthenticated delegation: %s",
			len(tokenless), len(peers), strings.Join(tokenless, ", ")),
		"add a token: AGEZT_PEERS=\"name=url|token,…\"")
}

// checkMeshLoops reports how many cross-node delegation loops the local node
// has refused (M226, surfacing the M209 loop guard). Each refusal is a
// `mesh.loop_refused` journal event: a peer handed this node a run whose hop
// count already exceeded the limit, so the REST API rejected it with 508 to
// break a federation cycle. The count comes from the journal's per-kind fold
// (CmdJournalStats), so this needs no new kernel state.
//
// Returns show=false (no line at all) when none have occurred — the healthy and
// single-node case — to keep doctor output quiet. A non-zero count is a WARN:
// the local node is fine (it correctly stopped the loop), but a peer delegating
// back into it points at a federation-topology mistake worth fixing.
func checkMeshLoops(ctx context.Context, client *controlplane.Client) (doctorCheck, bool) {
	res, err := client.Call(ctx, controlplane.CmdJournalStats, nil)
	if err != nil {
		// Journal stats unavailable — stay silent; daemon-health checks above
		// already cover a non-responsive control plane.
		return doctorCheck{}, false
	}
	byKind, _ := res["by_kind"].(map[string]any)
	return meshLoopCheck(byKind)
}

// meshLoopCheck is the pure decision behind checkMeshLoops: given the journal's
// per-kind event counts, decide whether to surface a mesh-loop warning. Split
// out so it is unit-testable without a control-plane round-trip.
func meshLoopCheck(byKind map[string]any) (doctorCheck, bool) {
	n := intOfStatus(byKind[string(event.KindMeshLoopRefused)])
	if n <= 0 {
		return doctorCheck{}, false
	}
	return warn("mesh-loops",
		fmt.Sprintf("%d mesh delegation loop(s) refused (incoming hop limit exceeded)", n),
		"a peer is delegating back into this node — check the federation topology for a cycle"), true
}

// checkTenantPeers validates AGEZT_TENANT_PEERS — the per-tenant mesh peer
// overrides (M219) — and summarises what loaded (M227). The daemon parses this
// up front and HARD-FAILS on a malformed spec, so a typo means the daemon
// refuses to start; doctor reads the same env and surfaces the problem first.
// It also gives the operator positive confirmation of which tenants have a
// dedicated peer set (without it, a typo'd tenant name silently falls back to
// the global peer set and there's no signal it happened).
//
// Only called when AGEZT_TENANT_PEERS is set (it's an advanced feature), so it
// returns show=false on an empty spec to keep ordinary output quiet. Peer URLs
// and tokens are never printed — only tenant names and peer counts.
func checkTenantPeers(spec string) (doctorCheck, bool) {
	if spec == "" {
		return doctorCheck{}, false
	}
	tp, err := peer.ParseTenantPeers(spec)
	if err != nil {
		return fail("tenant-peers", "AGEZT_TENANT_PEERS is malformed: "+err.Error(),
			"the daemon will refuse to start — fix the spec (JSON: {\"<tenant>\":\"name=url|token,…\"})"), true
	}
	// Tenants present in the spec but with an empty peer set are silently
	// dropped by the parser — and so ignored by the daemon. Surface that: the
	// override the operator wrote does nothing, and the tenant falls back to the
	// global set with no other signal.
	var raw map[string]string
	var dropped []string
	if json.Unmarshal([]byte(spec), &raw) == nil {
		for tenant := range raw {
			if tenant = strings.TrimSpace(tenant); tenant != "" {
				if _, kept := tp[tenant]; !kept {
					dropped = append(dropped, tenant)
				}
			}
		}
	}
	sort.Strings(dropped)

	names := make([]string, 0, len(tp))
	for t := range tp {
		names = append(names, t)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, t := range names {
		parts[i] = fmt.Sprintf("%s→%d peer(s)", t, len(tp[t]))
	}

	if len(dropped) > 0 {
		loaded := "none"
		if len(parts) > 0 {
			loaded = strings.Join(parts, ", ")
		}
		return warn("tenant-peers",
			fmt.Sprintf("loaded %d override(s) [%s]; ignored (empty peer set): %s",
				len(tp), loaded, strings.Join(dropped, ", ")),
			"give the ignored tenant(s) peers, or remove the empty entry"), true
	}
	if len(tp) == 0 {
		return ok("tenant-peers", "no per-tenant peer overrides"), true
	}
	return ok("tenant-peers",
		fmt.Sprintf("%d tenant override(s): %s", len(tp), strings.Join(parts, ", "))), true
}

// checkMeshHopLimit validates an explicitly-set AGEZT_MESH_MAX_HOPS (M211/M213). A valid
// override is reported OK with its effective value; an invalid one (non-integer, <1, or
// past the cap) is a WARN — the daemon silently falls back to the default 8, so without
// this an operator's typo in a safety-relevant setting would go unnoticed.
func checkMeshHopLimit() doctorCheck {
	eff, raw, valid := meshctx.MaxHopsConfig()
	if valid {
		return ok("mesh-hops", fmt.Sprintf("delegation hop limit = %d (AGEZT_MESH_MAX_HOPS)", eff))
	}
	return warn("mesh-hops",
		fmt.Sprintf("AGEZT_MESH_MAX_HOPS=%q is invalid and ignored; using default %d", raw, eff),
		fmt.Sprintf("set an integer in [1, %d]", meshctx.MaxConfigurableHops))
}

// checkPlugins validates the external-plugin env-specs (AGEZT_PLUGINS plus the
// optional AGEZT_PLUGIN_PINS / AGEZT_PLUGIN_TOOLS) without spawning anything
// (M225). The daemon parses these at startup and HARD-FAILS on a malformed
// one — so a typo means the daemon refuses to restart. doctor reads the same
// env the daemon would and surfaces the problem first:
//
//   - no AGEZT_PLUGINS → informational OK (no external plugins).
//   - a malformed spec → FAIL, naming which env var and the parse error (this
//     is startup-blocking, hence FAIL not WARN — unlike the mesh checks where
//     the daemon degrades rather than refusing to start).
//   - a valid spec whose pins/tools reference a prefix with no matching plugin
//     → WARN (the daemon would warn about the stale entry at startup).
//   - otherwise OK with the plugin count.
//
// It reads only the operator's environment; no running daemon is required.
func checkPlugins() doctorCheck {
	spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PLUGINS"))
	if spec == "" {
		return ok("plugins", "no external plugins configured")
	}
	entries, err := plugin.ParsePluginSpec(spec)
	if err != nil {
		return fail("plugins", "AGEZT_PLUGINS is malformed: "+err.Error(),
			"the daemon will refuse to start — fix the spec: AGEZT_PLUGINS=\"<prefix>=<path> [args],…\"")
	}

	prefixes := make([]string, len(entries))
	for i, e := range entries {
		prefixes[i] = e.Prefix
	}

	// Pins and tool-allowlists are parsed with the same hard-error semantics;
	// a malformed one is equally startup-blocking.
	var pins plugin.PinSpec
	if pinSpec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PLUGIN_PINS")); pinSpec != "" {
		pins, err = plugin.ParsePinSpec(pinSpec)
		if err != nil {
			return fail("plugins", "AGEZT_PLUGIN_PINS is malformed: "+err.Error(),
				"the daemon will refuse to start — fix the spec: AGEZT_PLUGIN_PINS=\"<prefix>=<hash>,…\"")
		}
	}
	var allowed plugin.ToolAllowlistSpec
	if toolSpec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PLUGIN_TOOLS")); toolSpec != "" {
		allowed, err = plugin.ParseToolAllowlistSpec(toolSpec)
		if err != nil {
			return fail("plugins", "AGEZT_PLUGIN_TOOLS is malformed: "+err.Error(),
				"the daemon will refuse to start — fix the spec: AGEZT_PLUGIN_TOOLS=\"<prefix>=<tool>+<tool>,…\"")
		}
	}

	// Stale pin/tool entries (a prefix with no matching plugin) are the daemon's
	// startup WARNINGs — surface them here too so a typo'd prefix is caught.
	var stale []string
	for _, p := range pins.UnusedPins(prefixes) {
		stale = append(stale, "pin:"+p)
	}
	for _, p := range allowed.Unused(prefixes) {
		stale = append(stale, "tools:"+p)
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		return warn("plugins",
			fmt.Sprintf("%d plugin(s) configured, but these entries reference no plugin prefix: %s",
				len(entries), strings.Join(stale, ", ")),
			"fix the prefix or remove the stale AGEZT_PLUGIN_PINS/AGEZT_PLUGIN_TOOLS entry")
	}

	detail := fmt.Sprintf("%d plugin(s) configured", len(entries))
	if len(pins) > 0 {
		detail += fmt.Sprintf(", %d pinned", len(pins))
	}
	if len(allowed) > 0 {
		detail += fmt.Sprintf(", %d allow-listed", len(allowed))
	}
	return ok("plugins", detail)
}

// checkMesh reports the health of the configured peer mesh (M8 / AGEZT_PEERS): each
// peer's REST /api/v1/health is probed (reusing the `agt peers` check). All reachable
// is OK; an unreachable peer is a WARN (the local node is fine, the mesh is degraded)
// naming the down peers; a malformed AGEZT_PEERS is a WARN; no peers configured is an
// informational OK (single-node). Tokens are never printed. It is independent of the
// local daemon — a peer is reached over its own network surface (M207).
func checkMesh() doctorCheck {
	peers, err := peer.ParsePeers(os.Getenv(brand.EnvPrefix + "PEERS"))
	if err != nil {
		return warn("mesh", "AGEZT_PEERS is malformed: "+err.Error(),
			"fix the spec: AGEZT_PEERS=\"name=url|token,…\"")
	}
	if len(peers) == 0 {
		return ok("mesh", "no peers configured (single-node)")
	}
	names := make([]string, 0, len(peers))
	for n := range peers {
		names = append(names, n)
	}
	sort.Strings(names)

	var down []string
	for _, n := range names {
		if !checkPeer(peers[n]).Reachable {
			down = append(down, n)
		}
	}
	if len(down) == 0 {
		return ok("mesh", fmt.Sprintf("%d peer(s) reachable: %s", len(peers), strings.Join(names, ", ")))
	}
	return warn("mesh",
		fmt.Sprintf("%d/%d peer(s) unreachable: %s", len(down), len(peers), strings.Join(down, ", ")),
		fmt.Sprintf("check the peer URLs/tokens and that those daemons are running; `%s peers` for detail", brand.CLI))
}

// checkSandbox warns when the OS warden has been silently downgrading isolation
// (M98) — a sandbox running weaker than requested is a real security gap an
// operator should see in their go-to diagnostic, not only in `agt warden stats`.
// Best-effort: no executions yet, or the call failing, is an informational OK.
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

// checkSchedules warns when an enabled schedule's most recent firing failed
// (M162). Scheduled runs are the autonomy axis: they fire unattended, so a run
// that errors leaves no one watching — the failure sits silently in the journal
// until someone thinks to run `agt schedule list`. Folding the last-firing
// outcome into the go-to diagnostic surfaces broken automation proactively. Same
// shape as the webhooks check. Best-effort: no schedules, or the call failing, is
// an informational OK, never a FAIL.
func checkSchedules(ctx context.Context, client *controlplane.Client) doctorCheck {
	res, err := client.Call(ctx, controlplane.CmdScheduleList, nil)
	if err != nil {
		return ok("schedules", "schedule list unavailable (—)")
	}
	return schedulesCheckFromList(res)
}

// schedulesCheckFromList is the pure verdict from a schedule-list response — split
// out so the failure logic is testable without a live daemon (M162). Only ENABLED
// schedules count: a disabled one the operator turned off shouldn't raise an
// alarm. A schedule that hasn't fired yet (no last_status) is healthy-by-default.
func schedulesCheckFromList(res map[string]any) doctorCheck {
	const name = "schedules"
	rows, _ := res["schedules"].([]any)
	if len(rows) == 0 {
		return ok(name, "no schedules configured")
	}
	enabled := 0
	var failedIDs []string
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
		switch s, _ := row["last_status"].(string); s {
		case "failed", "abandoned":
			id, _ := row["id"].(string)
			failedIDs = append(failedIDs, id)
			if ms := int64Of(row["last_fired_unix_ms"]); ms >= worstMS {
				worst, worstMS = id, ms
			}
		}
	}
	if len(failedIDs) == 0 {
		if enabled == 0 {
			return ok(name, fmt.Sprintf("%d schedule(s), none enabled", len(rows)))
		}
		return ok(name, fmt.Sprintf("%d enabled schedule(s), recent firings healthy", enabled))
	}
	detail := fmt.Sprintf("%d/%d enabled schedule(s) last firing failed", len(failedIDs), enabled)
	hint := fmt.Sprintf("inspect with `agt schedule fires --id %s` (or `agt runs`)", worst)
	return warn(name, detail, hint)
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
const (
	diskWarnPct = 10.0
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

// humanBytes renders a byte count as B/KB/MB/GB/TB with one decimal (M131).
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

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

func renderDoctorText(checks []doctorCheck, strict bool, stdout io.Writer) int {
	worst := statusOK
	var nOK, nWarn, nFail int
	fmt.Fprintf(stdout, "%s doctor:\n", brand.CLI)
	for _, c := range checks {
		fmt.Fprintf(stdout, "  [%-4s] %-16s : %s\n", c.Status.label(), c.Name, c.Detail)
		if c.Hint != "" && c.Status != statusOK {
			fmt.Fprintf(stdout, "           ↳ %s\n", c.Hint)
		}
		switch c.Status {
		case statusOK:
			nOK++
		case statusWarn:
			nWarn++
		case statusFail:
			nFail++
		}
		if c.Status > worst {
			worst = c.Status
		}
	}
	fmt.Fprintf(stdout, "\nsummary: %d ok, %d %s, %d failed\n",
		nOK, nWarn, plural(nWarn, "warning", "warnings"), nFail)
	// Under --strict, point out that the warnings are what produced the non-zero
	// exit, so the operator isn't left wondering why a warning-only run "failed".
	if strict && worst == statusWarn {
		fmt.Fprintf(stdout, "strict: warnings treated as failures (exit 1)\n")
	}
	return doctorExitCode(worst, strict)
}

func renderDoctorJSON(checks []doctorCheck, strict bool, stdout io.Writer) int {
	worst := statusOK
	for _, c := range checks {
		if c.Status > worst {
			worst = c.Status
		}
	}
	exit := doctorExitCode(worst, strict)
	out := map[string]any{
		"checks":  checks,
		"healthy": worst != statusFail,
		"worst":   worst.label(),
		"strict":  strict,
		// ok reflects the exit verdict under the chosen mode: false when this run
		// will exit non-zero (a FAIL, or a WARN under --strict).
		"ok": exit == 0,
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	return exit
}
