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
	ov   Overview
	sch  []cadence.Entry
	stnd []standing.Order
}

func (f *fakeSource) Overview() Overview         { return f.ov }
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
			Delegation:        Delegation{Enabled: true, MaxDepth: 1},
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
