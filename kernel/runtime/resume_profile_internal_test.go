// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/resume"
	"github.com/agezt/agezt/kernel/roster"
)

func TestResumeTicket_ProfileDefaultsAndExplicitOverrides(t *testing.T) {
	profile := roster.Profile{Slug: "ops", Soul: "ops soul", Model: "ops-model", TrustCeiling: "L2"}
	for _, tc := range []struct {
		name      string
		decorate  func(context.Context) context.Context
		resumable bool
	}{
		{"profile defaults", func(ctx context.Context) context.Context { return ctx }, true},
		{"explicit system", func(ctx context.Context) context.Context { return WithSystem(ctx, "custom") }, false},
		{"explicit model", func(ctx context.Context) context.Context { return WithModel(ctx, "custom") }, false},
		{"same system explicitly", func(ctx context.Context) context.Context { return WithSystem(ctx, AgentProfileSystem(profile)) }, false},
		{"same model explicitly", func(ctx context.Context) context.Context { return WithModel(ctx, profile.Model) }, false},
		{"explicit tools", func(ctx context.Context) context.Context { return WithTools(ctx, []string{"echo"}) }, false},
		{"explicit empty tools", func(ctx context.Context) context.Context { return WithTools(ctx, []string{}) }, false},
		{"retargeted identity", func(ctx context.Context) context.Context { return WithAgentIdent(ctx, "other", 0) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := WithAgentProfile(context.Background(), profile)
			ctx = WithMaxCost(ctx, 321)
			ctx = tc.decorate(ctx)
			ticket := (&Kernel{}).buildResumeTicket(ctx, "corr", "task", resume.KindRun, 0)
			if ticket.Resumable != tc.resumable {
				t.Fatalf("Resumable=%v want %v", ticket.Resumable, tc.resumable)
			}
			if ticket.TrustCeiling == nil || *ticket.TrustCeiling != int(edict.LevelAskFirst) || ticket.MaxCostMc != 321 {
				t.Fatalf("governance lost: %+v", ticket)
			}
		})
	}
	ctx := WithAgentProfile(context.Background(), roster.Profile{Soul: "unnamed soul", Model: "unnamed-model"})
	if (&Kernel{}).buildResumeTicket(ctx, "unnamed", "task", resume.KindRun, 0).Resumable {
		t.Fatal("unnamed profile cannot be reconstructed")
	}
}
