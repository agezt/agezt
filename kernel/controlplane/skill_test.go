// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestSkillLifecycleOverControlPlane(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Seed a draft directly via the kernel's Forge.
	sk, _, err := k.Forge().Create("seed", skill.CreateSpec{
		Name: "diagnose-ci", Description: "diagnose failing CI", Body: "do it",
	})
	if err != nil {
		t.Fatal(err)
	}

	// List shows it (1 skill, 0 active).
	res, err := c.Call(ctx, controlplane.CmdSkillList, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if cnt, _ := res["count"].(float64); cnt != 1 {
		t.Fatalf("count = %v want 1", res["count"])
	}
	if ac, _ := res["active_count"].(float64); ac != 0 {
		t.Errorf("active_count = %v want 0 (still a draft)", res["active_count"])
	}

	// Promote draft→shadow→active.
	res, err = c.Call(ctx, controlplane.CmdSkillPromote, map[string]any{"id": sk.ID})
	if err != nil || res["status"] != "shadow" {
		t.Fatalf("promote 1: status=%v err=%v", res["status"], err)
	}
	res, _ = c.Call(ctx, controlplane.CmdSkillPromote, map[string]any{"id": sk.ID})
	if res["status"] != "active" {
		t.Fatalf("promote 2: status=%v", res["status"])
	}

	// History folds the lifecycle chain (created + 2 promoted).
	res, err = c.Call(ctx, controlplane.CmdSkillHistory, map[string]any{"id": sk.ID})
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if cnt, _ := res["count"].(float64); cnt < 3 {
		t.Errorf("history count = %v want >=3", res["count"])
	}
}

func TestSkillPromoteIllegalErrors(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	sk, _, _ := k.Forge().Create("seed", skill.CreateSpec{Name: "s", Body: "b"})
	// draft→shadow→active, then a 3rd promote is illegal.
	_, _ = c.Call(context.Background(), controlplane.CmdSkillPromote, map[string]any{"id": sk.ID})
	_, _ = c.Call(context.Background(), controlplane.CmdSkillPromote, map[string]any{"id": sk.ID})
	if _, err := c.Call(context.Background(), controlplane.CmdSkillPromote, map[string]any{"id": sk.ID}); err == nil {
		t.Error("promoting an active skill should error")
	}
}

func TestSkillListEmpty(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdSkillList, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if sks, ok := res["skills"].([]any); !ok || len(sks) != 0 {
		t.Fatalf("empty skill list should be [], got %v", res["skills"])
	}
}
