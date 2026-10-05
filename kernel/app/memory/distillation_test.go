// SPDX-License-Identifier: MIT

package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	appmemory "github.com/agezt/agezt/kernel/app/memory"
	"github.com/agezt/agezt/kernel/contract/opapi"
	store "github.com/agezt/agezt/kernel/memory"
)

type fakeDistiller struct {
	next        int
	trace       []string
	contexts    []context.Context
	entryErrors []error
	cause       error
	brain       store.BrainDistillReport
	profile     store.ProfileReport
}

func TestDistillationRetainsAdmittedOperationIdentity(t *testing.T) {
	fake := &fakeDistiller{}
	service := appmemory.NewDistillation(fake)
	ctx := opapi.WithCorrelation(context.Background(), "owned-operation")
	brain, err := service.Consolidate(ctx, appmemory.DistillInput{})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := service.RebuildProfile(ctx, appmemory.DistillInput{})
	if err != nil {
		t.Fatal(err)
	}
	if brain.CorrelationID != "owned-operation" || profile.CorrelationID != "owned-operation" || fake.next != 0 || !reflect.DeepEqual(fake.trace, []string{"brain:owned-operation", "profile:owned-operation"}) {
		t.Fatalf("admitted identity replaced: brain=%s profile=%s generated=%d trace=%v", brain.CorrelationID, profile.CorrelationID, fake.next, fake.trace)
	}
}

func (f *fakeDistiller) NewCorrelation() string {
	f.next++
	f.trace = append(f.trace, "correlation")
	return fmt.Sprintf("fixture-%d", f.next)
}
func (f *fakeDistiller) DistillBrain(ctx context.Context, corr string) (store.BrainDistillReport, error) {
	f.trace = append(f.trace, "brain:"+corr)
	f.contexts = append(f.contexts, ctx)
	f.entryErrors = append(f.entryErrors, ctx.Err())
	return f.brain, f.cause
}
func (f *fakeDistiller) DistillProfile(ctx context.Context, corr string) (store.ProfileReport, error) {
	f.trace = append(f.trace, "profile:"+corr)
	f.contexts = append(f.contexts, ctx)
	f.entryErrors = append(f.entryErrors, ctx.Err())
	return f.profile, f.cause
}

func TestDistillationRetainsReportsIdentityAndOwnedBudget(t *testing.T) {
	fake := &fakeDistiller{brain: store.BrainDistillReport{ClustersFound: 3, ClustersMerged: 2, RecordsSuperseded: 4,
		ConsolidatedIDs: []string{"a", "b"}, SkippedNonJSON: 1, ActiveBefore: 9, ActiveAfterApprox: 7},
		profile: store.ProfileReport{InputRecords: 8, FacetsWritten: 2, Facets: []string{"style", "preferences"}}}
	service := appmemory.NewDistillation(fake)
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	brain, err := service.Consolidate(parent, appmemory.DistillInput{})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := service.RebuildProfile(parent, appmemory.DistillInput{})
	if err != nil {
		t.Fatal(err)
	}
	if brain.CorrelationID != "fixture-1" || brain.ClustersFound != 3 || brain.ClustersMerged != 2 || brain.RecordsSuperseded != 4 ||
		!reflect.DeepEqual(brain.ConsolidatedIDs, []string{"a", "b"}) || brain.SkippedNonJSON != 1 || brain.ActiveBefore != 9 || brain.ActiveAfter != 7 {
		t.Fatalf("brain=%+v", brain)
	}
	if profile.CorrelationID != "fixture-2" || profile.InputRecords != 8 || profile.FacetsWritten != 2 || !reflect.DeepEqual(profile.Facets, []string{"style", "preferences"}) {
		t.Fatalf("profile=%+v", profile)
	}
	if !reflect.DeepEqual(fake.trace, []string{"correlation", "brain:fixture-1", "correlation", "profile:fixture-2"}) {
		t.Fatalf("call order=%v", fake.trace)
	}
	for _, ctx := range fake.contexts {
		deadline, ok := ctx.Deadline()
		if !ok || deadline.Before(start.Add(5*time.Minute)) || deadline.After(time.Now().Add(5*time.Minute)) {
			t.Errorf("background ceiling=%v ok=%v", deadline, ok)
		}
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Errorf("owned context retained after return: %v", ctx.Err())
		}
	}
	for _, err := range fake.entryErrors {
		if err != nil {
			t.Errorf("background entry inherited caller cancellation: %v", err)
		}
	}
}

func TestDistillationRetainsPresentNullAndZeroFields(t *testing.T) {
	fake := &fakeDistiller{}
	service := appmemory.NewDistillation(fake)
	brain, err := service.Consolidate(context.Background(), appmemory.DistillInput{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(brain)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"clusters_found", "clusters_merged", "records_superseded", "skipped_non_json", "active_before", "active_after"} {
		if value, exists := wire[key]; !exists || value != float64(0) {
			t.Errorf("present zero %s=%v exists=%v", key, value, exists)
		}
	}
	if value, exists := wire["consolidated_ids"]; !exists || value != nil {
		t.Errorf("present null ids=%v exists=%v", value, exists)
	}
	if _, exists := wire["active_after_approx"]; exists {
		t.Fatal("store field leaked into native wire")
	}
	profile, err := service.RebuildProfile(context.Background(), appmemory.DistillInput{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	wire = map[string]any{}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if value, exists := wire["facets"]; !exists || value != nil {
		t.Errorf("present null facets=%v exists=%v", value, exists)
	}
	for _, key := range []string{"input_records", "facets_written"} {
		if value, exists := wire[key]; !exists || value != float64(0) {
			t.Errorf("present zero %s=%v exists=%v", key, value, exists)
		}
	}
}

func TestDistillationRetainsCauseAndReleasesFailedContexts(t *testing.T) {
	cause := errors.New("fixture distillation failed")
	fake := &fakeDistiller{cause: cause}
	service := appmemory.NewDistillation(fake)
	_, err := service.Consolidate(context.Background(), appmemory.DistillInput{})
	if !errors.Is(err, cause) {
		t.Errorf("brain cause=%v", err)
	}
	_, err = service.RebuildProfile(context.Background(), appmemory.DistillInput{})
	if !errors.Is(err, cause) {
		t.Errorf("profile cause=%v", err)
	}
	for _, ctx := range fake.contexts {
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Errorf("failed context retained: %v", ctx.Err())
		}
	}
}
