// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

// fakeConductorProvider replies by role (detected from the request System text),
// records every request for routing assertions, and serves a scripted sequence
// of verifier verdicts for the critique path.
type fakeConductorProvider struct {
	reqs      []agent.CompletionRequest
	workerOut string
	planOut   string
	verdicts  []string // sequential critique verdicts; default "PASS" once exhausted
	vIdx      int
}

func (p *fakeConductorProvider) Name() string { return "conductor-fake" }
func (p *fakeConductorProvider) Complete(_ context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	p.reqs = append(p.reqs, req)
	sys := strings.ToLower(req.System)
	var text string
	switch {
	case strings.Contains(sys, "thinker"):
		text = "PLAN: decompose the task"
	case strings.Contains(sys, "worker"):
		text = p.workerOut
	case strings.Contains(sys, "verifier"):
		if p.vIdx < len(p.verdicts) {
			text = p.verdicts[p.vIdx]
			p.vIdx++
		} else {
			text = "PASS: looks correct"
		}
	case strings.Contains(sys, "planner"):
		text = p.planOut
	default:
		text = "?"
	}
	return &agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, Content: text},
		StopReason: agent.StopEndTurn,
	}, nil
}

// fakeExec serves a scripted sequence of ok/!ok run outcomes.
type fakeExec struct {
	ok       []bool // per call: true=clean run
	idx      int
	calls    int
	lastLang string
	lastCode string
}

func (f *fakeExec) RunScript(_ context.Context, language, code, _ string) (string, bool, error) {
	f.calls++
	f.lastLang = language
	f.lastCode = code
	clean := false
	if f.idx < len(f.ok) {
		clean = f.ok[f.idx]
		f.idx++
	}
	return "ran:" + language, !clean, nil // isError = !clean
}

func openConductorKernel(t *testing.T, members []CouncilMember, prov agent.Provider, exec CodeExecutor) *Kernel {
	t.Helper()
	k, err := Open(Config{
		BaseDir:        t.TempDir(),
		Provider:       prov,
		Tools:          map[string]agent.Tool{},
		CouncilMembers: func() []CouncilMember { return members },
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if exec != nil {
		k.SetConductorExec(exec)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

func threeMembers() []CouncilMember {
	return []CouncilMember{{Seat: "A", Model: "model-a"}, {Seat: "B", Model: "model-b"}, {Seat: "C", Model: "model-c"}}
}

func TestConduct_ExecRetryLoopAndDefaults(t *testing.T) {
	prov := &fakeConductorProvider{workerOut: "```python\nassert add(2,2)==4\n```"}
	exec := &fakeExec{ok: []bool{false, true}} // fail first, pass on retry
	k := openConductorKernel(t, threeMembers(), prov, exec)

	res, err := k.Conduct(context.Background(), "c-exec", ConductorConfig{Task: "write add()", MaxRounds: 2})
	if err != nil {
		t.Fatalf("Conduct: %v", err)
	}
	if !res.Passed || res.Rounds != 2 {
		t.Errorf("passed=%v rounds=%d, want true/2", res.Passed, res.Rounds)
	}
	if exec.calls != 2 {
		t.Errorf("exec calls = %d, want 2", exec.calls)
	}
	if exec.lastLang != "python" {
		t.Errorf("exec language = %q, want python", exec.lastLang)
	}
	// Default-member diversity: distinct model per role.
	if res.Roles["thinker"] != "model-a" || res.Roles["worker"] != "model-b" || res.Roles["verifier"] != "model-c" {
		t.Errorf("roles = %v", res.Roles)
	}
	// Both verifier steps ran code.
	var verSteps int
	for _, s := range res.Steps {
		if s.Role == conductorRoleVerifier {
			verSteps++
			if s.Exec == nil || !s.Exec.Ran {
				t.Errorf("verifier step lacks exec: %+v", s)
			}
		}
	}
	if verSteps != 2 {
		t.Errorf("verifier steps = %d, want 2", verSteps)
	}

	// Lifecycle events journaled.
	var started, steps, done int
	_ = k.Journal().Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindConductorStarted:
			started++
		case event.KindConductorStep:
			steps++
		case event.KindConductorDone:
			done++
		}
		return nil
	})
	if started != 1 || done != 1 || steps == 0 {
		t.Errorf("events started=%d steps=%d done=%d", started, steps, done)
	}
}

func TestConduct_CritiqueRetryThenPass(t *testing.T) {
	prov := &fakeConductorProvider{workerOut: "the answer is 4", verdicts: []string{"FAIL: not shown", "PASS: good"}}
	k := openConductorKernel(t, threeMembers(), prov, nil) // no executor → critique path

	res, err := k.Conduct(context.Background(), "c-crit", ConductorConfig{Task: "what is 2+2?", MaxRounds: 3})
	if err != nil {
		t.Fatalf("Conduct: %v", err)
	}
	if !res.Passed || res.Rounds != 2 {
		t.Errorf("passed=%v rounds=%d, want true/2", res.Passed, res.Rounds)
	}
	for _, s := range res.Steps {
		if s.Role == conductorRoleVerifier && s.Exec != nil {
			t.Errorf("critique path should not record exec: %+v", s)
		}
	}
}

func TestConduct_ChainRoutingAndExplicitRoles(t *testing.T) {
	prov := &fakeConductorProvider{workerOut: "prose", verdicts: []string{"PASS: ok"}}
	k := openConductorKernel(t, threeMembers(), prov, nil)

	_, err := k.Conduct(context.Background(), "c-chain", ConductorConfig{
		Task: "x", Thinker: "model-t", Worker: "@fast", Verifier: "model-v", MaxRounds: 1,
	})
	if err != nil {
		t.Fatalf("Conduct: %v", err)
	}
	var sawWorkerChain, sawThinkerModel bool
	for _, r := range prov.reqs {
		sys := strings.ToLower(r.System)
		// Thinker first: its system prompt mentions "Worker" (the role it hands
		// off to), so a worker-first check would misclassify it.
		switch {
		case strings.Contains(sys, "thinker"):
			if r.Model == "model-t" && r.ModelChain == nil {
				sawThinkerModel = true
			}
		case strings.Contains(sys, "worker"):
			if len(r.ModelChain) == 1 && r.ModelChain[0] == "@fast" && r.Model == "" {
				sawWorkerChain = true
			}
		}
		if r.TaskType != "conductor" {
			t.Errorf("TaskType = %q, want conductor", r.TaskType)
		}
	}
	if !sawWorkerChain {
		t.Error("worker @fast should route via ModelChain")
	}
	if !sawThinkerModel {
		t.Error("thinker bare id should route via Model")
	}
}

func TestConduct_PlanTailorsRoleBriefs(t *testing.T) {
	prov := &fakeConductorProvider{
		workerOut: "prose",
		verdicts:  []string{"PASS: ok"},
		planOut:   "THINKER: think hard\nWORKER: write w-brief carefully\nVERIFIER: check v-brief",
	}
	k := openConductorKernel(t, threeMembers(), prov, nil)

	res, err := k.Conduct(context.Background(), "c-plan", ConductorConfig{Task: "x", MaxRounds: 1, Plan: true})
	if err != nil {
		t.Fatalf("Conduct: %v", err)
	}
	if !strings.Contains(res.Plan, "w-brief carefully") {
		t.Errorf("plan not stored: %q", res.Plan)
	}
	var workerSysHadBrief bool
	for _, r := range prov.reqs {
		if strings.Contains(strings.ToLower(r.System), "worker") && strings.Contains(r.System, "w-brief carefully") {
			workerSysHadBrief = true
		}
	}
	if !workerSysHadBrief {
		t.Error("worker system prompt should carry the tailored brief")
	}
}

func TestConduct_NoModelsAndEmptyTask(t *testing.T) {
	// No members, no explicit roles → error.
	k := openConductorKernel(t, nil, &fakeConductorProvider{}, nil)
	if _, err := k.Conduct(context.Background(), "c1", ConductorConfig{Task: "x"}); err == nil {
		t.Error("expected error with no available models")
	}
	// Empty task → error.
	if _, err := k.Conduct(context.Background(), "c2", ConductorConfig{Task: "  "}); err == nil {
		t.Error("expected error with empty task")
	}
	// One member fills all three roles.
	k2 := openConductorKernel(t, []CouncilMember{{Seat: "Solo", Model: "m"}}, &fakeConductorProvider{workerOut: "p", verdicts: []string{"PASS: y"}}, nil)
	res, err := k2.Conduct(context.Background(), "c3", ConductorConfig{Task: "x", MaxRounds: 1})
	if err != nil {
		t.Fatalf("Conduct: %v", err)
	}
	if res.Roles["thinker"] != "m" || res.Roles["worker"] != "m" || res.Roles["verifier"] != "m" {
		t.Errorf("single-member fill: %v", res.Roles)
	}
}

func TestParseVerdict(t *testing.T) {
	cases := []struct {
		in       string
		verdict  string
		reasonHy string // substring expected in reason
	}{
		{"PASS: looks good", "pass", "looks good"},
		{"FAIL: missing edge case", "fail", "missing edge case"},
		{"pass", "pass", ""},
		{"## PASS\nclean", "pass", "clean"},
		{"The answer is wrong", "fail", "wrong"},
	}
	for _, c := range cases {
		v, r := parseVerdict(c.in)
		if v != c.verdict {
			t.Errorf("parseVerdict(%q) verdict=%q want %q", c.in, v, c.verdict)
		}
		if c.reasonHy != "" && !strings.Contains(strings.ToLower(r), c.reasonHy) {
			t.Errorf("parseVerdict(%q) reason=%q want substring %q", c.in, r, c.reasonHy)
		}
	}
}

func TestParseRoleBriefs(t *testing.T) {
	briefs := parseRoleBriefs("THINKER: plan it\nmore plan\nWORKER: build it\nVERIFIER: check it")
	if briefs[conductorRoleThinker] != "plan it\nmore plan" {
		t.Errorf("thinker brief = %q", briefs[conductorRoleThinker])
	}
	if briefs[conductorRoleWorker] != "build it" || briefs[conductorRoleVerifier] != "check it" {
		t.Errorf("briefs = %v", briefs)
	}
	if len(parseRoleBriefs("no labels here")) != 0 {
		t.Error("unlabelled plan should yield no briefs")
	}
}

func TestExtractRunnableCode(t *testing.T) {
	lang, code, ok := extractRunnableCode("here:\n```python\nprint(1)\n```\ndone")
	if !ok || lang != "python" || code != "print(1)" {
		t.Errorf("got (%q,%q,%v)", lang, code, ok)
	}
	if _, _, ok := extractRunnableCode("```text\nhello\n```"); ok {
		t.Error("non-runnable language should not match")
	}
	if _, _, ok := extractRunnableCode("no fence at all"); ok {
		t.Error("no code block should not match")
	}
	// js/ts aliases normalise.
	if l, _, ok := extractRunnableCode("```js\nconsole.log(1)\n```"); !ok || l != "javascript" {
		t.Errorf("js alias → %q ok=%v", l, ok)
	}
	if l, _, ok := extractRunnableCode("```ts\nlet x=1\n```"); !ok || l != "typescript" {
		t.Errorf("ts alias → %q ok=%v", l, ok)
	}
}
