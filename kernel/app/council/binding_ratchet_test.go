// SPDX-License-Identifier: MIT

package council

// The council application's REASONING BINDING — the ratchet of W3.76v.
//
// The application layer used to bind the DAEMON for the reasoning capability:
// the council package's engines called the kernel's own `Council` and `Conduct`
// verbs, and the daemon held three forwarders over the reasoning module's own
// operations. Every council run went application -> daemon -> module.
//
// Now the application binds the MODULE — the face the daemon hands over
// (k.ReasoningService()) — and the daemon spells none of the operations. The
// module's service answers in the contract's names, so the council asks for
// `Council` and `Conduct` on the module's face, not on the daemon.
//
// The package still names the daemon for the OTHER families it owns (the
// default panel's seats and the settings store), so this ratchet is narrower
// than a whole-package one: it pins the REASONING's binding. Binding the
// daemon as the reasoning capability again would put the daemon back between
// the application and the module — and the daemon would have to grow the three
// verbs to answer.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: binding the daemon for the reasoning
// capability has to make this file fail.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// daemonAsReasoning is the binding this package must not carry any more: the
// reasoning capability taken from the daemon's handle.
var daemonAsReasoning = regexp.MustCompile(`\bk\.(Council|Conduct|Research)\s*\(`)

// TestTheCouncilApplicationBindsTheReasoningModuleNotTheDaemon pins the
// reasoning binding: the application asks the MODULE.
func TestTheCouncilApplicationBindsTheReasoningModuleNotTheDaemon(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the council application package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if daemonAsReasoning.Match(src) {
			t.Errorf("%s takes the reasoning capability from the daemon — the council application binds the reasoning module's face, not the host (W3.76v)", e.Name())
		}
	}
}
