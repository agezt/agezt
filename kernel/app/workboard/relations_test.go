// SPDX-License-Identifier: MIT

package workboard_test

import (
	"context"
	"encoding/json"
	"errors"
	appwork "github.com/agezt/agezt/kernel/app/workboard"
	"github.com/agezt/agezt/kernel/runtime"
	tasks "github.com/agezt/agezt/kernel/workboard"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"testing"
	"time"
)

type relationHost struct {
	cause  error
	method string
	args   []string
	policy *tasks.RetryPolicy
	stale  time.Duration
	limit  int
	rows   []tasks.Task
}

func (h *relationHost) result(method string, args ...string) (tasks.Task, error) {
	h.method = method
	h.args = args
	return tasks.Task{ID: "owned", Title: "fixture", Status: tasks.StatusTriage, Links: []tasks.Link{{Type: "artifact", Target: "owned"}}}, h.cause
}
func (h *relationHost) LinkWorkboardTask(corr, id, typ, target string) (tasks.Task, error) {
	return h.result("link", corr, id, typ, target)
}
func (h *relationHost) SetWorkboardRetryPolicy(corr, id, actor string, policy *tasks.RetryPolicy) (tasks.Task, error) {
	h.policy = policy
	return h.result("policy", corr, id, actor)
}
func (h *relationHost) AddWorkboardDependency(corr, id, on string) (tasks.Task, error) {
	return h.result("depend", corr, id, on)
}
func (h *relationHost) ReclaimStaleWorkboardTask(corr, id, actor string, stale time.Duration) (tasks.Task, error) {
	h.stale = stale
	return h.result("reclaim", corr, id, actor)
}
func (h *relationHost) SweepStaleWorkboardClaims(corr, actor string, stale time.Duration, limit int) ([]tasks.Task, error) {
	h.method = "sweep"
	h.args = []string{corr, actor}
	h.stale = stale
	h.limit = limit
	return h.rows, h.cause
}
func TestWorkboardRelationsRetainAllFacadeInputsAndOriginalCauses(t *testing.T) {
	h := &relationHost{}
	s := appwork.NewRelations(h)
	ctx := context.Background()
	policy := &tasks.RetryPolicy{MaxAttempts: 3, EscalateTo: "lead"}
	calls := []struct {
		name string
		args []string
		run  func() error
	}{{"link", []string{"corr", "id", "artifact", "target"}, func() error {
		out, err := s.Link(ctx, appwork.LinkInput{CorrelationID: "corr", ID: "id", Type: "artifact", Target: "target"})
		if err == nil && (out.Task.ID != "owned" || out.Task.LinkCount != 1) {
			t.Fatal("typed link output changed")
		}
		return err
	}}, {"policy", []string{"corr", "id", "actor"}, func() error {
		_, err := s.Policy(ctx, appwork.PolicyInput{CorrelationID: "corr", ID: "id", Actor: "actor", Policy: policy})
		return err
	}}, {"depend", []string{"corr", "id", "parent"}, func() error {
		_, err := s.Depend(ctx, appwork.DependInput{CorrelationID: "corr", ID: "id", DependsOn: "parent"})
		return err
	}}, {"reclaim", []string{"corr", "id", "actor"}, func() error {
		_, err := s.Reclaim(ctx, appwork.ReclaimInput{CorrelationID: "corr", ID: "id", Actor: "actor", StaleAfterMS: 1234})
		return err
	}}, {"sweep", []string{"corr", "sweeper"}, func() error {
		out, err := s.Sweep(ctx, appwork.SweepInput{CorrelationID: "corr", Actor: "sweeper", StaleAfterMS: 5678, Limit: 19})
		if err == nil && (out.ReclaimedCount != 2 || out.StaleAfterMS != 5678 || len(out.Tasks) != 2 || out.Tasks[0].ID != "one" || out.Tasks[1].ID != "two") {
			t.Fatalf("sweep=%+v", out)
		}
		return err
	}}}
	cause := errors.New("owned relation failure")
	for _, tc := range calls {
		*h = relationHost{rows: []tasks.Task{{ID: "one"}, {ID: "two"}}}
		if err := tc.run(); err != nil || h.method != tc.name || !reflect.DeepEqual(h.args, tc.args) {
			t.Fatalf("%s inputs=%+v err=%v", tc.name, h, err)
		}
		if tc.name == "policy" && h.policy != policy {
			t.Fatal("policy pointer changed")
		}
		if tc.name == "reclaim" && h.stale != 1234*time.Millisecond {
			t.Fatal("reclaim duration unit changed")
		}
		if tc.name == "sweep" && (h.stale != 5678*time.Millisecond || h.limit != 19) {
			t.Fatal("sweep duration/limit changed")
		}
		h.cause = cause
		if err := tc.run(); err != cause {
			t.Fatalf("%s original cause=%v", tc.name, err)
		}
	}
	h.cause = nil
	h.rows = nil
	out, err := s.Sweep(ctx, appwork.SweepInput{StaleAfterMS: 600000, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out)
	if err != nil || string(raw) != `{"tasks":[],"reclaimed_count":0,"stale_after_ms":600000}` {
		t.Fatalf("empty sweep=%s err=%v", raw, err)
	}
	if _, err := s.Policy(ctx, appwork.PolicyInput{ID: "owned", Policy: nil}); err != nil || h.policy != nil {
		t.Fatalf("clear policy=%v pointer=%v", err, h.policy)
	}
}
func TestWorkboardRelationsRetainRealKernelDependenciesAndMaintenance(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := appwork.NewRelations(k)
	ctx := context.Background()
	a, _, err := k.Workboard().Create(tasks.CreateSpec{Title: "child"}, time.UnixMilli(100))
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := k.Workboard().Create(tasks.CreateSpec{Title: "parent"}, time.UnixMilli(100))
	if err != nil {
		t.Fatal(err)
	}
	linked, err := s.Link(ctx, appwork.LinkInput{CorrelationID: "owned", ID: a.ID, Type: "artifact", Target: "fixture"})
	if err != nil || linked.Task.LinkCount != 1 {
		t.Fatalf("link=%+v err=%v", linked, err)
	}
	dep, err := s.Depend(ctx, appwork.DependInput{CorrelationID: "owned", ID: a.ID, DependsOn: b.ID})
	if err != nil || len(dep.Task.Dependencies) != 1 || dep.Task.Dependencies[0].ID != b.ID {
		t.Fatalf("dependency=%+v err=%v", dep, err)
	}
	if _, err := s.Depend(ctx, appwork.DependInput{CorrelationID: "owned", ID: b.ID, DependsOn: a.ID}); err == nil {
		t.Fatal("dependency cycle admitted")
	}
	p, err := s.Policy(ctx, appwork.PolicyInput{CorrelationID: "owned", ID: a.ID, Actor: "owner", Policy: &tasks.RetryPolicy{MaxAttempts: 2}})
	if err != nil || p.Task.RetryPolicy == nil || p.Task.MaxAttempts != 2 {
		t.Fatalf("policy=%+v err=%v", p, err)
	}
	if _, err := k.Workboard().Claim(a.ID, "writer", "run", time.UnixMilli(101)); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := s.Reclaim(ctx, appwork.ReclaimInput{CorrelationID: "owned", ID: a.ID, Actor: "owner", StaleAfterMS: 1})
	if err != nil || reclaimed.Task.Claim != nil || reclaimed.Task.FailedAttemptCount != 1 {
		t.Fatalf("reclaim=%+v err=%v", reclaimed, err)
	}
	if _, err := k.Workboard().Claim(b.ID, "writer", "run", time.UnixMilli(101)); err != nil {
		t.Fatal(err)
	}
	swept, err := s.Sweep(ctx, appwork.SweepInput{CorrelationID: "owned", Actor: "sweeper", StaleAfterMS: 1, Limit: 1})
	if err != nil || swept.ReclaimedCount != 1 || swept.Tasks[0].ID != b.ID {
		t.Fatalf("sweep=%+v err=%v", swept, err)
	}
}
