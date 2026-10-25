// SPDX-License-Identifier: MIT

package standing

import (
	"context"
	"errors"
	"testing"
)

// TestTheAppLayerRefusesWithoutALibrary: the standing library is the DAEMON's
// file (W3.50). A host that was given none holds a nil port, and the app
// layer must REFUSE — not panic, and not answer "nothing to do".
func TestTheAppLayerRefusesWithoutALibrary(t *testing.T) {
	ctx := context.Background()

	// The read surface.
	if _, err := New(nil, nil, Host{}).List(ctx, ListInput{}); !errors.Is(err, errNoStandingLibrary) {
		t.Fatalf("list must refuse without a library, got %v", err)
	}

	// The creation surface.
	if _, err := New(nil, nil, Host{}).Add(ctx, AddInput{}); !errors.Is(err, errNoStandingLibrary) {
		t.Fatalf("add must refuse without a library, got %v", err)
	}

	// The editing surface.
	if _, err := New(nil, nil, Host{}).Edit(ctx, EditInput{ID: "any"}); !errors.Is(err, errNoStandingLibrary) {
		t.Fatalf("edit must refuse without a library, got %v", err)
	}

	// The lifecycle surface: enable and remove.
	if _, err := New(nil, nil, Host{}).SetEnabled(ctx, EnableInput{ID: "any", Enabled: true}); !errors.Is(err, errNoStandingLibrary) {
		t.Fatalf("enable must refuse without a library, got %v", err)
	}
	if _, err := New(nil, nil, Host{}).Remove(ctx, RemoveInput{ID: "any"}); !errors.Is(err, errNoStandingLibrary) {
		t.Fatalf("remove must refuse without a library, got %v", err)
	}
}

// TestTheRefusalNamesItsReason: the refusal must say WHY — a bare error would
// leave the operator guessing between an unwired daemon, an empty library and
// a missing order.
func TestTheRefusalNamesItsReason(t *testing.T) {
	if errNoStandingLibrary.Error() != "standing: the standing library is not available on this daemon" {
		t.Fatalf("the refusal says %q", errNoStandingLibrary.Error())
	}
}
