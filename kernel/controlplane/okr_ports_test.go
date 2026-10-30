// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/okrapi"
	"github.com/agezt/agezt/kernel/modules/work/okr"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the objective application's ports — the APPLICATION
// fills them from the kernel handle.
//
// The test pins that binding end to end: the daemon's objective list answers
// from the kernel's library, and the request's ports follow the ROUTED
// kernel — a tenant's objectives are the tenant's objectives.
func TestTheAdapterBindsTheObjectivePortsToItsOwnKernel(t *testing.T) {
	dir := t.TempDir()
	store, err := okr.OpenStore(filepath.Join(dir, "okr"))
	if err != nil {
		t.Fatalf("okr.OpenStore: %v", err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), OKRStore: store})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()
	if _, err := k.OKR().Create(okr.CreateSpec{Title: "owned"}, 100); err != nil {
		t.Fatalf("Create: %v", err)
	}

	s := NewServer(k, dir)
	s.token = "primary"

	// The daemon's objective list — the adapter's own wiring — shows the
	// kernel's objectives.
	response := callAppHost(t, s, Request{ID: "list", Cmd: CmdOKRList, Token: "primary"})[0]
	if response.Type != RespResult {
		t.Fatalf("the objective list: %s", response.Error)
	}
	if response.Result["count"] != float64(1) {
		t.Errorf("the objective list counts %v, want the kernel's one objective", response.Result["count"])
	}

	// The request's ports follow the routed kernel.
	other, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), OKRStore: mustOpenOKR(t)})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer other.Close()
	routed, err := other.OKR().Create(okr.CreateSpec{Title: "routed"}, 100)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(other)})
	objectives := requestOKRPorts(ctx).Reader.List(okrapi.Filter{})
	if len(objectives) != 1 || objectives[0].ID != routed.ID {
		t.Fatalf("the request's objective ports read %+v, want the routed kernel's objective", objectives)
	}
}

// mustOpenOKR opens an objective library for a host.
func mustOpenOKR(t *testing.T) *okr.Store {
	t.Helper()
	store, err := okr.OpenStore(filepath.Join(t.TempDir(), "okr"))
	if err != nil {
		t.Fatalf("okr.OpenStore: %v", err)
	}
	return store
}
