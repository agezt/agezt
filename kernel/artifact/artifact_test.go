// SPDX-License-Identifier: MIT

package artifact

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

// TestPutGetRoundTrip: Put returns a content ref; Get returns the exact bytes.
func TestPutGetRoundTrip(t *testing.T) {
	s := openStore(t)
	data := []byte("a large-ish tool output that we do not want inlined in an event")
	ref, err := s.Put(data)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if ref != Ref(data) || len(ref) != refLen {
		t.Fatalf("ref %q is not the content hash", ref)
	}
	got, err := s.Get(ref)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("round-trip mismatch: got %q", got)
	}
	if n, _ := s.Size(ref); n != int64(len(data)) {
		t.Errorf("Size = %d, want %d", n, len(data))
	}
}

// TestPutDedups: identical bytes address identically and are written once;
// different bytes get different refs.
func TestPutDedups(t *testing.T) {
	s := openStore(t)
	data := []byte("dedupe me")
	r1, _ := s.Put(data)
	r2, _ := s.Put(data) // again
	if r1 != r2 {
		t.Fatalf("same content produced different refs: %s vs %s", r1, r2)
	}
	// Exactly one blob file on disk for this ref.
	matches, _ := filepath.Glob(filepath.Join(s.dir, r1[:2], r1))
	if len(matches) != 1 {
		t.Errorf("expected exactly one stored blob, found %d", len(matches))
	}
	r3, _ := s.Put([]byte("different"))
	if r3 == r1 {
		t.Errorf("different content collided to the same ref")
	}
}

// TestHas: present vs absent.
func TestHas(t *testing.T) {
	s := openStore(t)
	ref, _ := s.Put([]byte("x"))
	if ok, err := s.Has(ref); err != nil || !ok {
		t.Errorf("Has(present) = %v,%v want true,nil", ok, err)
	}
	absent := Ref([]byte("never stored"))
	if ok, err := s.Has(absent); err != nil || ok {
		t.Errorf("Has(absent) = %v,%v want false,nil", ok, err)
	}
}

// TestGetCorruptIsDetected: the ref is the integrity proof — a tampered blob
// returns ErrCorrupt, not silently-wrong bytes.
func TestGetCorruptIsDetected(t *testing.T) {
	s := openStore(t)
	ref, _ := s.Put([]byte("trustworthy"))
	// Overwrite the stored blob with different bytes (keeping the same path/ref).
	if err := os.WriteFile(s.pathFor(ref), []byte("tampered!!"), 0o600); err != nil {
		t.Fatalf("tamper write: %v", err)
	}
	if _, err := s.Get(ref); !errors.Is(err, ErrCorrupt) {
		t.Errorf("Get(tampered) err = %v, want ErrCorrupt", err)
	}
}

// TestNotFound: Get/Size on an unknown (well-formed) ref.
func TestNotFound(t *testing.T) {
	s := openStore(t)
	ref := Ref([]byte("absent"))
	if _, err := s.Get(ref); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(absent) err = %v, want ErrNotFound", err)
	}
	if _, err := s.Size(ref); !errors.Is(err, ErrNotFound) {
		t.Errorf("Size(absent) err = %v, want ErrNotFound", err)
	}
}

// TestBadRefRejected: a malformed/hostile ref never touches the filesystem and
// cannot traverse out of the store directory.
func TestBadRefRejected(t *testing.T) {
	s := openStore(t)
	for _, bad := range []string{
		"", "short", "../../etc/passwd",
		"ZZ" + Ref([]byte("x"))[2:], // non-hex
		Ref([]byte("x")) + "00",     // too long
		filepath.Join("..", "escape"),
	} {
		if _, err := s.Get(bad); !errors.Is(err, ErrBadRef) {
			t.Errorf("Get(%q) err = %v, want ErrBadRef", bad, err)
		}
		if _, err := s.Has(bad); !errors.Is(err, ErrBadRef) {
			t.Errorf("Has(%q) err = %v, want ErrBadRef", bad, err)
		}
	}
}

// TestPutEmpty: an empty blob is a valid, addressable artifact.
func TestPutEmpty(t *testing.T) {
	s := openStore(t)
	ref, err := s.Put(nil)
	if err != nil {
		t.Fatalf("Put(nil): %v", err)
	}
	got, err := s.Get(ref)
	if err != nil || len(got) != 0 {
		t.Errorf("Get(empty) = %q,%v want empty,nil", got, err)
	}
}
