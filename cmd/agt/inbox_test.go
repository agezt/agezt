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

func TestCmdInbox_ChannelNeedsValue(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdInbox([]string{"--channel"}, &out, &errOut); code != 2 {
		t.Errorf("--channel with no value should be exit 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "needs a value") {
		t.Errorf("expected a 'needs a value' error, got %q", errOut.String())
	}
}

func TestCmdInbox_HelpMentionsChannel(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdInbox([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "--channel") {
		t.Errorf("help should document --channel; got %q", out.String())
	}
}
