// SPDX-License-Identifier: MIT

package builtinguardians

// Small pure helpers for builtinguardians.go: defaultGuardianNoisePolicy,
// notifySeverityRank, trustRank, appendUnique, sameEventSubjects.
// Carved out during the Day 151 god-file split so the main file can stay
// focused on seeding and reconciling guardian profiles.
// Public API unchanged.

import (
	"strings"

	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/standing"
)
func defaultGuardianNoisePolicy() *roster.NoisePolicy {
	return &roster.NoisePolicy{
		SilentOnSuccess:      true,
		DisableMemoryWrites:  true,
		MinNotifySeverity:    defaultMinNotifySeverity,
		MinNotifyIntervalSec: minNotifyIntervalSec,
	}
}

func notifySeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func trustRank(level string) int {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "L0":
		return 0
	case "L1":
		return 1
	case "L2":
		return 2
	case "L3":
		return 3
	case "L4":
		return 4
	default:
		return 4
	}
}


func appendUnique(xs []string, want string) []string {
	for _, x := range xs {
		if x == want {
			return xs
		}
	}
	return append(xs, want)
}

func sameEventSubjects(triggers []standing.Trigger, subjects []string) bool {
	if len(subjects) == 0 {
		return false
	}
	got := map[string]int{}
	for _, t := range triggers {
		if t.Type != standing.TriggerEvent || t.Subject == "" {
			return false
		}
		got[t.Subject]++
	}
	if len(got) != len(subjects) {
		return false
	}
	for _, s := range subjects {
		if got[s] != 1 {
			return false
		}
	}
	return true
}
