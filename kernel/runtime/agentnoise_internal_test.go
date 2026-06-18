// SPDX-License-Identifier: MIT

package runtime

import (
	"testing"

	"github.com/agezt/agezt/kernel/roster"
)

func TestEffectiveAgentNoisePolicy_SystemDefaultsStayQuiet(t *testing.T) {
	got := effectiveAgentNoisePolicy(roster.Profile{Slug: "guardian-health", System: true})

	if !got.silentOnSuccess {
		t.Fatalf("system agents should be silent on success")
	}
	if !got.disableMemoryWrites {
		t.Fatalf("system agents should not write routine memory")
	}
	if got.minNotifySeverity != "warning" {
		t.Fatalf("min notify severity = %q, want warning", got.minNotifySeverity)
	}
	if got.minNotifyIntervalSec != 8*3600 {
		t.Fatalf("min notify interval = %d, want %d", got.minNotifyIntervalSec, 8*3600)
	}
}

func TestEffectiveAgentNoisePolicy_SystemRaisesLooseExplicitPolicy(t *testing.T) {
	got := effectiveAgentNoisePolicy(roster.Profile{
		Slug:   "guardian-health",
		System: true,
		NoisePolicy: &roster.NoisePolicy{
			MinNotifySeverity:    "info",
			MinNotifyIntervalSec: 4 * 3600,
		},
	})

	if got.minNotifySeverity != "warning" {
		t.Fatalf("min notify severity = %q, want warning", got.minNotifySeverity)
	}
	if got.minNotifyIntervalSec != 8*3600 {
		t.Fatalf("min notify interval = %d, want %d", got.minNotifyIntervalSec, 8*3600)
	}
}

func TestEffectiveAgentNoisePolicy_SilentSuccessRaisesNotifyFloor(t *testing.T) {
	got := effectiveAgentNoisePolicy(roster.Profile{
		Slug: "quiet-worker",
		NoisePolicy: &roster.NoisePolicy{
			SilentOnSuccess: true,
		},
	})

	if got.minNotifySeverity != "warning" {
		t.Fatalf("silent_on_success min notify severity = %q, want warning", got.minNotifySeverity)
	}
}

func TestEffectiveAgentNoisePolicy_SystemKeepsStricterExplicitPolicy(t *testing.T) {
	got := effectiveAgentNoisePolicy(roster.Profile{
		Slug:   "guardian-health",
		System: true,
		NoisePolicy: &roster.NoisePolicy{
			MinNotifySeverity:    "critical",
			MinNotifyIntervalSec: 12 * 3600,
		},
	})

	if got.minNotifySeverity != "critical" {
		t.Fatalf("min notify severity = %q, want critical", got.minNotifySeverity)
	}
	if got.minNotifyIntervalSec != 12*3600 {
		t.Fatalf("min notify interval = %d, want %d", got.minNotifyIntervalSec, 12*3600)
	}
}
