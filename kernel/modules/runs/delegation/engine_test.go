// SPDX-License-Identifier: MIT

package delegation

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/prompt"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
)

// fakeSkills is a stand-in for the skill library's curator. Only the two
// activation doors are used by the prompt service; the rest of the curator
// interface is embedded so a call to an unimplemented door panics loudly
// instead of silently answering.
type fakeSkills struct {
	skillapi.Curator
	active []skillapi.Scored
}

func (s *fakeSkills) ActivateFor(corr, agentSlug, intent string, limit int) ([]skillapi.Scored, error) {
	return s.active, nil
}

func (s *fakeSkills) ActivateExplicitFor(corr, agentSlug, intent string, refs []string, limit int) ([]skillapi.Scored, []string, error) {
	return s.active, nil, nil
}

// newSkillService builds a prompt service that activates one skill, so the
// engine's skill-outcome path is reachable.
func newSkillService(t *testing.T, h *fakeHost) *prompt.Service {
	t.Helper()
	return prompt.New(prompt.Config{SkillInject: true}, promptHost{h}, prompt.Deps{
		Skills: &fakeSkills{active: []skillapi.Scored{{Skill: skillapi.Skill{ID: "skill-1"}, Score: 1}}},
	})
}

func TestPrepare_TheProfileModelIsTheDefault(t *testing.T) {
	h := newFakeHost()
	h.profiles["named"] = agentapi.Profile{Slug: "named", Enabled: true, Model: "profile-model"}
	svc := newService(t, h, Config{})
	plan, err := svc.prepare(context.Background(), Request{Task: "t", AgentRef: "named"}, false)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if plan.SubModel != "profile-model" {
		t.Errorf("model = %q, want the profile's model", plan.SubModel)
	}
}

func TestPrepare_TheChildContextCarriesTheDepthRootAndActor(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	ctx := runctx.WithRoot(runctx.WithActor(context.Background(), "lead", "parent-corr"), "root-corr")
	plan, err := svc.prepare(ctx, Request{Task: "t"}, false)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if got := DepthFromCtx(plan.ChildCtx); got != 1 {
		t.Errorf("child depth = %d, want 1", got)
	}
	if got := runctx.RootFromCtx(plan.ChildCtx); got != "root-corr" {
		t.Errorf("child root = %q, want root-corr", got)
	}
	if got := runctx.CorrelationFromCtx(plan.ChildCtx); got != plan.ChildCorr {
		t.Errorf("child correlation = %q, want %q", got, plan.ChildCorr)
	}
	if got := runctx.ActorFromCtx(plan.ChildCtx); got != plan.Actor {
		t.Errorf("child actor = %q, want %q", got, plan.Actor)
	}
}

// TestPrepare_TheChildInheritsTheTreeRootFromItsParent covers the case where
// the spawning run IS the tree root: the child must carry that root, so the
// tree-wide bound (M629) keeps counting every descendant against the same
// root instead of re-seeding at each level.
func TestPrepare_TheChildInheritsTheTreeRootFromItsParent(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	ctx := runctx.WithActor(context.Background(), "lead", "parent-corr")
	plan, err := svc.prepare(ctx, Request{Task: "t"}, false)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if plan.RootCorr != "parent-corr" {
		t.Errorf("root = %q, want the spawning run's correlation", plan.RootCorr)
	}
	if got := runctx.RootFromCtx(plan.ChildCtx); got != "parent-corr" {
		t.Errorf("child root = %q, want parent-corr", got)
	}
}

func TestRun_RecordsTheSkillOutcome(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	svc.deps.Prompt = newSkillService(t, h)
	if _, err := svc.Run(context.Background(), Request{Task: "t"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(h.outcomes) != 1 {
		t.Fatalf("skill outcomes = %d, want 1", len(h.outcomes))
	}
	if len(h.outcomes[0].skillIDs) == 0 {
		t.Errorf("the outcome carries no skill ids: %+v", h.outcomes[0])
	}
	if !h.outcomes[0].ok {
		t.Error("a successful child must record a successful outcome")
	}
}

func TestRun_CompletesTheChildsLifecycle(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	plan, err := svc.prepare(context.Background(), Request{Task: "t"}, false)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if _, err := svc.execute(plan); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(h.lifecycles) != 1 || h.lifecycles[0] != plan.ChildCorr {
		t.Errorf("completed lifecycles = %v, want the child's correlation", h.lifecycles)
	}
}

func TestRun_RetriesAccordingToThePolicy(t *testing.T) {
	h := newFakeHost()
	h.profiles["flaky"] = agentapi.Profile{
		Slug:    "flaky",
		Enabled: true,
		RetryPolicy: &agentapi.RetryPolicy{
			MaxAttempts:  3,
			BaseDelaySec: 0, // no wait — the test must not sleep
			RetryOn:      []string{"error"},
		},
	}
	svc := newService(t, h, Config{})
	h.runFailures = 1 // the first child attempt fails
	ans, err := svc.Run(context.Background(), Request{Task: "t", AgentRef: "flaky"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans != "child answer" {
		t.Errorf("answer = %q, want the retried child's answer", ans)
	}
	if h.loops != 2 {
		t.Errorf("child attempts = %d, want 2 (one failure, one retry)", h.loops)
	}
	found := false
	for _, ev := range h.events {
		if ev.Kind == event.KindAgentRetry {
			found = true
			if !strings.Contains(ev.Subject, "flaky") {
				t.Errorf("retry subject = %q, want the agent's own subject", ev.Subject)
			}
		}
	}
	if !found {
		t.Error("the retry must be journaled")
	}
}

func TestRun_AGivingUpAfterThePolicyIsExhausted(t *testing.T) {
	h := newFakeHost()
	h.profiles["flaky"] = agentapi.Profile{
		Slug:        "flaky",
		Enabled:     true,
		RetryPolicy: &agentapi.RetryPolicy{MaxAttempts: 2, BaseDelaySec: 0},
	}
	svc := newService(t, h, Config{})
	h.runFailures = 99 // every attempt fails
	_, err := svc.Run(context.Background(), Request{Task: "t", AgentRef: "flaky"})
	if err == nil {
		t.Fatal("an exhausted retry policy must fail the delegation")
	}
	if h.loops != 2 {
		t.Errorf("child attempts = %d, want 2 (the policy's maximum)", h.loops)
	}
}

func TestAwait_CollectsOnlyOnce(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	id, err := svc.Spawn(context.Background(), Request{Task: "t"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if _, err := svc.Await(context.Background(), id); err != nil {
		t.Fatalf("Await: %v", err)
	}
	res, err := svc.Await(context.Background(), id)
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "unknown spawn id") {
		t.Fatalf("second await = %+v, want the handle to be consumed", res)
	}
}

func TestSpawn_RegistersTheChildAndAnnouncesTheCompletion(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	id, err := svc.Spawn(context.Background(), Request{Task: "t"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	// The child is registered in the live-run registry, so Halt and CancelRun
	// reach it exactly as they reach a root run.
	if got := svc.deps.Registry.Count(); got != 1 {
		t.Errorf("registered runs = %d, want 1 (the async child)", got)
	}
	if _, err := svc.Await(context.Background(), id); err != nil {
		t.Fatalf("Await: %v", err)
	}
	// The completion is announced under the parent correlation.
	deadline := time.Now().Add(2 * time.Second)
	for {
		found := false
		for _, ev := range h.events {
			if ev.Kind == event.KindSubAgentCompleted && ev.CorrelationID == id {
				found = true
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the async completion was never announced")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
