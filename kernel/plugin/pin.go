// SPDX-License-Identifier: MIT

package plugin

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	"lukechampine.com/blake3"
)

// resolvePluginPath resolves a bare-name plugin path (no path separator) to the
// absolute file exec would run, via $PATH — so the pin hash (HashFile → os.Open,
// CWD-relative) and the executed binary (exec.Command → $PATH lookup) are the SAME
// file (M422). Without this, AGEZT_PLUGINS="t=mytool" could hash ./mytool while
// executing $PATH/mytool, letting a pin guard the wrong binary. A path that already
// contains a separator, or a bare name not found on $PATH, is returned unchanged
// (exec and os.Open already agree on a separator path; an unresolvable name then
// fails closed at Start/HashFile).
func resolvePluginPath(path string) string {
	if strings.ContainsRune(path, '/') || strings.ContainsRune(path, filepath.Separator) {
		return path
	}
	if resolved, err := osexec.LookPath(path); err == nil {
		return resolved
	}
	return path
}

// makeChild wraps osexec.Command to centralise the construction
// (the host's Spawn used to call osexec.Command inline; the wrapper
// makes pin enforcement, future sandbox stubs, etc. easier to add).
func makeChild(path string, args []string) *osexec.Cmd {
	cmd := osexec.Command(path, args...)
	// Put the child in its own process group so teardown can kill the
	// whole tree, not just the direct child (M184). Platform-specific:
	// a real process group on Unix, a no-op on Windows (see proc_*.go).
	setProcessGroup(cmd)
	return cmd
}

// VerifyPin checks that the file at path has BLAKE3-256 digest
// matching pin (lowercase hex, 64 chars). Returns nil on match;
// a wrapped ErrPinMismatch on hash mismatch; a wrapped read error
// on I/O failure.
//
// Exported so the (small) `agt plugin hash` helper and tests can
// reuse the same code path the host enforces with.
//
// Why BLAKE3:
//   - Already a project dep (the only one). No new dep cost.
//   - Streaming friendly — we hash the binary without loading it
//     into memory.
//   - Fast: a 50 MB plugin hashes in <50 ms on commodity hardware,
//     keeping daemon startup under the operator's perception
//     threshold even with several pinned plugins.
//
// Why not GPG signatures: signature verification needs a public-key
// distribution story (whose key, how do operators get it, what does
// trust-on-first-use look like?). Hash-pinning sidesteps that:
// operators record the hash they've personally verified once, then
// the daemon refuses any drift. No PKI required, and the threat
// model the operator actually has — "did this binary change since I
// last looked at it" — is exactly what hash pinning answers.
func VerifyPin(path, pin string) error {
	expected := strings.ToLower(strings.TrimSpace(pin))
	if !looksLikeBLAKE3Pin(expected) {
		return fmt.Errorf("plugin: pin for %q is not a 64-char lowercase hex BLAKE3-256 digest", path)
	}
	got, err := HashFile(path)
	if err != nil {
		return fmt.Errorf("plugin: hash %q for pin check: %w", path, err)
	}
	if got != expected {
		return fmt.Errorf("%w: %q\n  expected: %s\n  got:      %s",
			ErrPinMismatch, path, expected, got)
	}
	return nil
}

// looksLikeBLAKE3Pin is a cheap sanity check on the pin format
// before we burn I/O on hashing the file. BLAKE3-256 → 32 bytes →
// 64 hex chars.
func looksLikeBLAKE3Pin(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// HashFile returns the lowercase-hex BLAKE3-256 digest of the file
// at path. Streams through a fixed-size buffer; safe for large
// binaries. Used by VerifyPin and exposed for the `agt plugin
// hash` helper that operators run once to record the pin.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := blake3.New(32, nil)
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum), nil
}

// ErrPinMismatch is wrapped in the error VerifyPin returns when the
// computed hash doesn't match the operator-supplied pin. Distinct
// sentinel so the daemon can produce a tailored stderr message
// (and so a future audit-event publisher can recognise the case
// without string-matching).
var ErrPinMismatch = errors.New("plugin: binary hash does not match pinned value")
