// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheWorkboardSpecWiresTheStorePort: the workboard tool is a plugin, so
// it may not name the module that keeps the task store — it asks for the
// CONTRACT's port and the daemon hands the live store over. The wiring has to
// be real: a task created through the kernel must show up in the tool's own
// op=list, or the agent would work on a board it cannot see.
func TestTheWorkboardSpecWiresTheStorePort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
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
