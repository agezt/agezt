// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/standingtool"
)

// The kernel IS the standing tool's host: the tool speaks the contract's rows
// and the daemon hands it the live kernel. A compile-time assertion — if the
// kernel stopped answering the port, the wiring site breaks instead of the
// tool.
var _ standingtool.Host = (*runtime.Kernel)(nil)

// The kernel's cadence store IS the contract's store port: the schedule tool
// drives it through the contract, never through the module.
var _ cadenceapi.Store = (*cadence.Store)(nil)

// TestStandingSpecWiresTheRosterLookup: the standing tool may not name the
// roster module, so the daemon hands it the profile lookup — and the wiring
// has to be real. A managed sub-agent must still be refused, with its
// manager named, when the daemon configures the spec against a live kernel.
func TestStandingSpecWiresTheRosterLookup(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	no := false
	if _, err := k.AddProfile(agentapi.Profile{Slug: "worker", ParentAgent: "lead", DirectCallable: &no}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	spec := specStanding()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{K: k}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(toolapi.WithAgent(context.Background(), "worker"), json.RawMessage(
		`{"op":"create_event","name":"self-wake","subject":"task.failed","plan":"wake me"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Fatalf("a managed sub-agent must not create a direct standing wake, got %s", res.Output)
	}
	if !strings.Contains(res.Output, "managed sub-agent") || !strings.Contains(res.Output, "wake lead") {
		t.Fatalf("the refusal must name the manager, got %s", res.Output)
	}
}

// TestScheduleSpecBindsTheLiveStore: the schedule tool may not name the
// cadence module, so it takes the store as the contract's port — and the
// daemon binds the live one. An entry the tool creates must land in the
// kernel's own schedule store, not in a store of its own.
func TestScheduleSpecBindsTheLiveStore(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	spec := specSchedule()
	built, err := spec.Build(toolreg.BuildDeps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := spec.Configure(built.Tool, toolreg.KernelDeps{K: k}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	res, err := built.Tool.Invoke(context.Background(), json.RawMessage(
		`{"op":"in","delay":"30m","intent":"do the thing"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the schedule tool errored: %s", res.Output)
	}
	if got := k.Schedules().Count(); got != 1 {
		t.Fatalf("entries in the live cadence store = %d, want 1", got)
	}
}
