// SPDX-License-Identifier: MIT

package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"encoding/json"
	"github.com/agezt/agezt/kernel/app"
	appmemory "github.com/agezt/agezt/kernel/app/memory"
	"github.com/agezt/agezt/kernel/contract/opapi"
	store "github.com/agezt/agezt/kernel/memory"
)

type waitingDistiller struct {
	entered chan context.Context
	release chan struct{}
}

func TestTypedDistillationRetainsCallerCancellation(t *testing.T) {
	for _, command := range []string{"memory_consolidate", "profile_rebuild"} {
		w := &waitingDistiller{entered: make(chan context.Context, 1), release: make(chan struct{})}
		operations, err := appmemory.Operations(func(context.Context) *appmemory.Service { return nil }, func(context.Context) *appmemory.Distillation { return appmemory.NewDistillation(w) }, func(context.Context) *appmemory.LogService { return nil })
		if err != nil {
			t.Fatal(err)
		}
		dispatcher, err := app.NewDispatcher(operations, app.Dependencies{Auth: memoryAuth{}, Router: memoryRouter{}, Audit: &memoryAudit{}})
		if err != nil {
			t.Fatal(err)
		}
		parent, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := dispatcher.Dispatch(parent, opapi.Caller{}, command, json.RawMessage(`{}`), nil)
			done <- err
		}()
		<-w.entered
		cancel()
		select {
		case err := <-done:
			close(w.release)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("%s typed cancellation=%v", command, err)
			}
		case <-time.After(250 * time.Millisecond):
			close(w.release)
			<-done
			t.Errorf("%s typed operation retained caller wait", command)
		}
	}
}

func (*waitingDistiller) NewCorrelation() string { return "owned-fallback" }
func (w *waitingDistiller) wait(ctx context.Context) error {
	w.entered <- ctx
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.release:
		return nil
	}
}
func (w *waitingDistiller) DistillBrain(ctx context.Context, _ string) (store.BrainDistillReport, error) {
	return store.BrainDistillReport{}, w.wait(ctx)
}
func (w *waitingDistiller) DistillProfile(ctx context.Context, _ string) (store.ProfileReport, error) {
	return store.ProfileReport{}, w.wait(ctx)
}

func TestDistillationCallerCancellationReleasesInFlightPort(t *testing.T) {
	for _, profile := range []bool{false, true} {
		w := &waitingDistiller{entered: make(chan context.Context, 1), release: make(chan struct{})}
		service := appmemory.NewDistillation(w)
		parent, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			var err error
			if profile {
				_, err = service.RebuildProfile(parent, appmemory.DistillInput{})
			} else {
				_, err = service.Consolidate(parent, appmemory.DistillInput{})
			}
			done <- err
		}()
		<-w.entered
		cancel()
		select {
		case err := <-done:
			close(w.release)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("profile=%v cancellation cause=%v", profile, err)
			}
		case <-time.After(250 * time.Millisecond):
			close(w.release)
			<-done
			t.Errorf("profile=%v port retained after caller cancellation", profile)
		}
	}
}

type distillationValueKey struct{}

func TestDistillationCallerDeadlineValuesAndPreCanceledAdmission(t *testing.T) {
	for _, profile := range []bool{false, true} {
		fake := &fakeDistiller{}
		service := appmemory.NewDistillation(fake)
		parent, cancel := context.WithTimeout(context.WithValue(context.Background(), distillationValueKey{}, "caller-model-context"), time.Second)
		var err error
		if profile {
			_, err = service.RebuildProfile(parent, appmemory.DistillInput{})
		} else {
			_, err = service.Consolidate(parent, appmemory.DistillInput{})
		}
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		ctx := fake.contexts[0]
		deadline, ok := ctx.Deadline()
		parentDeadline, _ := parent.Deadline()
		if !ok || !deadline.Equal(parentDeadline) || ctx.Value(distillationValueKey{}) != "caller-model-context" {
			t.Errorf("profile=%v lost caller context: deadline=%v want=%v value=%v", profile, deadline, parentDeadline, ctx.Value(distillationValueKey{}))
		}
		fake = &fakeDistiller{}
		service = appmemory.NewDistillation(fake)
		parent, cancel = context.WithCancel(context.Background())
		cancel()
		if profile {
			_, err = service.RebuildProfile(parent, appmemory.DistillInput{})
		} else {
			_, err = service.Consolidate(parent, appmemory.DistillInput{})
		}
		if !errors.Is(err, context.Canceled) || fake.next != 0 || len(fake.contexts) != 0 {
			t.Errorf("profile=%v pre-canceled effect: err=%v generated=%d calls=%d", profile, err, fake.next, len(fake.contexts))
		}
	}
}
