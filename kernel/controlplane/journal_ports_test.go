// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the journal family's ports — the APPLICATION fills
// them from the kernel handle.
//
// The test pins that binding end to end: the daemon's own journal answers for
// the daemon's kernel, and the request's journal follows the ROUTED kernel —
// a tenant's journal is the tenant's journal.
func TestTheAdapterBindsTheJournalPortsToItsOwnKernel(t *testing.T) {
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

	s := NewServer(root, t.TempDir())
	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{kernel: tenant})

	// The daemon's own ports answer for its own kernel.
	if s.journalPorts().Events == nil {
		t.Fatal("the daemon's journal ports hold no journal — the kernel holds one")
	}
	if requestJournalPorts(ctx).Events == nil {
		t.Fatal("the request's journal ports hold no journal — the routed kernel holds one")
	}
	if _, err := s.journalPorts().Events.Append(event.Spec{Subject: "daemon.event", Kind: event.KindTaskReceived, Actor: "test"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if !journalHasEvent(s.journalPorts().Events, "daemon.event") {
		t.Error("the daemon's journal ports do not answer for its own kernel")
	}

	// The request's ports follow the routed kernel.
	if _, err := requestJournalPorts(ctx).Events.Append(event.Spec{Subject: "tenant.event", Kind: event.KindTaskReceived, Actor: "test"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if !journalHasEvent(requestJournalPorts(ctx).Events, "tenant.event") {
		t.Error("the request's journal ports do not answer for the routed kernel")
	}
	if journalHasEvent(requestJournalPorts(ctx).Events, "daemon.event") {
		t.Error("the request's journal ports see the root kernel's event — a tenant's journal is the tenant's journal")
	}
}

// journalHasEvent reports whether the journal holds an event with the subject.
func journalHasEvent(events interface {
	Range(func(*event.Event) error) error
}, subject string) bool {
	seen := false
	_ = events.Range(func(e *event.Event) error {
		if e.Subject == subject {
			seen = true
		}
		return nil
	})
	return seen
}
