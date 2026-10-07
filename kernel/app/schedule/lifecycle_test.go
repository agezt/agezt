// SPDX-License-Identifier: MIT
package schedule_test

import (
	"context"
	"errors"
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	"github.com/agezt/agezt/kernel/cadence"
	"reflect"
	"testing"
	"time"
)

type lifeStore struct {
	entry          cadence.Entry
	found, updated bool
	cause          error
	calls          []string
	id             string
	enabled        bool
}

func (s *lifeStore) Get(id string) (cadence.Entry, bool) {
	s.calls = append(s.calls, "get")
	s.id = id
	return s.entry, s.found
}
func (s *lifeStore) Remove(id string) (bool, error) {
	s.calls = append(s.calls, "remove")
	s.id = id
	return s.updated, s.cause
}
func (s *lifeStore) RunNow(id string) (bool, error) {
	s.calls = append(s.calls, "run")
	s.id = id
	return s.updated, s.cause
}
func (s *lifeStore) SetEnabled(id string, value bool) (bool, error) {
	s.calls = append(s.calls, "set")
	s.id = id
	s.enabled = value
	return s.updated, s.cause
}
func TestScheduleLifecycleRetainsValidationEffectAndSuccessOnlyPublicationOrder(t *testing.T) {
	for _, mode := range []string{"remove", "run", "enable", "pause", "missing-enable", "unchanged-enable", "invalid-run", "invalid-enable", "store-cause"} {
		t.Run(mode, func(t *testing.T) {
			store := &lifeStore{entry: cadence.Entry{ID: "canonical", Target: cadence.TargetIntent, Agent: "writer", IntervalSec: 60}, found: true, updated: true}
			cause := errors.New("owned cause")
			if mode == "missing-enable" {
				store.found = false
				store.updated = false
			}
			if mode == "unchanged-enable" {
				store.updated = false
			}
			if mode == "store-cause" {
				store.cause = cause
			}
			published := 0
			service := appschedule.NewLifecycle(store, func(entry cadence.Entry) error {
				store.calls = append(store.calls, "validate")
				if entry.ID != "canonical" {
					t.Fatal(entry)
				}
				if mode == "invalid-run" || mode == "invalid-enable" {
					return cause
				}
				return nil
			}, func(id string, enabled bool, action string, current cadence.Entry) {
				store.calls = append(store.calls, "publish")
				published++
				if id != "alias" || current.ID != "canonical" || enabled != (mode != "pause") || action != map[bool]string{true: "resumed", false: "paused"}[enabled] {
					t.Fatal(id, enabled, action, current)
				}
			})
			ctx := context.Background()
			var err error
			switch mode {
			case "remove":
				out, cause := service.Remove(ctx, appschedule.IDInput{ID: "alias"})
				err = cause
				if !out.Removed {
					t.Fatal(out)
				}
			case "run", "invalid-run":
				out, cause := service.Run(ctx, appschedule.IDInput{ID: "alias"})
				err = cause
				if mode == "run" && !out.Triggered {
					t.Fatal(out)
				}
			default:
				out, cause := service.Enable(ctx, appschedule.EnableInput{ID: "alias", Enabled: mode != "pause"})
				err = cause
				if cause == nil && (out.ID != "alias" || out.Enabled != (mode != "pause") || out.Action != map[bool]string{true: "resumed", false: "paused"}[out.Enabled]) {
					t.Fatal(out)
				}
			}
			if store.id != "alias" {
				t.Fatal(store.id)
			}
			want := []string{}
			switch mode {
			case "remove":
				want = []string{"remove"}
			case "run":
				want = []string{"get", "validate", "run"}
			case "enable":
				want = []string{"get", "validate", "set", "publish"}
			case "pause":
				want = []string{"get", "set", "publish"}
			case "missing-enable":
				want = []string{"get", "set"}
			case "unchanged-enable":
				want = []string{"get", "validate", "set"}
			case "invalid-run", "invalid-enable":
				want = []string{"get", "validate"}
			case "store-cause":
				want = []string{"get", "validate", "set"}
			}
			if !reflect.DeepEqual(store.calls, want) {
				t.Fatal(store.calls, want)
			}
			failed := mode == "invalid-run" || mode == "invalid-enable" || mode == "store-cause"
			if failed && err != cause || !failed && err != nil {
				t.Fatal(err, cause)
			}
			wantPublish := mode == "enable" || mode == "pause"
			if (published == 1) != wantPublish {
				t.Fatal("publication count", mode, published)
			}
		})
	}
}
func TestScheduleLifecycleRetainsOriginalStoreCausesAndZeroOutputs(t *testing.T) {
	cause := errors.New("owned store cause")
	store := &lifeStore{cause: cause}
	service := appschedule.NewLifecycle(store, nil, func(string, bool, string, cadence.Entry) { t.Fatal("failure published") })
	if out, err := service.Remove(context.Background(), appschedule.IDInput{ID: "owned"}); err != cause || !reflect.DeepEqual(out, appschedule.RemoveOutput{}) {
		t.Fatal(out, err)
	}
	if out, err := service.Run(context.Background(), appschedule.IDInput{ID: "owned"}); err != cause || !reflect.DeepEqual(out, appschedule.RunOutput{}) {
		t.Fatal(out, err)
	}
	if out, err := service.Enable(context.Background(), appschedule.EnableInput{ID: "owned"}); err != cause || !reflect.DeepEqual(out, appschedule.EnableOutput{}) {
		t.Fatal(out, err)
	}
}
func TestScheduleLifecycleUsesActualCadenceStoreWithoutRunningWork(t *testing.T) {
	store, err := cadence.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entry, err := store.Add("owned", time.Hour, "", cadence.SourceOperator, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	publications := 0
	service := appschedule.NewLifecycle(store, func(cadence.Entry) error { return nil }, func(string, bool, string, cadence.Entry) { publications++ })
	out, err := service.Enable(context.Background(), appschedule.EnableInput{ID: entry.ID, Enabled: false})
	current, _ := store.Get(entry.ID)
	if err != nil || !out.Updated || out.Action != "paused" || current.Enabled {
		t.Fatal(out, current, err)
	}
	out, err = service.Enable(context.Background(), appschedule.EnableInput{ID: entry.ID, Enabled: true})
	if err != nil || !out.Updated || out.Action != "resumed" || publications != 2 {
		t.Fatal(out, err, publications)
	}
	run, err := service.Run(context.Background(), appschedule.IDInput{ID: entry.ID})
	current, _ = store.Get(entry.ID)
	if err != nil || !run.Triggered || current.NextRunUnix != 0 {
		t.Fatal(run, current, err)
	}
	removed, err := service.Remove(context.Background(), appschedule.IDInput{ID: entry.ID})
	if err != nil || !removed.Removed {
		t.Fatal(removed, err)
	}
	removed, err = service.Remove(context.Background(), appschedule.IDInput{ID: entry.ID})
	if err != nil || removed.Removed {
		t.Fatal("missing remove", removed, err)
	}
}
