// SPDX-License-Identifier: MIT

package runs

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

// The run's arguments are strict: a mistyped field is refused by the row
// before the family reads it, and the family's own readers say what a value
// means.

func TestTheRowsRefuseAMistypedArgument(t *testing.T) {
	// A list of texts is a list, and nothing else: a member that is not a
	// text is refused, the way the door refuses it.
	for _, raw := range []string{`"shell"`, `5`, `true`, `{}`, `["ok",3]`} {
		decodeError[opapi.TextArray](t, raw)
	}
	// The trust flag carries a boolean or the string a query string carries
	// for one: the row keeps both, and the door says which two it admits.
	flag := decode[opapi.BoolFlag](t, `"on"`)
	if flag.Text() != "on" {
		t.Fatalf("the flag's own spelling = %q, want on", flag.Text())
	}
	// A count is a number, not the string that carries one.
	for _, raw := range []string{`"1500"`, `true`, `{}`} {
		decodeError[opapi.Limit](t, raw)
	}
}

func TestTextList(t *testing.T) {
	// A list that was not sent is not a list at all.
	if v, ok := textList(opapi.TextArray{}); v != nil || ok {
		t.Errorf("absent: got (%v,%v), want (nil,false)", v, ok)
	}
	// Present empty array → ok=true (the restriction is set), zero names = no
	// tools at all.
	if v, ok := textList(decode[opapi.TextArray](t, `[]`)); len(v) != 0 || !ok {
		t.Errorf("empty array: got (%v,%v), want ([],true)", v, ok)
	}
	// The members are trimmed and the blank ones dropped.
	v, ok := textList(decode[opapi.TextArray](t, `["shell"," file ","","  "]`))
	if !ok || len(v) != 2 || v[0] != "shell" || v[1] != "file" {
		t.Errorf("names: got (%v,%v), want ([shell file],true)", v, ok)
	}
}

func TestApproveCaps(t *testing.T) {
	// A comma/space-separated string.
	got := approveCaps(decode[opapi.StrictTextList](t, `"a, b c"`))
	if !reflect.DeepEqual(got, map[string]bool{"a": true, "b": true, "c": true}) {
		t.Errorf("string caps = %v", got)
	}
	// A JSON array of texts.
	got = approveCaps(decode[opapi.StrictTextList](t, `["x","y"]`))
	if !reflect.DeepEqual(got, map[string]bool{"x": true, "y": true}) {
		t.Errorf("array caps = %v", got)
	}
	// Nothing to grant reads as no grant at all.
	if got := approveCaps(opapi.StrictTextList{}); got != nil {
		t.Errorf("absent caps = %v, want nil", got)
	}
	if got := approveCaps(decode[opapi.StrictTextList](t, `" , "`)); got != nil {
		t.Errorf("blank caps = %v, want nil", got)
	}
	// A member that is not a text is refused by the row itself.
	decodeError[opapi.StrictTextList](t, `[7]`)
}

func TestTruthy(t *testing.T) {
	// The truthy tokens, case- and space-insensitive.
	for _, raw := range []string{`true`, `"on"`, `"true"`, `"yes"`, `"1"`, `"ON"`, `"True"`, `" Yes "`, `" 1 "`} {
		if !truthy(decode[opapi.BoolFlag](t, raw)) {
			t.Errorf("truthy(%s) = false, want true", raw)
		}
	}
	// Everything else is false: rejected strings, null and non-string values.
	for _, raw := range []string{`false`, `"off"`, `"false"`, `"no"`, `"0"`, `""`, `"maybe"`, `null`, `42`, `1.5`, `["1"]`, `{"x":1}`} {
		if truthy(decode[opapi.BoolFlag](t, raw)) {
			t.Errorf("truthy(%s) = true, want false", raw)
		}
	}
}
