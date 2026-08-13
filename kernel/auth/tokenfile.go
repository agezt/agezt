// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/atomicfile"
)

// WriteTokenFile atomically persists token in an owner-only file directly
// under baseDir. filename must be one path segment; rejecting separators keeps
// a caller-controlled name from escaping the credential directory.
//
// The returned prefix is safe for diagnostics and boot banners. The full token
// must never be logged.
func WriteTokenFile(baseDir, filename, token string) (string, error) {
	if strings.TrimSpace(baseDir) == "" {
		return "", fmt.Errorf("auth: token file base directory is required")
	}
	if !validTokenFilename(filename) {
		return "", fmt.Errorf("auth: token filename must be one non-empty path segment")
	}
	if token == "" {
		return "", fmt.Errorf("auth: token is required")
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return "", fmt.Errorf("auth: create token directory: %w", err)
	}
	path := filepath.Join(baseDir, filename)
	if err := atomicfile.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("auth: write token file: %w", err)
	}
	return TokenPrefix(token), nil
}

// ReadTokenFile returns the credential WriteTokenFile persisted under baseDir,
// trimmed of the trailing newline it appends. It applies the same filename
// validation as the writer, so a caller-controlled name still cannot read
// outside the credential directory.
//
// A MISSING file is not an error: it returns ("", nil). That is what makes the
// pair usable as read-or-mint — an absent file means "no credential yet", which
// is a first-boot state, not a failure. Every other read error is returned so a
// caller can fail closed rather than silently mint a second credential over an
// unreadable one.
func ReadTokenFile(baseDir, filename string) (string, error) {
	if strings.TrimSpace(baseDir) == "" {
		return "", fmt.Errorf("auth: token file base directory is required")
	}
	if !validTokenFilename(filename) {
		return "", fmt.Errorf("auth: token filename must be one non-empty path segment")
	}
	data, err := os.ReadFile(filepath.Join(baseDir, filename))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("auth: read token file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// TokenPrefix returns a short credential fingerprint suitable for diagnostics.
// Tokens of eight characters or fewer are deliberately not surfaced: unlike a
// normal 256-bit daemon token, revealing one would reveal the whole secret.
func TokenPrefix(token string) string {
	if len(token) <= 8 {
		return ""
	}
	return token[:4] + "…" + token[len(token)-4:]
}

func validTokenFilename(filename string) bool {
	if filename == "" || filename == "." || filename == ".." {
		return false
	}
	if filepath.IsAbs(filename) || strings.ContainsAny(filename, `/\`) {
		return false
	}
	return filepath.Base(filename) == filename
}
