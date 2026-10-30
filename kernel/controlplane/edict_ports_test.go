// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"testing"

	core "github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the policy family's ports — the APPLICATION fills
// them from the kernel handle.
//
// The test pins that binding end to end: the daemon's own policy answers for
// the daemon's kernel, and the request's policy follows the ROUTED kernel — a
// tenant's policy is the tenant's policy. A policy that answered for the
// wrong kernel would let a tenant read the daemon's policy, and would decide
// the daemon's capabilities by the tenant's rules.
//
// The policy engine is PLATFORM: every kernel opens one, so the two engines
// here differ by their CONTENT (a level, an ask-policy), not by their
// existence.
func TestTheAdapterBindsTheEdictPortsToItsOwnKernel(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	tenant, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tenant.Close()

	root.Edict().SetLevel(core.Capability("shared.capability"), core.LevelDeny)
	tenant.Edict().SetLevel(core.Capability("shared.capability"), core.LevelAllow)
	root.Edict().SetAskPolicy(core.AskDeny)
	tenant.Edict().SetAskPolicy(core.AskPrompt)

	s := NewServer(root, t.TempDir())
	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(tenant)})

	// The daemon's own ports answer for its own kernel.
	if s.edictPorts().Engine() == nil {
		t.Fatal("the daemon's policy ports hold no engine — the kernel holds a policy engine")
	}
	daemonOutcome := s.edictPorts().DecideWithCeiling(core.Capability("shared.capability"), "", core.LevelAllow)
	if daemonOutcome.Decision != core.DecisionDeny {
		t.Fatalf("the daemon's policy ports decide %q, the daemon's engine denies", daemonOutcome.Decision)
	}
	if got := s.edictPorts().AskPolicy(); got != core.AskDeny.String() {
		t.Fatalf("the daemon's policy ports answer ask-policy %q, the daemon's engine answers %q", got, core.AskDeny.String())
	}

	// The request's ports follow the routed kernel.
	if requestEdictPorts(ctx).Engine() == nil {
		t.Fatal("the request's policy ports hold no engine — the routed kernel holds a policy engine")
	}
	tenantOutcome := requestEdictPorts(ctx).DecideWithCeiling(core.Capability("shared.capability"), "", core.LevelAllow)
	if tenantOutcome.Decision != core.DecisionAllow {
		t.Fatalf("the request's policy ports decide %q, the routed engine allows", tenantOutcome.Decision)
	}
	if got := requestEdictPorts(ctx).AskPolicy(); got != core.AskPrompt.String() {
		t.Fatalf("the request's policy ports answer ask-policy %q, the routed engine answers %q", got, core.AskPrompt.String())
	}
}
