// SPDX-License-Identifier: MIT

// controlplane/standing: wire-shape helpers (standingView +
// standingFrequencyWarning) used by the CRUD handlers.
// Split from standing.go during Day 211 god-file refactor (#46).
// Public API unchanged.
package controlplane

import (
	"encoding/json"
	"strings"

	"github.com/agezt/agezt/kernel/standing"
)


// standingView is the stable wire shape for one order.
func standingView(o standing.Order) map[string]any {
	b, _ := json.Marshal(o)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if warning := standingFrequencyWarning(o); warning != "" {
		m["frequency_warning"] = warning
	}
	return m
}

func standingFrequencyWarning(o standing.Order) string {
	hasEvent := false
	for _, t := range o.Triggers {
		if t.Type == standing.TriggerEvent {
			hasEvent = true
		}
		if t.Type == standing.TriggerCron {
			first := ""
			if fields := strings.Fields(strings.TrimSpace(t.Schedule)); len(fields) > 0 {
				first = fields[0]
			}
			if first == "*" || first == "*/1" || first == "0/1" {
				return "cron trigger may wake this standing order every minute"
			}
		}
	}
	if hasEvent && o.CooldownSec > 0 && o.CooldownSec < 15*60 {
		return "event cooldown is below the default 15m guard"
	}
	return ""
}
