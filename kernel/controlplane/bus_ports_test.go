// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"github.com/agezt/agezt/kernel/app/binding"
	"testing"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the event bus family's ports — the APPLICATION fills
// them from the kernel handle.
//
// The test pins that binding end to end: the daemon's own bus answers for the
// daemon's kernel, and the request's bus follows the ROUTED kernel — a
// tenant's bus is the tenant's bus.
//
// The bus persists through the journal, so the journal is where the effect is
// read: an event published on the daemon's bus lands in the daemon's journal
// and nowhere else.
func TestTheAdapterBindsTheBusPortsToItsOwnKernel(t *testing.T) {
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

	s := NewServer(binding.ScopeFromKernel(root, t.TempDir()))
	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(tenant)})

	// The daemon's own ports answer for its own kernel.
	if !s.busPorts().Ready() {
		t.Fatal("the daemon's bus ports are not ready — the kernel holds a bus")
	}
	if _, err := s.busPorts().Publish(event.Spec{Subject: "daemon.bus", Kind: event.KindInfo, Actor: "test"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if !journalHasEvent(root.Journal(), "daemon.bus") {
		t.Error("the daemon's bus ports do not answer for its own kernel")
	}

	// The request's ports follow the routed kernel.
	if !requestBusPorts(ctx).Ready() {
		t.Fatal("the request's bus ports are not ready — the routed kernel holds a bus")
	}
	if _, err := requestBusPorts(ctx).Publish(event.Spec{Subject: "tenant.bus", Kind: event.KindInfo, Actor: "test"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if !journalHasEvent(tenant.Journal(), "tenant.bus") {
		t.Error("the request's bus ports do not answer for the routed kernel")
	}
	if journalHasEvent(root.Journal(), "tenant.bus") {
		t.Error("the request's bus ports published on the root kernel's bus — a tenant's bus is the tenant's bus")
	}
}
