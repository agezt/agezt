// SPDX-License-Identifier: MIT
//
// cmd/agt doctor mesh-loop/fallback/tenant-peer checks
// (checkMeshLoops, meshLoopCheck, checkProviderFallbacks, providerFallbackCheck,
// checkTenantPeers). Extracted from doctor_mesh.go during Day 211 god-file refactor (#65).
// Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/tools/peer"
)

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
func meshLoopCheck(byKind map[string]any) (doctorCheck, bool) {
	n := intOfStatus(byKind[string(event.KindMeshLoopRefused)])
	if n <= 0 {
		return doctorCheck{}, false
	}
	return warn("mesh-loops",
		fmt.Sprintf("%d mesh delegation loop(s) refused (incoming hop limit exceeded)", n),
		"a peer is delegating back into this node — check the federation topology for a cycle"), true
}
func checkProviderFallbacks(ctx context.Context, client *controlplane.Client) (doctorCheck, bool) {
	res, err := client.Call(ctx, controlplane.CmdJournalStats, nil)
	if err != nil {
		return doctorCheck{}, false
	}
	byKind, _ := res["by_kind"].(map[string]any)
	return providerFallbackCheck(byKind)
}
func providerFallbackCheck(byKind map[string]any) (doctorCheck, bool) {
	n := intOfStatus(byKind[string(event.KindProviderFallback)])
	if n <= 0 {
		return doctorCheck{}, false
	}
	return warn("provider-fallbacks",
		fmt.Sprintf("%d provider fallback(s) — a primary provider errored and a backup served the run", n),
		"if unexpected, the primary is misconfigured/incompatible (key, base URL, or tool/model support) — runs may be served by the mock; check `agt status` for the last reason"), true
}
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
