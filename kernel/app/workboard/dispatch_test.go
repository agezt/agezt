// SPDX-License-Identifier: MIT

package workboard_test

import (
	"context"
	"errors"
	appwork "github.com/agezt/agezt/kernel/app/workboard"
	tasks "github.com/agezt/agezt/kernel/workboard"
	"reflect"
	"strings"
	"testing"
	"time"
)

type dispatchHost struct {
	correlations        int
	claimArgs, linkArgs []string
	claimErr, linkErr   error
	order               []string
}

func (h *dispatchHost) NewCorrelation() string {
	h.correlations++
	h.order = append(h.order, "correlation")
	return "owned-corr"
}
func (h *dispatchHost) ClaimWorkboardTask(corr, id, agent, run string) (tasks.Task, error) {
	h.claimArgs = []string{corr, id, agent, run}
	h.order = append(h.order, "claim")
	return tasks.Task{ID: id, Title: "fixture", Status: tasks.StatusRunning, Assignee: agent, Claim: &tasks.Claim{Agent: agent, RunID: run}}, h.claimErr
}
func (h *dispatchHost) LinkWorkboardTask(corr, id, typ, target string) (tasks.Task, error) {
	h.linkArgs = []string{corr, id, typ, target}
	h.order = append(h.order, "link")
	return tasks.Task{ID: id, Title: "fixture", Status: tasks.StatusRunning, Links: []tasks.Link{{Type: typ, Target: target}}}, h.linkErr
}
func TestWorkboardDispatchAdmissionPrecedesEveryEffect(t *testing.T) {
	for _, name := range []string{"empty", "missing-task", "dependency-error", "blocked", "no-agent", "missing-agent", "retired", "paused", "managed"} {
		t.Run(name, func(t *testing.T) {
			store := &watchStore{found: true, task: tasks.Task{ID: "owned", Assignee: "writer"}}
			host := &dispatchHost{}
			agent := appwork.DispatchAgent{Slug: "writer", Enabled: true, DirectAllowed: true, DirectError: "managed-source-error", Run: func(string, tasks.Task, string, string) { t.Error("rejected admission started") }}
			found := true
			id := "owned"
			cause := errors.New("owned dependency cause")
			switch name {
			case "empty":
				id = ""
			case "missing-task":
				store.found = false
			case "dependency-error":
				store.cause = cause
			case "blocked":
				store.blocked = []tasks.DependencyState{{ID: "parent", Status: tasks.StatusTriage, Title: "parent", Missing: true}}
			case "no-agent":
				store.task.Assignee = ""
			case "missing-agent":
				found = false
			case "retired":
				agent.Retired = true
			case "paused":
				agent.Enabled = false
			case "managed":
				agent.DirectAllowed = false
			}
			publishes := 0
			service := appwork.NewDispatch(store, host, func(string) (appwork.DispatchAgent, bool) { return agent, found }, func(string, tasks.Task, string, string, string, string, string) { publishes++ })
			_, err := service.Dispatch(context.Background(), appwork.DispatchInput{ID: id})
			if err == nil || host.correlations != 0 || host.claimArgs != nil || host.linkArgs != nil || publishes != 0 {
				t.Fatalf("%s err=%v host=%+v published=%d", name, err, host, publishes)
			}
			if name == "dependency-error" && err != cause {
				t.Fatal("dependency cause changed")
			}
			if name == "managed" && err.Error() != "managed-source-error" {
				t.Fatal("managed hint changed")
			}
			if name == "blocked" && !strings.Contains(err.Error(), "parent(parent:missing)") {
				t.Fatalf("dependency summary=%v", err)
			}
		})
	}
}
func TestWorkboardDispatchRetainsAcceptedOrderAndBackgroundInputs(t *testing.T) {
	store := &watchStore{found: true, task: tasks.Task{ID: "canonical", Assignee: " writer "}}
	host := &dispatchHost{}
	type run struct {
		corr           string
		task           tasks.Task
		intent, reason string
	}
	started := make(chan run, 1)
	resolved := ""
	agent := appwork.DispatchAgent{Slug: "canonical-agent", Enabled: true, DirectAllowed: true, Run: func(corr string, task tasks.Task, intent, reason string) { started <- run{corr, task, intent, reason} }}
	service := appwork.NewDispatch(store, host, func(ref string) (appwork.DispatchAgent, bool) { resolved = ref; return agent, true }, func(corr string, task tasks.Task, phase, slug, reason, answer, errText string) {
		host.order = append(host.order, "publish")
		if corr != "owned-corr" || task.ID != "canonical" || len(task.Links) != 1 || phase != "requested" || slug != "canonical-agent" || reason != "workboard dispatch" || answer != "" || errText != "" {
			t.Fatal("request publication changed")
		}
	})
	out, err := service.Dispatch(context.Background(), appwork.DispatchInput{ID: "raw", Reason: " "})
	if err != nil || !out.Accepted || out.Task.ID != "canonical" || out.Task.LinkCount != 1 || out.Agent != "canonical-agent" || out.CorrelationID != "owned-corr" || resolved != "writer" || !reflect.DeepEqual(host.claimArgs, []string{"owned-corr", "canonical", "canonical-agent", "owned-corr"}) || !reflect.DeepEqual(host.linkArgs, []string{"owned-corr", "canonical", "run", "owned-corr"}) || !reflect.DeepEqual(host.order, []string{"correlation", "claim", "link", "publish"}) {
		t.Fatalf("accepted=%+v host=%+v resolved=%q err=%v", out, host, resolved, err)
	}
	select {
	case input := <-started:
		if input.corr != "owned-corr" || input.task.ID != "canonical" || input.reason != "workboard dispatch" || !strings.Contains(input.intent, "Task ID: canonical") {
			t.Fatalf("background=%+v", input)
		}
	case <-time.After(time.Second):
		t.Fatal("background did not start")
	}
}
func TestWorkboardDispatchReturnsMutationCausesBeforePublishAndStart(t *testing.T) {
	for _, step := range []string{"claim", "link"} {
		store := &watchStore{found: true, task: tasks.Task{ID: "owned", Assignee: "writer"}}
		host := &dispatchHost{}
		cause := errors.New("owned mutation cause")
		if step == "claim" {
			host.claimErr = cause
		} else {
			host.linkErr = cause
		}
		calls := 0
		service := appwork.NewDispatch(store, host, func(string) (appwork.DispatchAgent, bool) {
			return appwork.DispatchAgent{Slug: "writer", Enabled: true, DirectAllowed: true, Run: func(string, tasks.Task, string, string) { t.Error("failed mutation started") }}, true
		}, func(string, tasks.Task, string, string, string, string, string) { calls++ })
		if _, err := service.Dispatch(context.Background(), appwork.DispatchInput{ID: "owned"}); err != cause || calls != 0 {
			t.Fatalf("%s err=%v publishes=%d", step, err, calls)
		}
		if step == "claim" && host.linkArgs != nil {
			t.Fatal("link followed failed claim")
		}
	}
}
func TestWorkboardDispatchIntentPreservesExplicitAndTaskFields(t *testing.T) {
	task := tasks.Task{ID: "owned", Title: "title", Status: tasks.StatusReady, Priority: 3, Tenant: "acme", Description: "description", Tags: []string{"one", "two"}}
	if got := appwork.DispatchIntent(" explicit ", task); got != "explicit" {
		t.Fatalf("explicit=%q", got)
	}
	got := appwork.DispatchIntent("", task)
	for _, text := range []string{"Task ID: owned", "Title: title", "Status: ready", "Priority: 3", "Tenant: acme", "Description:\ndescription", "Tags: one, two", `"id":"owned"`} {
		if !strings.Contains(got, text) {
			t.Fatalf("intent missing %q: %s", text, got)
		}
	}
}
