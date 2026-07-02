// SPDX-License-Identifier: MIT

package seat

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStoreRecoversFromTemp simulates the Windows atomic-write fallback crash:
// the main seats.json was removed but the retry rename never landed, leaving the
// only copy in seats.json.tmp. OpenStore must recover it, not start empty.
func TestStoreRecoversFromTemp(t *testing.T) {
	dir := t.TempDir()
	st, _ := OpenStore(dir)
	if _, err := st.Create(Seat{ID: "gpu-box", Name: "GPU Box", ExecutionProfile: "container"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	main := filepath.Join(dir, "seats.json")
	b, err := os.ReadFile(main)
	if err != nil {
		t.Fatalf("read main: %v", err)
	}
	// Recreate the crash: only the temp survives.
	if err := os.WriteFile(main+".tmp", b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(main); err != nil {
		t.Fatal(err)
	}

	re, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, ok := re.Get("gpu-box"); !ok {
		t.Fatal("custom seat not recovered from temp file")
	}
	// Recovery renames the temp back into place.
	if _, err := os.Stat(main); err != nil {
		t.Fatalf("temp not promoted to main: %v", err)
	}

	// A corrupt temp with no main file starts empty rather than failing boot.
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, "seats.json.tmp"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty, err := OpenStore(dir2)
	if err != nil {
		t.Fatalf("corrupt temp should not fail boot: %v", err)
	}
	if len(empty.List()) != len(Builtins()) {
		t.Fatal("corrupt temp should yield only built-ins")
	}
}

func TestStoreCreateGetListDelete(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	// List starts as the built-ins, all marked Builtin.
	base := st.List()
	if len(base) < 4 {
		t.Fatalf("expected built-ins, got %d", len(base))
	}
	for _, s := range base {
		if !s.Builtin {
			t.Fatalf("built-in %q not marked", s.ID)
		}
	}

	// Create a custom seat.
	made, err := st.Create(Seat{ID: "gpu-box", Name: "GPU Box", ExecutionProfile: "container"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if made.Builtin || made.ExecutionProfile != "container" {
		t.Fatalf("made = %+v", made)
	}
	// Resolves and appears in the list after built-ins.
	got, ok := st.Get("gpu-box")
	if !ok || got.Name != "GPU Box" {
		t.Fatalf("Get custom = %+v ok=%v", got, ok)
	}
	if len(st.List()) != len(base)+1 {
		t.Fatalf("custom seat not listed")
	}

	// Persists across reopen.
	re, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, ok := re.Get("gpu-box"); !ok {
		t.Fatal("custom seat did not persist")
	}

	// Delete it; built-ins are refused.
	if err := re.Delete("gpu-box"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := re.Get("gpu-box"); ok {
		t.Fatal("custom seat still present after delete")
	}
	if err := re.Delete("builder"); err != ErrBuiltin {
		t.Fatalf("deleting built-in err = %v, want ErrBuiltin", err)
	}
}

func TestStoreCreateValidation(t *testing.T) {
	st, _ := OpenStore(t.TempDir())
	cases := []struct {
		name string
		spec Seat
		want error
	}{
		{"builtin id", Seat{ID: "reader"}, ErrExists},
		{"bad id", Seat{ID: "Bad ID!"}, ErrInvalidID},
		{"bad iso", Seat{ID: "x1", ExecutionProfile: "ssh"}, ErrInvalidIso},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := st.Create(c.spec); err != c.want {
				t.Fatalf("Create(%+v) err = %v, want %v", c.spec, err, c.want)
			}
		})
	}
	// A valid create with an empty iso and a tool list restricts tools.
	made, err := st.Create(Seat{ID: "readonly2", Tools: []string{"web_search", "fetch"}})
	if err != nil {
		t.Fatalf("valid create: %v", err)
	}
	if !made.RestrictTools {
		t.Fatal("tools set should imply RestrictTools")
	}
}
