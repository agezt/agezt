// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"github.com/agezt/agezt/kernel/app/binding"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the persona's ports — the APPLICATION fills them from
// the kernel handle.
//
// The test pins that binding end to end: the identity follows the ROUTED
// kernel — a tenant's config reports the tenant's own model — and the daemon's
// own request answers for the daemon's own.
func TestTheAdapterBindsThePersonaPortsToTheRoutedKernel(t *testing.T) {
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
	root.SetModel("daemon-model")
	tenant.SetModel("tenant-model")

	tenantCtx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(tenant)})
	daemonCtx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(root)})

	// The request's ports follow the routed kernel.
	if got := requestPersonaPorts(tenantCtx).Identity.DefaultModel(); got != "tenant-model" {
		t.Fatalf("the request's persona ports report the model %q, want the tenant's own", got)
	}

	// The daemon's own request answers for the daemon's own.
	if got := requestPersonaPorts(daemonCtx).Identity.DefaultModel(); got != "daemon-model" {
		t.Fatalf("the daemon's own persona ports report the model %q, want the daemon's own", got)
	}
}

// The config reader reports the daemon's default identity THROUGH THE PORT —
// it holds no kernel handle at all.
//
// The test holds BOTH answers: a daemon with a prompt says so, and a daemon
// without one says the opposite. A reader that always said "set" would be
// wrong half the time.
func TestTheConfigReaderReportsTheIdentityThroughThePort(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	root.SetModel("the-model")
	root.SetSystem("the prompt")

	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(root)})
	reader := nativeConfigReader{persona: requestPersonaPorts(ctx)}

	if got := reader.Model(); got != "the-model" {
		t.Fatalf("the config reader reports the model %q, want the daemon's own", got)
	}
	if !reader.SystemPromptSet() {
		t.Fatal("the config reader says no prompt is set — the daemon has one")
	}

	// A daemon with no prompt says the opposite.
	bare, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer bare.Close()
	bareCtx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(bare)})
	bareReader := nativeConfigReader{persona: requestPersonaPorts(bareCtx)}

	if bareReader.SystemPromptSet() {
		t.Fatal("the config reader says a prompt is set — the daemon has none")
	}
	if got := bareReader.Model(); got != "" {
		t.Fatalf("the config reader reports the model %q, want none", got)
	}
}

// The daemon-level binding answers for the DAEMON's identity, and the prompt
// changes LIVE through it.
func TestThePersonaPortsEditTheDaemonsIdentity(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	root.SetSystem("before")

	s := NewServer(binding.ScopeFromKernel(root, t.TempDir()))
	s.personaPorts().Identity.SetPrompt("after")

	if got := root.System(); got != "after" {
		t.Fatalf("the kernel holds the prompt %q, want the one the persona port set", got)
	}
}
