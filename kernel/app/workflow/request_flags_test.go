// SPDX-License-Identifier: MIT

package workflow

import (
	"encoding/json"
	"testing"
)

// Preserve the original native web-facing boolean contract at its typed decoder.

func TestWorkflowRequestFlagAcceptsTheStringsAQueryCanCarry(t *testing.T) {
	t.Parallel()
	for _, in := range []any{true, "true", "TRUE", " yes ", "1"} {
		v, present, err := workflowFlagFixture(map[string]any{"f": in}, "f")
		if err != nil || !present || !v {
			t.Fatalf("workflowFlagFixture(%#v) = (%v, %v, %v), want (true, true, nil)", in, v, present, err)
		}
	}
	for _, in := range []any{false, "false", "FALSE", " no ", "0"} {
		v, present, err := workflowFlagFixture(map[string]any{"f": in}, "f")
		if err != nil || !present || v {
			t.Fatalf("workflowFlagFixture(%#v) = (%v, %v, %v), want (false, true, nil)", in, v, present, err)
		}
	}
}

func TestWorkflowRequestFlagDistinguishesAbsentFromFalse(t *testing.T) {
	t.Parallel()
	v, present, err := workflowFlagFixture(map[string]any{}, "f")
	if v || present || err != nil {
		t.Fatalf("absent flag = (%v, %v, %v), want (false, false, nil)", v, present, err)
	}
}

func TestWorkflowRequestFlagRejectsATypoRatherThanReadingItAsFalse(t *testing.T) {
	t.Parallel()
	// Silently treating "ture" as false is how a flag stops working without
	// anyone noticing — the exact failure argBool's strictness exists to stop.
	for _, in := range []any{"ture", "", 1.0, []any{true}} {
		if _, present, err := workflowFlagFixture(map[string]any{"f": in}, "f"); err == nil || !present {
			t.Fatalf("workflowFlagFixture(%#v) accepted a bad value (present=%v, err=%v)", in, present, err)
		}
	}
}

func workflowFlagFixture(args map[string]any, key string) (bool, bool, error) {
	value, present := args[key]
	var raw json.RawMessage
	if present {
		encoded, err := json.Marshal(value)
		if err != nil {
			return false, true, err
		}
		raw = encoded
	}
	out, err := requestFlag(raw, key)
	return out, present, err
}
