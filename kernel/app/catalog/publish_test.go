// SPDX-License-Identifier: MIT

package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/event"
)

// recordingPublisher records what the service published.
type recordingPublisher struct{ specs []event.Spec }

func (r *recordingPublisher) Publish(spec event.Spec) (*event.Event, error) {
	r.specs = append(r.specs, spec)
	return &event.Event{}, nil
}

// failingSyncer fails every fetch, so the service takes the failure path.
type failingSyncer struct{}

func (failingSyncer) Sync(context.Context, string, time.Duration) ([]byte, *catalogapi.Catalog, catalogapi.SyncResult, error) {
	return nil, nil, catalogapi.SyncResult{}, errors.New("the fetch failed")
}

// The catalog family publishes what happened, through the bus it was given.
//
// The event is the operator's trace of a sync that did not work: the service
// used to reach the kernel's own bus for it, and the bus is another family's
// port. The test pins that the family still PUBLISHES — a narrow view that
// stops publishing is a narrow view that lost the event.
func TestTheCatalogServicePublishesItsEventsThroughTheBusItWasGiven(t *testing.T) {
	pub := &recordingPublisher{}
	svc := New(Ports{syncer: func() catalogapi.Syncer { return failingSyncer{} }}, pub, t.TempDir())

	if _, err := svc.Sync(context.Background(), SyncInput{URL: "http://catalog.invalid/models.json"}); err == nil {
		t.Fatal("the sync must fail when the syncer fails")
	}
	if len(pub.specs) != 1 {
		t.Fatalf("the service published %d events, want the one that says the sync failed", len(pub.specs))
	}
	if pub.specs[0].Kind != event.KindCatalogSyncFailed {
		t.Fatalf("the service published %q, want the sync failure", pub.specs[0].Kind)
	}
	if pub.specs[0].Subject != "catalog.sync" {
		t.Fatalf("the service published the subject %q", pub.specs[0].Subject)
	}
}

// A host with no bus publishes nothing — and does not panic.
//
// The refusal is the service's own: an event that cannot be written is not the
// operation's failure, and the family spells that once, here.
func TestTheCatalogServiceSurvivesAHostWithNoBus(t *testing.T) {
	svc := New(Ports{syncer: func() catalogapi.Syncer { return failingSyncer{} }}, nil, t.TempDir())

	if _, err := svc.Sync(context.Background(), SyncInput{URL: "http://catalog.invalid/models.json"}); err == nil {
		t.Fatal("the sync must fail when the syncer fails")
	}
}
