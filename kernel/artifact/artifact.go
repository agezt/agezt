// SPDX-License-Identifier: MIT

// Package artifact is a content-addressed (BLAKE3) blob store — the substrate
// for SPEC-04 §3.6 artifacts: tool/run outputs that are too large to inline in an
// event are written here and referenced by their content hash, so the journal
// stays small while the bytes survive in the lineage and dedupe automatically.
//
// The store is deliberately minimal and pure: Put/Get/Has over a directory, no
// kernel or bus dependency. Higher layers (the agent loop's threshold-offload,
// the RawRef on tool.result, a retrieval endpoint) build on it.
//
// On disk: <dir>/<aa>/<ref>, sharded by the ref's first byte so one directory
// never holds the whole corpus. A ref is the lowercase hex BLAKE3-256 of the
// bytes, so identical content addresses identically (cross-source dedup) and any
// later read re-verifies the bytes against the ref (tamper/bit-rot detection).
package artifact

import (
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"lukechampine.com/blake3"
)

// refLen is the hex length of a BLAKE3-256 digest.
const refLen = 64

// ErrNotFound is returned by Get/Stat when the ref is not in the store.
var ErrNotFound = errors.New("artifact: not found")

// ErrCorrupt is returned by Get when the stored bytes no longer hash to the ref
// (tampering or bit-rot). The content address is the integrity guarantee.
var ErrCorrupt = errors.New("artifact: content does not match its ref")

// ErrBadRef is returned when a ref is not a 64-char lowercase hex string. Refs
// are validated before they touch the filesystem so a caller-supplied ref can
// never escape the store directory.
var ErrBadRef = errors.New("artifact: malformed ref")

// Store is a content-addressed blob store rooted at a directory.
type Store struct {
	dir string
}

// Open creates (if needed) and returns a Store rooted at dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("artifact: open %s: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

// Ref computes the content address (lowercase hex BLAKE3-256) of data without
// storing it. Useful to check Has before a Put, or to reference known content.
func Ref(data []byte) string {
	h := blake3.New(32, nil)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// Put stores data and returns its content ref. Idempotent: identical bytes
// produce the same ref and are written at most once (existing blobs are left
// untouched, so Put is safe to call repeatedly). The write is atomic (temp +
// rename) so a crash mid-write never leaves a partial blob at the final path.
func (s *Store) Put(data []byte) (string, error) {
	ref := Ref(data)
	path := s.pathFor(ref)
	if _, err := os.Stat(path); err == nil {
		return ref, nil // already present (dedup)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("artifact: put: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("artifact: put: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("artifact: put: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("artifact: put: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("artifact: put: %w", err)
	}
	return ref, nil
}

// Get returns the bytes for ref, re-verifying that they still hash to it. A ref
// not in the store returns ErrNotFound; a corrupted blob returns ErrCorrupt.
func (s *Store) Get(ref string) ([]byte, error) {
	if !validRef(ref) {
		return nil, ErrBadRef
	}
	data, err := os.ReadFile(s.pathFor(ref))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("artifact: get: %w", err)
	}
	// Constant-time compare is overkill for an integrity check but harmless and
	// makes the intent (the ref IS the integrity proof) explicit.
	if subtle.ConstantTimeCompare([]byte(Ref(data)), []byte(ref)) != 1 {
		return nil, ErrCorrupt
	}
	return data, nil
}

// Has reports whether ref is present (without reading the bytes).
func (s *Store) Has(ref string) (bool, error) {
	if !validRef(ref) {
		return false, ErrBadRef
	}
	_, err := os.Stat(s.pathFor(ref))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("artifact: has: %w", err)
}

// Size returns the byte length of the stored blob, or ErrNotFound.
func (s *Store) Size(ref string) (int64, error) {
	if !validRef(ref) {
		return 0, ErrBadRef
	}
	fi, err := os.Stat(s.pathFor(ref))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("artifact: size: %w", err)
	}
	return fi.Size(), nil
}

// pathFor maps a (validated) ref to its sharded on-disk path. Callers that pass
// unvalidated refs must validRef first; Put builds the ref itself so it is safe.
func (s *Store) pathFor(ref string) string {
	return filepath.Join(s.dir, ref[:2], ref)
}

// validRef enforces the ref shape (64 lowercase hex) so a malformed or hostile
// ref can never traverse out of the store directory.
func validRef(ref string) bool {
	if len(ref) != refLen {
		return false
	}
	for i := 0; i < len(ref); i++ {
		c := ref[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
