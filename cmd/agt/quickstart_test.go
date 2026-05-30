// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
)

func TestCmdQuickstart_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdQuickstart([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "quickstart") {
		t.Errorf("help missing usage; got %q", out.String())
	}
}

func TestCmdQuickstart_RejectsArgs(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdQuickstart([]string{"extra"}, &out, &errOut); code != 2 {
		t.Errorf("positional arg should be exit 2, got %d", code)
	}
}

func TestKeyedConfigured(t *testing.T) {
	cat := &catalog.Catalog{Providers: map[string]*catalog.Provider{
		"openai":       {ID: "openai", NPM: "@ai-sdk/openai", Env: []string{"OPENAI_API_KEY"}},
		"minimax":      {ID: "minimax", NPM: "@ai-sdk/anthropic", Env: []string{"MINIMAX_API_KEY"}},
		"ollama-local": {ID: "ollama-local", NPM: "@ai-sdk/ollama"}, // keyless → excluded
	}}
	store := creds.NewStore(t.TempDir())
	_ = store.Set("OPENAI_API_KEY", "sk-x")

	got := keyedConfigured(cat, store.Lookup)
	if len(got) != 1 || got[0] != "openai" {
		t.Errorf("keyedConfigured = %v want [openai] (keyless + unkeyed excluded)", got)
	}
}

func TestReadLine(t *testing.T) {
	if got := readLine(strings.NewReader("minimax-coding-plan\n")); got != "minimax-coding-plan" {
		t.Errorf("readLine = %q", got)
	}
	if got := readLine(strings.NewReader("trimmed\r\n")); got != "trimmed" {
		t.Errorf("readLine CRLF = %q", got)
	}
}
