// SPDX-License-Identifier: MIT

package resume

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// TestBuild_CapturesTheRunsDispatchContext: the ticket records what the run
// was, under which governance, and why it exists — so a restart can continue it
// without losing a tightened ceiling or guessing a cap.
func TestBuild_CapturesTheRunsDispatchContext(t *testing.T) {
	ceil := 2
	ticket := Build(Facts{
		Corr:         "corr-1",
		Intent:       "do the thing",
		Kind:         KindRun,
		AssureBudget: 3,
		AgentSlug:    "ops",
		MaxCostMc:    5000,
		TrustCeiling: &ceil,
		RunTimeoutMs: 1500,
		Wake: agentapi.WakeContext{
			Source:         "standing",
			Reason:         "daily digest",
			ScheduleID:     "sched-1",
			StandingID:     "stand-1",
			StandingName:   "digest",
			TriggerSubject: "repo",
		},
	})

	if ticket.Corr != "corr-1" || ticket.Intent != "do the thing" || ticket.Kind != KindRun {
		t.Fatalf("identity lost: %+v", ticket)
	}
	if ticket.AssureBudget != 3 || ticket.AgentSlug != "ops" {
		t.Fatalf("dispatch lost: %+v", ticket)
	}
	if ticket.MaxCostMc != 5000 || ticket.RunTimeoutMs != 1500 {
		t.Fatalf("governance ceilings lost: %+v", ticket)
	}
	if ticket.TrustCeiling == nil || *ticket.TrustCeiling != ceil {
		t.Fatalf("trust ceiling lost: %+v", ticket.TrustCeiling)
	}
	if ticket.WakeSource != "standing" || ticket.WakeReason != "daily digest" ||
		ticket.WakeScheduleID != "sched-1" || ticket.WakeStandingID != "stand-1" ||
		ticket.WakeStandingName != "digest" || ticket.WakeTriggerSubject != "repo" {
		t.Fatalf("wake provenance lost: %+v", ticket)
	}
	if ticket.Status != StatusActive {
		t.Fatalf("status = %q, want active", ticket.Status)
	}
}

// TestBuild_NoTrustCeilingIsNotAnError: a run that never had a ceiling records
// none (nil) — "never set" is legitimate, and must not be confused with a set
// value.
func TestBuild_NoTrustCeilingIsNotAnError(t *testing.T) {
	ticket := Build(Facts{Corr: "corr-2", Intent: "task", Kind: KindRun})
	if ticket.TrustCeiling != nil {
		t.Fatalf("trust ceiling = %v, want nil", ticket.TrustCeiling)
	}
}

// TestBuild_TheResumabilityVerdictIsTheModules: the module judges whether the
// run may resume from the ticket it just built.
func TestBuild_TheResumabilityVerdictIsTheModules(t *testing.T) {
	profile := Setting{Set: true, SourceProfile: "ops"}
	for _, tc := range []struct {
		name  string
		facts Facts
		want  bool
	}{
		{
			name: "profile defaults are reconstructible",
			facts: Facts{
				AgentSlug: "ops",
				System:    profile,
				Model:     profile,
			},
			want: true,
		},
		{
			name: "a per-run system override is not reconstructible",
			facts: Facts{
				AgentSlug: "ops",
				System:    Setting{Set: true},
				Model:     profile,
			},
			want: false,
		},
		{
			name: "a per-run tool allowlist is not reconstructible",
			facts: Facts{
				AgentSlug:       "ops",
				System:          profile,
				Model:           profile,
				ToolsOverridden: true,
			},
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ticket := Build(tc.facts)
			if ticket.Resumable != tc.want {
				t.Fatalf("Resumable = %v, want %v", ticket.Resumable, tc.want)
			}
		})
	}
}
