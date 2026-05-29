// SPDX-License-Identifier: MIT

package paths_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ersinkoc/agezt/internal/brand"
	"github.com/ersinkoc/agezt/internal/paths"
)

// TestBaseDir_HonoursEnvOverride proves the AGEZT_HOME env var
// shortcuts everything else. Critical for tests, sandboxes, and
// the daemon-in-container use case where home isn't writable.
func TestBaseDir_HonoursEnvOverride(t *testing.T) {
	want := "/tmp/agezt-test-override"
	t.Setenv(brand.EnvPrefix+"HOME", want)

	got, err := paths.BaseDir()
	if err != nil {
		t.Fatalf("BaseDir: %v", err)
	}
	if got != want {
		t.Errorf("BaseDir = %q, want %q", got, want)
	}
}

// TestBaseDir_FallsBackToUserHome verifies the no-override path
// constructs `<home>/.agezt` (or whatever brand.ConfigDir is).
// Doesn't pin the exact home path — that's OS-specific — but
// proves the result ends with the configured subdir.
func TestBaseDir_FallsBackToUserHome(t *testing.T) {
	t.Setenv(brand.EnvPrefix+"HOME", "")

	got, err := paths.BaseDir()
	if err != nil {
		// On some CI environments os.UserHomeDir returns an error
		// (no HOME, no USERPROFILE). Skip rather than fail — that's
		// a known portability hole BaseDir explicitly documents.
		t.Skipf("UserHomeDir unavailable: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(got), "/"+brand.ConfigDir) {
		t.Errorf("BaseDir = %q, want ending with /%s", got, brand.ConfigDir)
	}
}

// TestBaseDir_EnvOverrideWinsOverHome ensures the env var takes
// precedence even when UserHomeDir would succeed. Regression
// guard against accidentally reordering the conditions.
func TestBaseDir_EnvOverrideWinsOverHome(t *testing.T) {
	want := "/explicit/override/path"
	t.Setenv(brand.EnvPrefix+"HOME", want)

	got, err := paths.BaseDir()
	if err != nil {
		t.Fatalf("BaseDir: %v", err)
	}
	if got != want {
		t.Errorf("BaseDir = %q, want %q (env should win)", got, want)
	}
	// Sanity: the result must NOT contain the default subdir name
	// when override is set — that would mean we appended.
	if strings.Contains(got, brand.ConfigDir) {
		t.Errorf("BaseDir = %q contains default ConfigDir %q — env override should be used verbatim",
			got, brand.ConfigDir)
	}
}
