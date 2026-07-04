// SPDX-License-Identifier: MIT

package seat

import "testing"

func TestBuiltinsAndGet(t *testing.T) {
	b := Builtins()
	if len(b) < 4 {
		t.Fatalf("expected at least 4 seeded seats, got %d", len(b))
	}
	if b[0].ID != "default" {
		t.Fatalf("first seat = %q, want default", b[0].ID)
	}

	// Reader is a restricted, read-only tool tier with no isolation profile.
	r, ok := Get("reader")
	if !ok || !r.RestrictTools || len(r.Tools) == 0 || r.ExecutionProfile != "" {
		t.Fatalf("reader = %+v ok=%v", r, ok)
	}

	// Isolated pins the warden execution profile with full tools.
	iso, ok := Get("ISOLATED") // case-insensitive
	if !ok || iso.ExecutionProfile != "warden" || iso.RestrictTools {
		t.Fatalf("isolated = %+v ok=%v", iso, ok)
	}

	// Empty and "default" both resolve to the inherit-all seat.
	d1, ok1 := Get("")
	d2, ok2 := Get("default")
	if !ok1 || !ok2 || d1.ID != "default" || d2.ID != "default" {
		t.Fatalf("default resolution: %+v/%v %+v/%v", d1, ok1, d2, ok2)
	}
	if d1.ExecutionProfile != "" || d1.RestrictTools {
		t.Fatalf("default should override nothing: %+v", d1)
	}

	if _, ok := Get("nope"); ok {
		t.Fatal("unknown seat should not resolve")
	}
}

func TestValid(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("open seat store: %v", err)
	}
	if !st.Valid("") || !st.Valid("reader") || !st.Valid("Builder") {
		t.Fatal("expected empty/reader/Builder valid")
	}
	if st.Valid("bogus") {
		t.Fatal("bogus should be invalid")
	}
}

func TestBuiltinsIsCopy(t *testing.T) {
	b := Builtins()
	if len(b[1].Tools) > 0 {
		b[1].Tools[0] = "MUTATED"
	}
	if r, _ := Get("reader"); len(r.Tools) > 0 && r.Tools[0] == "MUTATED" {
		t.Fatal("Builtins() must return copies, not shared slices")
	}
}
