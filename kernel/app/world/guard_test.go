// SPDX-License-Identifier: MIT

package world

import (
	"context"
	"errors"
	"testing"
)

// TestTheAppLayerRefusesWithoutAGraph: the world model is the DAEMON's
// (W3.54). A host that was given none holds a nil port, and the app layer must
// REFUSE — not panic, and not answer "nothing to do".
func TestTheAppLayerRefusesWithoutAGraph(t *testing.T) {
	ctx := context.Background()
	svc := New(nil)

	if _, err := svc.Add(ctx, AddInput{Name: "n"}); !errors.Is(err, errNoWorldModel) {
		t.Fatalf("add must refuse without a graph, got %v", err)
	}
	if _, err := svc.Relate(ctx, RelateInput{From: "a", To: "b"}); !errors.Is(err, errNoWorldModel) {
		t.Fatalf("relate must refuse without a graph, got %v", err)
	}
	if _, err := svc.Forget(ctx, GetInput{ID: "e-1"}); !errors.Is(err, errNoWorldModel) {
		t.Fatalf("forget must refuse without a graph, got %v", err)
	}
}

// TestTheRefusalNamesItsReason: the refusal must say WHY — a bare error would
// leave the operator guessing between an unwired daemon, an empty graph and
// an unknown id.
func TestTheRefusalNamesItsReason(t *testing.T) {
	if errNoWorldModel.Error() != "world: the world model is not available on this daemon" {
		t.Fatalf("the refusal says %q", errNoWorldModel.Error())
	}
}
