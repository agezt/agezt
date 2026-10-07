package channels

// Provenance: SPDX-License-Identifier: MIT kernel/controlplane channel probe helpers
//             (channelAccountProbe, channelProbe, channelProbeMode). Extracted from
//             channels.go during Day 211 god-file refactor (#82). Public API
//             unchanged.

import (
	"github.com/agezt/agezt/kernel/channel"
)

func channelAccountProbe(m channel.Manifest, configured, live bool) AccountProbe {
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
	return AccountProbe{Configured: configured, Live: live, RoundtripStatus: status, RoundtripReady: live, Mode: channelProbeMode(m), Note: note}
}
func channelProbe(m channel.Manifest, accounts []ChannelAccount) ChannelProbe {
	configuredAccounts := 0
	liveAccounts := 0
	for _, account := range accounts {
		if account.Configured {
			configuredAccounts++
		}
		if account.Live {
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
	return ChannelProbe{Accounts: len(accounts), ConfiguredAccounts: configuredAccounts, LiveAccounts: liveAccounts, RoundtripStatus: status, RoundtripReady: liveAccounts > 0, Mode: channelProbeMode(m)}
}
func channelProbeMode(m channel.Manifest) string {
	if m.Duplex {
		return "two_way"
	}
	return "outbound"
}
