// SPDX-License-Identifier: MIT

package basedir

import (
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The data root's ports are filled by the APPLICATION from the kernel handle
// — the host never binds them itself.
//
// The test pins what that wiring buys: the root the port reports is the root
// the daemon keeps its files under. A port bound to another kernel would leave
// the operator looking at a directory the daemon does not write to.
func TestTheBaseDirPortsReachTheKernelRoot(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.Dir() != dir {
		t.Fatalf("the data-root port reports %q, the daemon keeps its files under %q", ports.Dir(), dir)
	}
}

// A host with no kernel has no data root.
//
// The empty root is the answer a host without a daemon has: there is no
// directory to report, and nothing is invented.
func TestTheBaseDirPortsSurviveAHostWithNoKernel(t *testing.T) {
	ports := PortsFromKernel(nil)
	if ports.Dir() != "" {
		t.Fatalf("a host with no kernel reports the data root %q", ports.Dir())
	}
}
