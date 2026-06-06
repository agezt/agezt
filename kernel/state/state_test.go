// SPDX-License-Identifier: MIT

package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	good := []string{
		"agents", "config", "agent_01H", "ns-with-dash", "ns.with.dot",
		// Exercise every far edge of the allowlist char ranges (M515): a-z, A-Z, 0-9.
		// The names above only hit the low edges ('a', '0') and mid-range letters, so
		// the upper bounds (`c <= 'z'`, `c <= 'Z'`, `c <= '9'`) and the upper-range
		// lower bound (`c >= 'A'`) went unpinned — a namespace using 'z'/'Z'/'A'/'9'
		// would be wrongly rejected under a `<= → <` / `>= → >` mutation, undetected.
		"azAZ09",
	}
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

// TestValidateNamespace_EnforcedOnAllAccessors locks in that EVERY accessor —
// not just Set — rejects a path-traversal / forbidden namespace. The ns is the
// only caller-controlled component that becomes a filename (pathFor), so a read
// path (Get/Keys) or Delete that skipped the guard would be a traversal hole.
// This test fails closed if someone adds an accessor without validateNamespace.
func TestValidateNamespace_EnforcedOnAllAccessors(t *testing.T) {
	s, _ := openTest(t)
	for _, ns := range []string{"", ".", "..", "../escape", "a/b", `a\b`} {
		if _, _, err := s.Get(ns, "k"); !errors.Is(err, ErrInvalidNamespace) {
			t.Errorf("Get(%q) err=%v, want ErrInvalidNamespace", ns, err)
		}
		if err := s.Delete(ns, "k"); !errors.Is(err, ErrInvalidNamespace) {
			t.Errorf("Delete(%q) err=%v, want ErrInvalidNamespace", ns, err)
		}
		if _, err := s.Keys(ns); !errors.Is(err, ErrInvalidNamespace) {
			t.Errorf("Keys(%q) err=%v, want ErrInvalidNamespace", ns, err)
		}
		if err := s.Set(ns, "k", "v"); !errors.Is(err, ErrInvalidNamespace) {
			t.Errorf("Set(%q) err=%v, want ErrInvalidNamespace", ns, err)
		}
	}
}

// TestConcurrentAccess_RaceSafe hammers the store from many goroutines doing
// Set / Get / Delete / Keys / Namespaces across several namespaces at once.
// The FileStore is shared by the daemon's agent loop, scheduler, and planner,
// so its RWMutex is load-bearing. Under `go test -race` (CI, where cgo is
// available) this pins the absence of a data race; without the detector it
// still exercises concurrent access and confirms the store stays self-
// consistent (no panic/deadlock, every reported key remains Gettable).
func TestConcurrentAccess_RaceSafe(t *testing.T) {
	s, _ := openTest(t)
	const workers = 16
	const iters = 200
	namespaces := []string{"agents", "config", "planner", "scheduler"}

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			ns := namespaces[id%len(namespaces)]
			for i := 0; i < iters; i++ {
				key := fmt.Sprintf("k-%d-%d", id, i%8)
				if err := s.Set(ns, key, map[string]int{"v": i}); err != nil {
					t.Errorf("Set: %v", err)
					return
				}
				_, _, _ = s.Get(ns, key)
				_, _ = s.Keys(ns)
				_ = s.Namespaces()
				if i%3 == 0 {
					_ = s.Delete(ns, key)
				}
			}
		}(w)
	}
	wg.Wait()

	// Store must still be usable and self-consistent after the storm: every
	// namespace's reported keys must each be Gettable.
	for _, ns := range s.Namespaces() {
		keys, err := s.Keys(ns)
		if err != nil {
			t.Fatalf("Keys(%q) after storm: %v", ns, err)
		}
		for _, k := range keys {
			if _, ok, err := s.Get(ns, k); err != nil || !ok {
				t.Errorf("Get(%q,%q) after storm: ok=%v err=%v", ns, k, ok, err)
			}
		}
	}
}

// TestSet_InvalidRawMessageRejectedNoPoison: a Set of an invalid json.RawMessage
// must be rejected up front and must NOT poison the namespace — before the fix the
// bad entry stayed in the in-memory map, wedging every later Set to that namespace
// and making Get return invalid JSON that diverged from disk (M426).
func TestSet_InvalidRawMessageRejectedNoPoison(t *testing.T) {
	s, _ := openTest(t)
	if err := s.Set("ns", "good", map[string]int{"n": 1}); err != nil {
		t.Fatalf("seed Set: %v", err)
	}
	// Invalid pre-serialized value → rejected.
	if err := s.Set("ns", "poison", json.RawMessage("{not valid")); err == nil {
		t.Fatal("Set of an invalid json.RawMessage should error")
	}
	// The namespace must still be usable: a later Set succeeds...
	if err := s.Set("ns", "after", map[string]int{"n": 2}); err != nil {
		t.Fatalf("namespace poisoned — later Set failed: %v", err)
	}
	// ...the poison key never landed...
	if _, ok, _ := s.Get("ns", "poison"); ok {
		t.Error("rejected value must not be stored")
	}
	// ...and the good values are intact and valid JSON.
	if v, ok, _ := s.Get("ns", "good"); !ok || !json.Valid(v) {
		t.Errorf("good value lost or invalid after rejected Set: ok=%v v=%s", ok, v)
	}
}
