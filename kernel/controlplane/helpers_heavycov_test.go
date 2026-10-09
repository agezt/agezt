// SPDX-License-Identifier: MIT

package controlplane

import (
	"testing"
)

// TestArgTruthy_AllForms drives argTruthy through every branch: real bools, the
// accepted truthy string tokens (case/space-insensitive), rejected strings, and
// non-string/non-bool values (which are always false).
func TestArgTruthy_AllForms(t *testing.T) {
	truthy := []any{
		true,
		"on", "true", "yes", "1",
		"ON", "True", " Yes ", " 1 ",
	}
	for _, v := range truthy {
		if !argTruthy(v) {
			t.Errorf("argTruthy(%#v) = false, want true", v)
		}
	}

	falsy := []any{
		false,
		"off", "false", "no", "0", "", "maybe",
		nil,
		42,         // non-string number
		float64(1), // float is not accepted (only bool/string)
		[]string{"1"},
		map[string]any{"x": 1},
	}
	for _, v := range falsy {
		if argTruthy(v) {
			t.Errorf("argTruthy(%#v) = true, want false", v)
		}
	}
}
