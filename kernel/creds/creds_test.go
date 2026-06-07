// SPDX-License-Identifier: MIT

package creds_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/creds"
)

func TestStore_LoadMissingFileIsEmpty(t *testing.T) {
	dir := t.TempDir()
	s := creds.NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load missing file: %v", err)
	}
	if got := s.Get("X"); got != "" {
		t.Errorf("Get on empty store=%q", got)
	}
}

func TestStore_SetSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s := creds.NewStore(dir)
	_ = s.Load()
	if err := s.Set("OPENAI_API_KEY", "sk-test-12345"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set("ANTHROPIC_API_KEY", "sk-ant-67890"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload into a fresh store.
	s2 := creds.NewStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := s2.Get("OPENAI_API_KEY"); got != "sk-test-12345" {
		t.Errorf("Get OPENAI_API_KEY=%q", got)
	}
	if got := s2.Get("ANTHROPIC_API_KEY"); got != "sk-ant-67890" {
		t.Errorf("Get ANTHROPIC_API_KEY=%q", got)
	}
}

func TestStore_SaveFilePerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits don't apply on windows")
	}
	dir := t.TempDir()
	s := creds.NewStore(dir)
	_ = s.Load()
	_ = s.Set("FOO", "bar")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "creds.json"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("file mode=%o want 0600", mode)
	}
}

func TestStore_SetEmptyRemoves(t *testing.T) {
	dir := t.TempDir()
	s := creds.NewStore(dir)
	_ = s.Load()
	_ = s.Set("X", "v")
	_ = s.Set("X", "") // shell-style unset
	if s.Has("X") {
		t.Error("X should be removed after empty set")
	}
}

func TestStore_Remove(t *testing.T) {
	dir := t.TempDir()
	s := creds.NewStore(dir)
	_ = s.Load()
	_ = s.Set("X", "v")
	if !s.Remove("X") {
		t.Error("Remove should report true when entry existed")
	}
	if s.Remove("X") {
		t.Error("Remove should report false on subsequent call")
	}
}

func TestStore_Names_Sorted(t *testing.T) {
	dir := t.TempDir()
	s := creds.NewStore(dir)
	_ = s.Load()
	for _, k := range []string{"Z", "A", "M"} {
		_ = s.Set(k, "v")
	}
	got := s.Names()
	want := []string{"A", "M", "Z"}
	if len(got) != len(want) {
		t.Fatalf("Names len=%d want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("Names[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestStore_SetRejectsEmptyOrWhitespaceName(t *testing.T) {
	dir := t.TempDir()
	s := creds.NewStore(dir)
	_ = s.Load()
	for _, bad := range []string{"", " ", "\t", "  X  "} {
		if err := s.Set(bad, "v"); err == nil {
			t.Errorf("Set(%q) should error", bad)
		}
	}
}

func TestStore_LoadMalformedJSONErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "creds.json"), []byte("not json"), 0600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s := creds.NewStore(dir)
	if err := s.Load(); err == nil {
		t.Error("Load should error on malformed JSON")
	}
}

func TestStore_LoadEmptyFileIsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "creds.json"), []byte{}, 0600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s := creds.NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load empty file: %v", err)
	}
	if len(s.Names()) != 0 {
		t.Errorf("empty file should yield empty vault; got %v", s.Names())
	}
}

func TestChainLookup_VaultBeforeEnv(t *testing.T) {
	vault := func(name string) string {
		if name == "FOO" {
			return "from-vault"
		}
		return ""
	}
	env := func(name string) string {
		if name == "FOO" {
			return "from-env"
		}
		if name == "BAR" {
			return "env-only"
		}
		return ""
	}
	lk := creds.ChainLookup(vault, env)
	if got := lk("FOO"); got != "from-vault" {
		t.Errorf("FOO=%q want from-vault (vault wins)", got)
	}
	if got := lk("BAR"); got != "env-only" {
		t.Errorf("BAR=%q want env-only (env fallback)", got)
	}
	if got := lk("MISSING"); got != "" {
		t.Errorf("MISSING=%q want empty", got)
	}
}

func TestChainLookup_SkipsNilSources(t *testing.T) {
	env := func(name string) string {
		if name == "X" {
			return "ok"
		}
		return ""
	}
	lk := creds.ChainLookup(nil, env, nil)
	if got := lk("X"); got != "ok" {
		t.Errorf("nil-skipping chain failed: %q", got)
	}
}

func TestMaskValue(t *testing.T) {
	cases := map[string]string{
		"":                "",
		"abc":             "•••",
		"12345678":        "••••••••", // exactly 8 → full mask
		"sk-ant-key-1234": "sk-a••••••1234",
		"verylongsecret":  "very••••••cret",
	}
	for in, want := range cases {
		if got := creds.MaskValue(in); got != want {
			t.Errorf("MaskValue(%q)=%q want %q", in, got, want)
		}
	}
}

func TestStore_AtomicWrite_NoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	s := creds.NewStore(dir)
	_ = s.Load()
	_ = s.Set("X", "v")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("tmp file leaked: %s", e.Name())
		}
	}
}

// TestStore_SaveUsesUniqueTemp pins M471: Save must not depend on a fixed
// "<path>.tmp" name. The fixed name was the root cause of corruption when two
// concurrent Save() calls (both under the read lock) raced on the same temp file —
// one renaming a partially-written temp while another was still writing it. A
// unique temp per Save removes the collision. We prove the fixed name is no longer
// used by occupying it with a directory: a fixed-name write would fail; a unique
// name is unaffected.
func TestStore_SaveUsesUniqueTemp(t *testing.T) {
	dir := t.TempDir()
	// Occupy the OLD fixed temp path so a fixed-name temp write cannot succeed.
	if err := os.Mkdir(filepath.Join(dir, "creds.json.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}

	s := creds.NewStore(dir)
	_ = s.Load()
	if err := s.Set("KEY", "secret-value"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save must not depend on the fixed <path>.tmp name (it is taken): %v", err)
	}

	// The vault round-trips cleanly.
	s2 := creds.NewStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := s2.Get("KEY"); got != "secret-value" {
		t.Errorf("KEY=%q want secret-value", got)
	}
	// No unique temp litter left behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".creds-") {
			t.Errorf("unique temp file leaked: %s", e.Name())
		}
	}
}
