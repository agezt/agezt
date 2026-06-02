// SPDX-License-Identifier: MIT

package warden_test

// Environment-scoping tests (M186): a nil Spec.Env must give the child
// an EMPTY environment (the documented "most restrictive" default), not
// inherit the daemon's — otherwise secrets in the daemon env leak into
// untrusted children.

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/warden"
)

// echoEnvArgv prints the value of env var name (empty/literal if unset),
// portable across Linux/macOS/Windows.
func echoEnvArgv(name string) []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/C", "echo %" + name + "%"}
	}
	return []string{"sh", "-c", "echo $" + name}
}

func TestRun_NilEnvDoesNotInheritParentEnv(t *testing.T) {
	const key = "AGEZT_WARDEN_LEAK_PROBE"
	const val = "super-secret-sentinel-d9f1a2"
	t.Setenv(key, val) // present in THIS (parent) process

	e := warden.New(nil)
	res, err := e.Run(context.Background(), warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    echoEnvArgv(key),
		Env:     nil, // documented: empty / most restrictive
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(string(res.Stdout), val) {
		t.Errorf("nil Env leaked parent secret into child: stdout=%q", res.Stdout)
	}
}

// Positive control: an explicit Env reaches the child (proves the echo
// mechanism actually surfaces a present var, so the leak test's "absent"
// is meaningful — and guards against regressing explicit pass-through).
func TestRun_ExplicitEnvIsPassedThrough(t *testing.T) {
	const key = "AGEZT_WARDEN_VISIBLE"
	const val = "explicitly-passed-7c3a"
	// Full inherited env PLUS the sentinel, so the child (cmd.exe on
	// Windows) starts cleanly and we prove explicit env reaches it.
	env := append(os.Environ(), key+"="+val)

	e := warden.New(nil)
	res, err := e.Run(context.Background(), warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    echoEnvArgv(key),
		Env:     env,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(string(res.Stdout), val) {
		t.Errorf("explicit Env not visible to child: stdout=%q", res.Stdout)
	}
}
