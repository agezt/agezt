// SPDX-License-Identifier: MIT

package streamlimit

import (
	"sync"
	"testing"
)

func active(l *Limiter, key string) int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active[key]
}

func TestAcquire_CapAndRelease(t *testing.T) {
	l := New(2)

	r1, ok := l.Acquire("a")
	if !ok {
		t.Fatal("first acquire should succeed")
	}
	r2, ok := l.Acquire("a")
	if !ok {
		t.Fatal("second acquire should succeed (at cap)")
	}
	if _, ok := l.Acquire("a"); ok {
		t.Fatal("third acquire should be refused (over cap)")
	}
	if got := active(l, "a"); got != 2 {
		t.Fatalf("active = %d, want 2", got)
	}

	// A different key has its own budget.
	if r, ok := l.Acquire("b"); !ok {
		t.Fatal("different key should have its own budget")
	} else {
		r()
	}

	// Releasing one frees a slot.
	r1()
	if _, ok := l.Acquire("a"); !ok {
		t.Fatal("after release a slot should be available")
	}
	r2()
}

func TestRelease_Idempotent(t *testing.T) {
	l := New(1)
	r, _ := l.Acquire("k")
	r()
	r() // second call must not underflow / free an extra slot
	if got := active(l, "k"); got != 0 {
		t.Fatalf("active = %d, want 0", got)
	}
	// Capacity is exactly 1, not 2 (no phantom slot from the double release).
	if _, ok := l.Acquire("k"); !ok {
		t.Fatal("slot should be free")
	}
	if _, ok := l.Acquire("k"); ok {
		t.Fatal("only one slot should exist")
	}
}

func TestNilAndZero_Unlimited(t *testing.T) {
	var nilL *Limiter
	for i := 0; i < 1000; i++ {
		if _, ok := nilL.Acquire("x"); !ok {
			t.Fatal("nil limiter must always allow")
		}
	}
	z := New(0)
	for i := 0; i < 1000; i++ {
		if _, ok := z.Acquire("x"); !ok {
			t.Fatal("max<=0 must always allow")
		}
	}
}

func TestKeyMapDrainsToZero(t *testing.T) {
	l := New(4)
	rels := make([]func(), 0, 4)
	for i := 0; i < 4; i++ {
		r, ok := l.Acquire("k")
		if !ok {
			t.Fatalf("acquire %d should succeed", i)
		}
		rels = append(rels, r)
	}
	for _, r := range rels {
		r()
	}
	if got := active(l, "k"); got != 0 {
		t.Fatalf("active = %d, want 0 (map should drain)", got)
	}
}

func TestConcurrentAcquireRelease(t *testing.T) {
	l := New(8)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r, ok := l.Acquire("shared"); ok {
				r()
			}
		}()
	}
	wg.Wait()
	if got := active(l, "shared"); got != 0 {
		t.Fatalf("active = %d, want 0 after all release", got)
	}
}
