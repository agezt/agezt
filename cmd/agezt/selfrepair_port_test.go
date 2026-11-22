// SPDX-License-Identifier: MIT

package main

// Provenance: the daemon's half of the W5.4 move — the self-repair module's
//             ports, filled from the kernel handle.
//
// The module may not hold the kernel (archcheck's principle P5), so the
// composition root hands it the capabilities one by one. These tests are the
// ratchet on the DAEMON's half: a capability the module needs and the daemon
// forgets to hand over is a healer that cannot heal — and it fails silently,
// because every port is optional by design.

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheDaemonFillsEverySelfRepairPort pins the wiring: every capability the
// module needs is handed over, and none of them is a nil door.
//
// The list is the module's (kernel/modules/agents/selfrepair/ports.go). A port
// the daemon forgets is a port the module reads as "the host holds no such
// thing" — and the healer goes quiet instead of saying why.
func TestTheDaemonFillsEverySelfRepairPort(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("openAppKernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	ports := selfrepairPorts(k)

	if ports.Bus == nil {
		t.Fatal("the daemon must hand the module its bus")
	}
	if ports.Journal == nil {
		t.Fatal("the daemon must hand the module its journal walk")
	}
	if ports.LiveProvider == nil {
		t.Fatal("the daemon must hand the module its live provider")
	}
	if ports.Scan == nil {
		t.Fatal("the daemon must hand the module the reaper's scan")
	}
	if ports.Profile == nil {
		t.Fatal("the daemon must hand the module the roster's read")
	}
	if ports.Profiles == nil {
		t.Fatal("the daemon must hand the module the roster's list")
	}
	if ports.Roster == nil {
		t.Fatal("the daemon must hand the module the roster's lifecycle")
	}
	if ports.Runner == nil {
		t.Fatal("the daemon must hand the module the run engine")
	}
	if ports.Correlation == nil {
		t.Fatal("the daemon must hand the module its identity door")
	}
	if ports.BindProfile == nil {
		t.Fatal("the daemon must hand the module its profile door")
	}
	if ports.CapCost == nil {
		t.Fatal("the daemon must hand the module its cost door")
	}
}

// TestTheSelfRepairProfileDoorBindsTheRun pins that the door the daemon hands
// over is the DAEMON'S door and not a copy of it: the run really carries the
// agent's profile, and the run really carries the cost ceiling.
//
// The two doors exist for exactly this module (kernel/runtime's
// accessors_ctx.go: "the doors a MODULE needs without naming the runs module
// — the profile and the ceiling — the self-repair module binds a run through
// them"), and the daemon's own ratchet pins them
// (kernel/runtime/runctx_ratchet_test.go).
func TestTheSelfRepairProfileDoorBindsTheRun(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("openAppKernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	ports := selfrepairPorts(k)

	ctx := ports.BindProfile(context.Background(), agentapi.Profile{
		Slug:            "ops",
		ConfigOverrides: map[string]string{"AGEZT_MODEL": "agent-model"},
	})
	if got := runctx.AgentConfigOverrides(ctx); got["AGEZT_MODEL"] != "agent-model" {
		t.Fatalf("the profile door bound nothing: %+v", got)
	}

	ctx = ports.CapCost(ctx, 321)
	if got := runctx.MaxCostFromCtx(ctx); got != 321 {
		t.Fatalf("the cost door bound nothing: %d", got)
	}
}

// TestTheSelfRepairPortsAnswerForNoKernel pins the empty case: a host that
// holds no kernel holds no healer, and the ports say so instead of calling
// into nothing.
func TestTheSelfRepairPortsAnswerForNoKernel(t *testing.T) {
	ports := selfrepairPorts(nil)
	if ports.Bus != nil || ports.Journal != nil || ports.Scan != nil || ports.Runner != nil {
		t.Fatal("a host with no kernel must hold no self-repair capability")
	}
}
