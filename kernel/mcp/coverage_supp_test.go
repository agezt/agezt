// SPDX-License-Identifier: MIT

package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenStore_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "servers.json")
	if err := os.WriteFile(path, []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := OpenStore(dir)
	if err == nil {
		t.Fatal("OpenStore with malformed JSON should error")
	}
}

func TestStore_List(t *testing.T) {
	s := openTestStore(t)
	if out := s.List(); len(out) != 0 {
		t.Fatalf("List on empty store = %d, want 0", len(out))
	}
	_, _ = s.Add(Server{Name: "b", Command: "cmd-b"})
	_, _ = s.Add(Server{Name: "a", Command: "cmd-a"})
	out := s.List()
	if len(out) != 2 {
		t.Fatalf("List = %d, want 2", len(out))
	}
	// Sorted by creation time. "b" was added first, then "a".
	if out[0].Name != "b" || out[1].Name != "a" {
		t.Errorf("List order: got %q, want [b a]", []string{out[0].Name, out[1].Name})
	}
}

func TestStore_GetNotFound(t *testing.T) {
	s := openTestStore(t)
	if _, found := s.Get("nonexistent"); found {
		t.Error("Get(nonexistent) should return false")
	}
}

func TestStore_RemoveNotFound(t *testing.T) {
	s := openTestStore(t)
	_, ok, err := s.Remove("nonexistent")
	if err != nil || ok {
		t.Fatalf("Remove(nonexistent) = (_, %v, %v), want false, nil", ok, err)
	}
}

func TestValidate_TooManyArgs(t *testing.T) {
	args := make([]string, maxArgs+1)
	for i := range args {
		args[i] = "arg"
	}
	srv := Server{Name: "test", Command: "cmd", Args: args}
	if err := Validate(srv); err == nil {
		t.Fatal("too many args should be rejected")
	}
}

func TestValidate_TooManyEnv(t *testing.T) {
	env := make(map[string]string, maxEnv+1)
	for i := 0; i <= maxEnv; i++ {
		env["KEY_"+itoa(i)] = "v"
	}
	srv := Server{Name: "test", Command: "cmd", Env: env}
	if err := Validate(srv); err == nil {
		t.Fatal("too many env vars should be rejected")
	}
}

func TestValidate_TooManyHeaders(t *testing.T) {
	h := make(map[string]string, maxHeaders+1)
	for i := 0; i <= maxHeaders; i++ {
		h["X-Hdr-"+itoa(i)] = "v"
	}
	srv := Server{Name: "test", URL: "https://example.com", Headers: h}
	if err := Validate(srv); err == nil {
		t.Fatal("too many headers should be rejected")
	}
}

func TestValidate_TooManyToolAllow(t *testing.T) {
	ta := make([]string, maxToolAllow+1)
	for i := range ta {
		ta[i] = "tool"
	}
	srv := Server{Name: "test", Command: "cmd", ToolAllow: ta}
	if err := Validate(srv); err == nil {
		t.Fatal("too many allowed tools should be rejected")
	}
}

func TestValidate_InvalidHeaderName(t *testing.T) {
	srv := Server{Name: "test", URL: "https://example.com", Headers: map[string]string{"bad header name!": "v"}}
	if err := Validate(srv); err == nil {
		t.Fatal("invalid header name should be rejected")
	}
}

func TestValidate_InvalidEnvKey(t *testing.T) {
	srv := Server{Name: "test", Command: "cmd", Env: map[string]string{"bad-key!": "v"}}
	if err := Validate(srv); err == nil {
		t.Fatal("invalid env key should be rejected")
	}
}

func itoa(i int) string {
	return string(rune('a' + (i % 26))) + string(rune('0' + (i % 10)))
}
