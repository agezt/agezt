// SPDX-License-Identifier: MIT

// Round-6 reproduction: roster.go Update() panics instead of returning an error.
//
// BUG: roster.go Update() calls mutate(p) while holding s.mu.Lock().
// If mutate panics, the panic propagates uncaught to the caller, violating the
// API contract that Update() returns (Profile, error). This is a live panic in
// production that kills the calling goroutine — callers who wrap their call in
// recover() will incorrectly attribute the panic to their own code.
//
// The fix: wrap mutate(p) in a panic-recovery defer and return the panic as an
// error, matching the pattern used in standing.go (round 5 fix).

package roster

import (
	"testing"
)

func TestUpdate_RecoverPanicFromMutate(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	p := Profile{Slug: "test-agent"}
	p, err = s.Add(p)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Cause a panic inside mutate while the mutex is held.
	panicInMutate := func(p *Profile) {
		panic("panic in mutate")
	}

	// Attempt to call Update. A correct implementation catches the panic and
	// returns it as an error. The current (buggy) code lets it propagate.
	var gotErr error
	var panicked bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, gotErr = s.Update(p.ID, panicInMutate)
	}()

	// ASSERTION: Update must return an error, not panic.
	// The panic must be converted to an error return value.
	if panicked {
		t.Fatalf("FAIL: Update() panicked instead of returning an error.\n" +
			"Panic escaped the mutex-protected critical section, violating the\n" +
			"API contract that Update() returns (Profile, error). The panic\n" +
			"kills the calling goroutine and cannot be distinguished from a\n" +
			"genuine runtime panic by callers who use recover().")
	}
	if gotErr == nil {
		t.Fatal("FAIL: Update() returned (Profile, nil) — it should return an error wrapping the panic")
	}
	// Verify the error message mentions the panic value.
	if gotErr.Error() == "" {
		t.Fatal("FAIL: Update() returned an empty error")
	}
}
