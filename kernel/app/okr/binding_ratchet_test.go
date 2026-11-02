// SPDX-License-Identifier: MIT

package okr

// The objective application's BINDING — the ratchet of W3.76r.
//
// The application layer used to bind the DAEMON: kernel/app/okr's ports took
// a *runtime.Kernel and asked it for six verbs that were themselves forwarders
// over the work module's own operations. Every objective write went
// application -> daemon -> module, and the application package imported
// kernel/runtime to say so.
//
// Now the application binds the MODULE: the ports take the work module's
// ObjectiveService and the contract's store port, and the package does not
// import the daemon at all.
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

// TestTheObjectiveApplicationDoesNotBindTheDaemon pins the binding: the
// application asks the MODULE.
func TestTheObjectiveApplicationDoesNotBindTheDaemon(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the objective application package: %v", err)
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
			t.Errorf("%s imports the daemon — the objective application binds the work module, not the host (W3.76r)", e.Name())
		}
	}
}
