// SPDX-License-Identifier: MIT

package configtool

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/configapi"
	"github.com/agezt/agezt/kernel/modules/providers/catalog"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The adapter answers the contract's port.
var _ configapi.Kernel = (*kernelSource)(nil)

// TestAMissingKernelIsANilPort: a daemon with no kernel hands over a NIL
// port, not a port wrapping nothing. A nil *Kernel behind an interface would
// be a non-nil interface, and the tool would call into a kernel that is not
// there instead of reporting that the daemon is not wired.
func TestAMissingKernelIsANilPort(t *testing.T) {
	if got := NewKernelSource(Ports{}); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
}

// TestTheAdapterAnswersTheLiveRoster: the agent-scoped write path reaches
// the LIVE kernel — a profile that was added to the roster is the profile
// the port edits, and the edit is really stored.
func TestTheAdapterAnswersTheLiveRoster(t *testing.T) {
	k, err := kernelruntime.Open(withRoster(t, kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, err := k.AddProfile(agentapi.Profile{Slug: "researcher"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	port := NewKernelSource(PortsFromKernel(k))
	got, found, err := port.UpdateProfile("researcher", func(p *agentapi.Profile) {
		p.ConfigOverrides = map[string]string{"AGEZT_MODEL": "agent-model"}
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if !found {
		t.Fatal("the port must find the profile the roster holds")
	}
	if got.ConfigOverrides["AGEZT_MODEL"] != "agent-model" {
		t.Fatalf("the edit is not visible on the answer: %+v", got.ConfigOverrides)
	}
	stored, ok := k.Roster().Get("researcher")
	if !ok || stored.ConfigOverrides["AGEZT_MODEL"] != "agent-model" {
		t.Fatalf("the edit was not stored in the roster: %+v ok=%v", stored.ConfigOverrides, ok)
	}
}

// TestTheAdapterReportsAnUnknownAgent: an unknown ref is answered as "not
// found" and no error — the standing pattern the tool reads as "unknown
// agent".
func TestTheAdapterReportsAnUnknownAgent(t *testing.T) {
	k, err := kernelruntime.Open(withRoster(t, kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	port := NewKernelSource(PortsFromKernel(k))
	_, found, err := port.UpdateProfile("nobody", func(*agentapi.Profile) {})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if found {
		t.Fatal("an unknown agent must not be found")
	}
}

// TestTheAdapterReloadsTheLiveDaemon: the live-apply path really reloads
// the provider — and the catalogue the reload produced is dropped, because
// the plugin may not name it.
func TestTheAdapterReloadsTheLiveDaemon(t *testing.T) {
	base := t.TempDir()
	k, err := kernelruntime.Open(withRoster(t, kernelruntime.Config{
		BaseDir:  base,
		Provider: mock.New(),
		// The catalog directory is the DAEMON's file (W3.51): the test opens
		// it and hands the kernel the contract's store port.
		CatalogStore: catalog.NewStore(filepath.Join(base, "catalog")),
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if err := NewKernelSource(PortsFromKernel(k)).Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
}

// TestTheAdapterReadsTheRunsOverrides: the overrides are the run's own —
// they arrive through the daemon's context convention, read through the port.
func TestTheAdapterReadsTheRunsOverrides(t *testing.T) {
	k, err := kernelruntime.Open(withRoster(t, kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	port := NewKernelSource(PortsFromKernel(k))
	if got := port.AgentConfigOverrides(context.Background()); len(got) != 0 {
		t.Fatalf("a plain context carries no overrides, got %v", got)
	}
	ctx := kernelruntime.WithAgentProfile(context.Background(), agentapi.Profile{
		Slug:            "researcher",
		ConfigOverrides: map[string]string{"AGEZT_MODEL": "agent-model"},
	})
	got := port.AgentConfigOverrides(ctx)
	if got["AGEZT_MODEL"] != "agent-model" {
		t.Fatalf("the port did not read the run's overrides, got %v", got)
	}
	// The map is a copy: the caller may mutate it without touching the run.
	got["AGEZT_MODEL"] = "mutated"
	if again := port.AgentConfigOverrides(ctx); again["AGEZT_MODEL"] != "agent-model" {
		t.Fatalf("the overrides must be a copy, got %v", again)
	}
}
