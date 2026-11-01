// SPDX-License-Identifier: MIT

package runctx

import "strings"

// NotifySeverityRank orders the notification severities: critical (3) above
// warning (2) above info (1), and anything unknown (0).
//
// The ranking moved here in W3.76c with the noise policy that reads it: the
// floor a run's noise policy must clear is the run's own vocabulary.
func NotifySeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 3
	case "warning", "warn":
		return 2
	case "info", "":
		return 1
	default:
		return 0
	}
}
