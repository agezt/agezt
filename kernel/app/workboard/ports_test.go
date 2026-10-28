// SPDX-License-Identifier: MIT

package workboard

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/modules/work/seat"
	workboardstore "github.com/agezt/agezt/kernel/modules/work/workboard"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The workboard application's ports are filled by the APPLICATION from the
// kernel handle — the host never binds them itself.
//
// The task board is the composition root's file (W3.47), so this test is such
// a host: it opens the store and hands the kernel the contract's port.
func TestTheWorkboardPortsReachTheKernelBoard(t *testing.T) {
	board, err := workboardstore.OpenStore(filepath.Join(t.TempDir(), "workboard"))
	if err != nil {
		t.Fatalf("workboard.OpenStore: %v", err)
	}
	seats, err := seat.OpenStore(filepath.Join(t.TempDir(), "seats"))
	if err != nil {
		t.Fatalf("seat.OpenStore: %v", err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Workboard: board, Seats: seats})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()
	if _, _, err := k.Workboard().Create(workboardstore.CreateSpec{Title: "owned", Status: workboardstore.StatusReady}, time.UnixMilli(100)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ports := PortsFromKernel(k)

	// The ports are FILLED: a port that is nil is not a narrow view, it is no
	// view at all, and an application that cannot read the board cannot do
	// its job at all.
	if ports.Reader == nil {
		t.Fatal("the reader port is nil — the application cannot read the board")
	}
	if ports.Store == nil {
		t.Fatal("the store port is nil — the application cannot watch the board")
	}
	if ports.SeatSetter == nil {
		t.Fatal("the seat setter port is nil — the application cannot seat a task")
	}
	if ports.Relations == nil {
		t.Fatal("the relations port is nil — the application cannot link a task")
	}
	if ports.Lifecycle == nil {
		t.Fatal("the lifecycle port is nil — the application cannot run a task's life")
	}
	if ports.Dispatch == nil {
		t.Fatal("the dispatch port is nil — the application cannot claim a task")
	}
	if ports.Execution == nil {
		t.Fatal("the execution port is nil — the application cannot execute a task")
	}
	if ports.SeatIndex == nil {
		t.Fatal("the seat index port is nil — the application cannot read the seats")
	}
	if ports.SeatStore == nil {
		t.Fatal("the seat store port is nil — the application cannot validate a seat")
	}
	if ports.TaskIndex == nil {
		t.Fatal("the task index port is nil — the application cannot read the tasks")
	}
	if ports.Events == nil {
		t.Fatal("the events port is nil — the application cannot read the journal")
	}

	// The reader port sees the kernel's tasks.
	list, err := New(ports.Reader).List(context.Background(), ListInput{})
	if err != nil || list.Count != 1 || list.Tasks[0].Title != "owned" {
		t.Fatalf("the task list = %+v %v, want the kernel's task", list, err)
	}

	// The lifecycle port writes THROUGH the kernel.
	created, err := NewLifecycle(ports.Lifecycle, ports.SeatSetter).Create(context.Background(), CreateInput{Spec: workboardstore.CreateSpec{Title: "written", Status: workboardstore.StatusReady}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Task.Title != "written" {
		t.Fatalf("Create = %+v, want the written task", created)
	}
	if _, ok := ports.TaskIndex.Get(created.Task.ID); !ok {
		t.Fatalf("the task index does not hold the task the lifecycle port wrote: %+v", created.Task)
	}
}

// A daemon with NO board holds a nil reader port: the application answers
// "the task board is not available on this daemon" rather than calling into
// nothing.
func TestTheWorkboardPortsSurviveADaemonWithNoBoard(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.Reader != nil {
		t.Error("the reader port is not nil — the daemon holds no board")
	}
	if ports.Store != nil {
		t.Error("the store port is not nil — the daemon holds no board")
	}

	if _, err := New(ports.Reader).List(context.Background(), ListInput{}); err == nil {
		t.Error("the application listed a daemon with no board, want the plain refusal")
	}
}
