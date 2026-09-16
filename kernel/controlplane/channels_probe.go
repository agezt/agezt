// SPDX-License-Identifier: MIT
//
// kernel/controlplane channel probe helpers (channelAccountProbe, channelProbe, channelProbeMode).
// Extracted from channels.go during Day 211 god-file refactor (#82).
// Public API unchanged.
package controlplane

import (
	"github.com/agezt/agezt/kernel/channel"
)

func channelAccountProbe(m channel.Manifest, configured, live bool) map[string]any {
	status := "needs_setup"
	note := "required channel settings are missing"
	switch {
	case live:
		status = "ready"
		if m.Duplex {
			note = "live two-way account can receive and send a test reply"
		} else {
			note = "live outbound account can send a test message"
		}
	case configured:
		status = "restart_required"
		note = "configured but not live in this daemon process"
	}
	return map[string]any{
		"configured":       configured,
		"live":             live,
		"roundtrip_status": status,
		"roundtrip_ready":  live,
		"mode":             channelProbeMode(m),
		"note":             note,
	}
}
func channelProbe(m channel.Manifest, accounts []map[string]any) map[string]any {
	configuredAccounts := 0
	liveAccounts := 0
	for _, account := range accounts {
		if b, _ := account["configured"].(bool); b {
			configuredAccounts++
		}
		if b, _ := account["live"].(bool); b {
			liveAccounts++
		}
	}
	status := "needs_setup"
	switch {
	case liveAccounts > 0:
		status = "ready"
	case configuredAccounts > 0:
		status = "restart_required"
	}
	return map[string]any{
		"accounts":            len(accounts),
		"configured_accounts": configuredAccounts,
		"live_accounts":       liveAccounts,
		"roundtrip_status":    status,
		"roundtrip_ready":     liveAccounts > 0,
		"mode":                channelProbeMode(m),
	}
}
func channelProbeMode(m channel.Manifest) string {
	if m.Duplex {
		return "two_way"
	}
	return "outbound"
}
