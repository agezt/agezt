// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCmdInbox_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdInbox([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "inbox [N]") {
		t.Errorf("help missing usage; got %q", out.String())
	}
}

func TestCmdInbox_RejectsBadArg(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdInbox([]string{"notanumber"}, &out, &errOut); code != 2 {
		t.Errorf("non-numeric arg should be exit 2, got %d", code)
	}
}
