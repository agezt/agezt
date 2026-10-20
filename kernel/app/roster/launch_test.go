// SPDX-License-Identifier: MIT

package roster

import (
	"errors"
	"strings"
	"testing"

	core "github.com/agezt/agezt/kernel/roster"
)

// launchRecorder collects what a launcher published.
type launchRecorder struct {
	subjects []string
	corrs    []string
	payloads []map[string]any
}

func (r *launchRecorder) publish(subject, corr string, payload map[string]any) {
	r.subjects = append(r.subjects, subject)
	r.corrs = append(r.corrs, corr)
	r.payloads = append(r.payloads, payload)
}

func (r *launchRecorder) last() map[string]any {
	if len(r.payloads) == 0 {
		return nil
	}
	return r.payloads[len(r.payloads)-1]
}

func TestLaunchOperatorWake_Completed(t *testing.T) {
	rec := &launchRecorder{}
	var gotRetry *core.RetryPolicy
	var gotCeiling int64
	ports := WakeLaunchPorts{
		Run: func(corr, intent string, retry *core.RetryPolicy, maxCostMc int64) (string, error) {
			gotRetry = retry
			gotCeiling = maxCostMc
			return "the answer", nil
		},
		Publish: rec.publish,
	}
	p := core.Profile{Slug: "ops", Soul: "s", Enabled: true}
	LaunchOperatorWake(ports, p, "corr-1", "do the thing", "why", IncidentLineage{IncidentID: "i", RootIncidentID: "r", ParentIncidentID: "p"})

	if len(rec.subjects) != 1 || rec.subjects[0] != "agent.wake" {
		t.Fatalf("published = %v, want one agent.wake", rec.subjects)
	}
	if rec.corrs[0] != "corr-1" {
		t.Errorf("correlation = %q, want corr-1", rec.corrs[0])
	}
	last := rec.last()
	if last["phase"] != "completed" || last["agent"] != "ops" || last["reason"] != "why" {
		t.Errorf("payload = %+v", last)
	}
	if last["answer"] != "the answer" {
		t.Errorf("answer = %v", last["answer"])
	}
	if last["incident_id"] != "i" || last["root_incident_id"] != "r" || last["parent_incident_id"] != "p" {
		t.Errorf("lineage = %+v", last)
	}
	runbook, ok := last["autonomy_runbook"].(map[string]any)
	if !ok || len(runbook) == 0 {
		t.Errorf("runbook = %v, want the canonical runbook", last["autonomy_runbook"])
	} else if runbook["trigger_contract"] != "operator_schedule_channel" {
		t.Errorf("runbook trigger = %v, want the operator schedule channel", runbook["trigger_contract"])
	}
	// A profile without a retry policy runs plain.
	if gotRetry != nil {
		t.Errorf("retry = %+v, want a plain run", gotRetry)
	}
	// A profile without a cost ceiling runs without one.
	if gotCeiling != 0 {
		t.Errorf("ceiling = %d, want no ceiling", gotCeiling)
	}
}

func TestLaunchOperatorWake_Failed(t *testing.T) {
	rec := &launchRecorder{}
	ports := WakeLaunchPorts{
		Run: func(corr, intent string, retry *core.RetryPolicy, maxCostMc int64) (string, error) {
			return "", errors.New("provider down")
		},
		Publish: rec.publish,
	}
	LaunchOperatorWake(ports, core.Profile{Slug: "ops"}, "corr-1", "intent", "why", IncidentLineage{})
	last := rec.last()
	if last["phase"] != "failed" || last["error"] != "provider down" {
		t.Errorf("payload = %+v, want the failed phase and the error", last)
	}
	if _, ok := last["answer"]; ok {
		t.Errorf("a failed wake has no answer: %+v", last)
	}
}

func TestLaunchOperatorWake_RetriesWhenTheProfileAsksFor(t *testing.T) {
	rec := &launchRecorder{}
	var gotRetry *core.RetryPolicy
	ports := WakeLaunchPorts{
		Run: func(corr, intent string, retry *core.RetryPolicy, maxCostMc int64) (string, error) {
			gotRetry = retry
			return "ok", nil
		},
		Publish: rec.publish,
	}
	p := core.Profile{Slug: "ops", RetryPolicy: &core.RetryPolicy{MaxAttempts: 3, Backoff: "fixed"}}
	LaunchOperatorWake(ports, p, "corr-1", "intent", "why", IncidentLineage{})
	if gotRetry == nil || gotRetry.MaxAttempts != 3 {
		t.Errorf("retry = %+v, want the profile's retry policy", gotRetry)
	}
}

// A single-attempt policy is a plain run: the profile's policy is not a
// request to retry.
func TestLaunchOperatorWake_SingleAttemptPolicyRunsPlain(t *testing.T) {
	rec := &launchRecorder{}
	var gotRetry *core.RetryPolicy
	ports := WakeLaunchPorts{
		Run: func(corr, intent string, retry *core.RetryPolicy, maxCostMc int64) (string, error) {
			gotRetry = retry
			return "ok", nil
		},
		Publish: rec.publish,
	}
	p := core.Profile{Slug: "ops", RetryPolicy: &core.RetryPolicy{MaxAttempts: 1}}
	LaunchOperatorWake(ports, p, "corr-1", "intent", "why", IncidentLineage{})
	if gotRetry != nil {
		t.Errorf("retry = %+v, want a plain run", gotRetry)
	}
}

// The agent's cost ceiling is the application's decision: the profile's own
// ceiling reaches the host, and a profile without one runs without a ceiling.
func TestLaunchOperatorWake_CostCeiling(t *testing.T) {
	rec := &launchRecorder{}
	var gotCeiling int64
	ports := WakeLaunchPorts{
		Run: func(corr, intent string, retry *core.RetryPolicy, maxCostMc int64) (string, error) {
			gotCeiling = maxCostMc
			return "ok", nil
		},
		Publish: rec.publish,
	}
	p := core.Profile{Slug: "ops", MaxCostMc: 500}
	LaunchOperatorWake(ports, p, "corr-1", "intent", "why", IncidentLineage{})
	if gotCeiling != 500 {
		t.Errorf("ceiling = %d, want the profile's ceiling", gotCeiling)
	}
}

func TestLaunchOperatorWake_AnswerIsTruncated(t *testing.T) {
	rec := &launchRecorder{}
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'x'
	}
	ports := WakeLaunchPorts{
		Run: func(corr, intent string, retry *core.RetryPolicy, maxCostMc int64) (string, error) {
			return string(long), nil
		},
		Publish: rec.publish,
	}
	LaunchOperatorWake(ports, core.Profile{Slug: "ops"}, "corr-1", "intent", "why", IncidentLineage{})
	answer, _ := rec.last()["answer"].(string)
	if len(answer) >= len(long) || !strings.HasSuffix(answer, "…") {
		t.Errorf("answer length = %d, want it truncated with an ellipsis", len(answer))
	}
}

func TestLaunchOperatorRepair_Completed(t *testing.T) {
	rec := &launchRecorder{}
	var gotSlug, gotReason string
	ports := RepairLaunchPorts{
		Repair: func(slug, reason string) (RepairOutcome, error) {
			gotSlug, gotReason = slug, reason
			return RepairOutcome{
				Applied:                       []string{"restart"},
				RoutingTaskType:               "code",
				RoutingTaskModelChain:         []string{"a", "b"},
				PreviousRoutingTaskModelChain: []string{"c"},
				Answer:                        "repaired",
			}, nil
		},
		Publish: rec.publish,
	}
	LaunchOperatorRepair(ports, core.Profile{Slug: "ops"}, "corr-1", "why", IncidentLineage{IncidentID: "i"})

	if gotSlug != "ops" || gotReason != "why" {
		t.Errorf("repair = %q/%q, want the agent and the reason", gotSlug, gotReason)
	}
	if len(rec.subjects) != 1 || rec.subjects[0] != "agent.repair" {
		t.Fatalf("published = %v, want one agent.repair", rec.subjects)
	}
	last := rec.last()
	if last["phase"] != "completed" || last["answer"] != "repaired" {
		t.Errorf("payload = %+v", last)
	}
	if last["routing_task_type"] != "code" {
		t.Errorf("task type = %v, want code", last["routing_task_type"])
	}
	applied, _ := last["applied"].([]string)
	if len(applied) != 1 || applied[0] != "restart" {
		t.Errorf("applied = %v", last["applied"])
	}
	chain, _ := last["routing_task_model_chain"].([]string)
	if len(chain) != 2 {
		t.Errorf("chain = %v", last["routing_task_model_chain"])
	}
	if last["incident_id"] != "i" {
		t.Errorf("lineage = %+v", last)
	}
}

func TestLaunchOperatorRepair_AnswerIsTruncated(t *testing.T) {
	rec := &launchRecorder{}
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'x'
	}
	ports := RepairLaunchPorts{
		Repair: func(slug, reason string) (RepairOutcome, error) {
			return RepairOutcome{Answer: string(long)}, nil
		},
		Publish: rec.publish,
	}
	LaunchOperatorRepair(ports, core.Profile{Slug: "ops"}, "corr-1", "why", IncidentLineage{})
	answer, _ := rec.last()["answer"].(string)
	if len(answer) >= len(long) || !strings.HasSuffix(answer, "…") {
		t.Errorf("answer length = %d, want it truncated with an ellipsis", len(answer))
	}
}

func TestLaunchOperatorRepair_Failed(t *testing.T) {
	rec := &launchRecorder{}
	ports := RepairLaunchPorts{
		Repair: func(slug, reason string) (RepairOutcome, error) {
			return RepairOutcome{}, errors.New("repair refused")
		},
		Publish: rec.publish,
	}
	LaunchOperatorRepair(ports, core.Profile{Slug: "ops"}, "corr-1", "why", IncidentLineage{})
	last := rec.last()
	if last["phase"] != "failed" || last["error"] != "repair refused" {
		t.Errorf("payload = %+v, want the failed phase and the error", last)
	}
}

// The panic firewall (WF-001): a panic inside the governed repair is caught,
// reported and journaled as a failed repair.
func TestLaunchOperatorRepair_ContainsPanics(t *testing.T) {
	rec := &launchRecorder{}
	var panickedSlug string
	var panicked any
	ports := RepairLaunchPorts{
		Repair: func(slug, reason string) (RepairOutcome, error) {
			panic("boom")
		},
		Publish: rec.publish,
		OnPanic: func(slug string, r any) {
			panickedSlug, panicked = slug, r
		},
	}
	LaunchOperatorRepair(ports, core.Profile{Slug: "ops"}, "corr-1", "why", IncidentLineage{IncidentID: "i"})

	if panickedSlug != "ops" || panicked != "boom" {
		t.Errorf("panic = %q/%v, want the agent and the value", panickedSlug, panicked)
	}
	last := rec.last()
	if len(rec.subjects) != 1 || rec.subjects[0] != "agent.repair" {
		t.Fatalf("published = %v, want one agent.repair", rec.subjects)
	}
	if last["phase"] != "failed" {
		t.Fatalf("payload = %+v, want the failed phase", last)
	}
	if last["error"] != "repair panicked: boom" {
		t.Errorf("error = %v, want the panic message", last["error"])
	}
	if last["incident_id"] != "i" {
		t.Errorf("lineage = %+v", last)
	}
}

// A repair that panics before it publishes anything still leaves the failed
// event — the firewall publishes it itself.
func TestLaunchOperatorRepair_PanicStillPublishesOneEvent(t *testing.T) {
	rec := &launchRecorder{}
	ports := RepairLaunchPorts{
		Repair: func(slug, reason string) (RepairOutcome, error) {
			panic("boom")
		},
		Publish: rec.publish,
	}
	LaunchOperatorRepair(ports, core.Profile{Slug: "ops"}, "corr-1", "why", IncidentLineage{})
	if len(rec.payloads) != 1 {
		t.Errorf("published %d events, want exactly the failed one", len(rec.payloads))
	}
}
