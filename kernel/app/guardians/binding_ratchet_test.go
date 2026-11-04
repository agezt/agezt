// SPDX-License-Identifier: MIT

package guardians

// The guardian seeder's STANDING BINDING — the ratchet of W3.76x.
//
// The standing library's lifecycle is the STANDING PACKAGE's. The seeder used
// to reach the DAEMON for its standing writes, and the daemon used to hold
// four forwarders over the library's own verbs to answer it.
//
// Now the seeder binds the LIBRARY — the face the daemon hands over
// (`k.StandingService()`) — and the daemon spells none of the operations.
//
// What is pinned here is the one thing that could still go wrong silently: a
// write straight into the library's STORE would plant a guardian's order
// without journalling its transition. The library's lifecycle owns the
// writes, and the journal with them.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: writing through the file port has to make
// this file fail.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// writesThroughTheFilePort is a write straight into the library's store: the
// file port answers the ROWS, and the lifecycle answers the WRITES.
//
// The store reaches the code in two shapes — bound to a name (`store.Add(…)`)
// and reached through the daemon's port (`k.Standing().Add(…)`) — so the
// ratchet knows both. A write through EITHER shape skips the library's
// lifecycle, and with it the journal.
var writesThroughTheFilePort = regexp.MustCompile(`(?:k\.Standing\(\)|store)\s*\.\s*(Add|SetEnabled|Update|Remove)\s*\(`)

// TestTheGuardianSeederWritesThroughTheLibrary pins the binding: the seeder's
// standing writes go through the library's lifecycle, not around it.
func TestTheGuardianSeederWritesThroughTheLibrary(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the guardian seeder's package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if writesThroughTheFilePort.Match(src) {
			t.Errorf("%s writes through the daemon's file port — the standing library's lifecycle owns the writes, and the journal with them (W3.76x)", e.Name())
		}
	}
}
