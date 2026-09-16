// SPDX-License-Identifier: MIT
//
// cmd/agt `status` mesh + schedule summary helpers (meshSummary, scheduleStatusLine).
// Extracted from status.go during Day 211 god-file refactor (#81).
// Public API unchanged.
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/plugins/tools/peer"
)

func meshSummary() []map[string]any {
	peers, err := peer.ParsePeers(os.Getenv(brand.EnvPrefix + "PEERS"))
	if err != nil || len(peers) == 0 {
		return nil
	}
	names := make([]string, 0, len(peers))
	for n := range peers {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]map[string]any, 0, len(peers))
	for _, n := range names {
		out = append(out, map[string]any{"name": n, "url": peers[n].URL})
	}
	return out
}
func scheduleStatusLine(sched map[string]any) string {
	total := intOfStatus(sched["total"])
	if total <= 0 {
		return ""
	}
	enabled := intOfStatus(sched["enabled"])
	running := intOfStatus(sched["running"])
	resident, hasResident := sched["resident"].(bool)
	switch {
	case running > 0 && hasResident && !resident:
		return fmt.Sprintf("%d (%d enabled, %d running, resident offline)", total, enabled, running)
	case running > 0:
		return fmt.Sprintf("%d (%d enabled, %d running)", total, enabled, running)
	case enabled > 0 && hasResident && !resident:
		return fmt.Sprintf("%d (%d enabled, resident offline)", total, enabled)
	default:
		return fmt.Sprintf("%d (%d enabled)", total, enabled)
	}
}
