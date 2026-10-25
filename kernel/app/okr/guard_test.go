// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"errors"
	"testing"
)

// TestTheReadSurfaceAnswersEmptyWithoutALibrary: the READ surface answers the
// empty list — "no objectives" is an honest answer, and a nil list would make
// every reader a potential panic.
func TestTheReadSurfaceAnswersEmptyWithoutALibrary(t *testing.T) {
	out, err := New(nil, nil).List(context.Background(), ListInput{})
	if err != nil {
		t.Fatalf("list must answer without a library, got %v", err)
	}
	if out.Count != 0 || len(out.Objectives) != 0 {
		t.Fatalf("list must answer the empty list, got %+v", out)
	}
	if out.Objectives == nil {
		t.Fatal("the empty list must be present, not nil — the wire shape is a JSON array")
	}
}

// TestShowRefusesWithoutALibrary: a NAMED objective cannot be answered from an
// empty library — "unknown objective" would blame the id for a library that
// is not there.
func TestShowRefusesWithoutALibrary(t *testing.T) {
	if _, err := New(nil, nil).Show(context.Background(), ShowInput{ID: "o-1"}); !errors.Is(err, errNoObjectiveLibrary) {
		t.Fatalf("show must refuse without a library, got %v", err)
	}
}

// TestTheRefusalNamesItsReason: the refusal must say WHY — a bare error would
// leave the operator guessing between an unwired daemon, an empty library and
// an unknown id.
func TestTheRefusalNamesItsReason(t *testing.T) {
	if errNoObjectiveLibrary.Error() != "okr: the objective library is not available on this daemon" {
		t.Fatalf("the refusal says %q", errNoObjectiveLibrary.Error())
	}
}
