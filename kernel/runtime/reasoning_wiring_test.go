// SPDX-License-Identifier: MIT

package runtime

import (
	"testing"

	"github.com/agezt/agezt/kernel/modules/reasoning"
)

// wireReasoning injects the reasoning module into a kernel the test opened.
//
// The kernel cannot import the module itself — a module may import another
// module's api package and nothing else (archcheck's principle P5) — so the
// composition root builds the service over the kernel's own ports. In
// production that root is the daemon (cmd/agezt); in a test it is the test.
func wireReasoning(t *testing.T, k *Kernel) *Kernel {
	t.Helper()
	k.SetReasoning(reasoning.New(k.ReasoningDependencies()))
	return k
}
