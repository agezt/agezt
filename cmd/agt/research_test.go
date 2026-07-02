// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// These exercise only the argument-parsing paths that return BEFORE dialing the
// daemon, so they need no running server.

func TestCmdResearch_NoQuestion(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdResearch(nil, &out, &errb); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage:") {
		t.Fatalf("expected usage on stderr, got %q", errb.String())
	}
}

func TestCmdResearch_Help(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdResearch([]string{"-h"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "deep-research") {
		t.Fatalf("expected help on stdout, got %q", out.String())
	}
}

func TestCmdResearch_BadFlag(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdResearch([]string{"--max-sources", "-5", "why is the sky blue"}, &out, &errb); code != 2 {
		t.Fatalf("exit = %d, want 2 for negative flag", code)
	}
	if !strings.Contains(errb.String(), "non-negative") {
		t.Fatalf("expected non-negative error, got %q", errb.String())
	}
}

func TestJSONNum100(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		{0.0, 0},
		{0.5, 50},
		{0.876, 88}, // rounds
		{1.0, 100},
		{"nope", 0},
	}
	for _, c := range cases {
		if got := jsonNum100(c.in); got != c.want {
			t.Errorf("jsonNum100(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
