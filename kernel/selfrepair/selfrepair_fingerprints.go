// SPDX-License-Identifier: MIT

// Self-repair generic fingerprint + reason + escalation helpers (M846):
// the pure-logic helpers used by both the coordinator (claim/handleTick/
// dispatch) and the apply pipeline (autoEscalate/autoWake).
// Extracted from selfrepair_fingerprints.go during the Day-202 god-file split.
// Public API unchanged.
package selfrepair

import (
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/roster"
)

func autoRepairPayloadString(pl map[string]any, key string) string {
	if pl == nil {
		return ""
	}
	v, _ := pl[key].(string)
	return strings.TrimSpace(v)
}

func autoRepairEscalationTarget(p roster.Profile) string {
	for _, ref := range []string{strings.TrimSpace(p.ParentAgent), strings.TrimSpace(p.OwnerAgent)} {
		if ref != "" && !strings.EqualFold(ref, p.Slug) {
			return ref
		}
	}
	return ""
}

func autoRepairEscalationFrom(p roster.Profile) string {
	if p.HealthPolicy != nil {
		if doc := strings.TrimSpace(p.HealthPolicy.DoctorAgent); doc != "" {
			return doc
		}
	}
	return "system:doctor"
}

func autoRepairFingerprint(parts ...string) string {
	cp := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			cp = append(cp, part)
		}
	}
	if len(cp) == 0 {
		return ""
	}
	sort.Strings(cp)
	return strings.Join(cp, "\n")
}

func autoRepairReason(issues []string) string {
	base := "deterministic auto-repair: invalid runtime override(s)"
	if len(issues) == 0 {
		return base
	}
	text := strings.Join(issues, "; ")
	if len(text) > 700 {
		text = text[:700] + "..."
	}
	return base + ": " + text
}
