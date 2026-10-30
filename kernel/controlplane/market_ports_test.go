// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/app/binding"
	"strings"
	"testing"

	appmarket "github.com/agezt/agezt/kernel/app/market"
	core "github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// marketPortLibrary serves one pack, so a tenant's marketplace and the
// daemon's own are told apart by what they carry.
type marketPortLibrary struct{ pack core.Pack }

func (l marketPortLibrary) Marketplaces() []core.Marketplace {
	return []core.Marketplace{{Name: "owned", Builtin: true, Packs: []core.MarketplaceEntry{{Name: l.pack.Name, Version: l.pack.Version}}}}
}

func (l marketPortLibrary) ResolvePack(_, name, _ string) (core.Pack, error) {
	if name != l.pack.Name {
		return core.Pack{}, errors.New("pack not found: " + name)
	}
	return l.pack, nil
}

// The host does not bind the marketplace's ports — the APPLICATION fills them
// from the kernel handle.
//
// The test pins that binding end to end: the manager follows the ROUTED kernel
// — a tenant's operations run on the tenant's own kernel, and the tenant's
// marketplace is the tenant's.
func TestTheAdapterBindsTheMarketPortsToTheRoutedKernel(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	root.SetMarket(core.NewManager(core.Config{Library: marketPortLibrary{pack: core.Pack{Name: "daemon-pack"}}, Store: core.NewStore(t.TempDir())}))

	tenant, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tenant.Close()
	tenant.SetMarket(core.NewManager(core.Config{Library: marketPortLibrary{pack: core.Pack{Name: "tenant-pack"}}, Store: core.NewStore(t.TempDir())}))

	tenantCtx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(tenant)})
	daemonCtx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(root)})
	// The server is on the context too, so a binding that asked the DAEMON's
	// kernel instead of the routed one would answer — wrongly — rather than
	// fall over.
	s := NewServer(binding.ScopeFromKernel(root, t.TempDir()))
	tenantCtx = context.WithValue(tenantCtx, systemHostKey{}, s)
	daemonCtx = context.WithValue(daemonCtx, systemHostKey{}, s)

	// The request's ports follow the routed kernel.
	if requestMarketPorts(tenantCtx).Manager() == nil {
		t.Fatal("the request's marketplace ports hand over no manager")
	}
	tenantPack, _, _, err := requestMarketPorts(tenantCtx).Manager().Show("", "tenant-pack")
	if err != nil {
		t.Fatalf("the tenant's own marketplace: %v", err)
	}
	if tenantPack.Name != "tenant-pack" {
		t.Fatalf("the request's marketplace ports report the pack %q, want the tenant's own", tenantPack.Name)
	}

	// The daemon's own request answers for the daemon's own kernel.
	daemonPack, _, _, err := requestMarketPorts(daemonCtx).Manager().Show("", "daemon-pack")
	if err != nil {
		t.Fatalf("the daemon's own marketplace: %v", err)
	}
	if daemonPack.Name != "daemon-pack" {
		t.Fatalf("the daemon's own marketplace ports report the pack %q, want the daemon's own", daemonPack.Name)
	}

	// The tenant does not see the daemon's marketplace.
	if _, _, _, err := requestMarketPorts(tenantCtx).Manager().Show("", "daemon-pack"); err == nil {
		t.Fatal("the tenant's marketplace serves the daemon's pack — a tenant's operations must run on the tenant's own marketplace")
	}
}

// The marketplace is the daemon's own file: a daemon that holds no manager
// holds no marketplace, and the surfaces that read it refuse.
func TestTheAdapterBindsTheMarketPortsToAHostWithNoManager(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(root)})
	if got := requestMarketPorts(ctx).Manager(); got != nil {
		t.Fatal("a daemon with no marketplace hands over a marketplace manager")
	}

	// The refusal is the application's own sentence, and it is written once:
	// the reads take the port's manager, and a port with no manager refuses.
	_, err = appmarket.NewReads(requestMarketPorts(ctx).Manager()).List(context.Background(), appmarket.ListInput{})
	if err == nil || !strings.Contains(err.Error(), "marketplace not available on this daemon") {
		t.Fatalf("the read reports %v, want the marketplace's own refusal", err)
	}
}

// The application reads through the port: the service takes the port's manager
// and asks it, and the port hands over the daemon's own.
func TestTheMarketServiceReadsThroughThePort(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	root.SetMarket(core.NewManager(core.Config{Library: marketPortLibrary{pack: core.Pack{Name: "owned"}}, Store: core.NewStore(t.TempDir())}))

	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(root)})
	out, err := appmarket.NewReads(requestMarketPorts(ctx).Manager()).List(context.Background(), appmarket.ListInput{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if out.Count != 1 {
		t.Fatalf("the marketplace list reports %d packs, want the daemon's own one", out.Count)
	}
}
