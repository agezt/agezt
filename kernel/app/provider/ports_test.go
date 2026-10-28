// SPDX-License-Identifier: MIT

package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// newGovernor opens the daemon's live provider: the governor that carries the
// routing tables and the spend.
func newGovernor(t *testing.T, cfg governor.Config) *governor.Governor {
	t.Helper()
	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{
		Name: "mock", Provider: mock.New(), AuthMode: governor.AuthLocal,
	}); err != nil {
		t.Fatalf("register the mock provider: %v", err)
	}
	cfg.Registry = reg
	g, err := governor.New(cfg)
	if err != nil {
		t.Fatalf("governor.New: %v", err)
	}
	return g
}

// The provider family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins what that wiring buys: the chains the port reads are the
// chains the daemon routes by, the routing view is the daemon's own, and the
// spend is the spend the daemon has made. A port bound to another provider
// would leave the operator editing a routing the daemon does not take.
func TestTheProviderPortsReachTheKernelProvider(t *testing.T) {
	dir := t.TempDir()
	gov := newGovernor(t, governor.Config{
		TaskModelChains: governor.TaskModelChains{"plan": {"model-a", "model-b"}},
		FallbackChains:  map[string][]string{"safe": {"model-c"}},
		DefaultChain:    "safe",
		TaskRoutes:      governor.TaskRoutes{"plan": {"primary"}},
		TaskModelOverrides: governor.TaskModelOverrides{
			"plan": "model-a",
		},
	})
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: gov})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.Live() == nil {
		t.Fatal("the provider port holds no provider — the kernel holds a provider")
	}

	// The chains are the daemon's own.
	chains, ok := ports.TaskChains()
	if !ok {
		t.Fatal("the provider port holds no task chains — the daemon routes by chains")
	}
	if got := chains["plan"]; len(got) != 2 || got[0] != "model-a" || got[1] != "model-b" {
		t.Fatalf("the provider port read chains %v, the daemon routes by %v", chains, governor.TaskModelChains{"plan": {"model-a", "model-b"}})
	}

	// And the port writes the chains the daemon then routes by.
	ports.SetTaskChains(governor.TaskModelChains{"plan": {"model-z"}})
	after, ok := ports.TaskChains()
	if !ok {
		t.Fatal("the provider port lost its chains after a write")
	}
	if got := after["plan"]; len(got) != 1 || got[0] != "model-z" {
		t.Fatalf("the provider port wrote chains %v, the daemon routes by %v", after, governor.TaskModelChains{"plan": {"model-z"}})
	}

	// The named chains are the daemon's own too.
	named, def, ok := ports.Named()
	if !ok {
		t.Fatal("the provider port holds no named chains — the daemon falls back by named chains")
	}
	if got := named["safe"]; len(got) != 1 || got[0] != "model-c" {
		t.Fatalf("the provider port read named chains %v, the daemon falls back by %v", named, map[string][]string{"safe": {"model-c"}})
	}
	if def != "safe" {
		t.Fatalf("the provider port read the default chain %q, the daemon falls back by %q", def, "safe")
	}

	// The routing view is the daemon's own.
	view, ok := ports.Routing()
	if !ok {
		t.Fatal("the provider port holds no routing view — the daemon reports its routing")
	}
	if got := view.TaskRoutesView()["plan"]; len(got) != 1 || got[0] != "primary" {
		t.Fatalf("the provider port read routes %v, the daemon routes by %v", view.TaskRoutesView(), governor.TaskRoutes{"plan": {"primary"}})
	}
	if got := view.TaskModelOverridesView()["plan"]; got != "model-a" {
		t.Fatalf("the provider port read the model override %q, the daemon overrides with %q", got, "model-a")
	}

	// And the spend is the daemon's own.
	budget := ports.Budget()
	if budget == nil {
		t.Fatal("the provider port holds no budget — the daemon keeps a budget")
	}
	spent, ok := ports.SpentMicrocents()
	if !ok {
		t.Fatal("the provider port cannot say what the daemon spent")
	}
	if spent != budget.Snapshot().SpentMicrocents {
		t.Fatalf("the provider port says %d spent, the keeper says %d", spent, budget.Snapshot().SpentMicrocents)
	}
}

// A host with no provider must not call into nothing.
//
// The provider is the daemon's live door to the model, and a host that was
// given none holds nothing: the chains are absent, the routing view is
// absent, the budget is absent. Nothing is invented, and nothing panics.
func TestTheProviderPortsSurviveAHostWithNoProvider(t *testing.T) {
	ports := PortsFromKernel(nil)
	if ports.Live() != nil {
		t.Fatal("a host with no kernel hands out a provider")
	}
	if chains, ok := ports.TaskChains(); ok {
		t.Fatalf("a host with no provider reads chains %v", chains)
	}
	ports.SetTaskChains(governor.TaskModelChains{"plan": {"model-a"}})
	if chains, ok := ports.TaskChains(); ok {
		t.Fatalf("a host with no provider reads chains %v after a write", chains)
	}
	if named, def, ok := ports.Named(); ok {
		t.Fatalf("a host with no provider reads named chains %v (default %q)", named, def)
	}
	ports.SetNamed(map[string][]string{"safe": {"model-c"}}, "safe")
	if view, ok := ports.Routing(); ok {
		t.Fatalf("a host with no provider holds the routing view %v", view)
	}
	if ports.Budget() != nil {
		t.Fatal("a host with no provider hands out a budget")
	}
	if spent, ok := ports.SpentMicrocents(); ok {
		t.Fatalf("a host with no provider says %d spent", spent)
	}
}

// A provider that carries none of the faces answers "absent", not "empty".
//
// The governor carries all of them, but a provider is the CONTRACT's — a host
// may hand the daemon a provider that routes nothing. The port must not
// invent a routing table it was not given.
func TestTheProviderPortsRefuseAProviderThatCarriesNoFaces(t *testing.T) {
	ports := Ports{provider: bareProvider{}}
	if ports.Live() == nil {
		t.Fatal("the port holds no provider")
	}
	if chains, ok := ports.TaskChains(); ok {
		t.Fatalf("a bare provider reads chains %v", chains)
	}
	if view, ok := ports.Routing(); ok {
		t.Fatalf("a bare provider holds the routing view %v", view)
	}
	if ports.Budget() != nil {
		t.Fatal("a bare provider hands out a budget")
	}
	if spent, ok := ports.SpentMicrocents(); ok {
		t.Fatalf("a bare provider says %d spent", spent)
	}
}

// bareProvider is a provider that routes nothing: the contract and nothing
// else.
type bareProvider struct{}

func (bareProvider) Name() string { return "bare" }

func (bareProvider) Complete(_ context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return nil, errors.New("the bare provider routes nothing")
}
