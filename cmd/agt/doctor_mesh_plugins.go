// SPDX-License-Identifier: MIT
//
// cmd/agt doctor plugin + mesh summary checks (checkPlugins, checkMesh).
// Extracted from doctor_mesh.go during Day 211 god-file refactor (#65).
// Public API unchanged.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/plugin"
	"github.com/agezt/agezt/plugins/tools/peer"
)

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
