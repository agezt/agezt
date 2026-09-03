// SPDX-License-Identifier: MIT

// Round-5 elite-bug-hunter reproduction: kernel/standing/standing.go
// panic escape from mutex-protected critical sections.
//
// BUG: kernel/standing/standing.go
//   - Update() held s.mu.Lock() across mutate(*Order) without recover.
//     If mutate panicked, the panic escaped to callers.
//   - SetEnabled() held s.mu.Lock() across save() without recover.
//     If save panicked, the panic escaped to callers.
//
// The fix adds a safeCall() helper (matching the safeFire pattern in
// runner.go) to contain panics, returning them as errors instead.
//
// Post-fix: both functions return an error, the mutex is released, and
// List() continues to work normally.
package standing

import (
	"strings"
	"testing"
)

func TestUpdate_RecoverPanicFromMutate(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	o, err := s.Add(Order{Name: "test", Triggers: []Trigger{{Type: TriggerEvent, Subject: "test.>"}}})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Update with a panicking mutate: should return an error, not panic.
	_, err = s.Update(o.ID, func(*Order) {
		panic("panic in mutate")
	})
	if err == nil {
		t.Fatal("Update with panicking mutate: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "mutate panicked") {
		t.Fatalf("Update error should mention 'mutate panicked', got: %v", err)
	}
	t.Log("Update correctly returned error:", err)

	// Store must still be usable after the panic — List() must not panic or block.
	orders := s.List()
	if len(orders) != 1 {
		t.Fatalf("List() returned %d orders, want 1", len(orders))
	}
}

func TestSetEnabled_RecoverPanicFromMutate(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	o, err := s.Add(Order{Name: "test", Triggers: []Trigger{{Type: TriggerEvent, Subject: "test.>"}}})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Use Update (which calls SetEnabled's save path) with a panicking mutate.
	_, err = s.Update(o.ID, func(o *Order) {
		panic("panic during SetEnabled's save path trigger")
	})
	if err == nil {
		t.Fatal("Update with panicking mutate: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("Update error should mention 'panicked', got: %v", err)
	}
	t.Log("Update correctly returned error:", err)

	// Store must still be usable after the panic — List() must not panic or block.
	orders := s.List()
	if len(orders) != 1 {
		t.Fatalf("List() returned %d orders, want 1", len(orders))
	}
}
