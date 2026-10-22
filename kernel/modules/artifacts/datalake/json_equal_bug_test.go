// SPDX-License-Identifier: MIT

// json_equal_bug_test.go — regression test for datalake.go:554-558
//
// Bug: jsonEqual() silently ignores json.Marshal errors.
// Two unserializable values (e.g., a function and a channel) both marshal
// to "null" with a non-nil error — jsonEqual returns true incorrectly.
// This causes Query's Equals filter to produce false-positive matches
// when comparing records with unserializable field types.

package datalake

import "testing"

func TestJSONEqual_SilentMarshalErrors(t *testing.T) {
	ch := make(chan int)
	fn := func() {}

	// json.Marshal on a channel returns []byte("null") and a non-nil error.
	// json.Marshal on a function returns []byte("null") and a non-nil error.
	// jsonEqual silently drops both errors and compares the byte slices.
	// "null" == "null" → jsonEqual returns true.
	// This is a false positive: a channel is not equal to a function.
	if jsonEqual(ch, fn) {
		t.Fatal("BUG: jsonEqual(chan, func) returned true — both marshal to \"null\" " +
			"with non-nil errors, which jsonEqual silently drops. " +
			"Query Equals filter will produce false-positive matches on unserializable fields.")
	}
}

func TestJSONEqual_KnownSerializable(t *testing.T) {
	// Sanity: serializable values that are actually equal should return true.
	if !jsonEqual("hello", "hello") {
		t.Error("jsonEqual should return true for equal strings")
	}
	if jsonEqual("hello", "world") {
		t.Error("jsonEqual should return false for different strings")
	}
	if !jsonEqual(42, 42) {
		t.Error("jsonEqual should return true for equal ints")
	}
	if !jsonEqual(map[string]any{"a": 1}, map[string]any{"a": 1}) {
		t.Error("jsonEqual should return true for equal maps")
	}
}
