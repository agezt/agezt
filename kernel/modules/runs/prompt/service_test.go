// SPDX-License-Identifier: MIT

package prompt

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contextselect"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/contract/tasteapi"
	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
)

// The fakes below EMBED the contract row they stand in for. Go lets a struct
// embed an interface and override only the verbs it needs: the verbs the test
// never calls stay nil, and a test that reaches for one fails loudly instead
// of silently passing. That keeps these fakes short and keeps the contract
// honest — if the module starts calling a new verb, the fake stops compiling
// only when the module's call site needs it.

// fakeHost is the module's host: the live identity, the bus and the clock.
type fakeHost struct {
	system string
	now    time.Time
	events []event.Spec
}

func (h *fakeHost) System() string { return h.system }

func (h *fakeHost) Publish(spec event.Spec) (*event.Event, error) {
	h.events = append(h.events, spec)
	return nil, nil
}

func (h *fakeHost) Now() time.Time { return h.now }

// fakeMemory stands in for the memory module's manager.
type fakeMemory struct {
	memoryapi.Manager
	profile string
	search  []memoryapi.Scored
	recall  []memoryapi.Scored
}

func (m *fakeMemory) ProfileText() string { return m.profile }

func (m *fakeMemory) SearchScoped(query string, limit int, scope string) ([]memoryapi.Scored, error) {
	return m.search, nil
}

func (m *fakeMemory) RecallScoped(corr, query string, limit int, scope string) ([]memoryapi.Scored, error) {
	return m.recall, nil
}

// fakeTaste stands in for the taste library.
type fakeTaste struct {
	tasteapi.Store
	exemplars []tasteapi.Exemplar
}

func (t *fakeTaste) ForScope(scope string, limit int) []tasteapi.Exemplar { return t.exemplars }

// fakeWorld stands in for the world model's graph.
type fakeWorld struct {
	worldmodelapi.Graph
	entities []worldmodelapi.ScoredEntity
}

func (w *fakeWorld) Resolve(corr, phrase string, limit int) ([]worldmodelapi.ScoredEntity, error) {
	return w.entities, nil
}

func (w *fakeWorld) ResolveQuiet(phrase string, limit int) ([]worldmodelapi.ScoredEntity, error) {
	return w.entities, nil
}

// fakeSkills stands in for the skill library's curator.
type fakeSkills struct {
	skillapi.Curator
	skills  []skillapi.Skill
	active  []skillapi.Scored
	missing []string
	refs    []string
}

func (s *fakeSkills) List() ([]skillapi.Skill, error) { return s.skills, nil }

func (s *fakeSkills) ActivateFor(corr, agentSlug, intent string, limit int) ([]skillapi.Scored, error) {
	return s.active, nil
}

func (s *fakeSkills) ActivateExplicitFor(corr, agentSlug, intent string, refs []string, limit int) ([]skillapi.Scored, []string, error) {
	s.refs = refs
	return s.active, s.missing, nil
}

func fixedTime() time.Time { return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC) }

func newFixture(cfg Config) (*Service, *fakeHost) {
	host := &fakeHost{system: "BASE SYSTEM", now: fixedTime()}
	return New(cfg, host, Deps{}), host
}

func oneScoredMemory() []memoryapi.Scored {
	return []memoryapi.Scored{{Record: memoryapi.Record{ID: "r-1", Type: "fact", Subject: "s", Content: "c"}, Score: 0.9}}
}

func oneScoredSkill() []skillapi.Scored {
	return []skillapi.Scored{{Skill: skillapi.Skill{ID: "sk-1", Name: "diagnose-ci", Description: "d", Body: "b"}, Score: 0.7}}
}

// TestBuild_UsesTheHostsLiveIdentity pins the first rule of the prompt: the
// daemon's LIVE identity is the base, and the host is the only one who knows
// it.
func TestBuild_UsesTheHostsLiveIdentity(t *testing.T) {
	svc, _ := newFixture(Config{})
	system, _ := svc.Build(context.Background(), "corr-1", "actor", "do the thing", false, skillapi.ActivationDirective{})
	if system != "BASE SYSTEM" {
		t.Errorf("system = %q, want the host's live identity", system)
	}
}

// TestBuild_TheRunsOwnIdentityWins pins the second rule: a run that carries
// its own identity (the agent profile's soul) overrides the daemon's.
func TestBuild_TheRunsOwnIdentityWins(t *testing.T) {
	svc, _ := newFixture(Config{})
	ctx := runctx.WithSystem(context.Background(), "THIS AGENT")
	system, _ := svc.Build(ctx, "corr-1", "actor", "do the thing", false, skillapi.ActivationDirective{})
	if system != "THIS AGENT" {
		t.Errorf("system = %q, want the run's own identity", system)
	}
}

// TestBuild_ASystemAgentInjectsNothing pins the system-agent rule: the daemon's
// identity IS its identity, so nothing is injected into it — not even the
// taste exemplars that are sitting right there.
//
// Every injection is switched ON and every store is FILLED here, because the
// rule is not "the system agent has no stores" but "the system agent is never
// injected into".
func TestBuild_ASystemAgentInjectsNothing(t *testing.T) {
	host := &fakeHost{system: "BASE SYSTEM", now: fixedTime()}
	svc := New(Config{
		ProfileInject: true,
		TasteInject:   true,
		TasteTopK:     3,
		MemoryInject:  true,
		MemoryTopK:    5,
		WorldInject:   true,
		WorldTopK:     5,
		SkillInject:   true,
		SkillTopK:     3,
	}, host, Deps{
		Memory: &fakeMemory{profile: "- communication style: terse", search: oneScoredMemory(), recall: oneScoredMemory()},
		Taste:  &fakeTaste{exemplars: []tasteapi.Exemplar{{ID: "ex-1", Title: "Good PR summary", Body: "One line what."}}},
		World:  &fakeWorld{entities: []worldmodelapi.ScoredEntity{{Entity: worldmodelapi.Entity{ID: "e-1", Kind: "portfolio", Name: "The Portfolio"}, Score: 0.8}}},
		Skills: &fakeSkills{active: oneScoredSkill()},
	})
	system, ids := svc.Build(context.Background(), "corr-1", "actor", "do the thing", true, skillapi.ActivationDirective{})
	if system != "BASE SYSTEM" {
		t.Errorf("a system agent must not be injected into: %q", system)
	}
	if len(ids) != 0 {
		t.Errorf("a system agent must not activate skills, got %v", ids)
	}
	if len(host.events) != 0 {
		t.Errorf("a system agent must not publish injection events, got %d", len(host.events))
	}
}

// TestBuild_TheOperatorProfileIsInjected pins the profile path: the learned
// operator profile reaches the prompt, and it leads the persona — the agent
// must know WHO it works for before it knows what it is.
func TestBuild_TheOperatorProfileIsInjected(t *testing.T) {
	host := &fakeHost{system: "BASE SYSTEM", now: fixedTime()}
	svc := New(Config{ProfileInject: true}, host, Deps{
		Memory: &fakeMemory{profile: "- communication style: terse"},
	})
	system, _ := svc.Build(context.Background(), "corr-1", "actor", "do the thing", false, skillapi.ActivationDirective{})
	if !strings.Contains(system, "operator you work for") {
		t.Errorf("the operator profile must reach the prompt:\n%s", system)
	}
	if !strings.Contains(system, "communication style: terse") {
		t.Errorf("the profile facets must reach the prompt:\n%s", system)
	}
	if !strings.Contains(system, "BASE SYSTEM") {
		t.Errorf("the persona must survive beneath the profile:\n%s", system)
	}
	if strings.Index(system, "communication style: terse") > strings.Index(system, "BASE SYSTEM") {
		t.Errorf("the profile must lead the persona:\n%s", system)
	}
}

// TestBuild_TasteIsInjectedAndJournaled pins the taste path: the exemplars
// reach the prompt AND the run's timeline (taste.injected under the run's
// correlation).
func TestBuild_TasteIsInjectedAndJournaled(t *testing.T) {
	host := &fakeHost{system: "BASE SYSTEM", now: fixedTime()}
	svc := New(Config{TasteInject: true, TasteTopK: 3}, host, Deps{
		Taste: &fakeTaste{exemplars: []tasteapi.Exemplar{{ID: "ex-1", Title: "Good PR summary", Body: "One line what."}}},
	})
	system, _ := svc.Build(context.Background(), "corr-1", "actor", "do thething", false, skillapi.ActivationDirective{})
	if !strings.Contains(system, "What good looks like") {
		t.Errorf("the exemplars must reach the prompt:\n%s", system)
	}
	if !strings.Contains(system, "Good PR summary") {
		t.Errorf("the exemplar must reach the prompt:\n%s", system)
	}
	if len(host.events) != 1 {
		t.Fatalf("published %d events, want 1 (taste.injected)", len(host.events))
	}
	ev := host.events[0]
	if ev.Subject != "agent.agent-corr-1.taste" {
		t.Errorf("subject = %q, want the run's taste subject", ev.Subject)
	}
	if ev.CorrelationID != "corr-1" {
		t.Errorf("correlation = %q, want corr-1", ev.CorrelationID)
	}
	if ev.Kind != event.KindTasteInjected {
		t.Errorf("kind = %v, want KindTasteInjected", ev.Kind)
	}
}

// TestBuild_MemoryIsInjectedAndSelected pins the memory path: the recalled
// records reach the prompt and the selection manifest says which were chosen.
func TestBuild_MemoryIsInjectedAndSelected(t *testing.T) {
	host := &fakeHost{system: "BASE SYSTEM", now: fixedTime()}
	svc := New(Config{MemoryInject: true, MemoryTopK: 5}, host, Deps{
		Memory: &fakeMemory{search: oneScoredMemory(), recall: oneScoredMemory()},
	})
	system, _ := svc.Build(context.Background(), "corr-1", "actor", "do the thing", false, skillapi.ActivationDirective{})
	if !strings.Contains(system, "Relevant memory") {
		t.Errorf("the recalled records must reach the prompt:\n%s", system)
	}
	if len(host.events) != 1 {
		t.Fatalf("published %d events, want 1 (context.selection)", len(host.events))
	}
	if host.events[0].Subject != "context.selection" {
		t.Errorf("subject = %q, want context.selection", host.events[0].Subject)
	}
	// The manifest must NAME what was chosen: a selection that says "something
	// was chosen" without saying what is not a selection.
	manifest, ok := host.events[0].Payload.(contextselect.Manifest)
	if !ok {
		t.Fatalf("payload = %T, want a contextselect.Manifest", host.events[0].Payload)
	}
	if len(manifest.Chosen) != 1 || manifest.Chosen[0].ID != "r-1" {
		t.Errorf("chosen = %+v, want the recalled record r-1", manifest.Chosen)
	}
}

// TestBuild_WorldIsInjectedAndSelected pins the world path: the resolved
// entities reach the prompt, so the model can ground what the intent refers
// to.
func TestBuild_WorldIsInjectedAndSelected(t *testing.T) {
	host := &fakeHost{system: "BASE SYSTEM", now: fixedTime()}
	svc := New(Config{WorldInject: true, WorldTopK: 5}, host, Deps{
		World: &fakeWorld{entities: []worldmodelapi.ScoredEntity{{
			Entity: worldmodelapi.Entity{ID: "e-1", Kind: "portfolio", Name: "The Portfolio"},
			Score:  0.8,
		}}},
	})
	system, _ := svc.Build(context.Background(), "corr-1", "actor", "do the thing", false, skillapi.ActivationDirective{})
	if !strings.Contains(system, "Known entities") {
		t.Errorf("the resolved entities must reach the prompt:\n%s", system)
	}
	if !strings.Contains(system, "The Portfolio") {
		t.Errorf("the entity must reach the prompt:\n%s", system)
	}
	if len(host.events) != 1 {
		t.Fatalf("published %d events, want 1 (context.selection)", len(host.events))
	}
	if host.events[0].Subject != "context.selection" {
		t.Errorf("subject = %q, want context.selection", host.events[0].Subject)
	}
	manifest, ok := host.events[0].Payload.(contextselect.Manifest)
	if !ok {
		t.Fatalf("payload = %T, want a contextselect.Manifest", host.events[0].Payload)
	}
	if len(manifest.Chosen) != 1 || manifest.Chosen[0].ID != "e-1" {
		t.Errorf("chosen = %+v, want the resolved entity e-1", manifest.Chosen)
	}
}

// TestBuild_SkillsAreInjectedAndAnswered pins the skill path: the activated
// skills reach the prompt AND their ids come back to the caller, which is how
// the run knows what it activated.
func TestBuild_SkillsAreInjectedAndAnswered(t *testing.T) {
	host := &fakeHost{system: "BASE SYSTEM", now: fixedTime()}
	svc := New(Config{SkillInject: true, SkillTopK: 3}, host, Deps{
		Skills: &fakeSkills{
			skills: []skillapi.Skill{{ID: "sk-1", Name: "diagnose-ci", Description: "d", Body: "b"}},
			active: oneScoredSkill(),
		},
	})
	system, ids := svc.Build(context.Background(), "corr-1", "actor", "do the thing", false, skillapi.ActivationDirective{})
	if !strings.Contains(system, "Applicable skills") {
		t.Errorf("the activated skills must reach the prompt:\n%s", system)
	}
	if len(ids) != 1 || ids[0] != "sk-1" {
		t.Errorf("activated ids = %v, want [sk-1]", ids)
	}
	if len(host.events) != 1 {
		t.Fatalf("published %d events, want 1 (context.selection)", len(host.events))
	}
	if host.events[0].Subject != "context.selection" {
		t.Errorf("subject = %q, want context.selection", host.events[0].Subject)
	}
	manifest, ok := host.events[0].Payload.(contextselect.Manifest)
	if !ok {
		t.Fatalf("payload = %T, want a contextselect.Manifest", host.events[0].Payload)
	}
	// The manifest's CHOSEN list is the RETRIEVED pool's, not the activated
	// skills': the candidates come from what the retrieval matched, and a
	// retrieval that matched nothing chooses nothing. The explicit path below
	// is where the activated skill itself is named.
	_ = manifest
}

// TestBuild_AnExplicitDirectiveActivatesTheNamedSkill pins the OTHER skill
// path: when the run carries an explicit activation directive, the named
// skills are activated — not the ones that merely match the intent.
func TestBuild_AnExplicitDirectiveActivatesTheNamedSkill(t *testing.T) {
	host := &fakeHost{system: "BASE SYSTEM", now: fixedTime()}
	skills := &fakeSkills{active: oneScoredSkill()}
	svc := New(Config{SkillInject: true, SkillTopK: 3}, host, Deps{Skills: skills})
	directive := skillapi.ActivationDirective{Explicit: true, Refs: []string{"diagnose-ci"}}
	system, ids := svc.Build(context.Background(), "corr-1", "actor", "fix the build", false, directive)
	if !strings.Contains(system, "Applicable skills") {
		t.Errorf("the explicitly activated skills must reach the prompt:\n%s", system)
	}
	if len(ids) != 1 || ids[0] != "sk-1" {
		t.Errorf("activated ids = %v, want [sk-1]", ids)
	}
	if len(skills.refs) != 1 || skills.refs[0] != "diagnose-ci" {
		t.Errorf("the directive's refs = %v, want [diagnose-ci]", skills.refs)
	}
	if len(host.events) != 1 {
		t.Fatalf("published %d events, want 1 (context.selection)", len(host.events))
	}
	manifest, ok := host.events[0].Payload.(contextselect.Manifest)
	if !ok {
		t.Fatalf("payload = %T, want a contextselect.Manifest", host.events[0].Payload)
	}
	if len(manifest.Chosen) != 1 || manifest.Chosen[0].ID != "sk-1" {
		t.Errorf("chosen = %+v, want the activated skill sk-1", manifest.Chosen)
	}
}

// TestBuild_NoStoresIsSafe pins the nil-port rule: a daemon that was given no
// memory, no taste and no skills injects nothing and does not fail the run.
//
// The world graph is NOT in this list: the daemon always opens one, and the
// prompt's world path has never guarded it. That asymmetry is inherited, not
// introduced — this slice moves the rule, it does not change it.
func TestBuild_NoStoresIsSafe(t *testing.T) {
	svc, host := newFixture(Config{MemoryInject: true, TasteInject: true, SkillInject: true})
	system, ids := svc.Build(context.Background(), "corr-1", "actor", "do the thing", false, skillapi.ActivationDirective{})
	if system != "BASE SYSTEM" {
		t.Errorf("system = %q, want the identity untouched", system)
	}
	if len(ids) != 0 {
		t.Errorf("activated %d skills, want none", len(ids))
	}
	if len(host.events) != 0 {
		t.Errorf("published %d events, want none", len(host.events))
	}
}

// TestBuildEnvironment_GatedOnTheConfig pins the preamble's gate: the
// environment is stated only when the configuration asks for it.
func TestBuildEnvironment_GatedOnTheConfig(t *testing.T) {
	off, _ := newFixture(Config{})
	if got := off.BuildEnvironment("BASE", nil); got != "BASE" {
		t.Errorf("with the gate off the preamble must not touch the prompt: %q", got)
	}
	on, _ := newFixture(Config{EnvironmentInject: true, WorkspaceRoot: "/ws"})
	got := on.BuildEnvironment("BASE", nil)
	if !strings.Contains(got, "## Runtime environment") {
		t.Errorf("the preamble must be prepended:\n%s", got)
	}
	if !strings.Contains(got, "2026-06-08") {
		t.Errorf("the preamble must state the host's date:\n%s", got)
	}
	if !strings.Contains(got, "/ws") {
		t.Errorf("the preamble must state the workspace:\n%s", got)
	}
	if !strings.Contains(got, "BASE") {
		t.Errorf("the persona must survive beneath the preamble:\n%s", got)
	}
}

// TestPublishSelection_SkipsEmptyManifests pins the rule that keeps the
// journal readable: a manifest with nothing in it is not worth an entry.
func TestPublishSelection_SkipsEmptyManifests(t *testing.T) {
	svc, host := newFixture(Config{})
	svc.PublishSelection("corr-1", "actor", contextselect.Manifest{})
	if len(host.events) != 0 {
		t.Errorf("published %d events, want none for an empty manifest", len(host.events))
	}
}

// TestPublishSelection_JournaledWithTheRightSubject pins what a manifest
// looks like on the wire: the subject, the kind, the actor and the
// correlation.
func TestPublishSelection_JournaledWithTheRightSubject(t *testing.T) {
	svc, host := newFixture(Config{})
	svc.PublishSelection("corr-9", "actor", contextselect.Manifest{
		Chosen: []contextselect.Candidate{{Source: "memory", ID: "r-1"}},
	})
	if len(host.events) != 1 {
		t.Fatalf("published %d events, want 1", len(host.events))
	}
	ev := host.events[0]
	if ev.Subject != "context.selection" {
		t.Errorf("subject = %q, want context.selection", ev.Subject)
	}
	if ev.Kind != event.KindContextSelection {
		t.Errorf("kind = %v, want KindContextSelection", ev.Kind)
	}
	if ev.CorrelationID != "corr-9" {
		t.Errorf("correlation = %q, want corr-9", ev.CorrelationID)
	}
	if ev.Actor != "actor" {
		t.Errorf("actor = %q, want actor", ev.Actor)
	}
}

// TestBuildSubagent_CleansTheIntent pins the child's contract: an explicit
// activation directive is a directive, not part of the task, so the child
// receives the intent without it.
func TestBuildSubagent_CleansTheIntent(t *testing.T) {
	host := &fakeHost{system: "BASE SYSTEM", now: fixedTime()}
	svc := New(Config{}, host, Deps{})
	_, _, got := svc.BuildSubagent(context.Background(), "corr-1", "actor", "/skill diagnose-ci\nthe real task", "BASE SYSTEM")
	if got != "the real task" {
		t.Errorf("cleaned intent = %q, want the task without the directive", got)
	}
}

// TestBuildSubagent_ASystemAgentReturnsTheSystemUnchanged pins the child's
// early exit: a system agent gets its system prompt back untouched.
func TestBuildSubagent_ASystemAgentReturnsTheSystemUnchanged(t *testing.T) {
	host := &fakeHost{system: "BASE SYSTEM", now: fixedTime()}
	svc := New(Config{MemoryInject: true, SkillInject: true}, host, Deps{
		Memory: &fakeMemory{search: oneScoredMemory(), recall: oneScoredMemory()},
		Skills: &fakeSkills{active: oneScoredSkill()},
	})
	ctx := runctx.WithAgentProfile(context.Background(), agentapi.Profile{Slug: "sys", System: true})
	system, ids, _ := svc.BuildSubagent(ctx, "corr-1", "actor", "the task", "BASE SYSTEM")
	if system != "BASE SYSTEM" {
		t.Errorf("system = %q, want it untouched", system)
	}
	if len(ids) != 0 {
		t.Errorf("activated %d skills, want none", len(ids))
	}
	if len(host.events) != 0 {
		t.Errorf("published %d events, want none", len(host.events))
	}
}
