// SPDX-License-Identifier: MIT

package creds_test

import (
	"bytes"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/creds"
)

// open returns a loaded Store over dir using passphrase pass ("" = plaintext).
func open(t *testing.T, dir, pass string) *creds.Store {
	t.Helper()
	s := creds.NewStore(dir)
	s.SetPassphraseFn(func() string { return pass })
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

func mustSet(t *testing.T, s *creds.Store, name, value string) {
	t.Helper()
	if err := s.Set(name, value); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save %s: %v", name, err)
	}
}

// TestSave_ConcurrentWritersKeepEachOthersKeys is the lost update this fixes:
// the daemon loads the vault at boot, `agt provider creds set` then adds a key
// from another process, and the daemon's next save (a key set in the console)
// used to write its stale whole map back — deleting the CLI's key.
func TestSave_ConcurrentWritersKeepEachOthersKeys(t *testing.T) {
	for _, pass := range []string{"", "pw"} {
		dir := t.TempDir()
		daemon := open(t, dir, pass)
		cli := open(t, dir, pass)

		mustSet(t, cli, "OPENAI_API_KEY", "from-agt")
		mustSet(t, daemon, "ANTHROPIC_API_KEY", "from-console")

		final := open(t, dir, pass)
		if final.Get("OPENAI_API_KEY") != "from-agt" || final.Get("ANTHROPIC_API_KEY") != "from-console" {
			t.Fatalf("pass=%q: vault lost a writer's key: names=%v", pass, final.Names())
		}
		// The saving Store also picks up the other writer's key.
		if daemon.Get("OPENAI_API_KEY") != "from-agt" {
			t.Errorf("pass=%q: saving Store did not adopt the merged view", pass)
		}
	}
}

// TestSave_RemoveOnlyRemovesWhatThisStoreRemoved: a removal is applied as a
// removal, not as "write my map", so it neither resurrects nor drops others.
func TestSave_RemoveOnlyRemovesWhatThisStoreRemoved(t *testing.T) {
	dir := t.TempDir()
	seed := open(t, dir, "")
	mustSet(t, seed, "A", "1")
	daemon := open(t, dir, "")
	cli := open(t, dir, "")

	mustSet(t, cli, "B", "2")
	daemon.Remove("A")
	if err := daemon.Save(); err != nil {
		t.Fatal(err)
	}
	final := open(t, dir, "")
	if final.Has("A") || final.Get("B") != "2" {
		t.Fatalf("names=%v, want only B", final.Names())
	}
}

// TestSave_ParallelGoroutines: many writers, each adding its own key, all land.
func TestSave_ParallelGoroutines(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	names := []string{"K_A", "K_B", "K_C", "K_D", "K_E", "K_F", "K_G", "K_H"}
	for _, n := range names {
		s := open(t, dir, "")
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Set(n, "v"); err != nil {
				t.Error(err)
				return
			}
			if err := s.Save(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got := open(t, dir, "").Names(); len(got) != len(names) {
		t.Fatalf("names=%v, want all %d", got, len(names))
	}
}

// TestSave_UnloadedStoreDoesNotClobber: a Store that never loaded used to save
// only its own keys, wiping the rest of the vault.
func TestSave_UnloadedStoreDoesNotClobber(t *testing.T) {
	dir := t.TempDir()
	mustSet(t, open(t, dir, ""), "EXISTING", "keep")
	fresh := creds.NewStore(dir)
	fresh.SetPassphraseFn(func() string { return "" })
	mustSet(t, fresh, "NEW", "added")
	final := open(t, dir, "")
	if final.Get("EXISTING") != "keep" || final.Get("NEW") != "added" {
		t.Fatalf("names=%v", final.Names())
	}
}

// TestSave_RefusesToOverwriteUnreadableVault: a Store that cannot decrypt the
// vault must not replace it.
func TestSave_RefusesToOverwriteUnreadableVault(t *testing.T) {
	dir := t.TempDir()
	mustSet(t, open(t, dir, "right"), "SECRET", "v")
	before, err := os.ReadFile(creds.NewStore(dir).Path)
	if err != nil {
		t.Fatal(err)
	}
	wrong := creds.NewStore(dir)
	wrong.SetPassphraseFn(func() string { return "wrong" })
	_ = wrong.Set("X", "y")
	if err := wrong.Save(); !errors.Is(err, creds.ErrWrongPassphrase) {
		t.Fatalf("Save with the wrong passphrase: err=%v, want ErrWrongPassphrase", err)
	}
	after, _ := os.ReadFile(creds.NewStore(dir).Path)
	if !bytes.Equal(before, after) {
		t.Fatal("an unreadable vault was overwritten")
	}
}

// TestRotate_CarriesOverOtherWritersKeys and the decrypt flow (write passphrase
// switched to "") both re-read with the passphrase that opened the file.
func TestRotate_CarriesOverOtherWritersKeys(t *testing.T) {
	dir := t.TempDir()
	daemon := open(t, dir, "old")
	mustSet(t, open(t, dir, "old"), "FROM_CLI", "v")
	if err := daemon.Rotate("new"); err != nil {
		t.Fatal(err)
	}
	if got := open(t, dir, "new").Get("FROM_CLI"); got != "v" {
		t.Fatalf("rotation dropped another writer's key (got %q)", got)
	}

	dec := open(t, dir, "new")
	dec.SetPassphraseFn(func() string { return "" })
	if err := dec.Save(); err != nil {
		t.Fatalf("decrypt-style save: %v", err)
	}
	plain := open(t, dir, "")
	if plain.IsEncrypted() || plain.Get("FROM_CLI") != "v" {
		t.Fatalf("decrypt flow: encrypted=%v names=%v", plain.IsEncrypted(), plain.Names())
	}
}
