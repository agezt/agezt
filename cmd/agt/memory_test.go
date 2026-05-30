// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// These tests cover the daemon-free paths: flag/arg validation, help, and
// usage errors. The dial-and-call paths are exercised by the control-plane
// integration tests.

func TestCmdMemory_RequiresSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdMemory(nil, &out, &errOut); code != 2 {
		t.Fatalf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "subcommand required") {
		t.Errorf("stderr should explain requirement; got %q", errOut.String())
	}
}

func TestCmdMemory_HelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdMemory([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "memory <subcommand>") {
		t.Errorf("help missing usage; got %q", out.String())
	}
}

func TestCmdMemory_RejectsUnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdMemory([]string{"flibber"}, &out, &errOut); code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
}

func TestCmdMemoryAdd_HelpAndTypes(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdMemoryAdd([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "PREFERENCE") {
		t.Errorf("add --help should list types; got %q", out.String())
	}
}

func TestCmdMemoryAdd_RequiresContent(t *testing.T) {
	var out, errOut bytes.Buffer
	// Zero positionals → usage error, exit 2, before any dial.
	if code := cmdMemoryAdd([]string{"--type", "FACT"}, &out, &errOut); code != 2 {
		t.Fatalf("exit=%d want 2; stderr=%q", code, errOut.String())
	}
}

func TestCmdMemoryAdd_FlagNeedsValue(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdMemoryAdd([]string{"subject", "content", "--type"}, &out, &errOut); code != 2 {
		t.Errorf("dangling --type should be exit 2, got %d", code)
	}
	out.Reset()
	errOut.Reset()
	if code := cmdMemoryAdd([]string{"s", "c", "--conf", "notanumber"}, &out, &errOut); code != 2 {
		t.Errorf("bad --conf should be exit 2, got %d", code)
	}
	out.Reset()
	errOut.Reset()
	if code := cmdMemoryAdd([]string{"s", "c", "--tag", "novalue"}, &out, &errOut); code != 2 {
		t.Errorf("malformed --tag should be exit 2, got %d", code)
	}
}

func TestCmdMemoryAdd_TooManyPositionals(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdMemoryAdd([]string{"a", "b", "c"}, &out, &errOut); code != 2 {
		t.Errorf("three positionals should be exit 2, got %d", code)
	}
}

func TestCmdMemorySearch_RequiresQuery(t *testing.T) {
	var out, errOut bytes.Buffer
	// Only flags, no query term → exit 2 before dialing.
	if code := cmdMemorySearch([]string{"--json"}, &out, &errOut); code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
}

func TestCmdMemoryGet_RequiresID(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdMemoryGet(nil, &out, &errOut); code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
}

func TestCmdMemoryForget_RequiresID(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdMemoryForget([]string{"--json"}, &out, &errOut); code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
}

func TestRenderRecordLine(t *testing.T) {
	line := renderRecordLine(map[string]any{
		"id": "abcdef0123456789", "type": "FACT", "subject": "proj", "content": "hello",
	})
	if !strings.Contains(line, "abcdef012345") || !strings.Contains(line, "[FACT]") || !strings.Contains(line, "proj: hello") {
		t.Errorf("unexpected line: %q", line)
	}
	// No subject → no trailing colon.
	bare := renderRecordLine(map[string]any{"id": "x", "type": "FACT", "content": "c"})
	if strings.Contains(bare, ":") {
		t.Errorf("subjectless record should not render a colon: %q", bare)
	}
}
