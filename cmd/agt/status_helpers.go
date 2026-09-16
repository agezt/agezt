// SPDX-License-Identifier: MIT
//
// cmd/agt `status` typed helpers (intOfStatus, fmtUptime).
// Extracted from status.go during Day 211 god-file refactor (#81).
// Public API unchanged.
package main

import (
	"github.com/agezt/agezt/cmd/agt/format"
)

func intOfStatus(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}
func fmtUptime(secs int64) string { return format.Uptime(secs) }
