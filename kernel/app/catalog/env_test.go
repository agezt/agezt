// SPDX-License-Identifier: MIT

package catalog

import (
	"os"
	"testing"
)

// TestEnvOrDefault exercises both the set (non-empty) and unset/empty fallback
// branches of envOrDefault.
func TestEnvOrDefault(t *testing.T) {
	const name = "AGEZT_HEAVYCOV_ENV_PROBE"

	t.Setenv(name, "custom-value")
	if got := envOrDefault(name, "fallback"); got != "custom-value" {
		t.Errorf("envOrDefault set = %q, want %q", got, "custom-value")
	}

	// Empty value → fallback.
	t.Setenv(name, "")
	if got := envOrDefault(name, "fallback"); got != "fallback" {
		t.Errorf("envOrDefault empty = %q, want fallback", got)
	}

	// Unset entirely → fallback.
	os.Unsetenv(name)
	if got := envOrDefault(name, "fallback2"); got != "fallback2" {
		t.Errorf("envOrDefault unset = %q, want fallback2", got)
	}
}
