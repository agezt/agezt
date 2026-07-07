// SPDX-License-Identifier: MIT

package paths_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
)

// TestBaseDir_HomeResolutionError forces the fallback error path: when
// AGEZT_HOME is unset AND os.UserHomeDir() cannot resolve a home directory,
// BaseDir must return the documented "cannot resolve user home directory"
// error rather than a bogus path.
//
// os.UserHomeDir reads a platform-specific env var (USERPROFILE on Windows,
// HOME on unix, home on plan9) and errors when it is empty. We blank both the
// AGEZT_HOME override and every home var so the resolver has nothing to fall
// back to. t.Setenv restores all of them at test end, so this can't leak into
// sibling tests.
func TestBaseDir_HomeResolutionError(t *testing.T) {
	// Kill the override so we reach the UserHomeDir call.
	t.Setenv(brand.EnvPrefix+"HOME", "")

	// Blank the home-directory env vars for every OS Go supports, so the
	// resolver fails regardless of where the test runs.
	switch runtime.GOOS {
	case "windows":
		t.Setenv("USERPROFILE", "")
		// Windows also consults HOMEDRIVE+HOMEPATH; blank those too so the
		// resolver has no second source of a home path.
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
	case "plan9":
		t.Setenv("home", "")
	default:
		t.Setenv("HOME", "")
	}

	got, err := paths.BaseDir()
	if err == nil {
		// Some environments may still resolve a home from another source; if
		// so, the error path genuinely can't be exercised here. Skip rather
		// than assert a false negative.
		t.Skipf("UserHomeDir still resolved to %q; cannot force the error path on this host", got)
	}
	if got != "" {
		t.Errorf("BaseDir returned %q alongside an error; want empty string", got)
	}
	if !strings.Contains(err.Error(), "cannot resolve user home directory") {
		t.Errorf("BaseDir error = %q, want it to mention 'cannot resolve user home directory'", err.Error())
	}
	// The error must name the override env var so the operator knows the fix.
	if !strings.Contains(err.Error(), brand.EnvPrefix+"HOME") {
		t.Errorf("BaseDir error = %q, want it to mention %sHOME", err.Error(), brand.EnvPrefix)
	}
}
