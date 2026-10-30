// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"github.com/agezt/agezt/kernel/app/binding"
	"path/filepath"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/modules/work/workboard"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the workboard application's ports — the APPLICATION
// fills them from the kernel handle.
//
// The test pins that binding end to end: the daemon's task list answers from
// the kernel's board, and the request's ports follow the ROUTED kernel — a
// tenant's board is the tenant's board.
func TestTheAdapterBindsTheWorkboardPortsToItsOwnKernel(t *testing.T) {
	dir := t.TempDir()
	board, err := workboard.OpenStore(filepath.Join(dir, "workboard"))
	if err != nil {
		t.Fatalf("workboard.OpenStore: %v", err)
	}
	k, err := runtime.Open(withRoster(t, runtime.Config{BaseDir: dir, Provider: mock.New(), Workboard: board}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()
	if _, _, err := k.Workboard().Create(workboard.CreateSpec{Title: "owned", Status: workboard.StatusReady}, time.UnixMilli(100)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	s := NewServer(binding.ScopeFromKernel(k, dir))
	s.token = "primary"

	// The daemon's task list — the adapter's own wiring — shows the kernel's
	// tasks.
	response := callAppHost(t, s, Request{ID: "list", Cmd: CmdWorkboardList, Token: "primary"})[0]
	if response.Type != RespResult {
		t.Fatalf("the task list: %s", response.Error)
	}
	if response.Result["count"] != float64(1) {
		t.Errorf("the task list counts %v, want the kernel's one task", response.Result["count"])
	}

	// The request's ports follow the routed kernel.
	other, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Workboard: mustOpenBoard(t)})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer other.Close()
	routed, _, err := other.Workboard().Create(workboard.CreateSpec{Title: "routed", Status: workboard.StatusReady}, time.UnixMilli(100))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(other)})
	tasks := requestWorkboardPorts(ctx).Reader.List(workboard.Filter{})
	if len(tasks) != 1 || tasks[0].ID != routed.ID {
		t.Fatalf("the request's board ports read %+v, want the routed kernel's task", tasks)
	}
}

// mustOpenBoard opens a task board for a host.
func mustOpenBoard(t *testing.T) *workboard.Store {
	t.Helper()
	store, err := workboard.OpenStore(filepath.Join(t.TempDir(), "workboard"))
	if err != nil {
		t.Fatalf("workboard.OpenStore: %v", err)
	}
	return store
}
