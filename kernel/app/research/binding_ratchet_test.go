// SPDX-License-Identifier: MIT

package research

// The research application's REASONING BINDING — the ratchet of W3.76v.
//
// The research harness is the REASONING MODULE's operation: the daemon used
// to hold a `Research` forwarder over it, and the research application took
// the capability from the daemon's handle. Every research run went
// application -> daemon -> module.
//
// Now the application binds the MODULE — the face the daemon hands over
// (k.ReasoningService()) — and the daemon spells none of the operations.
//
// The package still names the daemon for the type it fills its engines from
// (the kernel handle the composition root hands over), so this ratchet pins
// the REASONING's binding: the capability comes from the module's face, not
// from a daemon verb.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: taking the research capability from the
// daemon has to make this file fail.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// daemonAsResearch is the binding this package must not carry any more: the
// research capability taken from the daemon's handle.
var daemonAsResearch = regexp.MustCompile(`\bk\.(Council|Conduct|Research)\s*\(`)

// TestTheResearchApplicationBindsTheReasoningModuleNotTheDaemon pins the
// reasoning binding: the application asks the MODULE.
func TestTheResearchApplicationBindsTheReasoningModuleNotTheDaemon(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the research application package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if daemonAsResearch.Match(src) {
			t.Errorf("%s takes the reasoning capability from the daemon — the research application binds the reasoning module's face, not the host (W3.76v)", e.Name())
		}
	}
}
