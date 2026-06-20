// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestProviderChatGPTDispatch covers the arg validation that needs no daemon.
func TestProviderChatGPTDispatch(t *testing.T) {
	// No subcommand → usage (exit 2).
	var out, errb bytes.Buffer
	if code := cmdProviderChatGPT(nil, &out, &errb); code != 2 {
		t.Fatalf("no subcommand exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "subcommand required") {
		t.Fatalf("missing usage: %q", errb.String())
	}

	// Unknown subcommand → exit 2 with the valid set listed.
	out.Reset()
	errb.Reset()
	if code := cmdProviderChatGPT([]string{"frobnicate"}, &out, &errb); code != 2 {
		t.Fatalf("unknown subcommand exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "login") {
		t.Fatalf("unknown-subcommand error should list valid ones: %q", errb.String())
	}
}
