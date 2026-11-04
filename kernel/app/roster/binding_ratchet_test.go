// SPDX-License-Identifier: MIT

package roster

// The roster application's BINDING — the ratchet of W3.76y.
//
// The roster's lifecycle is the AGENTS MODULE's: the module keeps the profile
// rules, the events and the guardian rule, and the daemon hands over the
// module's FACE (k.RosterService()).
//
// The application used to reach the DAEMON for its writes — the daemon held
// five forwarders over the module's own operations (AddProfile,
// SetProfileEnabled, SetProfileRetired, UpdateProfile, RemoveProfile) and
// the application called them. Those five were a THIRD spelling of one family
// and they are gone.
//
// What is pinned here is the one thing that could still go wrong silently: a
// write straight into the roster's STORE would plant a profile without
// journalling its transition. The module's lifecycle owns the writes, and the
// journal with them.
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

// writesThroughTheFilePort is a write straight into the roster's store: the
// file port answers the ROWS, and the lifecycle answers the WRITES.
//
// The store reaches the code in two shapes — bound to a name (`store.Add(…)`)
// and reached through the daemon's port (`k.Roster().Add(…)`) — so the
// ratchet knows both. A write through EITHER shape skips the module's
// lifecycle, and with it the journal.
var writesThroughTheFilePort = regexp.MustCompile(`(?:k\.Roster\(\)|store)\s*\.\s*(Add|SetEnabled|SetRetired|Update|Remove)\s*\(`)

// TestTheRosterApplicationWritesThroughTheLibrary pins the binding: the
// application's writes go through the module's lifecycle, not around it.
func TestTheRosterApplicationWritesThroughTheLibrary(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the roster application package: %v", err)
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
			t.Errorf("%s writes through the daemon's file port — the roster's lifecycle owns the writes, and the journal with them (W3.76y)", e.Name())
		}
	}
}
