// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
)

func TestCmdProviderSetup_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdProviderSetup([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "provider setup") {
		t.Errorf("help missing usage; got %q", out.String())
	}
}

func TestFirstModelID(t *testing.T) {
	p := &catalog.Provider{Models: map[string]*catalog.Model{
		"MiniMax-M2.7": {ID: "MiniMax-M2.7"},
		"MiniMax-M2":   {ID: "MiniMax-M2"},
	}}
	if got := firstModelID(p); got != "MiniMax-M2" {
		t.Errorf("firstModelID = %q want MiniMax-M2 (alphabetically smallest)", got)
	}
	if got := firstModelID(&catalog.Provider{}); got != "<model-id>" {
		t.Errorf("empty provider → %q want <model-id>", got)
	}
}

func TestSuggestProviders(t *testing.T) {
	cat := &catalog.Catalog{Providers: map[string]*catalog.Provider{
		"minimax":             {ID: "minimax"},
		"minimax-coding-plan": {ID: "minimax-coding-plan"},
		"openai":              {ID: "openai"},
	}}
	sugg := suggestProviders(cat, "minmax-typo") // contains? no — substring match is on "minmax"
	if len(sugg) != 0 {
		t.Errorf("non-substring query should match nothing, got %v", sugg)
	}
	sugg = suggestProviders(cat, "minimax")
	if len(sugg) != 2 || sugg[0] != "minimax" {
		t.Errorf("substring 'minimax' → %v want [minimax minimax-coding-plan]", sugg)
	}
}

func TestListProvidersNeedingKeys(t *testing.T) {
	cat := &catalog.Catalog{Providers: map[string]*catalog.Provider{
		// keyed + unconfigured
		"minimax-coding-plan": {ID: "minimax-coding-plan", NPM: "@ai-sdk/anthropic", Env: []string{"MINIMAX_API_KEY"}},
		// keyed + configured
		"openai": {ID: "openai", NPM: "@ai-sdk/openai", Env: []string{"OPENAI_API_KEY"}},
		// keyless (local) — must be omitted entirely
		"ollama-local": {ID: "ollama-local", NPM: "@ai-sdk/ollama"},
	}}
	store := creds.NewStore(t.TempDir())
	_ = store.Set("OPENAI_API_KEY", "sk-configured")

	var out bytes.Buffer
	if code := listProvidersNeedingKeys(cat, store, &out); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	s := out.String()
	if !strings.Contains(s, "1 ready, 1 unconfigured") {
		t.Errorf("summary wrong; got:\n%s", s)
	}
	if !strings.Contains(s, "minimax-coding-plan") || !strings.Contains(s, "MINIMAX_API_KEY") {
		t.Errorf("unconfigured provider not surfaced; got:\n%s", s)
	}
	if strings.Contains(s, "ollama-local") {
		t.Errorf("keyless provider should be omitted; got:\n%s", s)
	}
}
