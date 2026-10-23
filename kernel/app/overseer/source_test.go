// SPDX-License-Identifier: MIT

package overseer

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/overseerapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The app-layer implementation must answer the contract port. A compile-time
// assertion: if the implementation's shape drifts from the port, the wiring
// site breaks instead of the tool.
var _ overseerapi.Source = (*kernelSource)(nil)

// TestKernelSourceAnswersThePortFromTheLiveKernel drives the moved
// implementation against a REAL kernel. The tool's own suite proves the
// op → port mapping with a fake; nothing proved that the implementation
// behind the port still reads and steers the live kernel — that is what this
// covers, and it is the behaviour the move had to preserve.
func TestKernelSourceAnswersThePortFromTheLiveKernel(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	src := NewKernelSource(k, dir)
	if src == nil {
		t.Fatal("NewKernelSource returned nil")
	}

	// The roster is empty until a profile is added.
	if got := src.Agents(); len(got) != 0 {
		t.Fatalf("agents on an empty roster = %d, want 0", len(got))
	}
	if _, err := k.AddProfile(agentapi.Profile{Slug: "scout", Name: "Scout", Enabled: true}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	// Read: the profile is visible through the port.
	got, ok, err := src.GetAgent("scout")
	if err != nil || !ok || got.Slug != "scout" {
		t.Fatalf("GetAgent = %+v ok=%v err=%v", got, ok, err)
	}
	if all := src.Agents(); len(all) != 1 || all[0].Slug != "scout" {
		t.Fatalf("Agents = %+v, want one scout", all)
	}
	if found := src.SearchAgents(overseerapi.SearchFilter{Query: "scout"}); len(found) != 1 {
		t.Fatalf("SearchAgents = %+v, want one scout", found)
	}

	// An unknown agent is refused, not invented.
	if _, found, err := src.GetAgent("ghost"); err != nil || found {
		t.Fatalf("GetAgent(ghost) found=%v err=%v, want not found", found, err)
	}
	if _, err := src.SetAgentEnabled("ghost", true); err == nil {
		t.Fatal("SetAgentEnabled on an unknown agent must be refused")
	}

	// Intervene: the pause reaches the live roster.
	paused, err := src.SetAgentEnabled("scout", false)
	if err != nil || paused.Enabled {
		t.Fatalf("SetAgentEnabled = %+v err=%v, want a paused profile", paused, err)
	}
	if cur, _, _ := src.GetAgent("scout"); cur.Enabled {
		t.Fatal("the pause did not reach the live roster")
	}

	// The daemon-wide halt and resume travel through the kernel.
	if src.IsHalted() {
		t.Fatal("a fresh kernel must not be halted")
	}
	src.Halt("test")
	if !src.IsHalted() {
		t.Fatal("Halt did not reach the kernel")
	}
	src.ResumeAll("test")
	if src.IsHalted() {
		t.Fatal("ResumeAll did not reach the kernel")
	}

	// The board is opened lazily; a daemon without one answers with an empty
	// list rather than an error (the overseer still reports on everything else).
	if open := src.OpenHelp(5); len(open) != 0 {
		t.Fatalf("OpenHelp on a boardless daemon = %+v, want empty", open)
	}
}

// TestKernelSourceProtectsSystemGuardians: the guards that make the agent-
// reachable fleet surface safe live in the implementation behind the port —
// they were never covered, because the tool's own suite drives a fake.
func TestKernelSourceProtectsSystemGuardians(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	src := NewKernelSource(k, dir)

	if _, err := k.AddProfile(agentapi.Profile{Slug: "guardian", System: true}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	// A protected guardian cannot be retuned by an agent.
	if _, err := src.EditAgent("guardian", agentapi.Profile{Name: "hijacked"}); err == nil ||
		!strings.Contains(err.Error(), "protected system guardian") {
		t.Fatalf("EditAgent on a guardian = %v, want a protection refusal", err)
	}
	cur, _, _ := src.GetAgent("guardian")
	if cur.Name == "hijacked" {
		t.Fatal("the guardian's name changed — the protection did not hold")
	}
	// Nor can it be hard-deleted.
	if ok, err := src.DeleteAgent("guardian"); err == nil && ok {
		t.Fatal("a protected guardian must not be deletable through the overseer")
	}

	// A created agent is never a guardian: the System flag is forced off.
	created, err := src.CreateAgent(agentapi.Profile{Slug: "spawned", System: true})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if created.System {
		t.Fatal("CreateAgent must force System off — only boot seeding may mint a guardian")
	}
}

// TestKernelSourceEditAppliesOnlyProvidedFields: the PATCH semantics of the
// agent-facing edit — a partial payload must not blank the fields it did not
// mention.
func TestKernelSourceEditAppliesOnlyProvidedFields(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	src := NewKernelSource(k, dir)

	if _, err := k.AddProfile(agentapi.Profile{Slug: "scout", Name: "Scout", Soul: "stay sharp"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	edited, err := src.EditAgent("scout", agentapi.Profile{Name: "Renamed"})
	if err != nil {
		t.Fatalf("EditAgent: %v", err)
	}
	if edited.Name != "Renamed" {
		t.Fatalf("name = %q, want Renamed", edited.Name)
	}
	if edited.Soul != "stay sharp" {
		t.Fatalf("soul = %q — an edit must not blank the fields it did not mention", edited.Soul)
	}
	if edited.Slug != "scout" {
		t.Fatalf("slug = %q — an edit must not rename the agent", edited.Slug)
	}
}
