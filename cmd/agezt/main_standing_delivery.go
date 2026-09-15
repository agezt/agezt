// SPDX-License-Identifier: MIT

// Delegation banner + scheduled-delivery helpers extracted from main_standing.go
// during Day 211 god-file refactor (#51). Public API unchanged.
package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// delegationBanner renders the active multi-agent delegation ceilings (M58) for
// the boot banner — the same effective caps `agt status` reports (M49), so the
// governance is visible at startup, not only on demand. "off" when the delegate
// tool is disabled; 0 fan-out / spend render as "unbounded".
func delegationBanner(k *kernelruntime.Kernel) string {
	l := k.SubAgentLimits()
	if !l.Enabled {
		return "off (AGEZT_SUBAGENT=off)"
	}
	fanout := "unbounded"
	if l.MaxFanout > 0 {
		fanout = fmt.Sprintf("≤%d", l.MaxFanout)
	}
	spend := "unbounded"
	if l.MaxSpendMicrocents > 0 {
		spend = fmt.Sprintf("$%.4f", float64(l.MaxSpendMicrocents)/1e9)
	}
	total := "unbounded"
	if l.MaxTotal > 0 {
		total = fmt.Sprintf("≤%d", l.MaxTotal)
	}
	return fmt.Sprintf("depth≤%d, fan-out %s, total %s, spend %s", l.MaxDepth, fanout, total, spend)
}
// buildCadence starts the scheduled-intents resident when AGEZT_SCHEDULE is set.
// Each firing journals a schedule.fired event (carrying the run's correlation so
// `agt why` links the schedule to the run) and then runs the intent through the
// normal governed loop. Returns the banner description; "" only when the env var
// is unset and the store is empty.
// deliverScheduled sends a scheduled run's answer to every configured channel
// recipient (M152), prefixed with the schedule id so the operator knows which job
// produced it. Empty answers are skipped. Returns the number of successful
// deliveries (for testing). Channel kinds are iterated in sorted order for
// deterministic delivery.
func deliverScheduled(ctx context.Context, send func(context.Context, string, string, string) error, targets map[string][]string, id, answer string) int {
	if strings.TrimSpace(answer) == "" || send == nil {
		return 0
	}
	text := "[scheduled: " + id + "]\n" + answer
	kinds := make([]string, 0, len(targets))
	for k := range targets {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	sent := 0
	for _, kind := range kinds {
		for _, recip := range targets[kind] {
			if send(ctx, kind, recip, text) == nil {
				sent++
			}
		}
	}
	return sent
}
