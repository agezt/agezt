// SPDX-License-Identifier: MIT
package selfrepair

import (
	"encoding/json"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

func autoRepairShouldHandle(ev *event.Event) bool {
	if ev == nil || ev.Subject != autoRepairPulseSubject || len(ev.Payload) == 0 {
		return false
	}
	var payload struct {
		Kind  string `json:"kind"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return false
	}
	return payload.Error == "" && payload.Kind == "reaper_candidates"
}
func autoRepairEligible(p roster.Profile) bool {
	if p.System || !p.Enabled || p.Retired || !p.AllowsDirectCall() {
		return false
	}
	return p.SelfRepairPolicy != nil && p.SelfRepairPolicy.Enabled
}
func autoRepairMaxAttempts(p roster.Profile) int {
	if p.SelfRepairPolicy == nil || p.SelfRepairPolicy.MaxAttempts <= 0 {
		return 0
	}
	return p.SelfRepairPolicy.MaxAttempts
}
func previousAutoRepairAttempts(k *kernelruntime.Kernel, slug, fingerprint string) int {
	if k == nil || k.Journal() == nil || strings.TrimSpace(slug) == "" || strings.TrimSpace(fingerprint) == "" {
		return 0
	}
	count := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindInfo || e.Subject != autoRepairEventSubject {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if strings.TrimSpace(autoRepairPayloadString(pl, "agent")) != slug || strings.TrimSpace(autoRepairPayloadString(pl, "fingerprint")) != fingerprint {
			return nil
		}
		switch strings.TrimSpace(autoRepairPayloadString(pl, "phase")) {
		case "queued", "routing_rollback_queued":
			count++
		}
		return nil
	})
	return count
}
