// SPDX-License-Identifier: MIT

package workboard

// The workboard application's BINDING — the ratchet of W3.76s.
//
// The application layer used to bind the DAEMON: kernel/app/workboard's ports
// took a *runtime.Kernel and asked it for sixteen verbs that were themselves
// forwarders over the work module's own operations. Every board write went
// application -> daemon -> module, and the application package imported
// kernel/runtime to say so.
//
// Now the application binds the board's RUNNER — the module's service under
// the contract's runner port — and the package does not import the daemon at
// all.
//
// This ratchet pins the binding: the production files of this package must not
// reach for the daemon again. (The test files may: a test that opens a kernel
// to prove the daemon holds nothing is a test ABOUT the daemon.)

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// daemonImport is the import this package must not carry any more.
const daemonImport = `"github.com/agezt/agezt/kernel/runtime"`

// TestTheWorkboardApplicationDoesNotBindTheDaemon pins the binding: the
// application asks the BOARD.
func TestTheWorkboardApplicationDoesNotBindTheDaemon(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the workboard application package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if strings.Contains(string(src), daemonImport) {
			t.Errorf("%s imports the daemon — the workboard application binds the board's runner, not the host (W3.76s)", e.Name())
		}
	}
}
