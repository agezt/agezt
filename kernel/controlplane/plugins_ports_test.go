// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"testing"

	appplugins "github.com/agezt/agezt/kernel/app/plugins"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the plugins' ports — the APPLICATION fills them from
// the kernel handle.
//
// The test pins that binding end to end: the manifest follows the ROUTED
// kernel — a tenant's operations run on the tenant's own kernel, and the
// tenant's manifest is the tenant's.
func TestTheAdapterBindsThePluginPortsToTheRoutedKernel(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(),
		Plugins: []runtime.PluginInfo{{Prefix: "daemon"}}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	tenant, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(),
		Plugins: []runtime.PluginInfo{{Prefix: "tenant"}}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tenant.Close()

	s := NewServer(root, t.TempDir())
	tenantCtx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: tenant})
	daemonCtx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: root})
	daemonCtx = context.WithValue(daemonCtx, systemHostKey{}, s)

	// The request's ports follow the routed kernel.
	if got := requestPluginPorts(tenantCtx).Manifest(); len(got) != 1 || got[0].Prefix != "tenant" {
		t.Fatalf("the request's plugin ports report %v, want the tenant's own manifest", got)
	}

	// The daemon's own request answers for the daemon's own kernel.
	if got := requestPluginPorts(daemonCtx).Manifest(); len(got) != 1 || got[0].Prefix != "daemon" {
		t.Fatalf("the daemon's own plugin ports report %v, want the daemon's own manifest", got)
	}
	if requestPluginPorts(tenantCtx).Manifest()[0].Prefix == requestPluginPorts(daemonCtx).Manifest()[0].Prefix {
		t.Fatal("the tenant's manifest is the daemon's — a tenant's operations must run on the tenant's own manifest")
	}
}

// The plugin manifest is the daemon's own list, and a daemon that was given
// none holds none. The binding says so instead of inventing a manifest.
func TestTheAdapterBindsThePluginPortsToAHostWithNoManifest(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: root})
	if got := requestPluginPorts(ctx).Manifest(); len(got) != 0 {
		t.Fatalf("a daemon with no plugin manifest reports %v", got)
	}
}

// The config reader reports the plugin count from the PLUGINS family's port.
//
// The reader holds the kernel handle for the daemon's own identity, and it used
// to reach for the kernel's plugin manifest as well — the count was the
// adapter's own spelling of a question another family answers.
func TestTheConfigReaderCountsThePluginsThroughThePort(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(),
		Plugins: []runtime.PluginInfo{{Prefix: "one"}, {Prefix: "two"}}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: root})
	reader := nativeConfigReader{plugins: requestPluginPorts(ctx)}

	if got := reader.PluginCount(); got != 2 {
		t.Fatalf("the config reader reports %d plugins, want the manifest's two", got)
	}
}

// The application reads the manifest through the port, and the port IS the
// application's own narrow view — the adapter hands it over as it is.
func TestThePluginServiceReadsTheManifestThroughThePort(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(),
		Plugins: []runtime.PluginInfo{{Prefix: "acme", Path: "/opt/acme"}}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: root})
	out, err := appplugins.New(requestPluginPorts(ctx)).List(context.Background(), appplugins.ListInput{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if out.Count != 1 || len(out.Plugins) != 1 || out.Plugins[0].Prefix != "acme" {
		t.Fatalf("the plugin list reports %v", out)
	}
}
