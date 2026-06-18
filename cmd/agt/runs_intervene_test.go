// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCmdRunsIntervene_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdRunsIntervene([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "halt|abort|redirect|adjust|query") {
		t.Fatalf("help missing primitive grammar: %q", out.String())
	}
}

func TestCmdRunsIntervene_RejectsBadLeaseBeforeDial(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdRunsIntervene([]string{"halt", "run-1", "--lease", "soon"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "bad --lease") {
		t.Fatalf("stderr=%q", errOut.String())
	}
}
