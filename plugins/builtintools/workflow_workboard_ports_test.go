// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/toolreg"
	workboardtool "github.com/agezt/agezt/plugins/tools/workboardtool"
	workflowtool "github.com/agezt/agezt/plugins/tools/workflowtool"
)

// fakeWorkflowRunner is the daemon's workflow library, faked: it answers the
// three verbs of the contract's port and RECORDS that it was called.
//
// The recording is the point: "the tool did not complain" is not enough — a
// tool could report unavailability for another reason and still look wired.
type fakeWorkflowRunner struct {
	saved   bool
	enabled bool
	ran     bool
}

func (f *fakeWorkflowRunner) SaveWorkflow(_ string, w workflowapi.Workflow) (workflowapi.Workflow, bool, error) {
	f.saved = true
	w.Name = "daily"
	return w, true, nil
}

func (f *fakeWorkflowRunner) SetWorkflowEnabled(_ string, _ string, enabled bool) (workflowapi.Workflow, error) {
	f.enabled = enabled
	return workflowapi.Workflow{Name: "daily", Enabled: enabled}, nil
}

func (f *fakeWorkflowRunner) RunWorkflow(_ context.Context, _, _ string, _ any) (workflowapi.RunResult, error) {
	f.ran = true
	return workflowapi.RunResult{}, nil
}

// fakeWorkboardRunner is the daemon's workboard, faked: it answers the
// thirteen verbs of the contract's port and records the calls.
type fakeWorkboardRunner struct {
	claimed bool
}

func (f *fakeWorkboardRunner) CreateWorkboardTask(_ string, _ workboardapi.CreateSpec) (workboardapi.Task, bool, error) {
	return workboardapi.Task{ID: "t1"}, true, nil
}

func (f *fakeWorkboardRunner) ClaimWorkboardTask(_, _, _, _ string) (workboardapi.Task, error) {
	f.claimed = true
	return workboardapi.Task{ID: "t1"}, nil
}

func (f *fakeWorkboardRunner) HeartbeatWorkboardTask(_, _, _, _ string) (workboardapi.Task, error) {
	return workboardapi.Task{ID: "t1"}, nil
}

func (f *fakeWorkboardRunner) CommentWorkboardTask(_, _, _, _ string) (workboardapi.Task, error) {
	return workboardapi.Task{ID: "t1"}, nil
}

func (f *fakeWorkboardRunner) BlockWorkboardTask(_, _, _, _ string) (workboardapi.Task, error) {
	return workboardapi.Task{ID: "t1"}, nil
}

func (f *fakeWorkboardRunner) FailWorkboardTask(_, _, _, _ string) (workboardapi.Task, workboardapi.RetryDecision, error) {
	return workboardapi.Task{ID: "t1"}, workboardapi.RetryDecision{}, nil
}

func (f *fakeWorkboardRunner) UnblockWorkboardTask(_, _, _ string) (workboardapi.Task, error) {
	return workboardapi.Task{ID: "t1"}, nil
}

func (f *fakeWorkboardRunner) CompleteWorkboardTask(_, _, _ string) (workboardapi.Task, error) {
	return workboardapi.Task{ID: "t1"}, nil
}

func (f *fakeWorkboardRunner) ArchiveWorkboardTask(_, _, _ string) (workboardapi.Task, error) {
	return workboardapi.Task{ID: "t1"}, nil
}

func (f *fakeWorkboardRunner) LinkWorkboardTask(_, _, _, _ string) (workboardapi.Task, error) {
	return workboardapi.Task{ID: "t1"}, nil
}

func (f *fakeWorkboardRunner) SetWorkboardRetryPolicy(_, _, _ string, _ *workboardapi.RetryPolicy) (workboardapi.Task, error) {
	return workboardapi.Task{ID: "t1"}, nil
}

func (f *fakeWorkboardRunner) AddWorkboardDependency(_, _, _ string) (workboardapi.Task, error) {
	return workboardapi.Task{ID: "t1"}, nil
}

func (f *fakeWorkboardRunner) ReclaimStaleWorkboardTask(_, _, _ string, _ time.Duration) (workboardapi.Task, error) {
	return workboardapi.Task{ID: "t1"}, nil
}

// fakeStore is the read side of the workboard: the tool needs it too, and
// the port is a separate one on purpose.
type fakeStore struct{}

func (fakeStore) Get(string) (workboardapi.Task, bool)         { return workboardapi.Task{}, false }
func (fakeStore) List(workboardapi.Filter) []workboardapi.Task { return nil }

// TestTheWorkflowSpecBindsTheRunnerPort: the workflow tool drives the
// daemon's workflow library through the contract's port, and the registry's
// row carries that port (W3.39c) — not the live kernel.
func TestTheWorkflowSpecBindsTheRunnerPort(t *testing.T) {
	fake := &fakeWorkflowRunner{}
	spec := specWorkflow()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{WorkflowRunner: fake}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(
		`{"op":"save","workflow":{"name":"daily"}}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the workflow tool errored on a wired runner port: %s", res.Output)
	}
	if !fake.saved {
		t.Fatal("the workflow tool must reach the daemon's workflow library")
	}
}

// TestTheWorkboardSpecBindsTheRunnerPort: the same for the workboard — the
// tool drives the durable, journaled path through the contract's port.
func TestTheWorkboardSpecBindsTheRunnerPort(t *testing.T) {
	fake := &fakeWorkboardRunner{}
	spec := specWorkboard()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{
		WorkboardRunner: fake,
		Workboard:       fakeStore{},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(
		`{"op":"claim","id":"t1","agent":"scout","run_id":"r1"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the workboard tool errored on a wired runner port: %s", res.Output)
	}
	if !fake.claimed {
		t.Fatal("the workboard tool must reach the daemon's workboard")
	}
}

// TestTheRunnerlessToolsReportUnavailable: without the ports the two tools
// must say they are unavailable — they must not call into nothing.
//
// The nil port is the trap the daemon's rule exists for: a port that wraps a
// nil implementation is NOT nil, so the tool would bind a runner that is not
// there and fail in a way the operator could not read.
func TestTheRunnerlessToolsReportUnavailable(t *testing.T) {
	spec := specWorkflow()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"save","workflow":{"name":"daily"}}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("an unwired workflow library must report that it is unavailable, got %+v", res)
	}

	spec = specWorkboard()
	built, err = spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	res, err = built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"claim","id":"t1"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("an unwired workboard must report that it is unavailable, got %+v", res)
	}
}

// TestTheToolsRunnersAreTheContractsPorts: the two tools' runner types ARE
// the contract's ports — the same type, not a copy with the same method set.
//
// The check is on the TYPE ITSELF, not on an assignment: an assignment would
// accept a copy (a copy has the same method set, so it satisfies the port),
// while two interface types with identical method sets are still different
// types. A tool that declared its own runner would keep working — and the
// contract's port would quietly stop being the thing the registry hands over.
func TestTheWorkflowAndWorkboardRunnersAreTheContractsPorts(t *testing.T) {
	for _, tc := range []struct {
		name string
		tool reflect.Type
		want reflect.Type
	}{
		{"workflow", reflect.TypeOf((*workflowtool.Kernel)(nil)).Elem(), reflect.TypeOf((*workflowapi.Runner)(nil)).Elem()},
		{"workboard", reflect.TypeOf((*workboardtool.Kernel)(nil)).Elem(), reflect.TypeOf((*workboardapi.Runner)(nil)).Elem()},
	} {
		if tc.tool != tc.want {
			t.Errorf("the %s tool's runner must BE the contract's port, got %v want %v", tc.name, tc.tool, tc.want)
		}
	}
}
