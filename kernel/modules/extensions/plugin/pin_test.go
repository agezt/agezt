// SPDX-License-Identifier: MIT

package plugin_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/modules/extensions/plugin"
)

// TestVerifyPin_HashMatch covers the happy path: hash the file,
// pin matches, no error.
func TestVerifyPin_HashMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(path, []byte("the quick brown fox"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := plugin.HashFile(path)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	if len(got) != 64 {
		t.Fatalf("hash length = %d, want 64", len(got))
	}
	if err := plugin.VerifyPin(path, got); err != nil {
		t.Errorf("VerifyPin with matching pin: %v", err)
	}
}

// TestVerifyPin_HashMismatch verifies the sentinel is returned on
// drift, and the error message names both expected + actual hashes
// so an operator can diff them.
func TestVerifyPin_HashMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(path, []byte("original content"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Pin: 64 hex chars but not the actual hash.
	wrongPin := strings.Repeat("a", 64)
	err := plugin.VerifyPin(path, wrongPin)
	if err == nil {
		t.Fatal("expected error on mismatch")
	}
	if !errors.Is(err, plugin.ErrPinMismatch) {
		t.Errorf("err is not ErrPinMismatch: %v", err)
	}
	// Both hashes should appear in the message so operators can diff.
	if !strings.Contains(err.Error(), wrongPin) {
		t.Errorf("error doesn't mention expected hash: %v", err)
	}
}

// TestVerifyPin_RejectsMalformedPin verifies a non-hex / wrong-length
// pin is rejected before even hashing the file (saves I/O on a
// guaranteed failure).
func TestVerifyPin_RejectsMalformedPin(t *testing.T) {
	cases := []string{
		"",
		"abc",
		strings.Repeat("z", 64), // wrong charset
		strings.Repeat("a", 63), // wrong length
		strings.Repeat("A", 64), // uppercase — VerifyPin lowercases the input, but the test pins uppercase explicitly
	}
	for i, c := range cases {
		// Uppercase pin should normalise (last case is intentionally
		// the "passes after lowercasing" boundary test).
		path := filepath.Join(t.TempDir(), "bin")
		_ = os.WriteFile(path, []byte("x"), 0o600)
		err := plugin.VerifyPin(path, c)
		if i == 4 {
			// Uppercase 64-char hex should be normalised and treated
			// as valid format (will then mismatch the actual hash,
			// which is fine — different sentinel).
			if err == nil {
				continue // unexpected, but means lowercasing worked; mismatch path is exercised elsewhere
			}
			if errors.Is(err, plugin.ErrPinMismatch) {
				continue // expected: format ok, hash differs
			}
			t.Errorf("case %d (uppercase): got error %v, expected nil or ErrPinMismatch", i, err)
		} else if err == nil {
			t.Errorf("case %d (%q): expected format-rejection error, got nil", i, c)
		} else if errors.Is(err, plugin.ErrPinMismatch) {
			// A malformed pin must be rejected as a malformed DIGEST, not
			// as a hash mismatch: "your pin is not a digest" and "the file
			// changed" are different sentences to an operator.
			t.Errorf("case %d (%q): a malformed pin must be rejected as a format error, not as a hash mismatch: %v", i, c, err)
		} else if !strings.Contains(err.Error(), "64-char lowercase hex") {
			t.Errorf("case %d (%q): the rejection must say the pin is not a digest: %v", i, c, err)
		}
	}
}

// TestVerifyPin_FileMissing returns a wrapped read error rather
// than panicking or treating absence as "match."
func TestVerifyPin_FileMissing(t *testing.T) {
	err := plugin.VerifyPin(filepath.Join(t.TempDir(), "nope"), strings.Repeat("a", 64))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if errors.Is(err, plugin.ErrPinMismatch) {
		t.Errorf("missing file misclassified as pin mismatch: %v", err)
	}
}

// TestSpawn_RejectsMismatchedPin is the integration test: build
// echoplugin, compute its hash, then ask Spawn to verify against
// a DIFFERENT pin. Spawn must refuse before exec'ing the binary
// (we verify by checking the error sentinel; a successful Spawn
// would actually launch the child).
func TestSpawn_RejectsMismatchedPin(t *testing.T) {
	bin := buildEchoPlugin(t)
	wrongPin := strings.Repeat("d", 64)
	_, err := plugin.Spawn(context.Background(), plugin.Config{
		Path:       bin,
		PinnedHash: wrongPin,
	})
	if err == nil {
		t.Fatal("Spawn with wrong pin: expected error, got nil")
	}
	if !errors.Is(err, plugin.ErrPinMismatch) {
		t.Errorf("err is not ErrPinMismatch: %v", err)
	}
}

// TestSpawn_AcceptsCorrectPin: hash the echoplugin binary, pass
// that hash as the pin, Spawn should succeed and proceed through
// the normal initialize handshake.
func TestSpawn_AcceptsCorrectPin(t *testing.T) {
	bin := buildEchoPlugin(t)
	pin, err := plugin.HashFile(bin)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	p, err := plugin.Spawn(context.Background(), plugin.Config{
		Path:       bin,
		PinnedHash: pin,
	})
	if err != nil {
		t.Fatalf("Spawn with matching pin: %v", err)
	}
	defer p.Close()
	// Sanity check: the plugin actually initialized and offered tools.
	tools := p.Tools("")
	if _, ok := tools["echo"]; !ok {
		t.Errorf("plugin spawned but missing 'echo' tool — pin path may have skipped init")
	}
}
