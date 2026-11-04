// SPDX-License-Identifier: MIT

package standing

// The standing application's BINDING — the ratchet of W3.76x.
//
// The standing library's lifecycle is the STANDING PACKAGE's. The application
// used to reach the DAEMON for its writes: the daemon held four forwarders
// over the library's own verbs, and the application called them.
//
// Now the application binds the LIBRARY — the face the daemon hands over
// (`k.StandingService()`) — and the daemon spells none of the operations.
//
// Two things are pinned here, and both matter:
//
//   - the application does not WRITE through the daemon's FILE port. The file
//     port is the library's store, and a write over it would bypass the
//     library's lifecycle — and with it the journal. That is exactly the bug
//     W3.76x closed: the guardians' orders were planted straight into the
//     store, so their transitions never reached the journal;
//   - the application does not bind the daemon handle as its WRITER. The
//     writer is the library's face (or the refusal when the daemon holds no
//     library).
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
// The store reaches the code in two shapes — bound to a name (`store.Add(…)`,
// as the application's reader is) and reached through the daemon's port
// (`k.Standing().Add(…)`) — so the ratchet knows both. A write through
// EITHER shape skips the library's lifecycle, and with it the journal.
var writesThroughTheFilePort = regexp.MustCompile(`(?:k\.Standing\(\)|store)\s*\.\s*(Add|SetEnabled|Update|Remove)\s*\(`)

// theDaemonAsTheWriter is the binding this package must not carry: the daemon
// handle as the application's writer.
//
// The value must BE the handle — `Writer: k` — not a call on it
// (`Writer: k.StandingService()`, which is the library's face).
var theDaemonAsTheWriter = regexp.MustCompile(`Writer\s*:\s*k\s*[,}]`)

// TestTheStandingApplicationWritesThroughTheLibrary pins the binding: the
// application's writes go through the library's lifecycle, not around it.
func TestTheStandingApplicationWritesThroughTheLibrary(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the standing application package: %v", err)
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
		if theDaemonAsTheWriter.Match(src) {
			t.Errorf("%s takes the writer from the daemon — the standing application binds the library's face, not the host (W3.76x)", e.Name())
		}
	}
}
