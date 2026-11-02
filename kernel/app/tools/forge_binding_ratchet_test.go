// SPDX-License-Identifier: MIT

package tools

// The tools application's FORGE BINDING — the ratchet of W3.76t.
//
// The application layer used to bind the DAEMON for the forge: the tools
// package's ForgeWriter port was the *runtime.Kernel itself, and the daemon
// held SEVEN forwarders over the extensions module's own operations
// (DraftScriptTool, UpdateScriptTool, TestScriptTool, PromoteScriptTool,
// RequestToolPromotion, QuarantineScriptTool and RemoveScriptTool). Every
// forge write went application -> daemon -> module.
//
// Now the application binds the MODULE's runner — the face the daemon hands
// over (k.ForgeRunner()) — and the daemon spells none of the operations.
//
// The package still names the daemon for the OTHER families it owns (the tool
// registry, the MCP registrations and attachments, the tool invocation), so
// this ratchet is narrower than the workboard's: it pins the FORGE's binding,
// not the whole package's. Binding the forge's writer to the daemon again
// would put the daemon back between the application and the module — and the
// daemon would have to grow the seven verbs to answer.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: binding the daemon as the forge writer
// has to make this file fail.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// daemonAsForgeWriter is the binding this package must not carry any more:
// the forge's writer port bound to the daemon's handle.
var daemonAsForgeWriter = regexp.MustCompile(`ForgeWriter:\s*k\s*[,}]`)

// TestTheToolsApplicationBindsTheForgeModuleNotTheDaemon pins the forge's
// binding: the application asks the MODULE.
func TestTheToolsApplicationBindsTheForgeModuleNotTheDaemon(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the tools application package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if daemonAsForgeWriter.Match(src) {
			t.Errorf("%s binds the daemon as the forge writer — the tools application binds the extensions module's runner, not the host (W3.76t)", e.Name())
		}
	}
}
