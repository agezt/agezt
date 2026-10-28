// SPDX-License-Identifier: MIT

package edict

import (
	"testing"

	core "github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The policy family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins what that wiring buys: the levels the port reads are the
// levels the kernel's engine holds, and the level the port writes is the level
// the kernel's engine keeps. A port bound to another engine would leave the
// operator reading a policy the daemon does not enforce.
func TestTheEdictPortsReachTheKernelEngine(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.Engine() == nil {
		t.Fatal("the policy port holds no engine — the kernel holds a policy engine")
	}
	if ports.WriteEngine() == nil {
		t.Fatal("the policy port holds no write engine — the kernel holds a policy engine")
	}

	// The write view changes the engine the read view reads.
	ports.WriteEngine().SetLevel(core.Capability("port-capability"), core.LevelAllow)
	level, ok := ports.Engine().Levels()[core.Capability("port-capability")]
	if !ok {
		t.Fatal("the policy port wrote a level the engine does not hold")
	}
	if level != core.LevelAllow {
		t.Fatalf("the policy port wrote level %s, the engine holds %s", level, level)
	}

	// And the ask-policy answer is the engine's own.
	ports.WriteEngine().SetAskPolicy(core.AskPrompt)
	if got := ports.AskPolicy(); got != core.AskPrompt.String() {
		t.Fatalf("the policy port answers ask-policy %q, the engine answers %q", got, core.AskPrompt.String())
	}
}

// A host with no engine must not decide anything.
//
// The adapter used to spell "there is no policy engine" itself: it reached for
// the kernel's engine and asked it. The application spells it once, and the
// refusal is a DENY: a policy that cannot be asked must not let a capability
// through.
func TestTheEdictPortsRefuseOnAHostWithNoEngine(t *testing.T) {
	ports := PortsFromKernel(nil)
	if ports.Engine() != nil {
		t.Fatal("a host with no kernel hands out a policy engine")
	}
	if ports.WriteEngine() != nil {
		t.Fatal("a host with no kernel hands out a policy write engine")
	}
	outcome := ports.DecideWithCeiling(core.Capability("file.write"), "", core.LevelAllow)
	if outcome.Decision != core.DecisionDeny {
		t.Fatalf("a host with no policy engine decides %q, want a refusal", outcome.Decision)
	}
	if outcome.Capability != core.Capability("file.write") {
		t.Fatalf("the refusal names capability %q, want the capability that was asked", outcome.Capability)
	}
	if outcome.Reason == "" {
		t.Fatal("the refusal carries no reason — an operator must be able to read it")
	}
	if got := ports.AskPolicy(); got != "" {
		t.Fatalf("a host with no policy engine answers ask-policy %q, want the empty answer", got)
	}
}
