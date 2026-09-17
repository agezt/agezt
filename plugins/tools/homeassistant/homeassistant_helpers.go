// SPDX-License-Identifier: MIT

// homeassistant_helpers.go: matchAllowed + errResult split off from homeassistant.go
// during the Day 211 god-file refactor (#136). Public API unchanged.
package homeassistant

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)


// matchAllowed reports whether target is permitted by patterns. A pattern is an
// exact dotted id ("light.turn_on"), a "domain.*" whole-domain wildcard, or "*"
// for anything. Matching is case-insensitive. The domain is the segment before
// the first dot.
func matchAllowed(patterns []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return false
	}
	tdom := target
	if d, _, ok := strings.Cut(target, "."); ok {
		tdom = d
	}
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if p == "*" || p == target {
			return true
		}
		if strings.HasSuffix(p, ".*") && p[:len(p)-2] == tdom {
			return true
		}
	}
	return false
}

// Capabilities returns a human-readable summary for the daemon banner /
// `agt status` (sorted for determinism). Empty axes are omitted.
func (t *Tool) Capabilities() string {
	var parts []string
	if n := len(t.ReadEntities); n > 0 {
		parts = append(parts, fmt.Sprintf("read=%d", n))
	}
	if n := len(t.AllowedServices); n > 0 {
		parts = append(parts, fmt.Sprintf("services=%d", n))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: msg, IsError: true}
}
