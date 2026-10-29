// SPDX-License-Identifier: MIT

package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheAppLayerRefusesWithoutALibrary: the catalog directory and its
// syncer are the DAEMON's (W3.51). A host that was given none holds nil
// ports, and the app layer must REFUSE — not panic, and not answer "nothing
// to do".
//
// The kernel here is a real one WITHOUT a library: that is exactly the state
// a daemon is in when the catalog directory was never handed over.
func TestTheAppLayerRefusesWithoutALibrary(t *testing.T) {
	ctx := context.Background()
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })
	svc := New(PortsFromKernel(k), nil, t.TempDir())

	// The sync surface.
	if _, err := svc.Sync(ctx, SyncInput{}); !errors.Is(err, ErrNoLibrary) {
		t.Fatalf("sync must refuse without a library, got %v", err)
	}

	// The discovery surface.
	if _, err := svc.Discover(ctx, DiscoverInput{}); !errors.Is(err, ErrNoLibrary) {
		t.Fatalf("discover must refuse without a library, got %v", err)
	}
}

// TestTheRefusalNamesItsReason: the refusal must say WHY — a bare error would
// leave the operator guessing between an unwired daemon, an empty catalog and
// a failed fetch.
func TestTheRefusalNamesItsReason(t *testing.T) {
	if ErrNoLibrary.Error() != "catalog: the catalog library is not available on this daemon" {
		t.Fatalf("the refusal says %q", ErrNoLibrary.Error())
	}
}
