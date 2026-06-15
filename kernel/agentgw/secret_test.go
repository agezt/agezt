// SPDX-License-Identifier: MIT

package agentgw

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestResolveTokenSecret_PersistsAcrossCalls proves the per-install signing
// secret is STABLE: the daemon (validator) and the `agt` CLI (minter) derive
// the SAME key from one base dir. This is the core of the C1 fix — without it,
// tokens minted by the CLI would never validate at the daemon.
func TestResolveTokenSecret_PersistsAcrossCalls(t *testing.T) {
	t.Setenv(TokenSecretEnv, "") // isolate from any real env override

	dir := t.TempDir()

	first, err := ResolveTokenSecret(dir)
	if err != nil {
		t.Fatalf("ResolveTokenSecret (1st): %v", err)
	}
	if len(first) < secretBytes {
		t.Fatalf("first secret too short: got %d bytes, want >= %d", len(first), secretBytes)
	}

	// A second resolution from the same dir must return the identical key
	// (read from the persisted file, not a fresh random one).
	second, err := ResolveTokenSecret(dir)
	if err != nil {
		t.Fatalf("ResolveTokenSecret (2nd): %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("ResolveTokenSecret returned DIFFERENT secrets across calls — daemon and CLI would disagree")
	}

	// The persisted file must be 0600 on POSIX (Windows ACLs don't honor the
	// Unix perm bits — os.OpenFile ignores them there — so only assert there).
	info, err := os.Stat(filepath.Join(dir, tokenSecretFile))
	if err != nil {
		t.Fatalf("stat secret file: %v", err)
	}
	if runtime.GOOS != "windows" {
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("secret file mode: got %o, want 0600", mode)
		}
	}
}

// TestResolveTokenSecret_EnvOverride confirms $AGEZT_AGENTGW_TOKEN_SECRET wins
// over the persisted file — the documented escape hatch for daemon and CLI on
// different hosts sharing one out-of-band secret.
func TestResolveTokenSecret_EnvOverride(t *testing.T) {
	const want = "operator-distributed-out-of-band-secret"
	t.Setenv(TokenSecretEnv, want)

	dir := t.TempDir()
	// Seed a persisted file first; the env var must still take precedence.
	if _, err := ResolveTokenSecret(dir); err != nil {
		t.Fatalf("seed persisted secret: %v", err)
	}

	got, err := ResolveTokenSecret(dir)
	if err != nil {
		t.Fatalf("ResolveTokenSecret: %v", err)
	}
	if string(got) != want {
		t.Errorf("env override ignored: got %q, want %q", string(got), want)
	}
}

// TestResolveTokenSecret_EmptyBaseDir returns a non-empty process-lifetime
// secret and must NOT touch the filesystem (safe default when there's nowhere
// to persist).
func TestResolveTokenSecret_EmptyBaseDir(t *testing.T) {
	t.Setenv(TokenSecretEnv, "")

	sec, err := ResolveTokenSecret("")
	if err != nil {
		t.Fatalf("ResolveTokenSecret(\"\"): %v", err)
	}
	if len(sec) < secretBytes {
		t.Errorf("empty base dir secret too short: got %d, want >= %d", len(sec), secretBytes)
	}
}

// TestResolveTokenSecret_NotHardcoded is the C1 regression guard: the resolved
// key must never equal the old public-source constant. A future refactor that
// re-introduces a hardcoded key would fail here.
func TestResolveTokenSecret_NotHardcoded(t *testing.T) {
	t.Setenv(TokenSecretEnv, "")

	sec, err := ResolveTokenSecret(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveTokenSecret: %v", err)
	}
	const old = "change-me-in-production"
	if string(sec) == old {
		t.Fatalf("resolved secret equals the hardcoded literal %q — auth is forgeable again", old)
	}
	// Two fresh base dirs must yield two DIFFERENT random keys (not a constant).
	a, _ := ResolveTokenSecret(t.TempDir())
	b, _ := ResolveTokenSecret(t.TempDir())
	if string(a) == string(b) {
		t.Error("two fresh installs produced identical secrets — secret generation is not random")
	}
}

// TestResolveTokenSecret_ConcurrentFirstRun exercises the O_EXCL first-run race:
// N callers resolving from the same not-yet-seeded dir must all converge on ONE
// key (whoever won the O_CREATE|O_EXCL race), never disagree.
func TestResolveTokenSecret_ConcurrentFirstRun(t *testing.T) {
	t.Setenv(TokenSecretEnv, "")

	dir := t.TempDir()
	const n = 16
	type res struct {
		sec []byte
		err error
	}
	ch := make(chan res, n)
	for range n {
		go func() {
			sec, err := ResolveTokenSecret(dir)
			ch <- res{sec, err}
		}()
	}
	var first []byte
	for range n {
		r := <-ch
		if r.err != nil {
			t.Fatalf("concurrent resolve errored: %v", r.err)
		}
		if len(r.sec) < secretBytes {
			t.Fatalf("short secret: %d bytes", len(r.sec))
		}
		if first == nil {
			first = r.sec
			continue
		}
		if string(first) != string(r.sec) {
			t.Fatalf("concurrent callers disagreed on the secret:\n  %x\n  %x", first, r.sec)
		}
	}
	// Sanity: ensure we didn't accidentally compare empty values.
	if strings.TrimSpace(string(first)) == "" {
		t.Fatal("converged on an empty secret")
	}
}

// TestResolveTokenSecret_PersistedFileEmpty tests that an empty persisted file
// returns an error (not silently regenerated), because if a secret file exists
// with invalid content, we should not overwrite it.
func TestResolveTokenSecret_PersistedFileEmpty(t *testing.T) {
	t.Setenv(TokenSecretEnv, "")

	dir := t.TempDir()
	path := filepath.Join(dir, tokenSecretFile)

	// Create empty secret file.
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}

	_, err := ResolveTokenSecret(dir)
	if err == nil {
		t.Error("ResolveTokenSecret: expected error for empty secret file, got nil")
	}
}

// TestResolveTokenSecret_PersistedFileWhitespaceOnly tests that a whitespace-only
// persisted file returns an error (not silently regenerated), because if a secret
// file exists with invalid content, we should not overwrite it.
func TestResolveTokenSecret_PersistedFileWhitespaceOnly(t *testing.T) {
	t.Setenv(TokenSecretEnv, "")

	dir := t.TempDir()
	path := filepath.Join(dir, tokenSecretFile)

	if err := os.WriteFile(path, []byte("   \n\t  \n"), 0o600); err != nil {
		t.Fatalf("write whitespace file: %v", err)
	}

	_, err := ResolveTokenSecret(dir)
	if err == nil {
		t.Error("ResolveTokenSecret: expected error for whitespace-only secret file, got nil")
	}
}

// TestResolveTokenSecret_PersistedFileInvalidHex tests that an invalid hex
// persisted file triggers fresh secret generation (falls back to raw bytes).
func TestResolveTokenSecret_PersistedFileInvalidHex(t *testing.T) {
	t.Setenv(TokenSecretEnv, "")

	dir := t.TempDir()
	path := filepath.Join(dir, tokenSecretFile)

	// Write something that is not valid hex.
	if err := os.WriteFile(path, []byte("not-valid-hex-content!@#$"), 0o600); err != nil {
		t.Fatalf("write invalid hex file: %v", err)
	}

	sec, err := ResolveTokenSecret(dir)
	if err != nil {
		t.Fatalf("ResolveTokenSecret: %v", err)
	}
	// decodeSecret treats non-hex as raw bytes, so this should work
	if string(sec) != "not-valid-hex-content!@#$" {
		t.Errorf("unexpected secret: got %q", string(sec))
	}
}

// TestResolveTokenSecret_PersistedFileShortHex tests that a hex-encoded secret
// that decodes to fewer than 32 bytes is treated as raw bytes (operator-edited
// passphrase file fallback).
func TestResolveTokenSecret_PersistedFileShortHex(t *testing.T) {
	t.Setenv(TokenSecretEnv, "")

	dir := t.TempDir()
	path := filepath.Join(dir, tokenSecretFile)

	// 16 bytes in hex = 32 hex chars. This is less than 32 bytes, so it should
	// be treated as raw bytes by decodeSecret, not rejected.
	shortHex := "00112233445566778899aabbccddeeff" // 16 bytes
	if err := os.WriteFile(path, []byte(shortHex), 0o600); err != nil {
		t.Fatalf("write short hex file: %v", err)
	}

	sec, err := ResolveTokenSecret(dir)
	if err != nil {
		t.Fatalf("ResolveTokenSecret: %v", err)
	}
	// Should be treated as raw bytes, not hex-decoded
	if string(sec) != shortHex {
		t.Errorf("secret: got %q, want %q (treated as raw bytes)", string(sec), shortHex)
	}
}

// TestResolveTokenSecret_PersistedFileValidHex tests that a valid hex-encoded
// 32-byte secret is correctly decoded.
func TestResolveTokenSecret_PersistedFileValidHex(t *testing.T) {
	t.Setenv(TokenSecretEnv, "")

	dir := t.TempDir()
	path := filepath.Join(dir, tokenSecretFile)

	// 32 bytes = 64 hex chars
	want := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
		t.Fatalf("write valid hex file: %v", err)
	}

	sec, err := ResolveTokenSecret(dir)
	if err != nil {
		t.Fatalf("ResolveTokenSecret: %v", err)
	}

	// Should be hex-decoded to 32 raw bytes
	if len(sec) != secretBytes {
		t.Errorf("secret length: got %d, want %d", len(sec), secretBytes)
	}
	// Verify the hex-decoded value matches
	expectedBytes, _ := hex.DecodeString(want)
	if string(sec) != string(expectedBytes) {
		t.Errorf("secret: got %x, want %x", sec, expectedBytes)
	}
}

// TestResolveTokenSecret_MkdirAllFailure tests that an unreadable base dir returns an error.
func TestResolveTokenSecret_MkdirAllFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping permission test on Windows")
	}
	t.Setenv(TokenSecretEnv, "")

	// Use /proc or similar that can't be created as a subdir
	dir := "/proc/not-a-real-dir"
	_, err := ResolveTokenSecret(dir)
	if err == nil {
		t.Error("expected error for unreadable base dir, got nil")
	}
}

// TestDecodeSecret_Empty tests that decodeSecret returns nil for empty input.
func TestDecodeSecret_Empty(t *testing.T) {
	got := decodeSecret([]byte{})
	if got != nil {
		t.Errorf("decodeSecret(empty): got %v, want nil", got)
	}
}

// TestDecodeSecret_WhitespaceOnly tests that decodeSecret returns nil for whitespace-only input.
func TestDecodeSecret_WhitespaceOnly(t *testing.T) {
	tests := [][]byte{
		[]byte("   "),
		[]byte("\n\t  \n"),
		[]byte(""),
	}
	for _, input := range tests {
		got := decodeSecret(input)
		if got != nil {
			t.Errorf("decodeSecret(%q): got %v, want nil", string(input), got)
		}
	}
}

// TestDecodeSecret_ValidHex tests that decodeSecret correctly decodes valid hex.
func TestDecodeSecret_ValidHex(t *testing.T) {
	// 32 bytes = 64 hex chars
	hexStr := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	got := decodeSecret([]byte(hexStr))
	if got == nil {
		t.Fatal("decodeSecret: got nil, want decoded bytes")
	}
	if len(got) != secretBytes {
		t.Errorf("decoded length: got %d, want %d", len(got), secretBytes)
	}
}

// TestDecodeSecret_InvalidHex_TreatedAsRaw tests that non-hex input is returned as raw bytes.
func TestDecodeSecret_InvalidHex_TreatedAsRaw(t *testing.T) {
	raw := []byte("my-passphrase")
	got := decodeSecret(raw)
	if got == nil {
		t.Fatal("decodeSecret: got nil, want raw bytes")
	}
	if string(got) != string(raw) {
		t.Errorf("decodeSecret: got %q, want %q", string(got), string(raw))
	}
}

// TestDecodeSecret_ShortHex_TreatedAsRaw tests that hex that decodes to < 32 bytes
// is treated as raw bytes, not rejected.
func TestDecodeSecret_ShortHex_TreatedAsRaw(t *testing.T) {
	// 16 bytes = 32 hex chars, but decodeSecret requires >= 32 raw bytes
	shortHex := "00112233445566778899aabbccddeeff" // 16 bytes
	got := decodeSecret([]byte(shortHex))
	if got == nil {
		t.Fatal("decodeSecret: got nil, want raw bytes (short hex treated as raw)")
	}
	if string(got) != shortHex {
		t.Errorf("decodeSecret: got %q, want %q (treated as raw)", string(got), shortHex)
	}
}

// TestDecodeSecret_HexWithWhitespace tests that hex with surrounding whitespace is trimmed.
func TestDecodeSecret_HexWithWhitespace(t *testing.T) {
	hexStr := "  00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff  \n"
	got := decodeSecret([]byte(hexStr))
	if got == nil {
		t.Fatal("decodeSecret: got nil, want decoded bytes")
	}
	if len(got) != secretBytes {
		t.Errorf("decoded length: got %d, want %d", len(got), secretBytes)
	}
}
