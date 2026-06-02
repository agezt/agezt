// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCmdSend_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdSend([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "send --channel") {
		t.Errorf("help missing usage; got %q", out.String())
	}
}

func TestCmdSend_RequiresArgs(t *testing.T) {
	// No flags at all → usage error (exit 2), before any dial.
	var out, errOut bytes.Buffer
	if code := cmdSend([]string{"hello"}, &out, &errOut); code != 2 {
		t.Errorf("text without --channel/--to should be exit 2, got %d", code)
	}
}

func TestCmdSend_ChannelNeedsValue(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdSend([]string{"--channel"}, &out, &errOut); code != 2 {
		t.Errorf("--channel with no value should be exit 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "needs a value") {
		t.Errorf("expected a 'needs a value' error, got %q", errOut.String())
	}
}

func TestCmdSend_ToNeedsValue(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdSend([]string{"--channel", "slack", "--to"}, &out, &errOut); code != 2 {
		t.Errorf("--to with no value should be exit 2, got %d", code)
	}
}
