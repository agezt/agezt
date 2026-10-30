// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/modules/work/workboard"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// withWorkboard opens the task board the composition root would open (W3.47)
// and hands the kernel the contract's host port. This test is such a host:
// the board's file is the daemon's business, and the kernel holds the port.
func withWorkboard(t *testing.T, cfg runtime.Config) runtime.Config {
	t.Helper()
	store, err := workboard.OpenStore(filepath.Join(cfg.BaseDir, "workboard"))
	if err != nil {
		t.Fatalf("open the workboard store: %v", err)
	}
	cfg.Workboard = store
	return cfg
}

// TestTheWorkboardSpecWiresTheStorePort: the workboard tool is a plugin, so
// it may not name the module that keeps the task store — it asks for the
// CONTRACT's port and the daemon hands the live store over. The wiring has to
// be real: a task created through the kernel must show up in the tool's own
// op=list, or the agent would work on a board it cannot see.
func TestTheWorkboardSpecWiresTheStorePort(t *testing.T) {
	k, err := runtime.Open(withWorkboard(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	k.SetWorkboard(workboard.NewService(k.Workboard(), k.WorkboardDependencies()))
	t.Cleanup(func() { k.Close() })

	if k.Workboard() == nil {
		t.Fatal("the kernel must own a task store")
	}

	spec := specWorkboard()
	built, err := spec.Build(toolregapi.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{WorkboardRunner: k, Workboard: k.Workboard()}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if _, _, err := k.CreateWorkboardTask("corr", workboardapi.CreateSpec{Title: "visible through the port"}); err != nil {
		t.Fatalf("CreateWorkboardTask: %v", err)
	}

	// The read op must see the very task the kernel just stored — the port
	// really carried the live implementation, not a copy of it.
	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke list: %v", err)
	}
	if res.IsError {
		t.Fatalf("the store port was not handed over: %s", res.Output)
	}
	if !strings.Contains(res.Output, "visible through the port") {
		t.Fatalf("the tool did not list the stored task: %s", res.Output)
	}
}

// TestTheWorkboardSpecSurvivesAMissingStorePort: a daemon that handed over
// only the runner must leave the read op reporting that the board is
// unavailable — not dereferencing nil.
func TestTheWorkboardSpecSurvivesAMissingStorePort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	spec := specWorkboard()
	built, err := spec.Build(toolregapi.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolregapi.KernelDeps{WorkboardRunner: k}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Fatalf("an unwired store port must report that, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "not available") {
		t.Fatalf("the unwired tool must say it is not available: %s", res.Output)
	}
}
