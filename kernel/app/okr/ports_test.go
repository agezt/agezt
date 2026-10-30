// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/okrapi"
	okrstore "github.com/agezt/agezt/kernel/modules/work/okr"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The objective application's ports are filled by the APPLICATION from the
// kernel handle — the host never binds them itself.
//
// The objective library is the DAEMON's file (W3.53), so this test is such a
// host: it opens the library and hands the kernel the contract's port.
func TestTheObjectivePortsReachTheKernelObjectives(t *testing.T) {
	store, err := okrstore.OpenStore(filepath.Join(t.TempDir(), "okr"))
	if err != nil {
		t.Fatalf("okr.OpenStore: %v", err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), OKRStore: store})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()
	// The objective verbs live in the WORK MODULE (W3.69): the daemon holds
	// the module's face and forwards to it. This test is such a host — it
	// builds the module's service over the store it opened.
	k.SetObjectiveLibrary(okrstore.NewService(k.OKR(), k.ObjectiveLibraryDependencies()))
	owned, err := k.OKR().Create(okrstore.CreateSpec{Title: "owned"}, 100)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ports := PortsFromKernel(k)

	// The ports are FILLED: a port that is nil is not a narrow view, it is no
	// view at all, and an application that cannot read the objectives cannot
	// do its job at all.
	if ports.Reader == nil {
		t.Fatal("the reader port is nil — the application cannot read the objectives")
	}
	if ports.Progress == nil {
		t.Fatal("the progress port is nil — the application cannot roll an objective up")
	}
	if ports.Mutation == nil {
		t.Fatal("the mutation port is nil — the application cannot write an objective")
	}

	// The reader port sees the kernel's objectives.
	list, err := New(ports.Reader, ports.Progress).List(context.Background(), ListInput{})
	if err != nil || list.Count != 1 || list.Objectives[0].Title != "owned" {
		t.Fatalf("the objective list = %+v %v, want the kernel's objective", list, err)
	}

	// The mutation port writes THROUGH the kernel.
	created, err := ports.Mutation.CreateObjective("", okrapi.CreateSpec{Title: "written"})
	if err != nil {
		t.Fatalf("CreateObjective: %v", err)
	}
	if created.Title != "written" {
		t.Fatalf("CreateObjective = %+v, want the written objective", created)
	}
	if found, ok := ports.Reader.Get(created.ID); !ok || found.Title != "written" {
		t.Fatalf("the reader port reads %+v %v, want the objective the mutation port wrote", found, ok)
	}

	// And the library the kernel holds is the one the port reads.
	if _, ok := k.OKR().Get(owned.ID); !ok {
		t.Error("the kernel's library does not hold the objective it created")
	}
}

// A daemon with NO objective library holds a nil reader port: the
// application answers "no objectives" rather than calling into nothing.
func TestTheObjectivePortsSurviveADaemonWithNoObjectiveLibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.Reader != nil {
		t.Error("the reader port is not nil — the daemon holds no objective library")
	}

	service := New(ports.Reader, ports.Progress)
	list, err := service.List(context.Background(), ListInput{})
	if err != nil || list.Count != 0 {
		t.Errorf("the objective list = %+v %v, want none from a daemon with no library", list, err)
	}
	// A named objective cannot be answered from an empty library: "unknown
	// objective" would blame the id for a library that is not there.
	if _, err := service.Show(context.Background(), ShowInput{ID: "anything"}); err == nil {
		t.Error("the application answered a named objective from an empty library")
	}
}
