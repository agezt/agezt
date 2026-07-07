// SPDX-License-Identifier: MIT

package introspecttool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/standing"
)

// fakeSource is an in-memory Source so the tool's formatting/dispatch is testable
// without standing up a kernel.
type fakeSource struct {
	ov     Overview
	reaper ReaperReport
	sch    []cadence.Entry
	stnd   []standing.Order
}

func (f *fakeSource) Overview() Overview         { return f.ov }
func (f *fakeSource) Reaper() ReaperReport       { return f.reaper }
func (f *fakeSource) Schedules() []cadence.Entry { return f.sch }
func (f *fakeSource) Standing() []standing.Order { return f.stnd }

func newBound() (*Tool, *fakeSource) {
	src := &fakeSource{
		ov: Overview{
			Daemon: "1.0.0", Protocol: 1, Model: "deepseek-chat",
			UptimeSeconds: 42, Halted: false, ActiveRuns: 1,
			Tools:         []string{"introspect", "memory", "shell"},
			MemoryRecords: 7, WorldEntities: 3, ActiveSkills: 2,
			JournalHead: 99, SchedulesTotal: 2, SchedulesEnabled: 1,
			PendingApprovals:  1,
			ProviderFallbacks: 0,
			Reaper:            ReaperOverview{DeadAgents: 1, DegradedAgents: 2, MisconfiguredAgents: 1, DeadSlugs: []string{"idle"}, MisconfiguredSlugs: []string{"builder"}},
			Delegation:        Delegation{Enabled: true, MaxDepth: 1},
		},
		reaper: ReaperReport{
			DeadAgents:          []ReaperAgent{{Slug: "idle"}},
			DegradedAgents:      []ReaperDegradedAgent{{Slug: "worker", Failures: 2}},
			MisconfiguredAgents: []ReaperMisconfiguredAgent{{Slug: "builder", Issues: []string{"AGEZT_MAX_ITER: must be an integer"}}},
		},
		sch: []cadence.Entry{
			{ID: "s-late", Intent: "weekly report", Mode: cadence.ModeOnce, Source: "operator", Enabled: true, NextRunUnix: 5000},
			{ID: "s-soon", Intent: "morning brief", Mode: cadence.ModeOnce, Source: "operator", Enabled: true, NextRunUnix: 1000, Fires: 3, Model: "deepseek-chat"},
		},
		stnd: []standing.Order{
			{
				ID: "o1", Name: "error-watch", Enabled: true,
				Triggers:   []standing.Trigger{{Type: standing.TriggerEvent, Subject: "task.failed"}},
				Initiative: standing.Initiative{Mode: standing.InitiativeAsk},
				Plan:       "investigate the failure", Assure: 2,
			},
		},
	}
	tl := New()
	tl.Bind(src)
	return tl, src
}

func invoke(t *testing.T, tl *Tool, raw string) (string, bool) {
	t.Helper()
	res, err := tl.Invoke(context.Background(), json.RawMessage(raw))
	if err != nil {
		t.Fatalf("Invoke(%s) returned a hard error: %v", raw, err)
	}
	return res.Output, res.IsError
}

func TestOverview_DefaultAndExplicit(t *testing.T) {
	tl, _ := newBound()
	for _, raw := range []string{`{}`, ``, `{"op":"overview"}`} {
		out, isErr := invoke(t, tl, raw)
		if isErr {
			t.Fatalf("overview(%q) was an error result: %s", raw, out)
		}
		var ov Overview
		if err := json.Unmarshal([]byte(out), &ov); err != nil {
			t.Fatalf("overview(%q) is not valid Overview JSON: %v\n%s", raw, err, out)
		}
		if ov.Model != "deepseek-chat" || ov.MemoryRecords != 7 || ov.ActiveRuns != 1 {
			t.Errorf("overview(%q) lost fields: %+v", raw, ov)
		}
		if ov.Reaper.MisconfiguredAgents != 1 || len(ov.Reaper.DeadSlugs) != 1 {
			t.Errorf("overview(%q) reaper wrong: %+v", raw, ov.Reaper)
		}
		if !ov.Delegation.Enabled || ov.Delegation.MaxDepth != 1 {
			t.Errorf("overview(%q) delegation wrong: %+v", raw, ov.Delegation)
		}
		// The agent must see its own capability surface.
		if !strings.Contains(out, "introspect") || !strings.Contains(out, "memory") {
			t.Errorf("overview(%q) should list registered tools, got:\n%s", raw, out)
		}
	}
}

func TestSchedules_ListedAndSortedBySoonest(t *testing.T) {
	tl, _ := newBound()
	out, isErr := invoke(t, tl, `{"op":"schedules"}`)
	if isErr {
		t.Fatalf("schedules error: %s", out)
	}
	var got struct {
		Count     int `json:"count"`
		Schedules []struct {
			ID          string `json:"id"`
			Cadence     string `json:"cadence"`
			NextRunUnix int64  `json:"next_run_unix"`
			Fires       int64  `json:"fires"`
		} `json:"schedules"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("schedules not JSON: %v\n%s", err, out)
	}
	if got.Count != 2 {
		t.Fatalf("count = %d, want 2", got.Count)
	}
	// Soonest next run first.
	if got.Schedules[0].ID != "s-soon" || got.Schedules[1].ID != "s-late" {
		t.Errorf("schedules not sorted by soonest next run: %+v", got.Schedules)
	}
	if got.Schedules[0].Cadence == "" {
		t.Error("schedule should carry a human-readable cadence string")
	}
}

func TestReaper_Listed(t *testing.T) {
	tl, _ := newBound()
	out, isErr := invoke(t, tl, `{"op":"reaper"}`)
	if isErr {
		t.Fatalf("reaper error: %s", out)
	}
	var got ReaperReport
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("reaper not JSON: %v\n%s", err, out)
	}
	if len(got.MisconfiguredAgents) != 1 || got.MisconfiguredAgents[0].Slug != "builder" {
		t.Fatalf("reaper rows wrong: %+v", got)
	}
}

func TestStanding_Listed(t *testing.T) {
	tl, _ := newBound()
	out, isErr := invoke(t, tl, `{"op":"standing"}`)
	if isErr {
		t.Fatalf("standing error: %s", out)
	}
	var got struct {
		Count  int `json:"count"`
		Orders []struct {
			Name     string `json:"name"`
			Mode     string `json:"mode"`
			Plan     string `json:"plan"`
			Assure   int    `json:"assure"`
			Triggers []struct {
				Type    string `json:"type"`
				Subject string `json:"subject"`
			} `json:"triggers"`
		} `json:"orders"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("standing not JSON: %v\n%s", err, out)
	}
	if got.Count != 1 || got.Orders[0].Name != "error-watch" {
		t.Fatalf("standing orders wrong: %+v", got)
	}
	o := got.Orders[0]
	if o.Mode != "ask" || o.Assure != 2 || o.Triggers[0].Subject != "task.failed" {
		t.Errorf("standing order detail lost: %+v", o)
	}
}

func TestDefinition_ReturnsStaticDef(t *testing.T) {
	tl := New()
	def := tl.Definition()
	if def.Name != "introspect" {
		t.Errorf("Definition().Name = %q, want %q", def.Name, "introspect")
	}
	if def.Description == "" {
		t.Error("Definition().Description should not be empty")
	}
	if def.InputSchema == nil {
		t.Error("Definition().InputSchema should not be nil")
	}
}

func TestScheduleView_OmitsZeroFields(t *testing.T) {
	// Entry with all zero optional fields — they should be absent from the map.
	v := scheduleView(cadence.Entry{
		ID: "s-test", Intent: "test", Mode: cadence.ModeOnce,
		Source: "operator", Enabled: true, NextRunUnix: 1000,
	})
	if v["last_run_unix"] != nil {
		t.Error("scheduleView should omit last_run_unix when it's 0")
	}
	if v["fires"] != nil {
		t.Error("scheduleView should omit fires when it's 0")
	}
	if v["model"] != nil {
		t.Error("scheduleView should omit model when it's empty")
	}
	if v["assure"] != nil {
		t.Error("scheduleView should omit assure when it's 0")
	}
}

func TestScheduleView_IncludesNonZeroFields(t *testing.T) {
	v := scheduleView(cadence.Entry{
		ID: "s-test", Intent: "test", Mode: cadence.ModeOnce,
		Source: "operator", Enabled: true, NextRunUnix: 1000,
		LastRunUnix: 500, Fires: 3, Model: "deepseek", Assure: 2,
	})
	if v["last_run_unix"].(int64) != 500 {
		t.Error("scheduleView should include last_run_unix when > 0")
	}
	if v["fires"].(int64) != 3 {
		t.Error("scheduleView should include fires when > 0")
	}
	if v["model"].(string) != "deepseek" {
		t.Error("scheduleView should include model when non-empty")
	}
	if v["assure"].(int) != 2 {
		t.Error("scheduleView should include assure when > 0")
	}
}

func TestNext_ReturnsZeroForMissingKey(t *testing.T) {
	got := next(map[string]any{"no_next": "value"})
	if got != 0 {
		t.Errorf("next(no next_run_unix) = %d, want 0", got)
	}
}

func TestNext_ReturnsValue(t *testing.T) {
	got := next(map[string]any{"next_run_unix": int64(42)})
	if got != 42 {
		t.Errorf("next = %d, want 42", got)
	}
}

func TestStandingView_OmitsZeroFields(t *testing.T) {
	o := standingView(standing.Order{
		ID: "o1", Name: "test", Enabled: true,
		Initiative: standing.Initiative{Mode: standing.InitiativeAsk},
		Plan:       "do something",
	})
	if o["assure"] != nil {
		t.Error("standingView should omit assure when it's 0")
	}
}

func TestStandingView_IncludesScheduleAndSubject(t *testing.T) {
	o := standingView(standing.Order{
		ID: "o1", Name: "test", Enabled: true,
		Triggers: []standing.Trigger{
			{Type: standing.TriggerCron, Schedule: "0 * * * *"},
			{Type: standing.TriggerEvent, Subject: "task.failed"},
		},
		Initiative: standing.Initiative{Mode: standing.InitiativeAsk},
		Plan: "do something", Assure: 2,
	})
	if o["assure"].(int) != 2 {
		t.Error("standingView should include assure when > 0")
	}
	trigs := o["triggers"].([]map[string]any)
	if len(trigs) != 2 {
		t.Fatalf("standingView triggers = %d, want 2", len(trigs))
	}
	if trigs[0]["schedule"] != "0 * * * *" {
		t.Error("standingView trigger 0 should have schedule field")
	}
	if trigs[1]["subject"] != "task.failed" {
		t.Error("standingView trigger 1 should have subject field")
	}
}

func TestOkJSON_MarshalError(t *testing.T) {
	// json.MarshalIndent only fails on types like channel,
	// circular refs, or non-serializable values.
	res := okJSON(make(chan int)) // channels can't be marshalled
	if !res.IsError {
		t.Error("okJSON(channel) should return an error result")
	}
}

func TestOkJSON_Success(t *testing.T) {
	res := okJSON(map[string]string{"hello": "world"})
	if res.IsError {
		t.Fatalf("okJSON(valid) returned error: %s", res.Output)
	}
	if !strings.Contains(res.Output, `"hello"`) {
		t.Errorf("okJSON output missing key: %s", res.Output)
	}
}

func TestUnboundAndUnknownOp(t *testing.T) {
	// Unbound: no Source wired.
	if out, isErr := invoke(t, New(), `{"op":"overview"}`); !isErr || !strings.Contains(out, "not available") {
		t.Errorf("unbound tool should error gracefully, got isErr=%v out=%q", isErr, out)
	}
	// Unknown op on a bound tool.
	tl, _ := newBound()
	if out, isErr := invoke(t, tl, `{"op":"frobnicate"}`); !isErr || !strings.Contains(out, "unknown op") {
		t.Errorf("unknown op should error, got isErr=%v out=%q", isErr, out)
	}
}
