// SPDX-License-Identifier: MIT
//
// kernel/controlplane channel probe total accumulators (addChannelProbeTotals, addChannelMediaTotals).
// Extracted from channels.go during Day 211 god-file refactor (#82).
// Public API unchanged.
package controlplane

import (
	"github.com/agezt/agezt/kernel/channel"
)

func addChannelProbeTotals(matrix map[string]any, probe map[string]any) {
	incr := func(key string) {
		n, _ := matrix[key].(int)
		matrix[key] = n + 1
	}
	incr("total")
	if n, _ := probe["configured_accounts"].(int); n > 0 {
		incr("configured")
	}
	if n, _ := probe["live_accounts"].(int); n > 0 {
		incr("live")
	}
	switch probe["roundtrip_status"] {
	case "ready":
		incr("roundtrip_ready")
	case "restart_required":
		incr("restart_needed")
	default:
		incr("needs_setup")
	}
}
func addChannelMediaTotals(matrix map[string]any, media channel.MediaCaps) {
	incr := func(key string) {
		n, _ := matrix[key].(int)
		matrix[key] = n + 1
	}
	if media.ImageIn {
		incr("image_in")
	}
	if media.ImageOut {
		incr("image_out")
	}
	if media.VoiceIn {
		incr("voice_in")
	}
	if media.VoiceOut {
		incr("voice_out")
	}
}
