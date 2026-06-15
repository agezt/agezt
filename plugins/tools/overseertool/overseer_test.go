// SPDX-License-Identifier: MIT

package overseertool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/roster"
)

// fakeSource is an in-memory Source recording interventions so the tool's
// op → kernel-method mapping is asserted without a live daemon.
type fakeSource struct {
	halted     bool
	runs       []string
	agents     []roster.Profile
	help       []board.Message
	impact     []string
	cancelled  string
	cancelOK   bool
	haltCalls  int
	resumes    int
	enabled    map[string]bool
	retired    map[string]bool
	setErr     error
	edited     string
	editFields roster.Profile
	created    roster.Profile
}

func (f *fakeSource) IsHalted() bool              { return f.halted }
func (f *fakeSource) ActiveRunIDs() []string      { return f.runs }
func (f *fakeSource) Agents() []roster.Profile    { return f.agents }
func (f *fakeSource) AgentImpact(string) []string { return f.impact }
func (f *fakeSource) OpenHelp(limit int) []board.Message {
	if limit > 0 && len(f.help) > limit {
		return f.help[:limit]
	}
	return f.help
}
func (f *fakeSource) CancelRun(corr string) bool { f.cancelled = corr; return f.cancelOK }
func (f *fakeSource) Halt(string)                { f.haltCalls++; f.halted = true }
func (f *fakeSource) ResumeAll(string)           { f.resumes++; f.halted = false }
func (f *fakeSource) SetAgentEnabled(ref string, enabled bool) (roster.Profile, error) {
	if f.setErr != nil {
		return roster.Profile{}, f.setErr
	}
	if f.enabled == nil {
		f.enabled = map[string]bool{}
	}
	f.enabled[ref] = enabled
	return roster.Profile{Slug: ref, Enabled: enabled}, nil
}
func (f *fakeSource) SetAgentRetired(ref string, retired bool) (roster.Profile, error) {
	if f.setErr != nil {
		return roster.Profile{}, f.setErr
	}
	if f.retired == nil {
		f.retired = map[string]bool{}
	}
	f.retired[ref] = retired
	return roster.Profile{Slug: ref, Retired: retired, Enabled: !retired}, nil
}
func (f *fakeSource) EditAgent(ref string, in roster.Profile) (roster.Profile, error) {
	if f.setErr != nil {
		return roster.Profile{}, f.setErr
	}
	f.edited = ref
	in.Slug = ref
	in.System = false // never settable via edit
	f.editFields = in
	return in, nil
}
func (f *fakeSource) CreateAgent(in roster.Profile) (roster.Profile, error) {
	if f.setErr != nil {
		return roster.Profile{}, f.setErr
	}
	in.System = false
	f.created = in
	return in, nil
}

func newTool(s Source) *Tool {
	t := New()
	t.Bind(s)
	return t
}

func invoke(t *testing.T, tool *Tool, in map[string]any) (map[string]any, bool) {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(res.Output), &out)
	return out, res.IsError
}

func TestDefinitionValid(t *testing.T) {
	d := New().Definition()
	if d.Name != "overseer" || !json.Valid(d.InputSchema) {
		t.Fatalf("bad definition: %+v", d)
	}
}

func TestStatus(t *testing.T) {
	f := &fakeSource{
		halted: true,
		runs:   []string{"c1", "c2"},
		agents: []roster.Profile{{Slug: "a"}, {Slug: "b"}},
		help:   []board.Message{{ID: "h1", Help: true}},
	}
	out, isErr := invoke(t, newTool(f), map[string]any{"op": "status"})
	if isErr {
		t.Fatalf("status errored: %v", out)
	}
	if out["halted"] != true || out["active_runs"].(float64) != 2 || out["agents"].(float64) != 2 || out["open_help"].(float64) != 1 {
		t.Fatalf("status wrong: %+v", out)
	}
}

func TestRunsAndCancel(t *testing.T) {
	f := &fakeSource{runs: []string{"c1", "c2"}, cancelOK: true}
	tool := newTool(f)

	runs, _ := invoke(t, tool, map[string]any{"op": "runs"})
	if runs["count"].(float64) != 2 {
		t.Fatalf("runs count = %v, want 2", runs["count"])
	}
	out, isErr := invoke(t, tool, map[string]any{"op": "cancel", "run": "c1"})
	if isErr {
		t.Fatalf("cancel errored: %v", out)
	}
	if f.cancelled != "c1" || out["cancelled"] != true {
		t.Fatalf("cancel wrong: cancelled=%q out=%+v", f.cancelled, out)
	}
	// Cancel needs a run id.
	if _, isErr := invoke(t, tool, map[string]any{"op": "cancel"}); !isErr {
		t.Error("op=cancel without run should error")
	}
}

func TestHaltResume(t *testing.T) {
	f := &fakeSource{}
	tool := newTool(f)
	invoke(t, tool, map[string]any{"op": "halt", "reason": "fire drill"})
	if f.haltCalls != 1 || !f.halted {
		t.Fatalf("halt not applied: %+v", f)
	}
	invoke(t, tool, map[string]any{"op": "resume"})
	if f.resumes != 1 || f.halted {
		t.Fatalf("resume not applied: %+v", f)
	}
}

func TestAgentInterventions(t *testing.T) {
	f := &fakeSource{impact: []string{"nightly (01ABC)"}}
	tool := newTool(f)

	// pause / unpause.
	invoke(t, tool, map[string]any{"op": "pause", "agent": "worker"})
	if f.enabled["worker"] != false {
		t.Errorf("pause did not disable worker: %+v", f.enabled)
	}
	invoke(t, tool, map[string]any{"op": "unpause", "agent": "worker"})
	if f.enabled["worker"] != true {
		t.Errorf("unpause did not enable worker: %+v", f.enabled)
	}

	// retire surfaces impact; revive clears it.
	out, _ := invoke(t, tool, map[string]any{"op": "retire", "agent": "worker"})
	if f.retired["worker"] != true {
		t.Errorf("retire did not retire worker: %+v", f.retired)
	}
	if imp, _ := out["impact"].([]any); len(imp) != 1 {
		t.Errorf("retire did not surface impact: %+v", out)
	}
	invoke(t, tool, map[string]any{"op": "revive", "agent": "worker"})
	if f.retired["worker"] != false {
		t.Errorf("revive did not revive worker: %+v", f.retired)
	}

	// each agent op needs a target.
	for _, op := range []string{"pause", "unpause", "retire", "revive", "impact"} {
		if _, isErr := invoke(t, tool, map[string]any{"op": op}); !isErr {
			t.Errorf("op=%s without agent should error", op)
		}
	}
}

func TestInterventionSurfacesError(t *testing.T) {
	f := &fakeSource{setErr: errors.New("no such agent")}
	if _, isErr := invoke(t, newTool(f), map[string]any{"op": "pause", "agent": "ghost"}); !isErr {
		t.Error("a kernel error should surface as a tool error")
	}
}

func TestHelpTriage(t *testing.T) {
	f := &fakeSource{help: []board.Message{
		{ID: "h1", From: "worker", Text: "build red", Help: true},
	}}
	out, _ := invoke(t, newTool(f), map[string]any{"op": "help"})
	if out["count"].(float64) != 1 {
		t.Fatalf("help count = %v, want 1", out["count"])
	}
}

func TestBadAndUnbound(t *testing.T) {
	if _, isErr := invoke(t, newTool(&fakeSource{}), map[string]any{"op": "bogus"}); !isErr {
		t.Error("unknown op should error")
	}
	if _, isErr := invoke(t, newTool(&fakeSource{}), map[string]any{}); !isErr {
		t.Error("missing op should error")
	}
	res, err := New().Invoke(context.Background(), json.RawMessage(`{"op":"status"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("an unbound overseer should return an error result")
	}
}

func TestEditAgent(t *testing.T) {
	f := &fakeSource{}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op":      "edit",
		"agent":   "scout",
		"profile": map[string]any{"model": "deepseek-chat", "max_daily_mc": 5000000, "soul": "Be terse."},
	})
	if isErr {
		t.Fatalf("edit errored: %v", out)
	}
	if f.edited != "scout" {
		t.Errorf("edited target = %q, want scout", f.edited)
	}
	if f.editFields.Model != "deepseek-chat" || f.editFields.MaxDailyMc != 5000000 || f.editFields.Soul != "Be terse." {
		t.Errorf("edit fields not applied: %+v", f.editFields)
	}
	if out["action"] != "edited" {
		t.Errorf("action = %v, want edited", out["action"])
	}
}

func TestEditNeedsAgentAndProfile(t *testing.T) {
	f := &fakeSource{}
	if _, isErr := invoke(t, newTool(f), map[string]any{"op": "edit", "profile": map[string]any{"model": "x"}}); !isErr {
		t.Error("op=edit without agent should error")
	}
	if _, isErr := invoke(t, newTool(f), map[string]any{"op": "edit", "agent": "scout"}); !isErr {
		t.Error("op=edit without profile should error")
	}
}

func TestCreateAgent(t *testing.T) {
	f := &fakeSource{}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op":      "create",
		"profile": map[string]any{"slug": "new-helper", "soul": "Help.", "system": true},
	})
	if isErr {
		t.Fatalf("create errored: %v", out)
	}
	if f.created.Slug != "new-helper" {
		t.Errorf("created slug = %q, want new-helper", f.created.Slug)
	}
	if f.created.System {
		t.Error("create must NOT let the caller set System")
	}
	if out["action"] != "created" {
		t.Errorf("action = %v, want created", out["action"])
	}
}

func TestCreateNeedsSlug(t *testing.T) {
	if _, isErr := invoke(t, newTool(&fakeSource{}), map[string]any{"op": "create", "profile": map[string]any{"soul": "x"}}); !isErr {
		t.Error("op=create without slug should error")
	}
}

var _ = agent.Tool(New())
