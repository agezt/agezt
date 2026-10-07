// SPDX-License-Identifier: MIT
package channels

import "github.com/agezt/agezt/kernel/channel"

func addChannelProbeTotals(matrix *ProbeMatrix, probe ChannelProbe) {
	matrix.Total++
	if probe.ConfiguredAccounts > 0 {
		matrix.Configured++
	}
	if probe.LiveAccounts > 0 {
		matrix.Live++
	}
	switch probe.RoundtripStatus {
	case "ready":
		matrix.RoundtripReady++
	case "restart_required":
		matrix.RestartNeeded++
	default:
		matrix.NeedsSetup++
	}
}
func addChannelMediaTotals(matrix *MediaMatrix, media channel.MediaCaps) {
	if media.ImageIn {
		matrix.ImageIn++
	}
	if media.ImageOut {
		matrix.ImageOut++
	}
	if media.VoiceIn {
		matrix.VoiceIn++
	}
	if media.VoiceOut {
		matrix.VoiceOut++
	}
}
