// SPDX-License-Identifier: MIT
//
// kernel/delegation utility helpers (SpawnLink, BudgetCostMicrocents, KeyedModelChain,
// AppendUniqueStrings, AppendUniqueString, ValidateSpawnTask, FormatDuration).
// Extracted from delegation.go during Day 211 god-file refactor (#90).
// Public API unchanged.
package delegation

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

func SpawnLink(payload json.RawMessage) (child, parent string) {
	var ev struct {
		Child  string `json:"child_correlation"`
		Parent string `json:"parent"`
	}
	if json.Unmarshal(payload, &ev) == nil {
		return ev.Child, ev.Parent
	}
	return "", ""
}
func BudgetCostMicrocents(payload json.RawMessage) int64 {
	var ev struct {
		CostMicrocents int64 `json:"cost_microcents"`
	}
	if json.Unmarshal(payload, &ev) == nil {
		return ev.CostMicrocents
	}
	return 0
}
func KeyedModelChain(subModel string, modelChain []string, avail func(string) bool, def string) (string, []string) {
	chain := []string{}
	if subModel != "" {
		chain = append(chain, subModel)
	}
	for _, m := range modelChain {
		if m != "" && !slices.Contains(chain, m) {
			chain = append(chain, m)
		}
	}
	kept := make([]string, 0, len(chain))
	for _, m := range chain {
		if avail(m) {
			kept = append(kept, m)
		}
	}
	if len(kept) == 0 {
		if d := strings.TrimSpace(def); d != "" {
			kept = append(kept, d)
		}
	}
	if len(kept) == 0 {
		return subModel, modelChain // nothing to do; keep originals
	}
	if len(kept) == 1 {
		return kept[0], nil
	}
	return kept[0], kept
}
func AppendUniqueStrings(in []string, values ...string) []string {
	for _, v := range values {
		in = AppendUniqueString(in, v)
	}
	return in
}
func AppendUniqueString(in []string, value string) []string {
	if slices.Contains(in, value) {
		return in
	}
	return append(in, value)
}
func ValidateSpawnTask(task string) error {
	if strings.TrimSpace(task) == "" {
		return fmt.Errorf("sub-agent task is empty")
	}
	if len(task) > 10000 {
		return fmt.Errorf("sub-agent task too long (%d chars, max 10000)", len(task))
	}
	return nil
}
func FormatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
}
