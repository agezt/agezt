// SPDX-License-Identifier: MIT

package runs

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The run's argument readers are strict: a flag that arrives with the wrong
// JSON type is a usage error, never a silent default.

func TestArgString(t *testing.T) {
	if v, ok, err := argString(json.RawMessage(`"hello"`), "s"); v != "hello" || !ok || err != nil {
		t.Errorf("present string: got (%q,%v,%v)", v, ok, err)
	}
	if v, ok, err := argString(nil, "missing"); v != "" || ok || err != nil {
		t.Errorf("absent: got (%q,%v,%v), want ('',false,nil)", v, ok, err)
	}
	// Present but wrong type → ok=true (present) AND err!=nil (so the caller
	// errors instead of using the zero value).
	if _, ok, err := argString(json.RawMessage(`5`), "n"); !ok || err == nil {
		t.Errorf("number-as-string: got (ok=%v,err=%v), want (true, non-nil)", ok, err)
	}
	if _, _, err := argString(json.RawMessage(`true`), "b"); err == nil {
		t.Error("bool-as-string: want error")
	}
	// A JSON null is not a string either.
	if _, ok, err := argString(json.RawMessage(`null`), "n"); !ok || err == nil {
		t.Errorf("null-as-string: got (ok=%v,err=%v), want (true, non-nil)", ok, err)
	}
}

func TestArgBool(t *testing.T) {
	if v, ok, err := argBool(json.RawMessage(`true`), "b"); !v || !ok || err != nil {
		t.Errorf("present bool: got (%v,%v,%v)", v, ok, err)
	}
	if v, ok, err := argBool(nil, "missing"); v || ok || err != nil {
		t.Errorf("absent: got (%v,%v,%v), want (false,false,nil)", v, ok, err)
	}
	// The dangerous case: dry_run sent as the string "true". Must be an error,
	// not a silent false (which would execute the run the operator meant to
	// preview).
	if _, ok, err := argBool(json.RawMessage(`"true"`), "s"); !ok || err == nil {
		t.Errorf("string-as-bool: got (ok=%v,err=%v), want (true, non-nil)", ok, err)
	}
	if _, _, err := argBool(json.RawMessage(`1`), "n"); err == nil {
		t.Error("number-as-bool: want error")
	}
}

func TestArgInt64(t *testing.T) {
	if v, ok, err := argInt64(json.RawMessage(`1500`), "n"); v != 1500 || !ok || err != nil {
		t.Errorf("present number: got (%d,%v,%v)", v, ok, err)
	}
	if v, ok, err := argInt64(nil, "missing"); v != 0 || ok || err != nil {
		t.Errorf("absent: got (%d,%v,%v), want (0,false,nil)", v, ok, err)
	}
	// A numeric value sent as a string is a type error, not a silent 0.
	if _, ok, err := argInt64(json.RawMessage(`"1500"`), "s"); !ok || err == nil {
		t.Errorf("string-as-number: got (ok=%v,err=%v), want (true, non-nil)", ok, err)
	}
	if _, _, err := argInt64(json.RawMessage(`true`), "b"); err == nil {
		t.Error("bool-as-number: want error")
	}
}

func TestArgStringList(t *testing.T) {
	if v, ok, err := argStringList(nil, "missing"); v != nil || ok || err != nil {
		t.Errorf("absent: got (%v,%v,%v)", v, ok, err)
	}
	// Present empty array → ok=true (restriction set), zero names = no tools.
	if v, ok, err := argStringList(json.RawMessage(`[]`), "empty"); len(v) != 0 || !ok || err != nil {
		t.Errorf("empty array: got (%v,%v,%v), want ([],true,nil)", v, ok, err)
	}
	if v, ok, err := argStringList(json.RawMessage(`["shell"," file ","","  "]`), "names"); err != nil || !ok || len(v) != 2 || v[0] != "shell" || v[1] != "file" {
		t.Errorf("names: got (%v,%v,%v), want ([shell file],true,nil)", v, ok, err)
	}
	// Present but a bare string (not an array) → error, NOT a silent empty list
	// (which would scope the run to zero tools).
	if _, ok, err := argStringList(json.RawMessage(`"shell"`), "notarray"); !ok || err == nil {
		t.Errorf("string-as-array: got (ok=%v,err=%v), want (true, non-nil)", ok, err)
	}
	if _, _, err := argStringList(json.RawMessage(`["ok",3]`), "badelem"); err == nil {
		t.Error("non-string element: want error")
	}
}

func TestParseCapList(t *testing.T) {
	// A comma/space-separated string.
	got := parseCapList(json.RawMessage(`"a, b c"`))
	if !reflect.DeepEqual(got, map[string]bool{"a": true, "b": true, "c": true}) {
		t.Errorf("string caps = %v", got)
	}
	// A JSON array of strings.
	got = parseCapList(json.RawMessage(`["x","y"]`))
	if !reflect.DeepEqual(got, map[string]bool{"x": true, "y": true}) {
		t.Errorf("array caps = %v", got)
	}
	// Nothing to grant reads as no grant at all.
	if got := parseCapList(nil); got != nil {
		t.Errorf("absent caps = %v, want nil", got)
	}
	if got := parseCapList(json.RawMessage(`" , "`)); got != nil {
		t.Errorf("blank caps = %v, want nil", got)
	}
}

func TestArgTruthy(t *testing.T) {
	// The truthy tokens, case- and space-insensitive.
	for _, raw := range []string{`true`, `"on"`, `"true"`, `"yes"`, `"1"`, `"ON"`, `"True"`, `" Yes "`, `" 1 "`} {
		if !argTruthy(json.RawMessage(raw)) {
			t.Errorf("argTruthy(%s) = false, want true", raw)
		}
	}
	// Everything else is false: rejected strings, null and non-string values.
	for _, raw := range []string{`false`, `"off"`, `"false"`, `"no"`, `"0"`, `""`, `"maybe"`, `null`, `42`, `1.5`, `["1"]`, `{"x":1}`} {
		if argTruthy(json.RawMessage(raw)) {
			t.Errorf("argTruthy(%s) = true, want false", raw)
		}
	}
}
