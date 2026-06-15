// SPDX-License-Identifier: MIT

package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStore_SetGetSaveLoad(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load fresh: %v", err)
	}
	if _, ok := s.Get("AGEZT_EMAIL_FROM"); ok {
		t.Error("fresh store should have nothing")
	}
	s.Set("AGEZT_EMAIL_FROM", "jarvis@example.com")
	s.Set("AGEZT_RATE_PER_MIN", "60")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload from disk → values survive.
	s2 := NewStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v, ok := s2.Get("AGEZT_EMAIL_FROM"); !ok || v != "jarvis@example.com" {
		t.Errorf("reloaded value = %q, %v", v, ok)
	}
	if got := s2.Names(); len(got) != 2 {
		t.Errorf("names = %v, want 2", got)
	}
	// File is the nested account form, 0600.
	info, err := os.Stat(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		// On Unix the file must not be group/other readable. Windows ignores mode.
		t.Errorf("config.json perms = %v, want 0600", info.Mode().Perm())
	}
}

func TestStore_Remove(t *testing.T) {
	s := NewStore(t.TempDir())
	_ = s.Load()
	s.Set("AGEZT_MODEL", "deepseek-chat")
	if !s.Remove("AGEZT_MODEL") {
		t.Error("Remove should report it was present")
	}
	if s.Remove("AGEZT_MODEL") {
		t.Error("second Remove should be false")
	}
	if _, ok := s.Get("AGEZT_MODEL"); ok {
		t.Error("value should be gone")
	}
}

func TestStore_LoadsLegacyFlatFile(t *testing.T) {
	dir := t.TempDir()
	// A hand-written flat {k:v} file (no account nesting) must still load.
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(`{"AGEZT_PROVIDER":"deepseek"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load flat: %v", err)
	}
	if v, ok := s.Get("AGEZT_PROVIDER"); !ok || v != "deepseek" {
		t.Errorf("flat value = %q, %v", v, ok)
	}
}

func TestStore_AllIsACopy(t *testing.T) {
	s := NewStore(t.TempDir())
	_ = s.Load()
	s.Set("AGEZT_MODEL", "x")
	all := s.All()
	all["AGEZT_MODEL"] = "mutated"
	if v, _ := s.Get("AGEZT_MODEL"); v != "x" {
		t.Error("All() must return a copy, not the live map")
	}
}

func TestSchema_SecretFieldsArePasswords(t *testing.T) {
	for _, sec := range Schema() {
		for _, f := range sec.Fields {
			if f.Secret && f.Type != TypePassword {
				t.Errorf("%s is Secret but type %q (want password)", f.Env, f.Type)
			}
			if f.Type == TypePassword && !f.Secret {
				t.Errorf("%s is a password field but not marked Secret", f.Env)
			}
			if f.Env == "" || f.Apply == "" {
				t.Errorf("field %+v missing Env or Apply", f)
			}
		}
	}
}

func TestSchema_FieldByEnv(t *testing.T) {
	f, ok := FieldByEnv("AGEZT_TELEGRAM_TOKEN")
	if !ok || !f.Secret || f.Apply != ApplyRestart {
		t.Errorf("telegram token field wrong: %+v ok=%v", f, ok)
	}
	if pf, ok := FieldByEnv("AGEZT_PROVIDER"); !ok || pf.Apply != ApplyLive {
		t.Errorf("provider should be live-apply: %+v ok=%v", pf, ok)
	}
	if _, ok := FieldByEnv("AGEZT_NOT_A_FIELD"); ok {
		t.Error("unknown env should not resolve")
	}
}

func TestValidate(t *testing.T) {
	num, _ := FieldByEnv("AGEZT_RATE_PER_MIN")
	if err := Validate(num, "abc"); err == nil {
		t.Error("non-numeric should fail number validation")
	}
	if err := Validate(num, "60"); err != nil {
		t.Errorf("60 should validate: %v", err)
	}
	if err := Validate(num, ""); err != nil {
		t.Error("empty always allowed (clearing a field)")
	}
	sel, _ := FieldByEnv("AGEZT_APPROVAL_MODE")
	if err := Validate(sel, "nonsense"); err == nil {
		t.Error("out-of-set select value should fail")
	}
	if err := Validate(sel, "ask"); err != nil {
		t.Errorf("ask is valid: %v", err)
	}
	bf, _ := FieldByEnv("AGEZT_ALLOW_ALL")
	if err := Validate(bf, "maybe"); err == nil {
		t.Error("non-bool should fail")
	}
	if err := Validate(bf, "on"); err != nil {
		t.Errorf("on is a valid bool: %v", err)
	}
}
