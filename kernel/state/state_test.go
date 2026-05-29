// SPDX-License-Identifier: MIT

package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) (*FileStore, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir
}

func TestSetGet(t *testing.T) {
	s, _ := openTest(t)
	if err := s.Set("agents", "01H", map[string]string{"status": "running"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok, err := s.Get("agents", "01H")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("expected key present")
	}
	var into map[string]string
	if err := json.Unmarshal(got, &into); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	if into["status"] != "running" {
		t.Errorf("status=%q want %q", into["status"], "running")
	}
}

func TestGet_Absent(t *testing.T) {
	s, _ := openTest(t)
	v, ok, err := s.Get("agents", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if ok || v != nil {
		t.Errorf("absent → got ok=%v v=%s, want false/nil", ok, v)
	}
}

func TestGet_ReturnsCopy(t *testing.T) {
	s, _ := openTest(t)
	if err := s.Set("ns", "k", "v"); err != nil {
		t.Fatal(err)
	}
	a, _, _ := s.Get("ns", "k")
	a[0] = 'X' // tamper the returned slice
	b, _, _ := s.Get("ns", "k")
	if bytes.Equal(a, b) {
		t.Error("mutating returned slice changed the store; Get must return a copy")
	}
}

func TestDelete(t *testing.T) {
	s, _ := openTest(t)
	if err := s.Set("ns", "k", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("ns", "k"); err != nil {
		t.Fatal(err)
	}
	_, ok, _ := s.Get("ns", "k")
	if ok {
		t.Error("Delete failed; key still present")
	}
	// Deleting an absent key is a no-op.
	if err := s.Delete("ns", "k"); err != nil {
		t.Errorf("Delete absent: %v", err)
	}
}

func TestKeys_Sorted(t *testing.T) {
	s, _ := openTest(t)
	for _, k := range []string{"c", "a", "b"} {
		if err := s.Set("ns", k, k); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := s.Keys("ns")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c"}
	if len(keys) != len(want) {
		t.Fatalf("got %v, want %v", keys, want)
	}
	for i, k := range want {
		if keys[i] != k {
			t.Errorf("keys[%d]=%q want %q", i, keys[i], k)
		}
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	s1, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.Set("agents", "01H", map[string]int{"n": 42}); err != nil {
		t.Fatal(err)
	}
	if err := s1.Set("config", "lang", "tr"); err != nil {
		t.Fatal(err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen → values must survive.
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	v, ok, err := s2.Get("agents", "01H")
	if err != nil || !ok {
		t.Fatalf("Get after reopen: ok=%v err=%v", ok, err)
	}
	var m map[string]int
	if err := json.Unmarshal(v, &m); err != nil {
		t.Fatal(err)
	}
	if m["n"] != 42 {
		t.Errorf("n=%d want 42", m["n"])
	}

	v2, ok, _ := s2.Get("config", "lang")
	if !ok || string(v2) != `"tr"` {
		t.Errorf("config.lang = %s ok=%v want \"tr\"", v2, ok)
	}
}

func TestSnapshot_AtomicOnDisk(t *testing.T) {
	s, dir := openTest(t)
	if err := s.Set("ns", "k", "v"); err != nil {
		t.Fatal(err)
	}
	// File should be present, no leftover .tmp.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var nsFile, tmpFile bool
	for _, e := range entries {
		switch e.Name() {
		case "ns.json":
			nsFile = true
		case "ns.json.tmp":
			tmpFile = true
		}
	}
	if !nsFile {
		t.Error("ns.json missing after Set")
	}
	if tmpFile {
		t.Error("ns.json.tmp leftover; atomicWrite did not rename")
	}
}

func TestEmptyNamespace_FileRemoved(t *testing.T) {
	s, dir := openTest(t)
	if err := s.Set("ns", "k", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("ns", "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ns.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("empty namespace file should be removed; got err=%v", err)
	}
}

func TestValidateNamespace(t *testing.T) {
	s, _ := openTest(t)
	bad := []string{"", ".", "..", "with/slash", "back\\slash", "col:on", "do$lar"}
	for _, ns := range bad {
		if err := s.Set(ns, "k", "v"); err == nil || !errors.Is(err, ErrInvalidNamespace) {
			t.Errorf("Set(%q) got err=%v, want ErrInvalidNamespace", ns, err)
		}
	}
	good := []string{"agents", "config", "agent_01H", "ns-with-dash", "ns.with.dot"}
	for _, ns := range good {
		if err := s.Set(ns, "k", "v"); err != nil {
			t.Errorf("Set(%q) unexpected err=%v", ns, err)
		}
	}
}

func TestRawMessage_Passthrough(t *testing.T) {
	s, _ := openTest(t)
	raw := json.RawMessage(`{"already":"json"}`)
	if err := s.Set("ns", "k", raw); err != nil {
		t.Fatal(err)
	}
	got, _, _ := s.Get("ns", "k")
	if string(got) != string(raw) {
		t.Errorf("RawMessage not stored as-is: got %s want %s", got, raw)
	}
}
