// SPDX-License-Identifier: MIT

package plugin

// White-box test for the advertised-tool-count cap (M182).

import (
	"errors"
	"testing"
)

func TestCapAdvertisedTools(t *testing.T) {
	mk := func(n int) []ToolDef {
		out := make([]ToolDef, n)
		for i := range out {
			out[i] = ToolDef{Name: "t"}
		}
		return out
	}
	if err := capAdvertisedTools(mk(3), 5); err != nil {
		t.Errorf("under cap: unexpected err %v", err)
	}
	if err := capAdvertisedTools(mk(5), 5); err != nil {
		t.Errorf("at cap: unexpected err %v", err)
	}
	err := capAdvertisedTools(mk(6), 5)
	if !errors.Is(err, ErrTooManyTools) {
		t.Errorf("over cap: err = %v want ErrTooManyTools", err)
	}
}
