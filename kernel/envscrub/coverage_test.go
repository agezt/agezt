// SPDX-License-Identifier: MIT

package envscrub

import (
	"strings"
	"testing"
)

// TestScrubbed_KeepsLCVariables covers the strings.HasPrefix(up, "LC_") branch:
// locale category variables are kept even though they aren't in the allow map.
func TestScrubbed_KeepsLCVariables(t *testing.T) {
	t.Setenv("LC_CTYPE", "en_US.UTF-8")
	joined := strings.ToUpper(strings.Join(Scrubbed(), "\n"))
	if !strings.Contains(joined, "LC_CTYPE=") {
		t.Fatalf("Scrubbed() should keep LC_* variables, got:\n%s", joined)
	}
}
