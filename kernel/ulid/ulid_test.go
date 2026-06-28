// SPDX-License-Identifier: MIT

package ulid

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNew_Format(t *testing.T) {
	id := New()
	if len(id) != EncodedSize {
		t.Fatalf("len=%d want %d (%q)", len(id), EncodedSize, id)
	}
	// Must be all-uppercase Crockford base32 (no I/L/O/U).
	for i, c := range []byte(id) {
		if !strings.ContainsRune(crockfordAlphabet, rune(c)) {
			t.Errorf("non-Crockford char %q at %d", c, i)
		}
	}
}

func TestNew_Unique(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := range n {
		id := New()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate ID at i=%d: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestNew_Sortable(t *testing.T) {
	// Two IDs minted in order, even with the same millisecond, will likely
	// sort correctly only because of the random tail. Use distinct ticks
	// to assert deterministic ordering.
	now := time.UnixMilli(1_700_000_000_000)
	clock := func() time.Time {
		t := now
		now = now.Add(time.Millisecond)
		return t
	}
	g := &Generator{nowFunc: clock, randSrc: &fixedReader{0xAB}}
	a := g.New()
	b := g.New()
	if a >= b {
		t.Errorf("expected a<b (timestamp prefix sorts), got a=%q b=%q", a, b)
	}
}

func TestConcurrentSafe(t *testing.T) {
	const goroutines = 16
	const per = 200
	var wg sync.WaitGroup
	out := make(chan string, goroutines*per)
	for range goroutines {
		wg.Go(func() {
			for range per {
				out <- New()
			}
		})
	}
	wg.Wait()
	close(out)
	seen := make(map[string]struct{}, goroutines*per)
	for id := range out {
		if _, dup := seen[id]; dup {
			t.Errorf("duplicate under concurrency: %s", id)
		}
		seen[id] = struct{}{}
	}
}

// fixedReader returns the same byte forever; used to make tests deterministic.
type fixedReader struct{ b byte }

func (r *fixedReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.b
	}
	return len(p), nil
}

// guard against accidental shared state mutation in encode.
func TestEncode_NoMutate(t *testing.T) {
	in := [Size]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	orig := in
	_ = encode(in)
	if !bytes.Equal(orig[:], in[:]) {
		t.Errorf("encode mutated input: got %v want %v", in, orig)
	}
}
