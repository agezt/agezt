// SPDX-License-Identifier: MIT

package schedule

import (
	"context"
	"errors"
	"testing"
)

// TestTheAppLayerRefusesWithoutALibrary: the typed schedule library is the
// DAEMON's file (W3.49) — the app layer holds the contract's port, and an
// app layer without one says so instead of dereferencing nothing.
//
// The refusal is the same answer the kernel gives, and it names the reason:
// "the schedule library is not available on this daemon". An operator must be
// able to tell an unwired daemon from an empty library.
func TestTheAppLayerRefusesWithoutALibrary(t *testing.T) {
	ctx := context.Background()

	// The creation surface.
	if _, err := NewCreation(nil, nil).Create(ctx, CreateInput{}); !errors.Is(err, errNoScheduleLibrary) {
		t.Fatalf("create must refuse without a library, got %v", err)
	}

	// The editing surface: the begin finds nothing, the apply refuses.
	if _, found := NewEditing(nil, EditingHost{}).Begin(ctx, IDInput{ID: "any"}); found {
		t.Fatal("begin must find nothing without a library")
	}
	if _, err := NewEditing(nil, EditingHost{}).Apply(ctx, EditInput{ID: "any"}); !errors.Is(err, errNoScheduleLibrary) {
		t.Fatalf("apply must refuse without a library, got %v", err)
	}

	// The lifecycle surface: remove, run and enable all refuse.
	life := NewLifecycle(nil, nil, nil)
	if _, err := life.Remove(ctx, IDInput{ID: "any"}); !errors.Is(err, errNoScheduleLibrary) {
		t.Fatalf("remove must refuse without a library, got %v", err)
	}
	if _, err := life.Run(ctx, IDInput{ID: "any"}); !errors.Is(err, errNoScheduleLibrary) {
		t.Fatalf("run must refuse without a library, got %v", err)
	}
	if _, err := life.Enable(ctx, EnableInput{ID: "any"}); !errors.Is(err, errNoScheduleLibrary) {
		t.Fatalf("enable must refuse without a library, got %v", err)
	}

	// The read surface.
	if _, err := New(nil, Host{}, nil).List(ctx, ListInput{}); !errors.Is(err, errNoScheduleLibrary) {
		t.Fatalf("list must refuse without a library, got %v", err)
	}
}

// TestTheRefusalNamesItsReason: the refusal must say WHY — a bare error would
// leave the operator guessing between an unwired daemon, an empty library and
// a missing schedule.
func TestTheRefusalNamesItsReason(t *testing.T) {
	if errNoScheduleLibrary.Error() != "schedule: the schedule library is not available on this daemon" {
		t.Fatalf("the refusal says %q", errNoScheduleLibrary.Error())
	}
}
