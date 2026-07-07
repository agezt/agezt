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
	repaired   string
	repairRes  RepairResult
	deleted    string
	deleteOK   bool
	deleteErr  error
	getRef     string
	getResult  roster.Profile
	getOK      bool
	cloneSrc   string
	cloneOver  roster.Profile
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
func (f *fakeSource) SetAgentRetired(ref string, retired bool, _ string) (roster.Profile, error) {
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
func (f *fakeSource) DeleteAgent(ref string) (bool, error) {
	if f.deleteErr != nil {
		return false, f.deleteErr
	}
	f.deleted = ref
	return f.deleteOK, nil
}
func (f *fakeSource) GetAgent(ref string) (roster.Profile, bool, error) {
	if f.setErr != nil {
		return roster.Profile{}, false, f.setErr
	}
	f.getRef = ref
	if f.getOK {
		return f.getResult, true, nil
	}
	return roster.Profile{}, false, nil
}
func (f *fakeSource) CloneAgent(source string, overrides roster.Profile) (roster.Profile, error) {
	if f.setErr != nil {
		return roster.Profile{}, f.setErr
	}
	f.cloneSrc = source
	f.cloneOver = overrides
	overrides.System = false
	return overrides, nil
}
func (f *fakeSource) SearchAgents(_ SearchFilter) []roster.Profile {
	return f.agents
}
func (f *fakeSource) BulkSetEnabled(slugs []string, enabled bool) []BulkResult {
	out := make([]BulkResult, len(slugs))
	for i, slug := range slugs {
		r := BulkResult{Slug: slug, Success: true}
		if f.setErr != nil {
			r.Success = false
			r.Error = f.setErr.Error()
		}
		out[i] = r
	}
	return out
}
func (f *fakeSource) BulkSetRetired(slugs []string, retired bool, _ string) []BulkResult {
	out := make([]BulkResult, len(slugs))
	for i, slug := range slugs {
		r := BulkResult{Slug: slug, Success: true}
		if f.setErr != nil {
			r.Success = false
			r.Error = f.setErr.Error()
		}
		out[i] = r
	}
	return out
}
func (f *fakeSource) BulkDelete(slugs []string) []BulkResult {
	out := make([]BulkResult, len(slugs))
	for i, slug := range slugs {
		r := BulkResult{Slug: slug, Success: f.deleteOK}
		if f.deleteErr != nil {
			r.Success = false
			r.Error = f.deleteErr.Error()
		}
		out[i] = r
	}
	return out
}
func (f *fakeSource) WakeAgent(ref, intent, reason string) (string, error) {
	if f.setErr != nil {
		return "", f.setErr
	}
	return "corr-" + ref, nil
}
func (f *fakeSource) RepairAgent(ref, _ string) (RepairResult, error) {
	if f.setErr != nil {
		return RepairResult{}, f.setErr
	}
	f.repaired = ref
	if f.repairRes.Agent == "" {
		f.repairRes = RepairResult{Agent: ref, Correlation: "corr-repair", Applied: []string{"config_overrides"}, Answer: "done"}
	}
	return f.repairRes, nil
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
		"op":    "edit",
		"agent": "scout",
		"profile": map[string]any{
			"model": "deepseek-chat", "max_daily_mc": 5000000, "soul": "Be terse.",
			"config_overrides": map[string]any{"AGEZT_MAX_ITER": "6"},
			"trust_ceiling":    "L2",
		},
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
	if f.editFields.ConfigOverrides["AGEZT_MAX_ITER"] != "6" || f.editFields.TrustCeiling != "L2" {
		t.Errorf("edit config/policy fields not applied: %+v", f.editFields)
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

// TestCreateFlatProfile asserts the tolerant parser accepts a flattened profile
// — fields placed at the top level instead of nested under "profile" — which is
// how some models shape the args. Without this they'd hit "a profile object is
// required".
func TestCreateFlatProfile(t *testing.T) {
	f := &fakeSource{}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op": "create", "slug": "atlas", "name": "Atlas", "soul": "Be sharp.", "model": "deepseek-chat",
	})
	if isErr {
		t.Fatalf("flat create errored: %v", out)
	}
	if f.created.Slug != "atlas" || f.created.Name != "Atlas" || f.created.Model != "deepseek-chat" {
		t.Errorf("flat create fields not applied: %+v", f.created)
	}
	if out["action"] != "created" {
		t.Errorf("action = %v, want created", out["action"])
	}
}

// TestEditFlatProfile is the op=edit twin: "agent" is the control ref, the rest
// of the top-level keys are read as the profile.
func TestEditFlatProfile(t *testing.T) {
	f := &fakeSource{}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op": "edit", "agent": "scout", "model": "deepseek-chat", "soul": "Be terse.",
	})
	if isErr {
		t.Fatalf("flat edit errored: %v", out)
	}
	if f.edited != "scout" {
		t.Errorf("edited target = %q, want scout", f.edited)
	}
	if f.editFields.Model != "deepseek-chat" || f.editFields.Soul != "Be terse." {
		t.Errorf("flat edit fields not applied: %+v", f.editFields)
	}
}

// TestCreateEmptyStillErrors: a payload with neither a profile object nor any
// flat fields must still be rejected — a guardian can't silently no-op.
func TestCreateEmptyStillErrors(t *testing.T) {
	if _, isErr := invoke(t, newTool(&fakeSource{}), map[string]any{"op": "create"}); !isErr {
		t.Error("op=create with no profile and no flat fields should error")
	}
	if _, isErr := invoke(t, newTool(&fakeSource{}), map[string]any{"op": "create", "profile": map[string]any{}}); !isErr {
		t.Error("op=create with empty profile object should error")
	}
}

func TestRepairAgent(t *testing.T) {
	f := &fakeSource{repairRes: RepairResult{Agent: "builder", Correlation: "corr-1", Applied: []string{"model"}, Answer: "patched"}}
	out, isErr := invoke(t, newTool(f), map[string]any{"op": "repair", "agent": "builder", "reason": "invalid runtime override"})
	if isErr {
		t.Fatalf("repair errored: %v", out)
	}
	if f.repaired != "builder" {
		t.Fatalf("repair target = %q, want builder", f.repaired)
	}
	if out["action"] != "repair" || out["correlation"] != "corr-1" {
		t.Fatalf("repair output wrong: %+v", out)
	}
}

func TestDeleteAgent(t *testing.T) {
	f := &fakeSource{deleteOK: true}
	tool := newTool(f)

	// Successful delete.
	out, isErr := invoke(t, tool, map[string]any{"op": "delete", "agent": "scout"})
	if isErr {
		t.Fatalf("delete errored: %v", out)
	}
	if f.deleted != "scout" {
		t.Errorf("deleted target = %q, want scout", f.deleted)
	}
	if out["action"] != "deleted" || out["removed"] != true {
		t.Errorf("delete output wrong: %+v", out)
	}
}

func TestDeleteNeedsAgent(t *testing.T) {
	f := &fakeSource{deleteOK: true}
	if _, isErr := invoke(t, newTool(f), map[string]any{"op": "delete"}); !isErr {
		t.Error("op=delete without agent should error")
	}
}

func TestDeleteNotFound(t *testing.T) {
	f := &fakeSource{deleteOK: false}
	out, isErr := invoke(t, newTool(f), map[string]any{"op": "delete", "agent": "ghost"})
	if isErr {
		t.Fatalf("delete not-found errored: %v", out)
	}
	if out["removed"] != false {
		t.Errorf("expected removed=false for unknown agent, got %v", out["removed"])
	}
}

func TestDeleteSurfacesError(t *testing.T) {
	f := &fakeSource{deleteErr: errors.New("cannot remove system agent")}
	if _, isErr := invoke(t, newTool(f), map[string]any{"op": "delete", "agent": "guardian"}); !isErr {
		t.Error("a kernel error from delete should surface as a tool error")
	}
}

func TestGetAgent(t *testing.T) {
	f := &fakeSource{getOK: true, getResult: roster.Profile{Slug: "scout", Name: "Scout", Model: "deepseek-chat", Enabled: true}}
	out, isErr := invoke(t, newTool(f), map[string]any{"op": "get", "agent": "scout"})
	if isErr {
		t.Fatalf("get errored: %v", out)
	}
	if f.getRef != "scout" {
		t.Errorf("get ref = %q, want scout", f.getRef)
	}
	prof, ok := out["profile"].(map[string]any)
	if !ok {
		t.Fatalf("get output missing profile: %+v", out)
	}
	if prof["slug"] != "scout" || prof["name"] != "Scout" {
		t.Errorf("profile content wrong: %+v", prof)
	}
}

func TestGetNeedsAgent(t *testing.T) {
	if _, isErr := invoke(t, newTool(&fakeSource{}), map[string]any{"op": "get"}); !isErr {
		t.Error("op=get without agent should error")
	}
}

func TestGetNotFound(t *testing.T) {
	if _, isErr := invoke(t, newTool(&fakeSource{getOK: false}), map[string]any{"op": "get", "agent": "ghost"}); !isErr {
		t.Error("op=get for unknown agent should error")
	}
}

func TestCloneAgent(t *testing.T) {
	f := &fakeSource{}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op": "clone", "source": "scout",
		"slug": "scout-copy", "name": "Scout Copy", "model": "gpt-5",
	})
	if isErr {
		t.Fatalf("clone errored: %v", out)
	}
	if f.cloneSrc != "scout" {
		t.Errorf("clone source = %q, want scout", f.cloneSrc)
	}
	if f.cloneOver.Slug != "scout-copy" || f.cloneOver.Name != "Scout Copy" || f.cloneOver.Model != "gpt-5" {
		t.Errorf("clone overrides not applied: %+v", f.cloneOver)
	}
	if out["action"] != "cloned" || out["source"] != "scout" {
		t.Errorf("clone output wrong: %+v", out)
	}
}

func TestCloneNeedsSource(t *testing.T) {
	if _, isErr := invoke(t, newTool(&fakeSource{}), map[string]any{"op": "clone", "slug": "new-agent"}); !isErr {
		t.Error("op=clone without source should error")
	}
}

func TestCloneNeedsSlug(t *testing.T) {
	if _, isErr := invoke(t, newTool(&fakeSource{}), map[string]any{"op": "clone", "source": "scout"}); !isErr {
		t.Error("op=clone without slug in profile should error")
	}
}

func TestSearchAgents(t *testing.T) {
	f := &fakeSource{agents: []roster.Profile{
		{Slug: "scout", Enabled: true},
		{Slug: "builder", Enabled: true},
	}}
	out, isErr := invoke(t, newTool(f), map[string]any{"op": "search"})
	if isErr {
		t.Fatalf("search errored: %v", out)
	}
	if out["count"].(float64) != 2 {
		t.Errorf("search count = %v, want 2", out["count"])
	}
}

func TestSearchAgentsWithFilter(t *testing.T) {
	f := &fakeSource{agents: []roster.Profile{
		{Slug: "scout", Enabled: true},
		{Slug: "builder", Enabled: true},
	}}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op": "search",
		"filter": map[string]any{"state": "enabled"},
	})
	if isErr {
		t.Fatalf("search with filter errored: %v", out)
	}
	if out["count"].(float64) != 2 {
		t.Errorf("search count = %v, want 2", out["count"])
	}
}

func TestBulkPause(t *testing.T) {
	f := &fakeSource{}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op": "bulk_pause", "agents": []string{"a1", "a2"},
	})
	if isErr {
		t.Fatalf("bulk_pause errored: %v", out)
	}
	if out["total"].(float64) != 2 {
		t.Errorf("bulk total = %v, want 2", out["total"])
	}
}

func TestBulkUnpause(t *testing.T) {
	f := &fakeSource{}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op": "bulk_unpause", "agents": []string{"a1", "a2"},
	})
	if isErr {
		t.Fatalf("bulk_unpause errored: %v", out)
	}
	if out["total"].(float64) != 2 {
		t.Errorf("bulk total = %v, want 2", out["total"])
	}
}

func TestBulkRetire(t *testing.T) {
	f := &fakeSource{}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op": "bulk_retire", "agents": []string{"a1", "a2"}, "reason": "cleanup",
	})
	if isErr {
		t.Fatalf("bulk_retire errored: %v", out)
	}
	if out["total"].(float64) != 2 {
		t.Errorf("bulk total = %v, want 2", out["total"])
	}
}

func TestBulkRevive(t *testing.T) {
	f := &fakeSource{}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op": "bulk_revive", "agents": []string{"a1", "a2"},
	})
	if isErr {
		t.Fatalf("bulk_revive errored: %v", out)
	}
	if out["total"].(float64) != 2 {
		t.Errorf("bulk total = %v, want 2", out["total"])
	}
}

func TestBulkDelete(t *testing.T) {
	f := &fakeSource{deleteOK: true}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op": "bulk_delete", "agents": []string{"a1", "a2"},
	})
	if isErr {
		t.Fatalf("bulk_delete errored: %v", out)
	}
	if out["total"].(float64) != 2 {
		t.Errorf("bulk total = %v, want 2", out["total"])
	}
}

func TestWakeAgent(t *testing.T) {
	f := &fakeSource{}
	out, isErr := invoke(t, newTool(f), map[string]any{
		"op": "wake", "agent": "scout", "intent": "sync catalogue", "reason": "scheduled refresh",
	})
	if isErr {
		t.Fatalf("wake errored: %v", out)
	}
	if out["action"] != "woken" || out["correlation_id"] != "corr-scout" {
		t.Errorf("wake output wrong: %+v", out)
	}
}

func TestWakeNeedsAgent(t *testing.T) {
	if _, isErr := invoke(t, newTool(&fakeSource{}), map[string]any{"op": "wake", "intent": "x"}); !isErr {
		t.Error("op=wake without agent should error")
	}
}

func TestWakeNeedsIntentOrReason(t *testing.T) {
	if _, isErr := invoke(t, newTool(&fakeSource{}), map[string]any{"op": "wake", "agent": "scout"}); !isErr {
		t.Error("op=wake without intent or reason should error")
	}
}

func TestBulkNeedsAgents(t *testing.T) {
	for _, op := range []string{"bulk_pause", "bulk_unpause", "bulk_retire", "bulk_revive", "bulk_delete"} {
		if _, isErr := invoke(t, newTool(&fakeSource{}), map[string]any{"op": op}); !isErr {
			t.Errorf("op=%s without agents should error", op)
		}
	}
}

var _ = agent.Tool(New())
