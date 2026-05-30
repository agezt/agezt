// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverCredentials_RecognisedAndDedup(t *testing.T) {
	recognised := map[string]string{"OPENAI_API_KEY": "openai", "ANTHROPIC_API_KEY": "anthropic"}
	sources := []credSource{
		{Label: "env", Values: map[string]string{"OPENAI_API_KEY": "sk-from-env", "RANDOM_VAR": "nope"}},
		{Label: ".env", Values: map[string]string{"OPENAI_API_KEY": "sk-from-dotenv", "ANTHROPIC_API_KEY": "sk-ant"}},
	}
	got := discoverCredentials(sources, recognised, false)

	if len(got) != 2 {
		t.Fatalf("expected 2 recognised creds, got %d: %+v", len(got), got)
	}
	// Sorted by name: ANTHROPIC first.
	if got[0].Name != "ANTHROPIC_API_KEY" || got[1].Name != "OPENAI_API_KEY" {
		t.Fatalf("unexpected order: %+v", got)
	}
	// First source wins for OPENAI (env before .env).
	if got[1].Value != "sk-from-env" || got[1].Source != "env" {
		t.Errorf("OPENAI should come from env, got value=%q source=%q", got[1].Value, got[1].Source)
	}
	// RANDOM_VAR is not recognised and not credential-shaped → dropped.
	for _, d := range got {
		if d.Name == "RANDOM_VAR" {
			t.Error("RANDOM_VAR must not be imported")
		}
	}
}

func TestDiscoverCredentials_HeuristicIncludesUnknown(t *testing.T) {
	sources := []credSource{
		{Label: "env", Values: map[string]string{
			"GROQ_API_KEY": "gsk-x",   // credential-shaped
			"HOME":         "/home/x", // not
			"MY_TOKEN":     "tok",     // credential-shaped
		}},
	}
	// No recognised names; includeUnrecognised on.
	got := discoverCredentials(sources, map[string]string{}, true)
	names := map[string]bool{}
	for _, d := range got {
		names[d.Name] = true
	}
	if !names["GROQ_API_KEY"] || !names["MY_TOKEN"] {
		t.Errorf("heuristic should catch *_API_KEY and *_TOKEN; got %+v", got)
	}
	if names["HOME"] {
		t.Error("HOME must not be treated as a credential")
	}
}

func TestDiscoverCredentials_SkipsEmptyValues(t *testing.T) {
	sources := []credSource{{Label: "env", Values: map[string]string{"OPENAI_API_KEY": "   "}}}
	got := discoverCredentials(sources, map[string]string{"OPENAI_API_KEY": "openai"}, false)
	if len(got) != 0 {
		t.Errorf("blank value must be skipped, got %+v", got)
	}
}

func TestLooksLikeCredName(t *testing.T) {
	yes := []string{"OPENAI_API_KEY", "GROQ_API_KEY", "X_API_TOKEN", "AWS_SECRET_ACCESS_KEY", "GITHUB_TOKEN", "FOO_SECRET"}
	no := []string{"home", "PATH", "EDITOR", "LANG", "my_api_key" /* lowercase */}
	for _, n := range yes {
		if !looksLikeCredName(n) {
			t.Errorf("%q should look like a credential", n)
		}
	}
	for _, n := range no {
		if looksLikeCredName(n) {
			t.Errorf("%q should NOT look like a credential", n)
		}
	}
}

func TestParseDotEnvFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	content := `# a comment
export OPENAI_API_KEY=sk-plain
ANTHROPIC_API_KEY="sk-quoted"
GROQ_API_KEY='gsk-single'

NOT_A_PAIR
EMPTY=
`
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got := parseDotEnvFile(p)
	if got["OPENAI_API_KEY"] != "sk-plain" {
		t.Errorf("export+plain = %q", got["OPENAI_API_KEY"])
	}
	if got["ANTHROPIC_API_KEY"] != "sk-quoted" {
		t.Errorf("double-quoted = %q", got["ANTHROPIC_API_KEY"])
	}
	if got["GROQ_API_KEY"] != "gsk-single" {
		t.Errorf("single-quoted = %q", got["GROQ_API_KEY"])
	}
	if _, ok := got["NOT_A_PAIR"]; ok {
		t.Error("non KEY=VALUE line must be ignored")
	}
	if got["EMPTY"] != "" {
		t.Errorf("EMPTY should be empty string, got %q", got["EMPTY"])
	}
}

func TestParseDotEnvFile_Missing(t *testing.T) {
	if got := parseDotEnvFile(filepath.Join(t.TempDir(), "nope.env")); got != nil {
		t.Errorf("missing file should yield nil, got %+v", got)
	}
}

func TestParseJSONCredFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(p, []byte(`{"openai_api_key":"sk-codex","other":"x","blank":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := parseJSONCredFile(p, map[string]string{"openai_api_key": "OPENAI_API_KEY"})
	if got["OPENAI_API_KEY"] != "sk-codex" {
		t.Errorf("json extraction = %+v", got)
	}
	if len(got) != 1 {
		t.Errorf("only mapped keys extracted, got %+v", got)
	}
}

func TestCmdProviderImport_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdProviderImport([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("help exit=%d", code)
	}
	if !strings.Contains(out.String(), "provider import") {
		t.Errorf("help text missing; got %q", out.String())
	}
}

func TestCmdProviderImport_UnknownArg(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdProviderImport([]string{"--bogus"}, &out, &errOut); code != 2 {
		t.Errorf("unknown arg should exit 2, got %d", code)
	}
}
