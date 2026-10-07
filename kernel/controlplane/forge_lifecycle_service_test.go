// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"encoding/json"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolforge"
	"github.com/agezt/agezt/plugins/providers/mock"
	"testing"
)

type forgeServiceRunner struct {
	ctx   context.Context
	calls int
	input string
}

func (r *forgeServiceRunner) RunScript(ctx context.Context, _, _, input string) (string, bool, error) {
	r.ctx = ctx
	r.calls++
	r.input = input
	return " owned output ", false, nil
}
func TestForgeLifecycleServiceOwnStoreJournalAndContext(t *testing.T) {
	runner := &forgeServiceRunner{}
	provider := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: provider, ScriptRunner: runner})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := apptools.NewForgeLifecycle(k)
	ctx := opapi.WithCorrelation(context.Background(), "owned-correlation")
	head, _ := k.Journal().Head()
	drafted, err := s.Draft(ctx, apptools.ForgeDraftInput{CorrelationID: "untrusted", Tool: toolforge.ScriptTool{ID: "poison", Name: "owned", Description: "desc", Language: "python", Code: "never executed code", Status: toolforge.StatusActive, TestedOK: true, CreatedMS: 1}})
	if err != nil || drafted.Tool.ID == "poison" || drafted.Tool.Status != toolforge.StatusDraft || drafted.Tool.TestedOK || drafted.Tool.CreatedMS == 1 {
		t.Fatal(drafted, err)
	}
	edited, err := s.Edit(ctx, apptools.ForgeEditInput{Ref: drafted.Tool.ID, Tool: toolforge.ScriptTool{Description: "new description", Name: "poison", ID: "poison", Status: toolforge.StatusActive}})
	if err != nil || edited.Tool.Name != "owned" || edited.Tool.ID != drafted.Tool.ID || edited.Tool.Status != toolforge.StatusDraft || edited.Tool.Description != "new description" {
		t.Fatal(edited, err)
	}
	if _, err := s.Promote(ctx, apptools.ForgeRefInput{Ref: "owned"}); err == nil {
		t.Fatal("untested promotion admitted")
	}
	tested, err := s.Test(ctx, apptools.ForgeTestInput{Ref: "owned", Sample: " ", CorrelationID: "untrusted"})
	if err != nil || !tested.OK || tested.Output != " owned output " || runner.ctx != ctx || runner.calls != 1 || runner.input != "{}" {
		t.Fatal(tested, err, runner)
	}
	promoted, err := s.Promote(ctx, apptools.ForgeRefInput{Ref: "owned"})
	if err != nil || promoted.Tool.Status != toolforge.StatusActive || promoted.Tool.CallableAs != "forge_owned" {
		t.Fatal(promoted, err)
	}
	quarantined, err := s.Quarantine(ctx, apptools.ForgeQuarantineInput{Ref: "owned", Reason: " raw reason "})
	if err != nil || quarantined.Tool.Status != toolforge.StatusQuarantined || quarantined.Tool.CallableAs != "" {
		t.Fatal(quarantined, err)
	}
	removed, err := s.Remove(ctx, apptools.ForgeRefInput{Ref: "owned"})
	if err != nil || !removed.Removed || k.ToolForge().Count() != 0 {
		t.Fatal(removed, err)
	}
	events, err := k.Journal().Tail(100)
	if err != nil {
		t.Fatal(err)
	}
	kinds := []event.Kind{event.KindScriptToolCreated, event.KindScriptToolUpdated, event.KindScriptToolTested, event.KindScriptToolPromoted, event.KindScriptToolQuarantined, event.KindScriptToolRemoved}
	seen := 0
	for _, e := range events {
		if e.Seq <= head {
			continue
		}
		if seen >= len(kinds) || e.Kind != kinds[seen] || e.Subject != "toolforge.owned" || e.Actor != "toolforge" || e.CorrelationID != "owned-correlation" {
			t.Fatal(seen, e)
		}
		if e.Kind == event.KindScriptToolQuarantined {
			var payload map[string]any
			_ = json.Unmarshal(e.Payload, &payload)
			if payload["reason"] != "raw reason" {
				t.Fatal(payload)
			}
		}
		seen++
	}
	if seen != 6 || provider.CallCount() != 0 {
		t.Fatal(seen, provider.CallCount())
	}
}
