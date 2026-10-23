// SPDX-License-Identifier: MIT

// Round-7 reproduction: toolforge.go Update() panics instead of returning an error.
//
// BUG: Update() calls mutate(st) while holding s.mu.Lock(). A panic in mutate
// propagates to the caller instead of being caught and returned as an error.
// After the fix (safeCall wrapper), Update() returns the panic as an error.
package toolforge

import "testing"

func TestUpdate_RecoverPanicFromMutate(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Seed a script tool matching the existing test fixture structure.
	seed := ScriptTool{
		Name:        "echo",
		Description: "echoes its input",
		Language:    "python",
		Code:        "print(open('stdin.txt').read())",
	}
	if _, err := s.Add(seed); err != nil {
		t.Fatalf("Add seed tool: %v", err)
	}

	// After the fix: Update must return an error, NOT panic.
	_, err = s.Update("echo", func(st *ScriptTool) {
		panic("panic in mutate")
	})
	if err == nil {
		t.Fatal("FAIL: Update() returned nil — fix is NOT applied. " +
			"Panic should be recovered and returned as an error.")
	}
	t.Logf("Update() correctly returned error: %v", err)
}
