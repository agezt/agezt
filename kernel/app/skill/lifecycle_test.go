// SPDX-License-Identifier: MIT
package skill_test

import (
	"context"
	"encoding/json"
	"errors"
	appskill "github.com/agezt/agezt/kernel/app/skill"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/journal"
	curated "github.com/agezt/agezt/kernel/skill"
	"testing"
)

func lifecycleFixture(t *testing.T) (*curated.Forge, *appskill.Lifecycle) {
	t.Helper()
	store, err := curated.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	forge := curated.NewForge(store, b)
	return forge, appskill.NewLifecycle(forge)
}
func TestSkillLifecycleRetainsActualTransitionsAndLineage(t *testing.T) {
	forge, s := lifecycleFixture(t)
	ctx := context.Background()
	seed, _, err := forge.Create("seed", curated.CreateSpec{Name: "fixture", Body: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []curated.Status{curated.StatusShadow, curated.StatusActive} {
		out, err := s.Promote(ctx, appskill.GetInput{ID: seed.ID})
		if err != nil || out.ID != seed.ID || out.Status != want {
			t.Fatalf("promote=%+v err=%v", out, err)
		}
	}
	if _, err := s.Promote(ctx, appskill.GetInput{ID: seed.ID}); !errors.Is(err, curated.ErrIllegalTransition) {
		t.Fatalf("illegal promote=%v", err)
	}
	out, err := s.Quarantine(ctx, appskill.ReasonInput{ID: seed.ID, Reason: "owned reason"})
	if err != nil || out.Status != curated.StatusQuarantined || out.ID != seed.ID {
		t.Fatalf("quarantine=%+v err=%v", out, err)
	}
	restored, err := s.Restore(ctx, appskill.RestoreInput{ID: seed.ID, Status: curated.StatusActive, Reason: "rollback"})
	if err != nil || restored.ID != seed.ID || restored.From != curated.StatusQuarantined || restored.Status != curated.StatusActive || restored.Reason != "rollback" {
		t.Fatalf("restore=%+v err=%v", restored, err)
	}
	if _, err := s.Restore(ctx, appskill.RestoreInput{ID: seed.ID, Status: "invalid"}); !errors.Is(err, curated.ErrIllegalTransition) {
		t.Fatalf("invalid restore=%v", err)
	}
	child, _, err := forge.Create("seed", curated.CreateSpec{Name: "fixture", Body: "v2"})
	if err != nil {
		t.Fatal(err)
	}
	revert, err := s.Revert(ctx, appskill.GetInput{ID: child.ID})
	if err != nil || revert.ID != child.ID || revert.Restored != seed.ID {
		t.Fatalf("revert=%+v err=%v", revert, err)
	}
	got, found, err := forge.Get(child.ID)
	if err != nil || !found || got.Status != curated.StatusArchived {
		t.Fatalf("child=%+v found=%v err=%v", got, found, err)
	}
	archived, err := s.Archive(ctx, appskill.ReasonInput{ID: seed.ID})
	if err != nil || archived.ID != seed.ID || archived.Status != curated.StatusArchived || archived.Reason != "" {
		t.Fatalf("archive=%+v err=%v", archived, err)
	}
	raw, err := json.Marshal(archived)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if reason, ok := fields["reason"]; !ok || reason != "" {
		t.Fatalf("empty archive reason=%s", raw)
	}
	if _, err := s.Archive(ctx, appskill.ReasonInput{ID: seed.ID}); err != nil {
		t.Fatalf("idempotent archive=%v", err)
	}
}

type lifecycleProbe struct {
	cause                    error
	id, reason, target, corr string
	calls                    int
}

func (p *lifecycleProbe) Promote(corr, id string) (curated.Status, error) {
	p.calls++
	p.corr = corr
	p.id = id
	return curated.StatusShadow, p.cause
}
func (p *lifecycleProbe) Quarantine(corr, id, reason string) error {
	p.calls++
	p.corr = corr
	p.id = id
	p.reason = reason
	return p.cause
}
func (p *lifecycleProbe) Archive(corr, id, reason string) error {
	p.calls++
	p.corr = corr
	p.id = id
	p.reason = reason
	return p.cause
}
func (p *lifecycleProbe) Revert(corr, id string) (string, error) {
	p.calls++
	p.corr = corr
	p.id = id
	return "parent", p.cause
}
func (p *lifecycleProbe) RestoreStatus(corr, id string, target curated.Status, reason string) (curated.Status, curated.Status, error) {
	p.calls++
	p.corr = corr
	p.id = id
	p.reason = reason
	p.target = string(target)
	return curated.StatusQuarantined, target, p.cause
}
func TestSkillLifecycleForwardsInputsAndOriginalCauses(t *testing.T) {
	p := &lifecycleProbe{}
	s := appskill.NewLifecycle(p)
	ctx := context.Background()
	calls := []struct {
		name   string
		run    func() error
		reason string
	}{{"promote", func() error { _, err := s.Promote(ctx, appskill.GetInput{ID: "owned"}); return err }, ""}, {"quarantine", func() error { _, err := s.Quarantine(ctx, appskill.ReasonInput{ID: "owned", Reason: "q"}); return err }, "q"}, {"archive", func() error { _, err := s.Archive(ctx, appskill.ReasonInput{ID: "owned", Reason: "a"}); return err }, "a"}, {"revert", func() error { _, err := s.Revert(ctx, appskill.GetInput{ID: "owned"}); return err }, ""}, {"restore", func() error {
		_, err := s.Restore(ctx, appskill.RestoreInput{ID: "owned", Status: curated.StatusDraft, Reason: "r"})
		return err
	}, "r"}}
	cause := errors.New("owned lifecycle failure")
	for _, tc := range calls {
		*p = lifecycleProbe{}
		if err := tc.run(); err != nil || p.calls != 1 || p.id != "owned" || p.corr != "" || p.reason != tc.reason {
			t.Fatalf("%s inputs=%+v err=%v", tc.name, p, err)
		}
		if tc.name == "restore" && p.target != "draft" {
			t.Fatalf("restore target=%q", p.target)
		}
		p.cause = cause
		if err := tc.run(); !errors.Is(err, cause) {
			t.Fatalf("%s cause=%v", tc.name, err)
		}
	}
}
