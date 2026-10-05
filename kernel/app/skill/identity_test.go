// SPDX-License-Identifier: MIT

package skill_test

import (
	"context"
	appskill "github.com/agezt/agezt/kernel/app/skill"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"testing"
)

func TestSkillMutationServicesRetainCallerOperationIdentity(t *testing.T) {
	ctx := opapi.WithCorrelation(context.Background(), "owned-operation")
	for _, name := range []string{"promote", "quarantine", "archive", "revert", "restore", "share", "reassign", "import"} {
		t.Run(name, func(t *testing.T) {
			l := &lifecycleProbe{}
			c := &curationProbe{found: true}
			lifecycle := appskill.NewLifecycle(l)
			curation := appskill.NewCuration(c, func(string) bool { return true })
			var err error
			switch name {
			case "promote":
				_, err = lifecycle.Promote(ctx, appskill.GetInput{ID: "owned"})
			case "quarantine":
				_, err = lifecycle.Quarantine(ctx, appskill.ReasonInput{ID: "owned"})
			case "archive":
				_, err = lifecycle.Archive(ctx, appskill.ReasonInput{ID: "owned"})
			case "revert":
				_, err = lifecycle.Revert(ctx, appskill.GetInput{ID: "owned"})
			case "restore":
				_, err = lifecycle.Restore(ctx, appskill.RestoreInput{ID: "owned", Status: "draft"})
			case "share":
				_, err = curation.Share(ctx, appskill.GetInput{ID: "owned"})
			case "reassign":
				_, err = curation.Reassign(ctx, appskill.ReassignInput{ID: "owned", Agent: "reviewer"})
			case "import":
				_, err = curation.Import(ctx, appskill.ImportInput{Name: "fixture", Body: "body"})
			}
			if err != nil {
				t.Fatal(err)
			}
			corr := l.corr
			if c.calls > 0 {
				corr = c.corr
			}
			if corr != "owned-operation" {
				t.Fatalf("EXPECTED: caller operation identity reaches %s; ACTUAL: %q", name, corr)
			}
		})
	}
}
