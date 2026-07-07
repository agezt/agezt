// SPDX-License-Identifier: MIT

package introspecttool

import "testing"

func TestIntrospectCoverageNewKernelSource(t *testing.T) {
	// NewKernelSource returns a Source that the introspect tool binds to;
	// verifying the constructor is enough to cover it without standing up
	// a real kernel (the rest of kernelsource.go is held under a nil
	// kernel and is exercised by the integration-style tests in
	// introspect_test.go).
	if NewKernelSource(nil) == nil {
		t.Fatal("NewKernelSource should return a Source even with a nil kernel")
	}
}
