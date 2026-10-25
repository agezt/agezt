// SPDX-License-Identifier: MIT

package taste

import (
	"context"
	"errors"
	"testing"
)

// TestTheAppLayerRefusesWithoutALibrary: the exemplar library is the DAEMON's
// (W3.52). A host that was given none holds a nil port, and the WRITE
// surfaces must REFUSE — not panic, and not answer "nothing to do".
func TestTheAppLayerRefusesWithoutALibrary(t *testing.T) {
	ctx := context.Background()
	svc := New(nil)

	// The create surface.
	if _, err := svc.Create(ctx, CreateInput{Title: "t", Body: "b"}); !errors.Is(err, errNoExemplarLibrary) {
		t.Fatalf("create must refuse without a library, got %v", err)
	}
	// The delete surface.
	if _, err := svc.Delete(ctx, DeleteInput{ID: "e"}); !errors.Is(err, errNoExemplarLibrary) {
		t.Fatalf("delete must refuse without a library, got %v", err)
	}
}

// TestTheReadSurfaceAnswersEmptyWithoutALibrary: the READ surface answers the
// empty list — "no exemplars" is an honest answer, and a nil list would make
// every reader a potential panic.
func TestTheReadSurfaceAnswersEmptyWithoutALibrary(t *testing.T) {
	out, err := New(nil).List(context.Background(), ListInput{})
	if err != nil {
		t.Fatalf("list must answer without a library, got %v", err)
	}
	if out.Count != 0 || len(out.Exemplars) != 0 {
		t.Fatalf("list must answer the empty list, got %+v", out)
	}
	if out.Exemplars == nil {
		t.Fatal("the empty list must be present, not nil — the wire shape is a JSON array")
	}
}

// TestTheRefusalNamesItsReason: the refusal must say WHY — a bare error would
// leave the operator guessing between an unwired daemon, an empty library and
// a failed write.
func TestTheRefusalNamesItsReason(t *testing.T) {
	if errNoExemplarLibrary.Error() != "taste: the exemplar library is not available on this daemon" {
		t.Fatalf("the refusal says %q", errNoExemplarLibrary.Error())
	}
}
