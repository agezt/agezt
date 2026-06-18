// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCmdConfigCenterSetHelpShowsAgentAccessFlags(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdConfigCenterSet([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0 stderr=%q", code, errOut.String())
	}
	got := out.String()
	for _, want := range []string{"--allow-agent", "--deny-agent"} {
		if !strings.Contains(got, want) {
			t.Fatalf("help missing %q:\n%s", want, got)
		}
	}
}
