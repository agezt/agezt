// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// promoteToActive walks a freshly created draft up to active.
func promoteToActive(t *testing.T, f *skill.Forge, id string) {
	t.Helper()
	if _, err := f.Promote("seed", id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Promote("seed", id); err != nil {
		t.Fatal(err)
	}
}

func TestActiveSkillInjectedIntoSystemPrompt(t *testing.T) {
	prov := mock.New(mock.FinalText("answered"))
	var gotSystem string
	prov.OnRequest = func(req agent.CompletionRequest) { gotSystem = req.System }

	k, err := runtime.Open(runtime.Config{
		BaseDir:     t.TempDir(),
		Provider:    prov,
		System:      "base prompt",
		SkillInject: true,
		SkillTopK:   3,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	sk, _, err := k.Forge().Create("seed", skill.CreateSpec{
		Name: "diagnose-ci", Description: "diagnose failing CI", Triggers: []string{"ci"},
		Body: "1. read the logs 2. find the first error",
	})
	if err != nil {
		t.Fatal(err)
	}
	promoteToActive(t, k.Forge(), sk.ID)

	if _, _, err := k.Run(context.Background(), "my CI build keeps failing"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotSystem, "read the logs") {
		t.Fatalf("active skill body was not injected; got:\n%s", gotSystem)
	}
	if !strings.Contains(gotSystem, "base prompt") {
		t.Fatal("original system prompt must be preserved")
	}
	if countKind(t, k, event.KindSkillActivated) != 1 {
		t.Fatal("injection must journal skill.activated for `agt why`")
	}
}

func TestDraftSkillNotInjected(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	var gotSystem string
	prov.OnRequest = func(req agent.CompletionRequest) { gotSystem = req.System }

	k, err := runtime.Open(runtime.Config{
		BaseDir: t.TempDir(), Provider: prov, System: "base", SkillInject: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	// A draft (never promoted) must not reach a run.
	_, _, _ = k.Forge().Create("seed", skill.CreateSpec{Name: "x", Description: "deploy app", Body: "secret steps"})
	if _, _, err := k.Run(context.Background(), "deploy the app"); err != nil {
		t.Fatal(err)
	}
	if gotSystem != "base" {
		t.Fatalf("draft skill must not be injected, got %q", gotSystem)
	}
}

func TestExplicitSkillDirectiveActivatesNamedSkill(t *testing.T) {
	prov := mock.New(mock.FinalText("answered"))
	var gotSystem, gotUser string
	prov.OnRequest = func(req agent.CompletionRequest) {
		gotSystem = req.System
		if len(req.Messages) > 0 {
			gotUser = req.Messages[0].Content
		}
	}

	k, err := runtime.Open(runtime.Config{
		BaseDir:     t.TempDir(),
		Provider:    prov,
		System:      "base prompt",
		SkillInject: true,
		SkillTopK:   3,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	sk, _, err := k.Forge().Create("seed", skill.CreateSpec{
		Name:        "diagnose-ci",
		Description: "only activates by explicit name in this test",
		Body:        "always inspect the CI log artifact first",
	})
	if err != nil {
		t.Fatal(err)
	}
	promoteToActive(t, k.Forge(), sk.ID)

	if _, _, err := k.Run(context.Background(), "/skill diagnose-ci\nplease help with this flaky build"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotSystem, "inspect the CI log artifact") {
		t.Fatalf("explicit skill body was not injected; got:\n%s", gotSystem)
	}
	if gotUser != "please help with this flaky build" {
		t.Fatalf("skill directive should be stripped from task prompt, got %q", gotUser)
	}
	var payload struct {
		Activation string   `json:"activation"`
		Refs       []string `json:"refs"`
		IDs        []string `json:"ids"`
	}
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindSkillActivated {
			_ = json.Unmarshal(e.Payload, &payload)
		}
		return nil
	})
	if payload.Activation != "explicit" || len(payload.Refs) != 1 || payload.Refs[0] != "diagnose-ci" || len(payload.IDs) != 1 || payload.IDs[0] != sk.ID {
		t.Fatalf("skill.activated payload = %+v", payload)
	}
}

func TestForgeProposesAfterMultiToolRun(t *testing.T) {
	prov := mock.New(
		mock.ToolUse("c1", "shell", map[string]string{"command": "echo hi"}),
		mock.FinalText("done the multi-step thing"),
		mock.FinalText(`{"skill":{"name":"do-the-thing","description":"a reusable procedure","triggers":["ops"],"body":"step one then step two","tools":["shell"]}}`),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:            t.TempDir(),
		Provider:           prov,
		Tools:              map[string]agent.Tool{"shell": shell.New()},
		SkillForge:         true,
		SkillForgeMinTools: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Run(context.Background(), "do a multi-step task"); err != nil {
		t.Fatal(err)
	}
	all, _ := k.Forge().List()
	if len(all) != 1 || all[0].Status != skill.StatusDraft || all[0].Name != "do-the-thing" {
		t.Fatalf("expected one proposed draft skill, got %+v", all)
	}
}

func TestForgeAndSkillOffByDefault(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	var gotSystem string
	prov.OnRequest = func(req agent.CompletionRequest) { gotSystem = req.System }
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: prov, System: "base"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	sk, _, _ := k.Forge().Create("seed", skill.CreateSpec{Name: "x", Description: "y", Body: "b"})
	promoteToActive(t, k.Forge(), sk.ID)
	if _, _, err := k.Run(context.Background(), "x y"); err != nil {
		t.Fatal(err)
	}
	if gotSystem != "base" {
		t.Fatalf("with SkillInject off the prompt must be untouched, got %q", gotSystem)
	}
}
