// SPDX-License-Identifier: MIT

package controlplane

import "testing"

// The Web UI reaches read commands through a query string, which carries text
// and nothing else — there is no boolean to send. argBool rejects "true"
// outright, which is right for the typed protocol path and wrong for anything
// the browser can reach. argFlag is the web-facing reader.

func TestArgFlagAcceptsTheStringsAQueryCanCarry(t *testing.T) {
	t.Parallel()
	for _, in := range []any{true, "true", "TRUE", " yes ", "1"} {
		v, present, err := argFlag(map[string]any{"f": in}, "f")
		if err != nil || !present || !v {
			t.Fatalf("argFlag(%#v) = (%v, %v, %v), want (true, true, nil)", in, v, present, err)
		}
	}
	for _, in := range []any{false, "false", "FALSE", " no ", "0"} {
		v, present, err := argFlag(map[string]any{"f": in}, "f")
		if err != nil || !present || v {
			t.Fatalf("argFlag(%#v) = (%v, %v, %v), want (false, true, nil)", in, v, present, err)
		}
	}
}

func TestArgFlagDistinguishesAbsentFromFalse(t *testing.T) {
	t.Parallel()
	v, present, err := argFlag(map[string]any{}, "f")
	if v || present || err != nil {
		t.Fatalf("absent flag = (%v, %v, %v), want (false, false, nil)", v, present, err)
	}
}

func TestArgFlagRejectsATypoRatherThanReadingItAsFalse(t *testing.T) {
	t.Parallel()
	// Silently treating "ture" as false is how a flag stops working without
	// anyone noticing — the exact failure argBool's strictness exists to stop.
	for _, in := range []any{"ture", "", 1.0, []any{true}} {
		if _, present, err := argFlag(map[string]any{"f": in}, "f"); err == nil || !present {
			t.Fatalf("argFlag(%#v) accepted a bad value (present=%v, err=%v)", in, present, err)
		}
	}
}
