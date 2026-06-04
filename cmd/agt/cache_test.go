// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdCache_ArgParsing covers the branches that return before dialing the
// daemon: --help (0 + usage), and malformed args (2). Keeps the CLI contract
// stable without needing a live daemon.
func TestCmdCache_ArgParsing(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		var out, errb bytes.Buffer
		if code := cmdCache([]string{"--help"}, &out, &errb); code != 0 {
			t.Fatalf("--help exit = %d want 0", code)
		}
		if !strings.Contains(out.String(), "usage:") || !strings.Contains(out.String(), "cache") {
			t.Errorf("usage missing: %q", out.String())
		}
	})

	t.Run("bad --since", func(t *testing.T) {
		var out, errb bytes.Buffer
		if code := cmdCache([]string{"--since", "nope"}, &out, &errb); code != 2 {
			t.Fatalf("bad --since exit = %d want 2", code)
		}
	})

	t.Run("missing --since value", func(t *testing.T) {
		var out, errb bytes.Buffer
		if code := cmdCache([]string{"--since"}, &out, &errb); code != 2 {
			t.Fatalf("dangling --since exit = %d want 2", code)
		}
	})

	t.Run("unexpected arg", func(t *testing.T) {
		var out, errb bytes.Buffer
		if code := cmdCache([]string{"--bogus"}, &out, &errb); code != 2 {
			t.Fatalf("unexpected arg exit = %d want 2", code)
		}
	})
}
