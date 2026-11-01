// SPDX-License-Identifier: MIT

package delegation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/prompt"
	"github.com/agezt/agezt/kernel/modules/runs/registry"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
	"github.com/agezt/agezt/kernel/modules/runs/steer"
)

// ---------------------------------------------------------------------------
// The fake host: every door the engine opens, answered by hand.
// ---------------------------------------------------------------------------

type fakeHost struct {
	profiles map[string]agentapi.Profile
	model    string
	system   string
	halted   bool
	haltErr  error
	corrSeq  int

	events   []event.Spec
	journal  []*event.Event
	steers   map[string]*steer.Control
	outcomes []skillOutcome

	lifecycles  []string
	runFailures int
	loops       int
	lastCfg     agent.LoopConfig
}

type skillOutcome struct {
	corr     string
	skillIDs []string
	ok       bool
}

func newFakeHost() *fakeHost {
	return &fakeHost{
		profiles: map[string]agentapi.Profile{},
		model:    "daemon-model",
		system:   "daemon-identity",
		steers:   map[string]*steer.Control{},
	}
}

func (h *fakeHost) RosterGet(ref string) (agentapi.Profile, bool) {
	p, ok := h.profiles[ref]
	return p, ok
}

func (h *fakeHost) ProfileModel(prof agentapi.Profile) string {
	if m := strings.TrimSpace(prof.Model); m != "" {
		return m
	}
	return ""
}

func (h *fakeHost) Model() string  { return h.model }
func (h *fakeHost) System() string { return h.system }
func (h *fakeHost) Halted() bool   { return h.halted }
func (h *fakeHost) HaltedError() error {
	if h.haltErr != nil {
		return h.haltErr
	}
	return errHalted
}

var errHalted = errors.New("runtime: kernel is halted")

func (h *fakeHost) NewCorrelation() string {
	h.corrSeq++
	return fmt.Sprintf("corr-%d", h.corrSeq)
}

func (h *fakeHost) Publish(spec event.Spec) (*event.Event, error) {
	h.events = append(h.events, spec)
	raw, err := json.Marshal(spec.Payload)
	if err != nil {
		return nil, err
	}
	return &event.Event{Kind: spec.Kind, CorrelationID: spec.CorrelationID, Payload: raw}, nil
}

func (h *fakeHost) RangeJournal(fn func(*event.Event) error) error {
	for _, e := range h.journal {
		if err := fn(e); err != nil {
			return err
		}
	}
	return nil
}

func (h *fakeHost) RegisterSteer(corr string, rc *steer.Control) { h.steers[corr] = rc }
func (h *fakeHost) UnregisterSteer(corr string)                  { delete(h.steers, corr) }

func (h *fakeHost) RecordSkillOutcome(corr string, skillIDs []string, ok bool) {
	h.outcomes = append(h.outcomes, skillOutcome{corr: corr, skillIDs: skillIDs, ok: ok})
}

func (h *fakeHost) BaseLoopConfig(ctx context.Context, corr, model string) agent.LoopConfig {
	return agent.LoopConfig{CorrelationID: corr, Model: model}
}

func (h *fakeHost) CompleteChildLifecycle(ctx context.Context, corr string) {
	h.lifecycles = append(h.lifecycles, corr)
}

// promptHost answers the prompt service's three doors.
type promptHost struct{ h *fakeHost }

func (p promptHost) System() string { return p.h.system }
func (p promptHost) Publish(spec event.Spec) (*event.Event, error) {
	return p.h.Publish(spec)
}
func (p promptHost) Now() time.Time { return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC) }

func newService(t *testing.T, h *fakeHost, cfg Config) *Service {
	t.Helper()
	svc := New(Deps{
		Host:   h,
		Config: func() Config { return cfg },
		RunChild: func(ctx context.Context, lc agent.LoopConfig, task string) (string, error) {
			h.loops++
			h.lastCfg = lc
			if h.runFailures > 0 {
				h.runFailures--
				return "", errors.New("child failed")
			}
			return "child answer", nil
		},
		Prompt:   prompt.New(prompt.Config{}, promptHost{h}, prompt.Deps{}),
		Registry: registry.New(),
	})
	return svc
}

func spawnPayload(t *testing.T, spec event.Spec) map[string]any {
	t.Helper()
	raw, err := json.Marshal(spec.Payload)
	if err != nil {
		t.Fatalf("spawn payload: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("spawn payload: %v", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// The guards
// ---------------------------------------------------------------------------

func TestPrepare_RefusesAnEmptyTask(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	if _, err := svc.prepare(context.Background(), Request{Task: "   "}, false); err == nil {
		t.Fatal("an empty task must be refused")
	}
}

func TestPrepare_RefusesAnUnknownAgent(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	_, err := svc.prepare(context.Background(), Request{Task: "t", AgentRef: "ghost"}, false)
	if err == nil || !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("err = %v, want an unknown-agent refusal", err)
	}
}

func TestPrepare_RefusesARetiredAgent(t *testing.T) {
	h := newFakeHost()
	h.profiles["old"] = agentapi.Profile{Slug: "old", Retired: true}
	svc := newService(t, h, Config{})
	_, err := svc.prepare(context.Background(), Request{Task: "t", AgentRef: "old"}, false)
	if err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatalf("err = %v, want a retired-agent refusal", err)
	}
}

func TestPrepare_RefusesAPausedAgent(t *testing.T) {
	h := newFakeHost()
	h.profiles["paused"] = agentapi.Profile{Slug: "paused", Enabled: false}
	svc := newService(t, h, Config{})
	_, err := svc.prepare(context.Background(), Request{Task: "t", AgentRef: "paused"}, false)
	if err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("err = %v, want a paused-agent refusal", err)
	}
}

func TestPrepare_RefusesWhenTheDaemonIsHalted(t *testing.T) {
	h := newFakeHost()
	h.halted = true
	svc := newService(t, h, Config{})
	_, err := svc.prepare(context.Background(), Request{Task: "t"}, false)
	if !errors.Is(err, errHalted) {
		t.Fatalf("err = %v, want the daemon's halt error", err)
	}
}

func TestPrepare_RefusesBeyondTheDepthBound(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{MaxDepth: 1})
	ctx := WithDepth(context.Background(), 1)
	_, err := svc.prepare(ctx, Request{Task: "t"}, false)
	if err == nil || !strings.Contains(err.Error(), "max sub-agent depth") {
		t.Fatalf("err = %v, want a depth refusal", err)
	}
}

func TestPrepare_RefusesBeyondTheFanoutBound(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{MaxFanout: 1})
	ctx := runctx.WithActor(context.Background(), "lead", "parent")
	if _, err := svc.prepare(ctx, Request{Task: "t"}, false); err != nil {
		t.Fatalf("first spawn: %v", err)
	}
	_, err := svc.prepare(ctx, Request{Task: "t"}, false)
	if err == nil || !strings.Contains(err.Error(), "fan-out") {
		t.Fatalf("err = %v, want a fan-out refusal", err)
	}
}

func TestPrepare_RefusesBeyondTheTreeTotalBound(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{MaxTotal: 1})
	ctx := runctx.WithRoot(runctx.WithActor(context.Background(), "lead", "parent"), "root")
	if _, err := svc.prepare(ctx, Request{Task: "t"}, false); err != nil {
		t.Fatalf("first spawn: %v", err)
	}
	// A DIFFERENT parent, but the SAME tree root: the total cap is a
	// tree-wide bound, not a per-spawner one.
	ctx2 := runctx.WithRoot(runctx.WithActor(context.Background(), "lead2", "other"), "root")
	_, err := svc.prepare(ctx2, Request{Task: "t"}, false)
	if err == nil || !strings.Contains(err.Error(), "total") {
		t.Fatalf("err = %v, want a tree-total refusal", err)
	}
}

func TestPrepare_RefusesBeyondTheSpendBound(t *testing.T) {
	h := newFakeHost()
	h.journal = []*event.Event{
		{Kind: event.KindSubAgentSpawned, CorrelationID: "parent", Payload: []byte(`{"child_correlation":"c1","parent":"parent"}`)},
		{Kind: event.KindBudgetConsumed, CorrelationID: "c1", Payload: []byte(`{"cost_microcents":9000}`)},
	}
	svc := newService(t, h, Config{MaxSpendMicrocents: 5000})
	ctx := runctx.WithActor(context.Background(), "lead", "parent")
	_, err := svc.prepare(ctx, Request{Task: "t"}, false)
	if err == nil || !strings.Contains(err.Error(), "spend") {
		t.Fatalf("err = %v, want a spend refusal", err)
	}
}

// ---------------------------------------------------------------------------
// The resolution
// ---------------------------------------------------------------------------

func TestPrepare_UsesTheLiveModelAndTaskTypeDefault(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	plan, err := svc.prepare(context.Background(), Request{Task: "t"}, false)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if plan.SubModel != "daemon-model" {
		t.Errorf("model = %q, want the daemon's live default", plan.SubModel)
	}
	if plan.TaskType != "delegate" {
		t.Errorf("task type = %q, want the delegate default", plan.TaskType)
	}
	if !strings.Contains(plan.System, "daemon-identity") {
		t.Errorf("system = %q, want the daemon's live identity appended", plan.System)
	}
}

func TestPrepare_ANamedAgentsSoulReplacesTheLiveIdentity(t *testing.T) {
	h := newFakeHost()
	h.profiles["named"] = agentapi.Profile{Slug: "named", Enabled: true, Soul: "the named soul"}
	svc := newService(t, h, Config{})
	plan, err := svc.prepare(context.Background(), Request{Task: "t", AgentRef: "named"}, false)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !strings.Contains(plan.System, "the named soul") {
		t.Errorf("system = %q, want the named agent's soul", plan.System)
	}
	if strings.Contains(plan.System, "daemon-identity") {
		t.Errorf("system = %q, want NO daemon identity under a named soul", plan.System)
	}
	if plan.AgentSlug != "named" {
		t.Errorf("agent slug = %q, want named", plan.AgentSlug)
	}
}

func TestPrepare_AnExplicitModelWinsOverTheProfile(t *testing.T) {
	h := newFakeHost()
	h.profiles["named"] = agentapi.Profile{Slug: "named", Enabled: true, Model: "profile-model"}
	svc := newService(t, h, Config{})
	plan, err := svc.prepare(context.Background(), Request{Task: "t", AgentRef: "named", Model: "explicit-model"}, false)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if plan.SubModel != "explicit-model" {
		t.Errorf("model = %q, want the explicit override", plan.SubModel)
	}
}

func TestPrepare_KeyedModelChainDropsUnroutableModels(t *testing.T) {
	h := newFakeHost()
	h.profiles["named"] = agentapi.Profile{Slug: "named", Enabled: true, Model: "unkeyed", Fallbacks: []string{"good"}}
	svc := newService(t, h, Config{ModelAvailable: func(m string) bool { return m == "good" }})
	plan, err := svc.prepare(context.Background(), Request{Task: "t", AgentRef: "named"}, false)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if plan.SubModel != "good" {
		t.Errorf("model = %q, want the keyed model", plan.SubModel)
	}
}

func TestPrepare_JournalstheSpawnUnderTheParentCorrelation(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	ctx := runctx.WithActor(context.Background(), "lead", "parent-corr")
	plan, err := svc.prepare(ctx, Request{Task: "do it"}, false)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if len(h.events) != 1 {
		t.Fatalf("events = %d, want exactly the spawn event", len(h.events))
	}
	ev := h.events[0]
	if ev.Kind != event.KindSubAgentSpawned {
		t.Errorf("kind = %v, want subagent.spawned", ev.Kind)
	}
	if ev.CorrelationID != "parent-corr" {
		t.Errorf("correlation = %q, want the parent's", ev.CorrelationID)
	}
	if !strings.HasPrefix(ev.Subject, "agent.subagent-") {
		t.Errorf("subject = %q, want the child's own agent subject", ev.Subject)
	}
	payload := spawnPayload(t, ev)
	if payload["task"] != "do it" {
		t.Errorf("payload task = %v", payload["task"])
	}
	if payload["child_correlation"] != plan.ChildCorr {
		t.Errorf("payload child = %v, want %q", payload["child_correlation"], plan.ChildCorr)
	}
	if payload["depth"] != float64(1) {
		t.Errorf("payload depth = %v, want 1", payload["depth"])
	}
}

func TestPrepare_RegistersTheChildsSteeringControl(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	plan, err := svc.prepare(context.Background(), Request{Task: "t"}, false)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if h.steers[plan.ChildCorr] == nil {
		t.Fatal("the child's steering control must be registered")
	}
}

// ---------------------------------------------------------------------------
// The execution
// ---------------------------------------------------------------------------

func TestRun_ExecutesTheChildAndReleasesItsTally(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	ans, err := svc.Run(context.Background(), Request{Task: "t"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans != "child answer" {
		t.Errorf("answer = %q, want the child's answer", ans)
	}
	if h.loops != 1 {
		t.Errorf("child loops = %d, want 1", h.loops)
	}
	if len(h.steers) != 0 {
		t.Errorf("steering controls left behind: %v", h.steers)
	}
}

func TestRun_SetsTheChildsLoopConfiguration(t *testing.T) {
	h := newFakeHost()
	h.profiles["named"] = agentapi.Profile{Slug: "named", Enabled: true, MaxCostMc: 4242}
	svc := newService(t, h, Config{})
	ctx := runctx.WithActor(context.Background(), "lead", "parent-corr")
	if _, err := svc.Run(ctx, Request{Task: "t", AgentRef: "named"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	lc := h.lastCfg
	if lc.TaskType != "delegate" {
		t.Errorf("task type = %q, want delegate", lc.TaskType)
	}
	if lc.WakeSource != "subagent" || lc.WakeReason != "delegation" {
		t.Errorf("wake = %q/%q, want subagent/delegation", lc.WakeSource, lc.WakeReason)
	}
	if lc.Steer == nil {
		t.Error("the child's steering control must reach the loop configuration")
	}
	if lc.Actor == "" || lc.CorrelationID == "" {
		t.Errorf("actor/correlation = %q/%q, want both set", lc.Actor, lc.CorrelationID)
	}
	if lc.ParentCorrelation != "parent-corr" {
		t.Errorf("parent correlation = %q, want parent-corr", lc.ParentCorrelation)
	}
	if lc.Agent != "named" {
		t.Errorf("agent = %q, want named", lc.Agent)
	}
	if lc.MaxRunCostMicrocents != 4242 {
		t.Errorf("per-run cost ceiling = %d, want the profile's 4242", lc.MaxRunCostMicrocents)
	}
}

// ---------------------------------------------------------------------------
// The async path
// ---------------------------------------------------------------------------

func TestSpawn_StartsTheChildAndAwaitCollectsIt(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	id, err := svc.Spawn(context.Background(), Request{Task: "t"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if id == "" {
		t.Fatal("spawn id must not be empty")
	}
	res, err := svc.Await(context.Background(), id)
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if res.IsError {
		t.Fatalf("await result = %+v, want the child's answer", res)
	}
	if res.Output != "child answer" {
		t.Errorf("await output = %q, want the child's answer", res.Output)
	}
}

func TestAwait_RefusesAForeignSpawn(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	ctx := runctx.WithActor(context.Background(), "lead", "parent-corr")
	id, err := svc.Spawn(ctx, Request{Task: "t"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	other := runctx.WithActor(context.Background(), "lead2", "someone-else")
	res, err := svc.Await(other, id)
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "another run") {
		t.Fatalf("await result = %+v, want a refusal", res)
	}
}

func TestAwait_RefusesAnUnknownSpawn(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	res, err := svc.Await(context.Background(), "nope")
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "unknown spawn id") {
		t.Fatalf("await result = %+v, want an unknown-spawn refusal", res)
	}
}

func TestCancelSpawns_CancelsTheChildrenOfARun(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	ctx := runctx.WithActor(context.Background(), "lead", "parent-corr")
	id, err := svc.Spawn(ctx, Request{Task: "t"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	orphans := svc.CancelSpawns("parent-corr")
	if len(orphans) != 1 {
		t.Fatalf("orphans = %d, want 1", len(orphans))
	}
	// The handle is dropped, so the id is no longer awaitable.
	res, err := svc.Await(context.Background(), id)
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "unknown spawn id") {
		t.Fatalf("await result = %+v, want the handle to be gone", res)
	}
}

func TestReleaseTallies_DropsTheCounters(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{MaxFanout: 1})
	ctx := runctx.WithActor(context.Background(), "lead", "parent-corr")
	if _, err := svc.prepare(ctx, Request{Task: "t"}, false); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	svc.ReleaseTallies("parent-corr")
	if _, err := svc.prepare(ctx, Request{Task: "t"}, false); err != nil {
		t.Fatalf("prepare after release: %v", err)
	}
}

// ---------------------------------------------------------------------------
// The spend walk
// ---------------------------------------------------------------------------

// TestSpendMicrocents_CycleGuardTerminates moved here from the daemon's
// subagent_cycle_internal_test.go in W3.76f: the spend walk is the engine's
// rule now. The daemon never emits a cyclic delegation graph, but a corrupt or
// forged journal could — and the cap's BFS walks parent→child links straight
// from journal payloads. Without the guard a cycle (A→B→A) would loop forever
// and wedge every later delegation's spend check.
func TestSpendMicrocents_CycleGuardTerminates(t *testing.T) {
	h := newFakeHost()
	h.journal = []*event.Event{
		{Kind: event.KindSubAgentSpawned, CorrelationID: "A", Payload: []byte(`{"child_correlation":"B","parent":"A"}`)},
		{Kind: event.KindSubAgentSpawned, CorrelationID: "B", Payload: []byte(`{"child_correlation":"A","parent":"B"}`)},
		{Kind: event.KindBudgetConsumed, CorrelationID: "A", Payload: []byte(`{"cost_microcents":1000}`)},
		{Kind: event.KindBudgetConsumed, CorrelationID: "B", Payload: []byte(`{"cost_microcents":2000}`)},
	}
	svc := newService(t, h, Config{})
	done := make(chan int64, 1)
	go func() { done <- svc.spendMicrocents("A") }()
	select {
	case total := <-done:
		// Descendants of A, excluding A itself: just B. So total == B's spend.
		if total != 2000 {
			t.Errorf("transitive spend = %d, want 2000 (B's spend; A excluded, cycle visited once)", total)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the spend walk did not terminate within 2s — the cycle guard regressed")
	}
}

func TestSpendMicrocents_CountsTheWholeTree(t *testing.T) {
	h := newFakeHost()
	h.journal = []*event.Event{
		{Kind: event.KindSubAgentSpawned, CorrelationID: "A", Payload: []byte(`{"child_correlation":"B","parent":"A"}`)},
		{Kind: event.KindSubAgentSpawned, CorrelationID: "B", Payload: []byte(`{"child_correlation":"C","parent":"B"}`)},
		{Kind: event.KindBudgetConsumed, CorrelationID: "A", Payload: []byte(`{"cost_microcents":1000}`)},
		{Kind: event.KindBudgetConsumed, CorrelationID: "B", Payload: []byte(`{"cost_microcents":2000}`)},
		{Kind: event.KindBudgetConsumed, CorrelationID: "C", Payload: []byte(`{"cost_microcents":4000}`)},
	}
	svc := newService(t, h, Config{})
	if got := svc.spendMicrocents("A"); got != 6000 {
		t.Errorf("transitive spend = %d, want 6000 (B + C, A excluded)", got)
	}
}

// ---------------------------------------------------------------------------
// The tools
// ---------------------------------------------------------------------------

func TestTool_InvokeWithoutARunnerRefuses(t *testing.T) {
	tool := NewTool()
	res, err := tool.Invoke(context.Background(), []byte(`{"task":"t"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not wired") {
		t.Fatalf("result = %+v, want an unwired refusal", res)
	}
}

func TestTool_InvokeRejectsBrokenInput(t *testing.T) {
	tool := NewTool()
	res, err := tool.Invoke(context.Background(), []byte(`{`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "invalid input") {
		t.Fatalf("result = %+v, want an invalid-input refusal", res)
	}
}

func TestTool_InvokeRunsTheDelegation(t *testing.T) {
	h := newFakeHost()
	h.profiles["named"] = agentapi.Profile{Slug: "named", Enabled: true}
	svc := newService(t, h, Config{})
	tool := NewTool()
	tool.Run = svc.Run
	res, err := tool.Invoke(context.Background(), []byte(`{"task":"do it","model":"m","task_type":"code","agent":"named"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError || res.Output != "child answer" {
		t.Fatalf("result = %+v, want the child's answer", res)
	}
}

func TestTool_InvokeSpawnsWhenAsked(t *testing.T) {
	h := newFakeHost()
	svc := newService(t, h, Config{})
	tool := NewTool()
	tool.Spawn = svc.Spawn
	res, err := tool.Invoke(context.Background(), []byte(`{"task":"do it","async":true}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError || !strings.Contains(res.Output, "spawned sub-agent") {
		t.Fatalf("result = %+v, want a spawn id", res)
	}
}

func TestAwaitTool_InvokeWithoutAnAwaiterRefuses(t *testing.T) {
	tool := NewAwaitTool()
	res, err := tool.Invoke(context.Background(), []byte(`{"spawn_id":"x"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not wired") {
		t.Fatalf("result = %+v, want an unwired refusal", res)
	}
}
