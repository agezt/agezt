// SPDX-License-Identifier: MIT

package creds

import "testing"

func newRing(t *testing.T) *Store {
	t.Helper()
	s := NewStore(t.TempDir())
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	return s
}

func active(list []KeyInfo) string {
	for _, k := range list {
		if k.Active {
			return k.Label
		}
	}
	return ""
}

func TestKeyring_AddActivateList(t *testing.T) {
	s := newRing(t)
	const name = "OPENAI_API_KEY"

	// First key becomes active and is mirrored to the bare name.
	changed, err := s.KeyringAdd(name, "work", "sk-work-1234", false)
	if err != nil || !changed {
		t.Fatalf("add work: changed=%v err=%v", changed, err)
	}
	if s.Get(name) != "sk-work-1234" {
		t.Fatalf("active not mirrored to bare name: %q", s.Get(name))
	}

	// Second key, not active.
	changed, err = s.KeyringAdd(name, "personal", "sk-pers-9876", false)
	if err != nil || changed {
		t.Fatalf("add personal: changed=%v err=%v", changed, err)
	}

	list := s.KeyringList(name)
	if len(list) != 2 {
		t.Fatalf("want 2 keys, got %d: %+v", len(list), list)
	}
	if active(list) != "work" {
		t.Fatalf("want work active, got %q", active(list))
	}
	// Fingerprints never expose the value (just the last 4 chars).
	for _, k := range list {
		if k.Last4 == "" || []rune(k.Last4)[0] != '…' {
			t.Errorf("bad fingerprint for %s: %q", k.Label, k.Last4)
		}
	}

	// Switch active.
	if err := s.KeyringActivate(name, "personal"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if s.Get(name) != "sk-pers-9876" {
		t.Fatalf("activate didn't mirror: %q", s.Get(name))
	}
	if active(s.KeyringList(name)) != "personal" {
		t.Fatalf("personal should be active now")
	}
}

func TestKeyring_LegacyBareKeyShowsAsDefault(t *testing.T) {
	s := newRing(t)
	const name = "ANTHROPIC_API_KEY"
	// A key set the old way: bare name only, no slot.
	_ = s.Set(name, "sk-ant-0001")
	list := s.KeyringList(name)
	if len(list) != 1 || list[0].Label != DefaultKeyLabel || !list[0].Active {
		t.Fatalf("legacy bare key should show as active default: %+v", list)
	}
}

func TestKeyring_RemoveActiveClearsBare(t *testing.T) {
	s := newRing(t)
	const name = "OPENAI_API_KEY"
	_, _ = s.KeyringAdd(name, "work", "sk-work-1234", true)
	_, _ = s.KeyringAdd(name, "personal", "sk-pers-9876", false)

	removed, wasActive := s.KeyringRemove(name, "work")
	if !removed || !wasActive {
		t.Fatalf("remove active: removed=%v wasActive=%v", removed, wasActive)
	}
	if s.Get(name) != "" {
		t.Fatalf("removing the active key should clear the bare name, got %q", s.Get(name))
	}
	// personal still stored; activating it re-credentials the provider.
	if err := s.KeyringActivate(name, "personal"); err != nil {
		t.Fatalf("re-activate personal: %v", err)
	}
	if s.Get(name) != "sk-pers-9876" {
		t.Fatalf("re-activate didn't restore: %q", s.Get(name))
	}
}

func TestKeyring_Validation(t *testing.T) {
	s := newRing(t)
	if _, err := s.KeyringAdd("OPENAI_API_KEY", "Bad Label", "x", true); err == nil {
		t.Error("expected bad label rejection")
	}
	if _, err := s.KeyringAdd("OPENAI_API_KEY", "ok", "", true); err == nil {
		t.Error("expected empty value rejection")
	}
	if err := s.KeyringActivate("OPENAI_API_KEY", "missing"); err == nil {
		t.Error("expected activate-missing rejection")
	}
}
