// SPDX-License-Identifier: MIT
//
// cmd/agt doctor mesh checks: checkMemoryStoreFile + checkMeshAuth +
// checkMeshLoops + meshLoopCheck + checkProviderFallbacks +
// providerFallbackCheck + checkTenantPeers + checkMeshHopLimit +
// checkPlugins + checkMesh. Split from doctor.go during Day 211
// god-file refactor (#40). Public API unchanged.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/meshctx"
	"github.com/agezt/agezt/kernel/plugin"
	"github.com/agezt/agezt/plugins/tools/peer"
)

func checkMemoryStoreFile(base string, repair bool) doctorCheck {
	const name = "memory store"
	path := filepath.Join(base, "memory", "memory.json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if !repair {
			return warn(name, "memory.json is missing", "run `agt doctor --repair` to recreate an empty memory store")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fail(name, "cannot create memory dir: "+err.Error(), "check filesystem permissions")
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			return fail(name, "cannot recreate memory.json: "+err.Error(), "check filesystem permissions")
		}
		return ok(name, "recreated missing memory.json as empty store")
	}
	if err != nil {
		return fail(name, "cannot read memory.json: "+err.Error(), "check filesystem permissions")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		if !repair {
			return warn(name, "memory.json is empty", "run `agt doctor --repair` to reset it to an empty JSON object")
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			return fail(name, "cannot repair empty memory.json: "+err.Error(), "check filesystem permissions")
		}
		return ok(name, "repaired empty memory.json as empty store")
	}
	clean := bytes.TrimPrefix(trimmed, []byte{0xEF, 0xBB, 0xBF})
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(clean, &decoded); err != nil {
		if !repair {
			return fail(name, "memory.json is not valid JSON: "+err.Error(), "run `agt doctor --repair` to back it up and recreate an empty store")
		}
		backup := path + ".bad-" + time.Now().Format("20060102-150405")
		if err := os.Rename(path, backup); err != nil {
			return fail(name, "cannot back up corrupt memory.json: "+err.Error(), "check filesystem permissions")
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			return fail(name, "backed up corrupt memory.json but could not recreate it: "+err.Error(), "restore from "+backup+" after fixing permissions")
		}
		return ok(name, "backed up corrupt memory.json to "+filepath.Base(backup)+" and recreated empty store")
	}
	if !bytes.Equal(clean, trimmed) {
		if !repair {
			return warn(name, "memory.json has a UTF-8 BOM", "daemon tolerates it, but `agt doctor --repair` will normalize the file")
		}
		if err := os.WriteFile(path, append(clean, '\n'), 0o600); err != nil {
			return fail(name, "cannot rewrite memory.json without BOM: "+err.Error(), "check filesystem permissions")
		}
		return ok(name, fmt.Sprintf("normalized memory.json (%d record(s), BOM removed)", len(decoded)))
	}
	return ok(name, fmt.Sprintf("memory.json valid (%d record(s))", len(decoded)))
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

// checkProviderFallbacks folds the journal's per-kind counts for
// provider.fallback events (M280) — how many times the governor fell back from a
// primary provider to a backup. Reuses CmdJournalStats like checkMeshLoops.
func checkProviderFallbacks(ctx context.Context, client *controlplane.Client) (doctorCheck, bool) {
	res, err := client.Call(ctx, controlplane.CmdJournalStats, nil)
	if err != nil {
		return doctorCheck{}, false
	}
	byKind, _ := res["by_kind"].(map[string]any)
	return providerFallbackCheck(byKind)
}

// providerFallbackCheck is the pure decision behind checkProviderFallbacks:
// given the journal's per-kind event counts, warn when any provider fallback has
// happened. Split out so it is unit-testable without a control-plane round-trip.
func providerFallbackCheck(byKind map[string]any) (doctorCheck, bool) {
	n := intOfStatus(byKind[string(event.KindProviderFallback)])
	if n <= 0 {
		return doctorCheck{}, false
	}
	return warn("provider-fallbacks",
		fmt.Sprintf("%d provider fallback(s) — a primary provider errored and a backup served the run", n),
		"if unexpected, the primary is misconfigured/incompatible (key, base URL, or tool/model support) — runs may be served by the mock; check `agt status` for the last reason"), true
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
