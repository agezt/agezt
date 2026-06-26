// SPDX-License-Identifier: MIT

package fixtures

import (
	"encoding/json"
	"os"
	"testing"
)

func readFixture(t *testing.T, name string, v any) {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
}

func TestContextCompactedSkillRescueFixture(t *testing.T) {
	var p struct {
		Elided             int `json:"elided"`
		ReclaimedChars     int `json:"reclaimed_chars"`
		ContextCharsBefore int `json:"context_chars_before"`
		ContextCharsAfter  int `json:"context_chars_after"`
		SkillRescuedCount  int `json:"skill_rescued_count"`
		SkillRescuedChars  int `json:"skill_rescued_chars"`
	}
	readFixture(t, "context_compacted_skill_rescue.json", &p)
	if p.Elided <= 0 || p.ReclaimedChars <= 0 || p.ContextCharsBefore <= p.ContextCharsAfter || p.SkillRescuedCount <= 0 || p.SkillRescuedChars <= 0 {
		t.Fatalf("invalid context rescue fixture: %+v", p)
	}
}

func TestSkillActivatedExplicitFixture(t *testing.T) {
	var p struct {
		Activation string   `json:"activation"`
		Refs       []string `json:"refs"`
		IDs        []string `json:"ids"`
	}
	readFixture(t, "skill_activated_explicit.json", &p)
	if p.Activation != "explicit" || len(p.Refs) == 0 || len(p.IDs) == 0 {
		t.Fatalf("invalid skill activation fixture: %+v", p)
	}
}

func TestSubAgentSpawnedFixture(t *testing.T) {
	var p struct {
		Task             string `json:"task"`
		ChildCorrelation string `json:"child_correlation"`
		Parent           string `json:"parent"`
		Depth            int    `json:"depth"`
		TaskType         string `json:"task_type"`
	}
	readFixture(t, "subagent_spawned.json", &p)
	if p.Task == "" || p.ChildCorrelation == "" || p.Parent == "" || p.Depth <= 0 || p.TaskType == "" {
		t.Fatalf("invalid subagent fixture: %+v", p)
	}
}

func TestSubAgentCompletedAsyncFixture(t *testing.T) {
	var p struct {
		ChildCorrelation string `json:"child_correlation"`
		OK               bool   `json:"ok"`
		Async            bool   `json:"async"`
		Chars            int    `json:"chars"`
		Error            string `json:"error"`
	}
	readFixture(t, "subagent_completed_async.json", &p)
	if p.ChildCorrelation == "" || !p.OK || !p.Async || p.Chars <= 0 || p.Error != "" {
		t.Fatalf("invalid async subagent completion fixture: %+v", p)
	}
}

func TestDelegateAwaitToolResultFixture(t *testing.T) {
	var p struct {
		Tool             string   `json:"tool"`
		CallID           string   `json:"call_id"`
		Output           string   `json:"output"`
		Error            bool     `json:"error"`
		DirectiveLike    bool     `json:"directive_like"`
		DirectiveMatches []string `json:"directive_matches"`
	}
	readFixture(t, "delegate_await_tool_result.json", &p)
	if p.Tool != "delegate_await" || p.CallID == "" || p.Output == "" || p.Error || p.DirectiveLike || len(p.DirectiveMatches) != 0 {
		t.Fatalf("invalid delegate_await fixture: %+v", p)
	}
}

func TestToolSearchResultFixture(t *testing.T) {
	var p struct {
		Query string `json:"query"`
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	readFixture(t, "tool_search_result.json", &p)
	if p.Query == "" || len(p.Tools) == 0 || p.Tools[0].Name == "" {
		t.Fatalf("invalid tool_search fixture: %+v", p)
	}
}

func TestConfigReloadBoundariesFixture(t *testing.T) {
	var p []struct {
		Apply string   `json:"apply"`
		Envs  []string `json:"envs"`
	}
	readFixture(t, "config_reload_boundaries.json", &p)
	if len(p) < 2 || p[0].Apply != "live" || p[1].Apply != "restart" || len(p[0].Envs) == 0 || len(p[1].Envs) == 0 {
		t.Fatalf("invalid config boundary fixture: %+v", p)
	}
}
