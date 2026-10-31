// SPDX-License-Identifier: MIT

package runctx

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// The noise policy is the run's dispatch vocabulary: what the daemon may stay
// quiet about. It moved to the runs module in W3.76c; these tests moved with
// it.

func TestEffectiveAgentNoisePolicy_SystemDefaultsStayQuiet(t *testing.T) {
	got := EffectiveAgentNoisePolicy(agentapi.Profile{Slug: "guardian-health", System: true})

	if !got.SilentOnSuccess {
		t.Fatalf("system agents should be silent on success")
	}
	if !got.DisableMemoryWrites {
		t.Fatalf("system agents should not write routine memory")
	}
	if got.MinNotifySeverity != "warning" {
		t.Fatalf("min notify severity = %q, want warning", got.MinNotifySeverity)
	}
	if got.MinNotifyIntervalSec != 8*3600 {
		t.Fatalf("min notify interval = %d, want %d", got.MinNotifyIntervalSec, 8*3600)
	}
}

func TestEffectiveAgentNoisePolicy_SystemRaisesLooseExplicitPolicy(t *testing.T) {
	got := EffectiveAgentNoisePolicy(agentapi.Profile{
		Slug:   "guardian-health",
		System: true,
		NoisePolicy: &agentapi.NoisePolicy{
			MinNotifySeverity:    "info",
			MinNotifyIntervalSec: 4 * 3600,
		},
	})

	if got.MinNotifySeverity != "warning" {
		t.Fatalf("min notify severity = %q, want warning", got.MinNotifySeverity)
	}
	if got.MinNotifyIntervalSec != 8*3600 {
		t.Fatalf("min notify interval = %d, want %d", got.MinNotifyIntervalSec, 8*3600)
	}
}

func TestEffectiveAgentNoisePolicy_SilentSuccessRaisesNotifyFloor(t *testing.T) {
	got := EffectiveAgentNoisePolicy(agentapi.Profile{
		Slug: "quiet-worker",
		NoisePolicy: &agentapi.NoisePolicy{
			SilentOnSuccess: true,
		},
	})

	if got.MinNotifySeverity != "warning" {
		t.Fatalf("silent_on_success min notify severity = %q, want warning", got.MinNotifySeverity)
	}
}

func TestEffectiveAgentNoisePolicy_SystemKeepsStricterExplicitPolicy(t *testing.T) {
	got := EffectiveAgentNoisePolicy(agentapi.Profile{
		Slug:   "guardian-health",
		System: true,
		NoisePolicy: &agentapi.NoisePolicy{
			MinNotifySeverity:    "critical",
			MinNotifyIntervalSec: 12 * 3600,
		},
	})

	if got.MinNotifySeverity != "critical" {
		t.Fatalf("min notify severity = %q, want critical", got.MinNotifySeverity)
	}
	if got.MinNotifyIntervalSec != 12*3600 {
		t.Fatalf("min notify interval = %d, want %d", got.MinNotifyIntervalSec, 12*3600)
	}
}

// TestEffectiveAgentNoisePolicy_NoPolicyMeansNoPolicy pins the floor of the
// resolver: a profile with no noise policy at all gets an EMPTY policy — not a
// default one. A run with no policy is a run that may notify whenever it
// likes; the module must not quietly invent a floor.
func TestEffectiveAgentNoisePolicy_NoPolicyMeansNoPolicy(t *testing.T) {
	got := EffectiveAgentNoisePolicy(agentapi.Profile{Slug: "plain"})
	if got != (AgentNoisePolicy{}) {
		t.Fatalf("profile without a noise policy = %+v, want the empty policy", got)
	}
}

// TestNotifySeverityRank_OrdersTheSeverities pins the ranking the noise policy
// compares against: an UNKNOWN severity ranks lowest (so it never raises the
// floor above "warning"), and the ordering is strict.
func TestNotifySeverityRank_OrdersTheSeverities(t *testing.T) {
	cases := []struct {
		severity string
		want     int
	}{
		{"critical", 3},
		{"warning", 2},
		{"warn", 2},
		{"info", 1},
		{"", 1},
		{"loud", 0},
	}
	for _, tc := range cases {
		if got := NotifySeverityRank(tc.severity); got != tc.want {
			t.Fatalf("NotifySeverityRank(%q) = %d, want %d", tc.severity, got, tc.want)
		}
	}
	if NotifySeverityRank("critical") <= NotifySeverityRank("warning") {
		t.Fatal("critical must outrank warning")
	}
	if NotifySeverityRank("warning") <= NotifySeverityRank("info") {
		t.Fatal("warning must outrank info")
	}
}

// TestAgentNoisePolicyIsCarriedOnTheContext pins the half of the noise policy
// that travels with the run: WithAgentProfile stamps the RESOLVED policy on
// the context, so the daemon reads one resolved value instead of re-resolving
// it per call.
func TestAgentNoisePolicyIsCarriedOnTheContext(t *testing.T) {
	ctx := WithAgentProfile(t.Context(), agentapi.Profile{
		Slug:   "quiet-worker",
		System: true,
	})

	got, ok := AgentNoisePolicyFromCtx(ctx)
	if !ok {
		t.Fatal("the resolved noise policy must ride on the context")
	}
	if !got.SilentOnSuccess || got.MinNotifySeverity != "warning" {
		t.Fatalf("carried noise policy = %+v, want the system floor", got)
	}
}
