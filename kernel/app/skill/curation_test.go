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

func TestSkillCurationRetainsDraftDedupeOwnershipAndBundles(t *testing.T) {
	forge, _ := lifecycleFixture(t)
	bundles, err := curated.OpenBundles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	forge.SetBundles(bundles)
	s := appskill.NewCuration(forge, func(agent string) bool { return agent == "writer" || agent == "reviewer" })
	ctx := context.Background()
	in := appskill.ImportInput{Name: "fixture", Description: "description", Body: "body", Agent: "writer", Triggers: []string{"ci"}, ToolsRequired: []string{"shell"}, Resources: map[string][]byte{"refs/guide.md": []byte("owned Unicode örnek")}}
	first, err := s.Import(ctx, in)
	if err != nil || !first.Created || first.Status != curated.StatusDraft || first.ID != curated.ContentID(in.Name, in.Body) || !reflect.DeepEqual(first.Resources, []string{"refs/guide.md"}) {
		t.Fatalf("import=%+v err=%v", first, err)
	}
	data, err := bundles.Read("fixture", "refs/guide.md")
	if err != nil || string(data) != "owned Unicode örnek" {
		t.Fatalf("bundle=%s err=%v", data, err)
	}
	if _, err := forge.Promote("seed", first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := forge.Promote("seed", first.ID); err != nil {
		t.Fatal(err)
	}
	in.Agent = "reviewer"
	again, err := s.Import(ctx, in)
	if err != nil || again.Created || again.ID != first.ID || again.Status != curated.StatusActive {
		t.Fatalf("dedupe=%+v err=%v", again, err)
	}
	stored, found, err := forge.Get(first.ID)
	if err != nil || !found || stored.Agent != "writer" || stored.Description != "description" || !reflect.DeepEqual(stored.Triggers, []string{"ci"}) || !reflect.DeepEqual(stored.ToolsRequired, []string{"shell"}) {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
	shared, err := s.Share(ctx, appskill.GetInput{ID: first.ID})
	if err != nil || !shared.Shared || shared.ID != first.ID || shared.Name == nil || *shared.Name != "fixture" {
		t.Fatalf("share=%+v err=%v", shared, err)
	}
	reassigned, err := s.Reassign(ctx, appskill.ReassignInput{ID: first.ID, Agent: "reviewer"})
	if err != nil || !reassigned.Reassigned || reassigned.ToAgent != "reviewer" || reassigned.ID != first.ID || reassigned.Name == nil || *reassigned.Name != "fixture" {
		t.Fatalf("reassign=%+v err=%v", reassigned, err)
	}
	stored, _, err = forge.Get(first.ID)
	if err != nil || stored.Agent != "reviewer" || stored.Status != curated.StatusActive {
		t.Fatalf("ownership/lifecycle=%+v err=%v", stored, err)
	}
	if _, err := s.Reassign(ctx, appskill.ReassignInput{ID: first.ID, Agent: "missing"}); err == nil || err.Error() != "no such agent: missing" {
		t.Fatalf("unknown agent=%v", err)
	}
	stored, _, err = forge.Get(first.ID)
	if err != nil || stored.Agent != "reviewer" {
		t.Fatalf("rejected reassign changed owner=%+v err=%v", stored, err)
	}
	if _, err := s.Reassign(ctx, appskill.ReassignInput{ID: first.ID}); err != nil {
		t.Fatal(err)
	}
	stored, _, err = forge.Get(first.ID)
	if err != nil || stored.Agent != "" {
		t.Fatalf("empty agent share=%+v err=%v", stored, err)
	}
}

type curationProbe struct {
	cause           error
	calls           int
	corr, id, agent string
	found           bool
	spec            curated.CreateSpec
}

func (p *curationProbe) Reassign(corr, id, agent string) (curated.Skill, bool, error) {
	p.calls++
	p.corr = corr
	p.id = id
	p.agent = agent
	return curated.Skill{Name: ""}, p.found, p.cause
}
func (p *curationProbe) Create(corr string, spec curated.CreateSpec) (curated.Skill, bool, error) {
	p.calls++
	p.corr = corr
	p.spec = spec
	return curated.Skill{ID: "new", Name: spec.Name, Status: curated.StatusDraft}, true, p.cause
}
func TestSkillCurationRetainsAdmissionWireAndCauses(t *testing.T) {
	p := &curationProbe{}
	s := appskill.NewCuration(p, nil)
	ctx := context.Background()
	if _, err := s.Reassign(ctx, appskill.ReassignInput{ID: "owned", Agent: "unknown"}); err == nil || p.calls != 0 {
		t.Fatalf("roster admission err=%v calls=%d", err, p.calls)
	}
	out, err := s.Share(ctx, appskill.GetInput{ID: "owned"})
	if err != nil || out.Shared || out.Name != nil || out.ID != "owned" || p.agent != "" || p.corr != "" {
		t.Fatalf("missing share=%+v probe=%+v err=%v", out, p, err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"shared":false,"id":"owned"}` {
		t.Fatalf("missing share wire=%s", raw)
	}
	p.found = true
	out, err = s.Share(ctx, appskill.GetInput{ID: "owned"})
	if err != nil || !out.Shared || out.Name == nil || *out.Name != "" {
		t.Fatalf("present empty name=%+v err=%v", out, err)
	}
	reassigned, err := s.Reassign(ctx, appskill.ReassignInput{ID: "owned"})
	if err != nil || !reassigned.Reassigned || reassigned.ID != "owned" || reassigned.ToAgent != "" || reassigned.Name == nil {
		t.Fatalf("reassign=%+v err=%v", reassigned, err)
	}
	raw, err = json.Marshal(reassigned)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if v, ok := fields["to_agent"]; !ok || v != "" {
		t.Fatalf("required empty target=%s", raw)
	}
	in := appskill.ImportInput{Name: "name", Description: "description", Body: "body", Agent: "private", Triggers: []string{"ci"}, ToolsRequired: []string{"shell"}, Resources: map[string][]byte{"ref.md": []byte("owned")}}
	imported, err := s.Import(ctx, in)
	if err != nil || imported.ID != "new" || imported.Name != "name" || !imported.Created || imported.Status != curated.StatusDraft {
		t.Fatalf("import=%+v err=%v", imported, err)
	}
	want := curated.CreateSpec{Name: in.Name, Description: in.Description, Body: in.Body, Agent: in.Agent, Triggers: in.Triggers, ToolsRequired: in.ToolsRequired, Resources: in.Resources}
	if p.corr != "" || !reflect.DeepEqual(p.spec, want) {
		t.Fatalf("import input=%+v", p)
	}
	raw, err = json.Marshal(imported)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if v, ok := fields["resources"]; !ok || v != nil {
		t.Fatalf("nil resource manifest=%s", raw)
	}
	cause := errors.New("owned curation failure")
	p.cause = cause
	for _, run := range []func() error{func() error { _, err := s.Share(ctx, appskill.GetInput{ID: "owned"}); return err }, func() error { _, err := s.Reassign(ctx, appskill.ReassignInput{ID: "owned"}); return err }, func() error { _, err := s.Import(ctx, in); return err }} {
		if err := run(); !errors.Is(err, cause) {
			t.Fatalf("curation cause=%v", err)
		}
	}
}
