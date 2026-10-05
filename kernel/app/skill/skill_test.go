// SPDX-License-Identifier: MIT
package skill_test

import (
	"context"
	"encoding/json"
	"errors"
	appskill "github.com/agezt/agezt/kernel/app/skill"
	curated "github.com/agezt/agezt/kernel/skill"
	"reflect"
	"testing"
)

type reader struct {
	skills []curated.Skill
	cause  error
	got    string
	found  bool
}

func (r *reader) List() ([]curated.Skill, error) { return r.skills, r.cause }
func (r *reader) Get(id string) (curated.Skill, bool, error) {
	r.got = id
	if len(r.skills) > 0 {
		return r.skills[0], r.found, r.cause
	}
	return curated.Skill{}, r.found, r.cause
}
func TestSkillReadPreservesProjectionOrderAndActiveCount(t *testing.T) {
	seed := curated.Skill{ID: "owned", Name: "fixture", Description: "description", Status: curated.StatusActive, Version: "0.1.0", Agent: "writer", CreatedMS: 100, LastSeenMS: 200, Metrics: curated.Metrics{Uses: 3, Successes: 2, Failures: 1, LastUsedMS: 150, ShadowEvals: 4, ShadowWins: 3}, Triggers: []string{"ci"}, ToolsRequired: []string{"shell"}, Resources: []string{"ref.md"}, Lineage: []string{"parent"}, Body: "body", SourceEvent: "event"}
	r := &reader{skills: []curated.Skill{seed, {ID: "draft", Status: curated.StatusDraft}}, found: true}
	s := appskill.New(r)
	out, err := s.List(context.Background(), appskill.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 || out.ActiveCount != 1 || len(out.Skills) != 2 || out.Skills[0].ID != "owned" || out.Skills[1].ID != "draft" {
		t.Fatalf("list=%+v", out)
	}
	got, err := s.Get(context.Background(), appskill.GetInput{ID: "input identity"})
	if err != nil {
		t.Fatal(err)
	}
	if r.got != "input identity" || !got.Found || got.Skill == nil || !reflect.DeepEqual(*got.Skill, out.Skills[0]) {
		t.Fatalf("get=%+v input=%q", got, r.got)
	}
	raw, err := json.Marshal(got.Skill)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "name", "description", "status", "version", "agent", "created_ms", "last_seen_ms", "metrics", "triggers", "tools_required", "resources", "lineage", "body", "source_event"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("missing %s in %s", key, raw)
		}
	}
	m := fields["metrics"].(map[string]any)
	if m["uses"] != float64(3) || m["successes"] != float64(2) || m["failures"] != float64(1) || m["last_used_ms"] != float64(150) || m["shadow_evals"] != float64(4) || m["shadow_wins"] != float64(3) {
		t.Fatalf("metrics=%v", m)
	}
}
func TestSkillReadRetainsEmptyMissingAndZeroWireFields(t *testing.T) {
	r := &reader{}
	s := appskill.New(r)
	out, err := s.List(context.Background(), appskill.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"skills":[],"count":0,"active_count":0}` {
		t.Fatalf("empty=%s", raw)
	}
	missing, err := s.Get(context.Background(), appskill.GetInput{ID: "missing"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(missing)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"found":false}` {
		t.Fatalf("missing=%s", raw)
	}
	r.skills = []curated.Skill{{}}
	r.found = true
	found, err := s.Get(context.Background(), appskill.GetInput{ID: "zero"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(found.Skill)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "name", "description", "status", "version", "agent", "created_ms", "last_seen_ms", "metrics"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("missing required %s", key)
		}
	}
	for _, key := range []string{"triggers", "tools_required", "resources", "lineage", "body", "source_event"} {
		if _, ok := fields[key]; ok {
			t.Fatalf("empty optional %s present", key)
		}
	}
	m := fields["metrics"].(map[string]any)
	if len(m) != 6 {
		t.Fatalf("zero metrics=%v", m)
	}
	for _, v := range m {
		if v != float64(0) {
			t.Fatalf("nonzero metrics=%v", m)
		}
	}
}
func TestSkillReadsReturnOriginalErrorCauses(t *testing.T) {
	cause := errors.New("owned reader failure")
	r := &reader{cause: cause}
	s := appskill.New(r)
	if _, err := s.List(context.Background(), appskill.ListInput{}); !errors.Is(err, cause) {
		t.Fatalf("list cause=%v", err)
	}
	if _, err := s.Get(context.Background(), appskill.GetInput{ID: "fixture"}); !errors.Is(err, cause) {
		t.Fatalf("get cause=%v", err)
	}
}
