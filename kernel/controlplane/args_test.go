// SPDX-License-Identifier: MIT

package controlplane

import "testing"

func TestArgString(t *testing.T) {
	args := map[string]any{"s": "hello", "n": float64(5), "b": true}

	if v, ok, err := argString(args, "s"); v != "hello" || !ok || err != nil {
		t.Errorf("present string: got (%q,%v,%v)", v, ok, err)
	}
	if v, ok, err := argString(args, "missing"); v != "" || ok || err != nil {
		t.Errorf("absent: got (%q,%v,%v), want ('',false,nil)", v, ok, err)
	}
	// Present but wrong type → ok=true (present) AND err!=nil (so the caller errors
	// instead of using the zero value).
	if _, ok, err := argString(args, "n"); !ok || err == nil {
		t.Errorf("number-as-string: got (ok=%v,err=%v), want (true, non-nil)", ok, err)
	}
	if _, _, err := argString(args, "b"); err == nil {
		t.Error("bool-as-string: want error")
	}
}

func TestArgBool(t *testing.T) {
	args := map[string]any{"b": true, "s": "true", "n": float64(1)}

	if v, ok, err := argBool(args, "b"); !v || !ok || err != nil {
		t.Errorf("present bool: got (%v,%v,%v)", v, ok, err)
	}
	if v, ok, err := argBool(args, "missing"); v || ok || err != nil {
		t.Errorf("absent: got (%v,%v,%v), want (false,false,nil)", v, ok, err)
	}
	// The dangerous case: dry_run sent as the string "true". Must be an error, not
	// a silent false (which would execute the run the operator meant to preview).
	if _, ok, err := argBool(args, "s"); !ok || err == nil {
		t.Errorf("string-as-bool: got (ok=%v,err=%v), want (true, non-nil)", ok, err)
	}
	if _, _, err := argBool(args, "n"); err == nil {
		t.Error("number-as-bool: want error")
	}
}

func TestArgInt64(t *testing.T) {
	args := map[string]any{"n": float64(1500), "s": "1500", "b": true}

	if v, ok, err := argInt64(args, "n"); v != 1500 || !ok || err != nil {
		t.Errorf("present number: got (%d,%v,%v)", v, ok, err)
	}
	if v, ok, err := argInt64(args, "missing"); v != 0 || ok || err != nil {
		t.Errorf("absent: got (%d,%v,%v), want (0,false,nil)", v, ok, err)
	}
	// A numeric value sent as a string is a type error, not a silent 0.
	if _, ok, err := argInt64(args, "s"); !ok || err == nil {
		t.Errorf("string-as-number: got (ok=%v,err=%v), want (true, non-nil)", ok, err)
	}
	if _, _, err := argInt64(args, "b"); err == nil {
		t.Error("bool-as-number: want error")
	}
}

func TestArgStringList(t *testing.T) {
	args := map[string]any{
		"empty":    []any{},
		"names":    []any{"shell", " file ", "", "  "}, // trims, skips empties
		"notarray": "shell",                            // the silent-no-tools trap
		"badelem":  []any{"ok", float64(3)},
	}

	if v, ok, err := argStringList(args, "missing"); v != nil || ok || err != nil {
		t.Errorf("absent: got (%v,%v,%v)", v, ok, err)
	}
	// Present empty array → ok=true (restriction set), zero names = --no-tools.
	if v, ok, err := argStringList(args, "empty"); len(v) != 0 || !ok || err != nil {
		t.Errorf("empty array: got (%v,%v,%v), want ([],true,nil)", v, ok, err)
	}
	if v, ok, err := argStringList(args, "names"); err != nil || !ok || len(v) != 2 || v[0] != "shell" || v[1] != "file" {
		t.Errorf("names: got (%v,%v,%v), want ([shell file],true,nil)", v, ok, err)
	}
	// Present but a bare string (not an array) → error, NOT a silent empty list
	// (which would scope the run to zero tools).
	if _, ok, err := argStringList(args, "notarray"); !ok || err == nil {
		t.Errorf("string-as-array: got (ok=%v,err=%v), want (true, non-nil)", ok, err)
	}
	if _, _, err := argStringList(args, "badelem"); err == nil {
		t.Error("non-string element: want error")
	}
}
