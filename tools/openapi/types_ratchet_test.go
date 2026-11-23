// SPDX-License-Identifier: MIT

package main

// The browser's VIEW of the daemon's contract must not narrow it.
//
// The renderer decides what the browser sees: a member the daemon ALWAYS sends
// keeps the null arm (the browser would otherwise read a null as a value), a
// member the daemon MAY omit carries the `?` instead (and the `?` already
// forces the guard a null needs). A field's nullability is the daemon's
// business — the browser may not promise a value the daemon may send as null.
//
// These tests pin the RULES the renderer follows, because the browser's types
// are generated from them: a rule that drifts turns every generated type into
// a promise the daemon does not keep.

import (
	"encoding/json"
	"strings"
	"testing"
)

// renderOne renders one schema and returns the type expression it produced
// and the body of the declaration it named ("Row" for an object schema).
func renderOne(t *testing.T, schema string) (string, string) {
	t.Helper()
	r := &tsRenderer{
		decls:     map[string]string{},
		aliases:   map[string]string{},
		canonical: map[string]string{},
		mode:      mode(),
	}
	out, err := r.renderType("Row", json.RawMessage(schema))
	if err != nil {
		t.Fatalf("render %s: %v", schema, err)
	}
	return out, r.decls["Row"]
}

// TestARequiredNullableMemberKeepsTheNullArm: the daemon always sends this
// member, so the browser has no `?` to lean on — the null arm is the only
// warning it gets that the value may be null.
func TestARequiredNullableMemberKeepsTheNullArm(t *testing.T) {
	_, body := renderOne(t, `{"type":"object","properties":{"a":{"type":["string","null"]}},"required":["a"]}`)
	if !strings.Contains(body, "a: string | null;") {
		t.Fatalf("a required nullable member lost its null arm: %s", body)
	}
}

// TestAnOptionalNullableMemberDropsTheNullArm: the daemon may omit this
// member, so the `?` already forces the guard; the null arm would only make
// the type noisier.
func TestAnOptionalNullableMemberDropsTheNullArm(t *testing.T) {
	_, body := renderOne(t, `{"type":"object","properties":{"a":{"type":["string","null"]}}}`)
	if !strings.Contains(body, "a?: string;") {
		t.Fatalf("an optional nullable member did not become an optional string: %s", body)
	}
}

// TestANullableArrayItemIsParenthesised: "X | null[]" is an array of nulls,
// not an array of possibly-null elements. The renderer must parenthesise the
// element union.
func TestANullableArrayItemIsParenthesised(t *testing.T) {
	out, _ := renderOne(t, `{"type":"array","items":{"type":["object","null"],"properties":{"a":{"type":"string"}}}}`)
	if strings.Contains(out, "| null[]") {
		t.Fatalf("the null arm landed on the array instead of its element: %s", out)
	}
	if !strings.Contains(out, "(RowItem | null)[]") {
		t.Fatalf("the element's null arm is not parenthesised: %s", out)
	}
}
