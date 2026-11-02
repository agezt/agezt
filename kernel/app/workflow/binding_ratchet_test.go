// SPDX-License-Identifier: MIT

package workflow

// The workflow application's ENGINE BINDING — the ratchet of W3.76u.
//
// The application layer used to bind the DAEMON for the engine: the workflow
// package's Writer and Designer ports were the *runtime.Kernel itself, and the
// daemon held EIGHT forwarders over the workflows module's own operations
// (SaveWorkflow, RestoreWorkflow, SetWorkflowEnabled, RemoveWorkflow,
// RunWorkflow, TestWorkflowNode, DraftWorkflow and RefineWorkflow). Every
// engine write went application -> daemon -> module.
//
// Now the application binds the MODULE's engine — the face the daemon hands
// over (k.WorkflowEngine()) — and the daemon spells none of the operations.
//
// The package still names the daemon for the OTHER families it owns (the
// library's file, the journal, the correlation mint), so this ratchet is
// narrower than the workboard's: it pins the ENGINE's binding, not the whole
// package's import. Binding the daemon as the engine's writer again would put
// the daemon back between the application and the module — and the daemon
// would have to grow the eight verbs to answer.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: binding the daemon as the engine's
// writer has to make this file fail.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// daemonAsEngineWriter is the binding this package must not carry any more:
// the engine's writer port bound to the daemon's handle.
var daemonAsEngineWriter = regexp.MustCompile(`(Writer|Designer):\s*k\s*[,}]`)

// TestTheWorkflowApplicationBindsTheEngineModuleNotTheDaemon pins the
// engine's binding: the application asks the MODULE.
func TestTheWorkflowApplicationBindsTheEngineModuleNotTheDaemon(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the workflow application package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if daemonAsEngineWriter.Match(src) {
			t.Errorf("%s binds the daemon as the engine's writer — the workflow application binds the workflows module's engine, not the host (W3.76u)", e.Name())
		}
	}
}
