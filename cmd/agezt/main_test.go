// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
)

func TestRunVersion(t *testing.T) {
	for _, flag := range []string{"-v", "--version", "version"} {
		var out, errOut bytes.Buffer
		code := run([]string{flag}, &out, &errOut)
		if code != 0 {
			t.Errorf("%s: exit=%d want 0; stderr=%q", flag, code, errOut.String())
		}
		if !strings.Contains(out.String(), brand.Version) {
			t.Errorf("%s: stdout missing version %q; got %q", flag, brand.Version, out.String())
		}
		if !strings.Contains(out.String(), brand.Binary) {
			t.Errorf("%s: stdout missing binary name %q; got %q", flag, brand.Binary, out.String())
		}
	}
}

func TestRunHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "usage:") {
		t.Errorf("help missing 'usage:'; got %q", out.String())
	}
	if !strings.Contains(out.String(), "ANTHROPIC_API_KEY") {
		t.Errorf("help missing ANTHROPIC_API_KEY note; got %q", out.String())
	}
}

func TestRunUnknown(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"bogus"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Errorf("stderr missing error; got %q", errOut.String())
	}
}

// Note: runDaemon needs a real ANTHROPIC_API_KEY to start, so we don't
// exercise it here. The end-to-end test under kernel/controlplane covers
// the same wire format with a mock provider.
