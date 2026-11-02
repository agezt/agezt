// SPDX-License-Identifier: MIT

package runtime_test

// The daemon's objectives: what it KEEPS and what it gave away.
//
// The daemon used to hold the library and spell out the objective lifecycle
// itself. Then it held the WORK MODULE's face and asked it. Now (W3.76r) it
// holds neither the lifecycle nor a forwarder over it: the application binds
// the MODULE directly, and the daemon keeps two things — the library's FILE
// (the host's business, W3.53) and the module's SERVICE, which the
// composition root's scope hands to the application.
//
// These tests pin that: the daemon's verbs are GONE (the ratchet in
// dead_surface_ratchet_test.go and the one in this package watch the names),
// and what the daemon still holds is the file and the module's face.

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/okrapi"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
	"github.com/agezt/agezt/kernel/modules/work/okr"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func openObjectiveDaemon(t *testing.T, withStore bool) *runtime.Kernel {
	t.Helper()
	return openObjectiveDaemonWithBuilder(t, withStore, nil)
}

// openObjectiveDaemonWithBuilder opens a daemon, optionally with a library
// store and optionally with the module's BUILDER in the config.
//
// The builder is the daemon's way of naming the module: the kernel calls it
// with the store the host opened and its own ports.
func openObjectiveDaemonWithBuilder(t *testing.T, withStore bool, builder workapi.ObjectiveServiceBuilder) *runtime.Kernel {
	t.Helper()
	cfg := runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), ObjectiveLibrary: builder}
	if withStore {
		store, err := okr.OpenStore(filepath.Join(cfg.BaseDir, "okr"))
		if err != nil {
			t.Fatalf("open the okr store: %v", err)
		}
		cfg.OKRStore = store
	}
	k, err := runtime.Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// The daemon holds the MODULE's face.
//
// The BUILDER in the config is the composition root's way of naming the
// module; the daemon calls it over the store it opened and hands the result
// over as the module's service. The application binds THAT (W3.76r).
func TestTheDaemonHoldsTheModulesObjectiveService(t *testing.T) {
	k := openObjectiveDaemonWithBuilder(t, true, okr.NewService)

	library := k.Objectives()
	if library == nil {
		t.Fatal("the daemon holds no objective service — the builder was never called")
	}
	goal, err := library.Create("corr", okrapi.CreateSpec{Title: "owned"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if goal.Title != "owned" {
		t.Fatalf("the service answered %+v, want the goal it was given", goal)
	}
}

// Without the builder, the daemon holds NO service.
//
// The daemon does not answer the module's verbs itself any more, so there is
// nothing to answer with — and the application says so with its own refusal
// rather than calling into nothing.
func TestTheDaemonHoldsNoObjectiveServiceWithoutTheBuilder(t *testing.T) {
	k := openObjectiveDaemon(t, true)

	if got := k.Objectives(); got != nil {
		t.Fatalf("a kernel with no objective builder must yield a nil service, got %v", got)
	}
}

// The daemon still holds the HOST's library file.
//
// The store file is the host's business (W3.53): the daemon opens it and hands
// the port to the module. The module's service is the lifecycle, and the port
// is still the daemon's to answer for.
func TestTheDaemonStillHoldsTheHostsLibraryFile(t *testing.T) {
	k := openObjectiveDaemonWithBuilder(t, true, okr.NewService)
	if k.OKR() == nil {
		t.Fatal("the daemon must hold the library it was given")
	}
	if k.Objectives() == nil {
		t.Fatal("wiring the module must not take the library file away from the daemon")
	}
}
