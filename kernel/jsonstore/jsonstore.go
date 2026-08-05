// SPDX-License-Identifier: MIT

// Package jsonstore is the shared persistence plumbing for the kernel's
// single-file JSON stores (Phase 1.1 of docs/REFACTORING-SCAN-2026-08.md):
// a tolerant Load and an atomic Save that a dozen store packages previously
// hand-rolled with drifting behavior (four grew a Windows rename-retry, one
// silently swallowed corrupt files, two stripped BOMs, the rest did neither).
//
// Deliberately NOT a generic Store[T] owning the mutex: every store's value
// is in its domain methods and locking discipline, which stay local. The
// duplication worth deleting was only the file IO — so that is all this
// package owns.
package jsonstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agezt/agezt/internal/atomicfile"
)

// Load reads the JSON file at path into out (a pointer, as for
// json.Unmarshal). A missing, empty, or whitespace-only file leaves out
// untouched and returns nil — first boot is not an error. A UTF-8 BOM is
// tolerated (files hand-edited on Windows often carry one). Any other read
// or parse failure is returned wrapped with the path, so boot errors name
// the file at fault.
func Load(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// LoadFrom is Load for the common Open(dir) shape: it ensures dir exists,
// then loads dir/name into out and returns the joined path for the store to
// keep. The MkdirAll runs even when the file is absent so the first Save has
// a directory to land in.
func LoadFrom(dir, name string, out any) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := Load(path, out); err != nil {
		return "", err
	}
	return path, nil
}

// Save writes v as indented JSON to path via atomicfile (unique temp +
// fsync + rename, with the Windows remove-retry and in-memory last-resort
// handling centralized there). Callers hold their own lock around Save; the
// store's mutex discipline stays in the store.
func Save(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(path, b, 0o644)
}
