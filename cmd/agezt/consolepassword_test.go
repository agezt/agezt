// SPDX-License-Identifier: MIT

package main

// SECRET-002 regression cover. The console is ON by default at 127.0.0.1:8787
// and, in the default (non-strict) mode, its password is a SUFFICIENT
// credential — `authorized()` is token OR session. The built-in default used to
// be `const defaultLoopbackWebPassword = "agezt"`, a compile-time credential in
// a public repository, so every install shared one publicly-known password.
//
// These tests pin the two properties that fix it: the built-in is minted per
// install (never a constant), and it is stable across boots of the same install.
// The second half pins the behaviour the fix must NOT break — an explicitly
// configured AGEZT_WEB_PASSWORD still wins, and the opt-out still opts out.

import (
	"encoding/hex"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"testing"

	"github.com/agezt/agezt/internal/brand"
)

func TestConsolePasswordIsMintedPerInstall(t *testing.T) {
	// Two installs = two base directories. With the old constant BOTH would
	// answer "agezt"; a minted password must differ, or knowing one install's
	// password means knowing every install's.
	first, minted, err := ensureConsolePassword(t.TempDir())
	if err != nil {
		t.Fatalf("ensureConsolePassword: %v", err)
	}
	if !minted {
		t.Fatal("first call did not report minting a password")
	}
	second, _, err := ensureConsolePassword(t.TempDir())
	if err != nil {
		t.Fatalf("ensureConsolePassword (second install): %v", err)
	}
	if first == second {
		t.Fatalf("two installs share the console password %q — a per-install secret must differ", first)
	}
	if first == "agezt" || second == "agezt" {
		t.Fatal(`the hardcoded default password "agezt" is back`)
	}
	// Enough entropy to be unguessable, and hex so it survives a copy-paste out
	// of a terminal into a browser login form.
	if want := consolePasswordBytes * 2; len(first) != want {
		t.Errorf("minted password length = %d, want %d hex chars", len(first), want)
	}
	if _, err := hex.DecodeString(first); err != nil {
		t.Errorf("minted password is not hex: %v", err)
	}
}

func TestConsolePasswordSurvivesRestart(t *testing.T) {
	// The password is persisted, not re-minted per boot: an operator who wrote
	// it down on first boot must still be able to log in after a restart.
	dir := t.TempDir()
	first, minted, err := ensureConsolePassword(dir)
	if err != nil || !minted {
		t.Fatalf("first mint: %q minted=%v err=%v", first, minted, err)
	}
	again, mintedAgain, err := ensureConsolePassword(dir)
	if err != nil {
		t.Fatalf("second boot: %v", err)
	}
	if mintedAgain {
		t.Error("second boot re-minted the password — the operator's copy would stop working")
	}
	if again != first {
		t.Errorf("second boot password = %q, want the persisted %q", again, first)
	}

	// Persisted owner-only, like every other credential under baseDir. Windows
	// does not carry POSIX mode bits, so only assert them where they mean
	// something.
	path := filepath.Join(dir, consolePasswordFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if stdruntime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("%s mode = %#o, want 0600", path, perm)
		}
	}
}

func TestEffectiveWebPasswordPrecedence(t *testing.T) {
	// The fix changes only the built-in. Everything an operator can configure
	// must behave exactly as before.
	const builtin = "0123456789abcdef01234567"

	t.Run("explicit password outranks the built-in", func(t *testing.T) {
		t.Setenv(brand.EnvPrefix+"WEB_PASSWORD", "operator-chosen")
		if got := effectiveWebPassword(builtin, "127.0.0.1:8787"); got != "operator-chosen" {
			t.Fatalf("password = %q, want the explicitly configured one", got)
		}
		// ...including on a bind where the built-in would not apply at all.
		if got := effectiveWebPassword(builtin, "0.0.0.0:8787"); got != "operator-chosen" {
			t.Fatalf("non-loopback password = %q, want the explicitly configured one", got)
		}
	})

	t.Run("built-in applies on loopback", func(t *testing.T) {
		if got := effectiveWebPassword(builtin, "127.0.0.1:8787"); got != builtin {
			t.Fatalf("loopback password = %q, want the minted built-in", got)
		}
	})

	t.Run("built-in never applies beyond loopback", func(t *testing.T) {
		if got := effectiveWebPassword(builtin, "0.0.0.0:8787"); got != "" {
			t.Fatalf("exposed bind password = %q, want none", got)
		}
	})

	t.Run("opt-out disables the built-in", func(t *testing.T) {
		for _, keyword := range []string{"off", "false", "0", "none"} {
			t.Setenv(brand.EnvPrefix+"WEB_PASSWORD_DEFAULT", keyword)
			if !webPasswordDefaultDisabled() {
				t.Errorf("webPasswordDefaultDisabled() = false for %q", keyword)
			}
			if got := effectiveWebPassword(builtin, "127.0.0.1:8787"); got != "" {
				t.Errorf("%s=%q: password = %q, want none", brand.EnvPrefix+"WEB_PASSWORD_DEFAULT", keyword, got)
			}
		}
	})

	t.Run("no built-in minted degrades to token-only", func(t *testing.T) {
		if got := effectiveWebPassword("", "127.0.0.1:8787"); got != "" {
			t.Fatalf("password = %q, want none when minting failed", got)
		}
	})
}
