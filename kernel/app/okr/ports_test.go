// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/okrapi"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
	okrstore "github.com/agezt/agezt/kernel/modules/work/okr"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The objective application's ports are filled by the APPLICATION from the
// MODULE's service and the contract's store port (W3.76r) — the host never
// binds them itself, and the daemon's six forwarders over the module's own
// operations are gone.
//
// The objective library's FILE is the DAEMON's (W3.53), so the second test is
// such a host: it opens the library and hands the daemon the contract's port.
func TestTheObjectivePortsReachTheLibrary(t *testing.T) {
	store, err := okrstore.OpenStore(filepath.Join(t.TempDir(), "okr"))
	if err != nil {
		t.Fatalf("okr.OpenStore: %v", err)
	}
	// The objective verbs live in the WORK MODULE: the module keeps the goals,
	// the key results and the rollup rule. This test is such a host — it
	// builds the module's service over the store it opened.
	library := okrstore.NewService(store, workapi.ObjectiveDependencies{})
	owned, err := store.Create(okrstore.CreateSpec{Title: "owned"}, 100)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ports := PortsFromLibrary(library, store)

	// The ports are FILLED: a port that is nil is not a narrow view, it is no
	// view at all, and an application that cannot read the objectives cannot
	// do its job at all.
	if ports.Reader == nil {
		t.Fatal("the reader port is nil — the application cannot read the objectives")
	}
	if ports.Library == nil {
		t.Fatal("the library port is nil — the application cannot roll an objective up")
	}

	// The reader port sees the library's objectives.
	list, err := New(ports.Reader, ports.Library).List(context.Background(), ListInput{})
	if err != nil || list.Count != 1 || list.Objectives[0].Title != "owned" {
		t.Fatalf("the objective list = %+v %v, want the library's objective", list, err)
	}

	// The library port writes THROUGH the module.
	created, err := ports.Library.Create("", okrapi.CreateSpec{Title: "written"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Title != "written" {
		t.Fatalf("Create = %+v, want the written objective", created)
	}
	if found, ok := ports.Reader.Get(created.ID); !ok || found.Title != "written" {
		t.Fatalf("the reader port reads %+v %v, want the objective the library port wrote", found, ok)
	}

	// And the store the reader reads is the one the library wrote.
	if _, ok := store.Get(owned.ID); !ok {
		t.Error("the store does not hold the objective it created")
	}
}

// A daemon with NO objective library holds a nil reader port and a nil library
// port: the application answers "no objectives" rather than calling into
// nothing.
func TestTheObjectivePortsSurviveADaemonWithNoObjectiveLibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromLibrary(k.Objectives(), k.OKR())
	if ports.Reader != nil {
		t.Error("the reader port is not nil — the daemon holds no objective library")
	}
	if ports.Library != nil {
		t.Error("the library port is not nil — the daemon was given no objective builder")
	}

	service := New(ports.Reader, ports.Library)
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
