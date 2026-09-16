// SPDX-License-Identifier: MIT
//
// cmd/agt `agent authority` typed helpers (anyToStringSlice, levelExceeds, levelRank, orDash).
// Extracted from agent_authority.go during Day 211 god-file refactor (#85).
// Public API unchanged.
package main

import (
	"strings"
)

func anyToStringSlice(v any) []string {
	switch xs := v.(type) {
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			if s := str(x); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return xs
	default:
		return nil
	}
}
func levelExceeds(level, ceiling string) bool {
	return levelRank(level) > levelRank(ceiling)
}
func levelRank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "l0", "deny", "":
		return 0
	case "l1", "ask":
		return 1
	case "l2", "askfirst":
		return 2
	case "l3", "askscoped":
		return 3
	case "l4", "allow":
		return 4
	default:
		return 0
	}
}
func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
